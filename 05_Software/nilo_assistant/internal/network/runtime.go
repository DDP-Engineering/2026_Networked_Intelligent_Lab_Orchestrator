package network

import (
	"context"
	"time"
)

// StartupRetryInterval is how often the maintenance loop retries connecting
// the startup selection while a prerequisite is missing.
const StartupRetryInterval = 30 * time.Second

// MaintenanceOptions are the periodic duties the runtime owns.
type MaintenanceOptions struct {
	Sampler    *Sampler
	Operations *OperationManager
	// Startup retries the startup selection until it reports it is done. It is
	// optional: a runtime without one has nothing to connect.
	Startup *Startup
	// Reconcile brings installed device-network forwarding in step with what is
	// present. It runs on the sampling cadence; a runtime without device
	// networks leaves it nil.
	Reconcile func(context.Context) error
	// Housekeeping runs on its own slow cadence: retention sweeps. Optional.
	Housekeeping func(context.Context) error
	Report       func(error)
}

// HousekeepingInterval is how often retention is applied.
const HousekeepingInterval = 5 * time.Minute

// RunMaintenance samples traffic and expires unconfirmed operations until the
// runtime is cancelled. The caller must join it before closing the database.
func RunMaintenance(ctx context.Context, sampler *Sampler, operations *OperationManager, report func(error)) {
	RunMaintenanceWith(ctx, MaintenanceOptions{Sampler: sampler, Operations: operations, Report: report})
}

// RunMaintenanceWith is RunMaintenance with every duty the runtime can own.
func RunMaintenanceWith(ctx context.Context, options MaintenanceOptions) {
	samples := time.NewTicker(SampleInterval)
	defer samples.Stop()
	expiry := time.NewTicker(time.Second)
	defer expiry.Stop()
	var startup <-chan time.Time
	if options.Startup != nil && !options.Startup.Done() {
		ticker := time.NewTicker(StartupRetryInterval)
		defer ticker.Stop()
		startup = ticker.C
	}
	var housekeeping <-chan time.Time
	if options.Housekeeping != nil {
		ticker := time.NewTicker(HousekeepingInterval)
		defer ticker.Stop()
		housekeeping = ticker.C
	}
	runMaintenanceLoop(ctx, options, samples.C, expiry.C, startup, housekeeping)
}

func runMaintenance(ctx context.Context, sampler *Sampler, operations *OperationManager, samples, expiry <-chan time.Time, report func(error)) {
	runMaintenanceWith(ctx, MaintenanceOptions{Sampler: sampler, Operations: operations, Report: report}, samples, expiry, nil)
}

func runMaintenanceWith(ctx context.Context, options MaintenanceOptions, samples, expiry, startup <-chan time.Time) {
	runMaintenanceLoop(ctx, options, samples, expiry, startup, nil)
}

func runMaintenanceLoop(ctx context.Context, options MaintenanceOptions, samples, expiry, startup, housekeeping <-chan time.Time) {
	report := options.Report
	if report == nil {
		report = func(error) {}
	}
	report(options.Sampler.Sample(ctx))
	for {
		select {
		case <-ctx.Done():
			return
		case <-samples:
			report(options.Sampler.Sample(ctx))
			if options.Reconcile != nil {
				report(options.Reconcile(ctx))
			}
		case <-housekeeping:
			if options.Housekeeping != nil {
				report(options.Housekeeping(ctx))
			}
		case <-expiry:
			options.Operations.ExpireDue(ctx)
			report(nil)
		case <-startup:
			if options.Startup != nil && !options.Startup.Done() {
				attempt, cancel := context.WithTimeout(ctx, 20*time.Second)
				options.Startup.Attempt(attempt)
				cancel()
			}
		}
	}
}
