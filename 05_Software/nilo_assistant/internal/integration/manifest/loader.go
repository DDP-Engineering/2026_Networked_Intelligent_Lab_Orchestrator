package manifest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"nilo.local/nilo/internal/config"
)

const maxDocumentBytes = 1 << 20

// Load validates the package manifest and every referenced local schema. All
// returned file paths are resolved beneath packageRoot.
func Load(packageRoot string) (Manifest, error) {
	manifestPath, err := config.ResolveWithin(packageRoot, "manifest.json")
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
	if err := validateManifest(&result, packageRoot); err != nil {
		return Manifest{}, err
	}
	return result, nil
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
	for _, name := range []string{"manifest.schema.json", "operation.schema.json", "event.schema.json"} {
		path, err := installedSchemaPath(name)
		if err != nil {
			return err
		}
		schemaDocument, err := readJSONDocument(path)
		if err != nil {
			return err
		}
		var schemaValue any
		if err := json.Unmarshal(schemaDocument, &schemaValue); err != nil {
			return fmt.Errorf("%w: parse built-in schema %s: %v", ErrInvalidManifest, name, err)
		}
		if err := compiler.AddResource(path, schemaValue); err != nil {
			return fmt.Errorf("%w: add built-in schema %s: %v", ErrInvalidManifest, name, err)
		}
	}
	path, err := installedSchemaPath("manifest.schema.json")
	if err != nil {
		return err
	}
	schema, err := compiler.Compile(path)
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
		if duration, err := time.ParseDuration(operation.Timeout); err != nil || duration <= 0 {
			return fmt.Errorf("%w: operation %q timeout", ErrInvalidManifest, operation.ID)
		}
		if err := resolveSchema(root, &operation.InputSchema); err != nil {
			return fmt.Errorf("%w: operation %q input schema: %w", ErrInvalidManifest, operation.ID, err)
		}
		if err := resolveSchema(root, &operation.ResultSchema); err != nil {
			return fmt.Errorf("%w: operation %q result schema: %w", ErrInvalidManifest, operation.ID, err)
		}
		for resourceIndex, resource := range operation.Resources {
			normalized := ResourceClaim(strings.ToLower(strings.TrimSpace(string(resource))))
			if !validResourceClaim(normalized) {
				return fmt.Errorf("%w: operation %q resource claim %q", ErrInvalidManifest, operation.ID, resource)
			}
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
		if duration, err := time.ParseDuration(healthCheck.Timeout); err != nil || duration <= 0 {
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

func resolveSchema(root string, path *string) error {
	if err := resolvePath(root, path, true); err != nil {
		return err
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	schemas := make(map[string]any)
	if err := collectLocalSchemas(root, *path, schemas); err != nil {
		return err
	}
	for schemaPath, value := range schemas {
		if err := compiler.AddResource(schemaPath, value); err != nil {
			return fmt.Errorf("%w: add schema: %v", ErrInvalidManifest, err)
		}
	}
	if _, err := compiler.Compile(*path); err != nil {
		return fmt.Errorf("%w: compile schema: %v", ErrInvalidManifest, err)
	}
	return nil
}

func collectLocalSchemas(root, path string, schemas map[string]any) error {
	if _, exists := schemas[path]; exists {
		return nil
	}
	document, err := readJSONDocument(path)
	if err != nil {
		return err
	}
	var value any
	if err := json.Unmarshal(document, &value); err != nil {
		return fmt.Errorf("%w: schema is not JSON: %v", ErrInvalidManifest, err)
	}
	schemas[path] = value
	for _, reference := range schemaReferences(value) {
		if isRemoteReference(reference) {
			return fmt.Errorf("%w: %q", ErrRemoteReference, reference)
		}
		referencePath, _, _ := strings.Cut(reference, "#")
		if referencePath == "" {
			continue
		}
		candidate := filepath.Join(filepath.Dir(path), filepath.FromSlash(referencePath))
		relative, err := filepath.Rel(root, candidate)
		if err != nil {
			return fmt.Errorf("%w: schema reference %q: %v", ErrInvalidManifest, reference, err)
		}
		resolved, err := config.ResolveWithin(root, relative)
		if err != nil {
			return fmt.Errorf("%w: schema reference %q: %v", ErrInvalidManifest, reference, err)
		}
		if err := collectLocalSchemas(root, resolved, schemas); err != nil {
			return fmt.Errorf("%w: schema reference %q: %w", ErrInvalidManifest, reference, err)
		}
	}
	return nil
}

func resolvePath(root string, path *string, requireFile bool) error {
	resolved, err := config.ResolveWithin(root, *path)
	if err != nil {
		return fmt.Errorf("%w: path %q: %v", ErrInvalidManifest, *path, err)
	}
	if requireFile {
		info, err := os.Stat(resolved)
		if err != nil {
			return fmt.Errorf("%w: path %q: %v", ErrInvalidManifest, *path, err)
		}
		if info.IsDir() {
			return fmt.Errorf("%w: path %q is a directory", ErrInvalidManifest, *path)
		}
	}
	*path = resolved
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

func schemaReferences(value any) []string {
	var references []string
	collectSchemaReferences(value, &references)
	return references
}

func collectSchemaReferences(value any, references *[]string) {
	switch current := value.(type) {
	case map[string]any:
		for key, child := range current {
			if key == "$ref" {
				if reference, ok := child.(string); ok {
					*references = append(*references, reference)
				}
			}
			collectSchemaReferences(child, references)
		}
	case []any:
		for _, child := range current {
			collectSchemaReferences(child, references)
		}
	}
}

func isRemoteReference(reference string) bool {
	lower := strings.ToLower(reference)
	return strings.HasPrefix(lower, "http:") || strings.HasPrefix(lower, "https:") || strings.HasPrefix(lower, "//") || strings.HasPrefix(lower, "file:")
}

func supportedPlatform(platform Platform) bool {
	if platform.OS != "linux" && platform.OS != "windows" {
		return false
	}
	return platform.Architecture == "amd64" || platform.Architecture == "arm64"
}

func validResourceClaim(resource ResourceClaim) bool {
	if resource == "" {
		return false
	}
	for index, character := range resource {
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9' && index > 0) || character == '_' || character == '-' {
			continue
		}
		return false
	}
	return resource[0] >= 'a' && resource[0] <= 'z'
}

func installedSchemaPath(name string) (string, error) {
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("%w: locate built-in schema", ErrInvalidManifest)
	}
	return filepath.Join(filepath.Dir(sourceFile), "..", "..", "..", "..", "schemas", "integration", "v1", name), nil
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
