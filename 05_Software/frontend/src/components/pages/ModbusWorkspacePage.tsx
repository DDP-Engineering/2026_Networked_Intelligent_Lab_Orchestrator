import { useCallback, useEffect, useState } from 'react';
import {
  BenchCoreError,
  type BenchCoreClient,
  type Credential,
} from '../../api/client';
import type {
  ModbusDefinitionSummary,
  ModbusEndpoint,
  ModbusProfile,
  ModbusSessionStatus,
} from '../../api/modbus';
import { useTranslate } from '../../i18n/languages';
import { MessageBanner } from '../MessageBanner';
import { ModbusLiveView } from './ModbusLiveView';
import { ModbusSessionsPage } from './ModbusSessionsPage';
import './modbus.css';

interface Props {
  client: BenchCoreClient;
  credential: Credential;
  endpoint?: ModbusEndpoint;
  view: 'live' | 'sessions' | 'maps' | 'definitions';
  onSessionEnded: () => void;
}

/** Shared Modbus definitions and the explicit network live workspace. */
export function ModbusWorkspacePage({
  client,
  credential,
  endpoint,
  view,
  onSessionEnded,
}: Props) {
  const t = useTranslate();
  const [endpoints, setEndpoints] = useState<ModbusEndpoint[]>([]);
  const [profiles, setProfiles] = useState<ModbusProfile[]>([]);
  const [maps, setMaps] = useState<ModbusDefinitionSummary[]>([]);
  const [workspaces, setWorkspaces] = useState<ModbusDefinitionSummary[]>([]);
  const [status, setStatus] = useState<ModbusSessionStatus>();
  const [error, setError] = useState('');
  const refresh = useCallback(async () => {
    try {
      const [savedEndpoints, savedProfiles, savedMaps, savedWorkspaces] =
        await Promise.all([
          client.listModbusEndpoints?.(credential) ?? Promise.resolve([]),
          client.listModbusProfiles?.(credential) ?? Promise.resolve([]),
          client.listModbusMaps?.(credential) ?? Promise.resolve([]),
          client.listModbusWorkspaces?.(credential) ?? Promise.resolve([]),
        ]);
      setEndpoints(savedEndpoints);
      setProfiles(savedProfiles);
      setMaps(savedMaps);
      setWorkspaces(savedWorkspaces);
      setError('');
      if (endpoint && client.getModbusSession) {
        try {
          setStatus(await client.getModbusSession(credential, endpoint.id));
        } catch (cause) {
          if (cause instanceof BenchCoreError && cause.failure === 'unauthorized')
            onSessionEnded();
          setStatus(undefined);
        }
      }
    } catch (cause) {
      if (cause instanceof BenchCoreError && cause.failure === 'unauthorized')
        onSessionEnded();
      setError(
        cause instanceof BenchCoreError
          ? cause.code || cause.message
          : t('modbus.unavailable'),
      );
    }
  }, [client, credential, endpoint, onSessionEnded, t]);
  useEffect(() => {
    const initial = window.setTimeout(() => void refresh(), 0);
    return () => window.clearTimeout(initial);
  }, [refresh]);
  const title = endpoint?.name ?? t('modbus.definitions');
  return (
    <div className="page">
      <header className="page-head">
        <div className="page-title-actions">
          <h2>{title}</h2>
          <button type="button" onClick={() => void refresh()}>
            {t('modbus.refresh')}
          </button>
        </div>
      </header>
      <div className="page-body modbus-workspace">
        {error && <MessageBanner tone="error">{error}</MessageBanner>}
        {endpoint && (
          <section className="instrument-card">
            <h3>{t('modbus.endpoint')}</h3>
            <p>
              {endpoint.transport.toUpperCase()} ·{' '}
              {endpoint.network?.host ?? endpoint.presence} · {endpoint.readiness} · r
              {endpoint.revision}
            </p>
            <p>
              {status?.connected
                ? `${t('modbus.status')}: ${status.state}`
                : t('modbus.disconnected')}
            </p>
          </section>
        )}
        {view === 'live' && endpoint && endpoint.transport !== 'rtu' && (
          <ModbusLiveView
            client={client}
            credential={credential}
            endpoint={endpoint}
            profiles={profiles.filter((item) => item.endpoint_id === endpoint.id)}
            workspaces={workspaces.filter((item) => item.endpoint_id === endpoint.id)}
            initialStatus={status}
            onSessionEnded={onSessionEnded}
          />
        )}
        {view === 'live' && endpoint?.transport === 'rtu' && (
          <MessageBanner tone="warning">{t('modbus.networkOnly')}</MessageBanner>
        )}
        {view === 'sessions' && (
          <ModbusSessionsPage client={client} credential={credential} endpointId={endpoint?.id} onSessionEnded={onSessionEnded} />
        )}
        {(view === 'maps' || view === 'definitions') && (
          <section className="instrument-card">
            <h3>{t('modbus.maps')}</h3>
            {maps.length === 0 ? (
              <p>{t('modbus.noMaps')}</p>
            ) : (
              <ul className="modbus-list">
                {maps.map((item) => (
                  <li key={item.id}>
                    {item.name} · r{item.revision}
                  </li>
                ))}
              </ul>
            )}
          </section>
        )}
        {view === 'definitions' && (
          <>
            <section className="instrument-card">
              <h3>{t('modbus.endpoints')}</h3>
              {endpoints.length === 0 ? (
                <p>{t('modbus.noEndpoints')}</p>
              ) : (
                <ul className="modbus-list">
                  {endpoints.map((item) => (
                    <li key={item.id}>
                      {item.name} · {item.readiness} · r{item.revision}
                    </li>
                  ))}
                </ul>
              )}
            </section>
            <section className="instrument-card">
              <h3>{t('modbus.profiles')}</h3>
              {profiles.length === 0 ? (
                <p>{t('modbus.noProfiles')}</p>
              ) : (
                <ul className="modbus-list">
                  {profiles.map((item) => (
                    <li key={item.id}>
                      {item.name} · {item.endpoint_id} · r{item.revision}
                    </li>
                  ))}
                </ul>
              )}
            </section>
            <section className="instrument-card">
              <h3>{t('modbus.workspaces')}</h3>
              {workspaces.length === 0 ? (
                <p>{t('modbus.noWorkspaces')}</p>
              ) : (
                <ul className="modbus-list">
                  {workspaces.map((item) => (
                    <li key={item.id}>
                      {item.name} · r{item.revision}
                    </li>
                  ))}
                </ul>
              )}
            </section>
          </>
        )}
      </div>
    </div>
  );
}
