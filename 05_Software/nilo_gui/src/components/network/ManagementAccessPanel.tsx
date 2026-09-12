import { useCallback, useEffect, useMemo, useState } from 'react';

import type {
  AdapterRoleConflict,
  ManagementAccessStatus,
} from '../../api/management-access';
import { BenchCoreError } from '../../api/client';
import type { MessageKey } from '../../i18n/en';
import { useTranslate } from '../../i18n/languages';
import { WorkflowHelp } from '../help/WorkflowHelp';

export interface ManagementAccessPanelProps {
  read: () => Promise<ManagementAccessStatus>;
  /** Shared observation owned by NetworkPage; omitted in isolated use/tests. */
  observedStatus?: ManagementAccessStatus | null;
  onStatusChange?: (status: ManagementAccessStatus) => void;
  update: (
    interfaces: string[],
    confirmCurrentInterface: boolean,
  ) => Promise<ManagementAccessStatus>;
  confirm: () => Promise<ManagementAccessStatus>;
  recover: () => Promise<ManagementAccessStatus>;
}

const ACCESS_STATE_KEYS: Record<ManagementAccessStatus['state'], MessageKey> = {
  pending: 'network.management.state.pending',
  applied: 'network.management.state.applied',
  failed: 'network.management.state.failed',
  recovered: 'network.management.state.recovered',
};

const INTERFACE_STATE_KEYS: Record<
  ManagementAccessStatus['interfaces'][number]['state'],
  MessageKey
> = {
  connected: 'network.state.connected',
  disconnected: 'network.state.disconnected',
  unavailable: 'network.state.unavailable',
};

interface FailureNotice {
  detail: string;
  conflict: AdapterRoleConflict | null;
}

function sameAdapter(first: string, second: string): boolean {
  return first.trim().toLowerCase() === second.trim().toLowerCase();
}

function reconcileFailure(
  current: FailureNotice | null,
  status: ManagementAccessStatus,
): FailureNotice | null {
  if (current === null) return null;
  const conflict = current.conflict;
  if (conflict === null) return null;
  return status.interfaces.some(
    (entry) =>
      sameAdapter(entry.adapterId, conflict.adapterId) &&
      entry.role === conflict.existingRole,
  )
    ? current
    : null;
}

/** Administrator-only management listener configuration and recovery. */
export function ManagementAccessPanel({
  read,
  observedStatus,
  onStatusChange,
  update,
  confirm,
  recover,
}: ManagementAccessPanelProps) {
  const t = useTranslate();
  const [localStatus, setLocalStatus] = useState<ManagementAccessStatus | null>(null);
  const [selected, setSelected] = useState<string[] | null>(null);
  const [working, setWorking] = useState(false);
  const [failure, setFailure] = useState<FailureNotice | null>(null);
  const [warnCurrent, setWarnCurrent] = useState(false);
  const status = observedStatus === undefined ? localStatus : observedStatus;
  const selectedInterfaces = useMemo(
    () => selected ?? status?.configuredInterfaces ?? [],
    [selected, status],
  );
  const outcomeDetail = useMemo(() => {
    if (status === null || status.outcome.code === '') return '';
    switch (status.outcome.code) {
      case 'startup_recovered':
        return t('network.management.outcome.startupRecovered');
      case 'console_confirmation_required':
        return t('network.management.outcome.consolePending');
      case 'role_conflict_recovered':
        return t('network.management.outcome.roleConflictRecovered', {
          interface: status.outcome.interface || t('network.dn.unnamed'),
          resource:
            status.outcome.resourceName || t('network.role.unnamedDeviceNetwork'),
        });
      case 'proposed_change_restored':
        return t('network.management.outcome.proposedRestored');
      case 'recovery_failed':
      case 'automatic_recovery_failed':
        return t('network.management.outcome.recoveryFailed');
      case 'automatic_change_restored':
        return t('network.management.outcome.automaticRestored');
    }
  }, [status, t]);
  const describeFailure = useCallback(
    (error: unknown): FailureNotice => {
      if (error instanceof BenchCoreError) {
        const conflict = error.adapterRoleConflict;
        if (conflict?.existingRole === 'device') {
          return {
            detail: t('network.role.deviceUnavailable', {
              name:
                conflict.conflictingResourceName ||
                t('network.role.unnamedDeviceNetwork'),
            }),
            conflict,
          };
        }
        if (conflict?.existingRole === 'management') {
          return {
            detail: t('network.role.managementUnavailable', {
              name: conflict.interface || conflict.adapterId,
            }),
            conflict,
          };
        }
        if (error.code === 'network_interface_role_conflict') {
          return { detail: t('network.role.malformedConflict'), conflict: null };
        }
        return { detail: error.detail, conflict: null };
      }
      return {
        detail: error instanceof Error ? error.message : String(error),
        conflict: null,
      };
    },
    [t],
  );

  const load = useCallback(async () => {
    try {
      const value = await read();
      setLocalStatus(value);
      onStatusChange?.(value);
      setSelected(null);
      setFailure((current) => reconcileFailure(current, value));
    } catch (error) {
      setFailure(describeFailure(error));
    }
  }, [describeFailure, onStatusChange, read]);

  useEffect(() => {
    if (observedStatus !== undefined) return;
    let abandoned = false;
    const refresh = async () => {
      try {
        const value = await read();
        if (!abandoned) {
          setLocalStatus(value);
          setFailure((current) => reconcileFailure(current, value));
        }
      } catch (error) {
        if (!abandoned) setFailure(describeFailure(error));
      }
    };
    void refresh();
    const timer = setInterval(() => void refresh(), 5000);
    return () => {
      abandoned = true;
      clearInterval(timer);
    };
  }, [describeFailure, observedStatus, read]);

  useEffect(() => {
    if (observedStatus !== undefined && observedStatus !== null) {
      setFailure((current) => reconcileFailure(current, observedStatus));
    }
  }, [observedStatus]);

  const usableSelected = useMemo(
    () =>
      status?.interfaces.some(
        (entry) => entry.usable && selectedInterfaces.includes(entry.name),
      ) ?? false,
    [selectedInterfaces, status],
  );

  const apply = useCallback(
    async (confirmCurrentInterface: boolean) => {
      setWorking(true);
      setFailure(null);
      try {
        const value = await update(selectedInterfaces, confirmCurrentInterface);
        setLocalStatus(value);
        onStatusChange?.(value);
        setSelected(null);
        setWarnCurrent(false);
      } catch (error) {
        setFailure(describeFailure(error));
      } finally {
        setWorking(false);
      }
    },
    [describeFailure, onStatusChange, selectedInterfaces, update],
  );

  const save = () => {
    if (
      status?.currentInterface &&
      !selectedInterfaces.includes(status.currentInterface)
    ) {
      setWarnCurrent(true);
      return;
    }
    void apply(false);
  };

  const settle = async (operation: () => Promise<ManagementAccessStatus>) => {
    setWorking(true);
    setFailure(null);
    try {
      const value = await operation();
      setLocalStatus(value);
      onStatusChange?.(value);
      setSelected(null);
    } catch (error) {
      setFailure(describeFailure(error));
    } finally {
      setWorking(false);
    }
  };

  return (
    <section aria-labelledby="management-access-title" className="management-access">
      <div className="network-section-head">
        <div>
          <h3 id="management-access-title">{t('network.management.title')}</h3>
          <p>{t('network.management.body')}</p>
        </div>
        <div className="form-actions">
          <WorkflowHelp
            prefix="help.roles"
            title={t('help.roles.title')}
            buttonLabel={t('help.roles.open')}
          />
          <button type="button" onClick={() => void load()} disabled={working}>
            {t('network.management.refresh')}
          </button>
        </div>
      </div>

      {failure !== null && (
        <p className="notice notice-unknown" role="alert">
          {failure.detail}
        </p>
      )}
      {status === null ? (
        <p>{t('network.reading')}</p>
      ) : (
        <>
          <p
            className={`notice ${status.state === 'failed' ? 'notice-unknown' : ''}`}
            role="status"
          >
            {t(ACCESS_STATE_KEYS[status.state])}
            {outcomeDetail === '' ? '' : ` ${outcomeDetail}`}
          </p>
          {status.tlsMissing.length > 0 && (
            <p className="notice notice-unknown" role="alert">
              {t('network.management.tlsMissing', {
                names: status.tlsMissing.join(', '),
              })}
            </p>
          )}
          <div className="table-scroll">
            <table className="data-table record-table">
              <thead>
                <tr>
                  <th scope="col">{t('network.management.enabled')}</th>
                  <th scope="col">{t('network.table.interface')}</th>
                  <th scope="col">{t('network.table.state')}</th>
                  <th scope="col">{t('network.table.address')}</th>
                </tr>
              </thead>
              <tbody>
                {status.interfaces.map((entry) => (
                  <tr key={entry.name}>
                    <td>
                      <input
                        type="checkbox"
                        aria-label={t('network.management.toggle', {
                          name: entry.name,
                        })}
                        checked={selectedInterfaces.includes(entry.name)}
                        disabled={
                          working || entry.kind === 'loopback' || !entry.roleAvailable
                        }
                        aria-describedby={
                          !entry.roleAvailable
                            ? `management-role-conflict-${entry.adapterId}`
                            : undefined
                        }
                        onChange={(event) =>
                          setSelected((current) => {
                            const selection = current ?? selectedInterfaces;
                            return event.target.checked
                              ? [...selection, entry.name]
                              : selection.filter((name) => name !== entry.name);
                          })
                        }
                      />
                      {!entry.roleAvailable && entry.role === 'device' && (
                        <small
                          id={`management-role-conflict-${entry.adapterId}`}
                          className="network-secondary"
                        >
                          {t('network.role.deviceUnavailable', {
                            name:
                              entry.conflictingResourceName ||
                              t('network.role.unnamedDeviceNetwork'),
                          })}
                        </small>
                      )}
                    </td>
                    <th scope="row">
                      {entry.name}
                      <small className="network-secondary">
                        {entry.kind}
                        {entry.current ? ` · ${t('network.management.current')}` : ''}
                      </small>
                    </th>
                    <td>{t(INTERFACE_STATE_KEYS[entry.state])}</td>
                    <td className="network-address">
                      {[...entry.ipv4, ...entry.ipv6].join(', ') ||
                        t('network.notReported')}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          {!usableSelected && (
            <p className="notice notice-unknown" role="alert">
              {t('network.management.noUsable')}
            </p>
          )}
          <div className="form-actions">
            <button type="button" onClick={save} disabled={working || !usableSelected}>
              {t('network.management.apply')}
            </button>
          </div>
          {warnCurrent && (
            <div
              className="notice notice-unknown"
              role="alertdialog"
              aria-labelledby="management-current-warning"
            >
              <h4 id="management-current-warning">
                {t('network.management.currentWarning')}
              </h4>
              <p>
                {t('network.management.currentWarningBody', {
                  name: status.currentInterface,
                })}
              </p>
              <button type="button" onClick={() => void apply(true)} disabled={working}>
                {t('network.management.currentApply')}
              </button>
              <button
                type="button"
                onClick={() => setWarnCurrent(false)}
                disabled={working}
              >
                {t('network.change.cancel')}
              </button>
            </div>
          )}
          {status.state === 'pending' && (
            <div className="notice" role="status">
              <p>{t('network.management.pendingHelp')}</p>
              <button
                type="button"
                onClick={() => void settle(confirm)}
                disabled={working}
              >
                {t('network.change.keep')}
              </button>
              <button
                type="button"
                onClick={() => void settle(recover)}
                disabled={working}
              >
                {t('network.change.restore')}
              </button>
            </div>
          )}
          <h4>{t('network.management.urls')}</h4>
          <ul>
            {status.urls.map((entry) => (
              <li key={entry.url}>
                <a href={entry.url}>{entry.url}</a> · {entry.kind}
              </li>
            ))}
          </ul>
          <p>{t('network.management.localOnly')}</p>
          <p>{t('network.management.dhcp')}</p>
        </>
      )}
    </section>
  );
}
