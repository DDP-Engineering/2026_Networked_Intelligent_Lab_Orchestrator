package manifest

import (
	"encoding/json"
	"errors"
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
