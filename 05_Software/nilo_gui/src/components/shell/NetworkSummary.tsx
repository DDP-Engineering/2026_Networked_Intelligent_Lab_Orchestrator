import { useTranslate } from '../../i18n/languages';
import { Icon } from './Icon';
import { Menu } from './Menu';

export interface NetworkSummaryProps {
  /** Whether Bench Core answered this interface over HTTP. */
  benchCoreReachable: boolean;
  onOpenNetwork: () => void;
}

/**
 * The compact network control in the top row.
 *
 * Bench Core reports no interface, address, link-speed or traffic data on any
 * route, so this states that appliance network reporting is unavailable and then
 * stops. The one thing it does report is whether Bench Core answered over HTTP,
 * labelled as exactly that: reading a VPN or LAN light off an HTTP request would
 * be a diagnosis an operator acts on, and it would be wrong.
 */
export function NetworkSummary({
  benchCoreReachable,
  onOpenNetwork,
}: NetworkSummaryProps) {
  const t = useTranslate();

  return (
    <Menu
      ariaLabel={t('top.network')}
      triggerClass="network-trigger"
      label={
        <>
          <span
            className={`state-dot ${benchCoreReachable ? 'state-ok' : 'state-danger'}`}
            aria-hidden="true"
          />
          <Icon name="network" size={16} />
          <span className="control-label">{t('top.network')}</span>
        </>
      }
    >
      {(close) => (
        <div className="network-summary">
          <p className="menu-title">{t('network.subtitle')}</p>
          <p className="menu-note" role="status">
            {t('network.unavailable.title')}
          </p>
          <p className="menu-body">{t('network.unavailable.body')}</p>
          <hr className="menu-rule" />
          <p className="menu-subtitle">{t('network.reachability.title')}</p>
          <p className="menu-body">
            {benchCoreReachable
              ? t('network.reachability.reachable')
              : t('network.reachability.unreachable')}
          </p>
          <button
            type="button"
            className="menu-link"
            onClick={() => {
              close();
              onOpenNetwork();
            }}
          >
            {t('network.summary.open')}
          </button>
        </div>
      )}
    </Menu>
  );
}
