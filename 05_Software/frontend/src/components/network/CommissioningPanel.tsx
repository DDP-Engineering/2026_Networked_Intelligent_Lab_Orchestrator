import { useCallback, useMemo, useState } from 'react';

import { BenchCoreError } from '../../api/client';
import type {
  ManagementAccessInterface,
  ManagementAccessStatus,
} from '../../api/management-access';
import { useTranslate } from '../../i18n/languages';

export interface CommissioningPanelProps {
  status: ManagementAccessStatus;
  canManage: boolean;
  busy: boolean;
  /** Ends first boot on the chosen interfaces. */
  complete: (
    interfaces: string[],
    confirmCurrentInterface: boolean,
  ) => Promise<ManagementAccessStatus>;
}

/**
 * Selectable means the appliance would accept the interface as a permanent
 * management path. An interface it is merely serving on right now - a container
 * interface would never be one - is shown but cannot be chosen, and says why.
 */
function selectable(entry: ManagementAccessInterface): boolean {
  return entry.enabled && entry.roleAvailable;
}

/**
 * The first-boot workflow.
 *
 * While the appliance is provisioning it answers on every eligible adapter,
 * which is the widest exposure it ever has. This panel says so first, shows what
 * is currently carrying management access, and ends that state by fixing the
 * allowlist to what the administrator chooses.
 */
export function CommissioningPanel({
  status,
  canManage,
  busy,
  complete,
}: CommissioningPanelProps) {
  const t = useTranslate();
  const eligible = useMemo(
    () => status.interfaces.filter(selectable),
    [status.interfaces],
  );
  const ineligible = useMemo(
    () => status.interfaces.filter((entry) => !selectable(entry)),
    [status.interfaces],
  );
  const [selected, setSelected] = useState<string[] | null>(null);
  const chosen = selected ?? eligible.map((entry) => entry.name);
  const [failure, setFailure] = useState<string | null>(null);
  const [warnCurrent, setWarnCurrent] = useState(false);
  const [working, setWorking] = useState(false);

  const toggle = useCallback(
    (name: string) => {
      setFailure(null);
      setWarnCurrent(false);
      setSelected((current) => {
        const base = current ?? eligible.map((entry) => entry.name);
        return base.includes(name)
          ? base.filter((entry) => entry !== name)
          : [...base, name];
      });
    },
    [eligible],
  );

  const send = useCallback(
    async (confirmCurrentInterface: boolean) => {
      setWorking(true);
      try {
        await complete(chosen, confirmCurrentInterface);
        setWarnCurrent(false);
        setFailure(null);
      } catch (error) {
        setFailure(
          error instanceof BenchCoreError
            ? error.detail
            : t('network.commissioning.failed'),
        );
      } finally {
        setWorking(false);
      }
    },
    [chosen, complete, t],
  );

  const submit = useCallback(() => {
    setFailure(null);
    if (chosen.length === 0) {
      setFailure(t('network.commissioning.noSelection'));
      return;
    }
    const givingUpCurrent =
      status.currentInterface !== '' && !chosen.includes(status.currentInterface);
    if (givingUpCurrent && !warnCurrent) {
      setWarnCurrent(true);
      return;
    }
    void send(givingUpCurrent);
  }, [chosen, send, status.currentInterface, t, warnCurrent]);

  return (
    <section aria-labelledby="commissioning-title">
      <h2 id="commissioning-title">{t('network.commissioning.title')}</h2>

      <p role="alert" data-testid="commissioning-warning">
        {t('network.commissioning.temporaryWarning')}
      </p>
      <p>{t('network.commissioning.body')}</p>

      <ul data-testid="commissioning-interfaces">
        {eligible.map((entry) => (
          <li key={entry.name} data-testid={`commissioning-interface-${entry.name}`}>
            <label>
              <input
                type="checkbox"
                checked={chosen.includes(entry.name)}
                disabled={!canManage || busy || working}
                onChange={() => toggle(entry.name)}
                aria-label={t('network.commissioning.select', { name: entry.name })}
              />
              <span>{entry.name}</span>
            </label>
            <span>{entry.kind}</span>
            <span>{entry.adapterId}</span>
            <span>{t(`network.state.${entry.state}` as const)}</span>
            <span>{[...entry.ipv4, ...entry.ipv6].join(', ')}</span>
            {entry.current && <span>{t('network.commissioning.current')}</span>}
          </li>
        ))}
      </ul>

      {ineligible.length > 0 && (
        <ul data-testid="commissioning-ineligible">
          {ineligible.map((entry) => (
            <li key={entry.name}>
              <span>{entry.name}</span>
              <span>{entry.roleDetail || t('network.commissioning.notEligible')}</span>
            </li>
          ))}
        </ul>
      )}

      <div data-testid="commissioning-urls">
        <h3>{t('network.commissioning.urls')}</h3>
        <ul>
          {status.urls.map((url) => (
            <li key={url.url}>{url.url}</li>
          ))}
        </ul>
        <p>{t('network.commissioning.localOnly')}</p>
        <p>{t('network.commissioning.dhcp')}</p>
      </div>

      {status.tlsMissing.length > 0 && (
        <p data-testid="commissioning-tls-missing">
          {t('network.commissioning.tlsMissing', {
            names: status.tlsMissing.join(', '),
          })}
        </p>
      )}

      {failure !== null && (
        <p role="alert" data-testid="commissioning-failure">
          {failure}
        </p>
      )}

      {warnCurrent && (
        <div data-testid="commissioning-current-warning">
          <p role="alert">
            {t('network.commissioning.currentWarning', {
              name: status.currentInterface,
            })}
          </p>
          <button
            type="button"
            disabled={busy || working}
            onClick={() => void send(true)}
          >
            {t('network.commissioning.currentConfirm', {
              name: status.currentInterface,
            })}
          </button>
        </div>
      )}

      {canManage && (
        <button type="button" disabled={busy || working} onClick={submit}>
          {t('network.commissioning.complete')}
        </button>
      )}
    </section>
  );
}
