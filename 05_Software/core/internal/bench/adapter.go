package bench

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"nilo.local/nilo/internal/identity"
	"nilo.local/nilo/internal/integration/manifest"
	"nilo.local/nilo/internal/integration/protocol"
)

// familyAdapter declares the core-owned portion of one package family.
// The manifest remains authoritative for public operations and artifacts.
type familyAdapter struct {
	displayName        string
	operations         map[string]bool
	artifacts          map[string]struct{}
	canonicalize       func(Submission) (Submission, error)
	resource           func(Job) string
	requiredArtifacts  map[string][]string
	runnerArguments    func(string, Options, []string) []string
	prepareExecution   func(context.Context, *jobRunner, protocol.ExecuteRequest) (protocol.ExecuteRequest, error)
	validateSubmission func(context.Context, *Manager, identity.Actor, Plugin, Submission) error
	artifactFile       func(string, string) (string, string, error)
}

var familyAdapters = map[string]familyAdapter{
	"modbus": {displayName: "Modbus", operations: map[string]bool{"connect": true, "serve": true}, artifacts: map[string]struct{}{}, canonicalize: modbusCanonicalSubmission, resource: func(j Job) string { return "modbus/endpoint/" + j.Submission.DeviceID }, validateSubmission: modbusValidateSubmission},
	"serial": {displayName: "USB to Serial", operations: map[string]bool{"discover_devices": false, "connect": true, "stream": true}, artifacts: map[string]struct{}{}, canonicalize: serialCanonicalSubmission, resource: func(j Job) string { return "serial/device/" + j.Submission.DeviceID }, validateSubmission: serialValidateSubmission},
	"sigrok": {
		displayName:        "Sigrok Instruments",
		operations:         map[string]bool{"discover_devices": false, "capture": true, "decode_uart": false},
		artifacts:          map[string]struct{}{"capture": {}},
		canonicalize:       sigrokCanonicalSubmission,
		resource:           sigrokResource,
		requiredArtifacts:  map[string][]string{"capture": {"capture"}},
		runnerArguments:    sigrokRunnerArguments,
		prepareExecution:   sigrokPrepareExecution,
		validateSubmission: sigrokValidateSubmission,
		artifactFile:       sigrokArtifactFile,
	},
	"camera": {
		displayName:        "Camera",
		operations:         map[string]bool{"discover_devices": false, "snapshot": true, "get_controls": false, "set_controls": true, "reset_controls": true, "workspace": true, "stream": true},
		artifacts:          map[string]struct{}{"image": {}},
		canonicalize:       cameraCanonicalSubmission,
		resource:           cameraResource,
		requiredArtifacts:  map[string][]string{"snapshot": {"image"}},
		runnerArguments:    cameraRunnerArguments,
		prepareExecution:   cameraPrepareExecution,
		validateSubmission: cameraValidateSubmission,
		artifactFile:       cameraArtifactFile,
	},
}

var artifactBasename = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,79}$`)

func sigrokArtifactFile(id, mediaType string) (string, string, error) {
	if id != "capture" || mediaType != "application/x-sigrok-session" {
		return "", "", ErrInvalid
	}
	return "capture.sr", "capture.sr", nil
}

func runnerArgumentsFor(packageID, root string, options Options, base []string) ([]string, error) {
	adapter, err := adapterFor(packageID)
	if err != nil {
		return nil, err
	}
	arguments := append([]string(nil), base...)
	if adapter.runnerArguments != nil {
		arguments = adapter.runnerArguments(root, options, arguments)
	}
	return arguments, nil
}

func sigrokRunnerArguments(root string, options Options, arguments []string) []string {
	backend := options.Backend
	bundled := filepath.Join(root, "runtime", "bin", "sigrok-cli")
	if info, err := os.Stat(bundled); err == nil && info.Mode().IsRegular() {
		backend = bundled
	}
	arguments = append(arguments, "--backend", backend)
	if options.Simulation {
		arguments = append(arguments, "--simulation")
	}
	return arguments
}

func adapterFor(packageID string) (familyAdapter, error) {
	adapter, exists := familyAdapters[packageID]
	if !exists {
		return familyAdapter{}, fmt.Errorf("%w: no adapter for %q", ErrUnavailable, packageID)
	}
	return adapter, nil
}

func verifyAdapterContract(descriptor manifest.Manifest) error {
	adapter, err := adapterFor(descriptor.Package.ID)
	if err != nil {
		return err
	}
	if adapter.canonicalize == nil || adapter.resource == nil || (len(adapter.artifacts) != 0 && adapter.artifactFile == nil) {
		return fmt.Errorf("%w: adapter is missing a required family hook", ErrInvalid)
	}
	if len(descriptor.Operations) != len(adapter.operations) || len(descriptor.Artifacts) != len(adapter.artifacts) {
		return fmt.Errorf("%w: adapter declaration count differs from manifest", ErrInvalid)
	}
	for _, operation := range descriptor.Operations {
		stateChanging, exists := adapter.operations[operation.ID]
		if !exists || stateChanging != operation.StateChanging {
			return fmt.Errorf("%w: adapter operation %q disagrees with manifest", ErrInvalid, operation.ID)
		}
	}
	for _, artifact := range descriptor.Artifacts {
		if _, exists := adapter.artifacts[artifact.ID]; !exists {
			return fmt.Errorf("%w: adapter artifact %q disagrees with manifest", ErrInvalid, artifact.ID)
		}
	}
	for operationID, artifacts := range adapter.requiredArtifacts {
		if _, exists := adapter.operations[operationID]; !exists {
			return fmt.Errorf("%w: required artifact operation %q is undeclared", ErrInvalid, operationID)
		}
		for _, artifactID := range artifacts {
			if _, exists := adapter.artifacts[artifactID]; !exists {
				return fmt.Errorf("%w: required artifact %q is undeclared", ErrInvalid, artifactID)
			}
		}
	}
	return nil
}
