package manifest

import (
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadAcceptsValidPackage(t *testing.T) {
	root := writePackage(t, nil)

	loaded, err := Load(root)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded.Package.ID != "sigrok-cli" {
		t.Fatalf("Package.ID = %q, want sigrok-cli", loaded.Package.ID)
	}
	if got, want := loaded.Operations[0].Resources, []ResourceClaim{"logic_analyzer", "usb_bus"}; !sameClaims(got, want) {
		t.Fatalf("operation resources = %v, want %v", got, want)
	}
	if !filepath.IsAbs(loaded.Operations[0].InputSchema) {
		t.Fatalf("InputSchema = %q, want resolved absolute path", loaded.Operations[0].InputSchema)
	}
}

func TestResourceClaimGrammar(t *testing.T) {
	valid := []ResourceClaim{
		"usb_bus",
		"fake/device/stable-001",
		"fake/device_2/stable-001",
	}
	for _, claim := range valid {
		t.Run("accepts_"+string(claim), func(t *testing.T) {
			if !validResourceClaim(claim) {
				t.Fatalf("validResourceClaim(%q) = false, want true", claim)
			}
		})
	}

	invalid := []ResourceClaim{
		"",
		"/fake/device",
		"fake/device/",
		"fake//device",
		"fake/./device",
		"fake/../device",
		"fake/device/..",
		"fake\\device",
		"fake/device?",
		"fake/Device",
		"fake/device name",
	}
	for _, claim := range invalid {
		t.Run("rejects_"+strings.NewReplacer("/", "_", "\\", "_", "?", "_").Replace(string(claim)), func(t *testing.T) {
			if validResourceClaim(claim) {
				t.Fatalf("validResourceClaim(%q) = true, want false", claim)
			}
		})
	}
}

func TestLoadAcceptsFakePythonPackage(t *testing.T) {
	root := filepath.Join("..", "..", "..", "..", "integrations", "fake_python")

	loaded, err := Load(root)
	if err != nil {
		t.Fatalf("Load(%q) error = %v", root, err)
	}
	if len(loaded.Operations) != 5 {
		t.Fatalf("operation count = %d, want 5", len(loaded.Operations))
	}
	for _, operation := range loaded.Operations {
		if !sameClaims(operation.Resources, []ResourceClaim{"fake/device/stable-001"}) {
			t.Fatalf("operation %q resources = %v, want fake/device/stable-001", operation.ID, operation.Resources)
		}
	}
}

func TestLoadRejectsMalformedResourceClaims(t *testing.T) {
	claims := []string{
		"",
		"/fake/device",
		"fake/device/",
		"fake//device",
		"fake/./device",
		"fake/../device",
		"fake\\device",
		"fake/device?",
		"fake/device name",
	}
	for _, claim := range claims {
		t.Run("rejects_"+strings.NewReplacer("/", "_", "\\", "_", "?", "_").Replace(claim), func(t *testing.T) {
			root := writePackage(t, func(_ *testing.T, document map[string]any) {
				document["operations"].([]any)[0].(map[string]any)["resources"] = []any{claim}
			})
			if _, err := Load(root); !errors.Is(err, ErrInvalidManifest) {
				t.Fatalf("Load() error = %v, want ErrInvalidManifest", err)
			}
		})
	}
}

func TestCanonicalPackageRootUsesPhysicalPath(t *testing.T) {
	root := writePackage(t, nil)
	want, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}

	got, err := canonicalPackageRoot(root)
	if err != nil {
		t.Fatalf("canonicalPackageRoot() error = %v", err)
	}
	if got != want {
		t.Fatalf("canonicalPackageRoot() = %q, want %q", got, want)
	}
}

func TestLoadAcceptsPackageOpenedThroughSymlink(t *testing.T) {
	root := writePackage(t, nil)
	if err := os.WriteFile(filepath.Join(root, "schemas", "child.json"), []byte(`{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "schemas", "input.json"), []byte(`{"$schema":"https://json-schema.org/draft/2020-12/schema","$ref":"child.json"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "package-link")
	if err := os.Symlink(root, link); err != nil {
		t.Skipf("symlink capability unavailable: os.Symlink(%q, %q): %v", root, link, err)
	}

	if _, err := Load(link); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
}

func TestLoadRejectsInvalidManifestContracts(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(t *testing.T, manifest map[string]any)
		want   error
	}{
		{
			name: "unsupported protocol",
			mutate: func(_ *testing.T, manifest map[string]any) {
				manifest["protocol"] = "nilo.integration/2"
			},
			want: ErrUnsupportedProtocol,
		},
		{
			name: "duplicate operation id",
			mutate: func(_ *testing.T, manifest map[string]any) {
				operations := manifest["operations"].([]any)
				duplicate := cloneMap(t, operations[0].(map[string]any))
				operations = append(operations, duplicate)
				manifest["operations"] = operations
			},
			want: ErrDuplicateOperationID,
		},
		{
			name: "missing policy",
			mutate: func(_ *testing.T, manifest map[string]any) {
				delete(manifest["operations"].([]any)[0].(map[string]any), "policy")
			},
			want: ErrInvalidManifest,
		},
		{
			name: "nonpositive timeout",
			mutate: func(_ *testing.T, manifest map[string]any) {
				manifest["operations"].([]any)[0].(map[string]any)["timeout"] = "-1s"
			},
			want: ErrInvalidManifest,
		},
		{
			name: "operation timeout exceeds maximum",
			mutate: func(_ *testing.T, manifest map[string]any) {
				manifest["operations"].([]any)[0].(map[string]any)["timeout"] = "2h"
			},
			want: ErrInvalidManifest,
		},
		{
			name: "health check timeout exceeds maximum",
			mutate: func(_ *testing.T, manifest map[string]any) {
				manifest["health_checks"].([]any)[0].(map[string]any)["timeout"] = "2m"
			},
			want: ErrInvalidManifest,
		},
		{
			name: "short integrity digest",
			mutate: func(_ *testing.T, manifest map[string]any) {
				manifest["package"].(map[string]any)["integrity"] = "sha256:abc"
			},
			want: ErrInvalidManifest,
		},
		{
			name: "colliding resource claims",
			mutate: func(_ *testing.T, manifest map[string]any) {
				manifest["operations"].([]any)[0].(map[string]any)["resources"] = []any{"usb_bus", " USB_BUS "}
			},
			want: ErrInvalidManifest,
		},
		{
			name: "unsupported platform",
			mutate: func(_ *testing.T, manifest map[string]any) {
				manifest["platforms"].([]any)[0].(map[string]any)["os"] = "plan9"
			},
			want: ErrUnsupportedPlatform,
		},
		{
			name: "schema path escape",
			mutate: func(_ *testing.T, manifest map[string]any) {
				manifest["operations"].([]any)[0].(map[string]any)["input_schema"] = "../outside.json"
			},
			want: ErrInvalidManifest,
		},
		{
			name: "absolute runner path",
			mutate: func(_ *testing.T, manifest map[string]any) {
				manifest["runner"].(map[string]any)["executable"] = filepath.Join(os.TempDir(), "sigrok-cli")
			},
			want: ErrInvalidManifest,
		},
		{
			name: "raw shell command runner",
			mutate: func(_ *testing.T, manifest map[string]any) {
				manifest["runner"] = "sigrok-cli --driver hantek-dso"
			},
			want: ErrInvalidManifest,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := writePackage(t, test.mutate)
			if _, err := Load(root); !errors.Is(err, test.want) {
				t.Fatalf("Load() error = %v, want errors.Is(_, %v)", err, test.want)
			}
		})
	}
}

func TestInvalidCommandFixtureIsRejected(t *testing.T) {
	fixture, err := os.ReadFile(filepath.Join("testdata", "invalid-command", "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "manifest.json"), fixture, 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := Load(root); !errors.Is(err, ErrInvalidManifest) {
		t.Fatalf("Load() error = %v, want ErrInvalidManifest", err)
	}
}

func TestLoadRejectsRemoteReferenceInPackageSchema(t *testing.T) {
	root := writePackage(t, nil)
	inputSchema := filepath.Join(root, "schemas", "input.json")
	if err := os.WriteFile(inputSchema, []byte(`{"$schema":"https://json-schema.org/draft/2020-12/schema","$ref":"https://example.invalid/schema.json"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := Load(root); !errors.Is(err, ErrRemoteReference) {
		t.Fatalf("Load() error = %v, want ErrRemoteReference", err)
	}
}

func TestLoadRejectsPackageSchemaReferenceEscape(t *testing.T) {
	root := writePackage(t, nil)
	escapePath := filepath.Join(filepath.Dir(root), "escape.json")
	if err := os.WriteFile(escapePath, []byte(`{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	inputSchema := filepath.Join(root, "schemas", "input.json")
	if err := os.WriteFile(inputSchema, []byte(`{"$schema":"https://json-schema.org/draft/2020-12/schema","$ref":"../../escape.json"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := Load(root); !errors.Is(err, ErrInvalidManifest) {
		t.Fatalf("Load() error = %v, want ErrInvalidManifest", err)
	}
}

func TestLoadRejectsDynamicReferenceEscape(t *testing.T) {
	root := writePackage(t, nil)
	escapePath := filepath.Join(t.TempDir(), "escape.json")
	if err := os.WriteFile(escapePath, []byte(`{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	inputSchema := filepath.Join(root, "schemas", "input.json")
	if err := os.WriteFile(inputSchema, []byte(`{"$schema":"https://json-schema.org/draft/2020-12/schema","$dynamicRef":`+jsonString(fileURL(escapePath))+`}`), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := Load(root); !errors.Is(err, ErrInvalidManifest) {
		t.Fatalf("Load() error = %v, want ErrInvalidManifest", err)
	}
}

func TestLoadRejectsChangedSchemaIDOutsidePackage(t *testing.T) {
	root := writePackage(t, nil)
	escapeDirectory := t.TempDir()
	if err := os.WriteFile(filepath.Join(escapeDirectory, "child.json"), []byte(`{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "schemas", "child.json"), []byte(`{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	inputSchema := filepath.Join(root, "schemas", "input.json")
	changedID := fileURL(filepath.Join(escapeDirectory, "base.json"))
	if err := os.WriteFile(inputSchema, []byte(`{"$schema":"https://json-schema.org/draft/2020-12/schema","$id":`+jsonString(changedID)+`,"$ref":"child.json"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := Load(root); !errors.Is(err, ErrInvalidManifest) {
		t.Fatalf("Load() error = %v, want ErrInvalidManifest", err)
	}
}

func TestLoadRejectsPhysicalPathEscapes(t *testing.T) {
	tests := []struct {
		name      string
		candidate func(root string) string
		contents  []byte
	}{
		{name: "runner", candidate: func(root string) string { return filepath.Join(root, "bin", "sigrok-cli") }, contents: []byte("outside runner")},
		{name: "schema", candidate: func(root string) string { return filepath.Join(root, "schemas", "input.json") }, contents: []byte(`{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object"}`)},
		{name: "fixture", candidate: func(root string) string { return filepath.Join(root, "fixtures", "capture.json") }, contents: []byte(`{"outside":true}`)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := writePackage(t, nil)
			target := filepath.Join(t.TempDir(), test.name)
			if err := os.WriteFile(target, test.contents, 0o600); err != nil {
				t.Fatal(err)
			}
			replaceWithSymlink(t, test.candidate(root), target)

			if _, err := Load(root); !errors.Is(err, ErrInvalidManifest) {
				t.Fatalf("Load() error = %v, want ErrInvalidManifest", err)
			}
		})
	}
}

func TestLoadRejectsSymlinkedSchemaDirectory(t *testing.T) {
	root := writePackage(t, nil)
	outside := t.TempDir()
	for _, name := range []string{"input.json", "result.json", "config.json"} {
		if err := os.WriteFile(filepath.Join(outside, name), []byte(`{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object"}`), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	replaceWithSymlink(t, filepath.Join(root, "schemas"), outside)

	if _, err := Load(root); !errors.Is(err, ErrInvalidManifest) {
		t.Fatalf("Load() error = %v, want ErrInvalidManifest", err)
	}
}

func TestLoadUsesEmbeddedSchemasWithTrimpath(t *testing.T) {
	root := writePackage(t, nil)
	if _, err := Load(root); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
}

func TestLoadRejectsOversizedReferencedSchema(t *testing.T) {
	root := writePackage(t, nil)
	inputSchema := filepath.Join(root, "schemas", "input.json")
	if err := os.WriteFile(inputSchema, []byte(strings.Repeat(" ", maxDocumentBytes+1)), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := Load(root); !errors.Is(err, ErrDocumentTooLarge) {
		t.Fatalf("Load() error = %v, want ErrDocumentTooLarge", err)
	}
}

func writePackage(t *testing.T, mutate func(t *testing.T, manifest map[string]any)) string {
	t.Helper()
	fixture, err := os.ReadFile(filepath.Join("testdata", "valid", "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(fixture, &document); err != nil {
		t.Fatal(err)
	}
	if mutate != nil {
		mutate(t, document)
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "schemas"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "bin"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "fixtures"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "manifest.json"), encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"input.json", "result.json", "config.json"} {
		if err := os.WriteFile(filepath.Join(root, "schemas", name), []byte(`{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object"}`), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "bin", "sigrok-cli"), []byte("runner"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "fixtures", "capture.json"), []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	return root
}

func cloneMap(t *testing.T, input map[string]any) map[string]any {
	t.Helper()
	encoded, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	var copy map[string]any
	if err := json.Unmarshal(encoded, &copy); err != nil {
		t.Fatal(err)
	}
	return copy
}

func sameClaims(got, want []ResourceClaim) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range want {
		if got[index] != want[index] {
			return false
		}
	}
	return true
}

func replaceWithSymlink(t *testing.T, candidate, target string) {
	t.Helper()
	if err := os.RemoveAll(candidate); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, candidate); err != nil {
		t.Skipf("symlink capability unavailable: os.Symlink(%q, %q): %v", target, candidate, err)
	}
}

func fileURL(path string) string {
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(path)}).String()
}

func jsonString(value string) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(encoded)
}
