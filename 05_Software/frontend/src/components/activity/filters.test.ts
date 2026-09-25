import { describe, expect, it } from 'vitest';

import {
  activeFilterCount,
  appliedOf,
  draftOf,
  localInputToRfc3339,
  rfc3339ToLocalInput,
  withoutPanelFilters,
} from './filters';

/** RFC 3339 with seconds and a zone, which is all Bench Core will parse. */
const RFC3339 = /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$/;

describe('the activity filter panel', () => {
  // The appliance parses `from` and `to` as RFC 3339 and refuses anything
  // else; a datetime-local control produces neither a zone nor seconds.
  it('converts a local datetime control value to a UTC RFC 3339 instant', () => {
    const converted = localInputToRfc3339('2026-09-04T10:30');

    expect(converted).toMatch(RFC3339);
    // Computed from the same local wall-clock time, so the assertion holds in
    // whatever zone the test host is in.
    expect(converted).toBe(
      new Date(2026, 8, 4, 10, 30, 0).toISOString().slice(0, 19) + 'Z',
    );
  });

  it('sends nothing for an empty or unfinished date rather than a bad filter', () => {
    expect(localInputToRfc3339('')).toBe('');
    expect(localInputToRfc3339('   ')).toBe('');
    expect(localInputToRfc3339('2026-09-')).toBe('');
    expect(localInputToRfc3339('not a date')).toBe('');
  });

  it('renders an RFC 3339 filter back into the local control it came from', () => {
    const local = rfc3339ToLocalInput(new Date(2026, 8, 4, 10, 30, 0).toISOString());

    expect(local).toBe('2026-09-04T10:30');
    expect(rfc3339ToLocalInput('')).toBe('');
    expect(rfc3339ToLocalInput('nonsense')).toBe('');
  });

  it('round-trips a wall-clock value through the appliance format', () => {
    expect(rfc3339ToLocalInput(localInputToRfc3339('2026-01-02T03:04'))).toBe(
      '2026-01-02T03:04',
    );
  });

  it('opens a filter as a draft in local time and applies it in UTC', () => {
    const initial = {
      from: new Date(2026, 8, 4, 10, 30, 0).toISOString(),
      component: 'network.interfaces',
    };

    const draft = draftOf(initial);
    expect(draft.from).toBe('2026-09-04T10:30');
    expect(draft.component).toBe('network.interfaces');

    const applied = appliedOf(draft);
    expect(applied.from).toMatch(RFC3339);
    expect(applied.cursor).toBe('');
  });

  it('counts only the panel fields that are narrowing the listing', () => {
    expect(activeFilterCount({})).toBe(0);
    // The category is a chip beside the title, not one of the panel's fields.
    expect(activeFilterCount({ category: 'audit', cursor: '5', limit: 10 })).toBe(0);
    expect(
      activeFilterCount({ actor: 'user-1', outcome: 'failed', operation: 'op-1' }),
    ).toBe(3);
    expect(activeFilterCount({ actor: '   ' })).toBe(0);
  });

  it('clears every panel field and keeps the selected category', () => {
    const cleared = withoutPanelFilters({
      category: 'audit',
      actor: 'user-1',
      from: '2026-09-04T10:30:00Z',
      operation: 'op-1',
    });

    expect(cleared).toEqual({ category: 'audit', cursor: '' });
    expect(activeFilterCount(cleared)).toBe(0);
  });
});
