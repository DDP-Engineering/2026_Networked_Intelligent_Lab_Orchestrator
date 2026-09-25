package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"time"

	"nilo.local/nilo/internal/network"
)

// DeviceNetworkRepository stores saved device networks. It refuses a record
// that fails the domain's own validation rather than storing a decision the
// planner could not act on.
type DeviceNetworkRepository struct {
	database *DB
}

func NewDeviceNetworkRepository(database *DB) *DeviceNetworkRepository {
	return &DeviceNetworkRepository{database: database}
}

func (repository *DeviceNetworkRepository) ready() error {
	if repository == nil || repository.database == nil || repository.database.SQL() == nil {
		return errors.New("device network repository database is required")
	}
	return nil
}

func prefixStrings(prefixes []netip.Prefix) []string {
	texts := make([]string, 0, len(prefixes))
	for _, prefix := range prefixes {
		texts = append(texts, prefix.String())
	}
	return texts
}

func parsePrefixes(texts []string) ([]netip.Prefix, error) {
	prefixes := make([]netip.Prefix, 0, len(texts))
	for _, text := range texts {
		prefix, err := netip.ParsePrefix(text)
		if err != nil {
			return nil, err
		}
		prefixes = append(prefixes, prefix)
	}
	return prefixes, nil
}

type storedServices struct {
	Protocols      []string `json:"protocols"`
	TCPPorts       []int    `json:"tcp_ports"`
	UDPPorts       []int    `json:"udp_ports"`
	PreserveSource bool     `json:"preserve_source"`
}

// Save writes one device network. The revision is the content digest, so an
// unchanged save keeps it and any edit changes it; creation time is kept.
func (repository *DeviceNetworkRepository) Save(ctx context.Context, record network.DeviceNetwork, at time.Time) (network.DeviceNetwork, error) {
	if err := repository.ready(); err != nil {
		return network.DeviceNetwork{}, err
	}
	if record.ID == "" {
		return network.DeviceNetwork{}, errors.New("a saved device network requires its identity")
	}
	if err := record.Validate(); err != nil {
		return network.DeviceNetwork{}, err
	}
	record.Revision = record.ContentRevision()
	record.UpdatedAt = at.UTC()
	if record.CreatedAt.IsZero() {
		record.CreatedAt = at.UTC()
	}
	localSources, _ := json.Marshal(prefixStrings(record.LocalSources))
	vpnSources, _ := json.Marshal(prefixStrings(record.VPNSources))
	services, _ := json.Marshal(storedServices{
		Protocols: record.Services.Protocols, TCPPorts: record.Services.TCPPorts,
		UDPPorts: record.Services.UDPPorts, PreserveSource: record.Services.PreserveSource,
	})
	target, exposed := "", ""
	if record.Target.IsValid() {
		target = record.Target.String()
	}
	if record.Exposed.IsValid() {
		exposed = record.Exposed.String()
	}
	_, err := repository.database.SQL().ExecContext(ctx, `
		INSERT INTO device_networks (
			id, name, location, device_interface, appliance_address, scope, target, addressing, exposed,
			access, local_interface, local_sources_json, vpn_profile_id, vpn_sources_json, services_json,
			enabled, revision, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET
			name = excluded.name, location = excluded.location, device_interface = excluded.device_interface,
			appliance_address = excluded.appliance_address, scope = excluded.scope, target = excluded.target,
			addressing = excluded.addressing, exposed = excluded.exposed, access = excluded.access,
			local_interface = excluded.local_interface, local_sources_json = excluded.local_sources_json,
			vpn_profile_id = excluded.vpn_profile_id, vpn_sources_json = excluded.vpn_sources_json,
			services_json = excluded.services_json, enabled = excluded.enabled, revision = excluded.revision,
			updated_at = excluded.updated_at
	`,
		record.ID, record.Name, record.Location, record.DeviceInterface, record.ApplianceAddress.String(),
		string(record.Scope), target, string(record.Addressing), exposed, string(record.Access),
		record.LocalInterface, string(localSources), record.VPNProfileID, string(vpnSources), string(services),
		flag(record.StartEnabled), record.Revision, record.CreatedAt.Format(time.RFC3339Nano), record.UpdatedAt.Format(time.RFC3339Nano),
	)
	if err != nil {
		return network.DeviceNetwork{}, fmt.Errorf("store device network: %w", err)
	}
	// The creation time of an existing row is what is stored, not what the
	// caller passed.
	var created string
	if err := repository.database.SQL().QueryRowContext(ctx, "SELECT created_at FROM device_networks WHERE id = ?", record.ID).Scan(&created); err == nil {
		if stamp, err := parseUTCTimestamp(created); err == nil {
			record.CreatedAt = stamp
		}
	}
	return record, nil
}

// List returns every saved device network, by name.
func (repository *DeviceNetworkRepository) List(ctx context.Context) (records []network.DeviceNetwork, err error) {
	if err := repository.ready(); err != nil {
		return nil, err
	}
	rows, err := repository.database.SQL().QueryContext(ctx, `
		SELECT id, name, location, device_interface, appliance_address, scope, target, addressing, exposed,
		       access, local_interface, local_sources_json, vpn_profile_id, vpn_sources_json, services_json,
		       enabled, revision, created_at, updated_at
		FROM device_networks ORDER BY name
	`)
	if err != nil {
		return nil, fmt.Errorf("read device networks: %w", err)
	}
	defer func() {
		closeErr := rows.Close()
		if err == nil && closeErr != nil {
			err = fmt.Errorf("close device network rows: %w", closeErr)
		}
	}()
	records = make([]network.DeviceNetwork, 0)
	for rows.Next() {
		var record network.DeviceNetwork
		var appliance, scope, target, addressing, exposed, access, localSources, vpnSources, services, created, updated string
		var enabled int
		if err := rows.Scan(
			&record.ID, &record.Name, &record.Location, &record.DeviceInterface, &appliance, &scope, &target,
			&addressing, &exposed, &access, &record.LocalInterface, &localSources, &record.VPNProfileID,
			&vpnSources, &services, &enabled, &record.Revision, &created, &updated,
		); err != nil {
			return nil, fmt.Errorf("scan device network: %w", err)
		}
		if record.ApplianceAddress, err = netip.ParsePrefix(appliance); err != nil {
			return nil, fmt.Errorf("parse stored appliance address: %w", err)
		}
		record.Scope, record.Addressing, record.Access = network.Scope(scope), network.Addressing(addressing), network.AccessMethod(access)
		if target != "" {
			if record.Target, err = netip.ParseAddr(target); err != nil {
				return nil, fmt.Errorf("parse stored target: %w", err)
			}
		}
		if exposed != "" {
			if record.Exposed, err = netip.ParsePrefix(exposed); err != nil {
				return nil, fmt.Errorf("parse stored exposed prefix: %w", err)
			}
		}
		var localTexts, vpnTexts []string
		if err := json.Unmarshal([]byte(localSources), &localTexts); err != nil {
			return nil, fmt.Errorf("decode stored local sources: %w", err)
		}
		if err := json.Unmarshal([]byte(vpnSources), &vpnTexts); err != nil {
			return nil, fmt.Errorf("decode stored VPN sources: %w", err)
		}
		if record.LocalSources, err = parsePrefixes(localTexts); err != nil {
			return nil, fmt.Errorf("parse stored local sources: %w", err)
		}
		if record.VPNSources, err = parsePrefixes(vpnTexts); err != nil {
			return nil, fmt.Errorf("parse stored VPN sources: %w", err)
		}
		var stored storedServices
		if err := json.Unmarshal([]byte(services), &stored); err != nil {
			return nil, fmt.Errorf("decode stored service policy: %w", err)
		}
		record.Services = network.ServicePolicy{Protocols: stored.Protocols, TCPPorts: stored.TCPPorts, UDPPorts: stored.UDPPorts, PreserveSource: stored.PreserveSource}
		record.StartEnabled = enabled == 1
		if record.CreatedAt, err = parseUTCTimestamp(created); err != nil {
			return nil, fmt.Errorf("parse stored creation time: %w", err)
		}
		if record.UpdatedAt, err = parseUTCTimestamp(updated); err != nil {
			return nil, fmt.Errorf("parse stored update time: %w", err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate device networks: %w", err)
	}
	return records, nil
}

// Delete removes one device network and reports when there was none.
func (repository *DeviceNetworkRepository) Delete(ctx context.Context, id string) error {
	if err := repository.ready(); err != nil {
		return err
	}
	result, err := repository.database.SQL().ExecContext(ctx, "DELETE FROM device_networks WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete device network: %w", err)
	}
	removed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count deleted device networks: %w", err)
	}
	if removed == 0 {
		return fmt.Errorf("%w: %s", network.ErrDeviceNetworkNotFound, id)
	}
	return nil
}

var _ network.DeviceNetworkStore = (*DeviceNetworkRepository)(nil)
