package appliance

import (
	"context"
	"errors"
)

// DurableCommissioner records the one-way transition out of first-boot
// provisioning. It is the adapter between the network service, which decides
// when the appliance is safely commissioned, and the store that remembers it.
//
// Where the store can do it, recording the transition also settles the
// published credential in the same transaction, so a password reset cannot slip
// between the two and carry the published verifier into normal service.
type DurableCommissioner struct {
	writer CommissioningWriter
}

// GuardedCommissioningWriter records the appliance as commissioned only if no
// identity under the given user name still stores a verifier matching the given
// password, both decided under one write lock.
type GuardedCommissioningWriter interface {
	CommissionUnlessPasswordMatches(ctx context.Context, username string, password []byte) error
}

// NewDurableCommissioner returns nil when there is nothing to record through,
// so a service built without a store cannot silently commission in memory.
func NewDurableCommissioner(writer CommissioningWriter) *DurableCommissioner {
	if writer == nil {
		return nil
	}
	return &DurableCommissioner{writer: writer}
}

// CompleteCommissioning records the appliance as commissioned, refusing if the
// published verifier is still stored at the moment the record would be written.
func (commissioner *DurableCommissioner) CompleteCommissioning(ctx context.Context) error {
	if commissioner == nil || commissioner.writer == nil {
		return errors.New("no appliance commissioning writer is configured")
	}
	if guarded, ok := commissioner.writer.(GuardedCommissioningWriter); ok {
		return guarded.CommissionUnlessPasswordMatches(
			ctx, PublishedAdministratorUsername, []byte(PublishedAdministratorPassword))
	}
	return commissioner.writer.SaveCommissioning(ctx, CommissioningCommissioned)
}
