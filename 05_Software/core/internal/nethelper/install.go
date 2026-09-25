package nethelper

import (
	"context"
	"errors"
	"time"

	"nilo.local/nilo/internal/network"
)

// InstallPlan contains no caller-selected package, executable or repository.
type InstallPlan struct {
	Revision  string `json:"revision"`
	Installed bool   `json:"installed"`
}

// InstallRecord is the last durable installation, independent of browser life.
// An interrupted record is never replayed automatically.
type InstallRecord struct {
	ID       string                 `json:"id"`
	State    network.OperationState `json:"state"`
	Progress string                 `json:"progress"`
	Failure  string                 `json:"failure,omitempty"`
}

type PackageInstaller interface {
	Plan(context.Context) (InstallPlan, error)
	Run(context.Context, func(string) error) error
}

type InstallStore interface {
	LoadInstall() (*InstallRecord, error)
	SaveInstall(InstallRecord) error
}

type installation struct {
	requests chan struct{}
	stopped  bool
	runner   PackageInstaller
	store    InstallStore
	record   *InstallRecord
}

// ConfigureInstaller is called once before accepting clients. RunInstaller must
// be joined before the helper releases its process lock.
func (e *Engine) ConfigureInstaller(runner PackageInstaller, store InstallStore) error {
	if runner == nil || store == nil || e.install != nil {
		return errors.New("invalid installation configuration")
	}
	r, err := store.LoadInstall()
	if err != nil {
		return err
	}
	if r != nil {
		if !validID.MatchString(r.ID) || (r.State != network.StateApplying && r.State != network.StateConfirmed && r.State != network.StateFailed) {
			return errors.New("invalid installation record")
		}
		if r.State == network.StateApplying {
			r.State, r.Progress, r.Failure = network.StateFailed, "interrupted", "installation_interrupted: check the package manager state before retrying"
			if err = store.SaveInstall(*r); err != nil {
				return err
			}
		}
	}
	e.install = &installation{requests: make(chan struct{}, 1), runner: runner, store: store, record: r}
	return nil
}

func (e *Engine) RunInstaller(ctx context.Context) {
	if e.install == nil {
		return
	}
	for {
		select {
		case <-ctx.Done():
			e.mu.Lock()
			e.install.stopped = true
			e.mu.Unlock()
			return
		case <-e.install.requests:
			e.runInstall(ctx)
		}
	}
}

func (e *Engine) installationPending() bool {
	return e.install != nil && e.install.record != nil && e.install.record.State == network.StateApplying
}

func (e *Engine) InstallPlan(ctx context.Context) (InstallPlan, error) {
	if e.install == nil {
		return InstallPlan{}, network.ErrUnsupported
	}
	return e.install.runner.Plan(ctx)
}

func (e *Engine) StartInstall(ctx context.Context, id, revision string) (InstallRecord, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.install == nil {
		return InstallRecord{}, network.ErrUnsupported
	}
	if !validID.MatchString(id) {
		return InstallRecord{}, errors.New("invalid installation identity")
	}
	i := e.install
	if i.record != nil && i.record.ID == id {
		return *i.record, nil
	}
	if pending(e.record) || e.installationPending() {
		return InstallRecord{}, network.ErrChangeInProgress
	}
	if i.stopped {
		return InstallRecord{}, errors.New("helper is stopping")
	}
	plan, err := i.runner.Plan(ctx)
	if err != nil {
		return InstallRecord{}, err
	}
	if revision == "" || revision != plan.Revision {
		return InstallRecord{}, network.ErrPreviewStale
	}
	r := InstallRecord{ID: id, State: network.StateApplying, Progress: "queued"}
	if err = i.store.SaveInstall(r); err != nil {
		return InstallRecord{}, errors.New("installation_state_unavailable: check helper storage")
	}
	i.record = &r
	i.requests <- struct{}{}
	return r, nil
}

func (e *Engine) runInstall(parent context.Context) {
	i := e.install
	ctx, cancel := context.WithTimeout(parent, 10*time.Minute)
	defer cancel()
	err := i.runner.Run(ctx, func(progress string) error {
		e.mu.Lock()
		defer e.mu.Unlock()
		next := *i.record
		next.Progress = progress
		if err := i.store.SaveInstall(next); err != nil {
			return errors.New("installation_state_unavailable: check helper storage")
		}
		i.record = &next
		return nil
	})
	e.mu.Lock()
	defer e.mu.Unlock()
	next := *i.record
	next.State, next.Progress = network.StateConfirmed, "complete"
	if err != nil {
		next.State, next.Progress, next.Failure = network.StateFailed, "failed", err.Error()
	}
	if i.store.SaveInstall(next) != nil {
		// Keep the reservation when a terminal result cannot be made durable.
		i.record.Progress = "storage_unavailable"
		return
	}
	i.record = &next
}

func (e *Engine) InstallStatus(id string) (InstallRecord, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.install == nil || e.install.record == nil || e.install.record.ID != id {
		return InstallRecord{}, network.ErrOperationNotFound
	}
	return *e.install.record, nil
}

func (c *Client) StartInstall(ctx context.Context, id, revision string) (network.Operation, error) {
	r, err := c.call(ctx, Request{Op: "wireguard.install.start", ID: id, Revision: revision})
	if err != nil && r.Error != "" {
		return network.Operation{State: network.StateFailed, Failure: err.Error()}, nil
	}
	return installOperation(r, err)
}
func (c *Client) PollInstall(ctx context.Context, id string) (network.Operation, error) {
	r, err := c.call(ctx, Request{Op: "wireguard.install.read", ID: id})
	if errors.Is(err, network.ErrOperationNotFound) {
		return network.Operation{State: network.StateFailed, Failure: "installation_not_found: obtain a new preview before retrying"}, nil
	}
	return installOperation(r, err)
}
func installOperation(r Response, err error) (network.Operation, error) {
	if err != nil {
		return network.Operation{}, err
	}
	if r.Install == nil {
		return network.Operation{}, errors.New("helper omitted installation state")
	}
	return network.Operation{State: r.Install.State, Progress: r.Install.Progress, Failure: r.Install.Failure}, nil
}
