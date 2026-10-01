import { useCallback, useEffect, useMemo, useState } from 'react';
import { BenchCoreError, type BenchCoreClient, type Credential } from '../../api/client';
import type { ModbusRecordedEvent, ModbusRecording, ModbusRecordingSearch } from '../../api/modbus';
import { useTranslate } from '../../i18n/languages';
import { MessageBanner } from '../MessageBanner';

interface Props {
  client: BenchCoreClient;
  credential: Credential;
  endpointId?: string;
  onSessionEnded: () => void;
}

const formats = ['jsonl', 'csv', 'text', 'bundle'] as const;

/** Saved recordings are inspected without acquiring a live Modbus session. */
export function ModbusSessionsPage({ client, credential, endpointId, onSessionEnded }: Props) {
  const t = useTranslate();
  const [records, setRecords] = useState<ModbusRecording[]>([]);
  const [before, setBefore] = useState('');
  const [recordingName, setRecordingName] = useState('');
  const [recordingTag, setRecordingTag] = useState('');
  const [startedFrom, setStartedFrom] = useState('');
  const [startedThrough, setStartedThrough] = useState('');
  const [appliedSearch, setAppliedSearch] = useState<ModbusRecordingSearch>({});
  const [searched, setSearched] = useState(false);
  const [selected, setSelected] = useState<ModbusRecording>();
  const [events, setEvents] = useState<ModbusRecordedEvent[]>([]);
  const [cursor, setCursor] = useState(0);
  const [partial, setPartial] = useState(false);
  const [query, setQuery] = useState('');
  const [kind, setKind] = useState('');
  const [hex, setHex] = useState('');
  const [unit, setUnit] = useState('');
  const [eventAddress, setEventAddress] = useState('');
  const [outcome, setOutcome] = useState('');
  const [name, setName] = useState('');
  const [tags, setTags] = useState('');
  const [format, setFormat] = useState<(typeof formats)[number]>('jsonl');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  const report = useCallback((cause: unknown) => {
    if (cause instanceof BenchCoreError && cause.failure === 'unauthorized') onSessionEnded();
    setError(cause instanceof BenchCoreError ? cause.code || cause.message : t('modbus.unavailable'));
  }, [onSessionEnded, t]);

  const refresh = useCallback(async (pageBefore = '') => {
    if (!client.listModbusRecordings) return;
    const page = await client.listModbusRecordings(credential, { ...appliedSearch, endpoint_id: endpointId, before: pageBefore });
    setRecords((prior) => pageBefore ? [...prior, ...page.recordings] : page.recordings);
    setBefore(page.next_cursor ?? '');
  }, [client, credential, endpointId, appliedSearch]);

  async function searchRecordings() {
    if (!client.listModbusRecordings) return;
    setBusy(true);
    const search: ModbusRecordingSearch = { name: recordingName.trim(), tag: recordingTag.trim(), endpoint_id: endpointId,
      started_from: startedFrom ? new Date(startedFrom).toISOString() : undefined,
      started_through: startedThrough ? new Date(`${startedThrough}T23:59:59.999`).toISOString() : undefined };
    try {
      const page = await client.listModbusRecordings(credential, search);
      setAppliedSearch(search);
      setSearched(true);
      setRecords(page.recordings);
      setBefore(page.next_cursor ?? '');
      setError('');
    } catch (cause) { report(cause); }
    finally { setBusy(false); }
  }

  useEffect(() => {
    let active = true;
    void (async () => {
      try {
        if (!client.listModbusRecordings) return;
        const page = await client.listModbusRecordings(credential, { endpoint_id: endpointId });
        if (active) {
          setRecords(page.recordings);
          setBefore(page.next_cursor ?? '');
        }
      } catch (cause) { if (active) report(cause); }
    })();
    return () => { active = false; };
  }, [client, credential, endpointId, report]);

  async function open(record: ModbusRecording) {
    setSelected(record);
    setName(record.name);
    setTags(record.tags.join(', '));
    setCursor(0);
    setEvents([]);
    setPartial(false);
    try {
      const page = await client.findModbusRecording?.(credential, record.id, 0);
      if (page) {
        setEvents(page.events);
        setCursor(page.next_cursor);
        setPartial(page.truncated);
      }
    } catch (cause) { report(cause); }
  }

  async function find(after = 0) {
    if (!selected || !client.findModbusRecording) return;
    setBusy(true);
    try {
      const page = await client.findModbusRecording(credential, selected.id, after, { kind, text: query, hex, unit, address: eventAddress, outcome });
      setEvents((prior) => after ? [...prior, ...page.events] : page.events);
      setCursor(page.next_cursor);
      setPartial(page.truncated);
      setError('');
    } catch (cause) { report(cause); }
    finally { setBusy(false); }
  }

  async function saveMetadata() {
    if (!selected || !client.updateModbusRecording) return;
    setBusy(true);
    try {
      const updated = await client.updateModbusRecording(credential, selected.id, name, tags.split(',').map((tag) => tag.trim()).filter(Boolean));
      setSelected(updated);
      setRecords((prior) => prior.map((record) => record.id === updated.id ? updated : record));
      setError('');
    } catch (cause) { report(cause); }
    finally { setBusy(false); }
  }

  async function download() {
    if (!selected || !client.downloadModbusRecording) return;
    setBusy(true);
    try {
      const blob = await client.downloadModbusRecording(credential, selected.id, format);
      const href = URL.createObjectURL(blob);
      const link = document.createElement('a');
      link.href = href;
      link.download = `${selected.id}.${format === 'bundle' ? 'zip' : format}`;
      link.click();
      window.setTimeout(() => URL.revokeObjectURL(href), 0);
      setError('');
    } catch (cause) { report(cause); }
    finally { setBusy(false); }
  }

  async function remove() {
    if (!selected || !client.deleteModbusRecording || !window.confirm(t('modbus.confirmRecordingDelete'))) return;
    setBusy(true);
    try {
      await client.deleteModbusRecording(credential, selected.id);
      setRecords((prior) => prior.filter((record) => record.id !== selected.id));
      setSelected(undefined);
      setEvents([]);
      setError('');
    } catch (cause) { report(cause); }
    finally { setBusy(false); }
  }

  const samples = useMemo(() => events.filter((event) => event.kind === 'sample'), [events]);
  const traffic = useMemo(() => events.filter((event) => event.kind === 'traffic' || event.kind === 'write'), [events]);
  const trends = useMemo(() => {
    const groups = new Map<string, number[]>();
    for (const event of samples) {
      const typed = event.typed as { value?: unknown } | undefined;
      if (typeof typed?.value !== 'number' || !Number.isFinite(typed.value)) continue;
      const key = event.point_id || event.group_id || String(event.request.address);
      groups.set(key, [...(groups.get(key) ?? []), typed.value]);
    }
    return [...groups.entries()].filter(([, values]) => values.length > 1);
  }, [samples]);

  return (
    <section className="instrument-card modbus-sessions">
      <h3>{t('modbus.sessions')}</h3>
      {error && <MessageBanner tone="error">{error}</MessageBanner>}
      <div className="modbus-find">
        <label>{t('modbus.name')} <input value={recordingName} onChange={(event) => setRecordingName(event.target.value)} /></label>
        <label>{t('modbus.tag')} <input value={recordingTag} onChange={(event) => setRecordingTag(event.target.value)} /></label>
        <label>{t('modbus.from')} <input type="date" value={startedFrom} onChange={(event) => setStartedFrom(event.target.value)} /></label>
        <label>{t('modbus.through')} <input type="date" value={startedThrough} onChange={(event) => setStartedThrough(event.target.value)} /></label>
        <button type="button" disabled={busy} onClick={() => void searchRecordings()}>{t('modbus.find')}</button>
      </div>
      {records.length === 0 && <p>{t(searched ? 'modbus.noMatches' : 'modbus.noRecordings')}</p>}
      <ul className="modbus-list">
        {records.map((record) => <li key={record.id}>
          <strong>{record.name || record.id}</strong> · {record.started_at} · {record.status}
          {record.reason && <span> · {record.reason}</span>}
          <button type="button" onClick={() => void open(record)}>{t('modbus.openRecording')}</button>
        </li>)}
      </ul>
      {before && <button type="button" disabled={busy} onClick={() => void refresh(before).catch(report)}>{t('modbus.more')}</button>}
      {selected && <>
        <h4>{selected.name || selected.id}</h4>
        <p>{selected.status} · {selected.last_sequence} {t('modbus.eventsCount')} · {selected.part_count} {t('modbus.parts')}</p>
        {selected.status === 'incomplete' && <MessageBanner tone="warning">{t('modbus.incomplete')}: {selected.reason}</MessageBanner>}
        <div className="modbus-find">
          <label>{t('modbus.find')} <input value={query} onChange={(event) => setQuery(event.target.value)} /></label>
          <label>{t('modbus.kind')} <select value={kind} onChange={(event) => setKind(event.target.value)}>
            <option value="">{t('modbus.allKinds')}</option><option value="sample">{t('modbus.registerGrid')}</option><option value="traffic">{t('modbus.traffic')}</option><option value="write">{t('modbus.write')}</option>
          </select></label>
          <label>{t('modbus.hex')} <input value={hex} onChange={(event) => setHex(event.target.value)} /></label>
          <label>{t('modbus.unit')} <input type="number" min="1" max="255" value={unit} onChange={(event) => setUnit(event.target.value)} /></label>
          <label>{t('modbus.address')} <input type="number" min="0" max="65535" value={eventAddress} onChange={(event) => setEventAddress(event.target.value)} /></label>
          <label>{t('modbus.outcome')} <input value={outcome} onChange={(event) => setOutcome(event.target.value)} /></label>
          <button type="button" disabled={busy} onClick={() => void find()}>{t('modbus.find')}</button>
        </div>
        {partial && <p role="status">{t('modbus.partialResults')}</p>}
        <div className="modbus-grid-viewport" tabIndex={0} aria-label={t('modbus.registerGrid')}>
          <table className="modbus-grid"><thead><tr><th>{t('modbus.point')}</th><th>{t('modbus.unit')}</th><th>{t('modbus.address')}</th><th>{t('modbus.raw')}</th><th>{t('modbus.value')}</th><th>{t('modbus.quality')}</th></tr></thead>
            <tbody>{samples.map((event) => <tr key={event.sequence}><td>{event.point_id || event.group_id}</td><td>{event.request.unit}</td><td>{event.request.address}</td><td>{JSON.stringify(event.raw ?? event.result.values)}</td><td>{JSON.stringify(event.typed ?? '')}</td><td>{event.quality}</td></tr>)}</tbody>
          </table>
        </div>
        <h4>{t('modbus.traffic')}</h4>
        <div className="modbus-traffic-list">{traffic.map((event) => <p className="modbus-traffic-pair" key={event.sequence}>{event.timestamp} · {event.kind} · FC{event.request.function} · {event.result.outcome}<br /><code>{event.request_hex || '—'} / {event.reply_hex || '—'}</code></p>)}</div>
        <h4>{t('modbus.trends')}</h4>
        {trends.map(([point, values]) => {
          const low = Math.min(...values), high = Math.max(...values);
          const span = high - low || 1;
          const polyline = values.map((value, index) => `${(index / (values.length - 1)) * 300},${90 - ((value - low) / span) * 80}`).join(' ');
          return <figure key={point}><figcaption>{point}</figcaption><svg className="modbus-trend" viewBox="0 0 300 100" role="img" aria-label={`${point}: ${values.join(', ')}`}><polyline fill="none" stroke="currentColor" strokeWidth="2" points={polyline} /></svg></figure>;
        })}
        {partial && <button type="button" disabled={busy} onClick={() => void find(cursor)}>{t('modbus.more')}</button>}
        <div className="modbus-actions">
          <label>{t('modbus.name')} <input value={name} onChange={(event) => setName(event.target.value)} /></label>
          <label>{t('modbus.tags')} <input value={tags} onChange={(event) => setTags(event.target.value)} /></label>
          <button type="button" disabled={busy || !client.updateModbusRecording} onClick={() => void saveMetadata()}>{t('modbus.save')}</button>
        </div>
        <div className="modbus-actions">
          <label>{t('modbus.export')} <select value={format} onChange={(event) => setFormat(event.target.value as typeof format)}>{formats.map((choice) => <option key={choice} value={choice}>{choice.toUpperCase()}</option>)}</select></label>
          <button type="button" disabled={busy || !client.downloadModbusRecording} onClick={() => void download()}>{t('modbus.download')}</button>
          <button type="button" disabled={busy || selected.status === 'active' || !client.deleteModbusRecording} onClick={() => void remove()}>{t('modbus.delete')}</button>
        </div>
      </>}
    </section>
  );
}
