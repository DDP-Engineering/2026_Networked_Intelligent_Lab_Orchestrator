import type { CameraSettings } from '../../api/cameraWorkspace';
import type { MessageKey } from '../../i18n/en';
import { useTranslate } from '../../i18n/languages';

/** Preserve calibrated overlays and crop controls alongside the quick effects. */
export function CameraCompositionSettings({
  section,
  settings,
  onChange,
  disabled,
  calibrated,
}: {
  section: 'camera' | 'overlays';
  settings: CameraSettings;
  onChange: (value: CameraSettings) => void;
  disabled: boolean;
  calibrated: boolean;
}) {
  const t = useTranslate();
  const processing = settings.processing;
  const overlayFields: Record<string, [string, MessageKey][]> = {
    line: [
      ['x', 'camera.lineStartX'],
      ['y', 'camera.lineStartY'],
      ['x2', 'camera.lineEndX'],
      ['y2', 'camera.lineEndY'],
    ],
    rectangle: [
      ['x', 'camera.rectangleStartX'],
      ['y', 'camera.rectangleStartY'],
      ['x2', 'camera.rectangleEndX'],
      ['y2', 'camera.rectangleEndY'],
    ],
    scale_bar: [
      ['x', 'camera.scaleBarX'],
      ['y', 'camera.scaleBarY'],
      ['length_mm', 'camera.scaleBarLength'],
    ],
  };
  function updateOverlay(kind: string, values: Record<string, unknown>) {
    onChange({
      ...settings,
      processing: {
        ...processing,
        overlays: (processing.overlays ?? []).map((item) =>
          item.kind === kind ? { ...item, ...values } : item,
        ),
      },
    });
  }
  return (
    <>
      {section === 'camera' && (
        <fieldset disabled={disabled} className="camera-settings-grid">
          {(['center_x', 'center_y'] as const).map((key) => (
            <label key={key}>
              {t(key === 'center_x' ? 'camera.cropX' : 'camera.cropY')}
              <input
                type="range"
                min={0}
                max={1}
                step={0.01}
                value={processing[key] ?? 0.5}
                onChange={(event) =>
                  onChange({
                    ...settings,
                    processing: { ...processing, [key]: Number(event.target.value) },
                  })
                }
              />
              <output>{processing[key] ?? 0.5}</output>
            </label>
          ))}
          <label>
            {t('camera.rotation')}
            <select
              value={processing.rotation ?? 0}
              onChange={(event) =>
                onChange({
                  ...settings,
                  processing: { ...processing, rotation: Number(event.target.value) },
                })
              }
            >
              {[0, 90, 180, 270].map((angle) => (
                <option key={angle} value={angle}>
                  {angle}°
                </option>
              ))}
            </select>
          </label>
        </fieldset>
      )}
      {section === 'overlays' && (
        <fieldset disabled={disabled} className="camera-settings-grid">
          {(['line', 'rectangle', 'scale_bar'] as const).map((kind) => {
            const item = processing.overlays?.find((overlay) => overlay.kind === kind);
            return (
              <div key={kind}>
                <label className="field-check">
                  <input
                    type="checkbox"
                    disabled={kind === 'scale_bar' && !calibrated}
                    checked={!!item}
                    onChange={(event) =>
                      onChange({
                        ...settings,
                        processing: {
                          ...processing,
                          overlays: [
                            ...(processing.overlays ?? []).filter(
                              (overlay) => overlay.kind !== kind,
                            ),
                            ...(event.target.checked
                              ? [
                                  {
                                    kind,
                                    x: 0.1,
                                    y: 0.1,
                                    x2: 0.8,
                                    y2: 0.8,
                                    length_mm: 1,
                                    color: '#ffffff',
                                    line_width: 2,
                                  },
                                ]
                              : []),
                          ],
                        },
                      })
                    }
                  />
                  {t(
                    kind === 'line'
                      ? 'camera.lineOverlay'
                      : kind === 'rectangle'
                        ? 'camera.rectangleOverlay'
                        : 'camera.scaleBarOverlay',
                  )}
                </label>
                {item && (
                  <div className="camera-settings-grid">
                    {overlayFields[kind].map(([key, label]) => (
                      <label key={key}>
                        {t(label)}
                        <input
                          type="number"
                          min={key === 'length_mm' ? 0.01 : 0}
                          max={key === 'length_mm' ? 1000 : 1}
                          step={0.01}
                          value={Number(item[key] ?? 0.5)}
                          onChange={(event) =>
                            updateOverlay(kind, { [key]: Number(event.target.value) })
                          }
                        />
                      </label>
                    ))}
                    <label>
                      {t('camera.overlayColor')}
                      <input
                        type="color"
                        value={String(item.color ?? '#ffffff')}
                        onChange={(event) =>
                          updateOverlay(kind, { color: event.target.value })
                        }
                      />
                    </label>
                    <label>
                      {t('camera.overlayLineWidth')}
                      <input
                        type="number"
                        min={1}
                        max={16}
                        value={Number(item.line_width ?? 2)}
                        onChange={(event) =>
                          updateOverlay(kind, {
                            line_width: Number(event.target.value),
                          })
                        }
                      />
                    </label>
                  </div>
                )}
              </div>
            );
          })}
        </fieldset>
      )}
    </>
  );
}
