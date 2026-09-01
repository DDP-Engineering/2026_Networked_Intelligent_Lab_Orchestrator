import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';

import App from './App';
import { BenchCoreError } from './api/client';
import type { BenchCoreClient, BenchCoreFailure } from './api/client';
import type {
  BenchCoreStatus,
  DeviceInventory,
  IntegrationInventory,
} from './api/contract';

const healthyStatus: BenchCoreStatus = {
  apiVersion: 'nilo.http/1',
  status: 'healthy',
  profile: 'simulation',
  simulation: true,
  build: { name: 'nilo', version: '0.1.0', integrationProtocol: 'nilo.integration/1' },
};

const readyIntegrations: IntegrationInventory = {
  apiVersion: 'nilo.http/1',
  scenario: 'healthy',
  items: [
    {
      id: 'simulation.fake-python',
      displayName: 'SIMULATION - Fake Python Integration',
      status: 'healthy',
      simulation: true,
      operations: ['artifact', 'crash', 'echo', 'fail', 'sleep'],
    },
  ],
};

const readyDevices: DeviceInventory = {
  apiVersion: 'nilo.http/1',
  scenario: 'healthy',
  items: [
    {
      id: 'simulation.fake-python.device.stable-001',
      integrationId: 'simulation.fake-python',
      displayName: 'SIMULATION - Electronics Bench 001',
      kind: 'simulated-electronics-bench',
      status: 'ready',
      simulation: true,
      capabilities: ['artifact', 'crash', 'echo', 'fail', 'sleep'],
    },
  ],
};

const unavailableIntegrations: IntegrationInventory = {
  ...readyIntegrations,
  scenario: 'unavailable',
  items: readyIntegrations.items.map((item) => ({ ...item, status: 'unavailable' })),
};

const unavailableDevices: DeviceInventory = {
  ...readyDevices,
  scenario: 'unavailable',
  items: readyDevices.items.map((item) => ({ ...item, status: 'unavailable' })),
};

const ENDPOINT = 'http://127.0.0.1:8090';

const BOUNDARY = /never represent evidence from physical equipment/i;

/** A client that answers everything healthily unless an override says otherwise. */
function stubClient(overrides: Partial<BenchCoreClient> = {}): BenchCoreClient {
  return {
    baseUrl: '',
    getStatus: () => Promise.resolve(healthyStatus),
    getIntegrations: () => Promise.resolve(readyIntegrations),
    getDevices: () => Promise.resolve(readyDevices),
    ...overrides,
  };
}

function rejecting(failure: BenchCoreFailure, detail = `simulated ${failure}`) {
  return () => Promise.reject(new BenchCoreError(failure, detail));
}

/** Every request fails the same way, as a stopped backend or a bad token does. */
function failing(failure: BenchCoreFailure): BenchCoreClient {
  return stubClient({
    getStatus: rejecting(failure),
    getIntegrations: rejecting(failure),
    getDevices: rejecting(failure),
  });
}

function tokenField(): HTMLElement {
  return screen.getByLabelText(/development bearer token/i);
}

function connectButton(): HTMLElement {
  return screen.getByRole('button', { name: /^connect$/i });
}

function submitToken(token: string): void {
  fireEvent.change(tokenField(), { target: { value: token } });
  fireEvent.click(connectButton());
}

function connectWith(client: BenchCoreClient, token = 'development-token'): void {
  render(<App client={client} apiMode="live" endpoint={ENDPOINT} />);
  submitToken(token);
}

function statusRegion(): HTMLElement {
  return screen.getByRole('region', { name: /bench core connected/i });
}

function integrationsRegion(): HTMLElement {
  return screen.getByRole('region', { name: /^integrations$/i });
}

function devicesRegion(): HTMLElement {
  return screen.getByRole('region', { name: /^devices$/i });
}

function expectSimulationIdentity(): void {
  expect(screen.getByRole('status', { name: 'Runtime profile' })).toHaveTextContent(
    'SIMULATION',
  );
  expect(screen.getByText(BOUNDARY)).toBeVisible();
}

afterEach(() => {
  vi.restoreAllMocks();
  localStorage.clear();
  sessionStorage.clear();
});

describe('development login screen', () => {
  it('offers a masked token field, a submit control, and the endpoint it will contact', () => {
    const client = stubClient();
    const getStatus = vi.spyOn(client, 'getStatus');
    render(<App client={client} apiMode="live" endpoint={ENDPOINT} />);

    expect(tokenField()).toHaveAttribute('type', 'password');
    expect(connectButton()).toBeVisible();
    expect(screen.getByText(/http:\/\/127\.0\.0\.1:8090/)).toBeVisible();
    expect(getStatus).not.toHaveBeenCalled();
  });

  it('refuses an empty token without contacting the backend', async () => {
    const client = stubClient();
    const getStatus = vi.spyOn(client, 'getStatus');
    render(<App client={client} apiMode="live" endpoint={ENDPOINT} />);

    fireEvent.click(connectButton());

    expect(await screen.findByText(/enter the bearer token/i)).toBeVisible();
    expect(getStatus).not.toHaveBeenCalled();
  });

  it('sends the entered token as the bearer credential for every resource', async () => {
    const client = stubClient();
    const getStatus = vi.spyOn(client, 'getStatus');
    const getIntegrations = vi.spyOn(client, 'getIntegrations');
    const getDevices = vi.spyOn(client, 'getDevices');

    connectWith(client, 'issued-development-token');

    await waitFor(() => {
      expect(getStatus).toHaveBeenCalledWith('issued-development-token');
      expect(getIntegrations).toHaveBeenCalledWith('issued-development-token');
      expect(getDevices).toHaveBeenCalledWith('issued-development-token');
    });
  });
});

describe('connection states', () => {
  it('shows a distinct in-progress state while the requests are pending', async () => {
    connectWith(
      stubClient({ getStatus: () => new Promise<BenchCoreStatus>(() => {}) }),
    );

    expect(await screen.findByText(/contacting bench core/i)).toBeVisible();
    expect(screen.queryByText(/bench core connected/i)).toBeNull();
    expectSimulationIdentity();
  });

  it('shows the reported status once Bench Core answers', async () => {
    connectWith(stubClient());

    expect(await screen.findByText(/bench core connected/i)).toBeVisible();
    expect(within(statusRegion()).getByText('simulation')).toBeVisible();
    expect(within(statusRegion()).getByText('healthy')).toBeVisible();
    expect(within(statusRegion()).getByText('0.1.0')).toBeVisible();
    expect(screen.getByRole('button', { name: /refresh/i })).toBeVisible();
    expectSimulationIdentity();
  });

  it('shows a distinct rejected-token state', async () => {
    connectWith(failing('unauthorized'));

    expect(await screen.findByText(/rejected this token/i)).toBeVisible();
    expect(screen.queryByText(/not reachable/i)).toBeNull();
    expect(
      screen.getByRole('button', { name: /use a different token/i }),
    ).toBeVisible();
    expectSimulationIdentity();
  });

  it('shows a distinct unreachable-backend state', async () => {
    connectWith(failing('unavailable'));

    expect(await screen.findByText(/is not reachable/i)).toBeVisible();
    expect(screen.queryByText(/rejected this token/i)).toBeNull();
    expect(screen.getByRole('button', { name: /try again/i })).toBeVisible();
    expectSimulationIdentity();
  });

  it('shows a distinct unsupported-status state', async () => {
    connectWith(failing('malformed'));

    expect(await screen.findByText(/unsupported status response/i)).toBeVisible();
    expect(screen.queryByText(/is not reachable/i)).toBeNull();
    expect(screen.getByRole('button', { name: /try again/i })).toBeVisible();
    expectSimulationIdentity();
  });

  it('returns to an empty login screen when a rejected token is replaced', async () => {
    connectWith(failing('unauthorized'));

    fireEvent.click(
      await screen.findByRole('button', { name: /use a different token/i }),
    );

    expect(tokenField()).toHaveValue('');
    expectSimulationIdentity();
  });

  it('retries every resource with the same token when the backend was unreachable', async () => {
    let attempts = 0;
    const client = stubClient({
      getStatus: () => {
        attempts += 1;
        return attempts === 1
          ? Promise.reject(new BenchCoreError('unavailable', 'simulated outage'))
          : Promise.resolve(healthyStatus);
      },
    });
    const getDevices = vi.spyOn(client, 'getDevices');
    connectWith(client, 'issued-development-token');

    fireEvent.click(await screen.findByRole('button', { name: /try again/i }));

    expect(await screen.findByText(/bench core connected/i)).toBeVisible();
    expect(attempts).toBe(2);
    expect(getDevices).toHaveBeenCalledTimes(2);
    expect(getDevices).toHaveBeenLastCalledWith('issued-development-token');
  });

  it('keeps the simulation identity on the unauthenticated screen', () => {
    render(<App client={stubClient()} apiMode="live" endpoint={ENDPOINT} />);

    expectSimulationIdentity();
    expect(screen.getByRole('heading', { name: 'NILO' })).toBeVisible();
    expect(screen.getByText(/development interface/i)).toBeVisible();
  });
});

describe('simulated inventory', () => {
  it('shows every integration the backend reports, with its state', async () => {
    connectWith(stubClient());

    const integrations = within(
      await screen.findByRole('region', { name: 'Integrations' }),
    );
    expect(
      integrations.getByText('SIMULATION - Fake Python Integration'),
    ).toBeVisible();
    expect(integrations.getByText('simulation.fake-python')).toBeVisible();
    expect(integrations.getByText('healthy')).toBeVisible();
    expectSimulationIdentity();
  });

  it('shows every device with the integration that owns it and its availability', async () => {
    connectWith(stubClient());

    const devices = within(await screen.findByRole('region', { name: 'Devices' }));
    expect(devices.getByText('SIMULATION - Electronics Bench 001')).toBeVisible();
    expect(devices.getByText('simulation.fake-python.device.stable-001')).toBeVisible();
    expect(devices.getByText('simulation.fake-python')).toBeVisible();
    expect(devices.getByText('ready')).toBeVisible();
    expect(devices.getByText('simulated-electronics-bench')).toBeVisible();
  });

  it('shows the unavailable scenario states rather than the ready ones', async () => {
    connectWith(
      stubClient({
        getIntegrations: () => Promise.resolve(unavailableIntegrations),
        getDevices: () => Promise.resolve(unavailableDevices),
      }),
    );

    await screen.findByRole('region', { name: 'Integrations' });
    expect(within(integrationsRegion()).getByText('unavailable')).toBeVisible();
    expect(within(devicesRegion()).getByText('unavailable')).toBeVisible();
    expect(within(devicesRegion()).queryByText('ready')).toBeNull();
    expect(within(statusRegion()).getByText('healthy')).toBeVisible();
    expectSimulationIdentity();
  });

  it('states that an inventory is empty instead of treating it as a failure', async () => {
    connectWith(
      stubClient({
        getIntegrations: () => Promise.resolve({ ...readyIntegrations, items: [] }),
        getDevices: () => Promise.resolve({ ...readyDevices, items: [] }),
      }),
    );

    await screen.findByRole('region', { name: 'Integrations' });
    expect(within(integrationsRegion()).getByText(/no integrations/i)).toBeVisible();
    expect(within(devicesRegion()).getByText(/no devices/i)).toBeVisible();
    expect(screen.queryByRole('alert')).toBeNull();
    expect(screen.queryByText(/could not be read/i)).toBeNull();
    expect(within(statusRegion()).getByText('healthy')).toBeVisible();
    expect(screen.getByRole('button', { name: /refresh/i })).toBeVisible();
    expect(
      screen.getByRole('button', { name: /use a different token/i }),
    ).toBeVisible();
    expectSimulationIdentity();
  });

  it('counts the entries it is showing', async () => {
    connectWith(stubClient());

    await screen.findByRole('region', { name: 'Integrations' });
    expect(within(integrationsRegion()).getByText(/1 entry/i)).toBeVisible();
    expect(within(devicesRegion()).getByText(/1 entry/i)).toBeVisible();
  });
});

describe('unusable inventory responses', () => {
  it('names the integration inventory and keeps the rest of the screen', async () => {
    connectWith(
      stubClient({ getIntegrations: rejecting('malformed', 'items is not a list') }),
    );

    await screen.findByRole('region', { name: 'Integrations' });
    const integrations = within(integrationsRegion());
    expect(
      integrations.getByText(/integration inventory could not be read/i),
    ).toBeVisible();
    expect(integrations.getByText(/items is not a list/i)).toBeVisible();
    expect(within(statusRegion()).getByText('healthy')).toBeVisible();
    expect(
      within(devicesRegion()).getByText('SIMULATION - Electronics Bench 001'),
    ).toBeVisible();
    expect(within(devicesRegion()).queryByText(/could not be read/i)).toBeNull();
    expectSimulationIdentity();
  });

  it('names the device inventory and keeps the rest of the screen', async () => {
    connectWith(
      stubClient({ getDevices: rejecting('malformed', 'entry 0 is missing kind') }),
    );

    await screen.findByRole('region', { name: 'Devices' });
    const devices = within(devicesRegion());
    expect(devices.getByText(/device inventory could not be read/i)).toBeVisible();
    expect(devices.getByText(/entry 0 is missing kind/i)).toBeVisible();
    expect(within(statusRegion()).getByText('healthy')).toBeVisible();
    expect(
      within(integrationsRegion()).getByText('SIMULATION - Fake Python Integration'),
    ).toBeVisible();
    expect(within(integrationsRegion()).queryByText(/could not be read/i)).toBeNull();
  });

  it('tells the two unusable inventories apart', async () => {
    connectWith(
      stubClient({ getIntegrations: rejecting('malformed', 'items is not a list') }),
    );
    expect(
      await screen.findByText(/integration inventory could not be read/i),
    ).toBeVisible();
    expect(screen.queryByText(/device inventory could not be read/i)).toBeNull();
  });

  it('reports an inventory the backend could not serve without claiming it is empty', async () => {
    connectWith(
      stubClient({
        getDevices: rejecting('unavailable', 'Bench Core answered HTTP 503'),
      }),
    );

    await screen.findByRole('region', { name: 'Devices' });
    expect(
      within(devicesRegion()).getByText(/device inventory is not reachable/i),
    ).toBeVisible();
    expect(within(devicesRegion()).queryByText(/no devices/i)).toBeNull();
  });

  it('is not the screen a rejected token or a stopped backend reaches', async () => {
    connectWith(failing('unauthorized'));

    expect(await screen.findByText(/rejected this token/i)).toBeVisible();
    expect(screen.queryByRole('region', { name: 'Integrations' })).toBeNull();
    expect(screen.queryByRole('region', { name: 'Devices' })).toBeNull();
    expect(screen.queryByText(/could not be read/i)).toBeNull();
  });
});

describe('bearer token retention', () => {
  it('never writes the token to a durable browser store', async () => {
    const stored = vi.spyOn(Storage.prototype, 'setItem');
    const cookie = vi.spyOn(Document.prototype, 'cookie', 'set');

    connectWith(stubClient(), 'issued-development-token');
    await screen.findByText(/bench core connected/i);
    fireEvent.click(screen.getByRole('button', { name: /refresh/i }));
    await screen.findByRole('region', { name: 'Devices' });

    expect(stored).not.toHaveBeenCalled();
    expect(cookie).not.toHaveBeenCalled();
    expect(JSON.stringify(localStorage)).not.toContain('issued-development-token');
    expect(JSON.stringify(sessionStorage)).not.toContain('issued-development-token');
    expect(document.body.innerHTML).not.toContain('issued-development-token');
  });

  it('presents the login screen again after the application is remounted', async () => {
    const { unmount } = render(
      <App client={stubClient()} apiMode="live" endpoint={ENDPOINT} />,
    );
    submitToken('issued-development-token');
    await screen.findByText(/bench core connected/i);
    unmount();

    render(<App client={stubClient()} apiMode="live" endpoint={ENDPOINT} />);

    expect(tokenField()).toHaveValue('');
    expect(screen.queryByText(/bench core connected/i)).toBeNull();
  });

  it('discards the token and the inventory when a different token is asked for', async () => {
    connectWith(stubClient(), 'issued-development-token');
    await screen.findByRole('region', { name: 'Devices' });

    fireEvent.click(screen.getByRole('button', { name: /use a different token/i }));

    expect(tokenField()).toHaveValue('');
    expect(screen.queryByRole('region', { name: 'Devices' })).toBeNull();
    expect(screen.queryByText('SIMULATION - Electronics Bench 001')).toBeNull();
  });
});

describe('mock mode', () => {
  it('says the interface is answering from its own contract mock', () => {
    render(<App client={stubClient()} apiMode="mock" endpoint={ENDPOINT} />);

    expect(screen.getByText(/contract mock/i)).toBeVisible();
    expectSimulationIdentity();
  });
});
