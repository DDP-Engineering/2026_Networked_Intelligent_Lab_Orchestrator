import { fireEvent, screen, waitFor, within } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';

import type {
  ActivityEvent,
  ActivityFilter,
  ActivityPage as ActivityPageBody,
} from '../../api/activity';
import {
  installViewportClass,
  renderInShell,
  restoreMatchMedia,
} from '../../test/render';
import { ActivityPage } from './ActivityPage';

function page(
  items: ActivityPageBody['items'],
  nextCursor = '',
  overrides: Partial<ActivityPageBody> = {},
): ActivityPageBody {
  return {
    items,
    nextCursor,
    total: items.length,
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
    },
    ...overrides,
  };
}

const event = (sequence: number, outcome: string): ActivityEvent => ({
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

/** The two appliance calls the page makes, so a mock keeps its argument types. */
type ReadActivity = (filter: ActivityFilter) => Promise<ActivityPageBody>;
type ExportActivity = (
  filter: ActivityFilter,
  format: 'jsonl' | 'csv',
) => Promise<string>;

/** Open the disclosure the less frequent filters live behind. */
function openMoreFilters(): void {
  fireEvent.click(screen.getByText('More filters'));
}

describe('ActivityPage', () => {
  afterEach(() => restoreMatchMedia());

  // REQ-UX-1c5eab92 and REQ-FUNC-d4f81a26: the page opens narrowed, sends its
  // filters to the appliance, pages on the server, and shows the retained window
  // and the gap retention left.
  it('opens with the given filters, changes pages and discloses the gap', async () => {
    const readActivity = vi.fn((filter: ActivityFilter) =>
      Promise.resolve(
        filter.page === 2
          ? page([event(3, 'confirmed')], '', { total: 26 })
          : page([event(1, 'requested'), event(2, 'applied')], '', { total: 26 }),
      ),
    );
    renderInShell(
      <ActivityPage
        initialFilter={{ component: 'network.interfaces', target: 'sim-enp1s0' }}
        readActivity={readActivity}
        canExport={false}
        canManageRetention={false}
      />,
      { theme: 'light', language: 'en' },
    );
    expect(await screen.findByText('Requested')).toBeVisible();
    expect(readActivity.mock.calls[0][0]).toMatchObject({
      component: 'network.interfaces',
      target: 'sim-enp1s0',
    });
    // A filter behind the disclosure is in effect, so the disclosure is open.
    expect(screen.getByLabelText('Component')).toBeVisible();
    expect(screen.getByLabelText('Component')).toHaveValue('network.interfaces');
    expect(screen.getByText(/Entries before .* were removed \(age\)/)).toBeVisible();

    fireEvent.click(screen.getByRole('button', { name: 'Next' }));
    expect(await screen.findByText('Confirmed')).toBeVisible();
    expect(readActivity.mock.calls[1][0]).toMatchObject({ page: 2, limit: 10 });
    expect(screen.queryByText('Applied')).toBeNull();
    expect(screen.getAllByRole('row')).toHaveLength(2);
  });

  // REQ-UX-f30891b4: the category is scope rather than a filter to apply, so a
  // chip re-queries the appliance on its own.
  it('reloads the listing as soon as a category chip is chosen', async () => {
    const readActivity = vi.fn<ReadActivity>(() =>
      Promise.resolve(page([event(1, 'applied')])),
    );
    renderInShell(
      <ActivityPage
        readActivity={readActivity}
        canExport={false}
        canManageRetention={false}
      />,
      { theme: 'light', language: 'en' },
    );
    await screen.findByText('Applied');

    const chips = screen.getByRole('group', { name: 'Category' });
    expect(within(chips).getByRole('button', { name: 'All' })).toHaveAttribute(
      'aria-pressed',
      'true',
    );

    // Typed but not applied, so the chip must not carry it to the appliance.
    fireEvent.change(screen.getByLabelText('Actor'), { target: { value: 'user-1' } });
    fireEvent.click(within(chips).getByRole('button', { name: 'Events' }));
    await waitFor(() =>
      expect(
        readActivity.mock.calls[readActivity.mock.calls.length - 1][0],
      ).toMatchObject({ category: 'event' }),
    );
    const sent = readActivity.mock.calls[readActivity.mock.calls.length - 1][0];
    expect(sent.actor ?? '').toBe('');
    expect(screen.getByLabelText('Actor')).toHaveValue('user-1');
    expect(within(chips).getByRole('button', { name: 'Events' })).toHaveAttribute(
      'aria-pressed',
      'true',
    );
    expect(within(chips).getByRole('button', { name: 'All' })).toHaveAttribute(
      'aria-pressed',
      'false',
    );
  });

  // The appliance parses `from` and `to` as RFC 3339 and refuses anything the
  // datetime-local control produces on its own.
  it('sends the local From and To as UTC RFC 3339 and reports the active count', async () => {
    const readActivity = vi.fn<ReadActivity>(() =>
      Promise.resolve(page([event(1, 'applied')])),
    );
    renderInShell(
      <ActivityPage
        readActivity={readActivity}
        canExport={false}
        canManageRetention={false}
      />,
      { theme: 'light', language: 'en' },
    );
    await screen.findByText('Applied');
    expect(screen.getByText('Filters in effect: none')).toBeVisible();

    fireEvent.change(screen.getByLabelText('From'), {
      target: { value: '2026-09-04T10:30' },
    });
    fireEvent.change(screen.getByLabelText('To'), {
      target: { value: '2026-09-04T12:00' },
    });
    fireEvent.change(screen.getByLabelText('Actor'), { target: { value: 'user-1' } });
    fireEvent.click(screen.getByRole('button', { name: 'Apply filters' }));

    await waitFor(() => expect(readActivity).toHaveBeenCalledTimes(2));
    const sent = readActivity.mock.calls[1][0] as ActivityFilter;
    expect(sent.from).toBe(
      new Date(2026, 8, 4, 10, 30, 0).toISOString().slice(0, 19) + 'Z',
    );
    expect(sent.to).toBe(
      new Date(2026, 8, 4, 12, 0, 0).toISOString().slice(0, 19) + 'Z',
    );
    expect(sent.from).toMatch(/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$/);
    expect(sent.actor).toBe('user-1');
    expect(await screen.findByText('Filters in effect: 3')).toBeVisible();

    // Clearing empties the panel, keeps the category, and re-queries at once.
    fireEvent.click(screen.getByRole('button', { name: 'Clear filters' }));
    expect(await screen.findByText('Filters in effect: none')).toBeVisible();
    expect(screen.getByLabelText('From')).toHaveValue('');
    expect(screen.getByLabelText('Actor')).toHaveValue('');
    await waitFor(() => expect(readActivity).toHaveBeenCalledTimes(3));
    const reset = readActivity.mock.calls[2][0] as ActivityFilter;
    expect(reset.from ?? '').toBe('');
    expect(reset.to ?? '').toBe('');
    expect(reset.actor ?? '').toBe('');
  });

  it('keeps the less frequent filters behind a disclosure and still sends them', async () => {
    const readActivity = vi.fn<ReadActivity>(() =>
      Promise.resolve(page([event(1, 'applied')])),
    );
    renderInShell(
      <ActivityPage
        readActivity={readActivity}
        canExport={false}
        canManageRetention={false}
      />,
      { theme: 'light', language: 'en' },
    );
    await screen.findByText('Applied');

    expect(screen.getByLabelText('Severity')).not.toBeVisible();
    openMoreFilters();
    expect(screen.getByLabelText('Severity')).toBeVisible();
    expect(screen.getByLabelText('Operation ID')).toBeVisible();

    fireEvent.change(screen.getByLabelText('Severity'), {
      target: { value: 'warning' },
    });
    fireEvent.click(screen.getByRole('button', { name: 'Apply filters' }));
    await waitFor(() =>
      expect(readActivity.mock.calls[1][0]).toMatchObject({ severity: 'warning' }),
    );
    expect(await screen.findByText('Filters in effect: 1')).toBeVisible();
  });

  it('narrows to every stage of an operation from the details dialog', async () => {
    const readActivity = vi.fn<ReadActivity>(() =>
      Promise.resolve(page([event(1, 'requested')])),
    );
    renderInShell(
      <ActivityPage
        readActivity={readActivity}
        canExport={false}
        canManageRetention={false}
      />,
      { theme: 'light', language: 'en' },
    );
    await screen.findByText('Requested');

    fireEvent.click(screen.getByRole('button', { name: 'Details for entry 1' }));
    const dialog = screen.getByRole('dialog', { name: 'Activity entry 1' });
    expect(within(dialog).getByText('Audit')).toBeVisible();
    expect(within(dialog).getByText('network.interfaces')).toBeVisible();
    fireEvent.click(
      within(dialog).getByRole('button', {
        name: 'Show every stage of operation op-1',
      }),
    );

    await waitFor(() =>
      expect(
        readActivity.mock.calls[readActivity.mock.calls.length - 1][0],
      ).toMatchObject({ operation: 'op-1' }),
    );
    // Every stage, so the category goes back to All: the stages of one change
    // are not all audit records.
    expect(
      readActivity.mock.calls[readActivity.mock.calls.length - 1][0].category ?? '',
    ).toBe('');
    expect(
      within(screen.getByRole('group', { name: 'Category' })).getByRole('button', {
        name: 'All',
      }),
    ).toHaveAttribute('aria-pressed', 'true');
    expect(screen.queryByRole('dialog')).toBeNull();
    // The one filter in effect is never hidden behind the disclosure.
    expect(screen.getByText('Filters in effect: 1')).toBeVisible();
    expect(screen.getByLabelText('Operation ID')).toBeVisible();
    expect(screen.getByLabelText('Operation ID')).toHaveValue('op-1');
  });

  // Rows read under one filter must never stand beneath the count and the
  // export of another, however briefly.
  it('takes the previous page down before a new filter is answered', async () => {
    let release: (value: ActivityPageBody) => void = () => {};
    const readActivity = vi.fn<ReadActivity>((filter) =>
      filter.category === 'event'
        ? new Promise<ActivityPageBody>((resolve) => {
            release = resolve;
          })
        : Promise.resolve(page([event(1, 'applied')])),
    );
    renderInShell(
      <ActivityPage
        readActivity={readActivity}
        canExport={false}
        canManageRetention={false}
      />,
      { theme: 'light', language: 'en' },
    );
    expect(await screen.findByText('Applied')).toBeVisible();

    fireEvent.click(
      within(screen.getByRole('group', { name: 'Category' })).getByRole('button', {
        name: 'Events',
      }),
    );
    expect(await screen.findByText('Reading the activity history…')).toBeVisible();
    expect(screen.queryByText('Applied')).toBeNull();

    release(page([event(2, 'denied')]));
    expect(await screen.findByText('Denied')).toBeVisible();
    expect(screen.queryByText('Applied')).toBeNull();
  });

  // REQ-FUNC-d4f81a26: an export contains what the listing contains, which is
  // the applied filter and never whatever has been typed since.
  it('exports the applied filter rather than the unapplied draft', async () => {
    const exportActivity = vi.fn<ExportActivity>(() =>
      Promise.resolve('{"a":1}\n{"b":2}\n{"c":3}\n'),
    );
    renderInShell(
      <ActivityPage
        readActivity={() => Promise.resolve(page([event(1, 'applied')]))}
        exportActivity={exportActivity}
        canExport
        canManageRetention={false}
      />,
      { theme: 'light', language: 'en' },
    );
    await screen.findByText('Applied');

    fireEvent.change(screen.getByLabelText('Actor'), { target: { value: 'user-1' } });
    fireEvent.click(screen.getByRole('button', { name: 'Apply filters' }));
    // Typed after the listing was narrowed, so it is not part of the export.
    fireEvent.change(screen.getByLabelText('Target'), { target: { value: 'draft' } });

    fireEvent.click(screen.getByRole('button', { name: 'Export' }));
    expect(
      screen.getByText('The export contains exactly what these filters show.'),
    ).toBeVisible();
    fireEvent.click(screen.getByRole('menuitem', { name: 'Export CSV' }));

    await waitFor(() => expect(exportActivity).toHaveBeenCalled());
    expect(exportActivity.mock.calls[0][0]).toMatchObject({ actor: 'user-1' });
    expect(exportActivity.mock.calls[0][0]).not.toMatchObject({ target: 'draft' });
    expect(exportActivity.mock.calls[0][1]).toBe('csv');
  });

  it('offers no export at all without the permission', async () => {
    renderInShell(
      <ActivityPage
        readActivity={() => Promise.resolve(page([event(1, 'applied')]))}
        exportActivity={() => Promise.resolve('')}
        canExport={false}
        canManageRetention={false}
      />,
      { theme: 'light', language: 'en' },
    );
    await screen.findByText('Applied');

    expect(screen.queryByRole('button', { name: 'Export' })).toBeNull();
  });

  it('states a degraded audit persistence as a refusal condition', async () => {
    renderInShell(
      <ActivityPage
        readActivity={() =>
          Promise.resolve(
            page([event(1, 'applied')], '', {
              loggingHealth: {
                auditWritable: false,
                failures: 3,
                lastFailure: 'the audit database is read-only',
              },
            }),
          )
        }
        canExport={false}
        canManageRetention={false}
      />,
      { theme: 'light', language: 'en' },
    );

    expect(
      await screen.findByText(/cannot record its activity: the audit database/),
    ).toBeVisible();
    expect(screen.queryByText(/is recording its activity/)).toBeNull();
  });

  it('reports when another page cannot be read', async () => {
    let first = true;
    const readActivity = vi.fn<ReadActivity>(() => {
      if (first) {
        first = false;
        return Promise.resolve(page([event(1, 'applied')], '', { total: 26 }));
      }
      return Promise.reject(new Error('unreachable'));
    });
    renderInShell(
      <ActivityPage
        readActivity={readActivity}
        canExport={false}
        canManageRetention={false}
      />,
      { theme: 'light', language: 'en' },
    );
    await screen.findByText('Applied');

    fireEvent.click(screen.getByRole('button', { name: 'Next' }));
    expect(
      await screen.findByText('The activity history could not be read.'),
    ).toBeVisible();
  });

  it('loads a selected page once while the request is running', async () => {
    let release: (value: ActivityPageBody) => void = () => {};
    const readActivity = vi.fn((filter: ActivityFilter) =>
      filter.page === 1
        ? Promise.resolve(page([event(1, 'applied')], '', { total: 26 }))
        : new Promise<ActivityPageBody>((resolve) => {
            release = resolve;
          }),
    );
    renderInShell(
      <ActivityPage
        readActivity={readActivity}
        canExport={false}
        canManageRetention={false}
      />,
      { theme: 'light', language: 'en' },
    );
    await screen.findByText('Applied');

    fireEvent.click(screen.getByRole('button', { name: 'Next' }));
    expect(await screen.findByText('Reading the activity history…')).toBeVisible();
    expect(readActivity).toHaveBeenCalledTimes(2);

    release(page([event(2, 'confirmed')]));
    expect(await screen.findByText('Confirmed')).toBeVisible();
  });

  it('forgets a page still in flight when the filter changes', async () => {
    let release: (value: ActivityPageBody) => void = () => {};
    const readActivity = vi.fn<ReadActivity>((filter) =>
      filter.page === 2
        ? new Promise<ActivityPageBody>((resolve) => {
            release = resolve;
          })
        : Promise.resolve(
            page([event(filter.category === 'event' ? 5 : 1, 'applied')], '', {
              total: 26,
            }),
          ),
    );
    renderInShell(
      <ActivityPage
        readActivity={readActivity}
        canExport={false}
        canManageRetention={false}
      />,
      { theme: 'light', language: 'en' },
    );
    await screen.findByText('Applied');
    fireEvent.click(screen.getByRole('button', { name: 'Next' }));
    expect(await screen.findByText('Reading the activity history…')).toBeVisible();

    // The filter moves on while that page is still outstanding.
    fireEvent.click(
      within(screen.getByRole('group', { name: 'Category' })).getByRole('button', {
        name: 'Events',
      }),
    );
    expect(await screen.findByRole('button', { name: 'Next' })).toBeEnabled();

    // The stale page arrives late and is disowned rather than appended.
    release(page([event(2, 'denied')]));
    await waitFor(() => expect(readActivity).toHaveBeenCalledTimes(3));
    expect(screen.queryByText('Denied')).toBeNull();
    expect(screen.getByRole('button', { name: 'Next' })).toBeEnabled();
    expect(screen.getAllByRole('row')).toHaveLength(2);
  });

  // On a field display the records come first: the filter panel is a drawer
  // whose summary carries the count, so it can be closed without being lost.
  it('keeps the filter panel in a closed drawer on a compact display', async () => {
    installViewportClass('compact');
    renderInShell(
      <ActivityPage
        initialFilter={{ actor: 'user-1' }}
        readActivity={() => Promise.resolve(page([event(1, 'applied')]))}
        canExport={false}
        canManageRetention={false}
      />,
      { theme: 'light', language: 'en' },
    );
    expect(await screen.findByText('Applied')).toBeVisible();

    const form = screen.getByRole('form', { name: 'Filters' });
    expect(form).not.toBeVisible();
    const summary = screen.getByText(/^Filters/, { selector: 'summary' });
    expect(summary).toBeVisible();
    expect(summary).toHaveTextContent('Filters in effect: 1');

    fireEvent.click(summary);
    expect(form).toBeVisible();
    expect(screen.getByLabelText('Actor')).toHaveValue('user-1');
  });

  it('shows the filter panel plainly on a wide display', async () => {
    installViewportClass('wide');
    renderInShell(
      <ActivityPage
        readActivity={() => Promise.resolve(page([event(1, 'applied')]))}
        canExport={false}
        canManageRetention={false}
      />,
      { theme: 'light', language: 'en' },
    );
    expect(await screen.findByText('Applied')).toBeVisible();

    expect(screen.getByRole('form', { name: 'Filters' })).toBeVisible();
    expect(screen.queryByText(/^Filters/, { selector: 'summary' })).toBeNull();
  });

  it('sorts the complete server listing from its column headers', async () => {
    const readActivity = vi.fn<ReadActivity>(() =>
      Promise.resolve(page([event(1, 'applied')], 'sorted-cursor', { total: 75 })),
    );
    renderInShell(
      <ActivityPage
        readActivity={readActivity}
        canExport={false}
        canManageRetention={false}
      />,
      { theme: 'light', language: 'en' },
    );
    await screen.findByText('Applied');

    expect(readActivity.mock.calls[0][0]).toMatchObject({
      sort: 'time',
      order: 'desc',
      cursor: '',
    });
    fireEvent.click(screen.getByRole('button', { name: 'Sort by Activity' }));
    await waitFor(() =>
      expect(readActivity.mock.calls.at(-1)?.[0]).toMatchObject({
        sort: 'activity',
        order: 'asc',
        cursor: '',
      }),
    );
    fireEvent.click(screen.getByRole('button', { name: 'Sort by Activity' }));
    await waitFor(() =>
      expect(readActivity.mock.calls.at(-1)?.[0]).toMatchObject({
        sort: 'activity',
        order: 'desc',
        cursor: '',
      }),
    );

    fireEvent.click(screen.getByRole('button', { name: 'Next' }));
    await waitFor(() =>
      expect(readActivity.mock.calls.at(-1)?.[0]).toMatchObject({
        sort: 'activity',
        order: 'desc',
        page: 2,
        cursor: '',
      }),
    );
  });

  it('selects component families, page sizes and numbered pages', async () => {
    const readActivity = vi.fn<ReadActivity>(() =>
      Promise.resolve(page([event(1, 'applied')], '', { total: 63 })),
    );
    renderInShell(
      <ActivityPage
        readActivity={readActivity}
        canExport={false}
        canManageRetention={false}
      />,
      { theme: 'light', language: 'en' },
    );
    await screen.findByText('Applied');

    // REQ-UX-7781cc4e: ten records is the page the listing opens at, and the
    // smallest the control offers.
    expect(readActivity.mock.calls[0][0]).toMatchObject({ limit: 10 });
    const sizes = screen.getByLabelText('Rows per page');
    expect(sizes).toHaveValue('10');
    expect(
      within(sizes)
        .getAllByRole('option')
        .map((option) => option.textContent),
    ).toEqual(['10', '25', '50', '100', 'All records']);

    fireEvent.click(screen.getByText('More filters'));
    fireEvent.change(screen.getByLabelText('Component'), {
      target: { value: 'network.*' },
    });
    fireEvent.click(screen.getByRole('button', { name: 'Apply filters' }));
    await waitFor(() =>
      expect(readActivity.mock.calls.at(-1)?.[0]).toMatchObject({
        component: 'network.*',
        page: 1,
      }),
    );

    fireEvent.change(screen.getByLabelText('Rows per page'), {
      target: { value: '50' },
    });
    await waitFor(() =>
      expect(readActivity.mock.calls.at(-1)?.[0]).toMatchObject({
        limit: 50,
        page: 1,
      }),
    );
    fireEvent.click(screen.getByRole('button', { name: 'Select page 2' }));
    await waitFor(() =>
      expect(readActivity.mock.calls.at(-1)?.[0]).toMatchObject({ page: 2 }),
    );

    fireEvent.change(screen.getByLabelText('Rows per page'), {
      target: { value: 'all' },
    });
    await waitFor(() =>
      expect(readActivity.mock.calls.at(-1)?.[0]).toMatchObject({
        limit: 'all',
        page: 1,
      }),
    );
  });

  it('reports the failure and the empty listing without losing the page', async () => {
    const unreadable = renderInShell(
      <ActivityPage
        readActivity={() => Promise.reject(new Error('unreachable'))}
        canExport={false}
        canManageRetention={false}
      />,
      { theme: 'light', language: 'en' },
    );
    expect(
      await screen.findByText('The activity history could not be read.'),
    ).toBeVisible();
    unreadable.unmount();

    renderInShell(
      <ActivityPage
        readActivity={() => Promise.resolve(page([]))}
        canExport={false}
        canManageRetention={false}
      />,
      { theme: 'light', language: 'en' },
    );
    expect(await screen.findByText('No entries match these filters.')).toBeVisible();
  });

  // REQ-UX-0291ba55: retention is the one control that decides how long the
  // rows are kept, so it sits above the filters rather than past every row -
  // collapsed, and only for an identity that may manage it.
  it('keeps retention as the only administration, above the filters', async () => {
    const saveRetention = vi.fn((settings) => Promise.resolve(settings));
    const { container } = renderInShell(
      <ActivityPage
        readActivity={() => Promise.resolve(page([]))}
        canExport={false}
        canManageRetention
        readRetention={() =>
          Promise.resolve({
            retentionDays: { audit: 90, event: 14 },
            quotaBytes: { audit: 268435456, event: 134217728 },
          })
        }
        saveRetention={saveRetention}
      />,
      { theme: 'light', language: 'en' },
    );

    expect(await screen.findByText('Activity retention')).toBeVisible();
    const retention = container.querySelector('details.activity-retention');
    const filters = screen.getByRole('form', { name: 'Filters' });
    expect(retention).not.toBeNull();
    expect(retention).not.toHaveAttribute('open');
    const order = (retention as Element).compareDocumentPosition(filters);
    expect(order & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(screen.queryByText('Temporary debug logging')).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Debug' })).not.toBeInTheDocument();
    expect(screen.queryByLabelText('Debug retention (days)')).not.toBeInTheDocument();
    expect(screen.queryByLabelText('Debug quota (MiB)')).not.toBeInTheDocument();
    fireEvent.click(screen.getByText('Activity retention', { selector: 'summary' }));
    const policy = screen.getByRole('form', { name: 'Retention policy' });
    fireEvent.change(within(policy).getByLabelText('Audit retention (days)'), {
      target: { value: '30' },
    });
    fireEvent.click(within(policy).getByRole('button', { name: 'Save policy' }));
    await waitFor(() => expect(saveRetention).toHaveBeenCalled());
    expect(saveRetention.mock.calls[0][0]).toMatchObject({
      retentionDays: { audit: 30 },
    });
    expect(await screen.findByText('The policy was saved.')).toBeVisible();
  });

  // REQ-UX-3e01a5b6: the header is the title and, for an exporter, Export -
  // and nothing else. The page is reached by its own tab; a sentence saying so
  // costs a line of the first screen.
  it('carries the title and Export on one header line, and no description', async () => {
    const exported = renderInShell(
      <ActivityPage
        readActivity={() => Promise.resolve(page([event(1, 'applied')]))}
        exportActivity={() => Promise.resolve('')}
        canExport
        canManageRetention={false}
      />,
      { theme: 'light', language: 'en' },
    );
    await screen.findByText('Applied');

    const header = exported.container.querySelector('header.page-head');
    expect(header).not.toBeNull();
    const head = within(header as HTMLElement);
    expect(head.getByRole('heading', { name: 'Activity' })).toBeVisible();
    expect(head.getByRole('button', { name: 'Export' })).toBeVisible();
    expect(header?.querySelectorAll('p')).toHaveLength(0);
    expect(
      screen.queryByText(
        'Every change and event the appliance recorded, newest first.',
      ),
    ).toBeNull();
    exported.unmount();

    const plain = renderInShell(
      <ActivityPage
        readActivity={() => Promise.resolve(page([event(1, 'applied')]))}
        canExport={false}
        canManageRetention={false}
      />,
      { theme: 'light', language: 'en' },
    );
    await screen.findByText('Applied');
    const bare = plain.container.querySelector('header.page-head');
    expect(
      within(bare as HTMLElement).getByRole('heading', { name: 'Activity' }),
    ).toBeVisible();
    expect(within(bare as HTMLElement).queryByRole('button')).toBeNull();
  });

  // REQ-UX-c5976f3b: the strip above the listing carries conditions only. A
  // healthy appliance with nothing purged says nothing there at all.
  it('says nothing above the listing while recording is healthy', async () => {
    const { container } = renderInShell(
      <ActivityPage
        readActivity={() =>
          Promise.resolve(
            page([event(1, 'applied')], '', {
              retainedWindow: {
                audit: {
                  oldest: new Date('2026-09-01T00:00:00Z'),
                  newest: new Date('2026-09-04T10:00:00Z'),
                  count: 1,
                  truncated: false,
                  purgedBefore: null,
                  purgeReason: '',
                },
              },
            }),
          )
        }
        canExport={false}
        canManageRetention={false}
      />,
      { theme: 'light', language: 'en' },
    );
    await screen.findByText('Applied');

    expect(screen.queryByText(/is recording its activity/)).toBeNull();
    expect(screen.queryByText(/entries retained/)).toBeNull();
    expect(container.querySelector('.activity-banner')).toBeNull();
  });

  // REQ-UX-f8c58c4c: on a wide display scope, fields and More filters are one
  // line, which means one container - the chips are part of the filter panel.
  it('puts the scope chips on the filter line on a wide display', async () => {
    installViewportClass('wide');
    renderInShell(
      <ActivityPage
        readActivity={() => Promise.resolve(page([event(1, 'applied')]))}
        canExport={false}
        canManageRetention={false}
      />,
      { theme: 'light', language: 'en' },
    );
    await screen.findByText('Applied');

    const filters = screen.getByRole('form', { name: 'Filters' });
    const chips = screen.getByRole('group', { name: 'Category' });
    expect(filters).toContainElement(chips);
    expect(filters).toContainElement(screen.getByText('More filters'));
    expect(filters).toContainElement(screen.getByLabelText('Actor'));
  });

  // REQ-UX-ba46f6d3: where the panel collapses, scope does not go with it.
  // 1024x600 is the bench panel, and scope is the filter always in play.
  it('keeps the scope chips outside the collapsed filter drawer', async () => {
    installViewportClass('compact');
    const readActivity = vi.fn<ReadActivity>(() =>
      Promise.resolve(page([event(1, 'applied')])),
    );
    renderInShell(
      <ActivityPage
        readActivity={readActivity}
        canExport={false}
        canManageRetention={false}
      />,
      { theme: 'light', language: 'en' },
    );
    await screen.findByText('Applied');

    const filters = screen.getByRole('form', { name: 'Filters' });
    expect(filters).not.toBeVisible();
    const chips = screen.getByRole('group', { name: 'Category' });
    expect(chips).toBeVisible();
    expect(filters).not.toContainElement(chips);

    fireEvent.click(within(chips).getByRole('button', { name: 'Audit' }));
    await waitFor(() =>
      expect(readActivity.mock.calls.at(-1)?.[0]).toMatchObject({ category: 'audit' }),
    );
    expect(filters).not.toBeVisible();
  });

  it('reads the page head and the filter panel in German and in Romanian', async () => {
    const german = renderInShell(
      <ActivityPage
        readActivity={() => Promise.resolve(page([event(1, 'applied')]))}
        canExport={false}
        canManageRetention={false}
      />,
      { theme: 'light', language: 'de' },
    );
    expect(await screen.findByText('Angewendet')).toBeVisible();
    expect(screen.getByRole('group', { name: 'Kategorie' })).toBeVisible();
    expect(screen.getByRole('form', { name: 'Filter' })).toBeVisible();
    expect(screen.getByText('Weitere Filter')).toBeVisible();
    expect(screen.getByText('Aktive Filter: keine')).toBeVisible();
    german.unmount();

    renderInShell(
      <ActivityPage
        readActivity={() => Promise.resolve(page([event(1, 'applied')]))}
        canExport={false}
        canManageRetention={false}
      />,
      { theme: 'light', language: 'ro' },
    );
    expect(await screen.findByText('Aplicat')).toBeVisible();
    expect(screen.getByRole('group', { name: 'Categorie' })).toBeVisible();
    expect(screen.getByRole('form', { name: 'Filtre' })).toBeVisible();
    expect(screen.getByText('Mai multe filtre')).toBeVisible();
    expect(screen.getByText('Filtre active: niciunul')).toBeVisible();
  });
});
