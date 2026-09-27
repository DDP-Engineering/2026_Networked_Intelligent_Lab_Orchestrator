import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { BenchCoreError } from '../../api/client';
import { serialAPI, type SerialLog, type SerialRetention } from '../../api/serial';
import { useTranslate } from '../../i18n/languages';
import { MessageBanner } from '../MessageBanner';
import { Modal } from '../Modal';
import type { SerialPageProps } from './SerialWorkspacePage';
import { SerialTerminal } from './SerialTerminal';
import { TerminalBuffer } from './serialBuffer';

const MIB = 1024 * 1024;
export function SerialSessionsPage({
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
  const [logs, setLogs] = useState<SerialLog[]>([]);
  const [offset, setOffset] = useState(0);
  const [total, setTotal] = useState(0);
  const [retention, setRetention] = useState<SerialRetention>();
  const [draft, setDraft] = useState<SerialRetention['limits']>();
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const [selected, setSelected] = useState<SerialLog>();
  const [remove, setRemove] = useState<SerialLog[]>();
  const [checked, setChecked] = useState<string[]>([]);
  const [format, setFormat] = useState('jsonl');
  const [rows, setRows] = useState<TerminalBuffer['rows']>([]);
  const [gap, setGap] = useState(false);
  const [cursor, setCursor] = useState(0);
  const [more, setMore] = useState(false);
  const buffer = useRef(new TerminalBuffer());
  const viewerRequest = useRef(0);
  const canManage =
    session.role === 'administrator' && session.permissions.includes('plugins.manage');
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
  const refresh = useCallback(async () => {
    const [inventory, limits] = await Promise.all([
      api.logs(device.id, offset),
      api.retention(device.id),
    ]);
    setLogs(inventory.logs ?? []);
    setTotal(inventory.total);
    setRetention(limits);
  }, [api, device.id, offset]);
  useEffect(() => {
    let active = true;
    void Promise.all([api.logs(device.id, offset), api.retention(device.id)])
      .then(([inventory, limits]) => {
        if (active) {
          setLogs(inventory.logs ?? []);
          setTotal(inventory.total);
          setRetention(limits);
        }
      })
      .catch((cause) => {
        if (active) report(cause);
      });
    return () => {
      active = false;
    };
  }, [api, device.id, offset, report]);
  async function run(action: () => Promise<unknown>) {
    setBusy(true);
    setError('');
    try {
      await action();
      await refresh();
    } catch (cause) {
      report(cause);
    } finally {
      setBusy(false);
    }
  }
  async function view(log: SerialLog, append = false) {
    const request = ++viewerRequest.current;
    const result = await api.events(log.id, append ? cursor : 0);
    if (request !== viewerRequest.current) return;
    if (!append) buffer.current.clear();
    buffer.current.add(result.events ?? [], result.gap);
    setRows([...buffer.current.rows]);
    setGap(buffer.current.dropped);
    setCursor(result.next_cursor);
    setMore((result.events ?? []).length > 0);
    setSelected(log);
  }
  function download(log: SerialLog) {
    const link = document.createElement('a');
    link.href = api.downloadURL(log.id, format);
    link.download = '';
    // The normal browser session cookie is sent to the same-origin attachment route.
    link.click();
  }
  const limits = draft ?? retention?.limits;
  return (
    <div className="page serial-workspace">
      <header className="page-head">
        <h2>
          {device.display_name} · {t('sw.sessions')}
        </h2>
        <button disabled={busy} onClick={() => void run(refresh)}>
          {t('instrument.refresh')}
        </button>
      </header>
      <div className="page-body instrument-workspace">
        {error && (
          <MessageBanner tone="error">
            {t('instrument.error')} {error}
          </MessageBanner>
        )}
        <section className="instrument-card">
          <h3>{t('cw.storage')}</h3>
          <p>
            {((retention?.usage_bytes ?? 0) / MIB).toFixed(1)} MiB /{' '}
            {((retention?.limits.total_bytes ?? 0) / MIB).toFixed(1)} MiB
          </p>
          {canManage && limits && (
            <details>
              <summary>{t('sw.retention')}</summary>
              <form
                onSubmit={(e) => {
                  e.preventDefault();
                  void run(async () => {
                    setRetention(
                      await api.saveRetention(device.id, retention!.revision, limits),
                    );
                    setDraft(undefined);
                  });
                }}
              >
                <div className="serial-settings">
                  {(['files', 'total_bytes', 'file_bytes'] as const).map((key) => (
                    <label key={key} className="field">
                      <span>
                        {t(
                          key === 'files'
                            ? 'sw.files'
                            : key === 'total_bytes'
                              ? 'sw.total'
                              : 'sw.fileSize',
                        )}
                      </span>
                      <input
                        required
                        type="number"
                        min={key === 'files' ? 1 : key === 'file_bytes' ? 0.0625 : 1}
                        max={
                          key === 'files'
                            ? 10000
                            : key === 'file_bytes'
                              ? 1024
                              : 1048576
                        }
                        step={key === 'files' ? 1 : 0.0625}
                        value={limits[key] / (key === 'files' ? 1 : MIB)}
                        onChange={(e) =>
                          setDraft({
                            ...limits,
                            [key]: +e.target.value * (key === 'files' ? 1 : MIB),
                          })
                        }
                      />
                    </label>
                  ))}
                </div>
                <p>{t('sw.retentionHint')}</p>
                <button
                  disabled={busy || !draft || limits.file_bytes > limits.total_bytes}
                >
                  {t('sw.saveLimits')}
                </button>
              </form>
            </details>
          )}
        </section>
        <section className="instrument-card">
          <div className="form-actions serial-toolbar">
            <h3>{t('sw.sessions')}</h3>
            <label>
              <span>{t('sw.export')}</span>
              <select value={format} onChange={(e) => setFormat(e.target.value)}>
                <option value="jsonl">JSONL</option>
                <option value="text">UTF-8</option>
                <option value="rx">RX (raw)</option>
                <option value="tx">TX (raw)</option>
              </select>
            </label>
          </div>
          {!logs.length && <p>{t('sw.noLogs')}</p>}
          {!!logs.length && (
            <button
              disabled={
                busy ||
                !checked.length ||
                !session.permissions.includes('bench.execute')
              }
              onClick={() =>
                setRemove(
                  logs.filter(
                    (log) => checked.includes(log.id) && log.status !== 'active',
                  ),
                )
              }
            >
              {t('cw.deleteSelected', { count: checked.length })}
            </button>
          )}
          <div className="serial-logs">
            {logs.map((log) => (
              <article className="serial-log" key={log.id}>
                <div>
                  <label>
                    <input
                      type="checkbox"
                      aria-label={`${t('sw.sessions')} ${new Date(log.started_at).toLocaleString()}`}
                      checked={checked.includes(log.id)}
                      disabled={
                        busy ||
                        log.status === 'active' ||
                        !session.permissions.includes('bench.execute')
                      }
                      onChange={(e) =>
                        setChecked(
                          e.target.checked
                            ? [...checked, log.id]
                            : checked.filter((id) => id !== log.id),
                        )
                      }
                    />
                    <strong>{new Date(log.started_at).toLocaleString()}</strong>
                  </label>
                  <p>
                    {log.status} · RX {log.rx_bytes} B · TX {log.tx_bytes} B ·{' '}
                    {(log.size_bytes / MIB).toFixed(2)} MiB
                  </p>
                  {log.ended_at && (
                    <p>
                      {new Date(log.ended_at).toLocaleString()} ·{' '}
                      {Math.max(
                        0,
                        Math.round(
                          (Date.parse(log.ended_at) - Date.parse(log.started_at)) /
                            1000,
                        ),
                      )}{' '}
                      s
                    </p>
                  )}
                </div>
                <div className="form-actions">
                  <button disabled={busy} onClick={() => void run(() => view(log))}>
                    {t('sw.preview')}
                  </button>
                  <button
                    disabled={busy || credential.kind !== 'session'}
                    onClick={() => download(log)}
                  >
                    {t('sw.export')}
                  </button>
                  <button
                    disabled={
                      busy ||
                      log.status === 'active' ||
                      !session.permissions.includes('bench.execute')
                    }
                    onClick={() => setRemove([log])}
                  >
                    {t('sw.delete')}
                  </button>
                </div>
              </article>
            ))}
          </div>
          {total > 0 && (
            <div className="form-actions">
              <button
                disabled={busy || offset === 0}
                onClick={() => {
                  setChecked([]);
                  setOffset(Math.max(0, offset - 100));
                }}
              >
                {t('sw.previous')}
              </button>
              <span>
                {Math.min(offset + 1, total)}–{Math.min(offset + logs.length, total)} /{' '}
                {total}
              </span>
              <button
                disabled={busy || offset + logs.length >= total}
                onClick={() => {
                  setChecked([]);
                  setOffset(offset + 100);
                }}
              >
                {t('sw.next')}
              </button>
            </div>
          )}
        </section>
        {selected && (
          <section className="instrument-card">
            <div className="form-actions">
              <h3>{new Date(selected.started_at).toLocaleString()}</h3>
              <button
                onClick={() => {
                  viewerRequest.current++;
                  setSelected(undefined);
                }}
              >
                {t('sw.close')}
              </button>
            </div>
            <SerialTerminal
              rows={rows}
              gap={gap}
              onClear={() => {
                buffer.current.clear();
                setRows([]);
                setGap(false);
              }}
            />
            <button
              disabled={busy || !more}
              onClick={() => void run(() => view(selected, true))}
            >
              {t('sw.more')}
            </button>
          </section>
        )}
        {remove && (
          <Modal title={t('sw.confirmDelete')} onDismiss={() => setRemove(undefined)}>
            <ul>
              {remove.map((log) => (
                <li key={log.id}>{new Date(log.started_at).toLocaleString()}</li>
              ))}
            </ul>
            <div className="form-actions">
              <button
                disabled={busy}
                onClick={() =>
                  void run(async () => {
                    for (const log of remove) {
                      await api.remove(log.id);
                      setChecked((old) => old.filter((id) => id !== log.id));
                      if (selected?.id === log.id) setSelected(undefined);
                    }
                    setRemove(undefined);
                  })
                }
              >
                {t('sw.delete')}
              </button>
              <button onClick={() => setRemove(undefined)}>{t('sw.cancel')}</button>
            </div>
          </Modal>
        )}
      </div>
    </div>
  );
}
