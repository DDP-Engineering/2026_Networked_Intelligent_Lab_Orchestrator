import { describe, expect, it } from 'vitest';

import { parseAdapterRoleConflict, parseManagementAccess } from './management-access';

const managementAccessBody = {
  api_version: 'nilo.http/1',
  state: 'applied',
  port: 8443,
  interfaces: [
    {
      name: 'enx1234',
      adapter_id: 'usb-ethernet-a',
      kind: 'ethernet',
      state: 'connected',
      ipv4: ['192.168.0.1/24'],
      ipv6: [],
      enabled: false,
      current: false,
      usable: true,
      role: 'device',
      role_available: false,
      role_detail: 'Assigned to device network PLC line',
      conflicting_resource_id: 'dn-plc',
      conflicting_resource_name: 'PLC line',
    },
  ],
  configured_interfaces: ['wlan0'],
  urls: [],
  tls_names: [],
  tls_missing: [],
  confirm_by: null,
  detail: '',
  outcome: { code: '', interface: '', resource_name: '' },
  current_interface: 'wlan0',
};

describe('management access contract', () => {
  it('keeps an addressless interface when the server sends empty arrays', () => {
    const parsed = parseManagementAccess({
      ...managementAccessBody,
      interfaces: [
        {
          ...managementAccessBody.interfaces[0],
          state: 'disconnected',
          ipv4: [],
          ipv6: [],
        },
      ],
    });

    expect(parsed.ok).toBe(true);
    if (parsed.ok) {
      expect(parsed.value.interfaces[0].name).toBe('enx1234');
      expect(parsed.value.interfaces[0].ipv4).toEqual([]);
      expect(parsed.value.interfaces[0].ipv6).toEqual([]);
    }
  });

  it('preserves safe adapter role metadata', () => {
    const parsed = parseManagementAccess({
      ...managementAccessBody,
      state: 'recovered',
      outcome: {
        code: 'role_conflict_recovered',
        interface: 'enx1234',
        resource_name: 'PLC line',
      },
    });

    expect(parsed.ok).toBe(true);
    if (parsed.ok) {
      expect(parsed.value.interfaces[0]).toMatchObject({
        adapterId: 'usb-ethernet-a',
        role: 'device',
        roleAvailable: false,
        roleDetail: 'Assigned to device network PLC line',
        conflictingResourceId: 'dn-plc',
        conflictingResourceName: 'PLC line',
      });
      expect(parsed.value.outcome).toEqual({
        code: 'role_conflict_recovered',
        interface: 'enx1234',
        resourceName: 'PLC line',
      });
    }
  });

  it('rejects an unknown interface role', () => {
    const parsed = parseManagementAccess({
      ...managementAccessBody,
      interfaces: [{ ...managementAccessBody.interfaces[0], role: 'uplink' }],
    });

    expect(parsed.ok).toBe(false);
  });

  it('rejects unknown management outcome codes', () => {
    expect(
      parseManagementAccess({
        ...managementAccessBody,
        outcome: { code: 'backend_sentence', interface: '', resource_name: '' },
      }).ok,
    ).toBe(false);
  });

  it('parses the exact safe role-conflict details and rejects unknown roles', () => {
    expect(
      parseAdapterRoleConflict({
        adapter_id: 'usb-ethernet-a',
        interface: 'enx1234',
        existing_role: 'device',
        requested_role: 'management',
        conflicting_resource_id: 'dn-plc',
        conflicting_resource_name: 'PLC line',
      }),
    ).toEqual({
      adapterId: 'usb-ethernet-a',
      interface: 'enx1234',
      existingRole: 'device',
      requestedRole: 'management',
      conflictingResourceId: 'dn-plc',
      conflictingResourceName: 'PLC line',
    });
    expect(
      parseAdapterRoleConflict({
        adapter_id: 'usb-ethernet-a',
        interface: 'enx1234',
        existing_role: 'uplink',
        requested_role: 'management',
        conflicting_resource_id: '',
        conflicting_resource_name: '',
      }),
    ).toBeNull();
  });
});
