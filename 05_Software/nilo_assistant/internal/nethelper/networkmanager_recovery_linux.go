//go:build linux

package nethelper

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/godbus/dbus/v5"
	"nilo.local/nilo/internal/network"
)

const (
	recoveryConnectionPrefix = "nilo-recovery-"
	addConnectionInMemory    = 0x2
)

func recoveryConnectionSettings(connectionUUID string, device Device) (map[string]map[string]dbus.Variant, error) {
	mac, err := hardwareAddressBytes(device.Interface.AdapterID)
	if err != nil {
		return nil, err
	}
	return map[string]map[string]dbus.Variant{
		"connection": {
			"id":             variant(recoveryConnectionPrefix + strings.ReplaceAll(strings.ToLower(device.Interface.AdapterID), ":", "")),
			"uuid":           variant(connectionUUID),
			"type":           variant("802-3-ethernet"),
			"interface-name": variant(device.Interface.Name),
			"autoconnect":    variant(false),
		},
		"802-3-ethernet": {"mac-address": variant(mac)},
		"ipv4": {
			"method":             variant("link-local"),
			"never-default":      variant(true),
			"ignore-auto-routes": variant(true),
			"ignore-auto-dns":    variant(true),
			"may-fail":           variant(false),
		},
		"ipv6": {"method": variant("disabled")},
	}, nil
}

func (n *NetworkManager) ActivateRecoveryLink(
	ctx context.Context, recordID, adapterID, interfaceName string,
) error {
	devices, err := n.Devices(ctx)
	if err != nil {
		return err
	}
	var device *Device
	for index := range devices {
		if strings.EqualFold(devices[index].Interface.AdapterID, adapterID) || devices[index].Interface.Name == interfaceName {
			device = &devices[index]
			break
		}
	}
	if device == nil {
		return errors.New("recovery adapter is no longer present")
	}
	if err := validateRecoveryDevice(*device, adapterID, interfaceName); err != nil {
		return err
	}
	settings, err := recoveryConnectionSettings(recordID, *device)
	if err != nil {
		return err
	}
	var path dbus.ObjectPath
	var result map[string]dbus.Variant
	if err := n.bus.Object(nm, settingsPath).CallWithContext(
		ctx, nm+".Settings.AddConnection2", 0, settings, uint32(addConnectionInMemory), map[string]dbus.Variant{},
	).Store(&path, &result); err != nil {
		return sanitizedBusError("create volatile recovery profile", err)
	}
	if err := n.activateOn(ctx, path, *device, recordID); err != nil {
		_ = n.bus.Object(nm, path).Call(connectionAPI+".Delete", 0).Err
		return err
	}
	return nil
}

func (n *NetworkManager) RestoreRecoveryLink(ctx context.Context, record DirectRecoveryRecord) error {
	var path dbus.ObjectPath
	if err := n.bus.Object(nm, settingsPath).CallWithContext(ctx, nm+".Settings.GetConnectionByUuid", 0, record.ID).Store(&path); err != nil {
		if recoveryProfileMissing(err) {
			return nil
		}
		return sanitizedBusError("find recovery profile", err)
	}
	settings, err := n.settingsOf(ctx, path)
	if err != nil {
		return err
	}
	properties, err := n.properties(ctx, path, connectionAPI)
	if err != nil {
		return err
	}
	if !recoveryProfileMatches(settings, record, value[bool](properties, "Unsaved")) {
		return fmt.Errorf("%w: recovery record is not owned by NILO", network.ErrExternalProfile)
	}
	if err := n.bus.Object(nm, path).CallWithContext(ctx, connectionAPI+".Delete", 0).Err; err != nil {
		return sanitizedBusError("remove volatile recovery profile", err)
	}
	return nil
}

func recoveryProfileMatches(
	settings map[string]map[string]dbus.Variant, record DirectRecoveryRecord, unsaved bool,
) bool {
	if !unsaved {
		return false
	}
	expected, err := recoveryConnectionSettings(record.ID, Device{Interface: network.Interface{
		Name: record.InterfaceName, AdapterID: record.AdapterID,
	}})
	if err != nil {
		return false
	}
	return normalizedRecoveryProfileEqual(settings, expected)
}

func normalizedRecoveryProfileEqual(
	observed, expected map[string]map[string]dbus.Variant,
) bool {
	for sectionName, expectedSection := range expected {
		observedSection, present := observed[sectionName]
		if !present {
			return false
		}
		for key, expectedValue := range expectedSection {
			observedValue, present := observedSection[key]
			if !present || !reflect.DeepEqual(observedValue.Value(), expectedValue.Value()) {
				return false
			}
		}
	}
	for sectionName, observedSection := range observed {
		expectedSection, expectedSectionPresent := expected[sectionName]
		for key, observedValue := range observedSection {
			if expectedValue, present := expectedSection[key]; present {
				if !reflect.DeepEqual(observedValue.Value(), expectedValue.Value()) {
					return false
				}
				continue
			}
			if expectedSectionPresent && recoveryRuntimeDefault(sectionName, key, observedValue) {
				continue
			}
			return false
		}
		if !expectedSectionPresent && sectionName != "proxy" {
			return false
		}
	}
	return true
}

func recoveryRuntimeDefault(section, key string, candidate dbus.Variant) bool {
	switch section {
	case "connection":
		if key == "timestamp" {
			_, valid := candidate.Value().(uint64)
			return valid
		}
		return key == "permissions" && variantEmptyCollection(candidate)
	case "802-3-ethernet":
		if key == "auto-negotiate" {
			value, valid := candidate.Value().(bool)
			return valid && !value
		}
		return key == "s390-options" && variantEmptyCollection(candidate)
	case "ipv4", "ipv6":
		switch key {
		case "address-data", "addresses", "route-data", "routes":
			return variantEmptyCollection(candidate)
		}
	}
	return false
}

func variantEmptyCollection(candidate dbus.Variant) bool {
	observed := reflect.ValueOf(candidate.Value())
	return observed.IsValid() &&
		(observed.Kind() == reflect.Array || observed.Kind() == reflect.Map || observed.Kind() == reflect.Slice) &&
		observed.Len() == 0
}

func recoveryProfileMissing(err error) bool {
	var busErr dbus.Error
	return errors.As(err, &busErr) && strings.HasSuffix(busErr.Name, ".UnknownConnection")
}

var _ DirectRecoveryDriver = (*NetworkManager)(nil)
