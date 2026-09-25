package bench

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"nilo.local/nilo/internal/domain"
	"nilo.local/nilo/internal/integration/manifest"
	"nilo.local/nilo/internal/integration/protocol"
	"nilo.local/nilo/internal/integration/supervisor"
)

func TestUnconfiguredCameraSerializesExplicitManagedState(t *testing.T) {
	encoded, err := json.Marshal(Device{ID: "cam-1", DisplayName: "Camera", Kind: "camera", Presence: "present", Readiness: "unconfigured", Capabilities: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`"configured":false`, `"enabled":false`, `"revision":0`} {
		if !bytes.Contains(encoded, []byte(field)) {
			t.Fatalf("managed state %s omitted from %s", field, encoded)
		}
	}
}

type recordingAdapterRunner struct {
	request protocol.ExecuteRequest
}

func (r *recordingAdapterRunner) Describe(context.Context, string) (protocol.DescribeResult, error) {
	return protocol.DescribeResult{}, nil
}

func (r *recordingAdapterRunner) Execute(_ context.Context, request protocol.ExecuteRequest, _ protocol.EventSink) (protocol.ExecuteResult, error) {
	r.request = request
	return protocol.ExecuteResult{Type: "result", Status: "completed", Data: json.RawMessage(`{}`)}, nil
}

func (r *recordingAdapterRunner) Cancel(context.Context, string, domain.RequestID) error { return nil }
func (r *recordingAdapterRunner) Shutdown(context.Context) error                         { return nil }

func TestExecutionUsesAdapterForPackageSpecificParameters(t *testing.T) {
	manager, _, _ := testManager(t, "auto")
	familyAdapters["testcam"] = familyAdapter{
		operations:   map[string]bool{"snapshot": true},
		canonicalize: func(input Submission) (Submission, error) { return input, nil },
		resource:     func(Job) string { return "" },
	}
	defer delete(familyAdapters, "testcam")
	runner := &recordingAdapterRunner{}
	job := Job{ID: "test-camera-job", PluginID: "testcam", Submission: Submission{PluginID: "testcam", Operation: "snapshot", DeviceID: "cam-1"}}
	executor := &jobRunner{Supervisor: runner, manager: manager, job: job, plugin: Plugin{ID: "testcam"}, stage: filepath.Join(t.TempDir(), "stage")}
	_, err := executor.Execute(t.Context(), protocol.ExecuteRequest{Type: "execute", Operation: "snapshot", Parameters: json.RawMessage(`{"width":640}`)}, protocol.EventSinkFunc(func(context.Context, protocol.Envelope) error { return nil }))
	if err != nil {
		t.Fatalf("snapshot execution failed before runner: %v", err)
	}
	if runner.request.Operation != "snapshot" || string(runner.request.Parameters) != `{"width":640}` {
		t.Fatalf("runner request = %+v", runner.request)
	}
}

func TestSubmissionAdmissionUsesAdapter(t *testing.T) {
	manager, actor, runner := testManager(t, "auto")
	original := familyAdapters["sigrok"]
	adapted := original
	adapted.operations = map[string]bool{"discover_devices": false, "capture": true, "decode_uart": false, "snapshot": true}
	adapted.canonicalize = func(input Submission) (Submission, error) { return input, nil }
	adapted.resource = func(Job) string { return "" }
	adapted.validateSubmission = nil
	adapted.prepareExecution = nil
	familyAdapters["sigrok"] = adapted
	defer func() { familyAdapters["sigrok"] = original }()

	root, err := manager.packageRoot("sigrok")
	if err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(root, "manifest.json")
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var descriptor map[string]any
	if err := json.Unmarshal(raw, &descriptor); err != nil {
		t.Fatal(err)
	}
	operation := descriptor["operations"].([]any)[1].(map[string]any)
	snapshot := make(map[string]any, len(operation))
	for key, value := range operation {
		snapshot[key] = value
	}
	snapshot["id"] = "snapshot"
	descriptor["operations"] = append(descriptor["operations"].([]any), snapshot)
	raw, _ = json.Marshal(descriptor)
	if err := os.WriteFile(manifestPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	indexPath := filepath.Join(root, "files.sha256.json")
	index, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	var files map[string]string
	if err := json.Unmarshal(index, &files); err != nil {
		t.Fatal(err)
	}
	files["manifest.json"] = fmt.Sprintf("%x", sha256.Sum256(raw))
	index, _ = json.Marshal(files)
	if err := os.WriteFile(indexPath, index, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.db().Exec(`UPDATE bench_plugins SET digest=? WHERE id='sigrok'`, fmt.Sprintf("%x", sha256.Sum256(index))); err != nil {
		t.Fatal(err)
	}

	input := Submission{PluginID: "sigrok", Operation: "snapshot", DeviceID: "cam-1", Parameters: json.RawMessage(`{}`), IdempotencyKey: "camera-admission", TimeoutSeconds: 10}
	job, err := manager.Submit(t.Context(), actor, input)
	if err != nil {
		t.Fatalf("registered snapshot refused before queue: %v", err)
	}
	awaitJob(t, manager, actor, job.ID, "running")
	runner.release <- struct{}{}
	awaitJob(t, manager, actor, job.ID, "completed")
}

func TestAdapterDeclaredImageArtifactPersists(t *testing.T) {
	manager, actor, runner := testManager(t, "auto")
	original := familyAdapters["sigrok"]
	adapted := original
	adapted.artifacts = map[string]struct{}{"image": {}}
	adapted.requiredArtifacts = map[string][]string{"capture": {"image"}}
	adapted.artifactFile = func(id, mediaType string) (string, string, error) {
		if id != "image" || mediaType != "image/jpeg" {
			return "", "", ErrInvalid
		}
		return "image.jpg", "snapshot.jpg", nil
	}
	familyAdapters["sigrok"] = adapted
	defer func() { familyAdapters["sigrok"] = original }()

	root, err := manager.packageRoot("sigrok")
	if err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(root, "manifest.json")
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var descriptor map[string]any
	if err := json.Unmarshal(raw, &descriptor); err != nil {
		t.Fatal(err)
	}
	declared := descriptor["artifacts"].([]any)[0].(map[string]any)
	declared["id"] = "image"
	declared["media_types"] = []string{"image/jpeg"}
	raw, _ = json.Marshal(descriptor)
	if err := os.WriteFile(manifestPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	indexPath := filepath.Join(root, "files.sha256.json")
	index, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	var files map[string]string
	if err := json.Unmarshal(index, &files); err != nil {
		t.Fatal(err)
	}
	files["manifest.json"] = fmt.Sprintf("%x", sha256.Sum256(raw))
	index, _ = json.Marshal(files)
	if err := os.WriteFile(indexPath, index, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.db().Exec(`UPDATE bench_plugins SET digest=? WHERE id='sigrok'`, fmt.Sprintf("%x", sha256.Sum256(index))); err != nil {
		t.Fatal(err)
	}
	runner.artifactID = "image"
	runner.artifactFile = "image.jpg"
	runner.mediaType = "image/jpeg"
	job, err := manager.Submit(t.Context(), actor, submission("adapter-image"))
	if err != nil {
		t.Fatal(err)
	}
	awaitJob(t, manager, actor, job.ID, "running")
	runner.release <- struct{}{}
	finished := awaitJob(t, manager, actor, job.ID, "completed")
	if finished.ArtifactID == "" {
		t.Fatal("image artifact was not retained")
	}
	content, metadata, err := manager.OpenArtifact(t.Context(), actor, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer content.Close()
	if metadata.MediaType != "image/jpeg" || string(metadata.PresentationHints) != `{"filename":"snapshot.jpg"}` {
		t.Fatalf("image metadata = %+v", metadata)
	}
	got, err := io.ReadAll(content)
	if err != nil || string(got) != "synthetic integration evidence" {
		t.Fatalf("image bytes = %q, %v", got, err)
	}
}

func TestSigrokAdapterRequiresManifestAgreement(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "integrations", "sigrok", "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var descriptor manifest.Manifest
	if err := json.Unmarshal(raw, &descriptor); err != nil {
		t.Fatal(err)
	}
	if err := verifyAdapterContract(descriptor); err != nil {
		t.Fatalf("matching Sigrok manifest: %v", err)
	}
	for _, test := range []struct {
		name string
		edit func(*manifest.Manifest)
	}{
		{"undeclared operation", func(m *manifest.Manifest) { m.Operations = m.Operations[1:] }},
		{"contradictory state", func(m *manifest.Manifest) { m.Operations[1].StateChanging = false }},
		{"undeclared artifact", func(m *manifest.Manifest) { m.Artifacts = nil }},
	} {
		t.Run(test.name, func(t *testing.T) {
			copy := descriptor
			copy.Operations = append([]manifest.Operation(nil), descriptor.Operations...)
			copy.Artifacts = append([]manifest.Artifact(nil), descriptor.Artifacts...)
			test.edit(&copy)
			if err := verifyAdapterContract(copy); !errors.Is(err, ErrInvalid) {
				t.Fatalf("contract error = %v, want invalid", err)
			}
		})
	}
	original := familyAdapters["sigrok"]
	broken := original
	broken.canonicalize = nil
	familyAdapters["sigrok"] = broken
	defer func() { familyAdapters["sigrok"] = original }()
	if err := verifyAdapterContract(descriptor); !errors.Is(err, ErrInvalid) {
		t.Fatalf("missing canonicalisation hook error = %v, want invalid", err)
	}
}

func TestCameraAdapterRequiresManifestAgreement(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "integrations", "camera", "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var descriptor manifest.Manifest
	if err := json.Unmarshal(raw, &descriptor); err != nil {
		t.Fatal(err)
	}
	if err := verifyAdapterContract(descriptor); err != nil {
		t.Fatalf("matching Camera manifest: %v", err)
	}
	descriptor.Operations[1].StateChanging = false
	if err := verifyAdapterContract(descriptor); !errors.Is(err, ErrInvalid) {
		t.Fatalf("contradictory Camera manifest error = %v, want invalid", err)
	}
}

func TestCameraDiscoveryCreatesManagedRecords(t *testing.T) {
	manager, _, _ := testManager(t, "auto")
	payload := json.RawMessage(`{"devices":[{"id":"cam-2","vendor":"0458","product":"708f","serial":"","port_path":"1-1.3","display_name":"FaceCam","endpoints":[{"id":"video0","capture":true,"formats":["MJPG","YUYV"],"frame_intervals":[{"format":"MJPG","width":640,"height":480,"numerator":1,"denominator":30}]},{"id":"video1","capture":false,"formats":[]}],"selected_endpoint":"video0"}]}`)
	devices, err := manager.reconcileCameraDiscovery(t.Context(), payload)
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 1 || devices[0].ID != "cam-1" || devices[0].Kind != "camera" || devices[0].Readiness != "unconfigured" {
		t.Fatalf("managed camera devices = %+v", devices)
	}
	record, err := cameraRecord(t.Context(), manager, "cam-1")
	if err != nil || record.EndpointID != "video0" {
		t.Fatalf("stored capture endpoint = %+v, %v", record, err)
	}
	var capabilities struct {
		FrameIntervals []struct {
			Numerator   int `json:"numerator"`
			Denominator int `json:"denominator"`
		} `json:"frame_intervals"`
	}
	if json.Unmarshal(record.Capabilities, &capabilities) != nil || len(capabilities.FrameIntervals) != 1 || capabilities.FrameIntervals[0].Denominator != 30 {
		t.Fatalf("stored frame intervals = %s", record.Capabilities)
	}
}

func TestCameraDeviceConfigurationInitializesAndAdmitsSnapshot(t *testing.T) {
	manager, actor, runner := testManager(t, "auto")
	copyCameraBundle(t, manager.options.BundleDir)
	if _, err := manager.Lifecycle(t.Context(), actor, "camera", "install", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Lifecycle(t.Context(), actor, "camera", "enable", "auto"); err != nil {
		t.Fatal(err)
	}
	plugin, err := manager.loadPlugin(t.Context(), "camera")
	if err != nil || plugin.Simulation {
		t.Fatalf("physical Camera plugin metadata = %+v, %v", plugin, err)
	}
	devices, err := manager.reconcileCameraDiscovery(t.Context(), json.RawMessage(`{"devices":[{"id":"cam-7","vendor":"0458","product":"708f","serial":"","port_path":"1-1.3","endpoints":[{"id":"video0","capture":true,"formats":["MJPG"]}],"selected_endpoint":"video0"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	runner.release <- struct{}{}
	configured, err := manager.ConfigureDevice(t.Context(), actor, "camera", devices[0].ID, devices[0].Revision, "Bench Camera", "DUT top view", json.RawMessage(`{}`))
	if err != nil || configured.Readiness != "ready" || !configured.Configured {
		t.Fatalf("configured camera = %+v, %v", configured, err)
	}
	job, err := manager.Submit(t.Context(), actor, Submission{PluginID: "camera", Operation: "snapshot", DeviceID: configured.ID, Parameters: json.RawMessage(`{"width":640,"height":480}`), IdempotencyKey: "camera-configured-snapshot", TimeoutSeconds: 20})
	if err != nil || job.ID == "" {
		t.Fatalf("configured camera admission = %+v, %v", job, err)
	}
}

func TestCameraPluginConfigurationUsesInstalledSchemaAndRevision(t *testing.T) {
	manager, actor, _ := testManager(t, "auto")
	copyCameraBundle(t, manager.options.BundleDir)
	if _, err := manager.Lifecycle(t.Context(), actor, "camera", "install", ""); err != nil {
		t.Fatal(err)
	}
	updated, err := manager.ConfigurePlugin(t.Context(), actor, "camera", 0, json.RawMessage(`{"maximum_width":1280}`))
	if err != nil || updated.Revision != 1 {
		t.Fatalf("valid plugin settings = %+v, %v", updated, err)
	}
	if _, err := manager.ConfigurePlugin(t.Context(), actor, "camera", 0, json.RawMessage(`{}`)); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale settings write = %v", err)
	}
	if _, err := manager.ConfigurePlugin(t.Context(), actor, "camera", 1, json.RawMessage(`{"maximum_width":20000}`)); !errors.Is(err, ErrInvalid) {
		t.Fatalf("out-of-schema settings = %v", err)
	}
}

type startupCameraRunner struct{ calls []string }

func (runner *startupCameraRunner) Describe(context.Context, string) (protocol.DescribeResult, error) {
	return protocol.DescribeResult{}, nil
}
func (runner *startupCameraRunner) Execute(_ context.Context, request protocol.ExecuteRequest, _ protocol.EventSink) (protocol.ExecuteResult, error) {
	runner.calls = append(runner.calls, request.Operation)
	data := json.RawMessage(`{}`)
	if request.Operation == "discover_devices" {
		data = json.RawMessage(`{"devices":[{"id":"cam-1","vendor":"0458","product":"708f","serial":"","port_path":"1-1.3","endpoints":[{"id":"video0","capture":true,"formats":["MJPG"]}],"selected_endpoint":"video0"}]}`)
	}
	return protocol.ExecuteResult{Type: "result", Status: "completed", Data: data}, nil
}
func (runner *startupCameraRunner) Cancel(context.Context, string, domain.RequestID) error {
	return nil
}
func (runner *startupCameraRunner) Shutdown(context.Context) error { return nil }

func TestConfiguredCameraResetsBeforeReadyAfterServerRestart(t *testing.T) {
	manager, actor, oldRunner := testManager(t, "auto")
	copyCameraBundle(t, manager.options.BundleDir)
	if _, err := manager.Lifecycle(t.Context(), actor, "camera", "install", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Lifecycle(t.Context(), actor, "camera", "enable", "auto"); err != nil {
		t.Fatal(err)
	}
	devices, err := manager.reconcileCameraDiscovery(t.Context(), json.RawMessage(`{"devices":[{"id":"cam-1","vendor":"0458","product":"708f","serial":"","port_path":"1-1.3","endpoints":[{"id":"video0","capture":true,"formats":["MJPG"]}],"selected_endpoint":"video0"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	oldRunner.release <- struct{}{}
	if _, err := manager.ConfigureDevice(t.Context(), actor, "camera", devices[0].ID, devices[0].Revision, "Bench camera", "", json.RawMessage(`{}`)); err != nil {
		t.Fatal(err)
	}
	if err := manager.Close(); err != nil {
		t.Fatal(err)
	}
	probe := &startupCameraRunner{}
	options := manager.options
	options.runner = func(Plugin) (supervisor.Supervisor, error) { return probe, nil }
	restarted, err := New(options)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	record, err := cameraRecord(t.Context(), restarted, "cam-1")
	if err != nil || record.Presence != "present" || record.Readiness != "ready" {
		t.Fatalf("camera after restart = %+v, %v", record, err)
	}
	if !slices.Equal(probe.calls, []string{"discover_devices", "reset_controls"}) {
		t.Fatalf("startup calls = %v", probe.calls)
	}
}

func TestCameraRebindRequiresConfirmationAndResetsBeforeReady(t *testing.T) {
	manager, actor, runner := testManager(t, "auto")
	copyCameraBundle(t, manager.options.BundleDir)
	if _, err := manager.Lifecycle(t.Context(), actor, "camera", "install", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Lifecycle(t.Context(), actor, "camera", "enable", "auto"); err != nil {
		t.Fatal(err)
	}
	original := json.RawMessage(`{"devices":[{"id":"cam-1","vendor":"0458","product":"708f","serial":"","port_path":"1-1.3","endpoints":[{"id":"video0","capture":true,"formats":["MJPG"]}],"selected_endpoint":"video0"}]}`)
	devices, err := manager.reconcileCameraDiscovery(t.Context(), original)
	if err != nil {
		t.Fatal(err)
	}
	runner.release <- struct{}{}
	if _, err := manager.ConfigureDevice(t.Context(), actor, "camera", devices[0].ID, devices[0].Revision, "Board view", "top", json.RawMessage(`{}`)); err != nil {
		t.Fatal(err)
	}
	moved := json.RawMessage(`{"devices":[{"id":"cam-2","vendor":"0458","product":"708f","serial":"","port_path":"1-1.4","endpoints":[{"id":"video2","capture":true,"formats":["MJPG"]}],"selected_endpoint":"video2"}]}`)
	devices, err = manager.reconcileCameraDiscovery(t.Context(), moved)
	if err != nil || len(devices) != 2 {
		t.Fatalf("moved = %+v, %v", devices, err)
	}
	if _, err := manager.RebindDevice(t.Context(), actor, "camera", devices[0].ID, devices[0].Revision, devices[1].ID, devices[1].Revision, "wrong"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("missing confirmation = %v", err)
	}
	probe := &startupCameraRunner{}
	manager.options.runner = func(Plugin) (supervisor.Supervisor, error) { return probe, nil }
	bound, err := manager.RebindDevice(t.Context(), actor, "camera", devices[0].ID, devices[0].Revision, devices[1].ID, devices[1].Revision, devices[0].ID)
	if err != nil || bound.ID != "cam-1" || bound.Readiness != "ready" || !slices.Equal(probe.calls, []string{"reset_controls"}) {
		t.Fatalf("rebind = %+v, calls %v, %v", bound, probe.calls, err)
	}
	if err := manager.ForgetDevice(t.Context(), actor, "camera", bound.ID, bound.Revision, "wrong"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("missing forget confirmation = %v", err)
	}
	if err := manager.ForgetDevice(t.Context(), actor, "camera", bound.ID, bound.Revision, bound.ID); err != nil {
		t.Fatal(err)
	}
	active, err := manager.ListManagedDevices(t.Context(), actor, "camera")
	if err != nil || len(active) != 0 {
		t.Fatalf("after forget = %+v, %v", active, err)
	}
}

func copyCameraBundle(t *testing.T, bundleDir string) {
	t.Helper()
	source := filepath.Join("..", "..", "..", "integrations", "camera")
	target := filepath.Join(bundleDir, "camera")
	files := map[string]string{}
	for _, name := range []string{"manifest.json", "runner", "schemas/plugin.schema.json", "schemas/device.schema.json", "schemas/discover.input.schema.json", "schemas/snapshot.input.schema.json", "schemas/controls.input.schema.json", "schemas/reset.input.schema.json", "schemas/result.schema.json", "schemas/event.schema.json"} {
		content, err := os.ReadFile(filepath.Join(source, name))
		if err != nil {
			t.Fatal(err)
		}
		if name == "manifest.json" {
			var descriptor map[string]any
			if err := json.Unmarshal(content, &descriptor); err != nil {
				t.Fatal(err)
			}
			descriptor["platforms"] = []map[string]string{{"os": runtime.GOOS, "architecture": runtime.GOARCH}}
			content, _ = json.Marshal(descriptor)
		}
		path := filepath.Join(target, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, content, 0o700); err != nil {
			t.Fatal(err)
		}
		files[name] = fmt.Sprintf("%x", sha256.Sum256(content))
	}
	index, _ := json.Marshal(files)
	if err := os.WriteFile(filepath.Join(target, "files.sha256.json"), index, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestSubmissionAndResourceDispatchThroughAdapter(t *testing.T) {
	familyAdapters["testcam"] = familyAdapter{
		operations: map[string]bool{"snapshot": true},
		canonicalize: func(input Submission) (Submission, error) {
			if input.Operation != "snapshot" || input.DeviceID != "cam-1" {
				return input, ErrInvalid
			}
			return input, nil
		},
		resource: func(job Job) string { return "testcam/device/" + job.Submission.DeviceID },
	}
	defer delete(familyAdapters, "testcam")
	input := Submission{PluginID: "testcam", Operation: "snapshot", DeviceID: "cam-1", Parameters: json.RawMessage(`{}`), IdempotencyKey: "camera-123", TimeoutSeconds: 30}
	canonical, err := canonicalSubmission(input)
	if err != nil || canonical.Operation != "snapshot" {
		t.Fatalf("canonicalSubmission() = %+v, %v", canonical, err)
	}
	resource := resourceFor(Job{PluginID: "testcam", Submission: canonical})
	if resource != "testcam/device/cam-1" {
		t.Fatalf("resource = %q", resource)
	}
}

func TestRunnerArgumentsDispatchThroughAdapter(t *testing.T) {
	familyAdapters["testcam"] = familyAdapter{
		runnerArguments: func(root string, options Options, base []string) []string {
			return append(base, "--camera-root", root)
		},
	}
	defer delete(familyAdapters, "testcam")
	arguments, err := runnerArgumentsFor("testcam", "camera-package", Options{}, []string{"--stdio"})
	if err != nil || len(arguments) != 3 || arguments[1] != "--camera-root" || arguments[2] != "camera-package" {
		t.Fatalf("runner arguments = %v, %v", arguments, err)
	}
}

func TestValidationAddsManifestResourcesToAdapterResource(t *testing.T) {
	manager, _, _ := testManager(t, "auto")
	root, err := manager.packageRoot("sigrok")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "manifest.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	operations := document["operations"].([]any)
	operations[1].(map[string]any)["resources"] = []any{"sigrok/bus/usb"}
	encoded, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	plugin, err := manager.loadPlugin(t.Context(), "sigrok")
	if err != nil {
		t.Fatal(err)
	}
	job := Job{PluginID: "sigrok", Submission: submission("resource-test")}
	validation, err := (&jobRunner{manager: manager, plugin: plugin, job: job}).validation()
	if err != nil {
		t.Fatal(err)
	}
	wantStatic := plugin.InstanceID + "/bus/usb"
	wantDynamic := plugin.InstanceID + "/device/atk-dl16-1.3"
	if len(validation.Resources) != 2 || validation.Resources[0] != wantStatic || validation.Resources[1] != wantDynamic {
		t.Fatalf("resources = %v, want static %q and dynamic %q", validation.Resources, wantStatic, wantDynamic)
	}
}
