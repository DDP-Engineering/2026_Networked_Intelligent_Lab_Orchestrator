import { useCallback, useEffect, useRef, useState } from 'react';
import {
  BenchCoreError,
  type BenchCoreClient,
  type Credential,
} from '../../api/client';
import type { BenchCoreSession } from '../../api/contract';
import {
  isJobTerminal,
  type InstrumentJob,
  type InstrumentPlugin,
  type InstrumentSubmission,
} from '../../api/bench';
import { useTranslate } from '../../i18n/languages';

interface Props {
  client: BenchCoreClient;
  credential: Credential;
  session: BenchCoreSession;
  onSessionEnded: () => void;
}

/** Managed plugins and server-owned transactions. Polling reattaches to durable
 * work after reload; a failed submission keeps its key for an identical retry. */
export function PluginsPage({ client, credential, session, onSessionEnded }: Props) {
  const t = useTranslate();
  const [plugins, setPlugins] = useState<InstrumentPlugin[]>([]);
  const [jobs, setJobs] = useState<InstrumentJob[]>([]);
  const [selected, setSelected] = useState('');
  const [detail, setDetail] = useState<InstrumentJob | null>(null);
  const [device, setDevice] = useState('');
  const [channels, setChannels] = useState(['D0']);
  const [rate, setRate] = useState(1000000);
  const [samples, setSamples] = useState(1024);
  const [timeout, setTimeoutValue] = useState(60);
  const [mode, setMode] = useState('ask');
  const [channel, setChannel] = useState('D0');
  const [baudrate, setBaudrate] = useState(115200);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const attempt = useRef<{ signature: string; key: string } | null>(null);
  const refreshing = useRef(false);
  const mounted = useRef(true);
  const canManage = session.permissions.includes('plugins.manage');
  const canExecute = session.permissions.includes('bench.execute');
  const canApprove =
    credential.kind === 'session' && session.permissions.includes('bench.approve');
  const sigrok = plugins.find((plugin) => plugin.id === 'sigrok');
  const devices = sigrok?.devices ?? [];
  const activeDevice = devices.find((item) => item.id === device) ?? devices[0];
  const current =
    detail?.transaction_id === selected
      ? detail
      : jobs.find((job) => job.transaction_id === selected);
  const capturedChannels = Array.isArray(current?.submission.parameters.channels)
    ? (current.submission.parameters.channels as string[])
    : [];
  const decodeChannel = capturedChannels.includes(channel)
    ? channel
    : (capturedChannels[0] ?? '');

  const report = useCallback(
    (cause: unknown) => {
      if (cause instanceof BenchCoreError && cause.failure === 'unauthorized')
        onSessionEnded();
      if (mounted.current)
        setError(
          cause instanceof BenchCoreError ? cause.code || cause.failure : 'unavailable',
        );
    },
    [onSessionEnded],
  );
  const refresh = useCallback(async () => {
    if (refreshing.current) return;
    refreshing.current = true;
    try {
      const [nextPlugins, nextJobs] = await Promise.all([
        client.listPlugins(credential),
        client.listInstrumentJobs(credential),
      ]);
      const nextDetail = selected
        ? await client.getInstrumentJob(credential, selected)
        : null;
      if (mounted.current) {
        setPlugins(nextPlugins);
        setJobs(nextJobs);
        setDetail(nextDetail);
      }
    } catch (cause) {
      report(cause);
    } finally {
      refreshing.current = false;
    }
  }, [client, credential, report, selected]);
  useEffect(() => {
    mounted.current = true;
    const initial = window.setTimeout(() => {
      void refresh();
    }, 0);
    const timer = window.setInterval(() => {
      void refresh();
    }, 1500);
    return () => {
      mounted.current = false;
      window.clearTimeout(initial);
      window.clearInterval(timer);
    };
  }, [refresh]);
  async function act(action: () => Promise<unknown>) {
    setBusy(true);
    setError('');
    try {
      await action();
      await refresh();
    } catch (cause) {
      report(cause);
    } finally {
      if (mounted.current) setBusy(false);
    }
  }
  async function submit(input: Omit<InstrumentSubmission, 'idempotency_key'>) {
    const signature = JSON.stringify(input);
    if (attempt.current?.signature !== signature)
      attempt.current = { signature, key: crypto.randomUUID() };
    const job = await client.submitInstrumentJob(credential, {
      ...input,
      idempotency_key: attempt.current.key,
    });
    attempt.current = null;
    setSelected(job.transaction_id);
    setDetail(job);
    setJobs((previous) => [
      job,
      ...previous.filter((item) => item.transaction_id !== job.transaction_id),
    ]);
  }
  async function download(id: string) {
    const blob = await client.downloadInstrumentArtifact(credential, id);
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = url;
    link.download = `capture-${id}.sr`;
    link.click();
    window.setTimeout(() => URL.revokeObjectURL(url), 1000);
  }
  return (
    <div className="page">
      <header className="page-head">
        <h2>{t('nav.plugins')}</h2>
        <button
          type="button"
          onClick={() => {
            setError('');
            void refresh();
          }}
        >
          {t('instrument.refresh')}
        </button>
      </header>
      <div className="page-body instrument-workspace">
        {error && (
          <p className="notice notice-unknown" role="alert">
            {t('instrument.error')} <code>{error}</code>
          </p>
        )}
        {plugins.length === 0 && <p>{t('instrument.noPackages')}</p>}
        <section className="instrument-packages" aria-label={t('nav.plugins')}>
          {plugins.map((plugin) => (
            <article className="instrument-card" key={plugin.id}>
              <h3>
                {plugin.display_name} <small>{plugin.version}</small>
              </h3>
              <p>
                {plugin.status} · {plugin.mode}{' '}
                {plugin.simulation && <strong>· {t('instrument.simulation')}</strong>}
              </p>
              {plugin.error && <p role="status">{plugin.error}</p>}
              {canManage && (
                <div className="instrument-actions">
                  {!plugin.installed && (
                    <button
                      disabled={busy}
                      onClick={() =>
                        void act(() =>
                          client.pluginAction(credential, plugin.id, 'install'),
                        )
                      }
                    >
                      {t('instrument.install')}
                    </button>
                  )}
                  {plugin.installed && (
                    <>
                      {!plugin.enabled && (
                        <>
                          <label>
                            {t('instrument.policy')}{' '}
                            <select
                              value={mode}
                              onChange={(event) => setMode(event.target.value)}
                            >
                              <option value="ask">{t('instrument.ask')}</option>
                              <option value="auto">{t('instrument.auto')}</option>
                            </select>
                          </label>
                          <button
                            disabled={busy}
                            onClick={() =>
                              void act(() =>
                                client.pluginAction(
                                  credential,
                                  plugin.id,
                                  'enable',
                                  mode,
                                ),
                              )
                            }
                          >
                            {t('instrument.enable')}
                          </button>
                        </>
                      )}
                      {plugin.enabled && (
                        <>
                          <button
                            disabled={busy}
                            onClick={() =>
                              void act(() =>
                                client.pluginAction(credential, plugin.id, 'disable'),
                              )
                            }
                          >
                            {t('instrument.disable')}
                          </button>
                          <button
                            disabled={busy}
                            onClick={() =>
                              void act(() =>
                                client.pluginAction(credential, plugin.id, 'restart'),
                              )
                            }
                          >
                            {t('instrument.restart')}
                          </button>
                        </>
                      )}
                      <button
                        disabled={busy}
                        onClick={() =>
                          void act(() =>
                            client.pluginAction(credential, plugin.id, 'uninstall'),
                          )
                        }
                      >
                        {t('instrument.uninstall')}
                      </button>
                    </>
                  )}
                </div>
              )}
              {plugin.enabled && canExecute && (
                <button
                  disabled={busy}
                  onClick={() =>
                    void act(() => client.discoverInstruments(credential, plugin.id))
                  }
                >
                  {t('instrument.discover')}
                </button>
              )}
            </article>
          ))}
        </section>
        {sigrok?.enabled && (
          <section className="instrument-card">
            <h3>{t('instrument.capture')}</h3>
            {devices.length === 0 ? (
              <p>{t('instrument.noDevices')}</p>
            ) : (
              <form
                onSubmit={(event) => {
                  event.preventDefault();
                  if (!activeDevice) return;
                  void act(() =>
                    submit({
                      plugin_id: 'sigrok',
                      operation: 'capture',
                      device_id: activeDevice.id,
                      parameters: { channels, samplerate: rate, samples },
                      timeout_seconds: timeout,
                    }),
                  );
                }}
              >
                <fieldset disabled={busy || !canExecute} className="instrument-fields">
                  <label>
                    {t('instrument.device')}
                    <select
                      value={activeDevice?.id ?? ''}
                      onChange={(event) => setDevice(event.target.value)}
                    >
                      {devices.map((item) => (
                        <option key={item.id} value={item.id}>
                          {item.display_name} ({item.connection})
                        </option>
                      ))}
                    </select>
                  </label>
                  <label>
                    {t('instrument.rate')}
                    <select
                      value={rate}
                      onChange={(event) => setRate(Number(event.target.value))}
                    >
                      {(activeDevice?.samplerates ?? []).map((value) => (
                        <option key={value} value={value}>
                          {value / 1000000} MHz
                        </option>
                      ))}
                    </select>
                  </label>
                  <label>
                    {t('instrument.samples')}
                    <input
                      type="number"
                      min={1}
                      max={1000000}
                      value={samples}
                      required
                      onChange={(event) => setSamples(Number(event.target.value))}
                    />
                  </label>
                  <label>
                    {t('instrument.timeout')}
                    <input
                      type="number"
                      min={1}
                      max={300}
                      value={timeout}
                      required
                      onChange={(event) => setTimeoutValue(Number(event.target.value))}
                    />
                  </label>
                  <div
                    className="instrument-channels"
                    role="group"
                    aria-label={t('instrument.channels')}
                  >
                    {(activeDevice?.channels ?? []).map((name) => (
                      <label key={name}>
                        <input
                          type="checkbox"
                          checked={channels.includes(name)}
                          onChange={(event) =>
                            setChannels((previous) =>
                              event.target.checked
                                ? [...previous, name]
                                : previous.filter((item) => item !== name),
                            )
                          }
                        />
                        {name}
                      </label>
                    ))}
                  </div>
                  <button type="submit" disabled={channels.length === 0}>
                    {t('instrument.submit')}
                  </button>
                </fieldset>
              </form>
            )}
          </section>
        )}
        <section className="instrument-card">
          <h3>{t('instrument.transactions')}</h3>
          <p>{t('instrument.queueHelp')}</p>
          {jobs.length === 0 ? (
            <p>{t('instrument.noJobs')}</p>
          ) : (
            <div className="table-scroll">
              <table className="data-table">
                <thead>
                  <tr>
                    <th>{t('instrument.transaction')}</th>
                    <th>{t('instrument.operation')}</th>
                    <th>{t('instrument.state')}</th>
                    <th>{t('instrument.deadline')}</th>
                  </tr>
                </thead>
                <tbody>
                  {jobs.map((job) => (
                    <tr key={job.transaction_id}>
                      <td>
                        <button
                          onClick={() => setSelected(job.transaction_id)}
                          aria-pressed={selected === job.transaction_id}
                        >
                          {job.transaction_id}
                        </button>
                      </td>
                      <td>{job.submission.operation}</td>
                      <td>{job.state}</td>
                      <td>{new Date(job.deadline).toLocaleTimeString()}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </section>
        {current && (
          <section className="instrument-card">
            <h3>{t('instrument.transaction')}</h3>
            <code className="instrument-id">{current.transaction_id}</code>
            <p role="status">
              {current.state}
              {current.error && ` · ${current.error}`}
            </p>
            <div className="instrument-actions">
              {current.state === 'pending_approval' && canApprove && (
                <button
                  disabled={busy}
                  onClick={() =>
                    void act(() =>
                      client.instrumentJobAction(
                        credential,
                        current.transaction_id,
                        'approve',
                      ),
                    )
                  }
                >
                  {t('instrument.approve')}
                </button>
              )}
              {!isJobTerminal(current) && canExecute && (
                <button
                  disabled={busy}
                  onClick={() =>
                    void act(() =>
                      client.instrumentJobAction(
                        credential,
                        current.transaction_id,
                        'cancel',
                      ),
                    )
                  }
                >
                  {t('instrument.cancel')}
                </button>
              )}
              {current.state === 'completed' && current.artifact_id && (
                <button
                  disabled={busy}
                  onClick={() => void act(() => download(current.transaction_id))}
                >
                  {t('instrument.download')}
                </button>
              )}
            </div>
            <DigitalPreview job={current} />
            <details>
              <summary>{t('instrument.result')}</summary>
              <pre className="instrument-json">
                {JSON.stringify(
                  {
                    request_id: current.request_id,
                    submission: current.submission,
                    result: current.result,
                    replies: current.replies,
                  },
                  null,
                  2,
                )}
              </pre>
            </details>
            {current.state === 'completed' &&
              current.artifact_id &&
              canExecute &&
              sigrok?.enabled && (
                <form
                  onSubmit={(event) => {
                    event.preventDefault();
                    void act(() =>
                      submit({
                        plugin_id: 'sigrok',
                        operation: 'decode_uart',
                        device_id: '',
                        parameters: {
                          capture_id: current.transaction_id,
                          channel: decodeChannel,
                          baudrate,
                        },
                        timeout_seconds: timeout,
                      }),
                    );
                  }}
                >
                  <fieldset disabled={busy} className="instrument-fields">
                    <legend>{t('instrument.decode')}</legend>
                    <label>
                      {t('instrument.channel')}
                      <select
                        value={decodeChannel}
                        onChange={(event) => setChannel(event.target.value)}
                      >
                        {capturedChannels.map((name) => (
                          <option key={name}>{name}</option>
                        ))}
                      </select>
                    </label>
                    <label>
                      {t('instrument.baudrate')}
                      <input
                        type="number"
                        required
                        min={1}
                        max={4000000}
                        value={baudrate}
                        onChange={(event) => setBaudrate(Number(event.target.value))}
                      />
                    </label>
                    <button type="submit">{t('instrument.decode')}</button>
                  </fieldset>
                </form>
              )}
          </section>
        )}
      </div>
    </div>
  );
}

function DigitalPreview({ job }: { job: InstrumentJob }) {
  const t = useTranslate();
  const values = job.result.preview;
  const channels = job.submission.parameters.channels;
  if (
    !Array.isArray(values) ||
    !values.every(Number.isSafeInteger) ||
    !Array.isArray(channels)
  )
    return null;
  const samples = values.slice(0, 256) as number[];
  return (
    <figure className="instrument-preview">
      <figcaption>{t('instrument.preview')}</figcaption>
      {channels
        .filter(
          (name): name is string =>
            typeof name === 'string' && /^D([0-9]|1[0-5])$/.test(name),
        )
        .map((name) => {
          const bit = Number(name.slice(1));
          const points = samples
            .map(
              (sample, index) =>
                `${index},${(sample >> bit) & 1 ? 3 : 21} ${index + 1},${(sample >> bit) & 1 ? 3 : 21}`,
            )
            .join(' ');
          return (
            <div key={name}>
              <span>{name}</span>
              <svg
                role="img"
                aria-label={name}
                viewBox={`0 0 ${Math.max(1, samples.length)} 24`}
                preserveAspectRatio="none"
              >
                <polyline
                  points={points}
                  fill="none"
                  stroke="currentColor"
                  strokeWidth="1"
                  vectorEffect="non-scaling-stroke"
                />
              </svg>
            </div>
          );
        })}
    </figure>
  );
}
