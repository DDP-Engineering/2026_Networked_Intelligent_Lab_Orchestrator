package domain

import (
	"errors"
	"fmt"
)

type SessionState string

const (
	PendingApproval SessionState = "pending_approval"
	Queued          SessionState = "queued"
	Running         SessionState = "running"
	Completed       SessionState = "completed"
	Failed          SessionState = "failed"
	Cancelled       SessionState = "cancelled"
	Incomplete      SessionState = "incomplete"
)

var ErrInvalidTransition = errors.New("invalid session transition")

func ValidateTransition(from, to SessionState) error {
	allowed := (from == PendingApproval && to == Queued) ||
		(from == Queued && to == Running) ||
		(from == Running && (to == Completed || to == Failed || to == Cancelled || to == Incomplete))
	if !allowed {
		return fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, from, to)
	}
	return nil
}
