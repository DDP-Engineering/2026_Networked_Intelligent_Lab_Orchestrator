import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import { AboutPage } from './SimplePages';
import { LanguageProvider } from '../../i18n/LanguageProvider';

describe('AboutPage appliance identity', () => {
  it('shows public discovery facts and offers printable label and Identify', async () => {
    const identify = vi
      .fn()
      .mockResolvedValue({ supported: true, activeUntil: '2026-09-15T12:00:10Z' });
    render(
      <LanguageProvider initial="en">
        <AboutPage
          status={{
            apiVersion: 'nilo.http/1',
            status: 'healthy',
            profile: 'appliance',
            simulation: false,
            build: {
              name: 'nilo',
              version: '0.8.0',
              integrationProtocol: 'nilo.integration/1',
            },
          }}
          appliance={{
            id: '0199a2d0-1234-7abc-8def-0123456789ab',
            serialName: 'nilo-p2rjmzdkf1',
            friendlyName: 'bench-west',
            state: 'commissioned',
            version: '0.8.0',
            managementUrl: 'https://nilo-p2rjmzdkf1.local:8443/',
            managementAddresses: ['192.168.1.42'],
            managementMacs: ['02:00:00:00:00:02'],
          }}
          labelUrl="/api/v1/appliance/label.svg"
          canIdentify
          onIdentify={identify}
        />
      </LanguageProvider>,
    );
    expect(screen.getByText('nilo-p2rjmzdkf1')).toBeInTheDocument();
    expect(screen.getByText('bench-west')).toBeInTheDocument();
    expect(screen.getByText('192.168.1.42')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: /printable label/i })).toHaveAttribute(
      'href',
      '/api/v1/appliance/label.svg',
    );
    fireEvent.click(screen.getByRole('button', { name: /identify/i }));
    await waitFor(() => expect(identify).toHaveBeenCalledOnce());
  });
});
