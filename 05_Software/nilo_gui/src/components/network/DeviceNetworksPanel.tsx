import { Fragment, useState, type ReactNode } from 'react';

import type { NetworkInterface, NetworkOperation } from '../../api/contract';
import type {
  AccessInstructions,
  DeviceNetwork,
  DeviceNetworkChange,
  DeviceNetworkCheck,
  DeviceNetworkDraft,
  DeviceNetworkState,
} from '../../api/devicenetworks';
import type { WireGuardProfile } from '../../api/wireguard';
import type { ManagementAccessInterface } from '../../api/management-access';
import { useTranslate } from '../../i18n/languages';
import type { MessageKey } from '../../i18n/en';
import { Modal } from '../Modal';
import { DetailsToggle, RowActions, type RowAction } from '../RowActions';
import { Field } from '../help/Field';
import { HelpPopover } from '../help/HelpPopover';
import { WorkflowHelp } from '../help/WorkflowHelp';
import { isAddress, isPrefix, subnetOf } from './addressing';

export interface DeviceNetworksPanelProps {
  networks: DeviceNetwork[] | null;
  /** Adapters the editor can offer as device-side or local-access adapters. */
  interfaces: NetworkInterface[];
  /** NILO-owned WireGuard profiles the editor can offer. */
  profiles: WireGuardProfile[];
  /** Role claims from the management selector, keyed by stable adapter identity. */
  managementInterfaces?: ManagementAccessInterface[];
  /** Distinguishes a known-empty role inventory from one that is unavailable. */
  managementRoleState?: 'reading' | 'read' | 'failed';
  canManage: boolean;
  busy: boolean;
  unreadable: boolean;
  propose: (change: DeviceNetworkChange) => Promise<NetworkOperation | null>;
  check: (id: string, tcpPort: number) => Promise<DeviceNetworkCheck>;
  instructions: (id: string) => Promise<AccessInstructions>;
  /** Opens the shared Activity page narrowed to this device network. */
  openActivity?: (networkId: string) => void;
}

const STATE_KEYS: Record<DeviceNetworkState, MessageKey> = {
  disabled: 'network.dn.state.disabled',
  active: 'network.dn.state.active',
  waiting: 'network.dn.state.waiting',
  conflict: 'network.dn.state.conflict',
  recovery_failed: 'network.dn.state.recovery_failed',
  unsupported: 'network.dn.state.unsupported',
};

const ACCESS_KEYS: Record<DeviceNetwork['access'], MessageKey> = {
  local: 'network.dn.access.local',
  wireguard: 'network.dn.access.wireguard',
  both: 'network.dn.access.both',
};

const FACT_KEYS: Record<string, MessageKey> = {
  passed: 'network.dn.fact.passed',
  failed: 'network.dn.fact.failed',
  unknown: 'network.dn.fact.unknown',
};

/** The editor's working copy, in the shape the appliance accepts. */
interface DraftForm {
  name: string;
  location: string;
  deviceInterface: string;
  applianceAddress: string;
  scope: 'host' | 'subnet';
  target: string;
  addressing: 'original' | 'translated';
  exposed: string;
  access: 'local' | 'wireguard' | 'both';
  localInterface: string;
  localSources: string;
  vpnProfileId: string;
  vpnSources: string;
  tcpPorts: string;
  udpPorts: string;
  protocols: string[];
  preserveSource: boolean;
}

type FieldErrors = Partial<Record<keyof DraftForm, string>>;

function emptyForm(
  interfaces: NetworkInterface[],
  blockedDeviceAdapters: Set<string> = new Set(),
): DraftForm {
  const adapters = interfaces.filter((entry) => entry.adapterId !== '');
  const available = adapters.filter(
    (entry) => !blockedDeviceAdapters.has(entry.adapterId.toLowerCase()),
  );
  const device = available[1] ?? available[0];
  const local = adapters.find((entry) => entry.adapterId !== device?.adapterId) ?? adapters[0];
  return {
    name: '',
    location: '',
    deviceInterface: device?.adapterId ?? '',
    applianceAddress: '192.168.0.1/24',
    scope: 'host',
    target: '',
    addressing: 'original',
    exposed: '',
    access: 'local',
    localInterface: local?.adapterId ?? '',
    localSources: '',
    vpnProfileId: '',
    vpnSources: '',
    tcpPorts: '',
    udpPorts: '',
    protocols: ['tcp', 'udp', 'icmp'],
    preserveSource: false,
  };
}

function formOf(network: DeviceNetwork): DraftForm {
  return {
    name: network.name,
    location: network.location,
    deviceInterface: network.deviceInterface,
    applianceAddress: network.applianceAddress,
    scope: network.scope,
    target: network.target,
    addressing: network.addressing,
    exposed: network.addressing === 'translated' ? network.exposed : '',
    access: network.access,
    localInterface: network.localInterface,
    localSources: network.localSources.join(', '),
    vpnProfileId: network.vpnProfileId,
    vpnSources: network.vpnSources.join(', '),
    tcpPorts: network.services.tcpPorts.join(', '),
    udpPorts: network.services.udpPorts.join(', '),
    protocols:
      network.services.protocols.length > 0
        ? network.services.protocols
        : ['tcp', 'udp', 'icmp'],
    preserveSource: network.services.preserveSource,
  };
}

function list(text: string): string[] {
  return text
    .split(',')
    .map((entry) => entry.trim())
    .filter((entry) => entry !== '');
}

function ports(text: string): number[] {
  return list(text)
    .map((entry) => Number(entry))
    .filter((entry) => Number.isInteger(entry) && entry > 0 && entry < 65536);
}

function isPortList(text: string): boolean {
  return list(text).every(
    (entry) => /^\d+$/.test(entry) && Number(entry) >= 1 && Number(entry) <= 65535,
  );
}

function isSourceList(text: string): boolean {
  return list(text).every((entry) => isAddress(entry) || isPrefix(entry));
}

/** Turn the form into what the appliance validates; it decides, not this page. */
function draftOf(form: DraftForm): DeviceNetworkDraft {
  return {
    name: form.name.trim(),
    location: form.location.trim(),
    device_interface: form.deviceInterface,
    appliance_address: form.applianceAddress.trim(),
    scope: form.scope,
    target: form.scope === 'host' ? form.target.trim() : '',
    addressing: form.addressing,
    exposed: form.addressing === 'translated' ? form.exposed.trim() : '',
    access: form.access,
    local_interface: form.access === 'wireguard' ? '' : form.localInterface,
    local_sources: form.access === 'wireguard' ? [] : list(form.localSources),
    vpn_profile_id: form.access === 'local' ? '' : form.vpnProfileId,
    vpn_sources: form.access === 'local' ? [] : list(form.vpnSources),
    services: {
      protocols: form.protocols,
      tcp_ports: ports(form.tcpPorts),
      udp_ports: ports(form.udpPorts),
      preserve_source: form.preserveSource,
    },
  };
}

/**
 * The device networks: what the operator decided, beside what the appliance
 * observes. Intent and observation stay two columns, because a network that is
 * enabled and waiting is neither "on" nor "off".
 *
 * One markup per network: the essential columns in the row, every other value
 * behind Details, and every permitted action in one compact Actions dialog. The editor
 * is grouped by the order an operator works in, keeps its advanced settings
 * closed, validates beside the field, and ends with a plain-language review.
 */
export function DeviceNetworksPanel({
  networks,
  interfaces,
  profiles,
  managementInterfaces = [],
  managementRoleState = 'read',
  canManage,
  busy,
  unreadable,
  propose,
  check,
  instructions,
  openActivity,
}: DeviceNetworksPanelProps) {
  const t = useTranslate();
  const [editing, setEditing] = useState<{ id: string | null; form: DraftForm } | null>(
    null,
  );
  const [errors, setErrors] = useState<FieldErrors>({});
  const [refusal, setRefusal] = useState('');
  const [expanded, setExpanded] = useState<string | null>(null);
  const [checking, setChecking] = useState<{
    network: DeviceNetwork;
    port: string;
    result: DeviceNetworkCheck | null;
  } | null>(null);
  const [showing, setShowing] = useState<{
    network: DeviceNetwork;
    set: AccessInstructions | null;
  } | null>(null);
  const adapters = interfaces.filter(
    (entry) => entry.adapterId !== '' && entry.controllable,
  );
  const managementClaims = new Map(
    managementInterfaces
      .filter((entry) => entry.adapterId !== '' && entry.role === 'management')
      .map((entry) => [entry.adapterId.toLowerCase(), entry]),
  );
  const managementRolesReady = managementRoleState === 'read';
  const selectedDeviceRoleConflict =
    editing !== null &&
    managementClaims.has(editing.form.deviceInterface.toLowerCase());
  const owned = profiles.filter((profile) => profile.niloOwned);
  const adapterName = (adapterId: string) =>
    interfaces.find((entry) => entry.adapterId === adapterId)?.name ??
    (adapterId === '' ? t('network.dn.unnamed') : adapterId);

  const validate = (form: DraftForm): FieldErrors => {
    const found: FieldErrors = {};
    if (form.name.trim() === '') found.name = t('network.dn.invalid.name');
    if (form.deviceInterface === '')
      found.deviceInterface = t('network.dn.invalid.adapter');
    if (!isPrefix(form.applianceAddress.trim())) {
      found.applianceAddress = t('network.dn.invalid.cidr');
    }
    if (form.scope === 'host' && !isAddress(form.target.trim())) {
      found.target = t('network.dn.invalid.address');
    }
    if (
      form.addressing === 'translated' &&
      !(isAddress(form.exposed.trim()) || isPrefix(form.exposed.trim()))
    ) {
      found.exposed = t('network.dn.invalid.exposed');
    }
    if (form.access !== 'wireguard' && !isSourceList(form.localSources)) {
      found.localSources = t('network.dn.invalid.sources');
    }
    if (form.access !== 'local') {
      if (form.vpnProfileId === '')
        found.vpnProfileId = t('network.dn.invalid.profile');
      if (!isSourceList(form.vpnSources))
        found.vpnSources = t('network.dn.invalid.sources');
    }
    if (!isPortList(form.tcpPorts)) found.tcpPorts = t('network.dn.invalid.ports');
    if (!isPortList(form.udpPorts)) found.udpPorts = t('network.dn.invalid.ports');
    return found;
  };

  const submit = async () => {
    if (editing === null) return;
    if (!managementRolesReady) {
      setRefusal(
        t(
          managementRoleState === 'failed'
            ? 'network.role.managementInventoryFailed'
            : 'network.role.managementInventoryReading',
        ),
      );
      return;
    }
    if (selectedDeviceRoleConflict) {
      const claim = managementClaims.get(editing.form.deviceInterface.toLowerCase());
      setRefusal(
        t('network.role.managementUnavailable', {
          name: claim?.name ?? adapterName(editing.form.deviceInterface),
        }),
      );
      return;
    }
    const found = validate(editing.form);
    setErrors(found);
    if (Object.keys(found).length > 0) {
      setRefusal(t('network.dn.invalid'));
      return;
    }
    const draft = draftOf(editing.form);
    const change: DeviceNetworkChange =
      editing.id === null
        ? { action: 'create', device_network: draft }
        : { action: 'update', target: editing.id, device_network: draft };
    const operation = await propose(change);
    if (operation !== null) {
      setEditing(null);
      setRefusal('');
    }
  };

  const runCheck = async () => {
    if (checking === null) return;
    try {
      const result = await check(checking.network.id, Number(checking.port) || 0);
      setChecking({ ...checking, result });
    } catch {
      setChecking({
        ...checking,
        result: {
          deviceNetworkId: checking.network.id,
          observedAt: new Date(),
          facts: [],
        },
      });
    }
  };

  const showInstructions = async (network: DeviceNetwork) => {
    setShowing({ network, set: null });
    try {
      setShowing({ network, set: await instructions(network.id) });
    } catch {
      setShowing({
        network,
        set: {
          deviceNetworkId: network.id,
          targetAddress: '',
          steps: [],
          unmetPrerequisites: [],
        },
      });
    }
  };

  const open = (id: string | null, form: DraftForm) => {
    setRefusal('');
    setErrors({});
    setEditing({ id, form });
  };

  const set = (key: keyof DraftForm, value: string | boolean | string[]) => {
    if (editing === null) return;
    setEditing({ ...editing, form: { ...editing.form, [key]: value } });
    if (errors[key] !== undefined) setErrors({ ...errors, [key]: undefined });
  };

  const textInput = (
    key: keyof DraftForm,
    label: MessageKey,
    options: { placeholder?: string; help?: MessageKey; guidance?: MessageKey } = {},
  ) =>
    editing === null ? null : (
      <Field
        id={`dn-${key}`}
        label={t(label)}
        help={options.help === undefined ? undefined : <p>{t(options.help)}</p>}
        guidance={options.guidance === undefined ? undefined : t(options.guidance)}
        error={errors[key]}
      >
        <input
          id={`dn-${key}`}
          value={String(editing.form[key])}
          placeholder={options.placeholder ?? ''}
          onChange={(event) => set(key, event.target.value)}
        />
      </Field>
    );
  const selectInput = (
    key: keyof DraftForm,
    label: MessageKey,
    choices: ReactNode,
    options: { help?: MessageKey; guidance?: MessageKey; disabled?: boolean } = {},
  ) =>
    editing === null ? null : (
      <Field
        id={`dn-${key}`}
        label={t(label)}
        help={options.help === undefined ? undefined : <p>{t(options.help)}</p>}
        guidance={options.guidance === undefined ? undefined : t(options.guidance)}
        error={errors[key]}
      >
        <select
          id={`dn-${key}`}
          value={String(editing.form[key])}
          disabled={options.disabled}
          onChange={(event) => set(key, event.target.value)}
        >
          {choices}
        </select>
      </Field>
    );
  const localAdapterChoices = (
    <>
      <option value="">—</option>
      {adapters.map((adapter) => (
        <option key={adapter.adapterId} value={adapter.adapterId}>
          {adapter.name} ({adapter.adapterId})
        </option>
      ))}
    </>
  );
  const deviceAdapterChoices = (
    <>
      <option value="">—</option>
      {adapters.map((adapter) => (
        <option
          key={adapter.adapterId}
          value={adapter.adapterId}
          disabled={managementClaims.has(adapter.adapterId.toLowerCase())}
        >
          {adapter.name} ({adapter.adapterId})
        </option>
      ))}
    </>
  );

  const review = (form: DraftForm): string[] => {
    const adapter = adapterName(form.deviceInterface);
    const address = form.applianceAddress.trim() || '—';
    const target =
      form.scope === 'host'
        ? form.target.trim() || '—'
        : subnetOf(form.applianceAddress);
    const lines = [
      t('network.dn.review.deviceSide', { adapter, address }),
      t('network.dn.review.target', { target }),
    ];
    if (form.access !== 'wireguard') {
      const sources = list(form.localSources);
      lines.push(
        sources.length === 0
          ? t('network.dn.review.localNone')
          : t('network.dn.review.local', {
              sources: sources.join(', '),
              adapter: adapterName(form.localInterface),
            }),
      );
    }
    if (form.access !== 'local') {
      const profile = owned.find((entry) => entry.id === form.vpnProfileId);
      lines.push(
        t('network.dn.review.vpn', {
          profile: profile?.name ?? '—',
          sources: list(form.vpnSources).join(', ') || t('network.dn.noneListed'),
        }),
      );
    } else {
      lines.push(t('network.dn.review.vpnNone'));
    }
    lines.push(
      form.addressing === 'translated'
        ? t('network.dn.review.translated', { exposed: form.exposed.trim() || '—' })
        : t('network.dn.review.original'),
    );
    const services = [
      form.protocols.join(', '),
      `${t('network.dn.fact.tcpPorts')}: ${list(form.tcpPorts).join(', ') || t('network.dn.allPorts')}`,
      `${t('network.dn.fact.udpPorts')}: ${list(form.udpPorts).join(', ') || t('network.dn.allPorts')}`,
    ].join('; ');
    lines.push(t('network.dn.review.services', { services }));
    lines.push(
      form.preserveSource
        ? t('network.dn.review.preserve')
        : t('network.dn.review.snat'),
    );
    lines.push(t('network.dn.review.management', { adapter }));
    return lines;
  };

  return (
    <section aria-labelledby="network-device-networks">
      <div className="network-section-head">
        <h3 id="network-device-networks">{t('network.dn.title')}</h3>
        {canManage && (
          <button
            type="button"
            disabled={busy}
            onClick={() =>
              open(
                null,
                emptyForm(
                  interfaces,
                  managementRolesReady
                    ? new Set(managementClaims.keys())
                    : new Set(adapters.map((entry) => entry.adapterId.toLowerCase())),
                ),
              )
            }
          >
            {t('network.dn.add')}
          </button>
        )}
      </div>
      {unreadable && (
        <p className="notice notice-unknown">{t('network.dn.unreadable')}</p>
      )}
      {networks !== null && networks.length === 0 && !unreadable && (
        <p>{t('network.dn.empty')}</p>
      )}
      {networks !== null && networks.length > 0 && (
        <div className="table-scroll">
          <table className="data-table record-table">
            <thead>
              <tr>
                <th scope="col">{t('auth.field.name')}</th>
                <th scope="col">{t('network.dn.col.interface')}</th>
                <th scope="col">{t('network.dn.col.subnet')}</th>
                <th scope="col">{t('network.dn.col.access')}</th>
                <th scope="col">{t('network.dn.col.status')}</th>
                <th scope="col">{t('network.table.actions')}</th>
              </tr>
            </thead>
            <tbody>
              {networks.map((network) => {
                const detailsId = `network-dn-${network.id}`;
                const isExpanded = expanded === network.id;
                const busyReason = busy ? t('network.busyReason') : undefined;
                const actions: RowAction[] = canManage
                  ? [
                      {
                        label: `${t('network.dn.edit')} ${network.name}`,
                        onSelect: () => open(network.id, formOf(network)),
                        disabled: busy,
                        reason: busyReason,
                      },
                      {
                        label: `${t(network.enabled ? 'network.dn.disable' : 'network.dn.enable')} ${network.name}`,
                        onSelect: () =>
                          void propose({
                            action: network.enabled ? 'disable' : 'enable',
                            target: network.id,
                          }),
                        disabled: busy,
                        reason: busyReason,
                      },
                      {
                        label: `${t('network.dn.delete')} ${network.name}`,
                        onSelect: () =>
                          void propose({ action: 'delete', target: network.id }),
                        disabled: busy,
                        reason: busyReason,
                        danger: true,
                      },
                      {
                        label: `${t('network.dn.check')} ${network.name}`,
                        onSelect: () =>
                          setChecking({
                            network,
                            port: String(network.services.tcpPorts[0] ?? ''),
                            result: null,
                          }),
                      },
                    ]
                  : [];
                actions.push({
                  label: `${t('network.dn.instructions')} ${network.name}`,
                  onSelect: () => void showInstructions(network),
                });
                if (openActivity !== undefined) {
                  actions.push({
                    label: `${t('network.dn.activity')} ${network.name}`,
                    onSelect: () => openActivity(network.id),
                  });
                }
                const stateDot =
                  network.state === 'active'
                    ? 'state-ok'
                    : network.state === 'conflict' ||
                        network.state === 'recovery_failed'
                      ? 'state-danger'
                      : network.state === 'waiting'
                        ? 'state-warn'
                        : 'state-unknown';
                const stateLabel = t(STATE_KEYS[network.state]);
                return (
                  <Fragment key={network.id}>
                    <tr>
                      <th scope="row">{network.name}</th>
                      <td data-label={t('network.dn.col.interface')}>
                        {adapterName(network.deviceInterface)}
                        <small className="network-secondary">
                          {network.applianceAddress}
                        </small>
                      </td>
                      <td
                        className="network-address"
                        data-label={t('network.dn.col.subnet')}
                      >
                        {network.scope === 'host'
                          ? network.target
                          : network.deviceSubnet}
                      </td>
                      <td data-label={t('network.dn.col.access')}>
                        {t(ACCESS_KEYS[network.access])}
                        <small className="network-secondary network-address">
                          {t('network.dn.col.exposed')}: {network.exposed}
                        </small>
                      </td>
                      <td data-label={t('network.dn.col.status')}>
                        <span className="network-state">
                          <span
                            className={`state-dot ${stateDot}`}
                            aria-hidden="true"
                          />
                          {stateLabel}
                        </span>
                        <small className="network-secondary">
                          {network.enabled
                            ? network.applied
                              ? t('network.dn.enabled')
                              : `${t('network.dn.enabled')}, ${t('network.dn.notApplied')}`
                            : t('network.wg.disabled')}
                        </small>
                        {network.detail !== '' && network.state !== 'active' && (
                          <small className="network-secondary">{network.detail}</small>
                        )}
                      </td>
                      <td className="record-cell-actions">
                        <RowActions
                          name={network.name}
                          typeLabel={t('actions.kind.deviceNetwork')}
                          status={stateLabel}
                          actions={actions}
                        >
                          <DetailsToggle
                            name={network.name}
                            expanded={isExpanded}
                            controls={detailsId}
                            onToggle={() => setExpanded(isExpanded ? null : network.id)}
                          />
                        </RowActions>
                      </td>
                    </tr>
                    {isExpanded && (
                      <tr className="record-details">
                        <td colSpan={6}>
                          <div
                            id={detailsId}
                            role="region"
                            aria-label={`${t('network.details')} ${network.name}`}
                          >
                            <dl className="record-facts">
                              <div>
                                <dt>{t('network.dn.fact.location')}</dt>
                                <dd>{network.location || '—'}</dd>
                              </div>
                              <div>
                                <dt>{t('network.dn.fact.subnet')}</dt>
                                <dd>{network.deviceSubnet}</dd>
                              </div>
                              <div>
                                <dt>{t('network.dn.col.exposed')}</dt>
                                <dd>{network.exposed}</dd>
                              </div>
                              {network.access !== 'wireguard' && (
                                <>
                                  <div>
                                    <dt>{t('network.dn.fact.localAccess')}</dt>
                                    <dd>{adapterName(network.localInterface)}</dd>
                                  </div>
                                  <div>
                                    <dt>{t('network.dn.fact.localSources')}</dt>
                                    <dd>
                                      {network.localSources.join(', ') ||
                                        t('network.dn.noneListed')}
                                    </dd>
                                  </div>
                                </>
                              )}
                              {network.access !== 'local' && (
                                <>
                                  <div>
                                    <dt>{t('network.dn.fact.vpn')}</dt>
                                    <dd>
                                      {profiles.find(
                                        (profile) =>
                                          profile.id === network.vpnProfileId,
                                      )?.name ?? network.vpnProfileId}
                                    </dd>
                                  </div>
                                  <div>
                                    <dt>{t('network.dn.fact.vpnSources')}</dt>
                                    <dd>
                                      {network.vpnSources.join(', ') ||
                                        t('network.dn.noneListed')}
                                    </dd>
                                  </div>
                                </>
                              )}
                              <div>
                                <dt>{t('network.dn.fact.services')}</dt>
                                <dd>{network.services.protocols.join(', ')}</dd>
                              </div>
                              <div>
                                <dt>{t('network.dn.fact.tcpPorts')}</dt>
                                <dd>
                                  {network.services.tcpPorts.join(', ') ||
                                    t('network.dn.allPorts')}
                                </dd>
                              </div>
                              <div>
                                <dt>{t('network.dn.fact.udpPorts')}</dt>
                                <dd>
                                  {network.services.udpPorts.join(', ') ||
                                    t('network.dn.allPorts')}
                                </dd>
                              </div>
                              <div>
                                <dt>{t('network.dn.fact.source')}</dt>
                                <dd>
                                  {t(
                                    network.services.preserveSource
                                      ? 'network.dn.preserve'
                                      : 'network.dn.snat',
                                  )}
                                </dd>
                              </div>
                              <div>
                                <dt>{t('network.dn.fact.startup')}</dt>
                                <dd>
                                  {t(
                                    network.enabled
                                      ? 'network.dn.startupOn'
                                      : 'network.dn.startupOff',
                                  )}
                                </dd>
                              </div>
                              <div>
                                <dt>{t('network.dn.fact.observed')}</dt>
                                <dd>{network.observedAt.toLocaleString()}</dd>
                              </div>
                            </dl>
                          </div>
                        </td>
                      </tr>
                    )}
                  </Fragment>
                );
              })}
            </tbody>
          </table>
        </div>
      )}

      {editing !== null && (
        <Modal
          title={
            editing.id === null
              ? t('network.dn.dialog.new')
              : t('network.dn.dialog.edit').replace('{name}', editing.form.name)
          }
          onDismiss={() => setEditing(null)}
        >
          <div className="network-section-head">
            <span />
            <WorkflowHelp prefix="help.dn" title={t('help.dn.title')} />
          </div>
          <form
            className="dialog-form"
            noValidate
            onSubmit={(event) => {
              event.preventDefault();
              void submit();
            }}
          >
            <fieldset
              className="form-group"
              aria-label={t('network.dn.group.identity')}
            >
              <legend>{t('network.dn.group.identity')}</legend>
              {textInput('name', 'network.dn.field.name')}
              {textInput('location', 'network.dn.field.location')}
            </fieldset>

            <fieldset
              className="form-group"
              aria-label={t('network.dn.group.deviceSide')}
            >
              <legend>{t('network.dn.group.deviceSide')}</legend>
              {selectInput(
                'deviceInterface',
                'network.dn.field.deviceInterface',
                deviceAdapterChoices,
                {
                  help: 'network.dn.help.deviceInterface',
                  disabled: !managementRolesReady,
                },
              )}
              {!managementRolesReady && (
                <p className="notice notice-unknown" role="status">
                  {t(
                    managementRoleState === 'failed'
                      ? 'network.role.managementInventoryFailed'
                      : 'network.role.managementInventoryReading',
                  )}
                </p>
              )}
              {managementClaims.size > 0 && (
                <div className="notice" role="note">
                  {adapters
                    .filter((adapter) =>
                      managementClaims.has(adapter.adapterId.toLowerCase()),
                    )
                    .map((adapter) => (
                      <p key={adapter.adapterId}>
                        {t('network.role.managementUnavailable', {
                          name: adapter.name,
                        })}
                      </p>
                    ))}
                  <WorkflowHelp
                    prefix="help.roles"
                    title={t('help.roles.title')}
                    buttonLabel={t('help.roles.open')}
                  />
                </div>
              )}
              {textInput('applianceAddress', 'network.dn.field.applianceAddress', {
                placeholder: '192.168.0.1/24',
                help: 'network.dn.help.applianceAddress',
                guidance: 'network.dn.guidance.applianceAddress',
              })}
              {selectInput(
                'scope',
                'network.dn.field.scope',
                <>
                  <option value="host">{t('network.dn.field.scope.host')}</option>
                  <option value="subnet">{t('network.dn.field.scope.subnet')}</option>
                </>,
                { help: 'network.dn.help.scope' },
              )}
              {editing.form.scope === 'host' &&
                textInput('target', 'network.dn.field.target', {
                  placeholder: '192.168.0.130',
                  help: 'network.dn.help.target',
                })}
            </fieldset>

            <fieldset className="form-group" aria-label={t('network.dn.group.access')}>
              <legend>{t('network.dn.group.access')}</legend>
              {selectInput(
                'access',
                'network.dn.field.access',
                <>
                  <option value="local">{t('network.dn.access.local')}</option>
                  <option value="wireguard">{t('network.dn.access.wireguard')}</option>
                  <option value="both">{t('network.dn.access.both')}</option>
                </>,
                { help: 'network.dn.help.access' },
              )}
              {editing.form.access !== 'wireguard' &&
                selectInput(
                  'localInterface',
                  'network.dn.field.localInterface',
                  localAdapterChoices,
                  {
                    help: 'network.dn.help.localInterface',
                  },
                )}
              {editing.form.access !== 'wireguard' &&
                textInput('localSources', 'network.dn.field.localSources', {
                  placeholder: '192.168.10.0/24',
                  help: 'network.dn.help.sources',
                  guidance: 'network.dn.guidance.sources',
                })}
              {editing.form.access !== 'local' &&
                selectInput(
                  'vpnProfileId',
                  'network.dn.field.vpnProfile',
                  <>
                    <option value="">—</option>
                    {owned.map((profile) => (
                      <option key={profile.id} value={profile.id}>
                        {profile.name}
                      </option>
                    ))}
                  </>,
                  {
                    help: 'network.dn.help.vpnProfile',
                    guidance: 'network.dn.guidance.vpn',
                  },
                )}
              {editing.form.access !== 'local' &&
                textInput('vpnSources', 'network.dn.field.vpnSources', {
                  placeholder: '10.8.0.0/24',
                  help: 'network.dn.help.sources',
                  guidance: 'network.dn.guidance.sources',
                })}
            </fieldset>

            <fieldset
              className="form-group"
              aria-label={t('network.dn.group.addressing')}
            >
              <legend>{t('network.dn.group.addressing')}</legend>
              {selectInput(
                'addressing',
                'network.dn.field.addressing',
                <>
                  <option value="original">
                    {t('network.dn.field.addressing.original')}
                  </option>
                  <option value="translated">
                    {t('network.dn.field.addressing.translated')}
                  </option>
                </>,
                { help: 'network.dn.help.addressing' },
              )}
              {editing.form.addressing === 'translated' &&
                textInput('exposed', 'network.dn.field.exposed', {
                  placeholder: '10.7.1.130',
                  help: 'network.dn.help.exposed',
                })}
            </fieldset>

            <fieldset
              className="form-group"
              aria-label={t('network.dn.group.services')}
            >
              <legend>{t('network.dn.group.services')}</legend>
              <div className="field">
                <span className="field-label">{t('network.dn.field.protocols')}</span>
                <div className="row-actions">
                  {['tcp', 'udp', 'icmp'].map((protocol) => (
                    <label key={protocol} className="scope">
                      <input
                        type="checkbox"
                        checked={editing.form.protocols.includes(protocol)}
                        onChange={(event) =>
                          set(
                            'protocols',
                            event.target.checked
                              ? [...editing.form.protocols, protocol]
                              : editing.form.protocols.filter(
                                  (entry) => entry !== protocol,
                                ),
                          )
                        }
                      />
                      {protocol}
                    </label>
                  ))}
                </div>
              </div>
              {textInput('tcpPorts', 'network.dn.field.tcpPorts', {
                placeholder: '502',
                help: 'network.dn.help.ports',
                guidance: 'network.dn.guidance.ports',
              })}
              {textInput('udpPorts', 'network.dn.field.udpPorts', {
                help: 'network.dn.help.ports',
              })}
            </fieldset>

            <details className="form-group">
              <summary>{t('network.dn.group.advanced')}</summary>
              <div className="field">
                <div className="field-label-row">
                  <label htmlFor="dn-preserveSource">
                    <input
                      id="dn-preserveSource"
                      type="checkbox"
                      checked={editing.form.preserveSource}
                      onChange={(event) => set('preserveSource', event.target.checked)}
                    />
                    {t('network.dn.field.preserveSource')}
                  </label>
                  <HelpPopover name={t('network.dn.field.preserveSource')}>
                    <p>{t('network.dn.help.preserveSource')}</p>
                  </HelpPopover>
                </div>
                <p className="field-guidance">
                  {t('network.dn.guidance.preserveSource')}
                </p>
              </div>
            </details>

            <section
              className="review"
              role="region"
              aria-label={t('network.dn.group.review')}
            >
              <h4>{t('network.dn.group.review')}</h4>
              <ul>
                {review(editing.form).map((line) => (
                  <li key={line}>{line}</li>
                ))}
              </ul>
            </section>

            {refusal !== '' && (
              <p role="alert" className="field-error">
                {refusal}
              </p>
            )}
            <div className="modal-actions">
              <button
                type="submit"
                className="button button-primary"
                disabled={busy || !managementRolesReady || selectedDeviceRoleConflict}
              >
                {t('network.dn.save')}
              </button>
              <button type="button" className="button" onClick={() => setEditing(null)}>
                {t('auth.cancel')}
              </button>
            </div>
          </form>
        </Modal>
      )}

      {checking !== null && (
        <Modal
          title={t('network.dn.check.title').replace('{name}', checking.network.name)}
          onDismiss={() => setChecking(null)}
        >
          <form
            className="dialog-form"
            onSubmit={(event) => {
              event.preventDefault();
              void runCheck();
            }}
          >
            <Field
              id="dn-check-port"
              label={t('network.dn.check.port')}
              help={<p>{t('network.dn.help.ports')}</p>}
            >
              <input
                id="dn-check-port"
                inputMode="numeric"
                value={checking.port}
                onChange={(event) =>
                  setChecking({ ...checking, port: event.target.value })
                }
              />
            </Field>
            {checking.result !== null && (
              <dl
                className="record-facts network-check-facts"
                aria-label={t('network.dn.check.title').replace(
                  '{name}',
                  checking.network.name,
                )}
              >
                {checking.result.facts.map((fact) => (
                  <div key={fact.name}>
                    <dt>{fact.name}</dt>
                    <dd>
                      {t(FACT_KEYS[fact.result] ?? 'network.dn.fact.unknown')}:{' '}
                      {fact.detail}
                    </dd>
                  </div>
                ))}
                {checking.result.facts.length === 0 && (
                  <p role="alert">{t('network.wg.pollFailed')}</p>
                )}
              </dl>
            )}
            <div className="modal-actions">
              <button type="submit" className="button button-primary">
                {t('network.dn.check.run')}
              </button>
              <button
                type="button"
                className="button"
                onClick={() => setChecking(null)}
              >
                {t('auth.cancel')}
              </button>
            </div>
          </form>
        </Modal>
      )}

      {showing !== null && (
        <Modal
          title={t('network.dn.instructions.title').replace(
            '{name}',
            showing.network.name,
          )}
          onDismiss={() => setShowing(null)}
        >
          {showing.set === null ? (
            <p>{t('network.reading')}</p>
          ) : (
            <>
              {showing.set.targetAddress !== '' && (
                <p>
                  {t('network.dn.instructions.target')}:{' '}
                  <code>{showing.set.targetAddress}</code>
                </p>
              )}
              {showing.set.unmetPrerequisites.length > 0 && (
                <p className="notice notice-unknown">
                  {t('network.dn.instructions.unmet')}{' '}
                  {showing.set.unmetPrerequisites.join('; ')}
                </p>
              )}
              <ol>
                {showing.set.steps.map((step, index) => (
                  <li key={index}>
                    <strong>{step.machine}</strong>: {step.text}
                  </li>
                ))}
              </ol>
              <div className="modal-actions">
                <button
                  type="button"
                  className="button"
                  onClick={() =>
                    void navigator.clipboard?.writeText(
                      (showing.set?.steps ?? [])
                        .map((step) => `${step.machine}: ${step.text}`)
                        .join('\n'),
                    )
                  }
                >
                  {t('network.dn.copy')}
                </button>
              </div>
            </>
          )}
        </Modal>
      )}
    </section>
  );
}
