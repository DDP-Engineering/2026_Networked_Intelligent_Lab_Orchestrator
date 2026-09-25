import { fireEvent, screen, within } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import type { ActivityEvent } from '../../api/activity';
import { renderInShell } from '../../test/render';
import { ActivityEventTable } from './ActivityEventTable';

function event(overrides: Partial<ActivityEvent> = {}): ActivityEvent {
  return {
    id: 'event-1',
    sequence: 1,
    createdAt: new Date('2026-09-04T10:00:00Z'),
    category: 'audit',
    component: 'network.interfaces',
    action: 'interface.disable',
    outcome: 'applied',
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
    details: { state: 'applied' },
    ...overrides,
  };
}

describe('ActivityEventTable', () => {
  it('offers sorting on data columns and reports the active direction', () => {
    const onSort = vi.fn();
    renderInShell(
      <ActivityEventTable
        events={[event()]}
        sort="time"
        order="desc"
        onSort={onSort}
      />,
      { language: 'en' },
    );

    expect(
      screen.getByRole('button', { name: 'Sort by Time' }).closest('th'),
    ).toHaveAttribute('aria-sort', 'descending');
    fireEvent.click(screen.getByRole('button', { name: 'Sort by Activity' }));
    expect(onSort).toHaveBeenCalledWith('activity');
    expect(screen.queryByRole('button', { name: /Sort by Details/ })).toBeNull();
  });

  // REQ-UX-f30891b4: six columns an operator can scan, with the action leading
  // its cell and the function beneath it rather than in a column of its own.
  it('gives the record six semantic columns and the action the prominent place', () => {
    renderInShell(<ActivityEventTable events={[event()]} />, { language: 'en' });

    for (const column of [
      'Time',
      'Activity',
      'Outcome',
      'Actor',
      'Target',
      'Details',
    ]) {
      expect(screen.getByRole('columnheader', { name: column })).toBeVisible();
    }
    const row = screen.getAllByRole('row')[1];
    expect(within(row).getByText('interface.disable')).toBeVisible();
    expect(within(row).getByText('network.interfaces')).toBeVisible();
    expect(within(row).getByText('Applied')).toBeVisible();
    expect(within(row).getByText('User')).toBeVisible();
    expect(within(row).getByText('user-1')).toBeVisible();
    expect(within(row).getByText('enp1s0')).toBeVisible();
    expect(within(row).getByText('interface · sim-enp1s0')).toBeVisible();
  });

  it('labels every cell so a compact record reads without its header row', () => {
    const view = renderInShell(<ActivityEventTable events={[event()]} />, {
      language: 'en',
    });

    const labels = [...view.container.querySelectorAll('tbody td[data-label]')].map(
      (cell) => cell.getAttribute('data-label'),
    );
    expect(labels).toEqual(['Time', 'Activity', 'Outcome', 'Actor', 'Target']);
  });

  it('shows an actor kind it does not know as written, never as a user', () => {
    const view = renderInShell(
      <ActivityEventTable
        events={[event({ actorKind: 'service', actorId: 'svc-1' })]}
      />,
      { language: 'en' },
    );
    expect(screen.getByText('service')).toBeVisible();
    expect(screen.queryByText('User')).toBeNull();
    view.unmount();

    renderInShell(
      <ActivityEventTable events={[event({ actorKind: '', actorId: '' })]} />,
      {
        language: 'en',
      },
    );
    const row = screen.getAllByRole('row')[1];
    expect(within(row).getByText('Not recorded')).toBeVisible();
    expect(within(row).queryByText('User')).toBeNull();
  });

  it('falls back to the subject when no target label was recorded', () => {
    renderInShell(
      <ActivityEventTable events={[event({ targetLabel: '', subjectId: 'sim-x' })]} />,
      { language: 'en' },
    );

    expect(screen.getByText('sim-x')).toBeVisible();
    expect(screen.getByText('interface')).toBeVisible();
  });

  // REQ-UX-2de87216: the record opens in a dialog rather than an inline row of
  // raw JSON, and the keyboard is never left on a control that has gone away.
  it('opens the record in a dialog and returns focus to the control that opened it', () => {
    renderInShell(<ActivityEventTable events={[event()]} />, { language: 'en' });

    const details = screen.getByRole('button', { name: 'Details for entry 1' });
    details.focus();
    fireEvent.click(details);

    const dialog = screen.getByRole('dialog', { name: 'Activity entry 1' });
    expect(dialog).toBeVisible();
    expect(dialog.contains(document.activeElement)).toBe(true);

    fireEvent.keyDown(document, { key: 'Escape' });
    expect(screen.queryByRole('dialog')).toBeNull();
    expect(document.activeElement).toBe(details);
  });

  it('states every recorded envelope field, and says which were not recorded', () => {
    renderInShell(
      <ActivityEventTable events={[event({ targetLabel: '', operationId: '' })]} />,
      { language: 'en' },
    );
    fireEvent.click(screen.getByRole('button', { name: 'Details for entry 1' }));
    const dialog = screen.getByRole('dialog');

    for (const label of [
      'Event ID',
      'Sequence',
      'Recorded at',
      'Category',
      'Function',
      'Action',
      'Outcome',
      'Severity',
      'Actor kind',
      'Actor ID',
      'Subject type',
      'Subject ID',
      'Target label',
      'Operation ID',
      'Appliance ID',
      'Boot ID',
      'Time source',
    ]) {
      expect(within(dialog).getByText(label)).toBeVisible();
    }
    expect(within(dialog).getByText('appliance-1')).toBeVisible();
    expect(within(dialog).getByText('boot-1')).toBeVisible();
    expect(within(dialog).getByText('Unsynchronized clock')).toBeVisible();
    // The readable label leads; the appliance's own code stays beside it.
    expect(within(dialog).getByText('unsynchronized')).toBeVisible();
    // Two fields carry nothing, and the dialog says so rather than guessing.
    expect(within(dialog).getAllByText('Not recorded').length).toBe(2);
  });

  it('shows a recorded change as separate Before and After sections', () => {
    renderInShell(
      <ActivityEventTable
        events={[
          event({
            details: {
              before: { enabled: true, address: '10.0.0.1' },
              after: { enabled: false, address: '10.0.0.1' },
              reason: 'operator request',
            },
          }),
        ]}
      />,
      { language: 'en' },
    );
    fireEvent.click(screen.getByRole('button', { name: 'Details for entry 1' }));
    const dialog = screen.getByRole('dialog');

    const before = within(dialog).getByRole('heading', { name: 'Before' });
    const after = within(dialog).getByRole('heading', { name: 'After' });
    expect(before).toBeVisible();
    expect(after).toBeVisible();
    expect(before.parentElement).toHaveTextContent('Yes');
    expect(after.parentElement).toHaveTextContent('No');
    // Anything beside the comparison is still shown, in its own section.
    expect(
      within(dialog).getByRole('heading', { name: 'Recorded details' }),
    ).toBeVisible();
    expect(within(dialog).getByText('operator request')).toBeVisible();
  });

  it('shows structured details as labelled values when there is no comparison', () => {
    renderInShell(
      <ActivityEventTable
        events={[event({ details: { scope: 'network', peers: ['office', 'lab'] } })]}
      />,
      { language: 'en' },
    );
    fireEvent.click(screen.getByRole('button', { name: 'Details for entry 1' }));
    const dialog = screen.getByRole('dialog');

    expect(within(dialog).queryByRole('heading', { name: 'Before' })).toBeNull();
    expect(within(dialog).getByText('scope')).toBeVisible();
    expect(within(dialog).getByText('network')).toBeVisible();
    expect(within(dialog).getByText('office')).toBeVisible();
    expect(within(dialog).getByText('lab')).toBeVisible();
  });

  it('says plainly when the appliance recorded no further detail', () => {
    renderInShell(<ActivityEventTable events={[event({ details: {} })]} />, {
      language: 'en',
    });
    fireEvent.click(screen.getByRole('button', { name: 'Details for entry 1' }));

    expect(screen.getByText('The appliance recorded no further detail.')).toBeVisible();
  });

  it('narrows to the operation from the dialog and closes it', () => {
    const onShowOperation = vi.fn();
    renderInShell(
      <ActivityEventTable events={[event()]} onShowOperation={onShowOperation} />,
      { language: 'en' },
    );
    fireEvent.click(screen.getByRole('button', { name: 'Details for entry 1' }));
    fireEvent.click(
      screen.getByRole('button', { name: 'Show every stage of operation op-1' }),
    );

    expect(onShowOperation).toHaveBeenCalledWith('op-1');
    expect(screen.queryByRole('dialog')).toBeNull();
  });

  it('offers no operation control where the screen has no listing to narrow', () => {
    renderInShell(<ActivityEventTable events={[event()]} />, { language: 'en' });
    fireEvent.click(screen.getByRole('button', { name: 'Details for entry 1' }));

    expect(
      screen.queryByRole('button', { name: /Show every stage of operation/ }),
    ).toBeNull();
  });

  it('reads the listing in German and in Romanian', () => {
    const german = renderInShell(<ActivityEventTable events={[event()]} />, {
      language: 'de',
    });
    expect(screen.getByRole('columnheader', { name: 'Vorgang' })).toBeVisible();
    expect(screen.getByText('Angewendet')).toBeVisible();
    expect(screen.getByRole('button', { name: 'Details zu Eintrag 1' })).toBeVisible();
    german.unmount();

    renderInShell(<ActivityEventTable events={[event()]} />, { language: 'ro' });
    expect(screen.getByRole('columnheader', { name: 'Activitate' })).toBeVisible();
    expect(screen.getByText('Aplicat')).toBeVisible();
    expect(
      screen.getByRole('button', { name: 'Detalii pentru intrarea 1' }),
    ).toBeVisible();
  });
});
