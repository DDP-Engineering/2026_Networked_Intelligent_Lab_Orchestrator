import { fireEvent, screen, within } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import { renderInShell } from '../../test/render';
import { NetworkSummary } from './NetworkSummary';

function open(
  overrides: Partial<Parameters<typeof NetworkSummary>[0]> = {},
): HTMLElement {
  renderInShell(
    <NetworkSummary
      interfaceState="disconnected"
      tunnelActive
      handshakeAgeSeconds={42}
      benchCoreReachable
      onOpenNetwork={() => {}}
      {...overrides}
    />,
    { theme: 'light', language: 'en' },
  );
  fireEvent.click(screen.getByRole('button', { name: 'Network' }));
  return screen.getByRole('menu');
}

describe('the top-row network indicator', () => {
  // An active tunnel and a reachable instrument are different claims, and this
  // indicator is exactly where they get confused. The four facts stay four.
  it('states all four facts separately, even when they disagree', () => {
    const menu = open();

    for (const label of [
      'Interface connection',
      'Tunnel',
      'Last handshake',
      'Bench Core reachability',
    ]) {
      expect(within(menu).getByText(label)).toBeInTheDocument();
    }

    // Disconnected interface, active tunnel, recent handshake, reachable core:
    // a state no single light could honestly summarise.
    expect(menu).toHaveTextContent(/disconnected/i);
    expect(menu).toHaveTextContent(/active/i);
    expect(menu).toHaveTextContent(/42 seconds ago/i);
    expect(menu).toHaveTextContent(/answered this interface over HTTP/i);
  });

  it('denies that any of this says a remote endpoint is reachable', () => {
    const menu = open();
    // The reachability sentence is a denial, and the denial is the point: it
    // scopes the claim to this one HTTP connection and explicitly disclaims
    // both the appliance's own networking and any remote endpoint.
    expect(menu).toHaveTextContent(/state of this one connection only/i);
    expect(menu).toHaveTextContent(/not the appliance LAN, Wi-Fi or VPN state/i);
    expect(menu).toHaveTextContent(
      /says nothing about whether any remote endpoint is reachable/i,
    );
  });

  it('says a fact is not reported rather than guessing it', () => {
    const menu = open({ tunnelActive: null, handshakeAgeSeconds: null });
    // Two of the four are unknown; neither becomes "off" or "never".
    expect(menu.textContent?.match(/Not reported/g) ?? []).toHaveLength(2);
  });

  it('opens the Network page from the indicator', () => {
    let opened = false;
    renderInShell(
      <NetworkSummary
        interfaceState="connected"
        tunnelActive={null}
        handshakeAgeSeconds={null}
        benchCoreReachable
        onOpenNetwork={() => {
          opened = true;
        }}
      />,
      { theme: 'light', language: 'en' },
    );
    fireEvent.click(screen.getByRole('button', { name: 'Network' }));
    fireEvent.click(screen.getByRole('button', { name: /open network/i }));
    expect(opened).toBe(true);
  });
});
