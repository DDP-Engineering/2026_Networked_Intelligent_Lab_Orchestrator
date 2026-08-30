package artifact

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"nilo.local/nilo/internal/domain"
)

var (
	ErrInvalidStoreConfiguration = errors.New("invalid artifact store configuration")
	ErrInvalidArtifact           = errors.New("invalid artifact metadata")
	ErrUnconfinedStagedPath      = errors.New("staged path is not confined")
	ErrSymlinkStagedPath         = errors.New("staged path contains a symbolic link")
	ErrArtifactTooLarge          = errors.New("artifact exceeds configured maximum")
	ErrSizeMismatch              = errors.New("artifact size does not match declaration")
	ErrDigestMismatch            = errors.New("artifact digest does not match declaration")
	ErrArtifactNotFound          = errors.New("artifact not found")
	ErrArtifactAlreadyRegistered = errors.New("artifact already registered")
)

type Repository interface {
	Save(ctx context.Context, value domain.Artifact) error
	Get(ctx context.Context, id domain.ArtifactID) (domain.Artifact, error)
}

type Store interface {
	Register(ctx context.Context, input RegisterInput) (domain.Artifact, error)
	Open(ctx context.Context, id domain.ArtifactID) (io.ReadCloser, domain.Artifact, error)
}

type Config struct {
	ArtifactRoot string
	StagingRoot  string
	MaximumBytes int64
}

type Dependencies struct {
	Repository    Repository
	NewArtifactID func() (domain.ArtifactID, error)
	Now           func() time.Time
}

type RegisterInput struct {
	StagedPath        string
	RequestID         domain.RequestID
	SessionID         domain.SessionID
	DeviceID          domain.DeviceID
	IntegrationID     domain.IntegrationID
	MediaType         string
	DeclaredDigest    [32]byte
	DeclaredSize      int64
	TerminalState     domain.SessionState
	PresentationHints json.RawMessage
}

type store struct {
	artifactRoot  string
	stagingRoot   string
	maximumBytes  int64
	repository    Repository
	newArtifactID func() (domain.ArtifactID, error)
	now           func() time.Time
}

func NewStore(config Config, dependencies Dependencies) (Store, error) {
	artifactRoot, err := prepareRoot(config.ArtifactRoot)
	if err != nil {
		return nil, fmt.Errorf("%w: artifact root: %v", ErrInvalidStoreConfiguration, err)
	}
	stagingRoot, err := prepareRoot(config.StagingRoot)
	if err != nil {
		return nil, fmt.Errorf("%w: staging root: %v", ErrInvalidStoreConfiguration, err)
	}
	if samePath(artifactRoot, stagingRoot) || pathContains(artifactRoot, stagingRoot) || pathContains(stagingRoot, artifactRoot) {
		return nil, fmt.Errorf("%w: artifact and staging roots must be disjoint", ErrInvalidStoreConfiguration)
	}
	if config.MaximumBytes <= 0 {
		return nil, fmt.Errorf("%w: maximum bytes must be positive", ErrInvalidStoreConfiguration)
	}
	if dependencies.Repository == nil || dependencies.NewArtifactID == nil || dependencies.Now == nil {
		return nil, fmt.Errorf("%w: repository, ID generator, and clock are required", ErrInvalidStoreConfiguration)
	}
	return &store{
		artifactRoot: artifactRoot, stagingRoot: stagingRoot, maximumBytes: config.MaximumBytes,
		repository: dependencies.Repository, newArtifactID: dependencies.NewArtifactID, now: dependencies.Now,
	}, nil
}

func (storage *store) Register(ctx context.Context, input RegisterInput) (domain.Artifact, error) {
	if err := storage.validateInput(input); err != nil {
		return domain.Artifact{}, err
	}
	stagedRelative, err := confinedRelative(storage.stagingRoot, input.StagedPath)
	if err != nil {
		return domain.Artifact{}, err
	}
	stagingRoot, err := os.OpenRoot(storage.stagingRoot)
	if err != nil {
		return domain.Artifact{}, fmt.Errorf("open staging root: %w", err)
	}
	defer stagingRoot.Close()
	stagedInfo, err := rejectSymlinkComponents(stagingRoot, stagedRelative)
	if err != nil {
		return domain.Artifact{}, err
	}
	if !stagedInfo.Mode().IsRegular() {
		return domain.Artifact{}, fmt.Errorf("%w: staged path is not a regular file", ErrUnconfinedStagedPath)
	}
	source, err := stagingRoot.Open(stagedRelative)
	if err != nil {
		return domain.Artifact{}, fmt.Errorf("open staged artifact: %w", err)
	}
	defer source.Close()
	openedInfo, err := source.Stat()
	if err != nil {
		return domain.Artifact{}, fmt.Errorf("stat opened staged artifact: %w", err)
	}
	if !openedInfo.Mode().IsRegular() || !os.SameFile(stagedInfo, openedInfo) {
		return domain.Artifact{}, fmt.Errorf("%w: staged file changed while opening", ErrSymlinkStagedPath)
	}

	artifactID, err := storage.newArtifactID()
	if err != nil {
		return domain.Artifact{}, fmt.Errorf("allocate artifact ID: %w", err)
	}
	if generatedUUID := uuid.UUID(artifactID); generatedUUID == uuid.Nil || generatedUUID.Version() != uuid.Version(7) {
		return domain.Artifact{}, fmt.Errorf("%w: generated artifact ID must be UUID-v7", ErrInvalidArtifact)
	}
	artifactRoot, err := os.OpenRoot(storage.artifactRoot)
	if err != nil {
		return domain.Artifact{}, fmt.Errorf("open artifact root: %w", err)
	}
	defer artifactRoot.Close()
	sessionRelative := input.SessionID.String()
	if err := artifactRoot.MkdirAll(sessionRelative, 0o700); err != nil {
		return domain.Artifact{}, fmt.Errorf("create artifact session directory: %w", err)
	}
	sessionInfo, err := artifactRoot.Lstat(sessionRelative)
	if err != nil || sessionInfo.Mode()&os.ModeSymlink != 0 || !sessionInfo.IsDir() {
		return domain.Artifact{}, fmt.Errorf("artifact session directory is unsafe: %w", err)
	}
	temporaryRelative := filepath.Join(sessionRelative, "."+artifactID.String()+".tmp")
	destinationRelative := filepath.Join(sessionRelative, artifactID.String())
	temporary, err := artifactRoot.OpenFile(temporaryRelative, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return domain.Artifact{}, fmt.Errorf("create artifact staging destination: %w", err)
	}
	temporaryClosed := false
	defer func() {
		if !temporaryClosed {
			_ = temporary.Close()
		}
		_ = artifactRoot.Remove(temporaryRelative)
	}()

	hasher := sha256.New()
	written, err := io.Copy(io.MultiWriter(temporary, hasher), io.LimitReader(source, storage.maximumBytes+1))
	if err != nil {
		return domain.Artifact{}, fmt.Errorf("stream staged artifact: %w", err)
	}
	if written > storage.maximumBytes {
		return domain.Artifact{}, ErrArtifactTooLarge
	}
	if written != input.DeclaredSize {
		return domain.Artifact{}, fmt.Errorf("%w: declared %d, streamed %d", ErrSizeMismatch, input.DeclaredSize, written)
	}
	var digest [sha256.Size]byte
	copy(digest[:], hasher.Sum(nil))
	if subtle.ConstantTimeCompare(digest[:], input.DeclaredDigest[:]) != 1 {
		return domain.Artifact{}, ErrDigestMismatch
	}
	if err := temporary.Sync(); err != nil {
		return domain.Artifact{}, fmt.Errorf("sync verified artifact: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return domain.Artifact{}, fmt.Errorf("close verified artifact: %w", err)
	}
	temporaryClosed = true
	if _, err := artifactRoot.Lstat(destinationRelative); err == nil {
		return domain.Artifact{}, ErrArtifactAlreadyRegistered
	} else if !errors.Is(err, os.ErrNotExist) {
		return domain.Artifact{}, fmt.Errorf("check artifact destination: %w", err)
	}
	if err := artifactRoot.Rename(temporaryRelative, destinationRelative); err != nil {
		return domain.Artifact{}, fmt.Errorf("atomically place verified artifact: %w", err)
	}

	value := domain.Artifact{
		ID: artifactID, RequestID: input.RequestID, SessionID: input.SessionID,
		DeviceID: input.DeviceID, IntegrationID: input.IntegrationID, MediaType: input.MediaType,
		Digest: digest, SizeBytes: written, TerminalState: input.TerminalState,
		PresentationHints: append([]byte(nil), input.PresentationHints...), CreatedAt: storage.now().UTC(),
	}
	if value.CreatedAt.IsZero() {
		_ = artifactRoot.Remove(destinationRelative)
		return domain.Artifact{}, fmt.Errorf("%w: clock returned zero time", ErrInvalidArtifact)
	}
	if err := storage.repository.Save(ctx, value); err != nil {
		removeErr := artifactRoot.Remove(destinationRelative)
		if removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			return domain.Artifact{}, fmt.Errorf("save artifact metadata: %w (cleanup destination: %v)", err, removeErr)
		}
		return domain.Artifact{}, fmt.Errorf("save artifact metadata: %w", err)
	}
	_ = stagingRoot.Remove(stagedRelative)
	return cloneArtifact(value), nil
}

func (storage *store) Open(ctx context.Context, id domain.ArtifactID) (io.ReadCloser, domain.Artifact, error) {
	if isZeroUUID(id.String()) {
		return nil, domain.Artifact{}, fmt.Errorf("%w: artifact ID is required", ErrInvalidArtifact)
	}
	value, err := storage.repository.Get(ctx, id)
	if err != nil {
		return nil, domain.Artifact{}, err
	}
	artifactRoot, err := os.OpenRoot(storage.artifactRoot)
	if err != nil {
		return nil, domain.Artifact{}, fmt.Errorf("open artifact root: %w", err)
	}
	defer artifactRoot.Close()
	artifactRelative := filepath.Join(value.SessionID.String(), value.ID.String())
	info, err := artifactRoot.Lstat(artifactRelative)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, domain.Artifact{}, ErrArtifactNotFound
		}
		return nil, domain.Artifact{}, fmt.Errorf("inspect artifact: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, domain.Artifact{}, fmt.Errorf("%w: durable artifact is not a regular file", ErrArtifactNotFound)
	}
	file, err := artifactRoot.Open(artifactRelative)
	if err != nil {
		return nil, domain.Artifact{}, fmt.Errorf("open artifact: %w", err)
	}
	return file, cloneArtifact(value), nil
}

func (storage *store) validateInput(input RegisterInput) error {
	if input.DeclaredSize < 0 {
		return fmt.Errorf("%w: declared size cannot be negative", ErrInvalidArtifact)
	}
	if input.DeclaredSize > storage.maximumBytes {
		return ErrArtifactTooLarge
	}
	if isZeroUUID(input.RequestID.String()) || isZeroUUID(input.SessionID.String()) ||
		isZeroUUID(input.DeviceID.String()) || isZeroUUID(input.IntegrationID.String()) {
		return fmt.Errorf("%w: producing identities are required", ErrInvalidArtifact)
	}
	if strings.TrimSpace(input.MediaType) == "" {
		return fmt.Errorf("%w: media type is required", ErrInvalidArtifact)
	}
	if !isTerminalArtifactState(input.TerminalState) {
		return fmt.Errorf("%w: terminal state %q cannot produce an artifact", ErrInvalidArtifact, input.TerminalState)
	}
	if len(input.PresentationHints) == 0 || !json.Valid(input.PresentationHints) {
		return fmt.Errorf("%w: presentation hints must be valid JSON", ErrInvalidArtifact)
	}
	return nil
}

func prepareRoot(root string) (string, error) {
	if strings.TrimSpace(root) == "" || !filepath.IsAbs(root) {
		return "", errors.New("root must be absolute")
	}
	clean := filepath.Clean(root)
	if err := os.MkdirAll(clean, 0o700); err != nil {
		return "", err
	}
	info, err := os.Lstat(clean)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return "", errors.New("root must be a real directory")
	}
	return clean, nil
}

func confinedRelative(root, candidate string) (string, error) {
	if strings.TrimSpace(candidate) == "" || !filepath.IsAbs(candidate) {
		return "", ErrUnconfinedStagedPath
	}
	clean := filepath.Clean(candidate)
	if clean != candidate {
		return "", ErrUnconfinedStagedPath
	}
	relative, err := filepath.Rel(root, clean)
	if err != nil || relative == "." || filepath.IsAbs(relative) {
		return "", ErrUnconfinedStagedPath
	}
	components := strings.Split(relative, string(filepath.Separator))
	if len(components) == 0 || components[0] == ".." {
		return "", ErrUnconfinedStagedPath
	}
	return relative, nil
}

func rejectSymlinkComponents(root *os.Root, relative string) (os.FileInfo, error) {
	components := strings.Split(relative, string(filepath.Separator))
	current := ""
	var info os.FileInfo
	for _, component := range components {
		current = filepath.Join(current, component)
		var err error
		info, err = root.Lstat(current)
		if err != nil {
			return nil, fmt.Errorf("inspect staged path: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, ErrSymlinkStagedPath
		}
	}
	return info, nil
}

func samePath(left, right string) bool {
	return strings.EqualFold(filepath.Clean(left), filepath.Clean(right))
}

func pathContains(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	if err != nil || relative == "." || filepath.IsAbs(relative) {
		return false
	}
	components := strings.Split(relative, string(filepath.Separator))
	return len(components) > 0 && components[0] != ".."
}

func isZeroUUID(value string) bool { return value == uuid.Nil.String() }

func isTerminalArtifactState(state domain.SessionState) bool {
	return state == domain.Completed || state == domain.Failed || state == domain.Cancelled || state == domain.Incomplete
}

func cloneArtifact(value domain.Artifact) domain.Artifact {
	value.PresentationHints = append([]byte(nil), value.PresentationHints...)
	return value
}

var _ Store = (*store)(nil)
