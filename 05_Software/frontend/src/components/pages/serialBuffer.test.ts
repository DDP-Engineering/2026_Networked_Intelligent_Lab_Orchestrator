import { expect, it } from 'vitest';
import { TerminalBuffer, validSerialSend } from './serialBuffer';
import type { SerialEvent } from '../../api/serial';
const event = (bytes: number[], sequence = 1): SerialEvent => ({
  sequence,
  timestamp: `time-${sequence}`,
  elapsed_ms: sequence,
  direction: 'rx',
  data_base64: btoa(String.fromCharCode(...bytes)),
  byte_count: bytes.length,
});
it('decodes split UTF-8 and stamps a line at its first byte without interpreting escapes', () => {
  const buffer = new TerminalBuffer();
  buffer.add([event([0xe2])]);
  buffer.add([event([0x82, 0xac, 27, 91, 65, 10], 2)]);
  expect(buffer.rows).toHaveLength(1);
  expect(buffer.rows[0].text).toBe('€\\x1b[A');
  expect(buffer.rows[0].timestamp).toBe('time-1');
  expect(buffer.rows[0].hex).toBe('e2 82 ac 1b 5b 41 0a ');
});
it('bounds long unterminated output and reports dropped history', () => {
  const buffer = new TerminalBuffer();
  const bytes = Array(4096).fill(65) as number[];
  for (let n = 0; n < 150; n++) buffer.add([event(bytes, n + 1)]);
  expect(buffer.dropped).toBe(true);
  expect(buffer.rows.length).toBeLessThanOrEqual(10000);
  expect(
    buffer.rows.reduce((n, row) => n + 2 * (row.text.length + row.hex.length + 80), 0),
  ).toBeLessThanOrEqual(2 * 1024 * 1024);
  buffer.clear();
  expect(buffer.rows).toEqual([]);
});
it('rejects malformed hex, non-ASCII and overlong encoded messages before sending', () => {
  expect(validSerialSend('01 ff\n0a', 'hex', 'crlf')).toBe(true);
  expect(validSerialSend('0', 'hex', 'none')).toBe(false);
  expect(validSerialSend('xx', 'hex', 'none')).toBe(false);
  expect(validSerialSend('é', 'ascii', 'none')).toBe(false);
  expect(validSerialSend('é', 'utf8', 'lf')).toBe(true);
  expect(validSerialSend('x'.repeat(4096), 'ascii', 'cr')).toBe(false);
  expect(validSerialSend('x'.repeat(4096), 'ascii', 'none')).toBe(true);
});

it('keeps state events in chronological order and escapes C1 controls', () => {
  const buffer = new TerminalBuffer();
  buffer.add([
    event([65]),
    { ...event([], 2), direction: 'state', message: 'logging started' },
    event([66, 0xc2, 0x85, 10], 3),
  ]);
  expect(buffer.rows.map((row) => row.text)).toEqual([
    'A',
    'logging started',
    'B\\x85',
  ]);
});
