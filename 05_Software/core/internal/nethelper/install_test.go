package nethelper

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"nilo.local/nilo/internal/network"
)

func TestInstallPolicyRejectsUnrelatedPackageChanges(t *testing.T) {
	for _, body := range []string{
		"VERSION 2\n\nwireguard-tools - < 1.0 /cache/wireguard-tools.deb\n",
		"VERSION 2\nAPT::Architecture=amd64\n\nwireguard-tools 1.0 = 1.0 **CONFIGURE**\n",
	} {
		if err := VerifyWireGuardTransaction(strings.NewReader(body)); err != nil {
			t.Fatal(err)
		}
	}
	for _, body := range []string{
		"VERSION 1\n/cache/wireguard-tools.deb\n",
		"VERSION 2\n\nlinux-image - < 6.12 /cache/kernel.deb\n",
		"VERSION 2\n\nwireguard-tools 1.0 > 0.9 /cache/wireguard-tools.deb\n",
		"VERSION 2\n\nwireguard-tools 1.0 = 1.0 **REMOVE**\n",
		"VERSION 2\n\nother 1.0 = 1.0 **CONFIGURE**\n",
		"VERSION 2\n\n" + strings.Repeat("x", 1<<20),
	} {
		if err := VerifyWireGuardTransaction(strings.NewReader(body)); !errors.Is(err, errPackageScope) {
			t.Fatalf("unsafe transaction accepted: %v", err)
		}
	}
	version, err := plannedVersion("Inst wireguard-tools (1.0.20210914-3 Debian:13 [amd64])\nConf wireguard-tools (1.0.20210914-3 Debian:13 [amd64])\n")
	if err != nil || version != "1.0.20210914-3" {
		t.Fatalf("candidate=%s err=%v", version, err)
	}
	for _, body := range []string{
		"Inst wireguard-tools [1.0] (1.1 Debian:13 [amd64])",
		"Inst libc6 (2.41 Debian:13 [amd64])",
		"Conf other (1.0)", "Remv other [1.0]", "no candidate",
	} {
		if _, err := plannedVersion(body); err == nil {
			t.Fatalf("accepted %q", body)
		}
	}
}

func TestInstallPolicyRequiresSupportedDistributionAndSignedSources(t *testing.T) {
	for _, distro := range []string{"ID=debian\nVERSION_ID=13", "ID=ubuntu\nVERSION_ID=\"24.04\""} {
		if !supportedDistribution(distro) {
			t.Fatal(distro)
		}
	}
	for _, distro := range []string{"ID=debian\nVERSION_ID=12", "ID=ubuntu\nVERSION_ID=26.04", "ID=other\nID_LIKE=debian\nVERSION_ID=13"} {
		if supportedDistribution(distro) {
			t.Fatal(distro)
		}
	}
	for _, source := range []string{
		"deb [trusted=yes] https://example.test/debian trixie main",
		"Types: deb\nTrusted:\n yes\nURIs: https://example.test",
		"Types: deb\nAllow-Insecure: yes",
		"deb [check-valid-until=no] https://example.test/debian trixie main",
	} {
		root := fstest.MapFS{"etc/apt/sources.list": {Data: []byte(source)}}
		if _, err := signedSources(root); err == nil {
			t.Fatal("accepted unsigned/expired source override")
		}
	}
	root := fstest.MapFS{"etc/apt/sources.list.d/debian.sources": {Data: []byte("Types: deb\nURIs: https://deb.debian.org/debian\nSuites: trixie\nComponents: main\nSigned-By: /usr/share/keyrings/debian-archive-keyring.gpg\n")}}
	if _, err := signedSources(root); err != nil {
		t.Fatal(err)
	}
}

type waitingInstaller struct {
	release chan struct{}
	entered chan struct{}
	fail    error
}

func (*waitingInstaller) Plan(context.Context) (InstallPlan, error) {
	return InstallPlan{Revision: "fixed"}, nil
}
func (i *waitingInstaller) Run(ctx context.Context, report func(string) error) error {
	if err := report("downloading"); err != nil {
		return err
	}
	close(i.entered)
	select {
	case <-i.release:
		return i.fail
	case <-ctx.Done():
		return errors.New("installation_interrupted")
	}
}

func TestInstallJobSurvivesRequestAndPersistsTerminalState(t *testing.T) {
	e, d, _, _ := fixture(t)
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	store, err := NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	runner := &waitingInstaller{release: make(chan struct{}), entered: make(chan struct{})}
	if err = e.ConfigureInstaller(runner, store); err != nil {
		t.Fatal(err)
	}
	worker, stop := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); e.RunInstaller(worker) }()
	defer func() { stop(); <-done }()
	request, cancel := context.WithCancel(context.Background())
	if _, err = e.StartInstall(request, "install-1", "stale"); !errors.Is(err, network.ErrPreviewStale) {
		t.Fatal(err)
	}
	r, err := e.StartInstall(request, "install-1", "fixed")
	if err != nil || r.State != network.StateApplying {
		t.Fatalf("%+v %v", r, err)
	}
	cancel()
	select {
	case <-runner.entered:
	case <-time.After(time.Second):
		t.Fatal("job did not start")
	}
	r, err = e.InstallStatus("install-1")
	if err != nil || r.Progress != "downloading" {
		t.Fatalf("%+v %v", r, err)
	}
	if _, err = e.Prepare(worker, "other", network.Change{Action: network.ActionInterfaceDisable, Target: "eth-a"}, network.ConfirmationWindow); !errors.Is(err, network.ErrChangeInProgress) {
		t.Fatal(err)
	}
	if _, err = e.StartInstall(worker, "install-2", "fixed"); !errors.Is(err, network.ErrChangeInProgress) {
		t.Fatal(err)
	}
	if len(d.events) != 0 {
		t.Fatal("installation changed network")
	}
	close(runner.release)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		r, _ = e.InstallStatus("install-1")
		if r.State != network.StateApplying {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if r.State != network.StateConfirmed {
		t.Fatalf("%+v", r)
	}
	saved, err := store.LoadInstall()
	if err != nil || saved.State != network.StateConfirmed {
		t.Fatalf("%+v %v", saved, err)
	}
	if again, err := e.StartInstall(worker, "install-1", "fixed"); err != nil || again.State != network.StateConfirmed {
		t.Fatal("replayed installation")
	}
}

func TestInstallRestartReportsInterruptionWithoutReplaying(t *testing.T) {
	e, _, _, _ := fixture(t)
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	store, err := NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.SaveInstall(InstallRecord{ID: "interrupted", State: network.StateApplying, Progress: "installing"}); err != nil {
		t.Fatal(err)
	}
	if err = e.ConfigureInstaller(&waitingInstaller{}, store); err != nil {
		t.Fatal(err)
	}
	r, err := e.InstallStatus("interrupted")
	if err != nil || r.State != network.StateFailed || !strings.Contains(r.Failure, "installation_interrupted") {
		t.Fatalf("%+v %v", r, err)
	}
	for _, q := range []Request{
		{Op: "wireguard.install.inspect", ID: "extra"},
		{Op: "wireguard.install.read", Revision: "extra"},
		{Op: "wireguard.install.start", ID: "new", Change: network.Change{Target: "arbitrary-package"}},
	} {
		if response := dispatch(context.Background(), e, q); response.Code != "invalid_request" {
			t.Fatalf("accepted parameters: %+v", response)
		}
	}
}
