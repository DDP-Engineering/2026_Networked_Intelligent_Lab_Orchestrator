import { useState } from 'react';

import type { NetworkInterface, NetworkOperation } from '../../api/contract';
import type {
  InterfaceSettingsDraft,
  WiFiJoinDraft,
  WiFiScan,
} from '../../api/settings';
import { useTranslate } from '../../i18n/languages';
import { Modal } from '../Modal';
import { Field } from '../help/Field';
import { HelpPopover } from '../help/HelpPopover';
import { WorkflowHelp } from '../help/WorkflowHelp';

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

const CIDR = /^\d+\.\d+\.\d+\.\d+\/\d+$/;

/**
 * The adapter configuration editor: addressing for any adapter, and on a
 * wireless adapter the networks in range and a join. The password is typed
 * once, sent once, and never displayed or stored here.
 *
 * Three groups — Addressing, DNS, Protection — each setting with its help
 * beside the label, the consequences that matter stated under the field, and
 * a how-to-configure help for the whole workflow.
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
  const [addressError, setAddressError] = useState('');
  const [scan, setScan] = useState<WiFiScan | null>(null);
  const [scanFailed, setScanFailed] = useState(false);
  const [ssid, setSsid] = useState(target.ssid);
  const [security, setSecurity] = useState('wpa2-personal');
  const [password, setPassword] = useState('');
  const [startup, setStartup] = useState(true);
  const wireless = target.kind === 'wifi';

  const submitSettings = async () => {
    if (method === 'static' && !CIDR.test(address.trim())) {
      setAddressError(t('network.settings.invalid'));
      return;
    }
    setAddressError('');
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

  const checkbox = (
    id: string,
    label: string,
    checked: boolean,
    onChange: (next: boolean) => void,
    help?: string,
    guidance?: string,
  ) => (
    <div className="field">
      <div className="field-label-row">
        <label htmlFor={id}>
          <input
            id={id}
            type="checkbox"
            checked={checked}
            onChange={(event) => onChange(event.target.checked)}
          />
          {label}
        </label>
        {help === undefined ? null : (
          <HelpPopover name={label}>
            <p>{help}</p>
          </HelpPopover>
        )}
      </div>
      {guidance === undefined ? null : <p className="field-guidance">{guidance}</p>}
    </div>
  );

  return (
    <Modal
      title={t('network.settings.dialog').replace('{name}', target.name)}
      onDismiss={onDismiss}
    >
      <div className="network-section-head">
        {target.settings !== null && target.settings.saved ? (
          <p className="card-copy">
            {t('network.settings.saved').replace('{method}', target.settings.method)}
            {!target.settings.applied && ` (${t('network.settings.notApplied')})`}
          </p>
        ) : (
          <span />
        )}
        <WorkflowHelp prefix="help.if" title={t('help.if.title')} />
      </div>
      <form
        className="dialog-form"
        aria-label={t('network.settings.configure')}
        noValidate
        onSubmit={(event) => {
          event.preventDefault();
          void submitSettings();
        }}
      >
        <fieldset
          className="form-group"
          aria-label={t('network.settings.group.addressing')}
        >
          <legend>{t('network.settings.group.addressing')}</legend>
          <Field
            id="settings-method"
            label={t('network.settings.method')}
            help={<p>{t('network.settings.help.method')}</p>}
          >
            <select
              id="settings-method"
              value={method}
              onChange={(event) =>
                setMethod(event.target.value === 'static' ? 'static' : 'automatic')
              }
            >
              <option value="automatic">
                {t('network.settings.method.automatic')}
              </option>
              <option value="static">{t('network.settings.method.static')}</option>
            </select>
          </Field>
          {method === 'static' && (
            <>
              <Field
                id="settings-address"
                label={t('network.settings.address')}
                help={<p>{t('network.settings.help.address')}</p>}
                error={addressError}
              >
                <input
                  id="settings-address"
                  value={address}
                  placeholder="192.168.0.1/24"
                  onChange={(event) => {
                    setAddress(event.target.value);
                    setAddressError('');
                  }}
                />
              </Field>
              <Field
                id="settings-gateway"
                label={t('network.settings.gateway')}
                help={<p>{t('network.settings.help.gateway')}</p>}
                guidance={t('network.settings.guidance.gateway')}
              >
                <input
                  id="settings-gateway"
                  value={gateway}
                  onChange={(event) => setGateway(event.target.value)}
                />
              </Field>
            </>
          )}
        </fieldset>

        <fieldset className="form-group" aria-label={t('network.settings.group.dns')}>
          <legend>{t('network.settings.group.dns')}</legend>
          {checkbox(
            'settings-dns-automatic',
            t('network.settings.dnsAutomatic'),
            dnsAutomatic,
            setDnsAutomatic,
            t('network.settings.help.dns'),
          )}
          {!dnsAutomatic && (
            <Field id="settings-dns" label={t('network.settings.dns')}>
              <input
                id="settings-dns"
                value={dns}
                onChange={(event) => setDns(event.target.value)}
              />
            </Field>
          )}
        </fieldset>

        <fieldset
          className="form-group"
          aria-label={t('network.settings.group.protection')}
        >
          <legend>{t('network.settings.group.protection')}</legend>
          {checkbox(
            'settings-protect',
            t('network.settings.protect'),
            protect,
            setProtect,
            t('network.settings.help.protect'),
            t('network.settings.guidance.protect'),
          )}
          {checkbox(
            'settings-never-default',
            t('network.settings.neverDefault'),
            neverDefault,
            setNeverDefault,
            t('network.settings.help.neverDefault'),
          )}
        </fieldset>

        <div className="modal-actions">
          <button type="submit" className="button button-primary" disabled={busy}>
            {t('network.settings.preview')}
          </button>
          <button type="button" className="button" onClick={onDismiss}>
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
          <div className="form-actions">
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
          <Field id="wifi-ssid" label={t('network.wifi.ssid')}>
            <input
              id="wifi-ssid"
              value={ssid}
              onChange={(event) => setSsid(event.target.value)}
            />
          </Field>
          <Field id="wifi-security" label={t('network.wifi.security')}>
            <select
              id="wifi-security"
              value={security}
              onChange={(event) => setSecurity(event.target.value)}
            >
              <option value="open">open</option>
              <option value="wpa2-personal">wpa2-personal</option>
              <option value="wpa3-personal">wpa3-personal</option>
            </select>
          </Field>
          {security !== 'open' && (
            <Field id="wifi-password" label={t('network.wifi.password')}>
              <input
                id="wifi-password"
                type="password"
                autoComplete="off"
                value={password}
                onChange={(event) => setPassword(event.target.value)}
              />
            </Field>
          )}
          {checkbox('wifi-startup', t('network.wifi.startup'), startup, setStartup)}
          <div className="modal-actions">
            <button
              type="submit"
              className="button button-primary"
              disabled={busy || ssid.trim() === ''}
            >
              {t('network.wifi.join')}
            </button>
          </div>
        </form>
      )}
    </Modal>
  );
}
