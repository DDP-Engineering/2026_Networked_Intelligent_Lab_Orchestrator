package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"nilo.local/nilo/internal/appliance"
)

// A clean installation has no appliance identity when the migration runs,
// because sqlite.Open applies the whole set before anything provisions one.
func TestNewDatabaseRecordsProvisioningCommissioningState(t *testing.T) {
	t.Parallel()

	database, err := Open(context.Background(), filepath.Join(t.TempDir(), "nilo.sqlite"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close: %v", err)
		}
	})

	state, err := NewCommissioningRepository(database).LoadCommissioning(context.Background())
	if err != nil {
		t.Fatalf("LoadCommissioning: %v", err)
	}
	if state != appliance.CommissioningProvisioning {
		t.Fatalf("state = %q, want %q", state, appliance.CommissioningProvisioning)
	}
}

// An upgraded appliance already holds its identity when the migration arrives,
// and must not be pulled back into first-boot provisioning. This covers the
// interface-managed and the legacy single-listener deployment alike, because
// both hold the identity row and neither is detected separately.
func TestUpgradedDatabaseRecordsCommissionedCommissioningState(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "nilo.sqlite")

	earlier, err := sql.Open(driverName, sqliteDSN(path))
	if err != nil {
		t.Fatalf("open at the earlier schema: %v", err)
	}
	earlier.SetMaxOpenConns(1)
	if err := configureConnection(ctx, earlier); err != nil {
		t.Fatalf("configure: %v", err)
	}
	// Everything up to and including 012_management_access, which is what an
	// appliance running the previous release has.
	previous := migrations[:len(migrations)-1]
	if previous[len(previous)-1].version != "012_management_access" {
		t.Fatalf("migration before the commissioning one = %q, want 012_management_access",
			previous[len(previous)-1].version)
	}
	if err := applyMigrationSet(ctx, earlier, previous); err != nil {
		t.Fatalf("apply the earlier schema: %v", err)
	}
	if _, err := earlier.ExecContext(ctx, `
		INSERT INTO appliance_identity (id, appliance_id, name, location, commissioned_at)
		VALUES (1, '0b6e0d3a-1f2c-4a5b-9c8d-000000000001', 'deployed bench', 'lab', '2026-09-01T00:00:00Z')
	`); err != nil {
		t.Fatalf("write the identity row at the earlier schema: %v", err)
	}
	if err := earlier.Close(); err != nil {
		t.Fatalf("close the earlier database: %v", err)
	}

	upgraded, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("upgrade in place: %v", err)
	}
	t.Cleanup(func() { _ = upgraded.Close() })

	state, err := NewCommissioningRepository(upgraded).LoadCommissioning(ctx)
	if err != nil {
		t.Fatalf("LoadCommissioning: %v", err)
	}
	if state != appliance.CommissioningCommissioned {
		t.Fatalf("state = %q, want %q", state, appliance.CommissioningCommissioned)
	}

	var applianceID, name string
	if err := upgraded.SQL().QueryRowContext(ctx,
		"SELECT appliance_id, name FROM appliance_identity WHERE id = 1").Scan(&applianceID, &name); err != nil {
		t.Fatalf("read the identity row after upgrade: %v", err)
	}
	if applianceID != "0b6e0d3a-1f2c-4a5b-9c8d-000000000001" || name != "deployed bench" {
		t.Fatalf("the stored identity changed on upgrade: %q/%q", applianceID, name)
	}
	if got := queryInt(t, upgraded.SQL(), "SELECT COUNT(*) FROM schema_migrations"); got != len(migrations) {
		t.Fatalf("schema_migrations count = %d, want %d", got, len(migrations))
	}
}

// The two CHECK constraints are what make the boundary observable at the
// database, not only in Go.
func TestCommissioningTableRejectsAnUnknownStateAndASecondRow(t *testing.T) {
	t.Parallel()

	database, err := Open(context.Background(), filepath.Join(t.TempDir(), "nilo.sqlite"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close: %v", err)
		}
	})

	for _, testCase := range []struct {
		name      string
		statement string
	}{
		{
			name:      "unknown state",
			statement: "UPDATE commissioning SET state = 'decommissioned' WHERE singleton = 1",
		},
		{
			name:      "second row",
			statement: "INSERT INTO commissioning (singleton, state) VALUES (2, 'provisioning')",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := database.SQL().ExecContext(context.Background(), testCase.statement); err == nil {
				t.Fatalf("statement %q was accepted", testCase.statement)
			}
		})
	}
}

func TestCommissioningRepositoryRefusesAnAbsentRow(t *testing.T) {
	t.Parallel()

	database, err := Open(context.Background(), filepath.Join(t.TempDir(), "nilo.sqlite"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close: %v", err)
		}
	})
	if _, err := database.SQL().ExecContext(context.Background(), "DELETE FROM commissioning"); err != nil {
		t.Fatalf("remove the recorded state: %v", err)
	}

	_, err = NewCommissioningRepository(database).LoadCommissioning(context.Background())
	if err == nil {
		t.Fatal("LoadCommissioning returned no error for a database with no recorded state")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "commissioning") {
		t.Fatalf("error %q does not name the missing commissioning state", err)
	}
}
