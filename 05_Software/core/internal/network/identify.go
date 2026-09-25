package network

import (
	"context"
	"errors"
	"sync"
	"time"
)

var ErrIdentifyUnsupported = errors.New("identify unsupported")

type IdentifyStatus string

const (
	IdentifyAccepted    IdentifyStatus = "accepted"
	IdentifyUnsupported IdentifyStatus = "unsupported"
	IdentifyFailed      IdentifyStatus = "failed"
	// IdentifyActionMaxDuration bounds a helper-controlled indicator request.
	IdentifyActionMaxDuration = 30 * time.Second
)

type IdentifyResult struct {
	Status      IdentifyStatus `json:"status"`
	Duration    time.Duration  `json:"-"`
	ActiveUntil time.Time      `json:"active_until,omitempty"`
}

type IdentifySource string

const (
	IdentifySourceInterface IdentifySource = "interface"
	IdentifySourceButton    IdentifySource = "button"

	IdentifyReannouncementNotRequested = "not_requested"
	IdentifyReannouncementComplete     = "complete"
	IdentifyReannouncementPartial      = "partial"
	IdentifyReannouncementUnavailable  = "unavailable"
)

// IdentifyActionRequest is the stable identity and absolute deadline of one
// externally visible Identify action. Executors must treat OperationID as an
// idempotency key and return the same durable result for every retry.
type IdentifyActionRequest struct {
	OperationID string         `json:"operation_id"`
	Source      IdentifySource `json:"source"`
	ActiveUntil time.Time      `json:"active_until"`
}

type IdentifyActionResult struct {
	Supported      bool           `json:"supported"`
	Status         IdentifyStatus `json:"status"`
	ActiveUntil    time.Time      `json:"active_until,omitempty"`
	Reannouncement string         `json:"reannouncement"`
}

type IdentifyActionExecutor interface {
	ExecuteIdentifyAction(context.Context, IdentifyActionRequest) (IdentifyActionResult, error)
	AcknowledgeIdentifyAction(context.Context, string) error
}

type IdentifyIndicator interface {
	SetIdentifyIndicator(context.Context, bool) error
}

type IdentifyOptions struct {
	Indicator IdentifyIndicator
	Duration  time.Duration
	After     func(time.Duration, func()) func()
	Now       func() time.Time
}

type identifyActionReceipt struct {
	request IdentifyActionRequest
	result  IdentifyActionResult
}

// IdentifyService coalesces requests into one bounded physical indication.
// Generic hosts deliberately construct it without an indicator.
type IdentifyService struct {
	mu         sync.Mutex
	actionMu   sync.Mutex
	indicator  IdentifyIndicator
	duration   time.Duration
	after      func(time.Duration, func()) func()
	now        func() time.Time
	generation uint64
	cancel     func()
	actions    map[string]identifyActionReceipt
}

func NewIdentifyService(options IdentifyOptions) (*IdentifyService, error) {
	duration := options.Duration
	if duration == 0 {
		duration = 10 * time.Second
	}
	if duration < time.Second || duration > 30*time.Second {
		return nil, errors.New("identify duration must be between one and thirty seconds")
	}
	after := options.After
	if after == nil {
		after = func(duration time.Duration, callback func()) func() {
			timer := time.AfterFunc(duration, callback)
			return func() { timer.Stop() }
		}
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}
	return &IdentifyService{
		indicator: options.Indicator, duration: duration, after: after, now: now,
		actions: make(map[string]identifyActionReceipt),
	}, nil
}

func (service *IdentifyService) Identify(ctx context.Context) (IdentifyResult, error) {
	if service.indicator == nil {
		return IdentifyResult{Status: IdentifyUnsupported}, nil
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	if err := service.indicator.SetIdentifyIndicator(ctx, true); err != nil {
		if errors.Is(err, ErrIdentifyUnsupported) {
			return IdentifyResult{Status: IdentifyUnsupported}, nil
		}
		return IdentifyResult{}, errors.New("identify indicator failed")
	}
	service.generation++
	generation := service.generation
	if service.cancel != nil {
		service.cancel()
	}
	service.cancel = service.after(service.duration, func() {
		service.mu.Lock()
		defer service.mu.Unlock()
		if service.generation != generation {
			return
		}
		_ = service.indicator.SetIdentifyIndicator(context.Background(), false)
		service.cancel = nil
	})
	return IdentifyResult{
		Status: IdentifyAccepted, Duration: service.duration,
		ActiveUntil: service.now().UTC().Add(service.duration),
	}, nil
}

// ExecuteIdentifyAction adapts the in-process indicator used by simulation to
// the operation-aware boundary. Appliance production uses the helper-owned
// durable executor instead.
func (service *IdentifyService) ExecuteIdentifyAction(
	ctx context.Context, request IdentifyActionRequest,
) (IdentifyActionResult, error) {
	if request.OperationID == "" || request.ActiveUntil.IsZero() ||
		(request.Source != IdentifySourceInterface && request.Source != IdentifySourceButton) ||
		request.ActiveUntil.After(service.now().UTC().Add(IdentifyActionMaxDuration)) {
		return IdentifyActionResult{}, errors.New("invalid identify action request")
	}
	service.actionMu.Lock()
	defer service.actionMu.Unlock()
	if stored, ok := service.actions[request.OperationID]; ok {
		if stored.request != request {
			return IdentifyActionResult{}, errors.New("identify operation identity conflicts with its request")
		}
		return stored.result, nil
	}
	result, err := service.Identify(ctx)
	actionResult := IdentifyActionResult{
		Supported: result.Status != IdentifyUnsupported, Status: result.Status,
		ActiveUntil: result.ActiveUntil, Reannouncement: IdentifyReannouncementNotRequested,
	}
	if err == nil {
		service.actions[request.OperationID] = identifyActionReceipt{request: request, result: actionResult}
	}
	return actionResult, err
}

func (service *IdentifyService) AcknowledgeIdentifyAction(_ context.Context, operationID string) error {
	service.actionMu.Lock()
	delete(service.actions, operationID)
	service.actionMu.Unlock()
	return nil
}
