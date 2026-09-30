import { useCallback, useEffect, useState } from 'react';
import {
  BenchCoreError,
  type BenchCoreClient,
  type Credential,
} from '../../api/client';
import {
  isJobTerminal,
  type InstrumentJob,
  type InstrumentPlugin,
  type PluginAction,
} from '../../api/bench';
import type {
  ModbusEndpoint,
  ModbusProfile,
  ModbusSettingsState,
  ModbusSettingsValues,
} from '../../api/modbus';
import type { BenchCoreSession } from '../../api/contract';
import { useTranslate } from '../../i18n/languages';
import { MessageBanner } from '../MessageBanner';
import './modbus.css';

interface Props {
  client: BenchCoreClient;
  credential: Credential;
  session: BenchCoreSession;
  onSessionEnded: () => void;
  onPluginsChanged?: () => void;
  onOpenActivity?: () => void;
}

const newEndpoint = (): ModbusEndpoint => ({
  schema_version: 1,
  id: '',
  revision: 1,
  name: '',
  transport: 'tcp',
  resource_id: '',
  enabled: true,
  presence: 'configured',
  readiness: 'offline',
  network: { host: '', port: 502, unit_min: 1, unit_max: 247 },
});
const newProfile = (): ModbusProfile => ({
  schema_version: 1,
  id: '',
  revision: 1,
  owner_id: '',
  name: '',
  endpoint_id: '',
  settings: {
    response_timeout_ms: 1000,
    connect_timeout_ms: 1000,
    inter_request_ms: 0,
    read_retries: 0,
  },
});

/** Administrator-facing Modbus configuration; mounting this page performs no transport I/O. */
export function ModbusPluginPage({
  client,
  credential,
  session,
  onSessionEnded,
  onPluginsChanged,
  onOpenActivity,
}: Props) {
  const t = useTranslate();
  const [plugin, setPlugin] = useState<InstrumentPlugin>();
  const [endpoints, setEndpoints] = useState<ModbusEndpoint[]>([]);
  const [profiles, setProfiles] = useState<ModbusProfile[]>([]);
  const [settings, setSettings] = useState<ModbusSettingsState>();
  const [settingsDraft, setSettingsDraft] = useState<ModbusSettingsValues>();
  const [endpointDraft, setEndpointDraft] = useState<ModbusEndpoint>();
  const [endpointExpectedRevision, setEndpointExpectedRevision] = useState(0);
  const [profileDraft, setProfileDraft] = useState<ModbusProfile>();
  const [profileExpectedRevision, setProfileExpectedRevision] = useState(0);
  const [selectedEndpoint, setSelectedEndpoint] = useState('');
  const [selectedProfile, setSelectedProfile] = useState('');
  const [job, setJob] = useState<InstrumentJob>();
  const [busy, setBusy] = useState(false);
  const [modeDraft, setModeDraft] = useState<'ask' | 'auto'>();
  const [feedback, setFeedback] = useState<{ tone: 'info' | 'error'; text: string }>();
  const canManage =
    session.role === 'administrator' && session.permissions.includes('plugins.manage');

  const report = useCallback(
    (error: unknown) => {
      if (error instanceof BenchCoreError && error.failure === 'unauthorized')
        onSessionEnded();
      setFeedback({
        tone: 'error',
        text:
          error instanceof BenchCoreError
            ? error.code || error.message
            : t('modbus.unavailable'),
      });
    },
    [onSessionEnded, t],
  );

  const refresh = useCallback(async () => {
    const [inventory, savedEndpoints, savedProfiles, savedSettings] = await Promise.all(
      [
        client.listPlugins(credential),
        client.listModbusEndpoints?.(credential) ?? Promise.resolve([]),
        client.listModbusProfiles?.(credential) ?? Promise.resolve([]),
        client.getModbusSettings?.(credential) ?? Promise.resolve(undefined),
      ],
    );
    setPlugin(inventory.find((item) => item.id === 'modbus'));
    setEndpoints(savedEndpoints);
    setProfiles(savedProfiles);
    setSettings(savedSettings);
    setSettingsDraft((old) => old ?? savedSettings?.settings);
  }, [client, credential]);

  useEffect(() => {
    if (!canManage) return;
    const initial = window.setTimeout(() => void refresh().catch(report), 0);
    return () => window.clearTimeout(initial);
  }, [canManage, refresh, report]);
  useEffect(() => {
    if (!job || isJobTerminal(job)) return;
    const timer = window.setInterval(() => {
      void client.getInstrumentJob(credential, job.transaction_id).then(setJob, report);
    }, 1000);
    return () => window.clearInterval(timer);
  }, [client, credential, job, report]);

  async function run(action: () => Promise<unknown>, updatePlugin = false) {
    setBusy(true);
    setFeedback(undefined);
    try {
      await action();
      await refresh();
      if (updatePlugin) onPluginsChanged?.();
      setFeedback({ tone: 'info', text: t('modbus.saved') });
    } catch (error) {
      report(error);
    } finally {
      setBusy(false);
    }
  }

  function lifecycle(action: PluginAction, mode?: 'ask' | 'auto') {
    if (action === 'uninstall' && !window.confirm(t('modbus.confirmUninstall'))) return;
    void run(() => client.pluginAction(credential, 'modbus', action, mode), true);
    if (action === 'enable') setModeDraft(undefined);
  }

  const selectableEndpoints = endpoints.filter(
    (item) => item.enabled && item.transport !== 'rtu',
  );
  const selected = selectableEndpoints.find((item) => item.id === selectedEndpoint);
  const selectedProfileRecord = profiles.find(
    (item) => item.id === selectedProfile && item.endpoint_id === selectedEndpoint,
  );

  if (!canManage)
    return (
      <div className="page">
        <header className="page-head">
          <h2>{t('modbus.admin')}</h2>
        </header>
        <div className="page-body">
          <MessageBanner tone="warning">{t('modbus.adminOnly')}</MessageBanner>
        </div>
      </div>
    );
  return (
    <div className="page">
      <header className="page-head">
        <div className="page-title-actions">
          <h2>{t('modbus.admin')}</h2>
          <button type="button" onClick={() => void refresh().catch(report)}>
            {t('modbus.refresh')}
          </button>
          {onOpenActivity && (
            <button type="button" onClick={onOpenActivity}>
              {t('network.activity.open')}
            </button>
          )}
        </div>
      </header>
      <div className="page-body modbus-admin">
        {feedback && (
          <MessageBanner tone={feedback.tone}>{feedback.text}</MessageBanner>
        )}
        <section className="instrument-card" aria-label={t('modbus.plugin')}>
          <h3>{t('modbus.plugin')}</h3>
          <p>
            {t('modbus.version')}: {plugin?.version || '—'} · {t('modbus.status')}:{' '}
            {plugin?.status || '—'}
          </p>
          <div className="modbus-actions">
            {!plugin?.installed && (
              <button
                type="button"
                disabled={busy || !plugin}
                onClick={() => lifecycle('install')}
              >
                {t('modbus.install')}
              </button>
            )}
            {plugin?.installed && (
              <>
                <button
                  type="button"
                  disabled={busy || plugin.enabled}
                  onClick={() => lifecycle('enable', modeDraft ?? plugin.mode)}
                >
                  {t('modbus.enable')}
                </button>
                <button
                  type="button"
                  disabled={busy || !plugin.enabled}
                  onClick={() => lifecycle('disable')}
                >
                  {t('modbus.disable')}
                </button>
                <button
                  type="button"
                  disabled={busy || !plugin.enabled}
                  onClick={() => lifecycle('restart')}
                >
                  {t('modbus.restart')}
                </button>
                <button
                  type="button"
                  disabled={busy || !plugin.update_available}
                  onClick={() => lifecycle('upgrade')}
                >
                  {t('modbus.update')}
                </button>
                <button
                  type="button"
                  disabled={busy}
                  onClick={() => lifecycle('uninstall')}
                >
                  {t('modbus.uninstall')}
                </button>
              </>
            )}
          </div>
          {plugin?.installed && (
            <label>
              {t('modbus.policy')}
              <select
                aria-label={t('modbus.policy')}
                value={modeDraft ?? plugin.mode}
                disabled={busy}
                onChange={(event) => {
                  const mode = event.target.value as 'ask' | 'auto';
                  if (plugin.enabled) lifecycle('enable', mode);
                  else setModeDraft(mode);
                }}
              >
                <option value="ask">{t('modbus.ask')}</option>
                <option value="auto">{t('modbus.auto')}</option>
              </select>
            </label>
          )}
        </section>

        <section className="instrument-card" aria-label={t('modbus.endpoints')}>
          <div className="page-title-actions">
            <h3>{t('modbus.endpoints')}</h3>
            <button
              type="button"
              onClick={() => {
                setEndpointExpectedRevision(0);
                setEndpointDraft(newEndpoint());
              }}
            >
              {t('modbus.newEndpoint')}
            </button>
          </div>
          {endpoints.length === 0 && <p>{t('modbus.noEndpoints')}</p>}
          <ul className="modbus-list">
            {endpoints.map((item) => (
              <li key={item.id}>
                <span>
                  <strong>{item.name}</strong> · {item.transport.toUpperCase()} ·{' '}
                  {item.network?.host ?? item.presence} · {item.readiness} · r
                  {item.revision}
                </span>
                {item.transport !== 'rtu' && (
                  <button
                    type="button"
                    onClick={() => {
                      setEndpointExpectedRevision(item.revision);
                      setEndpointDraft(item);
                    }}
                  >
                    {t('modbus.edit')} {item.name}
                  </button>
                )}
              </li>
            ))}
          </ul>
          {endpointDraft && (
            <form
              className="modbus-form"
              onSubmit={(event) => {
                event.preventDefault();
                if (!client.saveModbusEndpoint) return;
                void run(async () => {
                  await client.saveModbusEndpoint!(
                    credential,
                    endpointExpectedRevision,
                    endpointDraft,
                  );
                  setEndpointDraft(undefined);
                });
              }}
            >
              <label>
                {t('modbus.id')}
                <input
                  required
                  pattern="[A-Za-z0-9_.:-]+"
                  maxLength={128}
                  value={endpointDraft.id}
                  disabled={endpointExpectedRevision !== 0}
                  onChange={(event) =>
                    setEndpointDraft({ ...endpointDraft, id: event.target.value })
                  }
                />
              </label>
              <label>
                {t('modbus.name')}
                <input
                  required
                  maxLength={128}
                  value={endpointDraft.name}
                  onChange={(event) =>
                    setEndpointDraft({ ...endpointDraft, name: event.target.value })
                  }
                />
              </label>
              <label>
                {t('modbus.resourceId')}
                <input
                  required
                  pattern="[A-Za-z0-9_.:-]+"
                  maxLength={128}
                  value={endpointDraft.resource_id}
                  onChange={(event) =>
                    setEndpointDraft({
                      ...endpointDraft,
                      resource_id: event.target.value,
                    })
                  }
                />
              </label>
              <label>
                {t('modbus.host')}
                <input
                  required
                  value={endpointDraft.network?.host ?? ''}
                  onChange={(event) =>
                    setEndpointDraft({
                      ...endpointDraft,
                      network: { ...endpointDraft.network!, host: event.target.value },
                    })
                  }
                />
              </label>
              <label>
                {t('modbus.port')}
                <input
                  required
                  type="number"
                  min={1}
                  max={65535}
                  value={endpointDraft.network?.port ?? 502}
                  onChange={(event) =>
                    setEndpointDraft({
                      ...endpointDraft,
                      network: {
                        ...endpointDraft.network!,
                        port: Number(event.target.value),
                      },
                    })
                  }
                />
              </label>
              <label>
                {t('modbus.unitMin')}
                <input
                  required
                  type="number"
                  min={0}
                  max={255}
                  value={endpointDraft.network?.unit_min ?? 1}
                  onChange={(event) =>
                    setEndpointDraft({
                      ...endpointDraft,
                      network: {
                        ...endpointDraft.network!,
                        unit_min: Number(event.target.value),
                      },
                    })
                  }
                />
              </label>
              <label>
                {t('modbus.unitMax')}
                <input
                  required
                  type="number"
                  min={0}
                  max={255}
                  value={endpointDraft.network?.unit_max ?? 247}
                  onChange={(event) =>
                    setEndpointDraft({
                      ...endpointDraft,
                      network: {
                        ...endpointDraft.network!,
                        unit_max: Number(event.target.value),
                      },
                    })
                  }
                />
              </label>
              <label className="modbus-check">
                <input
                  type="checkbox"
                  checked={endpointDraft.enabled}
                  onChange={(event) =>
                    setEndpointDraft({
                      ...endpointDraft,
                      enabled: event.target.checked,
                    })
                  }
                />
                {t('modbus.enabled')}
              </label>
              <button type="submit" disabled={busy}>
                {t('modbus.save')}
              </button>
            </form>
          )}
        </section>

        <section className="instrument-card" aria-label={t('modbus.profiles')}>
          <div className="page-title-actions">
            <h3>{t('modbus.profiles')}</h3>
            <button
              type="button"
              onClick={() => {
                setProfileExpectedRevision(0);
                setProfileDraft(newProfile());
              }}
            >
              {t('modbus.newProfile')}
            </button>
          </div>
          {profiles.length === 0 && <p>{t('modbus.noProfiles')}</p>}
          <ul className="modbus-list">
            {profiles.map((item) => (
              <li key={item.id}>
                <span>
                  <strong>{item.name}</strong> · {item.endpoint_id} · r{item.revision}
                </span>
                <button
                  type="button"
                  onClick={() => {
                    setProfileExpectedRevision(item.revision);
                    setProfileDraft(item);
                  }}
                >
                  {t('modbus.edit')} {item.name}
                </button>
              </li>
            ))}
          </ul>
          {profileDraft && (
            <form
              className="modbus-form"
              onSubmit={(event) => {
                event.preventDefault();
                if (!client.saveModbusProfile) return;
                void run(async () => {
                  await client.saveModbusProfile!(
                    credential,
                    profileExpectedRevision,
                    profileDraft,
                  );
                  setProfileDraft(undefined);
                });
              }}
            >
              <label>
                {t('modbus.id')}
                <input
                  required
                  pattern="[A-Za-z0-9_.:-]+"
                  value={profileDraft.id}
                  disabled={profileExpectedRevision !== 0}
                  onChange={(event) =>
                    setProfileDraft({ ...profileDraft, id: event.target.value })
                  }
                />
              </label>
              <label>
                {t('modbus.name')}
                <input
                  required
                  value={profileDraft.name}
                  onChange={(event) =>
                    setProfileDraft({ ...profileDraft, name: event.target.value })
                  }
                />
              </label>
              <label>
                {t('modbus.endpoint')}
                <select
                  required
                  value={profileDraft.endpoint_id}
                  onChange={(event) =>
                    setProfileDraft({
                      ...profileDraft,
                      endpoint_id: event.target.value,
                    })
                  }
                >
                  <option value="">—</option>
                  {endpoints.map((item) => (
                    <option key={item.id} value={item.id}>
                      {item.name}
                    </option>
                  ))}
                </select>
              </label>
              {(
                [
                  'response_timeout_ms',
                  'connect_timeout_ms',
                  'inter_request_ms',
                  'read_retries',
                ] as const
              ).map((key) => (
                <label key={key}>
                  {t(
                    (
                      {
                        response_timeout_ms: 'modbus.responseTimeout',
                        connect_timeout_ms: 'modbus.connectTimeout',
                        inter_request_ms: 'modbus.interRequest',
                        read_retries: 'modbus.readRetries',
                      } as const
                    )[key],
                  )}
                  <input
                    required
                    type="number"
                    min={
                      key === 'response_timeout_ms' || key === 'connect_timeout_ms'
                        ? 1
                        : 0
                    }
                    value={profileDraft.settings[key]}
                    onChange={(event) =>
                      setProfileDraft({
                        ...profileDraft,
                        settings: {
                          ...profileDraft.settings,
                          [key]: Number(event.target.value),
                        },
                      })
                    }
                  />
                </label>
              ))}
              <button type="submit" disabled={busy}>
                {t('modbus.save')}
              </button>
            </form>
          )}
        </section>

        <section className="instrument-card" aria-label={t('modbus.test')}>
          <h3>{t('modbus.test')}</h3>
          <div className="modbus-form">
            <label>
              {t('modbus.endpoint')}
              <select
                value={selectedEndpoint}
                onChange={(event) => {
                  setSelectedEndpoint(event.target.value);
                  setSelectedProfile('');
                }}
              >
                <option value="">—</option>
                {selectableEndpoints.map((item) => (
                  <option key={item.id} value={item.id}>
                    {item.name}
                  </option>
                ))}
              </select>
            </label>
            <label>
              {t('modbus.profile')}
              <select
                value={selectedProfile}
                onChange={(event) => setSelectedProfile(event.target.value)}
              >
                <option value="">—</option>
                {profiles
                  .filter((item) => item.endpoint_id === selectedEndpoint)
                  .map((item) => (
                    <option key={item.id} value={item.id}>
                      {item.name}
                    </option>
                  ))}
              </select>
            </label>
            <button
              type="button"
              disabled={
                busy ||
                !plugin?.enabled ||
                !selected ||
                !selectedProfileRecord ||
                !client.testModbusConnection
              }
              onClick={() => {
                if (!selected || !selectedProfileRecord || !client.testModbusConnection)
                  return;
                setBusy(true);
                setFeedback(undefined);
                void client
                  .testModbusConnection(
                    credential,
                    selected.id,
                    selected.revision,
                    selectedProfileRecord.id,
                    selectedProfileRecord.revision,
                    crypto.randomUUID(),
                  )
                  .then(setJob, report)
                  .finally(() => setBusy(false));
              }}
            >
              {t('modbus.test')}
            </button>
          </div>
          {job && (
            <p role="status">
              {job.state === 'completed'
                ? t('modbus.testCompleted')
                : isJobTerminal(job)
                  ? t('modbus.testFailed')
                  : t('modbus.testPending')}{' '}
              ({job.state})
            </p>
          )}
        </section>

        <section className="instrument-card" aria-label={t('modbus.settings')}>
          <h3>{t('modbus.settings')}</h3>
          {settings && settingsDraft && (
            <form
              className="modbus-form"
              onSubmit={(event) => {
                event.preventDefault();
                if (!client.saveModbusSettings) return;
                void run(async () => {
                  const saved = await client.saveModbusSettings!(
                    credential,
                    settings.revision,
                    settingsDraft,
                  );
                  setSettingsDraft(saved.settings);
                });
              }}
            >
              {(
                [
                  ['max_sessions', 'modbus.maxSessions', 1, 16],
                  ['session_lease_seconds', 'modbus.lease', 60, 3600],
                  ['idle_seconds', 'modbus.idle', 30, 3600],
                  ['retention_count', 'modbus.retentionCount', 1, 1000],
                  ['retention_bytes', 'modbus.retentionBytes', 1048576, 17179869184],
                  ['part_bytes', 'modbus.partBytes', 1048576, 268435456],
                ] as const
              ).map(([key, label, min, max]) => (
                <label key={key}>
                  {t(label)}
                  <input
                    required
                    type="number"
                    min={min}
                    max={max}
                    value={settingsDraft[key]}
                    onChange={(event) =>
                      setSettingsDraft({
                        ...settingsDraft,
                        [key]: Number(event.target.value),
                      })
                    }
                  />
                </label>
              ))}
              <p className="modbus-wide">{t('modbus.futureRetention')}</p>
              <button type="submit" disabled={busy}>
                {t('modbus.save')}
              </button>
            </form>
          )}
        </section>
      </div>
    </div>
  );
}
