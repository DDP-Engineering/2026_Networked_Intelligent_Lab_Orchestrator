package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"nilo.local/nilo/internal/maintenance"
	"nilo.local/nilo/internal/nethelper"
	"nilo.local/nilo/internal/store/sqlite"
)

const maintenanceUsage = `usage: nilo maintenance <command>

  nilo maintenance inventory --data-dir <absolute-path>
  nilo maintenance migrate --data-dir <absolute-path>
  nilo maintenance snapshot --data-dir <absolute-path> --transaction <id>
  nilo maintenance restore --data-dir <absolute-path> --snapshot <absolute-path> --failed-target-version <version>
  nilo maintenance remove --data-dir <absolute-path> [--nethelper-socket <absolute-path>]
  nilo maintenance purge --data-dir <absolute-path> --confirm-local [--nethelper-socket <absolute-path>]`

func cleanupOwnedResources(
	ctx context.Context, dataDir, socket string, localConfirmed, purgeState bool,
) (nethelper.CleanupReport, error) {
	report := nethelper.CleanupReport{
		Schema:   1,
		Removed:  []nethelper.CleanupDisposition{},
		Retained: []nethelper.CleanupDisposition{},
		Failed:   []nethelper.CleanupDisposition{},
	}
	databasePath := filepath.Join(dataDir, "nilo.db")
	if _, err := os.Lstat(databasePath); err != nil {
		if os.IsNotExist(err) {
			return report, nil
		}
		return report, err
	}
	database, err := sqlite.Open(ctx, databasePath)
	if err != nil {
		return report, err
	}
	profiles, listErr := sqlite.NewVPNProfileRepository(database).List(ctx)
	closeErr := database.Close()
	if listErr != nil || closeErr != nil {
		return report, errors.Join(listErr, closeErr)
	}
	owned := make([]nethelper.OwnedTunnel, 0, len(profiles))
	for _, profile := range profiles {
		if profile.ConnectionRef != "" {
			owned = append(owned, nethelper.OwnedTunnel{
				ProfileID: profile.ID, ConnectionRef: profile.ConnectionRef,
			})
		}
	}
	socketInfo, err := os.Lstat(socket)
	if err != nil {
		if os.IsNotExist(err) {
			if len(owned) != 0 {
				return report, errors.New("privileged helper is unavailable while owned network records remain")
			}
			return report, nil
		}
		return report, err
	}
	if socketInfo.Mode()&os.ModeSymlink != 0 || socketInfo.Mode()&os.ModeSocket == 0 {
		return report, errors.New("privileged helper path is not a real Unix socket")
	}
	client := nethelper.NewClient(socket, "")
	return client.CleanupOwned(ctx, owned, localConfirmed, purgeState)
}

func runMaintenanceCommand(ctx context.Context, arguments []string, streams adminIO) int {
	if len(arguments) == 0 {
		fmt.Fprintln(streams.stderr, maintenanceUsage)
		return 2
	}
	command := arguments[0]
	flags := flag.NewFlagSet("maintenance "+command, flag.ContinueOnError)
	flags.SetOutput(streams.stderr)
	dataDir := flags.String("data-dir", "", "absolute runtime data directory")
	transaction := flags.String("transaction", "", "snapshot transaction identifier")
	targetVersion := flags.String("target-version", "", "package version that will consume the snapshot")
	failedTargetVersion := flags.String("failed-target-version", "", "failed package version whose upgrade created the snapshot")
	snapshot := flags.String("snapshot", "", "absolute snapshot directory")
	confirm := flags.Bool("confirm-local", false, "confirm local application-state deletion")
	nethelperSocket := flags.String("nethelper-socket", "/run/nilo/nethelper.sock", "installed privileged-helper socket")
	if err := flags.Parse(arguments[1:]); err != nil || flags.NArg() != 0 || !filepath.IsAbs(*dataDir) {
		fmt.Fprintln(streams.stderr, maintenanceUsage)
		return 2
	}
	encoder := json.NewEncoder(streams.stdout)
	switch command {
	case "migrate":
		schemaIdentity, err := sqlite.CheckpointAndInspect(ctx, filepath.Join(*dataDir, "nilo.db"))
		if err != nil {
			fmt.Fprintln(streams.stderr, err)
			return 1
		}
		if err := encoder.Encode(map[string]any{
			"schema": 1, "schema_identity": schemaIdentity, "status": "migrated",
		}); err != nil {
			fmt.Fprintln(streams.stderr, err)
			return 1
		}
		return 0
	case "inventory":
		files, err := maintenance.Inventory(*dataDir)
		if err != nil {
			fmt.Fprintln(streams.stderr, err)
			return 1
		}
		if err := encoder.Encode(struct {
			Files  []maintenance.File `json:"files"`
			Schema int                `json:"schema"`
		}{Files: files, Schema: 1}); err != nil {
			fmt.Fprintln(streams.stderr, err)
			return 1
		}
		return 0
	case "snapshot":
		if *transaction == "" {
			fmt.Fprintln(streams.stderr, "--transaction is required")
			return 2
		}
		result, err := maintenance.Snapshot(*dataDir, *transaction, *targetVersion)
		if err != nil {
			fmt.Fprintln(streams.stderr, err)
			return 1
		}
		if err := encoder.Encode(result); err != nil {
			fmt.Fprintln(streams.stderr, err)
			return 1
		}
		return 0
	case "restore":
		if !filepath.IsAbs(*snapshot) || *failedTargetVersion == "" {
			fmt.Fprintln(streams.stderr, "--snapshot must be absolute and --failed-target-version is required")
			return 2
		}
		if err := maintenance.Restore(*dataDir, *snapshot, *failedTargetVersion); err != nil {
			fmt.Fprintln(streams.stderr, err)
			return 1
		}
		_ = encoder.Encode(map[string]any{"schema": 1, "snapshot": *snapshot, "status": "restored"})
		return 0
	case "remove":
		cleanup, err := cleanupOwnedResources(ctx, *dataDir, *nethelperSocket, false, false)
		if err != nil {
			_ = encoder.Encode(map[string]any{"cleanup": cleanup, "schema": 1, "state": "preserved_with_cleanup_failure"})
			fmt.Fprintln(streams.stderr, err)
			return 1
		}
		files, err := maintenance.Inventory(*dataDir)
		if err != nil {
			fmt.Fprintln(streams.stderr, err)
			return 1
		}
		_ = encoder.Encode(map[string]any{"cleanup": cleanup, "files": len(files), "schema": 1, "state": "preserved"})
		return 0
	case "purge":
		if !*confirm {
			fmt.Fprintln(streams.stderr, "purge requires --confirm-local")
			return 2
		}
		backup, err := maintenance.CreatePurgeBackup(*dataDir)
		if err != nil {
			fmt.Fprintln(streams.stderr, err)
			return 1
		}
		cleanup, err := cleanupOwnedResources(ctx, *dataDir, *nethelperSocket, true, true)
		if err != nil {
			_ = encoder.Encode(map[string]any{"backup": backup, "cleanup": cleanup, "schema": 1, "state": "backup_retained_cleanup_failed"})
			fmt.Fprintln(streams.stderr, err)
			return 1
		}
		if err := maintenance.Purge(*dataDir); err != nil {
			fmt.Fprintln(streams.stderr, err)
			return 1
		}
		_ = encoder.Encode(map[string]any{"backup": backup, "cleanup": cleanup, "schema": 1, "state": "purged"})
		return 0
	default:
		fmt.Fprintln(streams.stderr, maintenanceUsage)
		return 2
	}
}
