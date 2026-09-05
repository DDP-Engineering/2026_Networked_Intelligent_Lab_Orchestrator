package bench

import (
	"context"
	"nilo.local/nilo/internal/integration/manifest"
	"nilo.local/nilo/internal/integration/supervisor"
	"os"
	"testing"
	"time"
)

// This opt-in target test runs the assembled bundle through the real wire
// supervisor. The normal suite uses controlled runners; deployment runs this
// on the target Python architecture before instrument acceptance.
func TestAssembledBundleDescribe(t *testing.T) {
	root := os.Getenv("NILO_TEST_BUNDLE")
	if root == "" {
		t.Skip("target bundle not supplied")
	}
	if _, err := verifyPackage(root); err != nil {
		t.Fatal(err)
	}
	descriptor, err := manifest.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	runner, err := supervisor.NewProcessSupervisor(supervisor.ProcessConfig{PackageID: "sigrok", PackageRoot: root, Runner: descriptor.Runner, GracePeriod: time.Second, Environment: map[string]string{"LANG": "C.UTF-8", "LC_ALL": "C.UTF-8"}})
	if err != nil {
		t.Fatal(err)
	}
	defer runner.Shutdown(context.Background())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	result, err := runner.Describe(ctx, "sigrok")
	if err != nil {
		t.Fatalf("describe: %v; diagnostic: %s", err, supervisor.Diagnostics(err))
	}
	if result.Integration != "sigrok" {
		t.Fatal(result)
	}
}
