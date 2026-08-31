package manifest

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"nilo.local/nilo/internal/config"
)

const (
	maxDocumentBytes      = 1 << 20
	maxOperationTimeout   = time.Hour
	maxHealthCheckTimeout = time.Minute
)

//go:embed schemas/*.json
var builtinSchemaFS embed.FS

// Load validates the package manifest and every referenced local schema. All
// returned file paths are resolved beneath packageRoot.
func Load(packageRoot string) (Manifest, error) {
	root, err := canonicalPackageRoot(packageRoot)
	if err != nil {
		return Manifest{}, fmt.Errorf("%w: package root: %v", ErrInvalidManifest, err)
	}
	manifestPath, err := config.ResolveWithin(root, "manifest.json")
	if err != nil {
		return Manifest{}, fmt.Errorf("%w: manifest path: %v", ErrInvalidManifest, err)
	}
	document, err := readJSONDocument(manifestPath)
	if err != nil {
		return Manifest{}, err
	}

	var result Manifest
	if err := json.Unmarshal(document, &result); err != nil {
		return Manifest{}, fmt.Errorf("%w: decode manifest: %v", ErrInvalidManifest, err)
	}
	if err := validateManifestDocument(document); err != nil {
		return Manifest{}, err
	}
	if err := validateManifest(&result, root); err != nil {
		return Manifest{}, err
	}
	return result, nil
}

func canonicalPackageRoot(packageRoot string) (string, error) {
	absRoot, err := filepath.Abs(packageRoot)
	if err != nil {
		return "", err
	}
	physicalRoot, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(physicalRoot)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("package root is not a directory")
	}
	return physicalRoot, nil
}

func validateManifestDocument(document []byte) error {
	var value any
	decoder := json.NewDecoder(bytes.NewReader(document))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return fmt.Errorf("%w: parse manifest JSON: %v", ErrInvalidManifest, err)
	}
	if err := requireEOF(decoder); err != nil {
		return fmt.Errorf("%w: parse manifest JSON: %v", ErrInvalidManifest, err)
	}

	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	compiler.UseLoader(denySchemaLoader{})
	for _, name := range []string{"manifest.schema.json", "operation.schema.json", "event.schema.json"} {
		schemaDocument, err := builtinSchemaFS.ReadFile("schemas/" + name)
		if err != nil {
			return fmt.Errorf("%w: read embedded schema %s: %v", ErrInvalidManifest, name, err)
		}
		if len(schemaDocument) > maxDocumentBytes {
			return fmt.Errorf("%w: embedded schema %s", ErrDocumentTooLarge, name)
		}
		var schemaValue any
		if err := json.Unmarshal(schemaDocument, &schemaValue); err != nil {
			return fmt.Errorf("%w: parse built-in schema %s: %v", ErrInvalidManifest, name, err)
		}
		if err := compiler.AddResource(builtinSchemaURL(name), schemaValue); err != nil {
			return fmt.Errorf("%w: add built-in schema %s: %v", ErrInvalidManifest, name, err)
		}
	}
	schema, err := compiler.Compile(builtinSchemaURL("manifest.schema.json"))
	if err != nil {
		return fmt.Errorf("%w: compile manifest schema: %v", ErrInvalidManifest, err)
	}
	if err := schema.Validate(value); err != nil {
		return fmt.Errorf("%w: schema validation: %v", ErrInvalidManifest, err)
	}
	return nil
}

func validateManifest(manifest *Manifest, root string) error {
	if manifest.Protocol != protocolV1 {
		return fmt.Errorf("%w: %q", ErrUnsupportedProtocol, manifest.Protocol)
	}
	for _, platform := range manifest.Platforms {
		if !supportedPlatform(platform) {
			return fmt.Errorf("%w: %s/%s", ErrUnsupportedPlatform, platform.OS, platform.Architecture)
		}
	}
	if err := selectRunnerForHost(manifest); err != nil {
		return err
	}
	if len(manifest.Runner.Arguments) == 0 {
		return fmt.Errorf("%w: runner arguments must not be empty", ErrInvalidManifest)
	}
	if err := resolvePath(root, &manifest.Runner.Executable, true); err != nil {
		return err
	}
	if err := resolveSchema(root, &manifest.AppConfigurationSchema); err != nil {
		return fmt.Errorf("%w: app configuration schema: %w", ErrInvalidManifest, err)
	}

	operationIDs := make(map[string]struct{}, len(manifest.Operations))
	for index := range manifest.Operations {
		operation := &manifest.Operations[index]
		if _, exists := operationIDs[operation.ID]; exists {
			return fmt.Errorf("%w: %q", ErrDuplicateOperationID, operation.ID)
		}
		operationIDs[operation.ID] = struct{}{}
		if err := operation.Policy.Validate(); err != nil {
			return fmt.Errorf("%w: operation %q policy: %v", ErrInvalidManifest, operation.ID, err)
		}
		if duration, err := time.ParseDuration(operation.Timeout); err != nil || duration <= 0 || duration > maxOperationTimeout {
			return fmt.Errorf("%w: operation %q timeout", ErrInvalidManifest, operation.ID)
		}
		if err := resolveSchema(root, &operation.InputSchema); err != nil {
			return fmt.Errorf("%w: operation %q input schema: %w", ErrInvalidManifest, operation.ID, err)
		}
		if err := resolveSchema(root, &operation.ResultSchema); err != nil {
			return fmt.Errorf("%w: operation %q result schema: %w", ErrInvalidManifest, operation.ID, err)
		}
		seenResources := make(map[ResourceClaim]struct{}, len(operation.Resources))
		for resourceIndex, resource := range operation.Resources {
			normalized := ResourceClaim(strings.ToLower(strings.TrimSpace(string(resource))))
			if !validResourceClaim(normalized) {
				return fmt.Errorf("%w: operation %q resource claim %q", ErrInvalidManifest, operation.ID, resource)
			}
			if _, exists := seenResources[normalized]; exists {
				return fmt.Errorf("%w: operation %q has duplicate resource claim %q", ErrInvalidManifest, operation.ID, normalized)
			}
			seenResources[normalized] = struct{}{}
			operation.Resources[resourceIndex] = normalized
		}
		slices.Sort(operation.Resources)
	}
	for index := range manifest.Events {
		if err := resolveSchema(root, &manifest.Events[index].Schema); err != nil {
			return fmt.Errorf("%w: event %q schema: %w", ErrInvalidManifest, manifest.Events[index].ID, err)
		}
	}
	for index := range manifest.Artifacts {
		if err := resolveSchema(root, &manifest.Artifacts[index].Schema); err != nil {
			return fmt.Errorf("%w: artifact %q schema: %w", ErrInvalidManifest, manifest.Artifacts[index].ID, err)
		}
	}
	for index := range manifest.ContractFixtures {
		if err := resolvePath(root, &manifest.ContractFixtures[index], true); err != nil {
			return fmt.Errorf("%w: contract fixture: %w", ErrInvalidManifest, err)
		}
	}
	for _, healthCheck := range manifest.HealthChecks {
		if duration, err := time.ParseDuration(healthCheck.Timeout); err != nil || duration <= 0 || duration > maxHealthCheckTimeout {
			return fmt.Errorf("%w: health check %q timeout", ErrInvalidManifest, healthCheck.ID)
		}
	}
	if manifest.Cancellation.Supported {
		if _, exists := operationIDs[manifest.Cancellation.SafeShutdown]; !exists {
			return fmt.Errorf("%w: safe shutdown operation %q is not declared", ErrInvalidManifest, manifest.Cancellation.SafeShutdown)
		}
	}
	return nil
}

func selectRunnerForHost(manifest *Manifest) error {
	for _, override := range manifest.Runner.PlatformOverrides {
		if override.OS == runtime.GOOS && override.Architecture == runtime.GOARCH {
			manifest.Runner.Executable = override.Executable
			manifest.Runner.Arguments = append([]string(nil), override.Arguments...)
			return nil
		}
	}
	return nil
}

func resolveSchema(root string, path *string) error {
	if err := resolvePath(root, path, true); err != nil {
		return err
	}
	document, err := readJSONDocument(*path)
	if err != nil {
		return err
	}
	value, err := decodeJSONDocument(document)
	if err != nil {
		return fmt.Errorf("%w: schema is not JSON: %v", ErrInvalidManifest, err)
	}
	if err := validateSchemaURIs(root, *path, value); err != nil {
		return err
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	compiler.UseLoader(confinedSchemaLoader{root: root})
	if err := compiler.AddResource(*path, value); err != nil {
		return fmt.Errorf("%w: add schema: %v", ErrInvalidManifest, err)
	}
	if _, err := compiler.Compile(*path); err != nil {
		return fmt.Errorf("%w: compile schema: %w", ErrInvalidManifest, err)
	}
	return nil
}

func resolvePath(root string, path *string, requireFile bool) error {
	resolved, err := config.ResolveWithin(root, *path)
	if err != nil {
		return fmt.Errorf("%w: path %q: %v", ErrInvalidManifest, *path, err)
	}
	physical, err := physicalPathWithin(root, resolved, requireFile)
	if err != nil {
		return fmt.Errorf("%w: path %q: %v", ErrInvalidManifest, *path, err)
	}
	if requireFile {
		info, err := os.Stat(physical)
		if err != nil {
			return fmt.Errorf("%w: path %q: %v", ErrInvalidManifest, *path, err)
		}
		if info.IsDir() {
			return fmt.Errorf("%w: path %q is a directory", ErrInvalidManifest, *path)
		}
	}
	*path = physical
	return nil
}

func readJSONDocument(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("%w: open %q: %v", ErrInvalidManifest, path, err)
	}
	defer file.Close()
	reader := io.LimitReader(file, maxDocumentBytes+1)
	document, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("%w: read %q: %v", ErrInvalidManifest, path, err)
	}
	if len(document) > maxDocumentBytes {
		return nil, fmt.Errorf("%w: %q", ErrDocumentTooLarge, path)
	}
	return document, nil
}

func physicalPathWithin(root, candidate string, requireFile bool) (string, error) {
	physicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	physicalCandidate, err := filepath.EvalSymlinks(candidate)
	if err != nil && !requireFile {
		physicalCandidate, err = filepath.Abs(candidate)
	}
	if err != nil {
		return "", err
	}
	if err := requireWithin(physicalRoot, physicalCandidate); err != nil {
		return "", err
	}
	return physicalCandidate, nil
}

func requireWithin(root, candidate string) error {
	relative, err := filepath.Rel(root, candidate)
	if err != nil {
		return err
	}
	components := strings.Split(relative, string(filepath.Separator))
	if relative == ".." || filepath.IsAbs(relative) || components[0] == ".." {
		return fmt.Errorf("path escapes package root")
	}
	return nil
}

func validateSchemaURIs(root, path string, value any) error {
	switch current := value.(type) {
	case map[string]any:
		for key, child := range current {
			if key == "$ref" || key == "$dynamicRef" || key == "$id" {
				if reference, ok := child.(string); ok {
					if err := validateSchemaURI(root, path, reference); err != nil {
						return err
					}
				}
			}
			if err := validateSchemaURIs(root, path, child); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range current {
			if err := validateSchemaURIs(root, path, child); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateSchemaURI(root, currentPath, reference string) error {
	referencePath, _, _ := strings.Cut(reference, "#")
	if referencePath == "" {
		return nil
	}
	parsed, err := url.Parse(referencePath)
	if err != nil {
		return fmt.Errorf("%w: invalid schema URI %q", ErrInvalidManifest, reference)
	}
	if parsed.Scheme != "" && parsed.Scheme != "file" {
		return fmt.Errorf("%w: %q", ErrRemoteReference, reference)
	}
	if parsed.Scheme == "file" && parsed.Host != "" && parsed.Host != "localhost" {
		return fmt.Errorf("%w: %q", ErrRemoteReference, reference)
	}
	candidate := filepath.Join(filepath.Dir(currentPath), filepath.FromSlash(referencePath))
	if parsed.Scheme == "file" {
		candidate, err = fileURLPath(referencePath)
		if err != nil {
			return err
		}
	}
	physicalParent, err := filepath.EvalSymlinks(filepath.Dir(candidate))
	if err != nil {
		return fmt.Errorf("%w: schema URI %q: %v", ErrInvalidManifest, reference, err)
	}
	physicalCandidate := filepath.Join(physicalParent, filepath.Base(candidate))
	if err := requireWithin(root, physicalCandidate); err != nil {
		return fmt.Errorf("%w: schema URI %q: %v", ErrInvalidManifest, reference, err)
	}
	return nil
}

type denySchemaLoader struct{}

func (denySchemaLoader) Load(rawURL string) (any, error) {
	return nil, fmt.Errorf("%w: embedded schema attempted to load %q", ErrInvalidManifest, rawURL)
}

type confinedSchemaLoader struct{ root string }

func (loader confinedSchemaLoader) Load(rawURL string) (any, error) {
	path, err := fileURLPath(rawURL)
	if err != nil {
		return nil, err
	}
	physical, err := physicalPathWithin(loader.root, path, true)
	if err != nil {
		return nil, fmt.Errorf("%w: schema URL %q: %v", ErrInvalidManifest, rawURL, err)
	}
	document, err := readJSONDocument(physical)
	if err != nil {
		return nil, err
	}
	value, err := decodeJSONDocument(document)
	if err != nil {
		return nil, fmt.Errorf("%w: schema URL %q: %v", ErrInvalidManifest, rawURL, err)
	}
	if err := validateSchemaURIs(loader.root, physical, value); err != nil {
		return nil, err
	}
	return value, nil
}

func fileURLPath(rawURL string) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme != "file" || (parsed.Host != "" && parsed.Host != "localhost") {
		return "", fmt.Errorf("%w: unsupported schema URL %q", ErrInvalidManifest, rawURL)
	}
	path := parsed.Path
	if filepath.Separator == '\\' {
		path = strings.TrimPrefix(path, "/")
	}
	return filepath.FromSlash(path), nil
}

func decodeJSONDocument(document []byte) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(document))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	if err := requireEOF(decoder); err != nil {
		return nil, err
	}
	return value, nil
}

func builtinSchemaURL(name string) string { return "nilo://schemas/" + name }

func supportedPlatform(platform Platform) bool {
	if platform.OS != "linux" && platform.OS != "windows" {
		return false
	}
	return platform.Architecture == "amd64" || platform.Architecture == "arm64"
}

func validResourceClaim(resource ResourceClaim) bool {
	segments := strings.Split(string(resource), "/")
	if len(segments) != 3 {
		return false
	}
	for _, segment := range segments {
		if !validResourceSegment(segment) {
			return false
		}
	}
	return true
}

// validResourceSegment permits a stable lowercase identifier within a
// slash-delimited resource namespace. Empty segments reject absolute paths and
// duplicate separators; the restricted character set also rejects traversal
// and platform-specific path syntax.
func validResourceSegment(segment string) bool {
	if segment == "" || segment[0] < 'a' || segment[0] > 'z' {
		return false
	}
	for index, character := range segment {
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9' && index > 0) || character == '_' || character == '-' {
			continue
		}
		return false
	}
	return true
}

func requireEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values")
		}
		return err
	}
	return nil
}
