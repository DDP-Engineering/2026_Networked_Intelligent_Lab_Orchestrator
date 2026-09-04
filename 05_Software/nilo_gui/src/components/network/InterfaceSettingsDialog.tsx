import { useState } from 'react';

import type { NetworkInterface, NetworkOperation } from '../../api/contract';
import type {
  InterfaceSettingsDraft,
  WiFiJoinDraft,
  WiFiScan,
} from '../../api/settings';
import { useTranslate } from '../../i18n/languages';
import { Modal } from '../Modal';

export interface InterfaceSettingsDialogProps {
  target: NetworkInterface;
  busy: boolean;
  previewSettings: (
    interfaceId: string,
    settings: InterfaceSettingsDraft,
  ) => Promise<NetworkOperation | null>;
  scanWiFi?: (interfaceId: string) => Promise<WiFiScan>;
  previewJoin?: (
    interfaceId: string,
    join: WiFiJoinDraft,
  ) => Promise<NetworkOperation | null>;
  onDismiss: () => void;
}

/**
 * The adapter configuration editor: addressing for any adapter, and on a
 * wireless adapter the networks in range and a join. The password is typed
 * once, sent once, and never displayed or stored here.
 */
export function InterfaceSettingsDialog({
  target,
  busy,
  previewSettings,
  scanWiFi,
  previewJoin,
  onDismiss,
}: InterfaceSettingsDialogProps) {
  const t = useTranslate();
  const [method, setMethod] = useState<'static' | 'automatic'>(
    target.settings?.method === 'static' ? 'static' : 'automatic',
  );
  const [address, setAddress] = useState(target.ipv4[0] ?? '');
  const [gateway, setGateway] = useState(target.gateway4 ?? '');
  const [dns, setDns] = useState('');
  const [dnsAutomatic, setDnsAutomatic] = useState(true);
  const [protect, setProtect] = useState(target.settings?.managementProtected ?? false);
  const [neverDefault, setNeverDefault] = useState(
    target.settings?.neverDefault ?? false,
  );
  const [invalid, setInvalid] = useState(false);
  const [scan, setScan] = useState<WiFiScan | null>(null);
  const [scanFailed, setScanFailed] = useState(false);
  const [ssid, setSsid] = useState(target.ssid);
  const [security, setSecurity] = useState('wpa2-personal');
  const [password, setPassword] = useState('');
  const [startup, setStartup] = useState(true);
  const wireless = target.kind === 'wifi';

  const submitSettings = async () => {
    if (method === 'static' && !/^\d+\.\d+\.\d+\.\d+\/\d+$/.test(address.trim())) {
      setInvalid(true);
      return;
    }
    setInvalid(false);
    const operation = await previewSettings(target.id, {
      method,
      address: method === 'static' ? address.trim() : '',
      gateway: method === 'static' ? gateway.trim() : '',
      dns: dnsAutomatic
        ? []
        : dns
            .split(',')
            .map((entry) => entry.trim())
            .filter((entry) => entry !== ''),
      dns_automatic: dnsAutomatic,
      management_protected: protect,
      never_default: neverDefault,
    });
    if (operation !== null) onDismiss();
  };

  const runScan = async () => {
    if (scanWiFi === undefined) return;
    try {
      setScan(await scanWiFi(target.id));
      setScanFailed(false);
    } catch {
      setScanFailed(true);
    }
  };

  const submitJoin = async () => {
    if (previewJoin === undefined || ssid.trim() === '') return;
    const operation = await previewJoin(target.id, {
      ssid: ssid.trim(),
      security,
      password: security === 'open' ? '' : password,
      connect_at_startup: startup,
    });
    setPassword('');
    if (operation !== null) onDismiss();
  };

  return (
    <Modal
      title={t('network.settings.dialog').replace('{name}', target.name)}
      onDismiss={onDismiss}
    >
      {target.settings !== null && target.settings.saved && (
        <p className="card-copy">
          {t('network.settings.saved').replace('{method}', target.settings.method)}
          {!target.settings.applied && ` (${t('network.settings.notApplied')})`}
        </p>
      )}
      <form
        className="dialog-form"
        aria-label={t('network.settings.configure')}
        onSubmit={(event) => {
          event.preventDefault();
          void submitSettings();
        }}
      >
        <div className="field">
          <label htmlFor="settings-method">{t('network.settings.method')}</label>
          <select
            id="settings-method"
            value={method}
            onChange={(event) =>
              setMethod(event.target.value === 'static' ? 'static' : 'automatic')
            }
          >
            <option value="automatic">{t('network.settings.method.automatic')}</option>
            <option value="static">{t('network.settings.method.static')}</option>
          </select>
        </div>
        {method === 'static' && (
          <>
            <div className="field">
              <label htmlFor="settings-address">{t('network.settings.address')}</label>
              <input
                id="settings-address"
                value={address}
                onChange={(event) => setAddress(event.target.value)}
              />
            </div>
            <div className="field">
              <label htmlFor="settings-gateway">{t('network.settings.gateway')}</label>
              <input
                id="settings-gateway"
                value={gateway}
                onChange={(event) => setGateway(event.target.value)}
              />
            </div>
          </>
        )}
        <div className="field">
          <label htmlFor="settings-dns-automatic">
            <input
              id="settings-dns-automatic"
              type="checkbox"
              checked={dnsAutomatic}
              onChange={(event) => setDnsAutomatic(event.target.checked)}
            />{' '}
            {t('network.settings.dnsAutomatic')}
          </label>
        </div>
        {!dnsAutomatic && (
          <div className="field">
            <label htmlFor="settings-dns">{t('network.settings.dns')}</label>
            <input
              id="settings-dns"
              value={dns}
              onChange={(event) => setDns(event.target.value)}
            />
          </div>
        )}
        <div className="field">
          <label htmlFor="settings-protect">
            <input
              id="settings-protect"
              type="checkbox"
              checked={protect}
              onChange={(event) => setProtect(event.target.checked)}
            />{' '}
            {t('network.settings.protect')}
          </label>
        </div>
        <div className="field">
          <label htmlFor="settings-never-default">
            <input
              id="settings-never-default"
              type="checkbox"
              checked={neverDefault}
              onChange={(event) => setNeverDefault(event.target.checked)}
            />{' '}
            {t('network.settings.neverDefault')}
          </label>
        </div>
        {invalid && (
          <p role="alert" className="field-error">
            {t('network.settings.invalid')}
          </p>
        )}
        <div className="modal-actions">
          <button type="submit" disabled={busy}>
            {t('network.settings.preview')}
          </button>
          <button type="button" onClick={onDismiss}>
            {t('auth.cancel')}
          </button>
        </div>
      </form>

      {wireless && scanWiFi !== undefined && previewJoin !== undefined && (
        <form
          className="dialog-form"
          aria-label={t('network.wifi.title')}
          onSubmit={(event) => {
            event.preventDefault();
            void submitJoin();
          }}
        >
          <h4>{t('network.wifi.title')}</h4>
          <div className="modal-actions">
            <button type="button" onClick={() => void runScan()}>
              {t('network.wifi.scan')}
            </button>
          </div>
          {scanFailed && (
            <p role="alert" className="field-error">
              {t('network.wifi.scanFailed')}
            </p>
          )}
          {scan !== null && scan.networks.length === 0 && (
            <p>{t('network.wifi.none')}</p>
          )}
          {scan !== null && scan.networks.length > 0 && (
            <div className="table-scroll">
              <table className="data-table">
                <thead>
                  <tr>
                    <th scope="col">{t('network.wifi.ssid')}</th>
                    <th scope="col">{t('network.wifi.security')}</th>
                    <th scope="col">{t('network.wifi.strength')}</th>
                  </tr>
                </thead>
                <tbody>
                  {scan.networks.map((network) => (
                    <tr key={network.ssid}>
                      <th scope="row">
                        {network.supported ? (
                          <button
                            type="button"
                            onClick={() => {
                              setSsid(network.ssid);
                              setSecurity(network.security);
                            }}
                          >
                            {network.ssid}
                          </button>
                        ) : (
                          network.ssid
                        )}
                      </th>
                      <td>
                        {network.supported
                          ? network.security
                          : t('network.wifi.unsupported')}
                      </td>
                      <td>{network.strength}%</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
          <div className="field">
            <label htmlFor="wifi-ssid">{t('network.wifi.ssid')}</label>
            <input
              id="wifi-ssid"
              value={ssid}
              onChange={(event) => setSsid(event.target.value)}
            />
          </div>
          <div className="field">
            <label htmlFor="wifi-security">{t('network.wifi.security')}</label>
            <select
              id="wifi-security"
              value={security}
              onChange={(event) => setSecurity(event.target.value)}
            >
              <option value="open">open</option>
              <option value="wpa2-personal">wpa2-personal</option>
              <option value="wpa3-personal">wpa3-personal</option>
            </select>
          </div>
          {security !== 'open' && (
            <div className="field">
              <label htmlFor="wifi-password">{t('network.wifi.password')}</label>
              <input
                id="wifi-password"
                type="password"
                autoComplete="off"
                value={password}
                onChange={(event) => setPassword(event.target.value)}
              />
            </div>
          )}
          <div className="field">
            <label htmlFor="wifi-startup">
              <input
                id="wifi-startup"
                type="checkbox"
                checked={startup}
                onChange={(event) => setStartup(event.target.checked)}
              />{' '}
              {t('network.wifi.startup')}
            </label>
          </div>
          <div className="modal-actions">
            <button type="submit" disabled={busy || ssid.trim() === ''}>
              {t('network.wifi.join')}
            </button>
          </div>
        </form>
      )}
    </Modal>
  );
}
