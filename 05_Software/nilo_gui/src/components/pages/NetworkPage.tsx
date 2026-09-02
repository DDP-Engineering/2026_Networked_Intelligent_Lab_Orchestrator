import { useTranslate } from '../../i18n/languages';

export interface NetworkPageProps {
  /** Whether Bench Core answered this interface over HTTP. */
  benchCoreReachable: boolean;
}

/** The three interfaces the appliance would report, once it reports anything. */
const INTERFACES = [
  'network.interface.lan',
  'network.interface.wifi',
  'network.interface.vpn',
] as const;

/**
 * The Network page.
 *
 * The structure the appliance will fill is here — the interfaces, and the
 * columns their state, address, link and traffic belong in — around an explicit
 * statement that none of it is reported yet. Bench Core serves no network data
 * on any route, so the illustrative addresses and traffic figures from the
 * design references appear nowhere: an invented interface address is a
 * diagnosis an operator would act on.
 *
 * The one fact this page does state is whether Bench Core answered over HTTP,
 * and it says so in those words. That is not the appliance's LAN, Wi-Fi or VPN
 * state, and it is not evidence that any remote endpoint is reachable.
 */
export function NetworkPage({ benchCoreReachable }: NetworkPageProps) {
  const t = useTranslate();

  return (
    <div className="page">
      <header className="page-head">
        <h2>{t('network.title')}</h2>
        <p className="page-sub">{t('network.subtitle')}</p>
      </header>

      <div className="page-body">
        <section
          className="notice notice-unknown"
          aria-labelledby="network-unavailable"
        >
          <h3 id="network-unavailable">{t('network.unavailable.title')}</h3>
          <p>{t('network.unavailable.body')}</p>
        </section>

        <div className="table-scroll">
          <table className="data-table">
            <thead>
              <tr>
                <th scope="col">{t('network.table.interface')}</th>
                <th scope="col">{t('network.table.state')}</th>
                <th scope="col">{t('network.table.address')}</th>
                <th scope="col">{t('network.table.link')}</th>
                <th scope="col">{t('network.table.rx')}</th>
                <th scope="col">{t('network.table.tx')}</th>
              </tr>
            </thead>
            <tbody>
              {INTERFACES.map((key) => (
                <tr key={key}>
                  <th scope="row">{t(key)}</th>
                  {/* Five identical unavailable cells rather than five
                      different-looking blanks: nothing here is measured, and
                      the row should not imply that some of it is. */}
                  {[0, 1, 2, 3, 4].map((column) => (
                    <td key={column} className="cell-unknown">
                      {t('network.notReported')}
                    </td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        </div>

        <section className="notice" aria-labelledby="network-reachability">
          <h3 id="network-reachability">{t('network.reachability.title')}</h3>
          <p>
            {benchCoreReachable
              ? t('network.reachability.reachable')
              : t('network.reachability.unreachable')}
          </p>
        </section>

        <p className="page-foot">{t('network.followUp')}</p>
      </div>
    </div>
  );
}
