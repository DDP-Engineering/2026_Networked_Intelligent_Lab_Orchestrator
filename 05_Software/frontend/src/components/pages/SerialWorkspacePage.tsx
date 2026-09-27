import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
  BenchCoreError,
  type BenchCoreClient,
  type Credential,
} from '../../api/client';
import type { BenchCoreSession } from '../../api/contract';
import {
  isJobTerminal,
  type InstrumentDevice,
  type InstrumentJob,
} from '../../api/bench';
import {
  defaultSerialSettings,
  serialAPI,
  type SerialSettings,
  type SerialStatus,
  type SerialSend,
} from '../../api/serial';
import { useTranslate } from '../../i18n/languages';
import { readPreference, writePreference } from '../../preferences/store';
import { MessageBanner } from '../MessageBanner';
import { TerminalBuffer, validSerialSend } from './serialBuffer';
import { SerialTerminal } from './SerialTerminal';
import { SerialSessionsPage } from './SerialSessionsPage';
import '../../styles/serial-workspace.css';

export interface SerialPageProps {
  client: BenchCoreClient;
  credential: Credential;
  session: BenchCoreSession;
  device: InstrumentDevice;
  onSessionEnded: () => void;
}
function savedSettings(): SerialSettings {
  try {
    const saved = JSON.parse(
      readPreference('serialSettings') ?? 'null',
    ) as SerialSettings | null;
    if (
      saved &&
      Number.isInteger(saved.baud_rate) &&
      saved.baud_rate >= 50 &&
      saved.baud_rate <= 4000000 &&
      [5, 6, 7, 8].includes(saved.data_bits) &&
      ['none', 'odd', 'even', 'mark', 'space'].includes(saved.parity) &&
      [1, 1.5, 2].includes(saved.stop_bits) &&
      ['none', 'rtscts', 'xonxoff'].includes(saved.flow_control) &&
      typeof saved.rts === 'boolean' &&
      typeof saved.dtr === 'boolean'
    )
      return saved;
  } catch {
    /* A malformed display preference falls back to defaults. */
  }
  return { ...defaultSerialSettings };
}

export function SerialWorkspacePage(
  props: SerialPageProps & { view: 'live' | 'sessions' },
) {
  return props.view === 'sessions' ? (
    <SerialSessionsPage {...props} />
  ) : (
    <SerialLivePage {...props} />
  );
}

function SerialLivePage({
  client,
  credential,
  session,
  device,
  onSessionEnded,
}: SerialPageProps) {
  const t = useTranslate();
  const api = useMemo(
    () => serialAPI(client.baseUrl, credential),
    [client.baseUrl, credential],
  );
  const [settings, setSettings] = useState(savedSettings);
  const [status, setStatus] = useState<SerialStatus>();
  const [job, setJob] = useState<InstrumentJob>();
  const jobRef = useRef<InstrumentJob | undefined>(undefined);
  const [error, setError] = useState('');
  const [info, setInfo] = useState('');
  const [warning, setWarning] = useState('');
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState('');
  const [encoding, setEncoding] = useState<SerialSend['encoding']>('utf8');
  const [ending, setEnding] = useState<SerialSend['ending']>('none');
  const [echo, setEcho] = useState(true);
  const [history, setHistory] = useState<string[]>([]);
  const [rows, setRows] = useState<TerminalBuffer['rows']>([]);
  const [gap, setGap] = useState(false);
  const [fill, setFill] = useState(false);
  const [fullscreen, setFullscreen] = useState(false);
  useEffect(() => {
    const changed = () => setFullscreen(document.fullscreenElement === panel.current);
    document.addEventListener('fullscreenchange', changed);
    return () => document.removeEventListener('fullscreenchange', changed);
  }, []);
  const panel = useRef<HTMLDivElement>(null);
  const buffer = useRef(new TerminalBuffer());
  const connectAttempt = useRef<{ signature: string; key: string } | undefined>(
    undefined,
  );
  const sendAttempt = useRef<SerialSend | undefined>(undefined);
  const canExecute = session.permissions.includes('bench.execute');
  const canControl = canExecute && !!status?.connected && !!status.can_control;
  const report = useCallback(
    (cause: unknown) => {
      if (cause instanceof BenchCoreError && cause.failure === 'unauthorized')
        onSessionEnded();
      setError(
        cause instanceof BenchCoreError
          ? cause.code || cause.failure
          : cause instanceof Error
            ? cause.message
            : 'unavailable',
      );
    },
    [onSessionEnded],
  );
  useEffect(() => {
    const controller = new AbortController();
    let connection = '';
    let cursor = 0;
    let timer: number;
    async function poll() {
      try {
        const next = await api.status(device.id, controller.signal);
        if (controller.signal.aborted) return;
        setStatus(next);
        if (jobRef.current && !isJobTerminal(jobRef.current)) {
          const updated = await client.getInstrumentJob(
            credential,
            jobRef.current.transaction_id,
          );
          if (controller.signal.aborted) return;
          jobRef.current = updated;
          setJob(updated);
          if (updated.error) setError(updated.error);
        }
        if (next.connected && next.connection_id) {
          if (connection !== next.connection_id) {
            connection = next.connection_id;
            cursor = 0;
            buffer.current.clear();
          }
          let data;
          try {
            data = await api.read(device.id, connection, cursor, controller.signal);
          } catch (cause) {
            if (controller.signal.aborted) return;
            if (
              cause instanceof BenchCoreError &&
              (cause.failure === 'unauthorized' || cause.failure === 'forbidden')
            )
              throw cause;
            // A port may close while its last read is in flight. Reconcile
            // against current state without concealing failures on a live port.
            const current = await api.status(device.id, controller.signal);
            if (controller.signal.aborted) return;
            setStatus(current);
            if (!current.connected || current.connection_id !== connection) return;
            throw cause;
          }
          if (controller.signal.aborted) return;
          cursor = data.next_cursor;
          buffer.current.add(data.events ?? [], data.gap);
          setRows([...buffer.current.rows]);
          setGap(buffer.current.dropped);
        }
      } catch (cause) {
        if (!controller.signal.aborted) report(cause);
      } finally {
        if (!controller.signal.aborted)
          timer = window.setTimeout(() => {
            void poll();
          }, 500);
      }
    }
    void poll();
    return () => {
      controller.abort();
      window.clearTimeout(timer);
    };
  }, [api, device.id, client, credential, report]);
  async function run(action: () => Promise<unknown>) {
    setBusy(true);
    setError('');
    setInfo('');
    setWarning('');
    try {
      await action();
      setStatus(await api.status(device.id));
    } catch (cause) {
      report(cause);
    } finally {
      setBusy(false);
    }
  }
  function updateSettings(next: SerialSettings) {
    setSettings(next);
    writePreference('serialSettings', JSON.stringify(next));
  }
  async function connect() {
    const signature = JSON.stringify({ revision: device.revision, settings });
    if (connectAttempt.current?.signature !== signature)
      connectAttempt.current = { signature, key: crypto.randomUUID() };
    const next = await api.connect(
      device.id,
      device.revision ?? 0,
      settings,
      connectAttempt.current.key,
    );
    connectAttempt.current = undefined;
    jobRef.current = next;
    setJob(next);
  }
  async function send() {
    if (!validSerialSend(message, encoding, ending)) {
      setError(t('sw.sendInvalid'));
      return;
    }
    const input = {
      connection_id: status!.connection_id!,
      encoding,
      data: message,
      ending,
    };
    const previous = sendAttempt.current;
    if (
      !previous ||
      previous.connection_id !== input.connection_id ||
      previous.encoding !== encoding ||
      previous.data !== message ||
      previous.ending !== ending
    )
      sendAttempt.current = { ...input, idempotency_key: crypto.randomUUID() };
    const result = await api.send(device.id, sendAttempt.current!);
    const feedback = `${t('sw.sendResult')}: ${result.status}, ${result.bytes_written} B${result.error ? ` · ${result.error}` : ''}`;
    if (result.status === 'sent') setInfo(feedback);
    else if (result.status === 'failed') setError(feedback);
    else setWarning(feedback);
    if (result.status === 'sent') {
      sendAttempt.current = undefined;
      setHistory((old) =>
        [message, ...old.filter((item) => item !== message)].slice(0, 20),
      );
      setMessage('');
    }
  }
  const pending = !!job && !isJobTerminal(job);
  const awaiting = job?.state === 'pending_approval';
  const visibleSettings =
    status?.connected && status.settings ? status.settings : settings;
  return (
    <div
      ref={panel}
      className={`page serial-workspace ${fill || fullscreen ? 'serial-fill' : ''}`}
    >
      <header className="page-head serial-toolbar">
        <h2>
          {device.display_name} · {t('sw.realtime')}
        </h2>
        <span>{t(status?.connected ? 'sw.connected' : 'sw.disconnected')}</span>
        <div className="workspace-actions">
          <button
            disabled={
              busy ||
              !canExecute ||
              !!status?.connected ||
              pending ||
              device.presence !== 'present' ||
              device.readiness !== 'ready'
            }
            onClick={() => void run(connect)}
          >
            {t('sw.connect')}
          </button>
          <button
            disabled={
              busy ||
              !status?.connected ||
              !(canControl || session.permissions.includes('plugins.manage'))
            }
            onClick={() =>
              void run(() => api.disconnect(device.id, status!.connection_id!))
            }
          >
            {t('sw.disconnect')}
          </button>
          <button
            disabled={busy || !canControl}
            onClick={() =>
              void run(() =>
                api.logging(
                  device.id,
                  status!.connection_id!,
                  status?.logging ? 'stop' : 'start',
                ),
              )
            }
          >
            {t(status?.logging ? 'sw.stopLog' : 'sw.startLog')}
          </button>
          <button onClick={() => setFill(!fill)}>
            {t(fill ? 'cw.normal' : 'cw.fill')}
          </button>
          <button
            onClick={() =>
              void (
                document.fullscreenElement
                  ? document.exitFullscreen()
                  : panel.current!.requestFullscreen()
              ).catch(report)
            }
          >
            {t(fullscreen ? 'sw.exitFullscreen' : 'cw.fullscreen')}
          </button>
        </div>
      </header>
      <div className="page-body instrument-workspace serial-body">
        {error && (
          <MessageBanner tone="error">
            {t('instrument.error')} {error}
          </MessageBanner>
        )}
        {info && <MessageBanner tone="info">{info}</MessageBanner>}
        {warning && <MessageBanner tone="warning">{warning}</MessageBanner>}
        {status?.warning && status.warning !== 'disconnected' && (
          <MessageBanner tone="warning">{status.warning}</MessageBanner>
        )}
        {pending && (
          <MessageBanner tone="info" presentation="inline">
            {awaiting ? t('sw.waiting') : job?.state}
            {awaiting &&
              credential.kind === 'session' &&
              session.permissions.includes('bench.approve') && (
                <button
                  disabled={busy}
                  onClick={() =>
                    void run(async () => {
                      const next = await client.instrumentJobAction(
                        credential,
                        job!.transaction_id,
                        'approve',
                      );
                      jobRef.current = next;
                      setJob(next);
                    })
                  }
                >
                  {t('sw.approve')}
                </button>
              )}
            {canExecute && (
              <button
                disabled={busy}
                onClick={() =>
                  void run(async () => {
                    const next = await client.instrumentJobAction(
                      credential,
                      job!.transaction_id,
                      'cancel',
                    );
                    jobRef.current = next;
                    setJob(next);
                  })
                }
              >
                {t('sw.cancel')}
              </button>
            )}
          </MessageBanner>
        )}
        <SerialTerminal
          workspace
          status={
            <div className="serial-status">
              <span>
                RX: {status?.rx_bytes ?? 0} B · TX: {status?.tx_bytes ?? 0} B{' '}
                {status?.logging ? ` · ${t('cw.recording')}` : ''}
              </span>
              {status?.connected && (
                <details className="serial-connection-details">
                  <summary>{t('message.connectionDetails')}</summary>
                  {status?.connected && status.owner_id && (
                    <span>
                      {t('sw.owner')}: {status.owner_id}
                    </span>
                  )}
                  {status?.connected && status.expires_at && (
                    <span>
                      {t('sw.expires')}: {new Date(status.expires_at).toLocaleString()}
                    </span>
                  )}
                </details>
              )}
            </div>
          }
          rows={echo ? rows : rows.filter((row) => row.direction !== 'tx')}
          gap={gap}
          onClear={() => {
            buffer.current.clear();
            setRows([]);
            setGap(false);
          }}
        >
          <form
            className="serial-send form-actions"
            onSubmit={(e) => {
              e.preventDefault();
              void run(send);
            }}
          >
            <label className="field serial-message">
              <span>{t('sw.message')}</span>
              <textarea
                rows={2}
                value={message}
                maxLength={12288}
                disabled={!canControl || busy}
                onChange={(e) => setMessage(e.target.value)}
              />
            </label>
            <label className="field">
              <span>{t('sw.encoding')}</span>
              <select
                value={encoding}
                onChange={(e) => setEncoding(e.target.value as SerialSend['encoding'])}
              >
                <option value="ascii">ASCII</option>
                <option value="utf8">UTF-8</option>
                <option value="hex">HEX</option>
              </select>
            </label>
            <label className="field">
              <span>{t('sw.ending')}</span>
              <select
                value={ending}
                onChange={(e) => setEnding(e.target.value as SerialSend['ending'])}
              >
                <option value="none">{t('sw.noneOption')}</option>
                <option value="cr">CR</option>
                <option value="lf">LF</option>
                <option value="crlf">CR-LF</option>
              </select>
            </label>
            <button
              disabled={
                !canControl || busy || !validSerialSend(message, encoding, ending)
              }
            >
              {t('sw.send')}
            </button>
          </form>
        </SerialTerminal>
        <details className="instrument-card serial-settings-section">
          <summary>{t('sw.port')}</summary>
          <fieldset
            disabled={busy || !!status?.connected || pending}
            className="serial-settings"
          >
            <label className="field">
              <span>{t('sw.baud')}</span>
              <input
                list="serial-baud"
                type="number"
                min={50}
                max={4000000}
                value={visibleSettings.baud_rate}
                onChange={(e) =>
                  updateSettings({ ...settings, baud_rate: +e.target.value })
                }
              />
              <datalist id="serial-baud">
                {[
                  1200, 2400, 4800, 9600, 19200, 38400, 57600, 115200, 230400, 460800,
                  921600,
                ].map((rate) => (
                  <option key={rate} value={rate} />
                ))}
              </datalist>
            </label>
            <label className="field">
              <span>{t('sw.bits')}</span>
              <select
                value={visibleSettings.data_bits}
                onChange={(e) =>
                  updateSettings({ ...settings, data_bits: +e.target.value })
                }
              >
                {[5, 6, 7, 8].map((value) => (
                  <option key={value}>{value}</option>
                ))}
              </select>
            </label>
            <label className="field">
              <span>{t('sw.parity')}</span>
              <select
                value={visibleSettings.parity}
                onChange={(e) =>
                  updateSettings({ ...settings, parity: e.target.value })
                }
              >
                {(['none', 'odd', 'even', 'mark', 'space'] as const).map((value) => (
                  <option key={value} value={value}>
                    {t(value === 'none' ? 'sw.noneOption' : `sw.${value}`)}
                  </option>
                ))}
              </select>
            </label>
            <label className="field">
              <span>{t('sw.stopBits')}</span>
              <select
                value={visibleSettings.stop_bits}
                onChange={(e) =>
                  updateSettings({ ...settings, stop_bits: +e.target.value })
                }
              >
                {[1, 1.5, 2].map((value) => (
                  <option key={value}>{value}</option>
                ))}
              </select>
            </label>
            <label className="field">
              <span>{t('sw.flow')}</span>
              <select
                value={visibleSettings.flow_control}
                onChange={(e) =>
                  updateSettings({ ...settings, flow_control: e.target.value })
                }
              >
                <option value="none">{t('sw.noneOption')}</option>
                <option value="rtscts">RTS/CTS</option>
                <option value="xonxoff">XON/XOFF</option>
              </select>
            </label>
            {(['rts', 'dtr'] as const).map((line) => (
              <label key={line}>
                <input
                  type="checkbox"
                  checked={visibleSettings[line]}
                  onChange={(e) =>
                    updateSettings({ ...settings, [line]: e.target.checked })
                  }
                />
                {line.toUpperCase()}
              </label>
            ))}
          </fieldset>
          <p>{t('sw.initialLines')}</p>
        </details>
        <details className="instrument-card serial-settings-section">
          <summary>{t('sw.transmit')}</summary>
          <div className="form-actions">
            <label>
              <input
                type="checkbox"
                checked={echo}
                onChange={(e) => setEcho(e.target.checked)}
              />
              {t('sw.echo')}
            </label>
            <label className="field">
              <span>{t('sw.history')}</span>
              <select value="" onChange={(e) => setMessage(e.target.value)}>
                <option value="">—</option>
                {history.map((item, index) => (
                  <option key={index} value={item}>
                    {item.slice(0, 80)}
                  </option>
                ))}
              </select>
            </label>
            {['rts', 'dtr', 'cts', 'dsr', 'ri', 'cd'].map((line) => (
              <label key={line}>
                {line.toUpperCase()}:{' '}
                {typeof status?.signals?.[line] !== 'boolean' ? (
                  t('sw.unknown')
                ) : line === 'rts' || line === 'dtr' ? (
                  <input
                    aria-label={line.toUpperCase()}
                    type="checkbox"
                    checked={!!status.signals[line]}
                    disabled={
                      !canControl ||
                      busy ||
                      (line === 'rts' && status.settings?.flow_control === 'rtscts')
                    }
                    onChange={(e) =>
                      void run(() =>
                        api.signals(device.id, status.connection_id!, {
                          [line]: e.target.checked,
                        }),
                      )
                    }
                  />
                ) : status.signals[line] ? (
                  '1'
                ) : (
                  '0'
                )}
              </label>
            ))}
          </div>
        </details>
      </div>
    </div>
  );
}
