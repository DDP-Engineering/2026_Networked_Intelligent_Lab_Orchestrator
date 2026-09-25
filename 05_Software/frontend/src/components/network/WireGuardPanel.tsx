import { Fragment, useState } from 'react';
import type { WireGuardChange, WireGuardStatus } from '../../api/wireguard';
import { useTranslate } from '../../i18n/languages';
import { DetailsToggle, RowActions, type RowAction } from '../RowActions';

/**
 * The WireGuard profiles.
 *
 * One markup per profile: name, state, tunnel address and the startup
 * selection in the row, everything else behind Details, and every permitted
 * change in one compact Actions dialog. Proposals go through the page's preview; nothing
 * here applies anything.
 */
export function WireGuardPanel({
  status,
  canManage,
  canInstall,
  propose,
  busy = false,
  showDiagnostics = true,
}: {
  status: WireGuardStatus | null;
  canManage: boolean;
  canInstall: boolean;
  propose: (change: WireGuardChange) => void;
  busy?: boolean;
  /** Kept for standalone use; the Network VPN page moves it to Diagnostics. */
  showDiagnostics?: boolean;
}) {
  const t = useTranslate();
  const manageProfiles = canManage && !status?.readinessOnly;
  const [importOpen, setImportOpen] = useState(false);
  const [expandedProfile, setExpandedProfile] = useState<string | null>(null);
  const [renaming, setRenaming] = useState('');
  const [name, setName] = useState('');
  const [file, setFile] = useState<File | null>(null);
  const [error, setError] = useState(false);
  const importProfile = async () => {
    if (file === null || name.trim() === '' || file.size > 48 * 1024) {
      setError(true);
      return;
    }
    try {
      const text = await file.text();
      propose({ action: 'import', target: name.trim(), profile_text: text });
      setError(false);
    } catch {
      setError(true);
    }
  };
  const missing = <span className="cell-unknown">{t('network.notReported')}</span>;
  const canChange = (profile: WireGuardStatus['profiles'][number]) =>
    manageProfiles && profile.niloOwned && status?.helperAvailable === true;

  return (
    <section aria-labelledby="network-wireguard">
      <div className="network-section-head">
        <h3 id="network-wireguard">{t('network.wireguard.title')}</h3>
        {manageProfiles && status?.reported && status.helperAvailable && (
          <button
            type="button"
            aria-expanded={importOpen}
            aria-controls="network-import"
            onClick={() => setImportOpen(!importOpen)}
          >
            {t('network.wg.add')}
          </button>
        )}
      </div>
      {status === null || !status.reported ? (
        <p>{t('network.wireguard.notReported')}</p>
      ) : status.readinessOnly ? (
        <p>{t('network.wg.profilesNotReported')}</p>
      ) : status.profiles.length === 0 ? (
        <p>{t('network.wg.empty')}</p>
      ) : (
        <div className="table-scroll">
          <table className="data-table record-table">
            <thead>
              <tr>
                <th scope="col">{t('auth.field.name')}</th>
                <th scope="col">{t('network.table.state')}</th>
                <th scope="col">{t('network.table.address')}</th>
                <th scope="col">{t('network.wg.startup')}</th>
                <th scope="col">{t('network.table.actions')}</th>
              </tr>
            </thead>
            <tbody>
              {status.profiles.map((profile) => {
                const detailsId = `network-profile-${profile.id}`;
                const expanded = expandedProfile === profile.id;
                const actions: RowAction[] = canChange(profile)
                  ? [
                      {
                        label: `${t(profile.active ? 'network.wg.disable' : 'network.wg.enable')} ${profile.name}`,
                        onSelect: () =>
                          propose({
                            action: profile.active ? 'deactivate' : 'activate',
                            target: profile.id,
                          }),
                      },
                      {
                        label: `${t('network.wg.remove')} ${profile.name}`,
                        onSelect: () =>
                          propose({ action: 'remove', target: profile.id }),
                        disabled: profile.active,
                        reason: profile.active
                          ? t('network.wg.removeActive')
                          : undefined,
                        danger: true,
                      },
                    ]
                  : [];
                const stateLabel = t(
                  profile.active ? 'network.tunnel.active' : 'network.tunnel.inactive',
                );
                const endpoints = [
                  ...new Set(
                    (profile.peers ?? []).map((peer) => peer.endpoint).filter(Boolean),
                  ),
                ];
                const routes = [
                  ...new Set((profile.peers ?? []).flatMap((peer) => peer.allowedIPs)),
                ];
                return (
                  <Fragment key={profile.id}>
                    <tr>
                      <th scope="row">
                        {profile.name}
                        {!profile.niloOwned && (
                          <small className="network-secondary">
                            {t('network.wg.external')}
                          </small>
                        )}
                      </th>
                      <td data-label={t('network.table.state')}>
                        <span className="network-state">
                          <span
                            className={`state-dot ${profile.active ? 'state-ok' : 'state-unknown'}`}
                            aria-hidden="true"
                          />
                          {stateLabel}
                        </span>
                      </td>
                      <td
                        className="network-address"
                        data-label={t('network.table.address')}
                      >
                        {profile.addresses?.length
                          ? profile.addresses.map((address) => (
                              <div key={address}>{address}</div>
                            ))
                          : missing}
                      </td>
                      <td data-label={t('network.wg.startup')}>
                        {canChange(profile) ? (
                          <label className="field-check">
                            <input
                              type="checkbox"
                              aria-label={`${t('network.wg.startup')}: ${profile.name}`}
                              checked={profile.autoconnect}
                              onChange={(event) =>
                                propose({
                                  action: 'autostart',
                                  target: profile.id,
                                  enabled: event.target.checked,
                                })
                              }
                            />
                            <span>
                              {t(
                                profile.autoconnect
                                  ? 'network.wg.enabled'
                                  : 'network.wg.disabled',
                              )}
                            </span>
                          </label>
                        ) : (
                          t(
                            profile.autoconnect
                              ? 'network.wg.enabled'
                              : 'network.wg.disabled',
                          )
                        )}
                      </td>
                      <td className="record-cell-actions">
                        <RowActions
                          name={profile.name}
                          typeLabel={t('actions.kind.wireguardProfile')}
                          status={stateLabel}
                          actions={actions}
                        >
                          <DetailsToggle
                            name={profile.name}
                            expanded={expanded}
                            controls={detailsId}
                            onToggle={() =>
                              setExpandedProfile(expanded ? null : profile.id)
                            }
                          />
                        </RowActions>
                      </td>
                    </tr>
                    {expanded && (
                      <tr className="record-details">
                        <td colSpan={5}>
                          <div
                            id={detailsId}
                            role="region"
                            aria-label={`${t('network.details')} ${profile.name}`}
                          >
                            <dl className="record-facts">
                              <div>
                                <dt>{t('network.wg.endpoint')}</dt>
                                <dd>
                                  {endpoints.length > 0
                                    ? endpoints.map((endpoint) => (
                                        <div key={endpoint}>{endpoint}</div>
                                      ))
                                    : missing}
                                </dd>
                              </div>
                              <div>
                                <dt>{t('network.fact.handshake')}</dt>
                                <dd>
                                  {profile.lastHandshake?.toLocaleString() ?? missing}
                                </dd>
                              </div>
                              <div>
                                <dt>DNS</dt>
                                <dd>{profile.dns?.join(', ') || missing}</dd>
                              </div>
                              <div>
                                <dt>{t('network.wg.routes')}</dt>
                                <dd>{routes.join(', ') || missing}</dd>
                              </div>
                              <div>
                                <dt>{t('network.wg.interface')}</dt>
                                <dd>{profile.interface || missing}</dd>
                              </div>
                              <div>
                                <dt>{t('network.wg.ownership')}</dt>
                                <dd>
                                  {t(
                                    profile.niloOwned
                                      ? 'network.wg.owned'
                                      : 'network.wg.external',
                                  )}
                                </dd>
                              </div>
                              {manageProfiles && profile.niloOwned && (
                                <div>
                                  <dt>{t('network.wg.rename')}</dt>
                                  <dd>
                                    <form
                                      className="network-import"
                                      onSubmit={(event) => {
                                        event.preventDefault();
                                        if (renaming.trim() === '') return;
                                        propose({
                                          action: 'rename',
                                          target: profile.id,
                                          name: renaming.trim(),
                                        });
                                        setRenaming('');
                                      }}
                                    >
                                      <label>
                                        {t('network.wg.newName')}
                                        <input
                                          maxLength={64}
                                          value={renaming}
                                          onChange={(event) =>
                                            setRenaming(event.target.value)
                                          }
                                        />
                                      </label>
                                      <button type="submit">
                                        {t('network.wg.rename')}
                                      </button>
                                    </form>
                                  </dd>
                                </div>
                              )}
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
      )}

      {importOpen && manageProfiles && status?.reported && status.helperAvailable && (
        <form
          id="network-import"
          className="network-import"
          onSubmit={(event) => {
            event.preventDefault();
            void importProfile();
          }}
        >
          <label>
            {t('network.wg.name')}
            <input
              maxLength={64}
              value={name}
              onChange={(event) => setName(event.target.value)}
              required
            />
          </label>
          <label>
            {t('network.wg.file')}
            <input
              type="file"
              accept=".conf,text/plain"
              onChange={(event) => setFile(event.target.files?.[0] ?? null)}
              required
            />
          </label>
          <button type="submit">{t('network.wg.import')}</button>
          <p className="field-guidance">{t('network.wg.help.file')}</p>
          {error && (
            <p role="alert" className="field-error">
              {t('network.wg.invalidFile')}
            </p>
          )}
        </form>
      )}

      {canInstall && status?.canInstall && (
        <button
          type="button"
          disabled={busy}
          onClick={() => propose({ action: 'install' })}
        >
          {t('network.wg.install')}
        </button>
      )}
      {showDiagnostics && <WireGuardDiagnostics status={status} />}
    </section>
  );
}

/** WireGuard and helper readiness, shown on the focused Diagnostics page. */
export function WireGuardDiagnostics({
  status,
  collapsible = true,
}: {
  status: WireGuardStatus | null;
  collapsible?: boolean;
}) {
  const t = useTranslate();
  if (status === null) return null;
  const facts = (
    <>
      <dl>
        <div>
          <dt>{t('network.wg.manager')}</dt>
          <dd>
            {status.networkManagerAvailable
              ? status.networkManagerVersion
              : t('network.wg.missing')}
          </dd>
        </div>
        <div>
          <dt>{t('network.wg.kernel')}</dt>
          <dd>
            {t(status.kernelSupported ? 'network.wg.available' : 'network.wg.missing')}
          </dd>
        </div>
        <div>
          <dt>{t('network.wg.helper')}</dt>
          <dd>
            {t(status.helperAvailable ? 'network.wg.available' : 'network.wg.missing')}
          </dd>
        </div>
        {status.tools.map((tool) => (
          <div key={tool.name}>
            <dt>{tool.name}</dt>
            <dd>{tool.installed ? tool.version : t('network.wg.missing')}</dd>
          </div>
        ))}
      </dl>
      {status.detail && <p>{status.detail}</p>}
    </>
  );
  return collapsible ? (
    <details className="network-diagnostics">
      <summary>{t('network.wg.diagnostics')}</summary>
      {facts}
    </details>
  ) : (
    <section
      className="network-diagnostics network-readiness"
      aria-labelledby="network-readiness"
    >
      <h3 id="network-readiness">{t('network.diagnostics.wireguard')}</h3>
      {facts}
    </section>
  );
}
