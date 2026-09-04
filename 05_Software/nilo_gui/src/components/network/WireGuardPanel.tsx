import { Fragment, useState } from 'react';
import type { WireGuardChange, WireGuardStatus } from '../../api/wireguard';
import { useTranslate } from '../../i18n/languages';

export function WireGuardPanel({
  status,
  canManage,
  canInstall,
  propose,
  busy = false,
}: {
  status: WireGuardStatus | null;
  canManage: boolean;
  canInstall: boolean;
  propose: (change: WireGuardChange) => void;
  busy?: boolean;
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
  const missing = <span title={t('network.notReported')}>—</span>;
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
          <table className="data-table">
            <thead>
              <tr>
                <th scope="col">{t('auth.field.name')}</th>
                <th scope="col">{t('network.table.state')}</th>
                <th scope="col">{t('network.table.address')}</th>
                <th scope="col">{t('network.wg.endpoint')}</th>
                <th scope="col">{t('network.wg.startup')}</th>
                <th scope="col">{t('network.table.actions')}</th>
              </tr>
            </thead>
            <tbody>
              {status.profiles.map((profile) => (
                <Fragment key={profile.id}>
                  <tr>
                    <th scope="row">
                      {profile.name}
                      <small className="network-secondary">
                        {profile.interface || '—'}
                      </small>
                      {!profile.niloOwned && (
                        <small className="network-secondary">
                          {t('network.wg.external')}
                        </small>
                      )}
                    </th>
                    <td>
                      <span className="network-state">
                        <span
                          className={`state-dot ${profile.active ? 'state-ok' : 'state-unknown'}`}
                          aria-hidden="true"
                        />
                        {t(
                          profile.active
                            ? 'network.tunnel.active'
                            : 'network.tunnel.inactive',
                        )}
                      </span>
                    </td>
                    <td className="network-address">
                      {profile.addresses?.length
                        ? profile.addresses.map((address) => (
                            <div key={address}>{address}</div>
                          ))
                        : missing}
                    </td>
                    <td className="network-address">
                      {profile.peers?.some((peer) => peer.endpoint)
                        ? [
                            ...new Set(
                              profile.peers
                                .map((peer) => peer.endpoint)
                                .filter(Boolean),
                            ),
                          ].map((endpoint) => <div key={endpoint}>{endpoint}</div>)
                        : missing}
                    </td>
                    <td>
                      {manageProfiles && profile.niloOwned && status.helperAvailable ? (
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
                      ) : (
                        t(
                          profile.autoconnect
                            ? 'network.wg.enabled'
                            : 'network.wg.disabled',
                        )
                      )}
                    </td>
                    <td>
                      <div className="network-actions">
                        {manageProfiles &&
                          profile.niloOwned &&
                          status.helperAvailable && (
                            <>
                              <button
                                type="button"
                                aria-label={`${t(profile.active ? 'network.wg.disable' : 'network.wg.enable')} ${profile.name}`}
                                onClick={() =>
                                  propose({
                                    action: profile.active ? 'deactivate' : 'activate',
                                    target: profile.id,
                                  })
                                }
                              >
                                {t(
                                  profile.active
                                    ? 'network.wg.disable'
                                    : 'network.wg.enable',
                                )}
                              </button>
                              {!profile.active && (
                                <button
                                  type="button"
                                  aria-label={`${t('network.wg.remove')} ${profile.name}`}
                                  onClick={() =>
                                    propose({ action: 'remove', target: profile.id })
                                  }
                                >
                                  {t('network.wg.remove')}
                                </button>
                              )}
                            </>
                          )}
                      </div>
                      <button
                        type="button"
                        className="network-detail-toggle"
                        aria-label={`${t('network.details')} ${profile.name}`}
                        aria-expanded={expandedProfile === profile.id}
                        aria-controls={`network-profile-${profile.id}`}
                        onClick={() =>
                          setExpandedProfile(
                            expandedProfile === profile.id ? null : profile.id,
                          )
                        }
                      >
                        {t('network.details')}
                      </button>
                    </td>
                  </tr>
                  {expandedProfile === profile.id && (
                    <tr id={`network-profile-${profile.id}`}>
                      <td colSpan={6}>
                        <dl className="network-profile-facts">
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
                          <div>
                            <dt>{t('network.wg.routes')}</dt>
                            <dd>
                              {[
                                ...new Set(
                                  profile.peers?.flatMap((peer) => peer.allowedIPs) ??
                                    [],
                                ),
                              ].join(', ') || missing}
                            </dd>
                          </div>
                        </dl>
                      </td>
                    </tr>
                  )}
                </Fragment>
              ))}
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
          {error && <p role="alert">{t('network.wg.invalidFile')}</p>}
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
      {status !== null && (
        <details className="network-diagnostics">
          <summary>{t('network.wg.diagnostics')}</summary>
          <dl>
            <dt>{t('network.wg.manager')}</dt>
            <dd>
              {status.networkManagerAvailable
                ? status.networkManagerVersion
                : t('network.wg.missing')}
            </dd>
            <dt>{t('network.wg.kernel')}</dt>
            <dd>
              {t(
                status.kernelSupported ? 'network.wg.available' : 'network.wg.missing',
              )}
            </dd>
            <dt>{t('network.wg.helper')}</dt>
            <dd>
              {t(
                status.helperAvailable ? 'network.wg.available' : 'network.wg.missing',
              )}
            </dd>
            {status.tools.map((tool) => (
              <div key={tool.name}>
                <dt>{tool.name}</dt>
                <dd>{tool.installed ? tool.version : t('network.wg.missing')}</dd>
              </div>
            ))}
          </dl>
          {status.detail && <p>{status.detail}</p>}
        </details>
      )}
    </section>
  );
}
