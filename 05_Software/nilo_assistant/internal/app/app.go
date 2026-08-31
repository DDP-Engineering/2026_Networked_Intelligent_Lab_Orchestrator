// Package app composes the durable Increment 1 foundation services.
package app

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"nilo.local/nilo/internal/artifact"
	"nilo.local/nilo/internal/auth"
	"nilo.local/nilo/internal/config"
	"nilo.local/nilo/internal/domain"
	"nilo.local/nilo/internal/session"
	"nilo.local/nilo/internal/store/sqlite"
)

type Options struct {
	DataDir     string
	BindAddress string
	Now         func() time.Time
}

type App struct {
	configuration     config.Config
	database          *sqlite.DB
	sessionRepository *sqlite.SessionRepository
	sessionService    session.Service
	artifactStore     artifact.Store
	authRepository    *sqlite.AuthRepository
	now               func() time.Time
}

// DevelopmentToken is the one-time token result and its attributed user.
type DevelopmentToken struct {
	UserID string
	Token  auth.CreatedToken
}

func New(ctx context.Context, options Options) (*App, error) {
	if options.Now == nil {
		return nil, errors.New("application clock is required")
	}
	configuration, err := config.Load(config.Inputs{DataDir: options.DataDir, BindAddress: options.BindAddress})
	if err != nil {
		return nil, err
	}
	stagingRoot := filepath.Join(configuration.DataDir, "staging")
	for _, directory := range []string{configuration.DataDir, configuration.ArtifactRoot, configuration.IntegrationDir, stagingRoot} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			return nil, fmt.Errorf("create foundation directory %q: %w", directory, err)
		}
	}
	database, err := sqlite.Open(ctx, configuration.DatabasePath)
	if err != nil {
		return nil, err
	}
	sessionRepository := sqlite.NewSessionRepository(database)
	sessionService, err := session.NewService(sessionRepository, func() time.Time { return options.Now().UTC() })
	if err != nil {
		_ = database.Close()
		return nil, err
	}
	if err := sessionService.ReconcileStartup(ctx); err != nil {
		_ = database.Close()
		return nil, fmt.Errorf("reconcile interrupted sessions: %w", err)
	}
	artifactStore, err := artifact.NewStore(
		artifact.Config{ArtifactRoot: configuration.ArtifactRoot, StagingRoot: stagingRoot, MaximumBytes: 64 << 20},
		artifact.Dependencies{Repository: sqlite.NewArtifactRepository(database), NewArtifactID: domain.NewArtifactID, Now: func() time.Time { return options.Now().UTC() }},
	)
	if err != nil {
		_ = database.Close()
		return nil, err
	}
	return &App{
		configuration: configuration, database: database,
		sessionRepository: sessionRepository, sessionService: sessionService,
		artifactStore: artifactStore, authRepository: sqlite.NewAuthRepository(database), now: options.Now,
	}, nil
}

func (application *App) Configuration() config.Config { return application.configuration }

// TokenRepository returns the narrow verifier used by authenticated transports.
func (application *App) TokenRepository() auth.TokenRepository { return application.authRepository }

// IssueDevelopmentToken creates a unique non-login identity and bearer token.
func (application *App) IssueDevelopmentToken(ctx context.Context) (DevelopmentToken, error) {
	if application == nil || application.authRepository == nil || application.now == nil {
		return DevelopmentToken{}, errors.New("application authentication service is required")
	}
	userID, err := uuid.NewV7()
	if err != nil {
		return DevelopmentToken{}, fmt.Errorf("create development user ID: %w", err)
	}
	now := application.now().UTC()
	created, verifier, err := auth.NewToken(rand.Reader, now)
	if err != nil {
		return DevelopmentToken{}, err
	}
	user := auth.UserRecord{
		ID:               userID.String(),
		Username:         "development-" + userID.String(),
		PasswordVerifier: auth.DisabledDevelopmentPasswordVerifier,
		CreatedAt:        now,
	}
	if err := application.authRepository.CreateUserAndToken(ctx, user, created, verifier); err != nil {
		return DevelopmentToken{}, err
	}
	return DevelopmentToken{UserID: user.ID, Token: created}, nil
}

func (application *App) Close() error {
	if application == nil || application.database == nil {
		return nil
	}
	err := application.database.Close()
	application.database = nil
	return err
}
