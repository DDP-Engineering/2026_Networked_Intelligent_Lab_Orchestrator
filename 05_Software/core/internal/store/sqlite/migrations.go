package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	_ "embed"
	"encoding/hex"
	"fmt"
	"time"
)

type migration struct {
	version string
	sql     string
}

//go:embed migrations/001_initial.sql
var initialMigration string

//go:embed migrations/002_policy_snapshots.sql
var policySnapshotsMigration string

//go:embed migrations/003_active_configuration.sql
var activeConfigurationMigration string

//go:embed migrations/004_identity_access_control.sql
var identityAccessControlMigration string

//go:embed migrations/005_network_traffic.sql
var networkTrafficMigration string

//go:embed migrations/006_appliance_audit_operations.sql
var applianceAuditOperationsMigration string

//go:embed migrations/007_vpn_profiles.sql
var vpnProfilesMigration string

//go:embed migrations/008_interface_settings.sql
var interfaceSettingsMigration string

//go:embed migrations/010_activity.sql
var activityMigration string

//go:embed migrations/009_device_networks.sql
var deviceNetworksMigration string

//go:embed migrations/011_bench.sql
var benchMigration string

//go:embed migrations/012_management_access.sql
var managementAccessMigration string

//go:embed migrations/013_commissioning.sql
var commissioningMigration string

//go:embed migrations/014_network_recovery.sql
var networkRecoveryMigration string

//go:embed migrations/015_password_policy.sql
var passwordPolicyMigration string

//go:embed migrations/016_password_policy_classes.sql
var passwordPolicyClassesMigration string

//go:embed migrations/017_appliance_discovery.sql
var applianceDiscoveryMigration string

//go:embed migrations/018_integration_package_state.sql
var integrationPackageStateMigration string

//go:embed migrations/019_managed_devices.sql
var managedDevicesMigration string

//go:embed migrations/020_bench_plugin_configuration.sql
var benchPluginConfigurationMigration string

//go:embed migrations/021_camera_workspace.sql
var cameraWorkspaceMigration string

//go:embed migrations/022_serial_workspace.sql
var serialWorkspaceMigration string

//go:embed migrations/023_operating_settings.sql
var operatingSettingsMigration string

//go:embed migrations/024_modbus_definitions.sql
var modbusDefinitionsMigration string

//go:embed migrations/025_modbus_sessions.sql
var modbusSessionsMigration string

//go:embed migrations/026_modbus_maps_workspaces.sql
var modbusMapsWorkspacesMigration string

//go:embed migrations/027_modbus_settings.sql
var modbusSettingsMigration string

//go:embed migrations/028_modbus_recordings.sql
var modbusRecordingsMigration string

var migrations = []migration{
	{version: "001_initial", sql: initialMigration},
	{version: "002_policy_snapshots", sql: policySnapshotsMigration},
	{version: "003_active_configuration", sql: activeConfigurationMigration},
	{version: "004_identity_access_control", sql: identityAccessControlMigration},
	{version: "005_network_traffic", sql: networkTrafficMigration},
	{version: "006_appliance_audit_operations", sql: applianceAuditOperationsMigration},
	{version: "007_vpn_profiles", sql: vpnProfilesMigration},
	{version: "008_interface_settings", sql: interfaceSettingsMigration},
	{version: "009_device_networks", sql: deviceNetworksMigration},
	{version: "010_activity", sql: activityMigration},
	{version: "011_bench", sql: benchMigration},
	{version: "012_management_access", sql: managementAccessMigration},
	{version: "013_commissioning", sql: commissioningMigration},
	{version: "014_network_recovery", sql: networkRecoveryMigration},
	{version: "015_password_policy", sql: passwordPolicyMigration},
	{version: "016_password_policy_classes", sql: passwordPolicyClassesMigration},
	{version: "017_appliance_discovery", sql: applianceDiscoveryMigration},
	{version: "018_integration_package_state", sql: integrationPackageStateMigration},
	{version: "019_managed_devices", sql: managedDevicesMigration},
	{version: "020_bench_plugin_configuration", sql: benchPluginConfigurationMigration},
	{version: "021_camera_workspace", sql: cameraWorkspaceMigration},
	{version: "022_serial_workspace", sql: serialWorkspaceMigration},
	{version: "023_operating_settings", sql: operatingSettingsMigration},
	{version: "024_modbus_definitions", sql: modbusDefinitionsMigration},
	{version: "025_modbus_sessions", sql: modbusSessionsMigration},
	{version: "026_modbus_maps_workspaces", sql: modbusMapsWorkspacesMigration},
	{version: "027_modbus_settings", sql: modbusSettingsMigration},
	{version: "028_modbus_recordings", sql: modbusRecordingsMigration},
}

func applyMigrations(ctx context.Context, database *sql.DB) error {
	return applyMigrationSet(ctx, database, migrations)
}

// applyMigrationSet exists so a test can build a database at an earlier schema
// and prove that opening it upgrades in place without disturbing what it holds.
// Production always passes the whole set.
func applyMigrationSet(ctx context.Context, database *sql.DB, set []migration) (err error) {
	connection, err := database.Conn(ctx)
	if err != nil {
		return fmt.Errorf("reserve SQLite migration connection: %w", err)
	}
	defer func() {
		closeErr := connection.Close()
		if err == nil && closeErr != nil {
			err = fmt.Errorf("release SQLite migration connection: %w", closeErr)
		}
	}()

	if _, err := connection.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return fmt.Errorf("begin immediate SQLite migration transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = connection.ExecContext(context.Background(), "ROLLBACK")
		}
	}()

	if _, err := connection.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version TEXT PRIMARY KEY,
			digest TEXT NOT NULL,
			applied_at TEXT NOT NULL
		)
	`); err != nil {
		return fmt.Errorf("create schema migration ledger: %w", err)
	}

	for _, migration := range set {
		digest := migrationDigest(migration.sql)
		var recordedDigest string
		err := connection.QueryRowContext(
			ctx,
			"SELECT digest FROM schema_migrations WHERE version = ?",
			migration.version,
		).Scan(&recordedDigest)
		switch {
		case err == nil:
			if recordedDigest != digest {
				return fmt.Errorf("migration %s digest mismatch", migration.version)
			}
			continue
		case err != sql.ErrNoRows:
			return fmt.Errorf("read migration %s ledger record: %w", migration.version, err)
		}

		if _, err := connection.ExecContext(ctx, migration.sql); err != nil {
			return fmt.Errorf("apply migration %s: %w", migration.version, err)
		}
		if _, err := connection.ExecContext(
			ctx,
			"INSERT INTO schema_migrations (version, digest, applied_at) VALUES (?, ?, ?)",
			migration.version,
			digest,
			time.Now().UTC().Format(time.RFC3339Nano),
		); err != nil {
			return fmt.Errorf("record migration %s: %w", migration.version, err)
		}
	}

	if _, err := connection.ExecContext(ctx, "COMMIT"); err != nil {
		return fmt.Errorf("commit SQLite migration transaction: %w", err)
	}
	committed = true
	return nil
}

func migrationDigest(sql string) string {
	digest := sha256.Sum256([]byte(sql))
	return hex.EncodeToString(digest[:])
}
