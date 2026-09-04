package mcp

import (
	"context"
	"crypto/rand"
	"path/filepath"
	"testing"
	"time"

	"nilo.local/nilo/internal/identity"
	"nilo.local/nilo/internal/store/sqlite"
)

// identityServiceForTest builds a real identity service over a real database,
// so an authorization refusal here is the same refusal the HTTP API would give
// rather than a stub agreeing with the test.
func identityServiceForTest(t *testing.T) *identity.Service {
	t.Helper()
	database, err := sqlite.Open(context.Background(), filepath.Join(t.TempDir(), "nilo.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	service, err := identity.NewService(identity.Options{
		Repository:         sqlite.NewIdentityRepository(database),
		Now:                func() time.Time { return time.Now().UTC() },
		Random:             rand.Reader,
		SessionLifetime:    12 * time.Hour,
		LoginFailureLimit:  5,
		LoginFailureWindow: 15 * time.Minute,
	})
	if err != nil {
		t.Fatalf("identity.NewService: %v", err)
	}
	return service
}
