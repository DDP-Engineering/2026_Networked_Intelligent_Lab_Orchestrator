package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"path/filepath"

	"nilo.local/nilo/internal/maintenance"
)

const maintenanceUsage = `usage: nilo maintenance <command>

  nilo maintenance inventory --data-dir <absolute-path>
  nilo maintenance snapshot --data-dir <absolute-path> --transaction <id>
  nilo maintenance restore --data-dir <absolute-path> --snapshot <absolute-path> --failed-target-version <version>
  nilo maintenance remove --data-dir <absolute-path>
  nilo maintenance purge --data-dir <absolute-path> --confirm-local`

func runMaintenanceCommand(_ context.Context, arguments []string, streams adminIO) int {
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
	if err := flags.Parse(arguments[1:]); err != nil || flags.NArg() != 0 || !filepath.IsAbs(*dataDir) {
		fmt.Fprintln(streams.stderr, maintenanceUsage)
		return 2
	}
	encoder := json.NewEncoder(streams.stdout)
	switch command {
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
		files, err := maintenance.Inventory(*dataDir)
		if err != nil {
			fmt.Fprintln(streams.stderr, err)
			return 1
		}
		_ = encoder.Encode(map[string]any{"files": len(files), "schema": 1, "state": "preserved"})
		return 0
	case "purge":
		if !*confirm {
			fmt.Fprintln(streams.stderr, "purge requires --confirm-local")
			return 2
		}
		if err := maintenance.Purge(*dataDir); err != nil {
			fmt.Fprintln(streams.stderr, err)
			return 1
		}
		_ = encoder.Encode(map[string]any{"schema": 1, "state": "purged"})
		return 0
	default:
		fmt.Fprintln(streams.stderr, maintenanceUsage)
		return 2
	}
}
