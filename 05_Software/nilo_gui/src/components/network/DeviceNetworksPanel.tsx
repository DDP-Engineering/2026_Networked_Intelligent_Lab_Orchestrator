import { useState, type ReactElement } from 'react';

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
import { useTranslate } from '../../i18n/languages';
import type { MessageKey } from '../../i18n/en';
import { Modal } from '../Modal';

export interface DeviceNetworksPanelProps {
  networks: DeviceNetwork[] | null;
  /** Adapters the editor can offer as device-side or local-access adapters. */
  interfaces: NetworkInterface[];
  /** NILO-owned WireGuard profiles the editor can offer. */
  profiles: WireGuardProfile[];
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

function emptyForm(interfaces: NetworkInterface[]): DraftForm {
  const adapters = interfaces.filter((entry) => entry.adapterId !== '');
  return {
    name: '',
    location: '',
    deviceInterface: adapters[1]?.adapterId ?? adapters[0]?.adapterId ?? '',
    applianceAddress: '192.168.0.1/24',
    scope: 'host',
    target: '',
    addressing: 'original',
    exposed: '',
    access: 'local',
    localInterface: adapters[0]?.adapterId ?? '',
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
 */
export function DeviceNetworksPanel({
  networks,
  interfaces,
  profiles,
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
  const [refusal, setRefusal] = useState('');
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
  const owned = profiles.filter((profile) => profile.niloOwned);

  const submit = async () => {
    if (editing === null) return;
    const draft = draftOf(editing.form);
    if (
      draft.name === '' ||
      draft.device_interface === '' ||
      (draft.scope === 'host' && draft.target === '')
    ) {
      setRefusal(t('network.dn.invalid'));
      return;
    }
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

  const field = (
    key: keyof DraftForm,
    label: MessageKey,
    input: (value: string, set: (next: string) => void) => ReactElement,
  ) => {
    if (editing === null) return null;
    const value = editing.form[key];
    return (
      <div className="field">
        <label htmlFor={`dn-${key}`}>{t(label)}</label>
        {input(String(value), (next) =>
          setEditing({ ...editing, form: { ...editing.form, [key]: next } }),
        )}
      </div>
    );
  };
  const textInput = (key: keyof DraftForm, label: MessageKey, placeholder = '') =>
    field(key, label, (value, set) => (
      <input
        id={`dn-${key}`}
        value={value}
        placeholder={placeholder}
        onChange={(event) => set(event.target.value)}
      />
    ));
  const adapterSelect = (key: keyof DraftForm, label: MessageKey) =>
    field(key, label, (value, set) => (
      <select
        id={`dn-${key}`}
        value={value}
        onChange={(event) => set(event.target.value)}
      >
        <option value="">—</option>
        {adapters.map((adapter) => (
          <option key={adapter.adapterId} value={adapter.adapterId}>
            {adapter.name} ({adapter.adapterId})
          </option>
        ))}
      </select>
    ));

  return (
    <section aria-labelledby="network-device-networks">
      <div className="network-section-head">
        <h3 id="network-device-networks">{t('network.dn.title')}</h3>
        {canManage && (
          <button
            type="button"
            disabled={busy}
            onClick={() => {
              setRefusal('');
              setEditing({ id: null, form: emptyForm(interfaces) });
            }}
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
          <table className="data-table">
            <thead>
              <tr>
                <th scope="col">{t('auth.field.name')}</th>
                <th scope="col">{t('network.dn.col.interface')}</th>
                <th scope="col">{t('network.dn.col.subnet')}</th>
                <th scope="col">{t('network.dn.col.access')}</th>
                <th scope="col">{t('network.dn.col.exposed')}</th>
                <th scope="col">{t('network.dn.col.status')}</th>
                <th scope="col">{t('network.table.actions')}</th>
              </tr>
            </thead>
            <tbody>
              {networks.map((network) => {
                const adapter = interfaces.find(
                  (entry) => entry.adapterId === network.deviceInterface,
                );
                return (
                  <tr key={network.id}>
                    <th scope="row">
                      {network.name}
                      {network.location !== '' && (
                        <small className="network-secondary">{network.location}</small>
                      )}
                    </th>
                    <td>
                      {adapter?.name ?? network.deviceInterface}
                      <small className="network-secondary">
                        {network.applianceAddress}
                      </small>
                    </td>
                    <td className="network-address">
                      {network.scope === 'host' ? network.target : network.deviceSubnet}
                    </td>
                    <td>{t(ACCESS_KEYS[network.access])}</td>
                    <td className="network-address">{network.exposed}</td>
                    <td title={network.detail || undefined}>
                      <span className="network-state">
                        <span
                          className={`state-dot ${network.state === 'active' ? 'state-ok' : 'state-unknown'}`}
                          aria-hidden="true"
                        />
                        {t(STATE_KEYS[network.state])}
                      </span>
                      <small className="network-secondary">
                        {network.enabled
                          ? network.applied
                            ? t('network.dn.enabled')
                            : `${t('network.dn.enabled')}, ${t('network.dn.notApplied')}`
                          : t('network.wg.disabled')}
                      </small>
                    </td>
                    <td>
                      <div className="network-actions">
                        {canManage && (
                          <>
                            <button
                              type="button"
                              disabled={busy}
                              aria-label={`${t('network.dn.edit')} ${network.name}`}
                              onClick={() => {
                                setRefusal('');
                                setEditing({ id: network.id, form: formOf(network) });
                              }}
                            >
                              {t('network.dn.edit')}
                            </button>
                            <button
                              type="button"
                              disabled={busy}
                              aria-label={`${t(network.enabled ? 'network.dn.disable' : 'network.dn.enable')} ${network.name}`}
                              onClick={() =>
                                void propose({
                                  action: network.enabled ? 'disable' : 'enable',
                                  target: network.id,
                                })
                              }
                            >
                              {t(
                                network.enabled
                                  ? 'network.dn.disable'
                                  : 'network.dn.enable',
                              )}
                            </button>
                            <button
                              type="button"
                              disabled={busy}
                              aria-label={`${t('network.dn.delete')} ${network.name}`}
                              onClick={() =>
                                void propose({ action: 'delete', target: network.id })
                              }
                            >
                              {t('network.dn.delete')}
                            </button>
                            <button
                              type="button"
                              aria-label={`${t('network.dn.check')} ${network.name}`}
                              onClick={() =>
                                setChecking({
                                  network,
                                  port: String(network.services.tcpPorts[0] ?? ''),
                                  result: null,
                                })
                              }
                            >
                              {t('network.dn.check')}
                            </button>
                          </>
                        )}
                        <button
                          type="button"
                          aria-label={`${t('network.dn.instructions')} ${network.name}`}
                          onClick={() => void showInstructions(network)}
                        >
                          {t('network.dn.instructions')}
                        </button>
                        {openActivity !== undefined && (
                          <button
                            type="button"
                            aria-label={`${t('network.dn.activity')} ${network.name}`}
                            onClick={() => openActivity(network.id)}
                          >
                            {t('network.dn.activity')}
                          </button>
                        )}
                      </div>
                    </td>
                  </tr>
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
          <form
            className="dialog-form"
            onSubmit={(event) => {
              event.preventDefault();
              void submit();
            }}
          >
            {textInput('name', 'network.dn.field.name')}
            {textInput('location', 'network.dn.field.location')}
            {adapterSelect('deviceInterface', 'network.dn.field.deviceInterface')}
            {textInput(
              'applianceAddress',
              'network.dn.field.applianceAddress',
              '192.168.0.1/24',
            )}
            {field('scope', 'network.dn.field.scope', (value, set) => (
              <select
                id="dn-scope"
                value={value}
                onChange={(event) => set(event.target.value)}
              >
                <option value="host">{t('network.dn.field.scope.host')}</option>
                <option value="subnet">{t('network.dn.field.scope.subnet')}</option>
              </select>
            ))}
            {editing.form.scope === 'host' &&
              textInput('target', 'network.dn.field.target', '192.168.0.130')}
            {field('addressing', 'network.dn.field.addressing', (value, set) => (
              <select
                id="dn-addressing"
                value={value}
                onChange={(event) => set(event.target.value)}
              >
                <option value="original">
                  {t('network.dn.field.addressing.original')}
                </option>
                <option value="translated">
                  {t('network.dn.field.addressing.translated')}
                </option>
              </select>
            ))}
            {editing.form.addressing === 'translated' &&
              textInput('exposed', 'network.dn.field.exposed', '10.7.1.130')}
            {field('access', 'network.dn.field.access', (value, set) => (
              <select
                id="dn-access"
                value={value}
                onChange={(event) => set(event.target.value)}
              >
                <option value="local">{t('network.dn.access.local')}</option>
                <option value="wireguard">{t('network.dn.access.wireguard')}</option>
                <option value="both">{t('network.dn.access.both')}</option>
              </select>
            ))}
            {editing.form.access !== 'wireguard' &&
              adapterSelect('localInterface', 'network.dn.field.localInterface')}
            {editing.form.access !== 'wireguard' &&
              textInput(
                'localSources',
                'network.dn.field.localSources',
                '192.168.10.0/24',
              )}
            {editing.form.access !== 'local' &&
              field('vpnProfileId', 'network.dn.field.vpnProfile', (value, set) => (
                <select
                  id="dn-vpnProfileId"
                  value={value}
                  onChange={(event) => set(event.target.value)}
                >
                  <option value="">—</option>
                  {owned.map((profile) => (
                    <option key={profile.id} value={profile.id}>
                      {profile.name}
                    </option>
                  ))}
                </select>
              ))}
            {editing.form.access !== 'local' &&
              textInput('vpnSources', 'network.dn.field.vpnSources', '10.8.0.0/24')}
            {textInput('tcpPorts', 'network.dn.field.tcpPorts', '502')}
            {textInput('udpPorts', 'network.dn.field.udpPorts')}
            <div className="field">
              <label htmlFor="dn-preserveSource">
                <input
                  id="dn-preserveSource"
                  type="checkbox"
                  checked={editing.form.preserveSource}
                  onChange={(event) =>
                    setEditing({
                      ...editing,
                      form: { ...editing.form, preserveSource: event.target.checked },
                    })
                  }
                />{' '}
                {t('network.dn.field.preserveSource')}
              </label>
            </div>
            {refusal !== '' && (
              <p role="alert" className="field-error">
                {refusal}
              </p>
            )}
            <div className="modal-actions">
              <button type="submit" disabled={busy}>
                {t('network.dn.save')}
              </button>
              <button type="button" onClick={() => setEditing(null)}>
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
            <div className="field">
              <label htmlFor="dn-check-port">{t('network.dn.check.port')}</label>
              <input
                id="dn-check-port"
                inputMode="numeric"
                value={checking.port}
                onChange={(event) =>
                  setChecking({ ...checking, port: event.target.value })
                }
              />
            </div>
            <div className="modal-actions">
              <button type="submit">{t('network.dn.check.run')}</button>
              <button type="button" onClick={() => setChecking(null)}>
                {t('auth.cancel')}
              </button>
            </div>
          </form>
          {checking.result !== null && (
            <dl
              className="network-profile-facts"
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
