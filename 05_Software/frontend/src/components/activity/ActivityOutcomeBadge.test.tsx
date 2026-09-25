import { screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import { renderInShell } from '../../test/render';
import { ActivityOutcomeBadge } from './ActivityOutcomeBadge';
import { outcomeTone } from './vocabulary';

describe('ActivityOutcomeBadge', () => {
  // REQ-UX-f30891b4: the outcome is readable without seeing its colour, so the
  // word is always present and the tone only repeats it.
  it('names every outcome the appliance writes and tones it', () => {
    for (const [outcome, label, tone] of [
      ['applied', 'Applied', 'ok'],
      ['confirmed', 'Confirmed', 'ok'],
      ['recovered', 'Recovered', 'ok'],
      ['success', 'Succeeded', 'ok'],
      ['requested', 'Requested', 'pending'],
      ['pending', 'Pending', 'pending'],
      ['denied', 'Denied', 'warn'],
      ['refused', 'Refused', 'warn'],
      ['failed', 'Failed', 'danger'],
      ['error', 'Error', 'danger'],
    ] as const) {
      const view = renderInShell(<ActivityOutcomeBadge outcome={outcome} />, {
        language: 'en',
      });
      const badge = view.container.querySelector('.status-badge');

      expect(badge).not.toBeNull();
      expect(badge).toHaveTextContent(label);
      expect(badge?.className).toContain(`status-badge-${tone}`);
      expect(outcomeTone(outcome)).toBe(tone);
      view.unmount();
    }
  });

  it('shows an unrecognised outcome as written rather than inventing a state', () => {
    const view = renderInShell(<ActivityOutcomeBadge outcome="quarantined" />, {
      language: 'en',
    });

    const badge = view.container.querySelector('.status-badge');
    expect(badge).toHaveTextContent('quarantined');
    expect(badge?.className).toContain('status-badge-unknown');
  });

  it('says so when no outcome was recorded at all', () => {
    renderInShell(<ActivityOutcomeBadge outcome="" />, { language: 'en' });

    expect(screen.getByText('Not recorded')).toBeVisible();
  });

  it('adds the severity beside the outcome only when it is not the ordinary one', () => {
    const ordinary = renderInShell(
      <ActivityOutcomeBadge outcome="applied" severity="info" />,
      { language: 'en' },
    );
    expect(screen.queryByText('Information')).toBeNull();
    ordinary.unmount();

    renderInShell(<ActivityOutcomeBadge outcome="failed" severity="warning" />, {
      language: 'en',
    });
    expect(screen.getByText('Warning')).toBeVisible();
  });

  it('reads the outcome in the selected language', () => {
    const german = renderInShell(<ActivityOutcomeBadge outcome="denied" />, {
      language: 'de',
    });
    expect(screen.getByText('Verweigert')).toBeVisible();
    german.unmount();

    renderInShell(<ActivityOutcomeBadge outcome="denied" />, { language: 'ro' });
    expect(screen.getByText('Refuzat')).toBeVisible();
  });
});
