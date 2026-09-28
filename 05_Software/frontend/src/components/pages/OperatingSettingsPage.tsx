import { useEffect, useState, type FormEvent } from 'react';

import { BenchCoreError } from '../../api/client';
import {
  OPERATING_BOUNDS,
  OPERATING_FIELDS,
  type OperatingField,
  type OperatingSettingsState,
  type OperatingSettingsValues,
} from '../../api/operating-settings';
import type { MessageKey } from '../../i18n/en';
import { useTranslate } from '../../i18n/languages';
import { HelpPopover } from '../help/HelpPopover';

interface Props {
  read: () => Promise<OperatingSettingsState>;
  save: (
    revision: number,
    settings: OperatingSettingsValues,
  ) => Promise<OperatingSettingsState>;
  onOpenActivity?: () => void;
}

export type OperatingSettingsScope = 'platform' | 'camera' | 'serial';

const fieldsByScope: Record<OperatingSettingsScope, readonly OperatingField[]> = {
  platform: [
    'executing_jobs',
    'outstanding_jobs',
    'default_job_timeout_seconds',
    'max_job_timeout_seconds',
  ],
  camera: ['camera_streams', 'camera_lifetime_minutes'],
  serial: ['serial_connections', 'serial_lifetime_minutes', 'serial_idle_minutes'],
};

const titleByScope: Record<OperatingSettingsScope, MessageKey> = {
  platform: 'platform.title',
  camera: 'platform.cameraTitle',
  serial: 'platform.serialTitle',
};

const labels: Record<OperatingField, MessageKey> = {
  executing_jobs: 'platform.executingJobs',
  outstanding_jobs: 'platform.outstandingJobs',
  serial_connections: 'platform.serialConnections',
  camera_streams: 'platform.cameraStreams',
  default_job_timeout_seconds: 'platform.defaultJobTimeout',
  max_job_timeout_seconds: 'platform.maxJobTimeout',
  serial_lifetime_minutes: 'platform.serialLifetime',
  serial_idle_minutes: 'platform.serialIdle',
  camera_lifetime_minutes: 'platform.cameraLifetime',
};

const descriptions: Record<OperatingField, MessageKey> = {
  executing_jobs: 'platform.help.executingJobs',
  outstanding_jobs: 'platform.help.outstandingJobs',
  serial_connections: 'platform.help.serialConnections',
  camera_streams: 'platform.help.cameraStreams',
  default_job_timeout_seconds: 'platform.help.defaultJobTimeout',
  max_job_timeout_seconds: 'platform.help.maxJobTimeout',
  serial_lifetime_minutes: 'platform.help.serialLifetime',
  serial_idle_minutes: 'platform.help.serialIdle',
  camera_lifetime_minutes: 'platform.help.cameraLifetime',
};

function valid(settings: OperatingSettingsValues): boolean {
  return (
    OPERATING_FIELDS.every((field) => {
      const value = settings[field];
      const bounds = OPERATING_BOUNDS[field];
      return Number.isInteger(value) && value >= bounds.min && value <= bounds.max;
    }) &&
    settings.default_job_timeout_seconds <= settings.max_job_timeout_seconds &&
    settings.serial_idle_minutes <= settings.serial_lifetime_minutes
  );
}

/** The same revisioned appliance settings, shown at the screen that owns each field. */
export function OperatingSettingsSection({
  scope,
  read,
  save,
  onOpenActivity,
}: Props & { scope: OperatingSettingsScope }) {
  const t = useTranslate();
  const [stored, setStored] = useState<OperatingSettingsState | null>(null);
  const [draft, setDraft] = useState<OperatingSettingsValues | null>(null);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState('');
  const [error, setError] = useState('');

  useEffect(() => {
    let active = true;
    void read().then(
      (state) => {
        if (!active) return;
        setStored(state);
        setDraft(state.settings);
      },
      () => {
        if (active) setError(t('platform.readFailed'));
      },
    );
    return () => {
      active = false;
    };
  }, [read, t]);

  const submit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (stored === null || draft === null) return;
    setMessage('');
    if (!valid(draft)) {
      setError(t('platform.invalid'));
      return;
    }
    setBusy(true);
    setError('');
    try {
      const state = await save(stored.revision, draft);
      setStored(state);
      setDraft(state.settings);
      setMessage(t('platform.saved'));
    } catch (failure) {
      setError(
        failure instanceof BenchCoreError &&
          failure.code === 'operating_settings_conflict'
          ? t('platform.conflict')
          : t('platform.saveFailed'),
      );
    } finally {
      setBusy(false);
    }
  };

  return (
    <section className="form-group operating-settings">
      <div className="operating-settings-head">
        <div className="page-title-actions">
          {scope === 'platform' ? (
            <h1>{t(titleByScope[scope])}</h1>
          ) : (
            <h2>{t(titleByScope[scope])}</h2>
          )}
          {scope === 'platform' && onOpenActivity && (
            <button type="button" onClick={onOpenActivity}>
              {t('network.activity.open')}
            </button>
          )}
        </div>
        <button
          type="submit"
          form={`operating-settings-${scope}`}
          className="button button-primary"
          disabled={busy || stored === null || draft === null}
        >
          {t('platform.save')}
        </button>
      </div>
      {stored === null || draft === null ? (
        error ? (
          <p role="alert">{error}</p>
        ) : (
          <p role="status">{t('platform.reading')}</p>
        )
      ) : (
        <form
          id={`operating-settings-${scope}`}
          onSubmit={(event) => void submit(event)}
        >
          {error && <p role="alert">{error}</p>}
          {message && <p role="status">{message}</p>}
          <div className="operating-settings-scroll">
            <table className="operating-settings-table">
              <thead>
                <tr>
                  <th scope="col">{t('platform.column.setting')}</th>
                  <th scope="col">{t('platform.column.range')}</th>
                  <th scope="col">{t('platform.column.value')}</th>
                  <th scope="col">{t('platform.column.usage')}</th>
                </tr>
              </thead>
              <tbody>
                {fieldsByScope[scope].map((field) => {
                  const bounds = OPERATING_BOUNDS[field];
                  const used =
                    field in stored.usage
                      ? stored.usage[field as keyof typeof stored.usage]
                      : undefined;
                  return (
                    <tr key={field}>
                      <th scope="row">
                        <label htmlFor={`operating-${field}`}>{t(labels[field])}</label>
                        <HelpPopover name={t(labels[field])}>
                          {t(descriptions[field])}
                        </HelpPopover>
                      </th>
                      <td>
                        {bounds.min}–{bounds.max} {t(`platform.unit.${bounds.unit}`)}
                      </td>
                      <td>
                        <input
                          id={`operating-${field}`}
                          type="number"
                          inputMode="numeric"
                          min={bounds.min}
                          max={bounds.max}
                          step={1}
                          required
                          value={draft[field]}
                          disabled={busy}
                          onChange={(event) =>
                            setDraft({ ...draft, [field]: Number(event.target.value) })
                          }
                        />
                      </td>
                      <td>
                        {used === undefined
                          ? '—'
                          : t('platform.usage', {
                              used: String(used),
                              limit: String(stored.settings[field]),
                            })}
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        </form>
      )}
    </section>
  );
}

/** Platform-owned job controls in the Nilo Settings destination. */
export function OperatingSettingsPage(props: Props) {
  return <OperatingSettingsSection {...props} scope="platform" />;
}
