package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"nilo.local/nilo/internal/store/sqlite"
)

func TestNiloServeCommandExposesAuthenticatedSimulationStatus(t *testing.T) {
	binary := buildNilo(t)
	dataDir := t.TempDir()
	issue := exec.Command(binary, "development", "issue-token", "--data-dir", dataDir)
	issuedOutput, err := issue.Output()
	if err != nil {
		t.Fatalf("issue development token: %v", err)
	}
	var issued struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(issuedOutput, &issued); err != nil {
		t.Fatalf("decode development token: %v", err)
	}

	reservation, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve loopback address: %v", err)
	}
	address := reservation.Addr().String()
	if err := reservation.Close(); err != nil {
		t.Fatalf("release loopback address: %v", err)
	}

	server := exec.Command(
		binary,
		"serve",
		"--profile", "simulation",
		"--data-dir", dataDir,
		"--bind", address,
	)
	var serverOutput bytes.Buffer
	server.Stdout = &serverOutput
	server.Stderr = &serverOutput
	if err := server.Start(); err != nil {
		t.Fatalf("start nilo serve: %v", err)
	}
	returned := make(chan error, 1)
	go func() { returned <- server.Wait() }()
	t.Cleanup(func() {
		if server.ProcessState == nil {
			_ = server.Process.Kill()
			<-returned
		}
	})

	client := &http.Client{Timeout: 250 * time.Millisecond}
	deadline := time.Now().Add(5 * time.Second)
	for {
		request, err := http.NewRequest(http.MethodGet, "http://"+address+"/api/v1/status", nil)
		if err != nil {
			t.Fatalf("create status request: %v", err)
		}
		request.Header.Set("Authorization", "Bearer "+issued.Token)
		response, requestErr := client.Do(request)
		if requestErr == nil {
			defer response.Body.Close()
			if response.StatusCode != http.StatusOK {
				t.Fatalf("status endpoint returned %d", response.StatusCode)
			}
			var status struct {
				Profile    string `json:"profile"`
				Simulation bool   `json:"simulation"`
			}
			if err := json.NewDecoder(response.Body).Decode(&status); err != nil {
				t.Fatalf("decode status: %v", err)
			}
			if status.Profile != "simulation" || !status.Simulation {
				t.Fatalf("unexpected status: %#v", status)
			}
			break
		}
		select {
		case err := <-returned:
			t.Fatalf("nilo serve exited before listening: %v; output=%s", err, serverOutput.String())
		default:
		}
		if time.Now().After(deadline) {
			t.Fatalf("status endpoint did not become ready at %s: %v; output=%s", address, requestErr, serverOutput.String())
		}
		time.Sleep(50 * time.Millisecond)
	}

	if err := server.Process.Kill(); err != nil {
		t.Fatalf("stop test server: %v", err)
	}
	if err := <-returned; err == nil {
		t.Fatal("killed test server returned success unexpectedly")
	}
	if server.ProcessState == nil {
		t.Fatalf("test server has no process state; output=%s", serverOutput.String())
	}
}

func TestDevelopmentIssueTokenCommandCreatesAuthenticatableLockedIdentity(t *testing.T) {
	binary := buildNilo(t)
	dataDir := t.TempDir()
	command := exec.Command(binary, "development", "issue-token", "--data-dir", dataDir)
	output, err := command.Output()
	if err != nil {
		t.Fatalf("nilo development issue-token: %v", err)
	}
	var issued struct {
		TokenID   string `json:"token_id"`
		UserID    string `json:"user_id"`
		Token     string `json:"token"`
		CreatedAt string `json:"created_at"`
	}
	if err := json.Unmarshal(output, &issued); err != nil {
		t.Fatalf("decode issued token: %v", err)
	}
	if issued.TokenID == "" || issued.UserID == "" || issued.Token == "" || issued.CreatedAt == "" {
		t.Fatalf("incomplete issued token: %#v", issued)
	}

	database, err := sqlite.Open(context.Background(), filepath.Join(dataDir, "nilo.db"))
	if err != nil {
		t.Fatalf("open issued-token database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	record, err := sqlite.NewAuthRepository(database).VerifyToken(context.Background(), issued.Token)
	if err != nil {
		t.Fatalf("authenticate issued token: %v", err)
	}
	if record.ID != issued.TokenID || record.UserID != issued.UserID {
		t.Fatalf("authenticated record = %#v, want token %q user %q", record, issued.TokenID, issued.UserID)
	}
	var passwordVerifier string
	if err := database.SQL().QueryRowContext(
		context.Background(),
		"SELECT password_hash FROM users WHERE id = ?",
		issued.UserID,
	).Scan(&passwordVerifier); err != nil {
		t.Fatalf("read development identity: %v", err)
	}
	if passwordVerifier != "!disabled-development-login" {
		t.Fatalf("development password verifier = %q, want disabled marker", passwordVerifier)
	}
}

func TestNiloCLIContract(t *testing.T) {
	binary := buildNilo(t)

	version := exec.Command(binary, "version")
	output, err := version.Output()
	if err != nil {
		t.Fatalf("nilo version: %v", err)
	}

	var info struct {
		Name                string `json:"name"`
		Version             string `json:"version"`
		IntegrationProtocol string `json:"integration_protocol"`
	}
	if err := json.Unmarshal(output, &info); err != nil {
		t.Fatalf("decode nilo version output: %v", err)
	}
	if info.Name != "nilo" || info.Version != "0.1.0-dev" || info.IntegrationProtocol != "nilo.integration/1" {
		t.Fatalf("unexpected build information: %#v", info)
	}

	foundation := exec.Command(binary, "serve-foundation-check", "--data-dir", t.TempDir())
	foundationOutput, err := foundation.Output()
	if err != nil {
		t.Fatalf("nilo serve-foundation-check: %v", err)
	}
	var health struct {
		Status     string `json:"status"`
		Reconciled bool   `json:"reconciled"`
	}
	if err := json.Unmarshal(foundationOutput, &health); err != nil {
		t.Fatalf("decode foundation health: %v", err)
	}
	if health.Status != "ok" || !health.Reconciled {
		t.Fatalf("unexpected foundation health: %#v", health)
	}

	invalid := exec.Command(binary, "invalid")
	if err := invalid.Run(); err == nil {
		t.Fatal("invalid invocation succeeded")
	} else if exitError, ok := err.(*exec.ExitError); !ok {
		t.Fatalf("invalid invocation did not return an exit error: %v", err)
	} else if exitError.ExitCode() != 2 {
		t.Fatalf("invalid invocation exit code = %d, want 2", exitError.ExitCode())
	}
}

func buildNilo(t *testing.T) string {
	t.Helper()
	name := "nilo"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	binary := filepath.Join(t.TempDir(), name)
	build := exec.Command("go", "build", "-o", binary, ".")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build nilo: %v\n%s", err, output)
	}
	return binary
}
