package appliance

import (
	"context"
	"errors"
)

// DurableCommissioner records the one-way transition out of first-boot
// provisioning. It is the adapter between the network service, which decides
// when the appliance is safely commissioned, and the store that remembers it.
type DurableCommissioner struct {
	writer CommissioningWriter
}

// NewDurableCommissioner returns nil when there is nothing to record through,
// so a service built without a store cannot silently commission in memory.
func NewDurableCommissioner(writer CommissioningWriter) *DurableCommissioner {
	if writer == nil {
		return nil
	}
	return &DurableCommissioner{writer: writer}
}

// CompleteCommissioning records the appliance as commissioned.
func (commissioner *DurableCommissioner) CompleteCommissioning(ctx context.Context) error {
	if commissioner == nil || commissioner.writer == nil {
		return errors.New("no appliance commissioning writer is configured")
	}
	return commissioner.writer.SaveCommissioning(ctx, CommissioningCommissioned)
}
