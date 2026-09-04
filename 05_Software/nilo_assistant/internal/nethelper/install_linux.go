//go:build linux

package nethelper

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"nilo.local/nilo/internal/network"
)

// APTInstaller is constructed only by privileged local startup. The configuration
// disables site hooks and fixes authentication; sources and keyrings remain the
// administrator's distribution configuration.
type APTInstaller struct {
	config    string
	readiness network.WireGuardReader
}

var helperExecutablePath = regexp.MustCompile(`^/[a-zA-Z0-9_./-]+$`)

func NewAPTInstaller(directory, executable string, readiness network.WireGuardReader) (*APTInstaller, error) {
	if !filepath.IsAbs(directory) || !helperExecutablePath.MatchString(executable) || strings.ContainsAny(directory, "\n\r\"\\") || readiness == nil {
		return nil, errors.New("invalid package installer configuration")
	}
	parts := filepath.Join(directory, "apt.conf.d")
	if err := os.MkdirAll(parts, 0700); err != nil {
		return nil, err
	}
	config := filepath.Join(directory, "apt.conf")
	command := executable + " --verify-wireguard-transaction"
	// APT indexes protocol options by the first command token, without its
	// arguments. The executable path is restricted to shell-safe characters.
	data := fmt.Sprintf("Dir::Etc::Parts \"%s\";\nDir::Etc::main \"/dev/null\";\nDPkg::Pre-Install-Pkgs {\"%s\";};\nDPkg::Tools::Options::%s::Version \"2\";\n", parts, command, executable)
	if err := os.WriteFile(config, []byte(data), 0600); err != nil {
		return nil, err
	}
	return &APTInstaller{config: config, readiness: readiness}, nil
}

func (a *APTInstaller) Plan(ctx context.Context) (InstallPlan, error) {
	root := os.DirFS("/")
	distro, err := readProbeFile(root, "etc/os-release")
	if err != nil || !supportedDistribution(string(distro)) {
		return InstallPlan{}, network.ErrUnsupported
	}
	sources, err := signedSources(root)
	if err != nil {
		return InstallPlan{}, err
	}
	tool, err := installedWG(ctx)
	if err != nil {
		return InstallPlan{}, errors.New("tool_probe_failed: check the installed wg executable")
	}
	digest := sha256.New()
	digest.Write(distro)
	digest.Write(sources)
	fmt.Fprintf(digest, "\x00wireguard-tools\x00%t\x00%s", tool.Installed, tool.Version)
	return InstallPlan{Revision: hex.EncodeToString(digest.Sum(nil)), Installed: tool.Installed}, nil
}

func (a *APTInstaller) Run(ctx context.Context, report func(string) error) error {
	plan, err := a.Plan(ctx)
	if err != nil {
		return err
	}
	if !plan.Installed {
		if err = a.install(ctx, report); err != nil {
			return err
		}
	}
	if err = report("verifying"); err != nil {
		return err
	}
	status, err := a.readiness.WireGuard(ctx)
	if err == nil {
		for _, tool := range status.Tools {
			if tool.Name == "wg" && tool.Installed && tool.Version != "" {
				return nil
			}
		}
	}
	return errors.New("installation_verification_failed: wg readiness could not be verified")
}

func (a *APTInstaller) install(ctx context.Context, report func(string) error) error {
	if err := report("refreshing"); err != nil {
		return err
	}
	if _, err := a.apt(ctx, "update"); err != nil {
		return err
	}
	if err := report("planning"); err != nil {
		return err
	}
	raw, err := a.apt(ctx, "--simulate", "install", "wireguard-tools")
	if err != nil {
		return err
	}
	version, err := plannedVersion(raw)
	if err != nil {
		return err
	}
	packageName := "wireguard-tools=" + version
	if err = report("downloading"); err != nil {
		return err
	}
	if _, err = a.apt(ctx, "--download-only", "install", packageName); err != nil {
		return err
	}
	if err = report("installing"); err != nil {
		return err
	}
	// The pre-install hook validates the actual locked transaction too.
	_, err = a.apt(ctx, "--no-download", "install", packageName)
	return err
}

func (a *APTInstaller) apt(ctx context.Context, action ...string) (string, error) {
	if _, err := signedSources(os.DirFS("/")); err != nil {
		return "", err
	}
	args := []string{"--assume-yes", "--no-install-recommends", "--no-upgrade", "--no-remove"}
	for _, setting := range []string{
		"APT::Get::AllowUnauthenticated=false", "Acquire::AllowInsecureRepositories=false",
		"Acquire::AllowWeakRepositories=false", "Acquire::AllowDowngradeToInsecureRepositories=false",
		"Acquire::Check-Valid-Until=true", "Acquire::Check-Date=true", "Acquire::Retries=0",
		"Acquire::http::Timeout=15", "Acquire::https::Timeout=15", "APT::Update::Error-Mode=any",
		"DPkg::Lock::Timeout=0", "Dpkg::Use-Pty=0", "APT::Get::AutomaticRemove=false",
	} {
		args = append(args, "-o", setting)
	}
	args = append(args, action...)
	cmd := exec.CommandContext(ctx, "/usr/bin/apt-get", args...)
	cmd.Dir = "/"
	cmd.Env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "LANG=C", "LC_ALL=C", "DEBIAN_FRONTEND=noninteractive", "APT_CONFIG=" + a.config}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		// Include dpkg and method workers on bounded shutdown. Interrupted
		// package state is reported; never repair unrelated packages here.
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
	cmd.WaitDelay = time.Second
	output := &aptOutput{}
	cmd.Stdout, cmd.Stderr = output, output
	err := cmd.Run()
	if err != nil {
		if ctx.Err() != nil {
			return "", errors.New("installation_interrupted: the package operation stopped; check package manager state before retrying")
		}
		return "", packageError(string(output.raw))
	}
	if output.truncated {
		return "", errors.New("package_output_limit: package output exceeded the supported bound")
	}
	return string(output.raw), nil
}

// Output is bounded and used only for parsing/classification, never returned to
// a caller. Repository URLs may contain credentials.
type aptOutput struct {
	raw       []byte
	truncated bool
}

func (b *aptOutput) Write(p []byte) (int, error) {
	n := len(p)
	const limit = 1 << 20
	if len(b.raw)+n > limit {
		b.truncated = true
		if n >= limit {
			b.raw = append(b.raw[:0], p[n-limit:]...)
			return n, nil
		}
		b.raw = b.raw[len(b.raw)+n-limit:]
	}
	b.raw = append(b.raw, p...)
	return n, nil
}

func packageError(raw string) error {
	s := strings.ToLower(raw)
	switch {
	case strings.Contains(s, "could not get lock"), strings.Contains(s, "unable to acquire"), strings.Contains(s, "is another process using it"):
		return errors.New("package_lock_busy: another package manager is running; wait for it to finish and retry")
	case strings.Contains(s, "package_scope_refused"):
		return errPackageScope
	case strings.Contains(s, "not signed"), strings.Contains(s, "signature"), strings.Contains(s, "unauthenticated"), strings.Contains(s, "no_pubkey"), strings.Contains(s, "release file"):
		return errors.New("repository_authentication_failed: check repository signatures, keys and system time")
	case strings.Contains(s, "failed to fetch"), strings.Contains(s, "resolve"), strings.Contains(s, "could not connect"), strings.Contains(s, "connection failed"):
		return errors.New("repository_unavailable: check connectivity and configured repositories, then retry")
	case strings.Contains(s, "dpkg was interrupted"), strings.Contains(s, "unmet dependencies"), strings.Contains(s, "not fully installed"), strings.Contains(s, "dependency problems"):
		return errors.New("package_state_requires_attention: repair the package manager separately before retrying")
	default:
		return errors.New("package_install_failed: inspect the local package manager state before retrying")
	}
}
