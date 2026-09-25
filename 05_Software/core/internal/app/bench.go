package app

import (
	"fmt"
	"nilo.local/nilo/internal/bench"
	"os"
)

// StartBench composes the appliance's integration queue. Call only from the
// server, once; command-line clients use the authenticated HTTP boundary.
func (application *App) StartBench(bundleDir, backend string, simulation bool) (*bench.Manager, error) {
	return bench.New(bench.Options{Database: application.database, DataDir: application.configuration.DataDir, BundleDir: bundleDir, Backend: backend, Simulation: simulation, Audit: application.auditWriter, Now: application.now, Report: func(err error) { fmt.Fprintln(os.Stderr, "bench:", err) }})
}
