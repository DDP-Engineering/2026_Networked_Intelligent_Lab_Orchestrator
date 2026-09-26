package bench

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUpdateAvailabilityRequiresNewerVerifiedBundle(t *testing.T) {
	manager, actor, _ := testManager(t, "auto")
	current, err := manager.loadPlugin(t.Context(), "sigrok")
	if err != nil {
		t.Fatal(err)
	}
	manager.describeUpdate(&current)
	if current.UpdateAvailable {
		t.Fatal("same version advertised as update")
	}
	rewriteCatalogVersion(t, manager, "9.0.0")
	plugins, err := manager.ListPlugins(t.Context(), actor)
	if err != nil || !plugins[0].UpdateAvailable || plugins[0].AvailableVersion != "9.0.0" {
		t.Fatalf("new version not advertised: %+v %v", plugins, err)
	}
	root := filepath.Join(manager.options.BundleDir, "sigrok")
	if err := os.WriteFile(filepath.Join(root, "runner"), []byte("tampered"), 0700); err != nil {
		t.Fatal(err)
	}
	plugins, err = manager.ListPlugins(t.Context(), actor)
	if err != nil || plugins[0].UpdateAvailable {
		t.Fatalf("invalid bundle advertised: %+v %v", plugins, err)
	}
}

func TestNewerPackageVersion(t *testing.T) {
	for _, test := range []struct {
		candidate, installed string
		want                 bool
	}{
		{"0.2.0", "0.1.9", true}, {"0.10.0", "0.9.0", true}, {"1.0.0", "1.0.0-rc.1", true},
		{"1.0.0-rc.10", "1.0.0-rc.2", true}, {"1.0.0-beta", "1.0.0-alpha", true},
		{"1.0.0-rc.1", "1.0.0", false}, {"1.0.0+new", "1.0.0+old", false},
		{"1.0.0", "2.0.0", false}, {"01.0.0", "0.1.0", false}, {"1.0.0-01", "1.0.0-0", false},
		{"garbage", "0.1.0", false}, {"1.0.0", "", false},
	} {
		t.Run(test.candidate+"_"+test.installed, func(t *testing.T) {
			if got := newerPackageVersion(test.candidate, test.installed); got != test.want {
				t.Fatalf("got %v, want %v", got, test.want)
			}
		})
	}
}
