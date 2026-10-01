import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
  BenchCoreError,
  type BenchCoreClient,
  type Credential,
} from '../../api/client';
import type {
  ModbusDefinitionSummary,
  ModbusEndpoint,
  ModbusMap,
  ModbusPollGroup,
  ModbusProfile,
  ModbusSessionStatus,
  ModbusWorkspace,
  ModbusEvent,
  ModbusPoint,
  ModbusDecodedPoint,
  ModbusRequest,
  ModbusWritePreview,
  ModbusCommand,
} from '../../api/modbus';
import { useTranslate } from '../../i18n/languages';
import { MessageBanner } from '../MessageBanner';
import { ModbusToolsPanel } from './ModbusToolsPanel';

interface Props {
  client: BenchCoreClient;
  credential: Credential;
  endpoint: ModbusEndpoint;
  profiles: ModbusProfile[];
  workspaces: ModbusDefinitionSummary[];
  initialStatus?: ModbusSessionStatus;
  onSessionEnded: () => void;
}

interface GridPoint {
  key: string;
  groupId: string;
  groupName: string;
  unitId: number;
  mapId: string;
  mapRevision: number;
  point: ModbusPoint;
}

interface LiveSample {
  raw: number[];
  decoded?: ModbusDecodedPoint;
  quality: string;
  lastValidAt?: string;
  changedAt?: string;
}

const areaForFunction = (fn: number) =>
  ({ 1: 'coil', 2: 'discrete', 3: 'holding', 4: 'input' })[fn] ?? '';
const displayAddress = (area: string, address: number, base: string) => {
  if (base === 'zero') return String(address);
  if (base === 'one') return String(address + 1);
  const prefix =
    ({ coil: '0', discrete: '1', input: '3', holding: '4' } as Record<string, string>)[
      area
    ] ?? '';
  return `${prefix}${String(address + 1).padStart(4, '0')}`;
};
const MAX_TRAFFIC = 200;
const MAX_TREND = 120;

function mappedPoints(
  workspace: ModbusWorkspace | undefined,
  maps: Record<string, ModbusMap>,
): GridPoint[] {
  if (!workspace) return [];
  return workspace.poll_groups.flatMap((group) => {
    const map = maps[group.map_ref.id];
    if (!map || map.revision !== group.map_ref.revision) return [];
    return map.points
      .filter(
        (point) =>
          point.area === areaForFunction(group.function) &&
          point.address >= group.address &&
          point.address + point.width <= group.address + group.quantity,
      )
      .map((point) => ({
        key: `${group.id}:${point.id}`,
        groupId: group.id,
        groupName: group.name,
        unitId: group.unit_id,
        mapId: map.id,
        mapRevision: map.revision,
        point,
      }));
  });
}

/** Live controls are explicit; selecting saved metadata never opens a transport. */
export function ModbusLiveView({
  client,
  credential,
  endpoint,
  profiles,
  workspaces,
  initialStatus,
  onSessionEnded,
}: Props) {
  const t = useTranslate();
  const [workspaceId, setWorkspaceId] = useState('');
  const [workspace, setWorkspace] = useState<ModbusWorkspace>();
  const [workspaceDirty, setWorkspaceDirty] = useState(false);
  const [maps, setMaps] = useState<Record<string, ModbusMap>>({});
  const [statusOverride, setStatus] = useState<ModbusSessionStatus>();
  const status = statusOverride ?? initialStatus;
  const [jobId, setJobId] = useState('');
  const [jobState, setJobState] = useState('');
  const [error, setError] = useState('');
  const [traffic, setTraffic] = useState<ModbusEvent[]>([]);
  const [samples, setSamples] = useState<Record<string, LiveSample>>({});
  const samplesRef = useRef<Record<string, LiveSample>>({});
  const [trend, setTrend] = useState<Record<string, number[]>>({});
  const cursorRef = useRef(0);
  const localSequenceRef = useRef(0);
  const [gap, setGap] = useState(false);
  const [displayPaused, setDisplayPaused] = useState(false);
  const [displayNow, setDisplayNow] = useState(() => Date.now());
  const [find, setFind] = useState('');
  const [qualityFilter, setQualityFilter] = useState('');
  const [gridSort, setGridSort] = useState<'address' | 'name'>('address');
  const [gridScroll, setGridScroll] = useState(0);
  const [writePoint, setWritePoint] = useState<GridPoint>();
  const [writeValue, setWriteValue] = useState('');
  const [writeKey, setWriteKey] = useState('');
  const [writePreview, setWritePreview] = useState<ModbusWritePreview>();
  const [writeCommand, setWriteCommand] = useState<ModbusCommand>();
  const [writeBusy, setWriteBusy] = useState(false);
  const [writeAttempted, setWriteAttempted] = useState(false);
  const [writeSubmitIssue, setWriteSubmitIssue] = useState('');
  const [readBack, setReadBack] = useState(false);
  const [readBackResult, setReadBackResult] = useState('');
  const [loggingMode, setLoggingMode] = useState<'values' | 'traffic' | 'both'>('both');
  const [loggingSampling, setLoggingSampling] = useState<
    'every' | 'interval' | 'change'
  >('every');
  const [loggingInterval, setLoggingInterval] = useState(1000);
  const [loggingName, setLoggingName] = useState('');
  const [loggingBusy, setLoggingBusy] = useState(false);
  const [toolsOpen, setToolsOpen] = useState(false);
  const dialogRef = useRef<HTMLDivElement>(null);
  const writeTriggerRef = useRef<HTMLButtonElement>(null);
  const matchingWorkspace =
    !status?.connected ||
    Boolean(
      workspace &&
      status.workspace_id === workspace.id &&
      status.workspace_revision === workspace.revision,
    );
  const points = useMemo(
    () => (matchingWorkspace ? mappedPoints(workspace, maps) : []),
    [workspace, maps, matchingWorkspace],
  );
  useEffect(() => {
    if (!writePoint) return;
    const previous =
      document.activeElement instanceof HTMLElement ? document.activeElement : null;
    dialogRef.current
      ?.querySelector<HTMLElement>('input[type="text"], select')
      ?.focus();
    return () => (writeTriggerRef.current ?? previous)?.focus();
  }, [writePoint]);

  const report = useCallback(
    (cause: unknown) => {
      if (cause instanceof BenchCoreError && cause.failure === 'unauthorized')
        onSessionEnded();
      setError(
        cause instanceof BenchCoreError
          ? cause.code || cause.message
          : t('modbus.unavailable'),
      );
    },
    [onSessionEnded, t],
  );

  const selectWorkspace = useCallback(
    async (id: string) => {
      setWorkspaceId(id);
      setWorkspace(undefined);
      setWorkspaceDirty(false);
      setMaps({});
      setTraffic([]);
      setSamples({});
      samplesRef.current = {};
      setTrend({});
      cursorRef.current = 0;
      localSequenceRef.current = 0;
      setGap(false);
      setWritePoint(undefined);
      setError('');
      if (!id || !client.getModbusWorkspace) return;
      try {
        const saved = await client.getModbusWorkspace(credential, id);
        if (saved.endpoint_id !== endpoint.id)
          throw new Error('Workspace endpoint mismatch');
        const loaded = await Promise.all(
          saved.map_refs.map(async (ref) =>
            client.getModbusMap?.(credential, ref.id, ref.revision),
          ),
        );
        setMaps(
          Object.fromEntries(
            loaded
              .filter((item): item is ModbusMap => Boolean(item))
              .map((item) => [item.id, item]),
          ),
        );
        setWorkspace(saved);
        setDisplayPaused(saved.layout.display_paused);
      } catch (cause) {
        report(cause);
      }
    },
    [client, credential, endpoint.id, report],
  );

  const groups: ModbusPollGroup[] = (workspace?.poll_groups ?? [])
    .filter((group) => group.enabled)
    .map((group) => ({
      id: group.id,
      interval_ms: group.interval_ms,
      timeout_ms: group.timeout_ms ?? 0,
      stop_on_error: group.stop_on_error ?? false,
      requests: [
        {
          function: group.function,
          unit: group.unit_id,
          address: group.address,
          count: group.quantity,
          values: [],
        },
      ],
    }));

  const refreshStatus = useCallback(async () => {
    if (!client.getModbusSession) return;
    try {
      setStatus(await client.getModbusSession(credential, endpoint.id));
    } catch (cause) {
      report(cause);
    }
  }, [client, credential, endpoint.id, report]);

  const applyEvents = useCallback(
    async (events: ModbusEvent[], hasGap: boolean) => {
      const next = { ...samplesRef.current };
      if (hasGap) {
        setGap(true);
        for (const key of Object.keys(next))
          next[key] = { ...next[key], quality: 'stale' };
      }
      const additions: Record<string, number[]> = {};
      for (const event of events) {
        const groupPoints = points.filter(
          ({ groupId, unitId, point }) =>
            groupId === event.group_id &&
            unitId === event.request.unit &&
            point.area === areaForFunction(event.request.function) &&
            point.address >= event.request.address &&
            point.address + point.width <= event.request.address + event.request.count,
        );
        for (const item of groupPoints) {
          const old = next[item.key];
          if (event.result.outcome !== 'acknowledged') {
            next[item.key] = {
              raw: old?.raw ?? [],
              decoded: old?.decoded,
              lastValidAt: old?.lastValidAt,
              changedAt: old?.changedAt,
              quality: event.result.outcome,
            };
            continue;
          }
          const offset = item.point.address - event.request.address;
          const raw = event.result.values
            .slice(offset, offset + item.point.width)
            .map((value) => (typeof value === 'boolean' ? Number(value) : value));
          try {
            const decoded = await client.decodeModbusPoint?.(
              credential,
              item.mapId,
              item.mapRevision,
              item.point.id,
              raw,
            );
            next[item.key] = {
              raw,
              decoded,
              quality: decoded?.quality ?? 'decode_error',
              lastValidAt: event.timestamp,
              changedAt:
                old?.decoded &&
                JSON.stringify(old.decoded.value) !== JSON.stringify(decoded?.value)
                  ? event.timestamp
                  : old?.changedAt,
            };
            if (typeof decoded?.value === 'number' && Number.isFinite(decoded.value)) {
              additions[item.key] = [...(additions[item.key] ?? []), decoded.value];
            }
          } catch {
            next[item.key] = {
              raw,
              quality: 'decode_error',
              lastValidAt: old?.lastValidAt,
              changedAt: old?.changedAt,
              decoded: old?.decoded,
            };
          }
        }
      }
      samplesRef.current = next;
      setSamples(next);
      setTrend((before) => {
        const updated = { ...before };
        for (const [key, values] of Object.entries(additions))
          updated[key] = [...(updated[key] ?? []), ...values].slice(-MAX_TREND);
        return updated;
      });
    },
    [client, credential, points],
  );

  const refreshingRef = useRef(false);
  const connected = status?.connected;
  const sessionId = status?.session_id;
  const refreshLive = useCallback(async () => {
    if (
      !connected ||
      !sessionId ||
      !client.modbusEvents ||
      refreshingRef.current ||
      displayPaused
    )
      return;
    refreshingRef.current = true;
    try {
      const page = await client.modbusEvents(
        credential,
        endpoint.id,
        sessionId,
        cursorRef.current,
      );
      cursorRef.current = page.next_cursor;
      if (page.events.length || page.gap) {
        setTraffic((before) => [...before, ...page.events].slice(-MAX_TRAFFIC));
        await applyEvents(page.events, page.gap);
      }
      await refreshStatus();
    } catch (cause) {
      report(cause);
    } finally {
      refreshingRef.current = false;
    }
  }, [
    connected,
    sessionId,
    client,
    credential,
    endpoint.id,
    applyEvents,
    refreshStatus,
    report,
    displayPaused,
  ]);

  useEffect(() => {
    if (!status?.connected || !workspace || displayPaused) return;
    const timer = window.setInterval(() => void refreshLive(), 1000);
    return () => window.clearInterval(timer);
  }, [status?.connected, workspace, displayPaused, refreshLive]);

  useEffect(() => {
    if (!status?.connected || !displayPaused) return;
    const timer = window.setInterval(() => void refreshStatus(), 3000);
    return () => window.clearInterval(timer);
  }, [status?.connected, displayPaused, refreshStatus]);

  useEffect(() => {
    if (!workspace || displayPaused) return;
    const timer = window.setInterval(() => setDisplayNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, [workspace, displayPaused]);

  useEffect(() => {
    if (!status?.connected) {
      const next = { ...samplesRef.current };
      for (const key of Object.keys(next))
        next[key] = { ...next[key], quality: 'disconnected' };
      samplesRef.current = next;
      setSamples(next);
    }
  }, [status?.connected]);

  useEffect(() => {
    if (!jobId) return;
    let active = true;
    const tick = async () => {
      try {
        const job = await client.getInstrumentJob(credential, jobId);
        if (!active) return;
        setJobState(job.state);
        if (job.state === 'completed') {
          setJobId('');
          await refreshStatus();
        }
        if (['failed', 'cancelled', 'incomplete'].includes(job.state)) {
          setJobId('');
          setError(job.error || job.state);
        }
      } catch (cause) {
        if (active) report(cause);
      }
    };
    void tick();
    const timer = window.setInterval(() => void tick(), 1000);
    return () => {
      active = false;
      window.clearInterval(timer);
    };
  }, [client, credential, jobId, refreshStatus, report]);

  const connect = async () => {
    if (!workspace || !client.connectModbus) return;
    setError('');
    try {
      const job = await client.connectModbus(credential, endpoint.id, {
        endpoint_revision: endpoint.revision,
        profile_id: workspace.profile_ref.id,
        profile_revision: workspace.profile_ref.revision,
        workspace_id: workspace.id,
        workspace_revision: workspace.revision,
        groups,
        idempotency_key: crypto.randomUUID(),
        timeout_seconds: 60,
      });
      setJobId(job.transaction_id);
      setJobState(job.state);
    } catch (cause) {
      report(cause);
    }
  };

  const changeGroup = (
    id: string,
    change: Partial<ModbusWorkspace['poll_groups'][number]>,
  ) => {
    setWorkspace(
      (before) =>
        before && {
          ...before,
          poll_groups: before.poll_groups.map((group) =>
            group.id === id ? { ...group, ...change } : group,
          ),
        },
    );
    setWorkspaceDirty(true);
  };

  const addGroup = (source?: ModbusWorkspace['poll_groups'][number]) => {
    if (!workspace || !workspace.map_refs.length || workspace.poll_groups.length >= 32)
      return;
    const id = `group-${crypto.randomUUID().slice(0, 8)}`;
    const group = source
      ? { ...source, id, name: `${source.name} ${t('modbus.copySuffix')}` }
      : {
          id,
          name: t('modbus.newGroup'),
          enabled: true,
          unit_id: endpoint.network?.unit_min ?? 1,
          function: 3,
          address: 0,
          quantity: 1,
          interval_ms: 1000,
          timeout_ms: 5000,
          stop_on_error: false,
          map_ref: workspace.map_refs[0],
        };
    setWorkspace({ ...workspace, poll_groups: [...workspace.poll_groups, group] });
    setWorkspaceDirty(true);
  };

  const saveWorkspace = async () => {
    if (!workspace || !client.saveModbusWorkspace || status?.connected) return;
    setError('');
    try {
      const saved = await client.saveModbusWorkspace(
        credential,
        workspace.revision,
        workspace,
      );
      setWorkspace(saved);
      setWorkspaceDirty(false);
    } catch (cause) {
      report(cause);
    }
  };

  const previewWrite = async () => {
    if (
      !writePoint ||
      !status?.session_id ||
      !client.encodeModbusPoint ||
      !client.previewModbusWrite
    )
      return;
    setWriteBusy(true);
    setWritePreview(undefined);
    setWriteCommand(undefined);
    setReadBackResult('');
    setError('');
    try {
      const value: unknown =
        writePoint.point.type === 'bool'
          ? writeValue === 'true'
            ? true
            : writeValue === 'false'
              ? false
              : NaN
          : ['ascii', 'int64', 'uint64'].includes(writePoint.point.type)
            ? writeValue
            : Number(writeValue);
      if (
        (typeof value === 'number' && !Number.isFinite(value)) ||
        writeValue.trim() === ''
      ) {
        setError(t('modbus.invalidValue'));
        return;
      }
      const encoded = await client.encodeModbusPoint(
        credential,
        writePoint.mapId,
        writePoint.mapRevision,
        writePoint.point.id,
        value,
      );
      const fn = encoded.area === 'coil' ? 5 : encoded.words.length === 1 ? 6 : 16;
      const request: ModbusRequest = {
        function: fn,
        unit: writePoint.unitId,
        address: encoded.address,
        count: encoded.words.length,
        values: encoded.words,
      };
      const key = crypto.randomUUID();
      const preview = await client.previewModbusWrite(
        credential,
        endpoint.id,
        status.session_id,
        key,
        request,
      );
      setWriteKey(key);
      setWritePreview(preview);
    } catch (cause) {
      report(cause);
    } finally {
      setWriteBusy(false);
    }
  };

  const submitWrite = async () => {
    if (
      !writePreview ||
      !writePoint ||
      !status?.session_id ||
      !client.writeModbus ||
      writeAttempted
    )
      return;
    setWriteAttempted(true);
    setWriteBusy(true);
    try {
      const command = await client.writeModbus(
        credential,
        endpoint.id,
        status.session_id,
        writeKey,
        writePreview.request,
      );
      setWriteCommand(command);
      if (!displayPaused)
        setTraffic((before) =>
          [
            ...before,
            {
              sequence: --localSequenceRef.current,
              timestamp: new Date().toISOString(),
              group_id: writePoint.groupId,
              request: writePreview.request,
              result: command.result,
            },
          ].slice(-MAX_TRAFFIC),
        );
      if (readBack && command.outcome === 'acknowledged' && client.readModbus) {
        const approved = groups.find((group) => group.id === writePoint.groupId)
          ?.requests[0];
        if (approved) {
          try {
            const result = await client.readModbus(
              credential,
              endpoint.id,
              status.session_id,
              approved,
            );
            setReadBackResult(`${result.outcome}: ${result.values.join(', ')}`);
          } catch (cause) {
            setReadBackResult(
              cause instanceof BenchCoreError
                ? cause.code || cause.message
                : t('modbus.unavailable'),
            );
          }
        }
      }
    } catch (cause) {
      report(cause);
      setWriteSubmitIssue(
        cause instanceof BenchCoreError &&
          ['forbidden', 'rejected', 'unauthorized'].includes(cause.failure)
          ? t('modbus.writeRejected')
          : t('modbus.writeUnconfirmed'),
      );
    } finally {
      setWriteBusy(false);
    }
  };

  const visibleQuality = (item: GridPoint) => {
    const sample = samples[item.key];
    if (!sample || sample.quality !== 'good' || !sample.lastValidAt)
      return sample?.quality ?? 'waiting';
    const group = workspace?.poll_groups.find(
      (candidate) => candidate.id === item.groupId,
    );
    const staleAfter = (group?.interval_ms ?? 1000) * 2 + (group?.timeout_ms ?? 5000);
    return displayNow - Date.parse(sample.lastValidAt) > staleAfter ? 'stale' : 'good';
  };
  const filteredPoints = points
    .filter(
      (item) =>
        `${item.point.name} ${item.point.id} ${item.point.address} ${item.groupName}`
          .toLowerCase()
          .includes(find.toLowerCase()) &&
        (!qualityFilter || visibleQuality(item) === qualityFilter),
    )
    .sort((a, b) =>
      gridSort === 'name'
        ? a.point.name.localeCompare(b.point.name)
        : a.unitId - b.unitId ||
          a.point.address - b.point.address ||
          a.point.name.localeCompare(b.point.name),
    );
  const gridStart = Math.max(0, Math.floor(gridScroll / 48) - 3);
  const visiblePoints = filteredPoints.slice(gridStart, gridStart + 35);
  const [trafficGroup, setTrafficGroup] = useState('');
  const [trendPoint, setTrendPoint] = useState('');
  const trendKey =
    trendPoint ||
    points.find((item) => workspace?.layout.series.includes(item.point.id))?.key ||
    points[0]?.key;
  const trendItem = points.find((item) => item.key === trendKey);
  const chartValues = trend[trendKey] ?? [];
  const low = chartValues.length ? Math.min(...chartValues) : 0;
  const high = chartValues.length ? Math.max(...chartValues) : 1;
  const polyline = chartValues
    .map(
      (value, index) =>
        `${(index / Math.max(1, chartValues.length - 1)) * 300},${90 - ((value - low) / (high - low || 1)) * 80}`,
    )
    .join(' ');

  return (
    <section className="instrument-card modbus-live">
      <h3>{t('modbus.realtime')}</h3>
      {error && <MessageBanner tone="error">{error}</MessageBanner>}
      <div className="modbus-form">
        <label>
          {t('modbus.workspace')}
          <select
            value={workspaceId}
            onChange={(event) => void selectWorkspace(event.target.value)}
          >
            <option value="">{t('modbus.chooseWorkspace')}</option>
            {workspaces.map((item) => (
              <option key={item.id} value={item.id}>
                {item.name} · r{item.revision}
              </option>
            ))}
          </select>
        </label>
      </div>
      {workspace && (
        <p>
          {workspace.name} · r{workspace.revision} · {groups.length}{' '}
          {t('modbus.pollGroups')}
        </p>
      )}
      {workspace && (
        <label className="modbus-profile-choice">
          {t('modbus.profile')}
          <select
            disabled={Boolean(status?.connected)}
            value={`${workspace.profile_ref.id}:${workspace.profile_ref.revision}`}
            onChange={(event) => {
              const profile = profiles.find(
                (item) => `${item.id}:${item.revision}` === event.target.value,
              );
              if (profile) {
                setWorkspace({
                  ...workspace,
                  profile_ref: { id: profile.id, revision: profile.revision },
                });
                setWorkspaceDirty(true);
              }
            }}
          >
            {!profiles.some(
              (item) =>
                item.id === workspace.profile_ref.id &&
                item.revision === workspace.profile_ref.revision,
            ) && (
              <option
                value={`${workspace.profile_ref.id}:${workspace.profile_ref.revision}`}
              >
                {workspace.profile_ref.id} · r{workspace.profile_ref.revision}
              </option>
            )}
            {profiles.map((item) => (
              <option key={item.id} value={`${item.id}:${item.revision}`}>
                {item.name} · r{item.revision}
              </option>
            ))}
          </select>
        </label>
      )}
      {workspace && (
        <div className="modbus-actions">
          <button
            type="button"
            disabled={
              Boolean(status?.connected) ||
              !workspace.map_refs.length ||
              workspace.poll_groups.length >= 32
            }
            onClick={() => addGroup()}
          >
            {t('modbus.addGroup')}
          </button>
          <button
            type="button"
            disabled={Boolean(status?.connected) || !workspaceDirty}
            onClick={() => void saveWorkspace()}
          >
            {t('modbus.saveWorkspace')}
          </button>
        </div>
      )}
      <div className="modbus-actions">
        <button
          type="button"
          disabled={
            !workspace || workspaceDirty || Boolean(status?.connected) || Boolean(jobId)
          }
          onClick={() => void connect()}
        >
          {t('modbus.connect')}
        </button>
        <button
          type="button"
          disabled={!status?.connected || !status.can_control}
          onClick={async () => {
            if (!status?.session_id || !client.disconnectModbus) return;
            try {
              await client.disconnectModbus(credential, endpoint.id, status.session_id);
              await refreshStatus();
            } catch (cause) {
              report(cause);
            }
          }}
        >
          {t('modbus.disconnect')}
        </button>
        <button
          type="button"
          disabled={!status?.connected || !status.can_control || !matchingWorkspace}
          onClick={async () => {
            if (!status?.session_id || !client.pollModbus) return;
            try {
              setStatus(
                await client.pollModbus(
                  credential,
                  endpoint.id,
                  status.session_id,
                  groups,
                  !status.poll_paused,
                ),
              );
            } catch (cause) {
              report(cause);
            }
          }}
        >
          {status?.poll_paused ? t('modbus.startPolling') : t('modbus.pausePolling')}
        </button>
        <button
          type="button"
          disabled={!status?.connected || displayPaused}
          onClick={() => void refreshLive()}
        >
          {t('modbus.refreshLive')}
        </button>
        <button
          type="button"
          disabled={!status?.connected}
          onClick={() => {
            setDisplayPaused((before) => !before);
          }}
        >
          {displayPaused ? t('modbus.resumeDisplay') : t('modbus.pauseDisplay')}
        </button>
        <button
          type="button"
          onClick={() => {
            setTraffic([]);
            setTrend({});
            setSamples({});
            samplesRef.current = {};
            setGap(false);
          }}
        >
          {t('modbus.clearDisplay')}
        </button>
      </div>
      {status?.connected && (
        <div className="modbus-recording-controls">
          <h4>{t('modbus.logging')}</h4>
          <div className="modbus-find">
            <label>
              {t('modbus.name')}{' '}
              <input
                value={loggingName}
                disabled={Boolean(status.logging)}
                onChange={(event) => setLoggingName(event.target.value)}
              />
            </label>
            <label>
              {t('modbus.loggingMode')}{' '}
              <select
                value={loggingMode}
                disabled={Boolean(status.logging)}
                onChange={(event) =>
                  setLoggingMode(event.target.value as typeof loggingMode)
                }
              >
                <option value="both">{t('modbus.loggingBoth')}</option>
                <option value="values">{t('modbus.loggingValues')}</option>
                <option value="traffic">{t('modbus.loggingTraffic')}</option>
              </select>
            </label>
            <label>
              {t('modbus.sampling')}{' '}
              <select
                value={loggingSampling}
                disabled={Boolean(status.logging)}
                onChange={(event) =>
                  setLoggingSampling(event.target.value as typeof loggingSampling)
                }
              >
                <option value="every">{t('modbus.samplingEvery')}</option>
                <option value="interval">{t('modbus.samplingInterval')}</option>
                <option value="change">{t('modbus.samplingChange')}</option>
              </select>
            </label>
            {loggingSampling === 'interval' && (
              <label>
                {t('modbus.sampleInterval')}{' '}
                <input
                  type="number"
                  min={100}
                  max={60000}
                  value={loggingInterval}
                  disabled={Boolean(status.logging)}
                  onChange={(event) => setLoggingInterval(Number(event.target.value))}
                />
              </label>
            )}
            {status.logging ? (
              <button
                type="button"
                disabled={
                  loggingBusy || !status.can_control || !client.stopModbusRecording
                }
                onClick={async () => {
                  if (!status.session_id || !client.stopModbusRecording) return;
                  setLoggingBusy(true);
                  try {
                    const ended = await client.stopModbusRecording(
                      credential,
                      endpoint.id,
                      status.session_id,
                    );
                    setStatus({ ...status, logging: false, recording_id: ended.id });
                    setError('');
                  } catch (cause) {
                    report(cause);
                  } finally {
                    setLoggingBusy(false);
                  }
                }}
              >
                {t('modbus.stopLogging')}
              </button>
            ) : (
              <button
                type="button"
                disabled={
                  loggingBusy ||
                  !status.can_control ||
                  !client.startModbusRecording ||
                  (loggingSampling === 'interval' &&
                    (loggingInterval < 100 || loggingInterval > 60000))
                }
                onClick={async () => {
                  if (!status.session_id || !client.startModbusRecording) return;
                  setLoggingBusy(true);
                  try {
                    const started = await client.startModbusRecording(
                      credential,
                      endpoint.id,
                      status.session_id,
                      {
                        mode: loggingMode,
                        sampling: loggingSampling,
                        interval_ms:
                          loggingSampling === 'interval' ? loggingInterval : 0,
                        name: loggingName,
                      },
                    );
                    setStatus({
                      ...status,
                      logging: true,
                      recording_id: started.id,
                      logging_mode: loggingMode,
                      logging_sampling: loggingSampling,
                    });
                    setError('');
                  } catch (cause) {
                    report(cause);
                  } finally {
                    setLoggingBusy(false);
                  }
                }}
              >
                {t('modbus.startLogging')}
              </button>
            )}
          </div>
          {status.logging && (
            <p role="status">
              {t('modbus.loggingActive')} · {status.recording_id}
            </p>
          )}
          {status.recording_warning && (
            <MessageBanner tone="warning">
              {t('modbus.incomplete')}: {status.recording_warning}
            </MessageBanner>
          )}
        </div>
      )}
      {jobState && (
        <p role="status">
          {t('modbus.connectionJob')}: {jobState}
        </p>
      )}
      {workspaceDirty && <p role="status">{t('modbus.saveBeforeConnect')}</p>}
      {status?.connected && endpoint.transport !== 'rtu' && (
        <>
          <button
            type="button"
            aria-expanded={toolsOpen}
            onClick={() => setToolsOpen((open) => !open)}
          >
            {t('modbus.tools')}
          </button>
          {toolsOpen && (
            <ModbusToolsPanel
              client={client}
              credential={credential}
              endpointId={endpoint.id}
              transport={endpoint.transport}
              sessionId={status.session_id ?? ''}
              canControl={Boolean(status.can_control)}
            />
          )}
        </>
      )}
      {status?.connected && workspace && (
        <p>
          {t('modbus.status')}: {status.state}
          {status.owner_id && (
            <>
              {' '}
              · {t('modbus.owner')}: {status.owner_id}
            </>
          )}
          {status.expires_at && (
            <>
              {' '}
              · {t('modbus.expires')}:{' '}
              <time dateTime={status.expires_at}>{status.expires_at}</time>
            </>
          )}
        </p>
      )}
      {workspace && !status?.connected && <p>{t('modbus.noLiveData')}</p>}
      {workspace && status?.connected && !matchingWorkspace && (
        <MessageBanner tone="warning">{t('modbus.workspaceMismatch')}</MessageBanner>
      )}
      {workspace && Object.keys(maps).length === 0 && <p>{t('modbus.noMaps')}</p>}
      {workspace && (
        <section aria-label={t('modbus.pollGroups')}>
          <h4>{t('modbus.pollGroups')}</h4>
          <div className="modbus-group-list">
            {workspace.poll_groups.map((group) => {
              const metrics = status?.group_stats?.[group.id];
              return (
                <div className="modbus-group" key={group.id}>
                  <strong>{group.name}</strong> · {t('modbus.unit')} {group.unit_id} ·
                  FC{group.function} · {group.address} + {group.quantity}
                  <div>
                    {t('modbus.requestedInterval')}: {group.interval_ms} ms (
                    {(1000 / group.interval_ms).toFixed(2)} Hz) ·{' '}
                    {t('modbus.actualInterval')}: {metrics?.actual_interval_ms ?? '—'}{' '}
                    ms{' '}
                    {metrics?.actual_interval_ms
                      ? `(${(1000 / metrics.actual_interval_ms).toFixed(2)} Hz)`
                      : ''}{' '}
                    · {t('modbus.latency')}: {metrics?.latency_ms ?? '—'} ms ·{' '}
                    {t('modbus.missed')}: {metrics?.missed_polls ?? 0} ·{' '}
                    {t('modbus.errors')}: {metrics?.errors ?? 0}
                  </div>
                  <div className="modbus-actions">
                    <button
                      type="button"
                      disabled={
                        Boolean(status?.connected) || workspace.poll_groups.length >= 32
                      }
                      aria-label={`${t('modbus.duplicate')} ${group.name}`}
                      onClick={() => addGroup(group)}
                    >
                      {t('modbus.duplicate')}
                    </button>
                    <button
                      type="button"
                      disabled={Boolean(status?.connected)}
                      aria-label={`${t('modbus.delete')} ${group.name}`}
                      onClick={() => {
                        setWorkspace(
                          (before) =>
                            before && {
                              ...before,
                              poll_groups: before.poll_groups.filter(
                                (item) => item.id !== group.id,
                              ),
                            },
                        );
                        setWorkspaceDirty(true);
                      }}
                    >
                      {t('modbus.delete')}
                    </button>
                    <button
                      type="button"
                      disabled={
                        !group.enabled ||
                        !status?.connected ||
                        !status.can_control ||
                        !matchingWorkspace
                      }
                      onClick={async () => {
                        if (!status?.session_id || !client.pauseModbusGroup) return;
                        try {
                          setStatus(
                            await client.pauseModbusGroup(
                              credential,
                              endpoint.id,
                              status.session_id,
                              group.id,
                              !metrics?.paused,
                            ),
                          );
                        } catch (cause) {
                          report(cause);
                        }
                      }}
                    >
                      {metrics?.paused
                        ? t('modbus.resumeGroup')
                        : t('modbus.pauseGroup')}
                    </button>
                    <button
                      type="button"
                      disabled={
                        displayPaused ||
                        !group.enabled ||
                        !status?.connected ||
                        !status.can_control ||
                        !matchingWorkspace
                      }
                      onClick={async () => {
                        if (!status?.session_id || !client.readModbus) return;
                        try {
                          const request = groups.find((item) => item.id === group.id)
                            ?.requests[0];
                          if (!request) return;
                          const result = await client.readModbus(
                            credential,
                            endpoint.id,
                            status.session_id,
                            request,
                          );
                          const observation = {
                            sequence: --localSequenceRef.current,
                            timestamp: new Date().toISOString(),
                            group_id: group.id,
                            request,
                            result,
                          };
                          setTraffic((before) =>
                            [...before, observation].slice(-MAX_TRAFFIC),
                          );
                          await applyEvents([observation], false);
                        } catch (cause) {
                          report(cause);
                        }
                      }}
                    >
                      {t('modbus.readOnce')}
                    </button>
                  </div>
                  {!status?.connected && (
                    <div className="modbus-form">
                      <label>
                        {t('modbus.name')}{' '}
                        <input
                          value={group.name}
                          onChange={(event) =>
                            changeGroup(group.id, { name: event.target.value })
                          }
                        />
                      </label>
                      <label>
                        {t('modbus.unit')}{' '}
                        <input
                          type="number"
                          min={endpoint.network?.unit_min ?? 0}
                          max={endpoint.network?.unit_max ?? 255}
                          value={group.unit_id}
                          onChange={(event) =>
                            changeGroup(group.id, {
                              unit_id: Number(event.target.value),
                            })
                          }
                        />
                      </label>
                      <label>
                        {t('modbus.function')}{' '}
                        <select
                          value={group.function}
                          onChange={(event) =>
                            changeGroup(group.id, {
                              function: Number(event.target.value),
                            })
                          }
                        >
                          {[1, 2, 3, 4].map((fn) => (
                            <option key={fn} value={fn}>
                              FC{fn}
                            </option>
                          ))}
                        </select>
                      </label>
                      <label>
                        {t('modbus.address')}{' '}
                        <input
                          type="number"
                          min="0"
                          max="65535"
                          value={group.address}
                          onChange={(event) =>
                            changeGroup(group.id, {
                              address: Number(event.target.value),
                            })
                          }
                        />
                      </label>
                      <label>
                        {t('modbus.quantity')}{' '}
                        <input
                          type="number"
                          min="1"
                          max="2000"
                          value={group.quantity}
                          onChange={(event) =>
                            changeGroup(group.id, {
                              quantity: Number(event.target.value),
                            })
                          }
                        />
                      </label>
                      <label>
                        {t('modbus.requestedInterval')} (ms){' '}
                        <input
                          type="number"
                          min="100"
                          max="60000"
                          value={group.interval_ms}
                          onChange={(event) =>
                            changeGroup(group.id, {
                              interval_ms: Number(event.target.value),
                            })
                          }
                        />
                      </label>
                      <label>
                        {t('modbus.responseTimeout')} (ms){' '}
                        <input
                          type="number"
                          min="100"
                          max="60000"
                          value={group.timeout_ms ?? 5000}
                          onChange={(event) =>
                            changeGroup(group.id, {
                              timeout_ms: Number(event.target.value),
                            })
                          }
                        />
                      </label>
                      <label>
                        {t('modbus.maps')}{' '}
                        <select
                          value={group.map_ref.id}
                          onChange={(event) => {
                            const ref = workspace.map_refs.find(
                              (item) => item.id === event.target.value,
                            );
                            if (ref) changeGroup(group.id, { map_ref: ref });
                          }}
                        >
                          {workspace.map_refs.map((ref) => (
                            <option key={ref.id} value={ref.id}>
                              {ref.id} · r{ref.revision}
                            </option>
                          ))}
                        </select>
                      </label>
                      <label className="modbus-check">
                        <input
                          type="checkbox"
                          checked={group.enabled}
                          onChange={(event) =>
                            changeGroup(group.id, { enabled: event.target.checked })
                          }
                        />
                        {t('modbus.enabled')}
                      </label>
                      <label className="modbus-check">
                        <input
                          type="checkbox"
                          checked={group.stop_on_error ?? false}
                          onChange={(event) =>
                            changeGroup(group.id, {
                              stop_on_error: event.target.checked,
                            })
                          }
                        />
                        {t('modbus.stopOnError')}
                      </label>
                    </div>
                  )}
                </div>
              );
            })}
          </div>
        </section>
      )}
      {workspace && (
        <section aria-label={t('modbus.registerGrid')}>
          <h4>{t('modbus.registerGrid')}</h4>
          <label className="modbus-find">
            {t('modbus.find')}{' '}
            <input
              type="search"
              value={find}
              onChange={(event) => setFind(event.target.value)}
            />
          </label>
          <label>
            {t('modbus.quality')}{' '}
            <select
              value={qualityFilter}
              onChange={(event) => {
                setQualityFilter(event.target.value);
                setGridScroll(0);
              }}
            >
              <option value="">{t('modbus.allQualities')}</option>
              {[
                'waiting',
                'good',
                'stale',
                'timeout',
                'disconnected',
                'decode_error',
              ].map((quality) => (
                <option key={quality} value={quality}>
                  {quality}
                </option>
              ))}
            </select>
          </label>
          <label>
            {t('modbus.sort')}{' '}
            <select
              value={gridSort}
              onChange={(event) => {
                setGridSort(event.target.value as 'address' | 'name');
                setGridScroll(0);
              }}
            >
              <option value="address">{t('modbus.address')}</option>
              <option value="name">{t('modbus.name')}</option>
            </select>
          </label>
          <div
            className="modbus-grid-viewport"
            onScroll={(event) => setGridScroll(event.currentTarget.scrollTop)}
          >
            <table className="modbus-grid" aria-rowcount={filteredPoints.length + 1}>
              <thead>
                <tr>
                  <th>{t('modbus.name')}</th>
                  <th>{t('modbus.unit')}</th>
                  <th>{t('modbus.area')}</th>
                  <th>{t('modbus.address')}</th>
                  <th>{t('modbus.raw')}</th>
                  <th>{t('modbus.value')}</th>
                  <th>{t('modbus.quality')}</th>
                  <th>{t('modbus.lastValid')}</th>
                  <th>{t('modbus.age')}</th>
                  <th>{t('modbus.actions')}</th>
                </tr>
              </thead>
              <tbody>
                {gridStart > 0 && (
                  <tr aria-hidden="true">
                    <td colSpan={10} style={{ height: gridStart * 48, padding: 0 }} />
                  </tr>
                )}
                {visiblePoints.map((item) => {
                  const sample = samples[item.key];
                  const value = sample?.decoded?.value;
                  const age = sample?.lastValidAt
                    ? Math.max(
                        0,
                        Math.floor(
                          (displayNow - Date.parse(sample.lastValidAt)) / 1000,
                        ),
                      )
                    : undefined;
                  return (
                    <tr
                      key={item.key}
                      aria-rowindex={filteredPoints.indexOf(item) + 2}
                      className={
                        sample?.changedAt &&
                        displayNow - Date.parse(sample.changedAt) < 3000
                          ? 'modbus-changed'
                          : undefined
                      }
                    >
                      <th scope="row">{item.point.name}</th>
                      <td>{item.unitId}</td>
                      <td>{item.point.area}</td>
                      <td>
                        {item.point.address} /{' '}
                        {displayAddress(
                          item.point.area,
                          item.point.address,
                          workspace.layout.address_base,
                        )}
                      </td>
                      <td>
                        {sample?.raw.join(', ') || '—'}
                        {sample?.raw.length ? (
                          <small>
                            {' '}
                            (
                            {sample.raw
                              .map((word) => `0x${word.toString(16).padStart(4, '0')}`)
                              .join(' ')}
                            )
                          </small>
                        ) : null}
                      </td>
                      <td>
                        {value === undefined
                          ? '—'
                          : typeof value === 'object'
                            ? JSON.stringify(value)
                            : String(value)}{' '}
                        {item.point.unit}
                        {sample?.decoded?.label && (
                          <small> · {sample.decoded.label}</small>
                        )}
                        {sample?.decoded?.set_bits?.length ? (
                          <small> · {sample.decoded.set_bits.join(', ')}</small>
                        ) : null}
                      </td>
                      <td>{visibleQuality(item)}</td>
                      <td>
                        {sample?.lastValidAt ? (
                          <time dateTime={sample.lastValidAt}>
                            {sample.lastValidAt}
                          </time>
                        ) : (
                          '—'
                        )}
                      </td>
                      <td>
                        {age === undefined || !Number.isFinite(age) ? '—' : `${age}s`}
                      </td>
                      <td>
                        <div className="modbus-actions">
                          <button
                            type="button"
                            onClick={() =>
                              void navigator.clipboard?.writeText(
                                `${item.point.name}\t${sample?.raw.join(', ') ?? ''}\t${value === undefined ? '' : String(value)}`,
                              )
                            }
                          >
                            {t('modbus.copy')}
                          </button>
                          {item.point.access !== 'read' && (
                            <button
                              type="button"
                              disabled={
                                !status?.connected ||
                                !status.can_control ||
                                !matchingWorkspace ||
                                !workspace.poll_groups.find(
                                  (group) => group.id === item.groupId,
                                )?.enabled
                              }
                              aria-label={`${t('modbus.write')} ${item.point.name}`}
                              onClick={(event) => {
                                writeTriggerRef.current = event.currentTarget;
                                setWritePoint(item);
                                setWriteValue('');
                                setWritePreview(undefined);
                                setWriteCommand(undefined);
                                setWriteKey('');
                                setWriteAttempted(false);
                                setWriteSubmitIssue('');
                                setReadBackResult('');
                              }}
                            >
                              {t('modbus.write')}
                            </button>
                          )}
                        </div>
                      </td>
                    </tr>
                  );
                })}
                {filteredPoints.length > gridStart + visiblePoints.length && (
                  <tr aria-hidden="true">
                    <td
                      colSpan={10}
                      style={{
                        height:
                          (filteredPoints.length - gridStart - visiblePoints.length) *
                          48,
                        padding: 0,
                      }}
                    />
                  </tr>
                )}
              </tbody>
            </table>
          </div>
        </section>
      )}
      {workspace && (
        <section aria-label={t('modbus.traffic')}>
          <h4>
            {t('modbus.traffic')}{' '}
            {gap && <span role="status">· {t('modbus.gap')}</span>}
          </h4>
          <label>
            {t('modbus.filterGroup')}{' '}
            <select
              value={trafficGroup}
              onChange={(event) => setTrafficGroup(event.target.value)}
            >
              <option value="">{t('modbus.allGroups')}</option>
              {workspace.poll_groups.map((group) => (
                <option key={group.id} value={group.id}>
                  {group.name}
                </option>
              ))}
            </select>
          </label>
          <div className="modbus-traffic-list">
            {traffic
              .filter((event) => !trafficGroup || event.group_id === trafficGroup)
              .map((event) => (
                <div className="modbus-traffic-pair" key={event.sequence}>
                  <time>{event.timestamp}</time> · {event.group_id}
                  <br />
                  <span>
                    TX: U{event.request.unit} FC{event.request.function} @
                    {event.request.address} ×{event.request.count}
                  </span>
                  <br />
                  <span>
                    RX: {event.result.outcome}{' '}
                    {event.result.values.length ? event.result.values.join(', ') : '—'}
                  </span>
                </div>
              ))}
          </div>
        </section>
      )}
      {workspace && (
        <section aria-label={t('modbus.trends')}>
          <h4>{t('modbus.trends')}</h4>
          <p role="status">
            {t('modbus.quality')}: {trendItem ? visibleQuality(trendItem) : 'waiting'}
            {gap && <> · {t('modbus.gap')}</>}
          </p>
          <label>
            {t('modbus.point')}{' '}
            <select
              value={trendPoint}
              onChange={(event) => setTrendPoint(event.target.value)}
            >
              {points.map((item) => (
                <option value={item.key} key={item.key}>
                  {item.point.name} · {item.groupName}
                </option>
              ))}
            </select>
          </label>
          <svg
            className="modbus-trend"
            viewBox="0 0 300 100"
            role="img"
            aria-label={`${t('modbus.trends')}: ${chartValues.length} ${t('modbus.samples')}`}
          >
            {chartValues.length > 1 && (
              <polyline
                points={polyline}
                fill="none"
                stroke="currentColor"
                strokeWidth="2"
              />
            )}
          </svg>
          <p>
            {chartValues.length} {t('modbus.samples')} · {low}–{high}
          </p>
        </section>
      )}
      {writePoint && (
        <div
          ref={dialogRef}
          role="dialog"
          aria-modal="true"
          aria-label={`${t('modbus.write')} ${writePoint.point.name}`}
          className="modbus-write-dialog"
          onKeyDown={(event) => {
            if (event.key === 'Escape') {
              event.preventDefault();
              setWritePoint(undefined);
            }
            if (event.key !== 'Tab') return;
            const focusable = Array.from(
              dialogRef.current?.querySelectorAll<HTMLElement>(
                'button:not([disabled]), input:not([disabled]), select:not([disabled])',
              ) ?? [],
            );
            if (!focusable.length) return;
            const position = focusable.indexOf(document.activeElement as HTMLElement);
            if (event.shiftKey && position <= 0) {
              event.preventDefault();
              focusable[focusable.length - 1].focus();
            } else if (!event.shiftKey && position === focusable.length - 1) {
              event.preventDefault();
              focusable[0].focus();
            }
          }}
        >
          <div className="modbus-write-content">
            <div className="page-title-actions">
              <h4>
                {t('modbus.write')} {writePoint.point.name}
              </h4>
              <button type="button" onClick={() => setWritePoint(undefined)}>
                {t('modbus.close')}
              </button>
            </div>
            <p>
              {endpoint.name} · {endpoint.network?.host} · {t('modbus.unit')}{' '}
              {writePoint.unitId} · {writePoint.point.area} · {t('modbus.address')}{' '}
              {writePoint.point.address} · {t('modbus.maps')} r{writePoint.mapRevision}
            </p>
            <p>
              {t('modbus.allowedRange')}: {writePoint.point.write_min ?? '—'}–
              {writePoint.point.write_max ?? '—'}
            </p>
            <label>
              {t('modbus.engineeringValue')}
              {writePoint.point.type === 'bool' ? (
                <select
                  value={writeValue}
                  onChange={(event) => {
                    setWriteValue(event.target.value);
                    setWritePreview(undefined);
                  }}
                >
                  <option value="">—</option>
                  <option value="true">true</option>
                  <option value="false">false</option>
                </select>
              ) : (
                <input
                  type="text"
                  value={writeValue}
                  onChange={(event) => {
                    setWriteValue(event.target.value);
                    setWritePreview(undefined);
                  }}
                />
              )}
            </label>
            <label className="modbus-check">
              <input
                type="checkbox"
                checked={readBack}
                onChange={(event) => setReadBack(event.target.checked)}
              />{' '}
              {t('modbus.readBack')}
            </label>
            <div className="modbus-actions">
              <button
                type="button"
                disabled={writeBusy || writeAttempted || !status?.connected}
                onClick={() => void previewWrite()}
              >
                {t('modbus.previewWrite')}
              </button>
              <button
                type="button"
                disabled={
                  !writePreview || writeBusy || writeAttempted || !status?.connected
                }
                onClick={() => void submitWrite()}
              >
                {t('modbus.submitWrite')}
              </button>
            </div>
            {writePreview && (
              <div className="modbus-write-preview">
                <p>
                  {t('modbus.function')}: FC{writePreview.request.function} ·{' '}
                  {t('modbus.raw')}: {writePreview.request.values.join(', ')}
                </p>
                <p>
                  PDU: <code>{writePreview.pdu_hex}</code>
                </p>
                <p>
                  {t('modbus.digest')}: <code>{writePreview.digest}</code>
                </p>
              </div>
            )}
            {writeCommand && (
              <p role="status">
                {t('modbus.writeOutcome')}: {writeCommand.outcome} ·{' '}
                {writeCommand.command_id}
              </p>
            )}
            {writeSubmitIssue && <p role="alert">{writeSubmitIssue}</p>}
            {readBackResult && (
              <p role="status">
                {t('modbus.readBack')}: {readBackResult}
              </p>
            )}
          </div>
        </div>
      )}
    </section>
  );
}
