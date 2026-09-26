import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import type { BenchCoreClient, Credential } from '../../api/client';
import type { BenchCoreSession } from '../../api/contract';
import type { InstrumentDevice, InstrumentJob } from '../../api/bench';
import {
  cameraWorkspaceAPI,
  defaultCameraSettings,
  type CameraSettings,
  type CameraWorkspaceState,
  type CameraMedia,
  type CameraRetention,
} from '../../api/cameraWorkspace';
import type { MessageKey } from '../../i18n/en';
import { useTranslate } from '../../i18n/languages';
import { MessageBanner } from '../MessageBanner';
import { Modal } from '../Modal';
import { CameraCompositionSettings } from './CameraCompositionSettings';

type View = 'live' | 'recordings';
interface Props {
  client: BenchCoreClient;
  credential: Credential;
  session: BenchCoreSession;
  device: InstrumentDevice;
  view: View;
}
interface Control {
  id: string;
  display_name: string;
  type: string;
  current: number | boolean;
  minimum: number;
  maximum: number;
  step: number;
  read_only: boolean;
  inactive: boolean;
  choices?: { value: number; label: string }[];
}

export function CameraWorkspacePage({
  client,
  credential,
  session,
  device,
  view,
}: Props) {
  const t = useTranslate();
  const api = useMemo(
    () => cameraWorkspaceAPI(client.baseUrl, credential),
    [client.baseUrl, credential],
  );
  const [status, setStatus] = useState<CameraWorkspaceState | null>(null);
  const [editor, setEditor] = useState({
    settings: defaultCameraSettings,
    dirty: false,
    automatic: false,
  });
  const { settings } = editor;
  const [limits, setLimits] = useState<CameraRetention | null>(null);
  const [items, setItems] = useState<CameraMedia[]>([]);
  const [offset, setOffset] = useState(0);
  const [filter, setFilter] = useState('all');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const [job, setJob] = useState<InstrumentJob | null>(null);
  const [controls, setControls] = useState<Control[]>(
    (device.capabilities?.controls ?? []) as Control[],
  );
  const [values, setValues] = useState<Record<string, number | boolean>>({});
  const [expanded, setExpanded] = useState(false);
  const [fullscreen, setFullscreen] = useState(false);
  const [selected, setSelected] = useState<CameraMedia | null>(null);
  const [deleting, setDeleting] = useState<CameraMedia[] | null>(null);
  const [checkedMedia, setCheckedMedia] = useState<string[]>([]);
  const [confirmLimits, setConfirmLimits] = useState(false);
  const [preview, setPreview] = useState('');
  const [stale, setStale] = useState(false);
  const viewer = useRef<HTMLDivElement>(null);
  const canExecute = session.permissions.includes('bench.execute');
  const recording = !!status?.recording;
  const live = status?.state === 'live' || recording;
  const stopVisible = live || status?.state === 'failed' || status?.state === 'busy';
  const refresh = useCallback(async () => {
    const value = await api.status(device.id);
    setStatus(value);
    setEditor((current) =>
      current.dirty
        ? current
        : {
            settings: { ...defaultCameraSettings, ...value.settings },
            dirty: false,
            automatic: false,
          },
    );
    setLimits((current) => current ?? value.retention);
    const media = await api.list(device.id, offset, filter);
    setItems(media.items);
  }, [api, device.id, offset, filter]);
  useEffect(() => {
    let stopped = false;
    const update = () => {
      if (!stopped)
        void refresh().catch((reason) => setError(String(reason.message ?? reason)));
    };
    update();
    const timer = setInterval(update, 2000);
    return () => {
      stopped = true;
      clearInterval(timer);
    };
  }, [refresh]);
  useEffect(() => {
    const update = () => setFullscreen(document.fullscreenElement === viewer.current);
    document.addEventListener('fullscreenchange', update);
    return () => document.removeEventListener('fullscreenchange', update);
  }, []);
  useEffect(() => {
    if (!expanded) return;
    const restore = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setExpanded(false);
    };
    document.addEventListener('keydown', restore);
    return () => document.removeEventListener('keydown', restore);
  }, [expanded]);
  useEffect(() => {
    if (!live || view === 'recordings') return;
    let stopped = false;
    let timer: ReturnType<typeof setTimeout>;
    let objectURL = '';
    const load = async () => {
      try {
        const headers: Record<string, string> = {};
        if (credential.kind === 'bearer')
          headers.Authorization = `Bearer ${credential.token}`;
        const response = await fetch(api.frame(device.id), {
          credentials: 'include',
          headers,
          cache: 'no-store',
          signal: AbortSignal.timeout(5000),
        });
        if (!response.ok) throw new Error('preview');
        const blob = await response.blob();
        if (stopped) return;
        const next = URL.createObjectURL(blob);
        setPreview(next);
        setStale(false);
        if (objectURL) URL.revokeObjectURL(objectURL);
        objectURL = next;
      } catch {
        if (!stopped) setStale(true);
      } finally {
        if (!stopped) timer = setTimeout(() => void load(), 100);
      }
    };
    void load();
    return () => {
      stopped = true;
      clearTimeout(timer);
      if (objectURL) URL.revokeObjectURL(objectURL);
    };
  }, [api, credential, device.id, live, view]);
  useEffect(() => {
    if (!job || ['completed', 'failed', 'cancelled', 'incomplete'].includes(job.state))
      return;
    const timer = setInterval(
      () =>
        void client
          .getInstrumentJob(credential, job.transaction_id)
          .then((next) => {
            setJob(next);
            if (next.state === 'completed') {
              if (Array.isArray(next.result.descriptors))
                setControls(next.result.descriptors as unknown as Control[]);
              if (next.result.values)
                setValues(next.result.values as Record<string, number | boolean>);
              if (next.submission.parameters.action === 'settings')
                setEditor((current) => ({
                  ...current,
                  dirty: false,
                  automatic: false,
                }));
              void refresh();
            } else if (['failed', 'incomplete'].includes(next.state))
              setError(next.error || next.state);
          })
          .catch((reason) => setError(String(reason))),
      400,
    );
    return () => clearInterval(timer);
  }, [client, credential, job, refresh]);
  const command = useCallback(
    async (action: string, extra: Record<string, unknown> = {}) => {
      if (action === 'settings')
        setEditor((current) => ({ ...current, automatic: false }));
      setBusy(true);
      setError('');
      try {
        if (action === 'stop' || action === 'record_stop') {
          await api.stop(device.id, action);
          await refresh();
          return;
        }
        const next = await client.submitInstrumentJob(credential, {
          plugin_id: 'camera',
          operation: 'workspace',
          device_id: device.id,
          parameters: { action, revision: status?.revision ?? 0, ...extra },
          idempotency_key: crypto.randomUUID(),
          timeout_seconds: 35,
        });
        setJob(next);
      } catch (reason) {
        setError(String(reason instanceof Error ? reason.message : reason));
      } finally {
        setBusy(false);
      }
    },
    [api, client, credential, device.id, refresh, status],
  );
  function change(next: CameraSettings) {
    setEditor({ settings: next, dirty: true, automatic: true });
  }
  useEffect(() => {
    if (
      !editor.automatic ||
      !live ||
      recording ||
      busy ||
      (job && !['completed', 'failed', 'cancelled', 'incomplete'].includes(job.state))
    )
      return;
    const timer = setTimeout(
      () => void command('settings', { settings, revision: status?.revision ?? 0 }),
      450,
    );
    return () => clearTimeout(timer);
  }, [
    editor.automatic,
    settings,
    live,
    recording,
    busy,
    job,
    command,
    status?.revision,
  ]);
  async function attempt(action: () => Promise<unknown>) {
    setBusy(true);
    setError('');
    try {
      await action();
      await refresh();
    } catch (reason) {
      setError(String(reason instanceof Error ? reason.message : reason));
    } finally {
      setBusy(false);
    }
  }
  const pending = job?.state === 'pending_approval';
  const working =
    busy ||
    (!!job && !['completed', 'failed', 'cancelled', 'incomplete'].includes(job.state));
  function sizesFor(format: string) {
    const advertised = (device.capabilities?.sizes ?? {}) as Record<string, number[][]>;
    return (advertised[format] ?? [[640, 480]]).filter(
      ([width, height]) =>
        width > 0 &&
        height > 0 &&
        width % 2 === 0 &&
        height % 2 === 0 &&
        width * height <= 2_000_000,
    );
  }
  const formats = (
    (device.capabilities?.formats ?? ['MJPG', 'YUYV']) as string[]
  ).filter(
    (format) => ['MJPG', 'YUYV'].includes(format) && sizesFor(format).length > 0,
  );
  const sizes = sizesFor(settings.format);
  function ratesFor(format: string, width: number, height: number) {
    const intervals = (device.capabilities?.frame_intervals ?? []) as {
      format: string;
      width: number;
      height: number;
      numerator: number;
      denominator: number;
    }[];
    const rates = intervals
      .filter(
        (item) =>
          item.format === format && item.width === width && item.height === height,
      )
      .map((item) => item.denominator / item.numerator)
      .filter((value) => Number.isInteger(value) && value >= 1 && value <= 30);
    return [...new Set(rates.length ? rates : [15])].sort((a, b) => a - b);
  }
  function changeMode(format: string, width: number, height: number) {
    const rates = ratesFor(format, width, height);
    change({
      ...settings,
      format,
      width,
      height,
      fps: rates.includes(settings.fps) ? settings.fps : rates[0],
    });
  }
  const stateLabel = recording
    ? t('cw.recording')
    : live
      ? t('cw.live')
      : status?.state === 'busy'
        ? t('cw.busy')
        : t('cw.stopped');
  const bytes = (n: number) => `${(n / 1048576).toFixed(1)} MiB`;
  const liveToolbar = (
    <div className="camera-live-toolbar">
      <strong
        title={
          live && status?.expires_at
            ? t('cw.sessionEnds', {
                time: new Date(status.expires_at).toLocaleTimeString(),
              })
            : undefined
        }
      >
        {stateLabel}
      </strong>
      <button
        disabled={
          !canExecute ||
          (!stopVisible && (working || pending)) ||
          (status?.state === 'busy' && !session.permissions.includes('plugins.manage'))
        }
        onClick={() => void command(stopVisible ? 'stop' : 'start')}
      >
        {stopVisible ? t('cw.stopLive') : t('cw.startLive')}
      </button>
      <button
        disabled={!canExecute || !live || working || pending}
        onClick={() => void command('snapshot')}
      >
        {t('cw.snapshot')}
      </button>
      <button
        disabled={!canExecute || !live || (!recording && (working || pending))}
        onClick={() => void command(recording ? 'record_stop' : 'record_start')}
      >
        {recording ? t('cw.stopRecording') : t('cw.startRecording')}
      </button>
      <button onClick={() => setExpanded(!expanded)}>
        {expanded ? t('cw.normal') : t('cw.fill')}
      </button>
      <button
        onClick={() =>
          void attempt(async () => {
            if (document.fullscreenElement) await document.exitFullscreen();
            else await viewer.current?.requestFullscreen();
          })
        }
      >
        {t('cw.fullscreen')}
      </button>
    </div>
  );
  const previewPanel = (
    <div
      className={`camera-live-panel ${expanded ? 'camera-live-expanded' : ''}`}
      ref={viewer}
    >
      {(expanded || fullscreen) && (
        <div className="camera-view-heading">
          <h2>
            {device.display_name} · {t('cw.realtime')}
          </h2>
          {liveToolbar}
        </div>
      )}
      {recording && (
        <p role="status">
          {status.recording!.elapsed.toFixed(1)} s · {bytes(status.recording!.bytes)} /{' '}
          {bytes(status.recording!.max_bytes ?? status.retention.video_bytes)} ·{' '}
          {status.recording!.max_seconds ?? status.retention.video_seconds} s{' '}
          {t('cw.limit')}
        </p>
      )}
      {live && stale && <MessageBanner tone="warning">{t('cw.stale')}</MessageBanner>}
      <div className="camera-live-image">
        {live && preview ? (
          <img src={preview} alt={device.display_name} />
        ) : (
          <p>{t('cw.previewHint')}</p>
        )}
      </div>
    </div>
  );
  const numberSetting = (
    label: string,
    key: 'fps' | 'quality' | 'interval' | 'playback_fps' | 'video_quality',
    min: number,
    max: number,
  ) => (
    <label>
      {label}
      <input
        type="number"
        min={min}
        max={max}
        value={settings[key]}
        disabled={recording}
        onChange={(e) => change({ ...settings, [key]: Number(e.target.value) })}
      />
    </label>
  );
  return (
    <div className="page camera-workspace-page">
      <div className="page-head camera-view-heading">
        <h2>
          {device.display_name} · {t(view === 'live' ? 'cw.realtime' : 'cw.recordings')}
        </h2>
        {view === 'live' && !expanded && !fullscreen && liveToolbar}
      </div>
      <div className="page-body">
        {error && <MessageBanner tone="error">{error}</MessageBanner>}
        {status?.state === 'failed' && (
          <MessageBanner tone="error">{t('cw.failed')}</MessageBanner>
        )}
        {pending && (
          <MessageBanner tone="warning">
            {t('cw.approval')}{' '}
            <button
              disabled={!session.permissions.includes('bench.approve')}
              onClick={() =>
                void attempt(async () =>
                  setJob(
                    await client.instrumentJobAction(
                      credential,
                      job.transaction_id,
                      'approve',
                    ),
                  ),
                )
              }
            >
              {t('cw.approve')}
            </button>
            <button
              onClick={() =>
                void attempt(async () =>
                  setJob(
                    await client.instrumentJobAction(
                      credential,
                      job.transaction_id,
                      'cancel',
                    ),
                  ),
                )
              }
            >
              {t('cw.cancel')}
            </button>
          </MessageBanner>
        )}
        {view !== 'recordings' && previewPanel}
        {view === 'live' && (
          <>
            {recording && <MessageBanner tone="info">{t('cw.frozen')}</MessageBanner>}
            <details className="instrument-card camera-settings-section">
              <summary>{t('cw.captureSettings')}</summary>
              <fieldset
                disabled={recording || !canExecute || working || pending}
                className="camera-settings-grid"
              >
                <label>
                  {t('cw.resolution')}
                  <select
                    value={`${settings.width}x${settings.height}`}
                    onChange={(e) => {
                      const [width, height] = e.target.value.split('x').map(Number);
                      changeMode(settings.format, width, height);
                    }}
                  >
                    {sizes.map(([w, h]) => (
                      <option key={`${w}x${h}`} value={`${w}x${h}`}>
                        {w}×{h}
                      </option>
                    ))}
                  </select>
                </label>
                <label>
                  {t('cw.captureFormat')}
                  <select
                    value={settings.format}
                    onChange={(e) => {
                      const format = e.target.value;
                      const [width, height] = sizesFor(format)[0];
                      changeMode(format, width, height);
                    }}
                  >
                    {formats.map((format) => (
                      <option key={format}>{format}</option>
                    ))}
                  </select>
                </label>
                <label>
                  {t('cw.fps')}
                  <select
                    value={settings.fps}
                    onChange={(event) =>
                      change({ ...settings, fps: Number(event.target.value) })
                    }
                  >
                    {ratesFor(settings.format, settings.width, settings.height).map(
                      (fps) => (
                        <option key={fps} value={fps}>
                          {fps}
                        </option>
                      ),
                    )}
                  </select>
                </label>
                <label>
                  {t('cw.zoom')} {settings.processing.zoom ?? 1}×
                  <input
                    type="range"
                    min="1"
                    max="8"
                    step="0.1"
                    value={settings.processing.zoom ?? 1}
                    onChange={(e) =>
                      change({
                        ...settings,
                        processing: {
                          ...settings.processing,
                          zoom: Number(e.target.value),
                        },
                      })
                    }
                  />
                </label>
                {(
                  ['grayscale', 'invert', 'flip_horizontal', 'flip_vertical'] as const
                ).map((key) => (
                  <label className="field-check" key={key}>
                    <input
                      type="checkbox"
                      checked={!!settings.processing[key]}
                      onChange={(e) =>
                        change({
                          ...settings,
                          processing: {
                            ...settings.processing,
                            [key]: e.target.checked,
                          },
                        })
                      }
                    />
                    {t(`cw.${key}`)}
                  </label>
                ))}
              </fieldset>
              <CameraCompositionSettings
                section="camera"
                settings={settings}
                onChange={change}
                disabled={recording || !canExecute || working || !!pending}
                calibrated={Number(device.settings?.calibration_mm_per_pixel ?? 0) > 0}
              />
              <button
                disabled={!canExecute || !live || recording || working || pending}
                onClick={() =>
                  void command('settings', {
                    settings,
                    revision: status?.revision ?? 0,
                  })
                }
              >
                {t('cw.apply')}
              </button>
            </details>
            <details className="instrument-card camera-settings-section">
              <summary>{t('cw.overlays')}</summary>
              <fieldset
                disabled={recording || !canExecute || working || pending}
                className="camera-settings-grid"
              >
                {(['crosshair', 'grid'] as const).map((kind) => (
                  <label className="field-check" key={kind}>
                    <input
                      type="checkbox"
                      checked={
                        !!settings.processing.overlays?.some(
                          (item) => item.kind === kind,
                        )
                      }
                      onChange={(e) =>
                        change({
                          ...settings,
                          processing: {
                            ...settings.processing,
                            overlays: [
                              ...(settings.processing.overlays ?? []).filter(
                                (item) => item.kind !== kind,
                              ),
                              ...(e.target.checked ? [{ kind, x: 0.5, y: 0.5 }] : []),
                            ],
                          },
                        })
                      }
                    />
                    {t(`cw.${kind}`)}
                  </label>
                ))}
              </fieldset>
              <CameraCompositionSettings
                section="overlays"
                settings={settings}
                onChange={change}
                disabled={recording || !canExecute || working || !!pending}
                calibrated={Number(device.settings?.calibration_mm_per_pixel ?? 0) > 0}
              />
              <fieldset disabled={recording || !canExecute || working || pending}>
                {['time', 'name', 'text'].map((kind) => {
                  const stamp = settings.stamps.find((item) => item.kind === kind);
                  return (
                    <div key={kind} className="camera-stamp-row">
                      <label className="field-check">
                        <input
                          type="checkbox"
                          checked={!!stamp}
                          onChange={(e) =>
                            change({
                              ...settings,
                              stamps: e.target.checked
                                ? [
                                    ...settings.stamps,
                                    {
                                      kind,
                                      text: '',
                                      position: 'bottom-left',
                                      background: 'black',
                                      size: 18,
                                      timezone: 'UTC',
                                    },
                                  ]
                                : settings.stamps.filter((item) => item.kind !== kind),
                            })
                          }
                        />
                        {t(
                          kind === 'time'
                            ? 'cw.timestamp'
                            : kind === 'name'
                              ? 'cw.cameraName'
                              : 'cw.text',
                        )}
                      </label>
                      {stamp && (
                        <>
                          {kind === 'text' && (
                            <input
                              aria-label={t('cw.text')}
                              maxLength={120}
                              value={stamp.text ?? ''}
                              onChange={(e) =>
                                change({
                                  ...settings,
                                  stamps: settings.stamps.map((item) =>
                                    item === stamp
                                      ? { ...item, text: e.target.value }
                                      : item,
                                  ),
                                })
                              }
                            />
                          )}
                          <select
                            aria-label={t('cw.position')}
                            value={stamp.position}
                            onChange={(e) =>
                              change({
                                ...settings,
                                stamps: settings.stamps.map((item) =>
                                  item === stamp
                                    ? { ...item, position: e.target.value }
                                    : item,
                                ),
                              })
                            }
                          >
                            {[
                              'top-left',
                              'top-center',
                              'top-right',
                              'bottom-left',
                              'bottom-center',
                              'bottom-right',
                            ].map((position) => (
                              <option key={position} value={position}>
                                {t(`cw.position.${position}` as MessageKey)}
                              </option>
                            ))}
                          </select>
                          <select
                            aria-label={t('cw.background')}
                            value={stamp.background}
                            onChange={(e) =>
                              change({
                                ...settings,
                                stamps: settings.stamps.map((item) =>
                                  item === stamp
                                    ? { ...item, background: e.target.value }
                                    : item,
                                ),
                              })
                            }
                          >
                            {['transparent', 'black', 'white'].map((background) => (
                              <option key={background} value={background}>
                                {t(`cw.background.${background}` as MessageKey)}
                              </option>
                            ))}
                          </select>
                          <input
                            aria-label={t('cw.textSize')}
                            type="number"
                            min={8}
                            max={48}
                            value={stamp.size}
                            onChange={(e) =>
                              change({
                                ...settings,
                                stamps: settings.stamps.map((item) =>
                                  item === stamp
                                    ? { ...item, size: Number(e.target.value) }
                                    : item,
                                ),
                              })
                            }
                          />
                          {kind === 'time' && (
                            <select
                              aria-label={t('cw.timezone')}
                              value={stamp.timezone}
                              onChange={(e) =>
                                change({
                                  ...settings,
                                  stamps: settings.stamps.map((item) =>
                                    item === stamp
                                      ? { ...item, timezone: e.target.value }
                                      : item,
                                  ),
                                })
                              }
                            >
                              {[
                                ...new Set([
                                  'UTC',
                                  Intl.DateTimeFormat().resolvedOptions().timeZone,
                                ]),
                              ].map((zone) => (
                                <option key={zone}>{zone}</option>
                              ))}
                            </select>
                          )}
                        </>
                      )}
                    </div>
                  );
                })}
              </fieldset>
            </details>
            <details className="instrument-card camera-settings-section">
              <summary>{t('cw.hardware')}</summary>
              <fieldset
                disabled={!canExecute || recording || working || pending}
                className="camera-settings-grid"
              >
                {controls.map((control) => (
                  <label key={control.id}>
                    {control.display_name}
                    {control.type === 'boolean' ? (
                      <input
                        type="checkbox"
                        disabled={control.read_only || control.inactive}
                        checked={Boolean(values[control.id] ?? control.current)}
                        onChange={(e) =>
                          setValues({ ...values, [control.id]: e.target.checked })
                        }
                      />
                    ) : control.choices?.length ? (
                      <select
                        disabled={control.read_only || control.inactive}
                        value={String(values[control.id] ?? control.current)}
                        onChange={(e) =>
                          setValues({ ...values, [control.id]: Number(e.target.value) })
                        }
                      >
                        {control.choices.map((choice) => (
                          <option key={choice.value} value={choice.value}>
                            {choice.label}
                          </option>
                        ))}
                      </select>
                    ) : (
                      <input
                        type="number"
                        disabled={control.read_only || control.inactive}
                        min={control.minimum}
                        max={control.maximum}
                        step={control.step || 1}
                        value={Number(values[control.id] ?? control.current ?? 0)}
                        onChange={(e) =>
                          setValues({ ...values, [control.id]: Number(e.target.value) })
                        }
                      />
                    )}
                  </label>
                ))}
              </fieldset>
              <div className="camera-live-toolbar">
                <button
                  disabled={
                    !live ||
                    recording ||
                    working ||
                    pending ||
                    Object.keys(values).length === 0
                  }
                  onClick={() =>
                    void command('controls', {
                      controls: Object.fromEntries(
                        Object.entries(values).filter(([id]) =>
                          controls.some(
                            (c) => c.id === id && !c.read_only && !c.inactive,
                          ),
                        ),
                      ),
                    })
                  }
                >
                  {t('cw.applyControls')}
                </button>
                <button
                  disabled={!live || working || pending}
                  onClick={() => void command('read_controls')}
                >
                  {t('cw.readControls')}
                </button>
                <button
                  disabled={!canExecute || !live || recording || working || pending}
                  onClick={() => void command('reset_controls')}
                >
                  {t('cw.resetControls')}
                </button>
              </div>
            </details>
          </>
        )}
        {view === 'recordings' && (
          <>
            <section className="instrument-card">
              <h3>{t('cw.output')}</h3>
              <fieldset
                disabled={recording || !canExecute || working || pending}
                className="camera-settings-grid"
              >
                <label>
                  {t('cw.imageEncoding')}
                  <select
                    value={settings.encoding}
                    onChange={(e) => change({ ...settings, encoding: e.target.value })}
                  >
                    <option value="jpeg">JPEG</option>
                    <option value="png">PNG</option>
                  </select>
                </label>
                {settings.encoding === 'jpeg' &&
                  numberSetting(t('cw.quality'), 'quality', 1, 95)}
                <label>
                  {t('cw.videoEncoding')}
                  <select value="mp4-h264" disabled>
                    <option value="mp4-h264">MP4 / H.264</option>
                  </select>
                </label>
                <label>
                  {t('cw.mode')}
                  <select
                    value={settings.mode}
                    onChange={(e) => change({ ...settings, mode: e.target.value })}
                  >
                    <option value="video">{t('cw.video')}</option>
                    <option value="timelapse">{t('cw.timelapse')}</option>
                  </select>
                </label>
                {numberSetting(t('cw.videoQuality'), 'video_quality', 18, 35)}
                {settings.mode === 'timelapse' && (
                  <>
                    {numberSetting(t('cw.interval'), 'interval', 1, 3600)}
                    {numberSetting(t('cw.playback'), 'playback_fps', 1, 30)}
                  </>
                )}
              </fieldset>
              <button
                disabled={!canExecute || !live || recording || working || pending}
                onClick={() =>
                  void command('settings', {
                    settings,
                    revision: status?.revision ?? 0,
                  })
                }
              >
                {t('cw.apply')}
              </button>
              {!live && <p>{t('cw.startForSettings')}</p>}
            </section>
            <section className="instrument-card">
              <h3>{t('cw.storage')}</h3>
              <p>
                {bytes(status?.used_bytes ?? 0)} /{' '}
                {bytes(status?.retention.total_bytes ?? 0)}
              </p>
              {session.permissions.includes('plugins.manage') && limits && (
                <details>
                  <summary>{t('cw.retention')}</summary>
                  <div className="camera-settings-grid">
                    <label>
                      {t('cw.keep')}
                      <input
                        type="number"
                        min={1}
                        max={10000}
                        value={limits.snapshots}
                        onChange={(e) =>
                          setLimits({ ...limits, snapshots: Number(e.target.value) })
                        }
                      />
                    </label>
                    <label>
                      {t('cw.totalLimit')}
                      <input
                        type="number"
                        min={64}
                        value={limits.total_bytes / 1048576}
                        onChange={(e) =>
                          setLimits({
                            ...limits,
                            total_bytes: Number(e.target.value) * 1048576,
                          })
                        }
                      />
                    </label>
                    <label>
                      {t('cw.videoLimit')}
                      <input
                        type="number"
                        min={8}
                        value={limits.video_bytes / 1048576}
                        onChange={(e) =>
                          setLimits({
                            ...limits,
                            video_bytes: Number(e.target.value) * 1048576,
                          })
                        }
                      />
                    </label>
                    <label>
                      {t('cw.durationLimit')}
                      <input
                        type="number"
                        min={1}
                        max={3500}
                        value={limits.video_seconds}
                        onChange={(e) =>
                          setLimits({
                            ...limits,
                            video_seconds: Number(e.target.value),
                          })
                        }
                      />
                    </label>
                  </div>
                  <p>{t('cw.retentionHint')}</p>
                  <button
                    disabled={live || busy}
                    onClick={() => setConfirmLimits(true)}
                  >
                    {t('cw.saveLimits')}
                  </button>
                </details>
              )}
            </section>
            <section className="instrument-card">
              <div className="camera-live-toolbar">
                <h3>{t('cw.recordings')}</h3>
                <select
                  aria-label={t('cw.filter')}
                  value={filter}
                  onChange={(e) => {
                    setFilter(e.target.value);
                    setOffset(0);
                    setCheckedMedia([]);
                  }}
                >
                  <option value="all">{t('cw.all')}</option>
                  <option value="image">{t('cw.images')}</option>
                  <option value="video">{t('cw.videos')}</option>
                </select>
              </div>
              <button
                disabled={!canExecute || checkedMedia.length === 0}
                onClick={() =>
                  setDeleting(items.filter((item) => checkedMedia.includes(item.id)))
                }
              >
                {t('cw.deleteSelected', { count: checkedMedia.length })}
              </button>
              <div className="camera-media-grid">
                {items
                  .filter((item) => filter === 'all' || item.kind === filter)
                  .map((item) => (
                    <article className="camera-media-card" key={item.id}>
                      <label className="field-check">
                        <input
                          type="checkbox"
                          aria-label={t('cw.selectCapture', {
                            date: new Date(item.created_at).toLocaleString(),
                          })}
                          disabled={!canExecute || item.state === 'pending'}
                          checked={checkedMedia.includes(item.id)}
                          onChange={(event) =>
                            setCheckedMedia((current) =>
                              event.target.checked
                                ? [...current, item.id]
                                : current.filter((id) => id !== item.id),
                            )
                          }
                        />
                      </label>
                      <button
                        disabled={item.state !== 'completed'}
                        onClick={() => setSelected(item)}
                      >
                        {item.state === 'completed' ? (
                          <img
                            src={api.content(item.id, '?thumbnail=true')}
                            alt={`${item.kind} ${item.created_at}`}
                            loading="lazy"
                          />
                        ) : (
                          <span>{item.state}</span>
                        )}
                      </button>
                      <p>
                        {new Date(item.created_at).toLocaleString()} ·{' '}
                        {bytes(item.bytes)}
                        {item.kind === 'video' &&
                          ` · ${(item.metadata.duration ?? 0).toFixed(1)} s`}
                      </p>
                      <div className="camera-live-toolbar">
                        <a
                          className="button"
                          href={api.content(item.id, '?download=true')}
                        >
                          {t('cw.export')}
                        </a>
                        <button
                          disabled={!canExecute || item.state === 'pending'}
                          onClick={() => setDeleting([item])}
                        >
                          {t('cw.delete')}
                        </button>
                      </div>
                    </article>
                  ))}
              </div>
              <div className="camera-live-toolbar">
                <button
                  disabled={offset === 0}
                  onClick={() => {
                    setOffset(Math.max(0, offset - 24));
                    setCheckedMedia([]);
                  }}
                >
                  {t('cw.previous')}
                </button>
                <button
                  disabled={items.length < 24}
                  onClick={() => {
                    setOffset(offset + 24);
                    setCheckedMedia([]);
                  }}
                >
                  {t('cw.next')}
                </button>
              </div>
            </section>
          </>
        )}
      </div>
      {selected && (
        <Modal title={t('cw.recordings')} onDismiss={() => setSelected(null)}>
          {selected.kind === 'video' ? (
            <video
              className="camera-media-player"
              src={api.content(selected.id)}
              controls
              autoPlay
            />
          ) : (
            <img
              className="camera-media-player"
              src={api.content(selected.id)}
              alt={selected.created_at}
            />
          )}
          <div className="modal-actions">
            <a className="button" href={api.content(selected.id, '?download=true')}>
              {t('cw.export')}
            </a>
          </div>
        </Modal>
      )}
      {deleting && (
        <Modal title={t('cw.delete')} onDismiss={() => setDeleting(null)}>
          <p>{t('cw.deleteConfirm')}</p>
          {error && <MessageBanner tone="error">{error}</MessageBanner>}
          <div className="modal-actions">
            <button disabled={busy} onClick={() => setDeleting(null)}>
              {t('cw.cancel')}
            </button>
            <button
              disabled={busy}
              onClick={() =>
                void attempt(async () => {
                  for (const item of deleting) {
                    await api.remove(item.id);
                    setDeleting(
                      (current) =>
                        current?.filter((value) => value.id !== item.id) ?? null,
                    );
                    setCheckedMedia((current) =>
                      current.filter((id) => id !== item.id),
                    );
                  }
                  setDeleting(null);
                })
              }
            >
              {t('cw.delete')}
            </button>
          </div>
        </Modal>
      )}
      {confirmLimits && limits && (
        <Modal title={t('cw.retention')} onDismiss={() => setConfirmLimits(false)}>
          <p>{t('cw.retentionHint')}</p>
          {error && <MessageBanner tone="error">{error}</MessageBanner>}
          {status?.retention_inventory && (
            <p>
              {t('cw.retentionEligible', {
                count: status.retention_inventory.count,
                size: bytes(status.retention_inventory.bytes),
              })}
            </p>
          )}
          <div className="modal-actions">
            <button onClick={() => setConfirmLimits(false)}>{t('cw.cancel')}</button>
            <button
              disabled={busy}
              onClick={() =>
                void attempt(async () => {
                  await api.retention(device.id, status?.revision ?? 0, limits);
                  setConfirmLimits(false);
                })
              }
            >
              {t('cw.saveLimits')}
            </button>
          </div>
        </Modal>
      )}
    </div>
  );
}
