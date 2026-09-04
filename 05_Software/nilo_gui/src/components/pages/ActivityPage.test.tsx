import { fireEvent, screen, waitFor, within } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import type {
  ActivityFilter,
  ActivityPage as ActivityPageBody,
} from '../../api/activity';
import { renderInShell } from '../../test/render';
import { ActivityPage } from './ActivityPage';

function page(items: ActivityPageBody['items'], nextCursor = ''): ActivityPageBody {
  return {
    items,
    nextCursor,
    retainedWindow: {
      audit: {
        oldest: new Date('2026-09-01T00:00:00Z'),
        newest: new Date('2026-09-04T10:00:00Z'),
        count: items.length,
        truncated: true,
        purgedBefore: new Date('2026-09-01T00:00:00Z'),
        purgeReason: 'age',
      },
    },
    loggingHealth: {
      auditWritable: true,
      failures: 0,
      lastFailure: '',
      droppedDebug: 2,
    },
  };
}

const event = (sequence: number, outcome: string) => ({
  id: `event-${sequence}`,
  sequence,
  createdAt: new Date('2026-09-04T10:00:00Z'),
  category: 'audit',
  component: 'network.interfaces',
  action: 'interface.disable',
  outcome,
  severity: 'info',
  actorKind: 'user',
  actorId: 'user-1',
  subjectType: 'interface',
  subjectId: 'sim-enp1s0',
  targetLabel: 'enp1s0',
  operationId: 'op-1',
  applianceId: 'appliance-1',
  bootId: 'boot-1',
  timeSource: 'unsynchronized',
  details: { state: outcome },
});

describe('ActivityPage', () => {
  // REQ-UX-1c5eab92 and REQ-FUNC-d4f81a26: the page opens narrowed, sends its
  // filters to the appliance, pages by cursor, and shows the retained window
  // and the gap retention left.
  it('opens with the given filters, pages by cursor and discloses the gap', async () => {
    const readActivity = vi.fn((filter: ActivityFilter) =>
      Promise.resolve(
        filter.cursor === '2'
          ? page([event(3, 'confirmed')])
          : page([event(1, 'requested'), event(2, 'applied')], '2'),
      ),
    );
    renderInShell(
      <ActivityPage
        initialFilter={{ component: 'network.interfaces', target: 'sim-enp1s0' }}
        readActivity={readActivity}
        canExport={false}
        canManageDiagnostics={false}
      />,
      { theme: 'light', language: 'en' },
    );
    expect(await screen.findByText('requested')).toBeVisible();
    expect(readActivity.mock.calls[0][0]).toMatchObject({
      component: 'network.interfaces',
      target: 'sim-enp1s0',
    });
    expect(screen.getByLabelText('Function')).toHaveValue('network.interfaces');
    expect(screen.getByText(/Entries before .* were removed \(age\)/)).toBeVisible();
    expect(screen.getByText(/2 debug records were dropped/)).toBeVisible();

    fireEvent.click(screen.getByRole('button', { name: 'Older entries' }));
    expect(await screen.findByText('confirmed')).toBeVisible();
    expect(readActivity.mock.calls[1][0]).toMatchObject({ cursor: '2' });
    expect(screen.queryByRole('button', { name: 'Older entries' })).toBeNull();

    // REQ-UX-f30891b4: time, outcome, action, subject and actor stay in the
    // row; category and function are in the explicit details with the raw
    // record.
    expect(screen.queryByRole('columnheader', { name: 'Category' })).toBeNull();
    expect(screen.queryByRole('columnheader', { name: 'Function' })).toBeNull();
    for (const column of ['Time', 'Outcome', 'Action', 'Target', 'Actor']) {
      expect(screen.getByRole('columnheader', { name: column })).toBeVisible();
    }
    expect(screen.queryByText('network.interfaces')).toBeNull();

    // A row expands to its safe details and can narrow to its operation.
    fireEvent.click(screen.getByRole('button', { name: 'Details 1' }));
    const details = screen.getByRole('region', { name: 'Details 1' });
    expect(within(details).getByText('Audit')).toBeVisible();
    expect(within(details).getByText('network.interfaces')).toBeVisible();
    expect(screen.getByText(/"state": "requested"/)).toBeVisible();
    fireEvent.click(
      screen.getByRole('button', { name: /Show every stage of operation op-1/ }),
    );
    await waitFor(() =>
      expect(
        readActivity.mock.calls[readActivity.mock.calls.length - 1][0],
      ).toMatchObject({
        operation: 'op-1',
      }),
    );
  });

  // REQ-FUNC-35da79f1 at the interface: a session is started for a function
  // and a duration, listed with its effective level, and stopped.
  it('starts and stops a timed debug session and saves the policy', async () => {
    const startSession = vi.fn(() =>
      Promise.resolve({
        id: 's-1',
        scope: 'network.wireguard',
        target: 'office',
        actorId: 'user-1',
        minutes: 15,
        startedAt: new Date(),
        expiresAt: new Date(Date.now() + 900_000),
        state: 'active' as const,
        endedAt: null,
        endReason: '',
        effective: 'debug' as const,
      }),
    );
    let started = false;
    const readSessions = vi.fn(() =>
      Promise.resolve({
        items: started
          ? [
              {
                id: 's-1',
                scope: 'network.wireguard',
                target: 'office',
                actorId: 'user-1',
                minutes: 15,
                startedAt: new Date(),
                expiresAt: new Date(Date.now() + 900_000),
                state: 'active' as const,
                endedAt: null,
                endReason: '',
                effective: 'debug' as const,
              },
            ]
          : [],
        scopes: ['network', 'network.wireguard', 'system'],
        durations: [5, 15, 60],
      }),
    );
    const stopSession = vi.fn(() =>
      Promise.resolve({
        id: 's-1',
        scope: 'network.wireguard',
        target: 'office',
        actorId: 'user-1',
        minutes: 15,
        startedAt: new Date(),
        expiresAt: new Date(),
        state: 'stopped' as const,
        endedAt: new Date(),
        endReason: 'stopped by the operator',
        effective: 'normal' as const,
      }),
    );
    const saveSettings = vi.fn((settings) => Promise.resolve(settings));
    renderInShell(
      <ActivityPage
        readActivity={() => Promise.resolve(page([]))}
        canExport={false}
        canManageDiagnostics
        readSessions={readSessions}
        startSession={startSession}
        stopSession={stopSession}
        readSettings={() =>
          Promise.resolve({
            retentionDays: { audit: 90, event: 14, debug: 1 },
            quotaBytes: { audit: 268435456, event: 134217728, debug: 67108864 },
          })
        }
        saveSettings={saveSettings}
      />,
      { theme: 'light', language: 'en' },
    );
    expect(await screen.findByText('No debug session has been started.')).toBeVisible();
    const form = screen.getByRole('form', { name: 'Start debug session' });
    fireEvent.change(within(form).getByLabelText('Function'), {
      target: { value: 'network.wireguard' },
    });
    fireEvent.change(within(form).getByLabelText('Target (optional)'), {
      target: { value: 'office' },
    });
    started = true;
    fireEvent.click(within(form).getByRole('button', { name: 'Start debug session' }));
    await waitFor(() =>
      expect(startSession).toHaveBeenCalledWith('network.wireguard', 'office', 15),
    );
    expect(
      await screen.findByRole('button', { name: 'Stop network.wireguard' }),
    ).toBeVisible();
    expect(screen.getByText('Active')).toBeVisible();
    fireEvent.click(screen.getByRole('button', { name: 'Stop network.wireguard' }));
    await waitFor(() => expect(stopSession).toHaveBeenCalledWith('s-1'));

    const policy = screen.getByRole('form', { name: 'Retention policy' });
    fireEvent.change(within(policy).getByLabelText('Audit retention (days)'), {
      target: { value: '30' },
    });
    fireEvent.click(within(policy).getByRole('button', { name: 'Save policy' }));
    await waitFor(() => expect(saveSettings).toHaveBeenCalled());
    expect(saveSettings.mock.calls[0][0]).toMatchObject({
      retentionDays: { audit: 30 },
    });
    expect(await screen.findByText('The policy was saved.')).toBeVisible();
  });
});
