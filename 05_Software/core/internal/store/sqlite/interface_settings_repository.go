package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"

	"nilo.local/nilo/internal/network"
)

// InterfaceSettingsRepository stores saved adapter configuration as public
// intent. It refuses a record that fails validation, which includes a password
// reference that looks like a passphrase, rather than storing and redacting.
type InterfaceSettingsRepository struct {
	database *DB
}

func NewInterfaceSettingsRepository(database *DB) *InterfaceSettingsRepository {
	return &InterfaceSettingsRepository{database: database}
}

func (repository *InterfaceSettingsRepository) ready() error {
	if repository == nil || repository.database == nil || repository.database.SQL() == nil {
		return errors.New("interface settings repository database is required")
	}
	return nil
}

func flag(value bool) int {
	if value {
		return 1
	}
	return 0
}

// Save writes one configuration under its adapter identity. The revision is
// the content digest, so an unchanged save keeps it and any edit changes it.
func (repository *InterfaceSettingsRepository) Save(
	ctx context.Context, settings network.InterfaceSettings, at time.Time,
) (network.InterfaceSettings, error) {
	if err := repository.ready(); err != nil {
		return network.InterfaceSettings{}, err
	}
	if err := settings.Validate(); err != nil {
		return network.InterfaceSettings{}, err
	}
	settings.AdapterID = strings.ToLower(strings.TrimSpace(settings.AdapterID))
	settings.Revision = settings.ContentRevision()
	settings.UpdatedAt = at.UTC()
	servers := make([]string, 0, len(settings.DNS))
	for _, server := range settings.DNS {
		servers = append(servers, server.String())
	}
	dns, err := json.Marshal(servers)
	if err != nil {
		return network.InterfaceSettings{}, fmt.Errorf("encode DNS servers: %w", err)
	}
	address, gateway := "", ""
	if settings.Address.IsValid() {
		address = settings.Address.String()
	}
	if settings.Gateway.IsValid() {
		gateway = settings.Gateway.String()
	}
	var ssid, security, secretRef string
	startup := 0
	if settings.WiFi != nil {
		ssid, security, secretRef = settings.WiFi.SSID, string(settings.WiFi.Security), settings.WiFi.SecretRef
		startup = flag(settings.WiFi.ConnectAtStartup)
	}
	_, err = repository.database.SQL().ExecContext(ctx, `
		INSERT INTO network_interface_settings (
			adapter_id, connection_ref, method, address, gateway, dns_json, dns_automatic,
			management_protected, never_default, wifi_ssid, wifi_security, wifi_startup,
			wifi_secret_ref, revision, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (adapter_id) DO UPDATE SET
			connection_ref = excluded.connection_ref,
			method = excluded.method,
			address = excluded.address,
			gateway = excluded.gateway,
			dns_json = excluded.dns_json,
			dns_automatic = excluded.dns_automatic,
			management_protected = excluded.management_protected,
			never_default = excluded.never_default,
			wifi_ssid = excluded.wifi_ssid,
			wifi_security = excluded.wifi_security,
			wifi_startup = excluded.wifi_startup,
			wifi_secret_ref = excluded.wifi_secret_ref,
			revision = excluded.revision,
			updated_at = excluded.updated_at
	`,
		settings.AdapterID, settings.ConnectionRef, string(settings.Method), address, gateway, string(dns),
		flag(settings.DNSAutomatic), flag(settings.ManagementProtected), flag(settings.NeverDefault),
		ssid, security, startup, secretRef, settings.Revision, settings.UpdatedAt.Format(time.RFC3339Nano),
	)
	if err != nil {
		return network.InterfaceSettings{}, fmt.Errorf("store interface settings: %w", err)
	}
	return settings, nil
}

// List returns every saved configuration, by adapter identity.
func (repository *InterfaceSettingsRepository) List(ctx context.Context) (records []network.InterfaceSettings, err error) {
	if err := repository.ready(); err != nil {
		return nil, err
	}
	rows, err := repository.database.SQL().QueryContext(ctx, `
		SELECT adapter_id, connection_ref, method, address, gateway, dns_json, dns_automatic,
		       management_protected, never_default, wifi_ssid, wifi_security, wifi_startup,
		       wifi_secret_ref, revision, updated_at
		FROM network_interface_settings ORDER BY adapter_id
	`)
	if err != nil {
		return nil, fmt.Errorf("read interface settings: %w", err)
	}
	defer func() {
		closeErr := rows.Close()
		if err == nil && closeErr != nil {
			err = fmt.Errorf("close interface settings rows: %w", closeErr)
		}
	}()
	records = make([]network.InterfaceSettings, 0)
	for rows.Next() {
		var record network.InterfaceSettings
		var method, address, gateway, dns, ssid, security, secretRef, updated string
		var dnsAutomatic, protected, neverDefault, startup int
		if err := rows.Scan(
			&record.AdapterID, &record.ConnectionRef, &method, &address, &gateway, &dns, &dnsAutomatic,
			&protected, &neverDefault, &ssid, &security, &startup, &secretRef, &record.Revision, &updated,
		); err != nil {
			return nil, fmt.Errorf("scan interface settings: %w", err)
		}
		record.Method = network.AddressMethod(method)
		if address != "" {
			if record.Address, err = netip.ParsePrefix(address); err != nil {
				return nil, fmt.Errorf("parse stored address: %w", err)
			}
		}
		if gateway != "" {
			if record.Gateway, err = netip.ParseAddr(gateway); err != nil {
				return nil, fmt.Errorf("parse stored gateway: %w", err)
			}
		}
		var servers []string
		if err := json.Unmarshal([]byte(dns), &servers); err != nil {
			return nil, fmt.Errorf("decode stored DNS servers: %w", err)
		}
		for _, server := range servers {
			parsed, err := netip.ParseAddr(server)
			if err != nil {
				return nil, fmt.Errorf("parse stored DNS server: %w", err)
			}
			record.DNS = append(record.DNS, parsed)
		}
		record.DNSAutomatic, record.ManagementProtected, record.NeverDefault = dnsAutomatic == 1, protected == 1, neverDefault == 1
		if ssid != "" {
			record.WiFi = &network.WiFiSettings{
				SSID: ssid, Security: network.WiFiSecurity(security), ConnectAtStartup: startup == 1, SecretRef: secretRef,
			}
		}
		if record.UpdatedAt, err = parseUTCTimestamp(updated); err != nil {
			return nil, fmt.Errorf("parse stored settings time: %w", err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate interface settings: %w", err)
	}
	return records, nil
}

// Delete removes one adapter's configuration and reports when there was none.
func (repository *InterfaceSettingsRepository) Delete(ctx context.Context, adapterID string) error {
	if err := repository.ready(); err != nil {
		return err
	}
	result, err := repository.database.SQL().ExecContext(ctx,
		"DELETE FROM network_interface_settings WHERE adapter_id = ?", strings.ToLower(strings.TrimSpace(adapterID)))
	if err != nil {
		return fmt.Errorf("delete interface settings: %w", err)
	}
	removed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count deleted interface settings: %w", err)
	}
	if removed == 0 {
		return fmt.Errorf("%w: %s", network.ErrInterfaceSettingsNotFound, adapterID)
	}
	return nil
}

var _ network.InterfaceSettingsStore = (*InterfaceSettingsRepository)(nil)
