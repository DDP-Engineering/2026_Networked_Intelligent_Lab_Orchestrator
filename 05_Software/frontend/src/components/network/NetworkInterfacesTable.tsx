import { Fragment, useState } from 'react';
import type { NetworkInterface } from '../../api/contract';
import type { DeviceNetwork } from '../../api/devicenetworks';
import type {
  ManagementAccessInterface,
  ManagementAccessStatus,
} from '../../api/management-access';
import { useTranslate } from '../../i18n/languages';
import { DetailsToggle, RowActions, type RowAction } from '../RowActions';
import { uplinkOf } from './addressing';

export interface NetworkInterfacesTableProps {
  interfaces?: readonly NetworkInterface[];
  deviceNetworks?: readonly DeviceNetwork[] | null;
  busy?: boolean;
  onToggle?: (entry: NetworkInterface) => void;
  onConfigure?: (entry: NetworkInterface) => void;
  management?: {
    status: ManagementAccessStatus | null;
    selected: readonly string[];
    disabled: boolean;
    onSelectionChange: (name: string, checked: boolean) => void;
  };
}

function formatRate(bytesPerSecond: number): string {
  if (bytesPerSecond <= 0) {
    return '0 B/s';
  }
  const units = ['B/s', 'kB/s', 'MB/s', 'GB/s'];
  let value = bytesPerSecond;
  let unit = 0;
  while (value >= 1000 && unit < units.length - 1) {
    value /= 1000;
    unit += 1;
  }
  return `${value >= 100 || unit === 0 ? Math.round(value) : value.toFixed(1)} ${units[unit]}`;
}

/** One inventory; saved missing identities must never inherit replacement actions. */
export function NetworkInterfacesTable({
  interfaces = [],
  deviceNetworks,
  busy = false,
  onToggle,
  onConfigure,
  management,
}: NetworkInterfacesTableProps) {
  const t = useTranslate();
  const [expandedInterface, setExpandedInterface] = useState<string | null>(null);
  const accessEntries = management?.status?.interfaces ?? [];
  const matches = (observed: NetworkInterface, access: ManagementAccessInterface) =>
    observed.name === access.name &&
    observed.adapterId.trim().toLowerCase() === access.adapterId.trim().toLowerCase();
  const rows: { observed?: NetworkInterface; access?: ManagementAccessInterface }[] =
    interfaces.map((observed) => ({
      observed,
      access: accessEntries.find((access) => matches(observed, access)),
    }));
  for (const access of accessEntries) {
    if (!interfaces.some((observed) => matches(observed, access)))
      rows.push({ access });
  }
  if (rows.length === 0) return <p>{t('network.noInterfaces')}</p>;
  return (
    <div className="table-scroll">
      <table
        className="data-table record-table network-interfaces-table"
        aria-label={t('network.interfaces.title')}
      >
        <thead>
          <tr>
            <th scope="col">{t('network.table.interface')}</th>
            {management && (
              <th scope="col" className="network-management-cell">
                {t('network.management.enabled')}
              </th>
            )}
            <th scope="col">{t('network.table.state')}</th>
            <th scope="col">{t('network.table.address')}</th>
            <th scope="col">{t('network.table.gateway')}</th>
            <th scope="col">{t('network.table.actions')}</th>
          </tr>
        </thead>
        <tbody>
          {rows.map(({ observed, access }) => {
            const entry = observed ?? access!;
            const rowKey = observed?.id ?? `saved-${entry.adapterId}:${entry.name}`;
            const roleReasonId = `management-role-conflict-${rowKey}`;
            const detailsId = `network-interface-${rowKey}`;
            const expanded = expandedInterface === rowKey;
            const uplink = uplinkOf(interfaces);
            const deviceSide = (deviceNetworks ?? []).some(
              (network) =>
                entry.adapterId !== '' && network.deviceInterface === entry.adapterId,
            );
            const role =
              observed !== undefined && uplink?.id === observed.id
                ? t('network.role.uplink')
                : deviceSide
                  ? t('network.role.deviceSide')
                  : '';
            const stateLabel =
              entry.state === 'connected'
                ? t('network.state.connected')
                : entry.state === 'disconnected'
                  ? t('network.state.disconnected')
                  : t('network.state.unavailable');
            const notReported = (
              <span className="cell-unknown">{t('network.notReported')}</span>
            );
            const actions: RowAction[] = [];
            if (observed?.controllable && onToggle) {
              actions.push({
                label: t(
                  observed.state === 'connected'
                    ? 'network.action.disable'
                    : 'network.action.enable',
                ).replace('{name}', observed.name),
                onSelect: () => onToggle(observed),
                disabled: busy,
                reason: busy ? t('network.busyReason') : undefined,
              });
              if (observed.adapterId !== '' && onConfigure) {
                actions.push({
                  label: t('network.action.configure', { name: observed.name }),
                  onSelect: () => onConfigure(observed),
                  disabled: busy,
                  reason: busy ? t('network.busyReason') : undefined,
                });
              }
            }
            return (
              <Fragment key={rowKey}>
                <tr>
                  <th scope="row">
                    {entry.name}
                    <small className="network-secondary">
                      {entry.kind}
                      {role === '' ? '' : ` · ${role}`}
                      {access?.current ? ` · ${t('network.management.current')}` : ''}
                    </small>
                    {entry.state === 'unavailable' && entry.adapterId !== '' && (
                      <small className="network-secondary">{entry.adapterId}</small>
                    )}
                    {access && !access.roleAvailable && access.role === 'device' && (
                      <small id={roleReasonId} className="network-secondary">
                        {t('network.role.deviceSummary', {
                          name:
                            access.conflictingResourceName ||
                            t('network.role.unnamedDeviceNetwork'),
                        })}
                      </small>
                    )}
                  </th>
                  {management && (
                    <td
                      className="network-management-cell"
                      data-label={t('network.management.enabled')}
                    >
                      <input
                        type="checkbox"
                        aria-label={t('network.management.toggle', {
                          name: entry.name,
                        })}
                        checked={
                          access
                            ? access.state === 'unavailable' && !access.roleAvailable
                              ? access.enabled
                              : management.selected.includes(access.name)
                            : false
                        }
                        disabled={
                          management.disabled ||
                          !access ||
                          access.kind === 'loopback' ||
                          !access.roleAvailable
                        }
                        aria-describedby={
                          access && !access.roleAvailable && access.role === 'device'
                            ? roleReasonId
                            : undefined
                        }
                        onChange={(event) =>
                          management.onSelectionChange(entry.name, event.target.checked)
                        }
                      />
                    </td>
                  )}
                  <td data-label={t('network.table.state')}>
                    <span className="network-state">
                      <span
                        className={`state-dot ${entry.state === 'connected' ? 'state-ok' : 'state-unknown'}`}
                        aria-hidden="true"
                      />
                      {stateLabel}
                    </span>
                  </td>
                  <td
                    className="network-address"
                    data-label={t('network.table.address')}
                  >
                    {[...entry.ipv4, ...entry.ipv6].length === 0
                      ? notReported
                      : [...entry.ipv4, ...entry.ipv6].map((address) => (
                          <div key={address}>{address}</div>
                        ))}
                  </td>
                  <td
                    className="network-address"
                    data-label={t('network.table.gateway')}
                  >
                    {[observed?.gateway4, observed?.gateway6].filter(Boolean).length ===
                    0
                      ? notReported
                      : [observed?.gateway4, observed?.gateway6]
                          .filter(Boolean)
                          .map((gateway) => <div key={gateway}>{gateway}</div>)}
                  </td>
                  <td className="record-cell-actions">
                    {observed && (
                      <RowActions
                        name={entry.name}
                        typeLabel={t('actions.kind.interface')}
                        status={stateLabel}
                        actions={actions}
                      >
                        <DetailsToggle
                          name={entry.name}
                          expanded={expanded}
                          controls={detailsId}
                          onToggle={() =>
                            setExpandedInterface(expanded ? null : rowKey)
                          }
                        />
                      </RowActions>
                    )}
                  </td>
                </tr>
                {expanded && observed && (
                  <tr className="record-details">
                    <td colSpan={management ? 6 : 5}>
                      <div
                        id={detailsId}
                        role="region"
                        aria-label={`${t('network.details')} ${observed.name}`}
                      >
                        <dl className="record-facts">
                          <div>
                            <dt>{t('network.fact.link')}</dt>
                            <dd>
                              {observed.linkCapacityMbps > 0
                                ? `${observed.linkCapacityMbps} Mbit/s`
                                : notReported}
                            </dd>
                          </div>
                          <div>
                            <dt>{t('network.table.rx')}</dt>
                            <dd>{formatRate(observed.rxBytesPerSecond)}</dd>
                          </div>
                          <div>
                            <dt>{t('network.table.tx')}</dt>
                            <dd>{formatRate(observed.txBytesPerSecond)}</dd>
                          </div>
                          <div>
                            <dt>{t('network.fact.adapterId')}</dt>
                            <dd>{observed.adapterId || notReported}</dd>
                          </div>
                          {observed.kind === 'wifi' && (
                            <div>
                              <dt>{t('network.fact.ssid')}</dt>
                              <dd>{observed.ssid || notReported}</dd>
                            </div>
                          )}
                          <div>
                            <dt>{t('network.fact.saved')}</dt>
                            <dd>
                              {observed.settings !== null && observed.settings.saved ? (
                                <>
                                  <div>
                                    {t('network.settings.saved').replace(
                                      '{method}',
                                      observed.settings.method,
                                    )}
                                    {!observed.settings.applied &&
                                      ` (${t('network.settings.notApplied')})`}
                                  </div>
                                  <div>
                                    {t(
                                      observed.settings.managementProtected
                                        ? 'network.fact.protected'
                                        : 'network.fact.notProtected',
                                    )}
                                  </div>
                                  {observed.settings.neverDefault && (
                                    <div>{t('network.fact.neverDefault')}</div>
                                  )}
                                </>
                              ) : (
                                t('network.fact.notSaved')
                              )}
                            </dd>
                          </div>
                          <div>
                            <dt>{t('network.fact.control')}</dt>
                            <dd>
                              {observed.controllable
                                ? t('network.fact.controllable')
                                : observed.notControllableReason || notReported}
                            </dd>
                          </div>
                        </dl>
                      </div>
                    </td>
                  </tr>
                )}
              </Fragment>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}
