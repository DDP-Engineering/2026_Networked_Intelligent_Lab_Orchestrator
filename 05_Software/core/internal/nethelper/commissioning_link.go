package nethelper

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"nilo.local/nilo/internal/network"
)

type CommissioningProfileOwnership struct {
	Version     int                                    `json:"version"`
	Profiles    []CommissioningProfileRecord           `json:"profiles"`
	AccessPoint *CommissioningAccessPointProfileRecord `json:"access_point,omitempty"`
}

type CommissioningProfileRecord struct {
	AdapterID string `json:"adapter_id"`
	Interface string `json:"interface"`
	UUID      string `json:"uuid"`
}

type CommissioningAccessPointProfileRecord struct {
	AdapterID string `json:"adapter_id"`
	Interface string `json:"interface"`
	UUID      string `json:"uuid"`
	SSID      string `json:"ssid"`
}

type CommissioningProfileStore interface {
	LoadCommissioningProfiles() (CommissioningProfileOwnership, error)
	SaveCommissioningProfiles(CommissioningProfileOwnership) error
}

type commissioningStoreConsumer interface {
	ConfigureCommissioningProfileStore(CommissioningProfileStore) error
}

type CommissioningLinkDriver interface {
	ReconcileCommissioningLinks(context.Context, []network.CommissioningLink) error
}

var ErrCommissioningLinkUnsupported = errors.New("automatic commissioning links are unavailable")
var ErrCommissioningLinkForeign = errors.New("commissioning profile ownership cannot be proved")

func validCommissioningProfileOwnership(record CommissioningProfileOwnership) bool {
	if record.Version != 1 || len(record.Profiles) > 128 {
		return false
	}
	adapters := map[string]bool{}
	uuids := map[string]bool{}
	for _, profile := range record.Profiles {
		adapterID := strings.ToLower(strings.TrimSpace(profile.AdapterID))
		if adapterID == "" || profile.AdapterID != adapterID || profile.Interface == "" || uuid.Validate(profile.UUID) != nil ||
			adapters[adapterID] || uuids[profile.UUID] {
			return false
		}
		adapters[adapterID], uuids[profile.UUID] = true, true
	}
	if profile := record.AccessPoint; profile != nil {
		adapterID := strings.ToLower(strings.TrimSpace(profile.AdapterID))
		if adapterID == "" || profile.AdapterID != adapterID || profile.Interface == "" ||
			uuid.Validate(profile.UUID) != nil || !validPermanentSerialName(profile.SSID) ||
			adapters[adapterID] || uuids[profile.UUID] {
			return false
		}
	}
	return true
}

func (e *Engine) ReconcileCommissioningLinks(ctx context.Context, links []network.CommissioningLink) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	driver, ok := e.driver.(CommissioningLinkDriver)
	if !ok {
		return ErrCommissioningLinkUnsupported
	}
	return driver.ReconcileCommissioningLinks(ctx, links)
}
