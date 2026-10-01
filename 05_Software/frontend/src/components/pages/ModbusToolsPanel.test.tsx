import { fireEvent, screen, waitFor } from '@testing-library/react';
import { expect, it, vi } from 'vitest';
import { createBenchCoreClient, sessionCredential } from '../../api/client';
import { renderInShell } from '../../test/render';
import { ModbusToolsPanel } from './ModbusToolsPanel';
import type { ModbusRequest } from '../../api/modbus';

it('starts a bounded scan only after an explicit action and shows failed probes', async () => {
  const start = vi.fn(async () => ({
    scan_id: 'scan',
    session_id: 'live',
    plan: {
      session_id: 'live',
      function: 3,
      unit_from: 1,
      unit_to: 1,
      address_from: 10,
      address_to: 11,
      rate_per_second: 2,
      deadline_seconds: 5,
    },
    state: 'completed',
    total: 2,
    completed: 2,
    results: [
      { function: 3, unit: 1, address: 10, outcome: 'acknowledged', values: [42] },
      { function: 3, unit: 1, address: 11, outcome: 'timeout', values: [] },
    ],
  }));
  const client = { ...createBenchCoreClient({ baseUrl: '' }), startModbusScan: start };
  renderInShell(
    <ModbusToolsPanel
      client={client}
      credential={sessionCredential('csrf')}
      endpointId="boiler"
      transport="tcp"
      sessionId="live"
      canControl
    />,
  );
  expect(start).not.toHaveBeenCalled();
  fireEvent.change(screen.getByLabelText('Address from'), { target: { value: '10' } });
  fireEvent.change(screen.getByLabelText('Address to'), { target: { value: '11' } });
  fireEvent.click(screen.getByRole('button', { name: 'Start scan' }));
  await waitFor(() => expect(start).toHaveBeenCalledTimes(1));
  expect(await screen.findByText('timeout')).toBeTruthy();
  expect(start).toHaveBeenCalledWith(
    expect.anything(),
    'boiler',
    expect.objectContaining({ function: 3, address_from: 10, address_to: 11 }),
  );
});

it('previews one FC22 write and submits the exact previewed request only once', async () => {
  const preview = vi.fn(
    async (
      _credential: unknown,
      _endpoint: string,
      _session: string,
      _key: string,
      request: ModbusRequest,
    ) => ({
      session_id: 'live',
      endpoint_revision: 1,
      profile_revision: 1,
      request,
      pdu_hex: '160000ffff0000',
      digest: 'digest',
    }),
  );
  const write = vi.fn(async () => ({
    command_id: 'command',
    outcome: 'delivery_unknown',
    result: {
      request_id: 'write',
      outcome: 'delivery_unknown',
      attempts: 1,
      values: [],
    },
  }));
  const client = {
    ...createBenchCoreClient({ baseUrl: '' }),
    previewModbusWrite: preview,
    writeModbus: write,
  };
  renderInShell(
    <ModbusToolsPanel
      client={client}
      credential={sessionCredential('csrf')}
      endpointId="boiler"
      transport="tcp"
      sessionId="live"
      canControl
    />,
  );
  fireEvent.click(screen.getByRole('button', { name: 'Preview write' }));
  expect(await screen.findByText('160000ffff0000')).toBeTruthy();
  fireEvent.click(screen.getByRole('button', { name: 'Submit write' }));
  await waitFor(() => expect(write).toHaveBeenCalledTimes(1));
  expect(write).toHaveBeenCalledWith(
    expect.anything(),
    'boiler',
    'live',
    expect.any(String),
    expect.objectContaining({ function: 22, and_mask: 65535, or_mask: 0 }),
  );
  expect(screen.getByRole('button', { name: 'Submit write' })).toBeDisabled();
  expect(screen.getByText(/delivery_unknown/)).toBeTruthy();
});
