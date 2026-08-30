package artifact_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"

	"nilo.local/nilo/internal/artifact"
	"nilo.local/nilo/internal/domain"
	"nilo.local/nilo/internal/store/sqlite"
)

type memoryRepository struct {
	artifacts map[domain.ArtifactID]domain.Artifact
	saveError error
}

func (repository *memoryRepository) Save(_ context.Context, value domain.Artifact) error {
	if repository.saveError != nil {
		return repository.saveError
	}
	if repository.artifacts == nil {
		repository.artifacts = make(map[domain.ArtifactID]domain.Artifact)
	}
	repository.artifacts[value.ID] = value
	return nil
}

func (repository *memoryRepository) Get(_ context.Context, id domain.ArtifactID) (domain.Artifact, error) {
	value, ok := repository.artifacts[id]
	if !ok {
		return domain.Artifact{}, artifact.ErrArtifactNotFound
	}
	return value, nil
}

type artifactFixture struct {
	artifactRoot  string
	stagingRoot   string
	stagedPath    string
	contents      []byte
	artifactID    domain.ArtifactID
	requestID     domain.RequestID
	sessionID     domain.SessionID
	deviceID      domain.DeviceID
	integrationID domain.IntegrationID
	createdAt     time.Time
}

func TestRegisterRejectsUnconfinedAndNonAbsoluteStagedPaths(t *testing.T) {
	t.Parallel()

	fixture := newArtifactFixture(t, []byte("measurement"))
	repository := &memoryRepository{}
	store := newTestStore(t, fixture, repository, int64(len(fixture.contents)+10))
	outsidePath := filepath.Join(t.TempDir(), "outside.bin")
	if err := os.WriteFile(outsidePath, fixture.contents, 0o600); err != nil {
		t.Fatalf("WriteFile(outside) error = %v", err)
	}

	traversalPath := fixture.stagingRoot + string(filepath.Separator) + "nested" + string(filepath.Separator) + ".." + string(filepath.Separator) + filepath.Base(fixture.stagedPath)
	for _, stagedPath := range []string{outsidePath, "relative.bin", traversalPath} {
		input := fixture.registerInput(stagedPath)
		if _, err := store.Register(context.Background(), input); !errors.Is(err, artifact.ErrUnconfinedStagedPath) {
			t.Errorf("Register(%q) error = %v, want ErrUnconfinedStagedPath", stagedPath, err)
		}
	}
}

func TestRegisterAcceptsOnlyTerminalArtifactStates(t *testing.T) {
	t.Parallel()

	for _, state := range []domain.SessionState{domain.PendingApproval, domain.Queued, domain.Running} {
		t.Run(string(state), func(t *testing.T) {
			fixture := newArtifactFixture(t, []byte("measurement"))
			store := newTestStore(t, fixture, &memoryRepository{}, 1024)
			input := fixture.registerInput(fixture.stagedPath)
			input.TerminalState = state

			if _, err := store.Register(context.Background(), input); !errors.Is(err, artifact.ErrInvalidArtifact) {
				t.Fatalf("Register() error = %v, want ErrInvalidArtifact", err)
			}
		})
	}
}

func TestRegisterRejectsGeneratedArtifactIdentityThatIsNotVersionSeven(t *testing.T) {
	t.Parallel()

	fixture := newArtifactFixture(t, []byte("measurement"))
	nonVersionSevenID := domain.ArtifactID(uuid.New())
	store, err := artifact.NewStore(artifact.Config{
		ArtifactRoot: fixture.artifactRoot,
		StagingRoot:  fixture.stagingRoot,
		MaximumBytes: 1024,
	}, artifact.Dependencies{
		Repository:    &memoryRepository{},
		NewArtifactID: func() (domain.ArtifactID, error) { return nonVersionSevenID, nil },
		Now:           func() time.Time { return fixture.createdAt },
	})
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}

	if _, err := store.Register(context.Background(), fixture.registerInput(fixture.stagedPath)); !errors.Is(err, artifact.ErrInvalidArtifact) {
		t.Fatalf("Register() error = %v, want ErrInvalidArtifact", err)
	}
	if _, err := os.Stat(filepath.Join(fixture.artifactRoot, fixture.sessionID.String(), nonVersionSevenID.String())); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("non-v7 artifact destination exists or stat failed: %v", err)
	}
}

func TestRegisterRejectsSymlinkedStagedFile(t *testing.T) {
	t.Parallel()

	fixture := newArtifactFixture(t, []byte("measurement"))
	targetPath := filepath.Join(fixture.stagingRoot, "target.bin")
	linkPath := filepath.Join(fixture.stagingRoot, "link.bin")
	if err := os.Rename(fixture.stagedPath, targetPath); err != nil {
		t.Fatalf("Rename() error = %v", err)
	}
	if err := os.Symlink(targetPath, linkPath); err != nil {
		t.Skipf("symbolic links unavailable: %v", err)
	}
	store := newTestStore(t, fixture, &memoryRepository{}, 1024)
	input := fixture.registerInput(linkPath)

	if _, err := store.Register(context.Background(), input); !errors.Is(err, artifact.ErrSymlinkStagedPath) {
		t.Fatalf("Register() error = %v, want ErrSymlinkStagedPath", err)
	}
}

func TestRegisterRejectsWrongDeclaredSizeAndDigest(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name   string
		mutate func(*artifact.RegisterInput)
		want   error
	}{
		{
			name: "wrong size",
			mutate: func(input *artifact.RegisterInput) {
				input.DeclaredSize++
			},
			want: artifact.ErrSizeMismatch,
		},
		{
			name: "wrong digest",
			mutate: func(input *artifact.RegisterInput) {
				input.DeclaredDigest = sha256.Sum256([]byte("different"))
			},
			want: artifact.ErrDigestMismatch,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newArtifactFixture(t, []byte("measurement"))
			store := newTestStore(t, fixture, &memoryRepository{}, 1024)
			input := fixture.registerInput(fixture.stagedPath)
			test.mutate(&input)

			if _, err := store.Register(context.Background(), input); !errors.Is(err, test.want) {
				t.Fatalf("Register() error = %v, want %v", err, test.want)
			}
			assertNoArtifactDestination(t, fixture)
		})
	}
}

func TestRegisterBoundsActualStreamedBytes(t *testing.T) {
	t.Parallel()

	fixture := newArtifactFixture(t, bytes.Repeat([]byte("x"), 33))
	store := newTestStore(t, fixture, &memoryRepository{}, 32)
	input := fixture.registerInput(fixture.stagedPath)

	if _, err := store.Register(context.Background(), input); !errors.Is(err, artifact.ErrArtifactTooLarge) {
		t.Fatalf("Register() error = %v, want ErrArtifactTooLarge", err)
	}
	assertNoArtifactDestination(t, fixture)
}

func TestRegisterAtomicallyMovesVerifiedBytesAndPersistsMetadata(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	fixture := newArtifactFixture(t, []byte("measurement\x00result"))
	databasePath := filepath.Join(t.TempDir(), "artifacts.db")
	database, err := sqlite.Open(ctx, databasePath)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	seedArtifactForeignKeys(t, database, fixture)
	repository := sqlite.NewArtifactRepository(database)
	store := newTestStore(t, fixture, repository, 1024)
	hints := []byte(`{"label":"oscilloscope trace"}`)
	input := fixture.registerInput(fixture.stagedPath)
	input.PresentationHints = hints

	registered, err := store.Register(ctx, input)
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if registered.ID != fixture.artifactID || registered.SizeBytes != int64(len(fixture.contents)) {
		t.Fatalf("Register() metadata = %#v, want ID %s and size %d", registered, fixture.artifactID.String(), len(fixture.contents))
	}
	if registered.Digest != sha256.Sum256(fixture.contents) {
		t.Fatalf("Register() digest = %x, want %x", registered.Digest, sha256.Sum256(fixture.contents))
	}
	hints[2] = 'X'
	if string(registered.PresentationHints) != `{"label":"oscilloscope trace"}` {
		t.Fatalf("PresentationHints changed with caller buffer: %s", registered.PresentationHints)
	}
	if _, err := os.Stat(fixture.stagedPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("staged source still exists or stat failed: %v", err)
	}
	destination := filepath.Join(fixture.artifactRoot, fixture.sessionID.String(), fixture.artifactID.String())
	destinationBytes, err := os.ReadFile(destination)
	if err != nil {
		t.Fatalf("ReadFile(destination) error = %v", err)
	}
	if !bytes.Equal(destinationBytes, fixture.contents) {
		t.Fatalf("destination bytes = %q, want %q", destinationBytes, fixture.contents)
	}
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(destination), ".*.tmp"))
	if err != nil || len(matches) != 0 {
		t.Fatalf("temporary destinations = %v, glob error = %v", matches, err)
	}

	reader, opened, err := store.Open(ctx, fixture.artifactID)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	openedBytes, readErr := io.ReadAll(reader)
	closeErr := reader.Close()
	if readErr != nil || closeErr != nil {
		t.Fatalf("read error = %v, close error = %v", readErr, closeErr)
	}
	if !bytes.Equal(openedBytes, fixture.contents) || opened.ID != fixture.artifactID {
		t.Fatalf("Open() returned bytes %q and metadata %#v", openedBytes, opened)
	}

	if err := database.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	reopened, err := sqlite.Open(ctx, databasePath)
	if err != nil {
		t.Fatalf("reopen error = %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	persisted, err := sqlite.NewArtifactRepository(reopened).Get(ctx, fixture.artifactID)
	if err != nil {
		t.Fatalf("Get() after reopen error = %v", err)
	}
	if persisted.ID != fixture.artifactID || persisted.Digest != registered.Digest || persisted.CreatedAt != fixture.createdAt {
		t.Fatalf("persisted metadata = %#v, want %#v", persisted, registered)
	}
}

func TestRegisterRemovesOnlyItsNewDestinationWhenMetadataSaveFails(t *testing.T) {
	t.Parallel()

	fixture := newArtifactFixture(t, []byte("measurement"))
	unrelatedDirectory := filepath.Join(fixture.artifactRoot, "unrelated")
	if err := os.MkdirAll(unrelatedDirectory, 0o700); err != nil {
		t.Fatalf("MkdirAll(unrelated) error = %v", err)
	}
	unrelatedPath := filepath.Join(unrelatedDirectory, "keep.bin")
	if err := os.WriteFile(unrelatedPath, []byte("keep"), 0o600); err != nil {
		t.Fatalf("WriteFile(unrelated) error = %v", err)
	}
	repositoryFailure := errors.New("metadata unavailable")
	store := newTestStore(t, fixture, &memoryRepository{saveError: repositoryFailure}, 1024)

	if _, err := store.Register(context.Background(), fixture.registerInput(fixture.stagedPath)); !errors.Is(err, repositoryFailure) {
		t.Fatalf("Register() error = %v, want metadata failure", err)
	}
	assertNoArtifactDestination(t, fixture)
	if contents, err := os.ReadFile(unrelatedPath); err != nil || string(contents) != "keep" {
		t.Fatalf("unrelated file = %q, error = %v", contents, err)
	}
	if contents, err := os.ReadFile(fixture.stagedPath); err != nil || !bytes.Equal(contents, fixture.contents) {
		t.Fatalf("staged source = %q, error = %v", contents, err)
	}
}

func newArtifactFixture(t *testing.T, contents []byte) artifactFixture {
	t.Helper()
	base := t.TempDir()
	fixture := artifactFixture{
		artifactRoot: filepath.Join(base, "durable"),
		stagingRoot:  filepath.Join(base, "incoming"),
		contents:     append([]byte(nil), contents...),
		createdAt:    time.Date(2026, time.August, 30, 12, 30, 0, 123456789, time.UTC),
	}
	if err := os.MkdirAll(fixture.stagingRoot, 0o700); err != nil {
		t.Fatalf("MkdirAll(staging) error = %v", err)
	}
	fixture.stagedPath = filepath.Join(fixture.stagingRoot, "result.bin")
	if err := os.WriteFile(fixture.stagedPath, fixture.contents, 0o600); err != nil {
		t.Fatalf("WriteFile(staged) error = %v", err)
	}
	var err error
	if fixture.artifactID, err = domain.NewArtifactID(); err != nil {
		t.Fatalf("NewArtifactID() error = %v", err)
	}
	if fixture.requestID, err = domain.NewRequestID(); err != nil {
		t.Fatalf("NewRequestID() error = %v", err)
	}
	if fixture.sessionID, err = domain.NewSessionID(); err != nil {
		t.Fatalf("NewSessionID() error = %v", err)
	}
	if fixture.deviceID, err = domain.NewDeviceID(); err != nil {
		t.Fatalf("NewDeviceID() error = %v", err)
	}
	if fixture.integrationID, err = domain.NewIntegrationID(); err != nil {
		t.Fatalf("NewIntegrationID() error = %v", err)
	}
	return fixture
}

func newTestStore(t *testing.T, fixture artifactFixture, repository artifact.Repository, maximumBytes int64) artifact.Store {
	t.Helper()
	store, err := artifact.NewStore(artifact.Config{
		ArtifactRoot: fixture.artifactRoot,
		StagingRoot:  fixture.stagingRoot,
		MaximumBytes: maximumBytes,
	}, artifact.Dependencies{
		Repository:    repository,
		NewArtifactID: func() (domain.ArtifactID, error) { return fixture.artifactID, nil },
		Now:           func() time.Time { return fixture.createdAt },
	})
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	return store
}

func (fixture artifactFixture) registerInput(path string) artifact.RegisterInput {
	return artifact.RegisterInput{
		StagedPath:        path,
		RequestID:         fixture.requestID,
		SessionID:         fixture.sessionID,
		DeviceID:          fixture.deviceID,
		IntegrationID:     fixture.integrationID,
		MediaType:         "application/octet-stream",
		DeclaredDigest:    sha256.Sum256(fixture.contents),
		DeclaredSize:      int64(len(fixture.contents)),
		TerminalState:     domain.Completed,
		PresentationHints: []byte(`{"label":"result"}`),
	}
}

func assertNoArtifactDestination(t *testing.T, fixture artifactFixture) {
	t.Helper()
	destination := filepath.Join(fixture.artifactRoot, fixture.sessionID.String(), fixture.artifactID.String())
	if _, err := os.Stat(destination); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("destination exists or stat failed: %v", err)
	}
}

func seedArtifactForeignKeys(t *testing.T, database *sqlite.DB, fixture artifactFixture) {
	t.Helper()
	actorID, err := domain.NewActorID()
	if err != nil {
		t.Fatalf("NewActorID() error = %v", err)
	}
	configurationID, err := domain.NewSnapshotID()
	if err != nil {
		t.Fatalf("NewSnapshotID() error = %v", err)
	}
	policyID, err := domain.NewSnapshotID()
	if err != nil {
		t.Fatalf("NewSnapshotID() error = %v", err)
	}
	created := fixture.createdAt.Add(-time.Minute).Format(time.RFC3339Nano)
	deadline := fixture.createdAt.Add(time.Minute).Format(time.RFC3339Nano)
	if _, err := database.SQL().Exec(`INSERT INTO config_versions (id, configuration_json, digest, created_at) VALUES (?, '{}', ?, ?)`, configurationID.String(), []byte{1}, created); err != nil {
		t.Fatalf("insert config version: %v", err)
	}
	if _, err := database.SQL().Exec(`
		INSERT INTO operation_requests (
			id, actor_id, integration_id, operation_name, device_id, canonical_parameters,
			configuration_snapshot_id, policy_snapshot_id, deadline_at, created_at
		) VALUES (?, ?, ?, 'capture', ?, '{}', ?, ?, ?, ?)
	`, fixture.requestID.String(), actorID.String(), fixture.integrationID.String(), fixture.deviceID.String(), configurationID.String(), policyID.String(), deadline, created); err != nil {
		t.Fatalf("insert operation request: %v", err)
	}
	if _, err := database.SQL().Exec(`INSERT INTO sessions (id, request_id, state, created_at, updated_at, terminal_at) VALUES (?, ?, 'completed', ?, ?, ?)`, fixture.sessionID.String(), fixture.requestID.String(), created, fixture.createdAt.Format(time.RFC3339Nano), fixture.createdAt.Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("insert session: %v", err)
	}
}
