import { useEffect, useState } from 'react';
import type { BenchCoreClient, Credential } from '../../api/client';
import type {
  InstrumentDevice,
  InstrumentJob,
  InstrumentSubmission,
} from '../../api/bench';
import type { PluginConfiguration } from '../../api/bench';
import { useTranslate } from '../../i18n/languages';
import { Modal, ModalActions } from '../Modal';
import { toImagePoint, toScenePoint, toViewerPoint } from './cameraGeometry';

interface ControlDescriptor {
  id: string;
  display_name: string;
  type: string;
  minimum: number;
  maximum: number;
  step: number;
  current?: number | boolean;
  read_only: boolean;
  inactive: boolean;
  choices?: { value: number; label: string }[];
}

interface ShapeDraft {
  enabled: boolean;
  x: number;
  y: number;
  x2: number;
  y2: number;
}

interface FrameInterval {
  format: string;
  width: number;
  height: number;
  numerator: number;
  denominator: number;
}

function frameIntervalsOf(value: unknown): FrameInterval[] {
  if (!Array.isArray(value)) return [];
  return value.filter(
    (item): item is FrameInterval =>
      typeof item === 'object' &&
      item !== null &&
      (item.format === 'MJPG' || item.format === 'YUYV') &&
      Number.isInteger(item.width) &&
      Number.isInteger(item.height) &&
      Number.isInteger(item.numerator) &&
      item.numerator > 0 &&
      Number.isInteger(item.denominator) &&
      item.denominator > 0,
  );
}

function controlsOf(value: unknown): ControlDescriptor[] {
  if (!Array.isArray(value)) return [];
  return value.filter(
    (item): item is ControlDescriptor =>
      typeof item === 'object' &&
      item !== null &&
      typeof item.id === 'string' &&
      typeof item.display_name === 'string' &&
      typeof item.type === 'string',
  );
}

function CameraPreview({
  client,
  credential,
  job,
  alt,
}: {
  client: BenchCoreClient;
  credential: Credential;
  job: InstrumentJob;
  alt: string;
}) {
  const [url, setURL] = useState('');
  useEffect(() => {
    if (!client.previewInstrumentArtifact) return;
    let active = true;
    let objectURL = '';
    void client
      .previewInstrumentArtifact(credential, job.transaction_id)
      .then((blob) => {
        if (!active || !blob.type.startsWith('image/')) return;
        objectURL = URL.createObjectURL(blob);
        setURL(objectURL);
      })
      .catch(() => setURL(''));
    return () => {
      active = false;
      if (objectURL) URL.revokeObjectURL(objectURL);
    };
  }, [client, credential, job.transaction_id]);
  return (
    url && (
      <img
        loading="lazy"
        src={url}
        alt={alt}
        style={{
          display: 'block',
          maxWidth: '100%',
          maxHeight: 240,
          objectFit: 'contain',
        }}
      />
    )
  );
}

interface Props {
  view: 'overview' | 'snapshot' | 'settings' | 'administration';
  focusDevice?: string;
  client: BenchCoreClient;
  credential: Credential;
  devices: InstrumentDevice[];
  jobs: InstrumentJob[];
  busy: boolean;
  canManage: boolean;
  canExecute: boolean;
  run: (action: () => Promise<unknown>) => void;
  submit: (input: Omit<InstrumentSubmission, 'idempotency_key'>) => Promise<void>;
}

/** Operates only on cameras reported by the installed physical camera package. */
export function CameraSection({
  client,
  credential,
  devices,
  jobs,
  busy,
  canManage,
  canExecute,
  run,
  submit,
  focusDevice,
  view,
}: Props) {
  const t = useTranslate();
  const [selected, setSelected] = useState('');
  const camera = focusDevice
    ? devices.find((item) => item.id === focusDevice)
    : (devices.find((item) => item.id === selected) ?? devices[0]);
  const [nameDraft, setNameDraft] = useState<{ device: string; value: string } | null>(
    null,
  );
  const [purposeDraft, setPurposeDraft] = useState<{
    device: string;
    value: string;
  } | null>(null);
  const [settingsDraft, setSettingsDraft] = useState<{
    device: string;
    revision: number;
    values: Record<string, unknown>;
  } | null>(null);
  const [width, setWidth] = useState(640);
  const [height, setHeight] = useState(480);
  const [format, setFormat] = useState('MJPG');
  const [frameInterval, setFrameInterval] = useState('');
  const [zoom, setZoom] = useState(1);
  const [centerX, setCenterX] = useState(0.5);
  const [centerY, setCenterY] = useState(0.5);
  const [rotation, setRotation] = useState(0);
  const [flipHorizontal, setFlipHorizontal] = useState(false);
  const [flipVertical, setFlipVertical] = useState(false);
  const [grayscale, setGrayscale] = useState(false);
  const [invert, setInvert] = useState(false);
  const [encoding, setEncoding] = useState<'jpeg' | 'png'>('jpeg');
  const [captureJpegQuality, setCaptureJpegQuality] = useState(85);
  const [maximumDimension, setMaximumDimension] = useState(0);
  const [crosshair, setCrosshair] = useState(false);
  const [grid, setGrid] = useState(false);
  const [overlayText, setOverlayText] = useState('');
  const [line, setLine] = useState<ShapeDraft>({
    enabled: false,
    x: 0.1,
    y: 0.1,
    x2: 0.9,
    y2: 0.9,
  });
  const [rectangle, setRectangle] = useState<ShapeDraft>({
    enabled: false,
    x: 0.1,
    y: 0.1,
    x2: 0.9,
    y2: 0.9,
  });
  const [scaleBar, setScaleBar] = useState({
    enabled: false,
    x: 0.1,
    y: 0.9,
    lengthMM: 10,
  });
  const [overlayColor, setOverlayColor] = useState('#ffffff');
  const [overlayLineWidth, setOverlayLineWidth] = useState(2);
  const [historySelection, setHistorySelection] = useState<{
    device: string;
    job: string;
  } | null>(null);
  const [compareFirst, setCompareFirst] = useState('');
  const [compareSecond, setCompareSecond] = useState('');
  const [viewZoom, setViewZoom] = useState(1);
  const [viewPanX, setViewPanX] = useState(50);
  const [viewPanY, setViewPanY] = useState(50);
  const [viewRotation, setViewRotation] = useState(0);
  const [viewFlipHorizontal, setViewFlipHorizontal] = useState(false);
  const [viewFlipVertical, setViewFlipVertical] = useState(false);
  const [viewInvert, setViewInvert] = useState(false);
  const [markers, setMarkers] = useState<{ x: number; y: number }[]>([]);
  const [imageSize, setImageSize] = useState({ width: 1, height: 1 });
  const [image, setImage] = useState<{
    device: string;
    job: string;
    url: string;
  } | null>(null);
  const [controlDraft, setControlDraft] = useState<{
    device: string;
    values: Record<string, number | boolean>;
  } | null>(null);
  const [configuration, setConfiguration] = useState<PluginConfiguration | null>(null);
  const [limitsDraft, setLimitsDraft] = useState<{
    revision: number;
    values: Record<string, number>;
  } | null>(null);
  const [dialog, setDialog] = useState<{
    kind: 'rebind' | 'forget';
    device: InstrumentDevice;
  } | null>(null);
  const [confirmation, setConfirmation] = useState('');
  const [provisionalID, setProvisionalID] = useState('');
  const controls = controlsOf(camera?.capabilities?.controls);
  const frameIntervals = frameIntervalsOf(camera?.capabilities?.frame_intervals).filter(
    (item) => item.format === format && item.width === width && item.height === height,
  );
  const selectedFrameInterval = frameIntervals.find(
    (item) => `${item.numerator}/${item.denominator}` === frameInterval,
  );
  const viewTransform = {
    zoom: viewZoom,
    panX: viewPanX / 100,
    panY: viewPanY / 100,
    rotation: viewRotation,
    flipHorizontal: viewFlipHorizontal,
    flipVertical: viewFlipVertical,
  };
  const settings =
    settingsDraft &&
    camera &&
    settingsDraft.device === camera.id &&
    settingsDraft.revision === camera.revision
      ? settingsDraft.values
      : (camera?.settings ?? {});
  const baseline =
    typeof settings.baseline_controls === 'object' &&
    settings.baseline_controls !== null &&
    !Array.isArray(settings.baseline_controls)
      ? (settings.baseline_controls as Record<string, number | boolean>)
      : {};
  const setSetting = (key: string, value: unknown) => {
    if (!camera) return;
    setSettingsDraft({
      device: camera.id,
      revision: camera.revision ?? 0,
      values: { ...settings, [key]: value },
    });
  };
  const setBaseline = (id: string, value: number | boolean | undefined) => {
    const next = { ...baseline };
    if (value === undefined) delete next[id];
    else next[id] = value;
    setSetting('baseline_controls', next);
  };
  const history = jobs
    .filter(
      (job) =>
        job.plugin_id === 'camera' &&
        job.submission.device_id === camera?.id &&
        job.submission.operation === 'snapshot' &&
        job.state === 'completed' &&
        job.artifact_id,
    )
    .sort((left, right) => right.created_at.localeCompare(left.created_at));
  const saved = jobs
    .filter(
      (job) =>
        job.plugin_id === 'camera' &&
        job.submission.operation === 'snapshot' &&
        job.state === 'completed' &&
        job.artifact_id,
    )
    .sort((left, right) => right.created_at.localeCompare(left.created_at));
  const firstComparison =
    saved.find((job) => job.transaction_id === compareFirst) ?? saved[0];
  const secondComparison =
    saved.find((job) => job.transaction_id === compareSecond) ??
    saved.find((job) => job.transaction_id !== firstComparison?.transaction_id);
  const latest = history[0];
  const selectedImage =
    history.find(
      (job) =>
        job.transaction_id === historySelection?.job &&
        historySelection.device === camera?.id,
    ) ?? latest;
  const latestControls = jobs.find(
    (job) =>
      job.plugin_id === 'camera' &&
      job.submission.device_id === camera?.id &&
      ['get_controls', 'set_controls', 'reset_controls'].includes(
        job.submission.operation,
      ) &&
      job.state === 'completed',
  );

  const name =
    nameDraft && camera && nameDraft.device === camera.id
      ? nameDraft.value
      : (camera?.display_name ?? '');
  const purpose =
    purposeDraft && camera && purposeDraft.device === camera.id
      ? purposeDraft.value
      : (camera?.purpose ?? '');
  const readBack = latestControls?.result.values;
  const currentControls =
    typeof readBack === 'object' && readBack !== null && !Array.isArray(readBack)
      ? (Object.fromEntries(
          Object.entries(readBack).filter(
            ([, value]) => typeof value === 'number' || typeof value === 'boolean',
          ),
        ) as Record<string, number | boolean>)
      : (Object.fromEntries(
          controls
            .filter((item) => item.current !== undefined)
            .map((item) => [item.id, item.current!]),
        ) as Record<string, number | boolean>);
  const controlValues = {
    ...currentControls,
    ...(controlDraft && camera && controlDraft.device === camera.id
      ? controlDraft.values
      : {}),
  };
  const setControlValue = (id: string, value: number | boolean) =>
    setControlDraft((previous) => ({
      device: camera?.id ?? '',
      values: {
        ...(previous && camera && previous.device === camera.id ? previous.values : {}),
        [id]: value,
      },
    }));
  useEffect(() => {
    if (!canManage || view !== 'administration' || !client.getPluginConfiguration)
      return;
    let active = true;
    void client
      .getPluginConfiguration(credential, 'camera')
      .then((value) => {
        if (active) setConfiguration(value);
      })
      .catch(() => {});
    return () => {
      active = false;
    };
  }, [canManage, client, credential, view]);
  const limitSettings = configuration?.settings ?? {};
  const limits =
    limitsDraft && configuration && limitsDraft.revision === configuration.revision
      ? limitsDraft.values
      : {};
  const limitValue = (key: string, fallback: number) =>
    limits[key] ??
    (typeof limitSettings[key] === 'number'
      ? (limitSettings[key] as number)
      : fallback);
  const maximumWidth = limitValue('maximum_width', 4096);
  const maximumHeight = limitValue('maximum_height', 4096);
  const maximumPixels = limitValue('maximum_pixels', 2000000);
  const jpegQuality = limitValue('default_jpeg_quality', 85);
  const setLimit = (key: string, value: number) =>
    setLimitsDraft((previous) => ({
      revision: configuration?.revision ?? -1,
      values: {
        ...(previous && configuration && previous.revision === configuration.revision
          ? previous.values
          : {}),
        [key]: value,
      },
    }));
  const latestID = selectedImage?.transaction_id;
  const cameraID = camera?.id;
  useEffect(() => {
    if (view !== 'snapshot' || !latestID || !cameraID) return;
    let current = true;
    let objectUrl = '';
    if (!client.previewInstrumentArtifact) return;
    void client
      .previewInstrumentArtifact(credential, latestID)
      .then((blob) => {
        if (!current || !blob.type.startsWith('image/')) return;
        objectUrl = URL.createObjectURL(blob);
        setImage({ device: cameraID, job: latestID, url: objectUrl });
      })
      .catch(() => setImage(null));
    return () => {
      current = false;
      if (objectUrl) URL.revokeObjectURL(objectUrl);
    };
  }, [client, credential, latestID, cameraID, view]);

  const formats = Array.isArray(camera?.capabilities?.formats)
    ? camera.capabilities.formats.filter(
        (item): item is string => item === 'MJPG' || item === 'YUYV',
      )
    : [];
  const ready =
    camera?.configured &&
    camera.enabled &&
    camera.presence === 'present' &&
    camera.readiness === 'ready';
  const provisional = devices.filter(
    (item) => item.presence === 'present' && !item.configured,
  );
  return (
    <section className="instrument-card" aria-label={t('camera.title')}>
      <h3>{t('camera.title')}</h3>
      {view === 'administration' &&
        canManage &&
        configuration &&
        client.savePluginConfiguration && (
          <details>
            <summary>{t('camera.packageLimits')}</summary>
            <form
              onSubmit={(event) => {
                event.preventDefault();
                run(() =>
                  client.savePluginConfiguration!(
                    credential,
                    'camera',
                    configuration.revision,
                    {
                      ...configuration.settings,
                      maximum_width: maximumWidth,
                      maximum_height: maximumHeight,
                      maximum_pixels: maximumPixels,
                      default_jpeg_quality: jpegQuality,
                    },
                  ).then((updated) => {
                    setConfiguration(updated);
                    setLimitsDraft(null);
                  }),
                );
              }}
            >
              <div className="instrument-fields">
                <label>
                  {t('camera.maximumWidth')}
                  <input
                    type="number"
                    min={16}
                    max={4096}
                    value={maximumWidth}
                    onChange={(event) =>
                      setLimit('maximum_width', Number(event.target.value))
                    }
                  />
                </label>
                <label>
                  {t('camera.maximumHeight')}
                  <input
                    type="number"
                    min={16}
                    max={4096}
                    value={maximumHeight}
                    onChange={(event) =>
                      setLimit('maximum_height', Number(event.target.value))
                    }
                  />
                </label>
                <label>
                  {t('camera.maximumPixels')}
                  <input
                    type="number"
                    min={256}
                    max={16777216}
                    value={maximumPixels}
                    onChange={(event) =>
                      setLimit('maximum_pixels', Number(event.target.value))
                    }
                  />
                </label>
                <label>
                  {t('camera.jpegQuality')}
                  <input
                    type="number"
                    min={1}
                    max={95}
                    value={jpegQuality}
                    onChange={(event) =>
                      setLimit('default_jpeg_quality', Number(event.target.value))
                    }
                  />
                </label>
                <button disabled={busy} type="submit">
                  {t('camera.savePackageLimits')}
                </button>
              </div>
            </form>
          </details>
        )}
      {devices.length === 0 || (focusDevice !== undefined && camera === undefined) ? (
        <p>{t('camera.none')}</p>
      ) : (
        <>
          {view === 'overview' && (
            <div
              aria-label={t('camera.overview')}
              style={{
                display: 'grid',
                gridTemplateColumns: 'repeat(auto-fit, minmax(220px, 1fr))',
                gap: 12,
                marginBottom: 18,
              }}
            >
              {devices
                .filter((item) => item.configured)
                .map((item) => {
                  const recent = saved.find(
                    (job) => job.submission.device_id === item.id,
                  );
                  const available =
                    item.configured &&
                    item.enabled &&
                    item.presence === 'present' &&
                    item.readiness === 'ready';
                  const supported = Array.isArray(item.capabilities?.formats)
                    ? item.capabilities.formats.filter(
                        (value): value is string =>
                          value === 'MJPG' || value === 'YUYV',
                      )
                    : [];
                  const preferred = String(item.settings?.default_format ?? 'MJPG');
                  const quickFormat = supported.includes(preferred)
                    ? preferred
                    : (supported[0] ?? 'MJPG');
                  return (
                    <div
                      key={item.id}
                      style={{
                        padding: 16,
                        border: '1px solid var(--border-color, #b5c5ca)',
                        borderRadius: 8,
                      }}
                    >
                      <button
                        type="button"
                        aria-pressed={camera?.id === item.id}
                        onClick={() => {
                          setSelected(item.id);
                          setMarkers([]);
                        }}
                        style={{ display: 'block', textAlign: 'left', width: '100%' }}
                      >
                        <strong>{item.display_name}</strong>
                        <br />
                        <code>{item.id}</code>
                        <br />
                        <span>
                          {item.presence} · {item.readiness}
                        </span>
                      </button>
                      {recent && (
                        <CameraPreview
                          client={client}
                          credential={credential}
                          job={recent}
                          alt={t('camera.savedFrom', { name: item.display_name })}
                        />
                      )}
                      <button
                        type="button"
                        disabled={busy || !canExecute || !available}
                        onClick={() =>
                          run(() =>
                            submit({
                              plugin_id: 'camera',
                              operation: 'snapshot',
                              device_id: item.id,
                              parameters: {
                                width: Number(item.settings?.default_width ?? 640),
                                height: Number(item.settings?.default_height ?? 480),
                                format: quickFormat,
                              },
                              timeout_seconds: 60,
                            }),
                          )
                        }
                      >
                        {t('camera.quickSnapshot', { name: item.display_name })}
                      </button>
                    </div>
                  );
                })}
            </div>
          )}
          {view === 'overview' && saved.length > 1 && (
            <section>
              <h4>{t('camera.compare')}</h4>
              <div className="instrument-fields">
                <label>
                  {t('camera.compareFirst')}
                  <select
                    value={firstComparison?.transaction_id ?? ''}
                    onChange={(event) => setCompareFirst(event.target.value)}
                  >
                    {saved.map((job) => (
                      <option key={job.transaction_id} value={job.transaction_id}>
                        {devices.find((item) => item.id === job.submission.device_id)
                          ?.display_name ?? job.submission.device_id}
                        {' · '}
                        {new Date(job.created_at).toLocaleString()}
                      </option>
                    ))}
                  </select>
                </label>
                <label>
                  {t('camera.compareSecond')}
                  <select
                    value={secondComparison?.transaction_id ?? ''}
                    onChange={(event) => setCompareSecond(event.target.value)}
                  >
                    {saved
                      .filter(
                        (job) => job.transaction_id !== firstComparison?.transaction_id,
                      )
                      .map((job) => (
                        <option key={job.transaction_id} value={job.transaction_id}>
                          {devices.find((item) => item.id === job.submission.device_id)
                            ?.display_name ?? job.submission.device_id}
                          {' · '}
                          {new Date(job.created_at).toLocaleString()}
                        </option>
                      ))}
                  </select>
                </label>
              </div>
              <div
                style={{
                  display: 'grid',
                  gridTemplateColumns:
                    'repeat(auto-fit, minmax(min(100%, 280px), 1fr))',
                  gap: 12,
                }}
              >
                {firstComparison && (
                  <CameraPreview
                    client={client}
                    credential={credential}
                    job={firstComparison}
                    alt={t('camera.compareFirst')}
                  />
                )}
                {secondComparison && (
                  <CameraPreview
                    client={client}
                    credential={credential}
                    job={secondComparison}
                    alt={t('camera.compareSecond')}
                  />
                )}
              </div>
            </section>
          )}
          {view === 'administration' && canManage && (
            <div className="table-scroll">
              <table className="data-table">
                <thead>
                  <tr>
                    <th>{t('camera.title')}</th>
                    <th>{t('camera.presence')}</th>
                    <th>{t('camera.readiness')}</th>
                    <th>{t('camera.actions')}</th>
                  </tr>
                </thead>
                <tbody>
                  {devices.map((item) => (
                    <tr key={item.id}>
                      <td>
                        {item.display_name} ({item.id})
                      </td>
                      <td>{item.presence}</td>
                      <td>{item.readiness}</td>
                      <td>
                        <div className="instrument-actions">
                          <button type="button" onClick={() => setSelected(item.id)}>
                            {t('camera.configure')}
                          </button>
                          {item.configured && client.instrumentDeviceAction && (
                            <button
                              type="button"
                              disabled={busy}
                              onClick={() =>
                                run(() =>
                                  client.instrumentDeviceAction!(
                                    credential,
                                    'camera',
                                    item.id,
                                    item.enabled ? 'disable' : 'enable',
                                    item.revision ?? 0,
                                  ),
                                )
                              }
                            >
                              {item.enabled
                                ? t('instrument.disable')
                                : t('instrument.enable')}
                            </button>
                          )}
                          {item.configured &&
                            item.enabled &&
                            item.presence === 'present' &&
                            item.readiness === 'failed' &&
                            client.instrumentDeviceAction && (
                              <button
                                type="button"
                                disabled={busy}
                                onClick={() =>
                                  run(() =>
                                    client.instrumentDeviceAction!(
                                      credential,
                                      'camera',
                                      item.id,
                                      'retry',
                                      item.revision ?? 0,
                                    ),
                                  )
                                }
                              >
                                {t('camera.retry')}
                              </button>
                            )}
                          {item.configured &&
                            item.presence === 'absent' &&
                            provisional.length > 0 && (
                              <button
                                type="button"
                                onClick={() => {
                                  setConfirmation('');
                                  setProvisionalID(provisional[0].id);
                                  setDialog({ kind: 'rebind', device: item });
                                }}
                              >
                                {t('camera.rebind')}
                              </button>
                            )}
                          <button
                            type="button"
                            onClick={() => {
                              setConfirmation('');
                              setDialog({ kind: 'forget', device: item });
                            }}
                          >
                            {t('camera.forget')}
                          </button>
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
          {view === 'administration' && (
            <label>
              {t('camera.title')}
              <select
                value={camera?.id ?? ''}
                onChange={(event) => setSelected(event.target.value)}
              >
                {devices.map((item) => (
                  <option key={item.id} value={item.id}>
                    {item.display_name} ({item.id})
                  </option>
                ))}
              </select>
            </label>
          )}
          <p role="status">
            {camera?.presence} · {camera?.readiness}
            {camera?.configured ? '' : ` · ${t('camera.configurationRequired')}`}
          </p>
          {view === 'administration' &&
            canManage &&
            camera &&
            client.configureInstrumentDevice && (
              <form
                onSubmit={(event) => {
                  event.preventDefault();
                  run(() =>
                    client.configureInstrumentDevice!(
                      credential,
                      'camera',
                      camera.id,
                      camera.revision ?? 0,
                      name.trim(),
                      purpose.trim(),
                      settings,
                    ),
                  );
                }}
              >
                <div className="instrument-fields">
                  <label>
                    {t('camera.name')}
                    <input
                      value={name}
                      maxLength={64}
                      required
                      onChange={(event) =>
                        setNameDraft({ device: camera.id, value: event.target.value })
                      }
                    />
                  </label>
                  <label>
                    {t('camera.purpose')}
                    <input
                      value={purpose}
                      maxLength={200}
                      onChange={(event) =>
                        setPurposeDraft({
                          device: camera.id,
                          value: event.target.value,
                        })
                      }
                    />
                  </label>
                  <label>
                    {t('camera.defaultWidth')}
                    <input
                      type="number"
                      min={16}
                      max={4096}
                      value={Number(settings.default_width ?? 640)}
                      onChange={(event) =>
                        setSetting('default_width', Number(event.target.value))
                      }
                    />
                  </label>
                  <label>
                    {t('camera.defaultHeight')}
                    <input
                      type="number"
                      min={16}
                      max={4096}
                      value={Number(settings.default_height ?? 480)}
                      onChange={(event) =>
                        setSetting('default_height', Number(event.target.value))
                      }
                    />
                  </label>
                  <label>
                    {t('camera.defaultFormat')}
                    <select
                      value={String(settings.default_format ?? 'MJPG')}
                      onChange={(event) =>
                        setSetting('default_format', event.target.value)
                      }
                    >
                      {(formats.length ? formats : ['MJPG', 'YUYV']).map((item) => (
                        <option key={item}>{item}</option>
                      ))}
                    </select>
                  </label>
                  <label>
                    {t('camera.mountingRotation')}
                    <select
                      value={Number(settings.mounting_rotation ?? 0)}
                      onChange={(event) =>
                        setSetting('mounting_rotation', Number(event.target.value))
                      }
                    >
                      {[0, 90, 180, 270].map((angle) => (
                        <option key={angle} value={angle}>
                          {angle}°
                        </option>
                      ))}
                    </select>
                  </label>
                  <label>
                    <input
                      type="checkbox"
                      checked={Boolean(settings.mounting_flip_horizontal)}
                      onChange={(event) =>
                        setSetting('mounting_flip_horizontal', event.target.checked)
                      }
                    />{' '}
                    {t('camera.mountingFlipHorizontal')}
                  </label>
                  <label>
                    <input
                      type="checkbox"
                      checked={Boolean(settings.mounting_flip_vertical)}
                      onChange={(event) =>
                        setSetting('mounting_flip_vertical', event.target.checked)
                      }
                    />{' '}
                    {t('camera.mountingFlipVertical')}
                  </label>
                  <label>
                    {t('camera.calibration')}
                    <input
                      type="number"
                      min="0.000001"
                      step="any"
                      value={
                        typeof settings.calibration_mm_per_pixel === 'number'
                          ? settings.calibration_mm_per_pixel
                          : ''
                      }
                      onChange={(event) => {
                        const next = { ...settings };
                        if (event.target.value)
                          next.calibration_mm_per_pixel = Number(event.target.value);
                        else delete next.calibration_mm_per_pixel;
                        setSettingsDraft({
                          device: camera.id,
                          revision: camera.revision ?? 0,
                          values: next,
                        });
                      }}
                    />
                  </label>
                  {controls
                    .filter((control) => !control.read_only && !control.inactive)
                    .map((control) => (
                      <div key={control.id} className="instrument-actions">
                        <label>
                          <input
                            type="checkbox"
                            checked={Object.hasOwn(baseline, control.id)}
                            onChange={(event) =>
                              setBaseline(
                                control.id,
                                event.target.checked
                                  ? (controlValues[control.id] ?? control.current ?? 0)
                                  : undefined,
                              )
                            }
                          />
                          {t('camera.baseline', { name: control.display_name })}
                        </label>
                        {Object.hasOwn(baseline, control.id) &&
                          (control.type === 'boolean' ? (
                            <label>
                              <input
                                type="checkbox"
                                checked={Boolean(baseline[control.id])}
                                onChange={(event) =>
                                  setBaseline(control.id, event.target.checked)
                                }
                              />{' '}
                              {t('camera.on')}
                            </label>
                          ) : control.type === 'menu' ? (
                            <select
                              aria-label={t('camera.baseline', {
                                name: control.display_name,
                              })}
                              value={Number(baseline[control.id])}
                              onChange={(event) =>
                                setBaseline(control.id, Number(event.target.value))
                              }
                            >
                              {(control.choices ?? []).map((choice) => (
                                <option key={choice.value} value={choice.value}>
                                  {choice.label}
                                </option>
                              ))}
                            </select>
                          ) : (
                            <input
                              aria-label={t('camera.baseline', {
                                name: control.display_name,
                              })}
                              type="number"
                              min={control.minimum}
                              max={control.maximum}
                              step={control.step || 1}
                              value={Number(baseline[control.id])}
                              onChange={(event) =>
                                setBaseline(control.id, Number(event.target.value))
                              }
                            />
                          ))}
                      </div>
                    ))}
                  <button disabled={busy || !name.trim()} type="submit">
                    {t('camera.saveConfiguration')}
                  </button>
                </div>
              </form>
            )}
          {view === 'snapshot' && (
            <form
              onSubmit={(event) => {
                event.preventDefault();
                if (!camera) return;
                const overlays: Record<string, unknown>[] = [];
                if (crosshair) overlays.push({ kind: 'crosshair', x: 0.5, y: 0.5 });
                if (grid) overlays.push({ kind: 'grid', spacing: 0.1 });
                if (overlayText.trim())
                  overlays.push({
                    kind: 'text',
                    x: 0.05,
                    y: 0.05,
                    text: overlayText.trim(),
                  });
                if (line.enabled)
                  overlays.push({
                    kind: 'line',
                    x: line.x,
                    y: line.y,
                    x2: line.x2,
                    y2: line.y2,
                    color: overlayColor,
                    line_width: overlayLineWidth,
                  });
                if (rectangle.enabled)
                  overlays.push({
                    kind: 'rectangle',
                    x: rectangle.x,
                    y: rectangle.y,
                    x2: rectangle.x2,
                    y2: rectangle.y2,
                    color: overlayColor,
                    line_width: overlayLineWidth,
                  });
                if (scaleBar.enabled)
                  overlays.push({
                    kind: 'scale_bar',
                    x: scaleBar.x,
                    y: scaleBar.y,
                    length_mm: scaleBar.lengthMM,
                    color: overlayColor,
                    line_width: overlayLineWidth,
                  });
                for (const marker of markers)
                  overlays.push({
                    kind: 'crosshair',
                    ...toScenePoint(marker.x, marker.y, {
                      zoom,
                      centerX,
                      centerY,
                      rotation,
                      flipHorizontal,
                      flipVertical,
                    }),
                    color: '#ff2200',
                  });
                run(() =>
                  submit({
                    plugin_id: 'camera',
                    operation: 'snapshot',
                    device_id: camera.id,
                    parameters: {
                      width,
                      height,
                      format,
                      encoding,
                      jpeg_quality: captureJpegQuality,
                      ...(selectedFrameInterval
                        ? {
                            frame_interval: {
                              numerator: selectedFrameInterval.numerator,
                              denominator: selectedFrameInterval.denominator,
                            },
                          }
                        : {}),
                      ...(maximumDimension
                        ? { maximum_dimension: maximumDimension }
                        : {}),
                      processing: {
                        zoom,
                        center_x: centerX,
                        center_y: centerY,
                        rotation,
                        flip_horizontal: flipHorizontal,
                        flip_vertical: flipVertical,
                        grayscale,
                        invert,
                        overlays,
                      },
                    },
                    timeout_seconds: 60,
                  }),
                );
              }}
            >
              <fieldset
                disabled={busy || !canExecute || !ready}
                className="instrument-fields"
              >
                <label>
                  {t('camera.width')}
                  <input
                    type="number"
                    min={16}
                    max={4096}
                    value={width}
                    onChange={(event) => setWidth(Number(event.target.value))}
                  />
                </label>
                <label>
                  {t('camera.height')}
                  <input
                    type="number"
                    min={16}
                    max={4096}
                    value={height}
                    onChange={(event) => setHeight(Number(event.target.value))}
                  />
                </label>
                <label>
                  {t('camera.format')}
                  <select
                    value={format}
                    onChange={(event) => setFormat(event.target.value)}
                  >
                    {(formats.length ? formats : ['MJPG', 'YUYV']).map((item) => (
                      <option key={item}>{item}</option>
                    ))}
                  </select>
                </label>
                {frameIntervals.length > 0 && (
                  <label>
                    {t('camera.frameInterval')}
                    <select
                      value={selectedFrameInterval ? frameInterval : ''}
                      onChange={(event) => setFrameInterval(event.target.value)}
                    >
                      <option value="">{t('camera.deviceDefaultInterval')}</option>
                      {frameIntervals.map((item) => (
                        <option
                          key={`${item.numerator}/${item.denominator}`}
                          value={`${item.numerator}/${item.denominator}`}
                        >
                          {`${item.denominator / item.numerator} fps`}
                        </option>
                      ))}
                    </select>
                  </label>
                )}
                <label>
                  {t('camera.digitalZoom')}
                  <input
                    type="number"
                    min={1}
                    max={8}
                    step={0.1}
                    value={zoom}
                    onChange={(event) => setZoom(Number(event.target.value))}
                  />
                </label>
                <label>
                  {t('camera.cropX')}
                  <input
                    type="number"
                    min={0}
                    max={1}
                    step={0.05}
                    value={centerX}
                    onChange={(event) => setCenterX(Number(event.target.value))}
                  />
                </label>
                <label>
                  {t('camera.cropY')}
                  <input
                    type="number"
                    min={0}
                    max={1}
                    step={0.05}
                    value={centerY}
                    onChange={(event) => setCenterY(Number(event.target.value))}
                  />
                </label>
                <label>
                  {t('camera.rotation')}
                  <select
                    value={rotation}
                    onChange={(event) => setRotation(Number(event.target.value))}
                  >
                    {[0, 90, 180, 270].map((angle) => (
                      <option key={angle} value={angle}>
                        {angle}°
                      </option>
                    ))}
                  </select>
                </label>
                <label>
                  <input
                    type="checkbox"
                    checked={grayscale}
                    onChange={(event) => setGrayscale(event.target.checked)}
                  />{' '}
                  {t('camera.grayscale')}
                </label>
                <label>
                  <input
                    type="checkbox"
                    checked={invert}
                    onChange={(event) => setInvert(event.target.checked)}
                  />{' '}
                  {t('camera.invert')}
                </label>
                <label>
                  <input
                    type="checkbox"
                    checked={flipHorizontal}
                    onChange={(event) => setFlipHorizontal(event.target.checked)}
                  />{' '}
                  {t('camera.flipHorizontal')}
                </label>
                <label>
                  <input
                    type="checkbox"
                    checked={flipVertical}
                    onChange={(event) => setFlipVertical(event.target.checked)}
                  />{' '}
                  {t('camera.flipVertical')}
                </label>
                <label>
                  {t('camera.encoding')}
                  <select
                    value={encoding}
                    onChange={(event) =>
                      setEncoding(event.target.value as 'jpeg' | 'png')
                    }
                  >
                    <option value="jpeg">JPEG</option>
                    <option value="png">PNG</option>
                  </select>
                </label>
                {encoding === 'jpeg' && (
                  <label>
                    {t('camera.jpegQuality')}
                    <input
                      type="number"
                      min={1}
                      max={95}
                      value={captureJpegQuality}
                      onChange={(event) =>
                        setCaptureJpegQuality(Number(event.target.value))
                      }
                    />
                  </label>
                )}
                <label>
                  {t('camera.maximumDimension')}
                  <input
                    type="number"
                    min={0}
                    max={4096}
                    value={maximumDimension}
                    onChange={(event) =>
                      setMaximumDimension(Number(event.target.value))
                    }
                  />
                </label>
                <label>
                  <input
                    type="checkbox"
                    checked={crosshair}
                    onChange={(event) => setCrosshair(event.target.checked)}
                  />{' '}
                  {t('camera.crosshair')}
                </label>
                <label>
                  <input
                    type="checkbox"
                    checked={grid}
                    onChange={(event) => setGrid(event.target.checked)}
                  />{' '}
                  {t('camera.grid')}
                </label>
                <label>
                  {t('camera.textOverlay')}
                  <input
                    maxLength={120}
                    value={overlayText}
                    onChange={(event) => setOverlayText(event.target.value)}
                  />
                </label>
                <details open style={{ gridColumn: '1 / -1' }}>
                  <summary>{t('camera.shapeOverlays')}</summary>
                  <div className="instrument-fields">
                    <label>
                      {t('camera.overlayColor')}
                      <input
                        type="color"
                        value={overlayColor}
                        onChange={(event) => setOverlayColor(event.target.value)}
                      />
                    </label>
                    <label>
                      {t('camera.overlayLineWidth')}
                      <input
                        type="number"
                        min={1}
                        max={16}
                        value={overlayLineWidth}
                        onChange={(event) =>
                          setOverlayLineWidth(Number(event.target.value))
                        }
                      />
                    </label>
                    <label>
                      <input
                        type="checkbox"
                        checked={line.enabled}
                        onChange={(event) =>
                          setLine({ ...line, enabled: event.target.checked })
                        }
                      />{' '}
                      {t('camera.lineOverlay')}
                    </label>
                    {line.enabled && (
                      <>
                        <label>
                          {t('camera.lineStartX')}
                          <input
                            type="number"
                            min={0}
                            max={1}
                            step={0.05}
                            value={line.x}
                            onChange={(event) =>
                              setLine({ ...line, x: Number(event.target.value) })
                            }
                          />
                        </label>
                        <label>
                          {t('camera.lineStartY')}
                          <input
                            type="number"
                            min={0}
                            max={1}
                            step={0.05}
                            value={line.y}
                            onChange={(event) =>
                              setLine({ ...line, y: Number(event.target.value) })
                            }
                          />
                        </label>
                        <label>
                          {t('camera.lineEndX')}
                          <input
                            type="number"
                            min={0}
                            max={1}
                            step={0.05}
                            value={line.x2}
                            onChange={(event) =>
                              setLine({ ...line, x2: Number(event.target.value) })
                            }
                          />
                        </label>
                        <label>
                          {t('camera.lineEndY')}
                          <input
                            type="number"
                            min={0}
                            max={1}
                            step={0.05}
                            value={line.y2}
                            onChange={(event) =>
                              setLine({ ...line, y2: Number(event.target.value) })
                            }
                          />
                        </label>
                      </>
                    )}
                    <label>
                      <input
                        type="checkbox"
                        checked={rectangle.enabled}
                        onChange={(event) =>
                          setRectangle({ ...rectangle, enabled: event.target.checked })
                        }
                      />{' '}
                      {t('camera.rectangleOverlay')}
                    </label>
                    {rectangle.enabled && (
                      <>
                        <label>
                          {t('camera.rectangleStartX')}
                          <input
                            type="number"
                            min={0}
                            max={1}
                            step={0.05}
                            value={rectangle.x}
                            onChange={(event) =>
                              setRectangle({
                                ...rectangle,
                                x: Number(event.target.value),
                              })
                            }
                          />
                        </label>
                        <label>
                          {t('camera.rectangleStartY')}
                          <input
                            type="number"
                            min={0}
                            max={1}
                            step={0.05}
                            value={rectangle.y}
                            onChange={(event) =>
                              setRectangle({
                                ...rectangle,
                                y: Number(event.target.value),
                              })
                            }
                          />
                        </label>
                        <label>
                          {t('camera.rectangleEndX')}
                          <input
                            type="number"
                            min={0}
                            max={1}
                            step={0.05}
                            value={rectangle.x2}
                            onChange={(event) =>
                              setRectangle({
                                ...rectangle,
                                x2: Number(event.target.value),
                              })
                            }
                          />
                        </label>
                        <label>
                          {t('camera.rectangleEndY')}
                          <input
                            type="number"
                            min={0}
                            max={1}
                            step={0.05}
                            value={rectangle.y2}
                            onChange={(event) =>
                              setRectangle({
                                ...rectangle,
                                y2: Number(event.target.value),
                              })
                            }
                          />
                        </label>
                      </>
                    )}
                    <label>
                      <input
                        type="checkbox"
                        checked={scaleBar.enabled}
                        disabled={typeof settings.calibration_mm_per_pixel !== 'number'}
                        onChange={(event) =>
                          setScaleBar({ ...scaleBar, enabled: event.target.checked })
                        }
                      />{' '}
                      {t('camera.scaleBarOverlay')}
                    </label>
                    {scaleBar.enabled && (
                      <>
                        <label>
                          {t('camera.scaleBarX')}
                          <input
                            type="number"
                            min={0}
                            max={1}
                            step={0.05}
                            value={scaleBar.x}
                            onChange={(event) =>
                              setScaleBar({
                                ...scaleBar,
                                x: Number(event.target.value),
                              })
                            }
                          />
                        </label>
                        <label>
                          {t('camera.scaleBarY')}
                          <input
                            type="number"
                            min={0}
                            max={1}
                            step={0.05}
                            value={scaleBar.y}
                            onChange={(event) =>
                              setScaleBar({
                                ...scaleBar,
                                y: Number(event.target.value),
                              })
                            }
                          />
                        </label>
                        <label>
                          {t('camera.scaleBarLength')}
                          <input
                            type="number"
                            min={0.01}
                            max={1000}
                            step="any"
                            value={scaleBar.lengthMM}
                            onChange={(event) =>
                              setScaleBar({
                                ...scaleBar,
                                lengthMM: Number(event.target.value),
                              })
                            }
                          />
                        </label>
                      </>
                    )}
                  </div>
                </details>
                <button type="submit">{t('camera.snapshot')}</button>
              </fieldset>
            </form>
          )}
          {view === 'settings' && controls.length > 0 && (
            <section>
              <h4>{t('camera.hardwareControls')}</h4>
              <form
                onSubmit={(event) => {
                  event.preventDefault();
                  if (!camera) return;
                  const updates = Object.fromEntries(
                    controls
                      .filter(
                        (control) =>
                          !control.read_only &&
                          !control.inactive &&
                          controlValues[control.id] !== undefined &&
                          controlValues[control.id] !== control.current,
                      )
                      .map((control) => [control.id, controlValues[control.id]]),
                  );
                  if (Object.keys(updates).length === 0) return;
                  setControlDraft(null);
                  run(() =>
                    submit({
                      plugin_id: 'camera',
                      operation: 'set_controls',
                      device_id: camera.id,
                      parameters: { controls: updates },
                      timeout_seconds: 30,
                    }),
                  );
                }}
              >
                <fieldset
                  disabled={busy || !canExecute || !ready}
                  className="instrument-fields"
                >
                  {controls.map((control) => (
                    <label key={control.id}>
                      {control.display_name}
                      {control.type === 'boolean' ? (
                        <input
                          type="checkbox"
                          checked={Boolean(controlValues[control.id])}
                          disabled={control.read_only || control.inactive}
                          onChange={(event) =>
                            setControlValue(control.id, event.target.checked)
                          }
                        />
                      ) : control.type === 'menu' ? (
                        <select
                          value={String(
                            controlValues[control.id] ?? control.current ?? '',
                          )}
                          disabled={control.read_only || control.inactive}
                          onChange={(event) =>
                            setControlValue(control.id, Number(event.target.value))
                          }
                        >
                          {(control.choices ?? []).map((choice) => (
                            <option key={choice.value} value={choice.value}>
                              {choice.label}
                            </option>
                          ))}
                        </select>
                      ) : (
                        <input
                          type="number"
                          min={control.minimum}
                          max={control.maximum}
                          step={control.step || 1}
                          value={Number(
                            controlValues[control.id] ?? control.current ?? 0,
                          )}
                          disabled={control.read_only || control.inactive}
                          onChange={(event) =>
                            setControlValue(control.id, Number(event.target.value))
                          }
                        />
                      )}
                    </label>
                  ))}
                  <button
                    type="submit"
                    disabled={controls.every(
                      (control) => control.read_only || control.inactive,
                    )}
                  >
                    {t('camera.applyControls')}
                  </button>
                  <button
                    type="button"
                    onClick={() =>
                      camera &&
                      run(() =>
                        submit({
                          plugin_id: 'camera',
                          operation: 'get_controls',
                          device_id: camera.id,
                          parameters: {},
                          timeout_seconds: 30,
                        }),
                      )
                    }
                  >
                    {t('camera.readControls')}
                  </button>
                  <button
                    type="button"
                    onClick={() =>
                      camera &&
                      run(() =>
                        submit({
                          plugin_id: 'camera',
                          operation: 'reset_controls',
                          device_id: camera.id,
                          parameters: {},
                          timeout_seconds: 30,
                        }),
                      )
                    }
                  >
                    {t('camera.resetControls')}
                  </button>
                </fieldset>
              </form>
            </section>
          )}
          {view === 'snapshot' && history.length > 0 && (
            <section>
              <h4>{t('camera.savedSnapshots')}</h4>
              <div className="table-scroll">
                <table className="data-table">
                  <thead>
                    <tr>
                      <th>{t('camera.captured')}</th>
                      <th>{t('camera.transaction')}</th>
                      <th>{t('camera.image')}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {history.map((job) => (
                      <tr key={job.transaction_id}>
                        <td>{new Date(job.created_at).toLocaleString()}</td>
                        <td>
                          <code>{job.transaction_id}</code>
                        </td>
                        <td>
                          <button
                            type="button"
                            aria-pressed={job.transaction_id === latestID}
                            onClick={() => {
                              setHistorySelection({
                                device: camera!.id,
                                job: job.transaction_id,
                              });
                              setMarkers([]);
                            }}
                          >
                            {t('camera.viewSnapshot')}
                          </button>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </section>
          )}
          {view === 'snapshot' &&
            image &&
            camera &&
            image.device === camera.id &&
            image.job === latestID && (
              <section>
                <h4>{t('camera.imageViewer')}</h4>
                <p>{t('camera.viewerHelp')}</p>
                <div className="instrument-fields">
                  <label>
                    {t('camera.viewZoom')}
                    <input
                      type="range"
                      min={1}
                      max={4}
                      step={0.25}
                      value={viewZoom}
                      onChange={(event) => setViewZoom(Number(event.target.value))}
                    />{' '}
                    {viewZoom}×
                  </label>
                  <label>
                    {t('camera.panHorizontally')}
                    <input
                      type="range"
                      min={0}
                      max={100}
                      value={viewPanX}
                      onChange={(event) => setViewPanX(Number(event.target.value))}
                    />
                  </label>
                  <label>
                    {t('camera.panVertically')}
                    <input
                      type="range"
                      min={0}
                      max={100}
                      value={viewPanY}
                      onChange={(event) => setViewPanY(Number(event.target.value))}
                    />
                  </label>
                  <label>
                    {t('camera.viewRotation')}
                    <select
                      value={viewRotation}
                      onChange={(event) => setViewRotation(Number(event.target.value))}
                    >
                      {[0, 90, 180, 270].map((angle) => (
                        <option key={angle} value={angle}>
                          {angle}°
                        </option>
                      ))}
                    </select>
                  </label>
                  <label>
                    <input
                      type="checkbox"
                      checked={viewFlipHorizontal}
                      onChange={(event) => setViewFlipHorizontal(event.target.checked)}
                    />{' '}
                    {t('camera.viewFlipHorizontal')}
                  </label>
                  <label>
                    <input
                      type="checkbox"
                      checked={viewFlipVertical}
                      onChange={(event) => setViewFlipVertical(event.target.checked)}
                    />{' '}
                    {t('camera.viewFlipVertical')}
                  </label>
                  <label>
                    <input
                      type="checkbox"
                      checked={viewInvert}
                      onChange={(event) => setViewInvert(event.target.checked)}
                    />{' '}
                    {t('camera.viewInvert')}
                  </label>
                  <button
                    type="button"
                    onClick={() => {
                      setViewZoom(1);
                      setViewPanX(50);
                      setViewPanY(50);
                      setViewRotation(0);
                      setViewFlipHorizontal(false);
                      setViewFlipVertical(false);
                      setViewInvert(false);
                      setMarkers([]);
                    }}
                  >
                    {t('camera.resetView')}
                  </button>
                </div>
                <button
                  type="button"
                  aria-label={t('camera.placeMarker')}
                  onClick={(event) => {
                    const bounds = event.currentTarget.getBoundingClientRect();
                    if (!bounds.width || !bounds.height) return;
                    const point = toImagePoint(
                      (event.clientX - bounds.left) / bounds.width,
                      (event.clientY - bounds.top) / bounds.height,
                      viewTransform,
                      bounds.width,
                      bounds.height,
                    );
                    setMarkers((current) => [...current.slice(-25), point]);
                  }}
                  style={{
                    display: 'block',
                    position: 'relative',
                    overflow: 'hidden',
                    padding: 0,
                    maxWidth: '100%',
                    width: 'min(100%, 1280px)',
                    border: 0,
                    cursor: 'crosshair',
                  }}
                >
                  <img
                    src={image.url}
                    alt={t('camera.savedFrom', { name: camera?.display_name ?? '' })}
                    onLoad={(event) =>
                      setImageSize({
                        width: event.currentTarget.clientWidth || 1,
                        height: event.currentTarget.clientHeight || 1,
                      })
                    }
                    style={{
                      display: 'block',
                      width: '100%',
                      height: 'auto',
                      transform: `scale(${viewZoom}) rotate(${viewRotation}deg) scaleX(${viewFlipHorizontal ? -1 : 1}) scaleY(${viewFlipVertical ? -1 : 1})`,
                      filter: viewInvert ? 'invert(1)' : undefined,
                      transformOrigin: `${viewPanX}% ${viewPanY}%`,
                    }}
                  />
                  {markers.map((marker, index) => {
                    const point = toViewerPoint(
                      marker,
                      viewTransform,
                      imageSize.width,
                      imageSize.height,
                    );
                    return (
                      <span
                        key={index}
                        aria-hidden="true"
                        style={{
                          position: 'absolute',
                          left: `${point.x * 100}%`,
                          top: `${point.y * 100}%`,
                          width: 12,
                          height: 12,
                          border: '2px solid #fff',
                          background: '#d10',
                          borderRadius: '50%',
                          transform: 'translate(-50%, -50%)',
                          pointerEvents: 'none',
                        }}
                      />
                    );
                  })}
                </button>
                <p>{t('camera.localMarkers', { count: markers.length })}</p>
              </section>
            )}
        </>
      )}
      {view === 'administration' && dialog && (
        <Modal
          title={`${dialog.kind === 'rebind' ? t('camera.rebind') : t('camera.forget')} ${dialog.device.id}`}
          onDismiss={() => setDialog(null)}
        >
          <form
            className="dialog-form"
            onSubmit={(event) => {
              event.preventDefault();
              if (confirmation !== dialog.device.id || !dialog.device.revision) return;
              const source = dialog.device;
              const target = provisional.find((item) => item.id === provisionalID);
              if (
                dialog.kind === 'rebind' &&
                (!target || !client.rebindInstrumentDevice)
              )
                return;
              if (dialog.kind === 'forget' && !client.forgetInstrumentDevice) return;
              setDialog(null);
              if (dialog.kind === 'rebind' && target)
                run(() =>
                  client.rebindInstrumentDevice!(
                    credential,
                    'camera',
                    source.id,
                    source.revision!,
                    target.id,
                    target.revision ?? 0,
                    confirmation,
                  ),
                );
              else
                run(() =>
                  client.forgetInstrumentDevice!(
                    credential,
                    'camera',
                    source.id,
                    source.revision!,
                    confirmation,
                  ),
                );
            }}
          >
            <p>{t('camera.confirmRecord', { id: dialog.device.id })}</p>
            {dialog.kind === 'rebind' && (
              <label>
                {t('camera.newBinding')}
                <select
                  value={provisionalID}
                  onChange={(event) => setProvisionalID(event.target.value)}
                >
                  {provisional.map((item) => (
                    <option value={item.id} key={item.id}>
                      {item.display_name} ({item.id})
                    </option>
                  ))}
                </select>
              </label>
            )}
            <label>
              {t('camera.cameraID')}
              <input
                autoComplete="off"
                spellCheck={false}
                value={confirmation}
                onChange={(event) => setConfirmation(event.target.value)}
              />
            </label>
            <ModalActions
              confirmLabel={
                dialog.kind === 'rebind'
                  ? t('camera.rebindCamera')
                  : t('camera.forgetCamera')
              }
              working={busy}
              destructive
              onDismiss={() => setDialog(null)}
              blocked={
                confirmation !== dialog.device.id ||
                (dialog.kind === 'rebind' && !provisionalID)
              }
            />
          </form>
        </Modal>
      )}
    </section>
  );
}
