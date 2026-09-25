package policy

import (
	"context"
	"time"

	"nilo.local/nilo/internal/domain"
)

// Repository persists immutable policy decisions and request-bound
// administrator decisions.
type Repository interface {
	SaveSnapshot(ctx context.Context, snapshot Snapshot) error
	RecordApproval(ctx context.Context, record ApprovalRecord) error
	FindValidApproval(ctx context.Context, requestID domain.RequestID, digest [32]byte, now time.Time) (ApprovalRecord, error)
}
