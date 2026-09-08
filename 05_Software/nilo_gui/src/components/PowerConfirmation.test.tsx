import { fireEvent, screen, waitFor } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import { renderInShell } from '../test/render';
import { BenchCoreError } from '../api/client';
import { PowerConfirmation } from './PowerConfirmation';

describe('PowerConfirmation', () => {
  it('sends nothing when cancelled', () => {
    const request = vi.fn(() => Promise.resolve());
    const dismiss = vi.fn();
    renderInShell(
      <PowerConfirmation action="power_off" onConfirm={request} onDismiss={dismiss} />,
    );

    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));

    expect(dismiss).toHaveBeenCalledTimes(1);
    expect(request).not.toHaveBeenCalled();
  });

  it('requires one explicit confirmation and reports acceptance', async () => {
    const request = vi.fn(() => Promise.resolve());
    renderInShell(
      <PowerConfirmation action="restart" onConfirm={request} onDismiss={() => {}} />,
    );

    expect(screen.getByText(/restart the whole NILO appliance/i)).toBeVisible();
    fireEvent.click(screen.getByRole('button', { name: 'Restart NILO' }));

    await waitFor(() => expect(request).toHaveBeenCalledTimes(1));
    expect(screen.getByRole('status')).toHaveTextContent(
      /restart request was accepted/i,
    );
  });

  it('does not infer success from a lost connection or retry automatically', async () => {
    const request = vi.fn(() => Promise.reject(new Error('connection lost')));
    renderInShell(
      <PowerConfirmation action="power_off" onConfirm={request} onDismiss={() => {}} />,
    );

    fireEvent.click(screen.getByRole('button', { name: 'Power off NILO' }));

    expect(await screen.findByRole('alert')).toHaveTextContent(
      /did not confirm the power-off request/i,
    );
    expect(request).toHaveBeenCalledTimes(1);
  });

  it('distinguishes a server refusal from an unconfirmed connection loss', async () => {
    const request = vi.fn(() =>
      Promise.reject(new BenchCoreError('forbidden', 'denied', 'forbidden')),
    );
    renderInShell(
      <PowerConfirmation action="restart" onConfirm={request} onDismiss={() => {}} />,
    );

    fireEvent.click(screen.getByRole('button', { name: 'Restart NILO' }));

    expect(await screen.findByRole('alert')).toHaveTextContent(
      /restart request was refused/i,
    );
    expect(screen.queryByText(/did not confirm/i)).toBeNull();
  });
});
