import type { SerialEvent, SerialSend } from '../../api/serial';

export interface TerminalRow {
  id: number;
  direction: string;
  timestamp: string;
  text: string;
  hex: string;
  offset?: number;
}
const MAX_BYTES = 2 * 1024 * 1024;
const MAX_ROWS = 10000;
const escapeControls = (text: string) =>
  Array.from(text)
    .map((char) => {
      const code = char.codePointAt(0)!;
      return (code < 32 && char !== '\t') || (code >= 127 && code <= 159)
        ? `\\x${code.toString(16).padStart(2, '0')}`
        : char;
    })
    .join('');

/** Bounded plain-text projection. Split UTF-8 decoding is independent for RX and TX. */
export class TerminalBuffer {
  rows: TerminalRow[] = [];
  dropped = false;
  private decoders = { rx: new TextDecoder(), tx: new TextDecoder() };
  private bytes = 0;
  private next = 0;
  private offsets = { rx: 0, tx: 0 };
  private pending: Partial<Record<'rx' | 'tx', TerminalRow>> = {};
  clear() {
    this.rows = [];
    this.bytes = 0;
    this.pending = {};
    this.dropped = false;
    this.decoders = { rx: new TextDecoder(), tx: new TextDecoder() };
    this.offsets = { rx: 0, tx: 0 };
  }
  add(events: SerialEvent[], gap = false) {
    if (gap) {
      this.dropped = true;
      this.pending = {};
      this.decoders = { rx: new TextDecoder(), tx: new TextDecoder() };
    }
    for (const event of events) {
      if (event.direction !== 'rx' && event.direction !== 'tx') {
        this.pending = {};
        this.rows.push({
          id: ++this.next,
          direction: event.direction,
          timestamp: event.timestamp,
          text: event.message ?? event.direction,
          hex: '',
        });
        continue;
      }
      const direction = event.direction;
      this.pending[direction === 'rx' ? 'tx' : 'rx'] = undefined;
      const raw = Uint8Array.from(atob(event.data_base64 || ''), (c) =>
        c.charCodeAt(0),
      );
      // Decode one byte at a time to preserve the timestamp of a split codepoint/line.
      for (const byte of raw) {
        let row = this.pending[direction];
        if (!row || row.hex.length >= 1536) {
          row = {
            id: ++this.next,
            direction,
            timestamp: event.timestamp,
            text: '',
            hex: '',
            offset: this.offsets[direction],
          };
          this.rows.push(row);
          this.pending[direction] = row;
        }
        row.hex += `${byte.toString(16).padStart(2, '0')} `;
        this.offsets[direction]++;
        row.text += escapeControls(
          this.decoders[direction].decode(Uint8Array.of(byte), { stream: true }),
        );
        if (byte === 10) {
          row.text = row.text.replace(/\\x0a$/, '');
          this.pending[direction] = undefined;
        }
      }
    }
    this.bytes = this.rows.reduce(
      (total, row) => total + 2 * (row.text.length + row.hex.length + 80),
      0,
    );
    while (this.rows.length > MAX_ROWS || this.bytes > MAX_BYTES) {
      const row = this.rows.shift()!;
      this.bytes -= 2 * (row.text.length + row.hex.length + 80);
      if (this.pending.rx === row) this.pending.rx = undefined;
      if (this.pending.tx === row) this.pending.tx = undefined;
      this.dropped = true;
    }
  }
}

/** Validate complete payload and terminator before any transport call. */
export function validSerialSend(
  data: string,
  encoding: SerialSend['encoding'],
  ending: SerialSend['ending'],
): boolean {
  let count: number;
  if (encoding === 'hex') {
    if (!/^(?:\s*[0-9a-fA-F]{2}\s*)*$/.test(data)) return false;
    count = data.replace(/\s/g, '').length / 2;
  } else {
    if (encoding === 'ascii' && Array.from(data).some((c) => c.codePointAt(0)! > 127))
      return false;
    count = new TextEncoder().encode(data).length;
  }
  count += ending === 'crlf' ? 2 : ending === 'none' ? 0 : 1;
  return count > 0 && count <= 4096;
}
