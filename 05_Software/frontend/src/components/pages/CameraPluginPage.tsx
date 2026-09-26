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

interface Props {
  client: BenchCoreClient;
  credential: Credential;
  session: BenchCoreSession;
  onSessionEnded: () => void;
  onPluginsChanged?: () => void;
  onOpenActivity?: () => void;
}

/** Administrator-only configuration; capture remains in the instrument workspace. */
export function CameraPluginPage(props: Props) {
  const t = useTranslate();
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
  onSessionEnded,
  onPluginsChanged,
  onOpenActivity,
}: Props) {
  const t = useTranslate();
  const [plugin, setPlugin] = useState<InstrumentPlugin>();
  const [busy, setBusy] = useState(false);
  const [modeDraft, setModeDraft] = useState<string>();
  const [feedback, setFeedback] = useState<{ tone: 'info' | 'error'; text: string }>();
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
    setPlugin(inventory.find((item) => item.id === 'camera'));
  }, [client, credential]);
  useEffect(() => {
    let active = true;
    const load = () =>
      client
        .listPlugins(credential)
        .then((items) => {
          if (active) setPlugin(items.find((item) => item.id === 'camera'));
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
  }, [client, credential, report]);
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
                () => client.pluginAction(credential, 'camera', 'upgrade'),
                t('camera.updated'),
              )
            }
          >
            {t('camera.updatePlugin')}
          </button>
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
                      'camera',
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
                  () => client.pluginAction(credential, 'camera', 'disable'),
                  t('camera.pluginDisabled'),
                )
              }
            >
              {t('instrument.disable')}
            </button>
            <label className="field camera-approval-policy">
              <span>{t('instrument.policy')}</span>
              <select
                value={modeDraft ?? plugin?.mode ?? 'ask'}
                disabled={busy || !plugin?.installed}
                onChange={(event) => {
                  const mode = event.target.value;
                  if (!plugin?.enabled) setModeDraft(mode);
                  else {
                    setModeDraft(undefined);
                    void run(
                      () => client.pluginAction(credential, 'camera', 'enable', mode),
                      t('camera.policySaved'),
                    );
                  }
                }}
              >
                <option value="ask">{t('instrument.ask')}</option>
                <option value="auto">{t('instrument.auto')}</option>
              </select>
            </label>
            <button
              type="button"
              disabled={busy || !plugin?.enabled}
              onClick={() =>
                void run(
                  () => client.discoverInstruments(credential, 'camera'),
                  t('camera.discoveryDone'),
                )
              }
            >
              {t('camera.discover')}
            </button>
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
              device={device}
              client={client}
              credential={credential}
              busy={busy || !plugin?.enabled}
              run={run}
            />
          ))}
        </section>
      </div>
    </div>
  );
}

function CameraConfigurationRow({
  device,
  client,
  credential,
  busy,
  run,
}: {
  device: InstrumentDevice;
  client: BenchCoreClient;
  credential: Credential;
  busy: boolean;
  run: (action: () => Promise<unknown>, success: string) => Promise<void>;
}) {
  const t = useTranslate();
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
                'camera',
                device.id,
                device.revision ?? 0,
                name.trim(),
                device.purpose ?? '',
                device.settings ?? {},
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
                  'camera',
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
                  'camera',
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
