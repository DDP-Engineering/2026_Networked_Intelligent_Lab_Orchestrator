import { useCallback, useEffect, useState } from 'react';
import {
  BenchCoreError,
  type BenchCoreClient,
  type Credential,
} from '../../api/client';
import type { BenchCoreSession } from '../../api/contract';
import type { InstrumentDevice, InstrumentPlugin } from '../../api/bench';
import { useTranslate } from '../../i18n/languages';
import { MessageBanner } from '../MessageBanner';
import { OperatingSettingsSection } from './OperatingSettingsPage';
import { HelpPopover } from '../help/HelpPopover';

interface Props {
  pluginId?: 'camera' | 'serial';
  client: BenchCoreClient;
  credential: Credential;
  session: BenchCoreSession;
  onSessionEnded: () => void;
  onPluginsChanged?: () => void;
  onOpenActivity?: () => void;
}

/** Administrator-only configuration; capture remains in the instrument workspace. */
export function CameraPluginPage(props: Props) {
  const translate = useTranslate();
  const t = (
    key: Parameters<typeof translate>[0],
    values?: Parameters<typeof translate>[1],
  ) => translate(props.pluginId === 'serial' ? serialAdminKey(key) : key, values);
  if (
    props.session.role !== 'administrator' ||
    !props.session.permissions.includes('plugins.manage')
  )
    return (
      <div className="page">
        <header className="page-head">
          <h2>{t('camera.pluginTitle')}</h2>
        </header>
        <div className="page-body">
          <MessageBanner tone="warning">{t('camera.adminOnly')}</MessageBanner>
        </div>
      </div>
    );
  return <CameraPluginConfiguration {...props} />;
}

function CameraPluginConfiguration({
  client,
  credential,
  pluginId = 'camera',
  session,
  onSessionEnded,
  onPluginsChanged,
  onOpenActivity,
}: Props) {
  const translate = useTranslate();
  const t = useCallback(
    (key: Parameters<typeof translate>[0], values?: Parameters<typeof translate>[1]) =>
      translate(pluginId === 'serial' ? serialAdminKey(key) : key, values),
    [translate, pluginId],
  );
  const [plugin, setPlugin] = useState<InstrumentPlugin>();
  const [busy, setBusy] = useState(false);
  const [modeDraft, setModeDraft] = useState<string>();
  const [feedback, setFeedback] = useState<{ tone: 'info' | 'error'; text: string }>();
  const readOperatingSettings = useCallback(
    () => client.getOperatingSettings(credential),
    [client, credential],
  );
  const saveOperatingSettings = useCallback(
    (revision: number, settings: Parameters<typeof client.setOperatingSettings>[2]) =>
      client.setOperatingSettings(credential, revision, settings),
    [client, credential],
  );
  const report = useCallback(
    (cause: unknown) => {
      if (cause instanceof BenchCoreError && cause.failure === 'unauthorized')
        onSessionEnded();
      setFeedback({
        tone: 'error',
        text: `${t('instrument.error')} ${cause instanceof BenchCoreError ? cause.code || cause.failure : t('camera.unavailable')}`,
      });
    },
    [onSessionEnded, t],
  );
  const refresh = useCallback(async () => {
    const inventory = await client.listPlugins(credential);
    setPlugin(inventory.find((item) => item.id === pluginId));
  }, [client, credential, pluginId]);
  useEffect(() => {
    let active = true;
    const load = () =>
      client
        .listPlugins(credential)
        .then((items) => {
          if (active) setPlugin(items.find((item) => item.id === pluginId));
        })
        .catch((cause) => {
          if (active) report(cause);
        });
    void load();
    const timer = window.setInterval(() => {
      void load();
    }, 10000);
    return () => {
      active = false;
      window.clearInterval(timer);
    };
  }, [client, credential, report, pluginId]);
  async function run(action: () => Promise<unknown>, success: string) {
    setBusy(true);
    setFeedback(undefined);
    try {
      await action();
      await refresh();
      onPluginsChanged?.();
      setFeedback({ tone: 'info', text: success });
    } catch (cause) {
      report(cause);
    } finally {
      setBusy(false);
    }
  }
  const connectedCameras =
    plugin?.devices.filter((device) => device.presence === 'present') ?? [];
  const failedCameras = connectedCameras.filter(
    (device) => device.configured && device.enabled && device.readiness === 'failed',
  );
  return (
    <div className="page">
      <header className="page-head camera-plugin-head">
        <div className="page-title-actions">
          <h2>{t('camera.pluginTitle')}</h2>
          {onOpenActivity && (
            <button type="button" onClick={onOpenActivity}>
              {t('network.activity.open')}
            </button>
          )}
        </div>
        <div className="form-actions camera-plugin-update">
          <span>
            {t('camera.currentVersion')}: {plugin?.version || '—'}
          </span>
          <button
            type="button"
            disabled={busy || !plugin?.update_available}
            title={
              plugin?.update_available ? plugin.available_version : t('camera.noUpdate')
            }
            onClick={() =>
              void run(
                () => client.pluginAction(credential, pluginId, 'upgrade'),
                t('camera.updated'),
              )
            }
          >
            {t('camera.updatePlugin')}
          </button>
          <HelpPopover name={t('camera.updatePlugin')}>
            {t('platform.help.updatePlugin')}
          </HelpPopover>
        </div>
      </header>
      <div className="page-body instrument-workspace">
        <section
          className="instrument-card camera-plugin-panel"
          aria-label={t('camera.pluginTitle')}
        >
          <div className="camera-plugin-controls">
            <button
              type="button"
              disabled={busy || !plugin?.installed || plugin.enabled}
              onClick={() =>
                void run(
                  () =>
                    client.pluginAction(
                      credential,
                      pluginId,
                      'enable',
                      modeDraft ?? plugin?.mode,
                    ),
                  t('camera.pluginEnabled'),
                )
              }
            >
              {t('instrument.enable')}
            </button>
            <button
              type="button"
              disabled={busy || !plugin?.enabled}
              onClick={() =>
                void run(
                  () => client.pluginAction(credential, pluginId, 'disable'),
                  t('camera.pluginDisabled'),
                )
              }
            >
              {t('instrument.disable')}
            </button>
            <div className="field camera-approval-policy">
              <span>
                <label htmlFor={`${pluginId}-approval-policy`}>
                  {t('instrument.policy')}
                </label>
                <HelpPopover name={t('instrument.policy')}>
                  {t('platform.help.approvalPolicy')}
                </HelpPopover>
              </span>
              <select
                id={`${pluginId}-approval-policy`}
                value={modeDraft ?? plugin?.mode ?? 'ask'}
                disabled={busy || !plugin?.installed}
                onChange={(event) => {
                  const mode = event.target.value;
                  if (!plugin?.enabled) setModeDraft(mode);
                  else {
                    setModeDraft(undefined);
                    void run(
                      () => client.pluginAction(credential, pluginId, 'enable', mode),
                      t('camera.policySaved'),
                    );
                  }
                }}
              >
                <option value="ask">{t('instrument.ask')}</option>
                <option value="auto">{t('instrument.auto')}</option>
              </select>
            </div>
            <button
              type="button"
              disabled={busy || !plugin?.enabled}
              onClick={() =>
                void run(
                  () => client.discoverInstruments(credential, pluginId),
                  t('camera.discoveryDone'),
                )
              }
            >
              {t('camera.discover')}
            </button>
            <HelpPopover name={t('camera.discover')}>
              {t('platform.help.discoverPlugin')}
            </HelpPopover>
          </div>
          {feedback && (
            <MessageBanner tone={feedback.tone}>{feedback.text}</MessageBanner>
          )}
          {plugin?.error && (
            <MessageBanner tone="warning">{plugin.error}</MessageBanner>
          )}
          {failedCameras.length > 0 && (
            <MessageBanner tone="warning">
              {t('camera.needsAttention', {
                names: failedCameras.map((device) => device.display_name).join(', '),
              })}
            </MessageBanner>
          )}
          {!plugin?.enabled && <p>{t('camera.enableToDiscover')}</p>}
          {plugin?.enabled && !connectedCameras.length && <p>{t('camera.none')}</p>}
          {connectedCameras.map((device) => (
            <CameraConfigurationRow
              key={device.id}
              pluginId={pluginId}
              device={device}
              client={client}
              credential={credential}
              busy={busy || !plugin?.enabled}
              run={run}
            />
          ))}
        </section>
        {session.permissions.includes('platform.settings.manage') && (
          <OperatingSettingsSection
            scope={pluginId}
            read={readOperatingSettings}
            save={saveOperatingSettings}
          />
        )}
      </div>
    </div>
  );
}

function CameraConfigurationRow({
  pluginId,
  device,
  client,
  credential,
  busy,
  run,
}: {
  pluginId: 'camera' | 'serial';
  device: InstrumentDevice;
  client: BenchCoreClient;
  credential: Credential;
  busy: boolean;
  run: (action: () => Promise<unknown>, success: string) => Promise<void>;
}) {
  const translate = useTranslate();
  const t = (key: Parameters<typeof translate>[0]) =>
    translate(pluginId === 'serial' ? serialAdminKey(key) : key);
  const [draft, setDraft] = useState<string>();
  const name = draft ?? device.display_name;
  return (
    <div className="camera-configuration-row">
      <strong>{device.id}</strong>
      <label className="field">
        <span>{t('camera.userName')}</span>
        <input
          value={name}
          maxLength={64}
          disabled={busy}
          onChange={(event) => setDraft(event.target.value)}
        />
      </label>
      <div className="form-actions">
        <button
          type="button"
          disabled={
            busy ||
            !name.trim() ||
            !client.configureInstrumentDevice ||
            (device.configured && name.trim() === device.display_name)
          }
          onClick={() =>
            void run(async () => {
              await client.configureInstrumentDevice!(
                credential,
                pluginId,
                device.id,
                device.revision ?? 0,
                name.trim(),
                device.purpose ?? '',
                pluginId === 'serial' ? {} : (device.settings ?? {}),
              );
              setDraft(undefined);
            }, t('camera.configurationSaved'))
          }
        >
          {t('camera.saveConfiguration')}
        </button>
        <button
          type="button"
          disabled={
            busy ||
            !device.configured ||
            device.enabled ||
            !client.instrumentDeviceAction
          }
          onClick={() =>
            void run(
              () =>
                client.instrumentDeviceAction!(
                  credential,
                  pluginId,
                  device.id,
                  'enable',
                  device.revision ?? 0,
                ),
              t('camera.deviceEnabled'),
            )
          }
        >
          {t('instrument.enable')}
        </button>
        <button
          type="button"
          disabled={busy || !device.enabled || !client.instrumentDeviceAction}
          onClick={() =>
            void run(
              () =>
                client.instrumentDeviceAction!(
                  credential,
                  pluginId,
                  device.id,
                  'disable',
                  device.revision ?? 0,
                ),
              t('camera.deviceDisabled'),
            )
          }
        >
          {t('instrument.disable')}
        </button>
      </div>
    </div>
  );
}

function serialAdminKey(
  key: Parameters<ReturnType<typeof useTranslate>>[0],
): Parameters<ReturnType<typeof useTranslate>>[0] {
  const aliases: Partial<
    Record<
      Parameters<ReturnType<typeof useTranslate>>[0],
      Parameters<ReturnType<typeof useTranslate>>[0]
    >
  > = {
    'camera.adminOnly': 'sw.adminOnly',
    'instrument.ask': 'sw.askSessions',
    'camera.needsAttention': 'sw.needsAttention',
    'camera.pluginTitle': 'sw.pluginTitle',
    'camera.discover': 'sw.discover',
    'camera.none': 'sw.none',
    'camera.enableToDiscover': 'sw.enableToDiscover',
    'camera.configurationSaved': 'sw.saved',
    'camera.saveConfiguration': 'sw.saveConfiguration',
    'camera.pluginEnabled': 'sw.enabled',
    'camera.pluginDisabled': 'sw.disabled',
    'camera.deviceEnabled': 'sw.deviceEnabled',
    'camera.deviceDisabled': 'sw.deviceDisabled',
    'camera.discoveryDone': 'sw.discoveryDone',
    'camera.updated': 'sw.updated',
  };
  return aliases[key] ?? key;
}
