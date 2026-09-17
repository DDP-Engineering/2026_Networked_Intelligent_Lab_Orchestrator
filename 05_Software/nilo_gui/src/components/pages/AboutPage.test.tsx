import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import { AboutPage } from './SimplePages';
import { LanguageProvider } from '../../i18n/LanguageProvider';

describe('AboutPage appliance identity', () => {
  it('leads with controls and shows software, system, identity, and an inline label', async () => {
    const identify = vi
      .fn()
      .mockResolvedValue({ supported: true, activeUntil: '2026-09-15T12:00:10Z' });
    const saveFriendlyName = vi.fn();
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
            system: {
              operatingSystem: 'Debian GNU/Linux 13 (trixie)',
              kernelVersion: '6.12.47+rpt-rpi-v8',
              architecture: 'arm64',
              storageFreeBytes: 5 * 1024 ** 3,
              storageTotalBytes: 16 * 1024 ** 3,
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
          onFriendlyName={saveFriendlyName}
        />
      </LanguageProvider>,
    );
    expect(screen.getByText('nilo-p2rjmzdkf1')).toBeInTheDocument();
    expect(screen.getByText('bench-west')).toBeInTheDocument();
    expect(screen.getByText('192.168.1.42')).toBeInTheDocument();
    expect(screen.getByText('0.8.0')).toBeInTheDocument();
    expect(screen.getByText('Debian GNU/Linux 13 (trixie)')).toBeInTheDocument();
    expect(screen.getByText('6.12.47+rpt-rpi-v8')).toBeInTheDocument();
    expect(screen.getByText('arm64')).toBeInTheDocument();
    expect(screen.getByText('5.0 GiB free of 16 GiB')).toBeInTheDocument();
    const software = screen.getByRole('region', { name: 'Software' });
    expect(within(software).getByText('Software version')).toBeInTheDocument();
    expect(within(software).getByText('API version')).toBeInTheDocument();
    expect(within(software).getByText('Integration protocol')).toBeInTheDocument();
    const system = screen.getByRole('region', { name: 'System' });
    expect(within(system).getByText('Linux version')).toBeInTheDocument();
    expect(within(system).getByText('Kernel version')).toBeInTheDocument();
    expect(within(system).getByText('Architecture')).toBeInTheDocument();
    expect(within(system).getByText('Storage')).toBeInTheDocument();
    const management = screen.getByRole('region', { name: 'Management' });
    expect(within(management).getByText('Management URL')).toBeInTheDocument();
    expect(within(management).getByText('Management addresses')).toBeInTheDocument();
    expect(screen.queryByText('Hardware addresses')).not.toBeInTheDocument();
    expect(screen.getByLabelText('Friendly name')).toHaveValue('bench-west');
    expect(
      screen.getByRole('button', { name: /save friendly name/i }),
    ).toBeInTheDocument();
    expect(screen.queryByRole('table')).not.toBeInTheDocument();
    expect(screen.queryByText('Open printable label')).not.toBeInTheDocument();
    expect(screen.getByRole('img', { name: /printable label/i })).toHaveAttribute(
      'src',
      '/api/v1/appliance/label.svg',
    );
    fireEvent.click(screen.getByRole('button', { name: /identify/i }));
    await waitFor(() => expect(identify).toHaveBeenCalledOnce());
  });
});
