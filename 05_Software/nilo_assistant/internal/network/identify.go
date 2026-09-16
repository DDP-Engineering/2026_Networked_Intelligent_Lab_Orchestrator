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
)

type IdentifyResult struct {
	Status      IdentifyStatus `json:"status"`
	Duration    time.Duration  `json:"-"`
	ActiveUntil time.Time      `json:"active_until,omitempty"`
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

// IdentifyService coalesces requests into one bounded physical indication.
// Generic hosts deliberately construct it without an indicator.
type IdentifyService struct {
	mu         sync.Mutex
	indicator  IdentifyIndicator
	duration   time.Duration
	after      func(time.Duration, func()) func()
	now        func() time.Time
	generation uint64
	cancel     func()
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
	return &IdentifyService{indicator: options.Indicator, duration: duration, after: after, now: now}, nil
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
