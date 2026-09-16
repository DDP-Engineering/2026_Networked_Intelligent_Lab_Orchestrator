package network

import (
	"context"
	"errors"
	"sort"
	"strings"
)

type CommissioningAccessPoint struct {
	Enabled     bool   `json:"enabled"`
	RetainOwned bool   `json:"retain_owned,omitempty"`
	SerialName  string `json:"serial_name,omitempty"`
	AdapterID   string `json:"adapter_id,omitempty"`
	Interface   string `json:"interface,omitempty"`
	Address     string `json:"address,omitempty"`
	Port        uint16 `json:"port,omitempty"`
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
	current CommissioningAccessPoint
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
		service.current = CommissioningAccessPoint{}
		return nil
	}
	interfaces, err := service.options.Provider.Interfaces(ctx)
	if err != nil {
		return err
	}
	sort.Slice(interfaces, func(i, j int) bool { return interfaces[i].AdapterID < interfaces[j].AdapterID })
	if service.current.Enabled {
		for _, item := range interfaces {
			if strings.EqualFold(item.AdapterID, service.current.AdapterID) && item.Name == service.current.Interface &&
				item.Kind == KindWiFi && item.Physical {
				return service.options.Driver.ReconcileCommissioningAccessPoint(ctx, service.current)
			}
		}
	}
	for _, item := range interfaces {
		if item.Kind != KindWiFi || !item.Physical || item.AdapterID == "" || item.Settings != nil || len(item.SavedConnections) != 0 || item.SSID != "" || item.State == StateConnected {
			continue
		}
		claim := AdapterRoleClaim{AdapterID: item.AdapterID, Interface: item.Name, Role: AdapterRoleManagement}
		if service.options.Roles != nil && service.options.Roles.Check([]AdapterRoleClaim{claim}) != nil {
			continue
		}
		plan := CommissioningAccessPoint{
			Enabled: true, SerialName: service.options.SerialName, AdapterID: item.AdapterID,
			Interface: item.Name, Address: "192.168.247.1/24",
			Port: service.options.Port,
		}
		if err := service.options.Driver.ReconcileCommissioningAccessPoint(ctx, plan); err != nil {
			return err
		}
		service.current = plan
		return nil
	}
	return service.options.Driver.ReconcileCommissioningAccessPoint(ctx, CommissioningAccessPoint{
		RetainOwned: true, SerialName: service.options.SerialName, Address: "192.168.247.1/24", Port: service.options.Port,
	})
}
