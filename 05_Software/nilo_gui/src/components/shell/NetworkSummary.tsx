import { useTranslate } from '../../i18n/languages';
import { Icon } from './Icon';
import { Menu } from './Menu';

export interface NetworkSummaryProps {
  /** Whether any manageable interface is connected, or unknown. */
  interfaceState: 'connected' | 'disconnected' | 'unknown';
  /** Whether a NILO-managed tunnel is active. Null means nothing reported it. */
  tunnelActive: boolean | null;
  /** Age of the last successful handshake. Null means nothing reported it. */
  handshakeAgeSeconds: number | null;
  /** Whether Bench Core answered this interface over HTTP. */
  benchCoreReachable: boolean;
  onOpenNetwork: () => void;
}

/**
 * The compact network control in the top row.
 *
 * Four facts, stated separately and never merged into one light:
 *
 *   1. whether an appliance interface is connected,
 *   2. whether a tunnel is active,
 *   3. how long ago the tunnel last completed a handshake,
 *   4. whether Bench Core answered this browser over HTTP.
 *
 * They are different claims that routinely disagree — a tunnel can be up while
 * the interface behind it is down, and Bench Core answering says nothing about
 * either. Collapsing them into one indicator would produce a diagnosis an
 * operator acts on, and it would be wrong. A fact nothing reported reads as not
 * reported, never as "off" or "never".
 */
export function NetworkSummary({
  interfaceState,
  tunnelActive,
  handshakeAgeSeconds,
  benchCoreReachable,
  onOpenNetwork,
}: NetworkSummaryProps) {
  const t = useTranslate();

  const interfaceText =
    interfaceState === 'unknown'
      ? t('network.notReported')
      : t(
          interfaceState === 'connected'
            ? 'network.state.connected'
            : 'network.state.disconnected',
        );
  const tunnelText =
    tunnelActive === null
      ? t('network.notReported')
      : t(tunnelActive ? 'network.tunnel.active' : 'network.tunnel.inactive');
  const handshakeText =
    handshakeAgeSeconds === null
      ? t('network.notReported')
      : t('network.handshake.ago').replace('{seconds}', String(handshakeAgeSeconds));

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

          <p className="menu-subtitle">{t('network.fact.interface')}</p>
          <p className="menu-body">{interfaceText}</p>

          <p className="menu-subtitle">{t('network.fact.tunnel')}</p>
          <p className="menu-body">{tunnelText}</p>

          <p className="menu-subtitle">{t('network.fact.handshake')}</p>
          <p className="menu-body">{handshakeText}</p>

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
