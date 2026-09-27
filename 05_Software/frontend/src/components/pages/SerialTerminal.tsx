import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { useTranslate } from '../../i18n/languages';
import type { TerminalRow } from './serialBuffer';
import { MessageBanner } from '../MessageBanner';
import { readPreference, writePreference } from '../../preferences/store';

function displayPreference() {
  try {
    const saved = JSON.parse(readPreference('serialDisplay') ?? 'null');
    if (
      saved &&
      ['ascii', 'utf8', 'hex'].includes(saved.encoding) &&
      typeof saved.timestamps === 'boolean' &&
      typeof saved.wrap === 'boolean'
    )
      return saved as { encoding: string; timestamps: boolean; wrap: boolean };
  } catch {
    /* Invalid preferences use the normal display. */
  }
  return { encoding: 'utf8', timestamps: true, wrap: true };
}

export function SerialTerminal({
  rows,
  gap,
  onClear,
  children,
  status,
  workspace = false,
}: {
  rows: TerminalRow[];
  gap: boolean;
  onClear: () => void;
  children?: ReactNode;
  status?: ReactNode;
  workspace?: boolean;
}) {
  const t = useTranslate();
  const [paused, setPaused] = useState<TerminalRow[] | null>(null);
  const [encoding, setEncoding] = useState(() => displayPreference().encoding);
  const [timestamps, setTimestamps] = useState(() => displayPreference().timestamps);
  const [wrap, setWrap] = useState(() => displayPreference().wrap);
  const [direction, setDirection] = useState('all');
  const [find, setFind] = useState('');
  const [autoScroll, setAutoScroll] = useState(true);
  const [scroll, setScroll] = useState(0);
  const [size, setSize] = useState({ width: 640, height: 320 });
  const viewport = useRef<HTMLDivElement>(null);
  useEffect(
    () =>
      writePreference('serialDisplay', JSON.stringify({ encoding, timestamps, wrap })),
    [encoding, timestamps, wrap],
  );
  useEffect(() => {
    if (!viewport.current || typeof ResizeObserver === 'undefined') return;
    const observer = new ResizeObserver((entries) => {
      const { width, height } = entries[0].contentRect;
      setSize({ width, height });
    });
    observer.observe(viewport.current);
    return () => observer.disconnect();
  }, []);
  const lines = useMemo(() => {
    const columns = Math.max(8, Math.floor((size.width - 24) / 9));
    return (paused ?? rows)
      .filter(
        (row) =>
          direction === 'all' ||
          row.direction === direction ||
          !['rx', 'tx'].includes(row.direction),
      )
      .flatMap((row) => {
        const ascii =
          encoding === 'ascii'
            ? row.hex
                .trim()
                .split(' ')
                .filter(Boolean)
                .map((byte) => {
                  const value = parseInt(byte, 16);
                  return value === 10
                    ? ''
                    : value >= 32 && value < 127
                      ? String.fromCharCode(value)
                      : `\\x${byte}`;
                })
                .join('')
            : '';
        const data = !row.hex
          ? row.text
          : encoding === 'hex'
            ? `[${(row.offset ?? 0).toString(16).padStart(8, '0')}] ${row.hex}`
            : encoding === 'ascii'
              ? ascii
              : row.text;
        const content = `${timestamps ? `${row.timestamp} ` : ''}${row.direction.toUpperCase()} ${data}`;
        if (find && !content.toLocaleLowerCase().includes(find.toLocaleLowerCase()))
          return [];
        if (!wrap) return [{ id: `${row.id}`, content, direction: row.direction }];
        const chunks = [];
        for (let index = 0; index < content.length; index += columns)
          chunks.push({
            id: `${row.id}-${index}`,
            content: content.slice(index, index + columns),
            direction: row.direction,
          });
        return chunks;
      });
  }, [rows, paused, encoding, timestamps, direction, find, wrap, size.width]);
  const rowHeight = 24;
  const start = Math.max(0, Math.floor(scroll / rowHeight) - 5);
  const count = Math.ceil(size.height / rowHeight) + 10;
  useEffect(() => {
    if (autoScroll && !paused && viewport.current)
      viewport.current.scrollTop = lines.length * rowHeight;
  }, [lines.length, autoScroll, paused]);
  return (
    <>
      <section
        className={`serial-terminal-panel ${workspace ? 'instrument-card serial-live-panel' : ''}`}
      >
        <div className="form-actions serial-toolbar">
          <button
            onClick={() => {
              onClear();
              setPaused(null);
            }}
          >
            {t('sw.clear')}
          </button>
          <button
            aria-pressed={paused !== null}
            onClick={() => setPaused(paused ? null : rows.map((row) => ({ ...row })))}
          >
            {t(paused ? 'sw.resume' : 'sw.pause')}
          </button>
          <label>
            <input
              type="checkbox"
              checked={autoScroll}
              onChange={(e) => setAutoScroll(e.target.checked)}
            />
            {t('sw.autoScroll')}
          </label>
          {status}
          <input
            type="search"
            aria-label={t('sw.find')}
            placeholder={t('sw.find')}
            value={find}
            onChange={(e) => setFind(e.target.value)}
          />
        </div>

        {gap && <MessageBanner tone="warning">{t('sw.gap')}</MessageBanner>}
        <div
          ref={viewport}
          className="serial-terminal"
          tabIndex={0}
          role="region"
          aria-label={t('sw.realtime')}
          onScroll={(e) => {
            const node = e.currentTarget;
            setScroll(node.scrollTop);
            if (node.scrollHeight - node.clientHeight - node.scrollTop > 48)
              setAutoScroll(false);
          }}
        >
          {!lines.length && <p>{t('sw.empty')}</p>}
          <div
            style={{
              height: lines.length * rowHeight,
              position: 'relative',
              minWidth: wrap ? undefined : 'max-content',
            }}
          >
            {lines.slice(start, start + count).map((line, index) => (
              <div
                className={`serial-line serial-${line.direction}`}
                key={line.id}
                style={{
                  position: 'absolute',
                  top: (start + index) * rowHeight,
                  height: rowHeight,
                }}
              >
                {line.content}
              </div>
            ))}
          </div>
        </div>
        {children}
      </section>
      <details
        className={workspace ? 'instrument-card serial-settings-section' : undefined}
      >
        <summary>{t('sw.display')}</summary>
        <div className="form-actions serial-toolbar">
          <label>
            {t('sw.display')}
            <select value={encoding} onChange={(e) => setEncoding(e.target.value)}>
              <option value="utf8">UTF-8</option>
              <option value="ascii">ASCII</option>
              <option value="hex">HEX</option>
            </select>
          </label>
          <label>
            <input
              type="checkbox"
              checked={timestamps}
              onChange={(e) => setTimestamps(e.target.checked)}
            />
            {t('sw.timestamps')}
          </label>
          <label>
            <input
              type="checkbox"
              checked={wrap}
              onChange={(e) => setWrap(e.target.checked)}
            />
            {t('sw.wrap')}
          </label>
          <select
            aria-label={t('sw.all')}
            value={direction}
            onChange={(e) => setDirection(e.target.value)}
          >
            <option value="all">{t('sw.all')}</option>
            <option value="rx">RX</option>
            <option value="tx">TX</option>
          </select>
        </div>
      </details>
    </>
  );
}
