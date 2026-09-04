import { useTranslate } from '../../i18n/languages';

const phases = {
  queued: 'network.install.queued',
  refreshing: 'network.install.refreshing',
  planning: 'network.install.planning',
  downloading: 'network.install.downloading',
  installing: 'network.install.installing',
  verifying: 'network.install.verifying',
  helper_unavailable: 'network.install.helperUnavailable',
  storage_unavailable: 'network.install.storageUnavailable',
} as const;

export function InstallationProgress({ progress }: { progress?: string }) {
  const t = useTranslate();
  const key =
    progress !== undefined && Object.hasOwn(phases, progress)
      ? phases[progress as keyof typeof phases]
      : phases.queued;
  return (
    <p className="notice" role="status">
      {t('network.install.title')} {t(key)}
    </p>
  );
}
