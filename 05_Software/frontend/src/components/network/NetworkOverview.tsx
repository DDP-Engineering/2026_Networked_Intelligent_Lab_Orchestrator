import type { NetworkInterface, NetworkOperation } from '../../api/contract';
import type { DeviceNetwork } from '../../api/devicenetworks';
import type { WireGuardStatus } from '../../api/wireguard';
import type { MessageKey } from '../../i18n/en';
import { useTranslate } from '../../i18n/languages';
import { uplinkOf } from './addressing';

export interface NetworkOverviewProps {
  interfaces: readonly NetworkInterface[];
  deviceNetworks: readonly DeviceNetwork[] | null;
  wireGuard: WireGuardStatus | null;
  /** The operation in flight or just settled, if any. */
  running: NetworkOperation | null;
}

const DN_STATE_KEYS: Record<DeviceNetwork['state'], MessageKey> = {
  disabled: 'network.dn.state.disabled',
  active: 'network.dn.state.active',
  waiting: 'network.dn.state.waiting',
  conflict: 'network.dn.state.conflict',
  recovery_failed: 'network.dn.state.recovery_failed',
  unsupported: 'network.dn.state.unsupported',
};

/**
 * The first thing the Network page says, and only what decides the next step:
 * where NILO is reachable, which side faces the devices, whether a tunnel is
 * up, and whether something is waiting on the operator. Nothing here repeats
 * the shell's runtime indicator or the rows below.
 */
export function NetworkOverview({
  interfaces,
  deviceNetworks,
  wireGuard,
  running,
}: NetworkOverviewProps) {
  const t = useTranslate();
  const uplink = uplinkOf(interfaces);
  const none = t('network.overview.none');

  const deviceSide = (deviceNetworks ?? [])
    .map((network) => {
      const adapter = interfaces.find(
        (entry) => entry.adapterId === network.deviceInterface,
      );
      return `${adapter?.name ?? network.deviceInterface} · ${network.applianceAddress}`;
    })
    .filter((entry, index, all) => all.indexOf(entry) === index);

  const activeProfile =
    wireGuard?.reported && !wireGuard.readinessOnly
      ? wireGuard.profiles.find((profile) => profile.niloOwned && profile.active)
      : undefined;

  const attention: string[] = [];
  if (running !== null) {
    const values = { action: running.action, target: running.target };
    if (running.state === 'awaiting_confirmation') {
      attention.push(t('network.overview.awaiting', values));
    } else if (running.state === 'applying') {
      attention.push(t('network.overview.applying', values));
    } else if (running.state === 'rolling_back') {
      attention.push(t('network.overview.rollingBack', values));
    } else if (running.state === 'rollback_failed' || running.state === 'failed') {
      attention.push(t('network.overview.failed', values));
    }
  }
  for (const network of deviceNetworks ?? []) {
    if (network.state === 'conflict' || network.state === 'recovery_failed') {
      attention.push(
        t('network.overview.networkAttention', {
          name: network.name,
          state: t(DN_STATE_KEYS[network.state]),
        }),
      );
    }
  }

  return (
    <section aria-label={t('network.overview.title')} className="network-overview">
      <dl className="fact-grid">
        <div className="fact">
          <dt>{t('network.overview.uplink')}</dt>
          <dd>
            {uplink === undefined
              ? none
              : `${uplink.name} · ${uplink.ipv4[0] ?? uplink.ipv6[0] ?? ''}`.trim()}
          </dd>
        </div>
        <div className="fact">
          <dt>{t('network.overview.deviceSide')}</dt>
          <dd>{deviceSide.length === 0 ? none : deviceSide.join(', ')}</dd>
        </div>
        <div className="fact">
          <dt>{t('network.overview.profile')}</dt>
          <dd>{activeProfile === undefined ? none : activeProfile.name}</dd>
        </div>
        <div className="fact">
          <dt>{t('network.overview.attention')}</dt>
          <dd>
            {attention.length === 0
              ? t('network.overview.nothingPending')
              : attention.join(' ')}
          </dd>
        </div>
      </dl>
    </section>
  );
}
