package network

import (
	"context"
	"errors"
	"sort"
	"strings"
)

type CommissioningAccessPoint struct {
	Enabled            bool     `json:"enabled"`
	RetainOwned        bool     `json:"retain_owned,omitempty"`
	EligibleAdapterIDs []string `json:"eligible_adapter_ids,omitempty"`
	SerialName         string   `json:"serial_name,omitempty"`
	AdapterID          string   `json:"adapter_id,omitempty"`
	Interface          string   `json:"interface,omitempty"`
	Address            string   `json:"address,omitempty"`
	Port               uint16   `json:"port,omitempty"`
}

type CommissioningAccessPointDriver interface {
	ReconcileCommissioningAccessPoint(context.Context, CommissioningAccessPoint) error
}

type CommissioningAccessPointOptions struct {
	Provider   Provider
	Driver     CommissioningAccessPointDriver
	Roles      *AdapterRoleCoordinator
	SerialName string
	Port       uint16
}

type CommissioningAccessPointService struct {
	options CommissioningAccessPointOptions
}

func NewCommissioningAccessPointService(options CommissioningAccessPointOptions) (*CommissioningAccessPointService, error) {
	if options.Provider == nil || options.Driver == nil || !validPermanentMDNSSerial(options.SerialName) || options.Port == 0 {
		return nil, errors.New("commissioning access point requires provider, driver and permanent name")
	}
	return &CommissioningAccessPointService{options: options}, nil
}

func (service *CommissioningAccessPointService) Reconcile(ctx context.Context, provisioning bool) error {
	if !provisioning {
		if err := service.options.Driver.ReconcileCommissioningAccessPoint(ctx, CommissioningAccessPoint{}); err != nil {
			return err
		}
		return nil
	}
	interfaces, err := service.options.Provider.Interfaces(ctx)
	if err != nil {
		return err
	}
	sort.Slice(interfaces, func(i, j int) bool { return interfaces[i].AdapterID < interfaces[j].AdapterID })
	eligible := make([]string, 0, len(interfaces))
	var fallback CommissioningAccessPoint
	for _, item := range interfaces {
		if service.ownedRadioEligible(item) {
			eligible = append(eligible, strings.ToLower(item.AdapterID))
		}
		if fallback.Enabled || !service.freshRadioEligible(item) {
			continue
		}
		fallback = CommissioningAccessPoint{
			Enabled: true, SerialName: service.options.SerialName, AdapterID: item.AdapterID,
			Interface: item.Name, Address: "192.168.247.1/24",
			Port: service.options.Port,
		}
	}
	if len(eligible) == 0 {
		return service.options.Driver.ReconcileCommissioningAccessPoint(ctx, CommissioningAccessPoint{})
	}
	fallback.RetainOwned = true
	fallback.EligibleAdapterIDs = eligible
	fallback.SerialName = service.options.SerialName
	fallback.Address = "192.168.247.1/24"
	fallback.Port = service.options.Port
	return service.options.Driver.ReconcileCommissioningAccessPoint(ctx, fallback)
}

func (service *CommissioningAccessPointService) freshRadioEligible(item Interface) bool {
	return service.commonRadioEligible(item) && len(item.SavedConnections) == 0 && item.SSID == "" && item.State != StateConnected
}

func (service *CommissioningAccessPointService) ownedRadioEligible(item Interface) bool {
	if !service.commonRadioEligible(item) || len(item.SavedConnections) > 1 {
		return false
	}
	if item.SSID != "" && item.SSID != service.options.SerialName {
		return false
	}
	return item.State != StateConnected || item.SSID == service.options.SerialName
}

func (service *CommissioningAccessPointService) commonRadioEligible(item Interface) bool {
	if item.Kind != KindWiFi || !item.Physical || item.AdapterID == "" || item.Name == "" || item.Settings != nil {
		return false
	}
	claim := AdapterRoleClaim{AdapterID: item.AdapterID, Interface: item.Name, Role: AdapterRoleManagement}
	return service.options.Roles == nil || service.options.Roles.Check([]AdapterRoleClaim{claim}) == nil
}
