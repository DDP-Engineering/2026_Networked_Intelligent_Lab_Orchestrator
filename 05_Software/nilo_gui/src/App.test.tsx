import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';

import App from './App';
import { BenchCoreError } from './api/client';
import type { BenchCoreClient } from './api/client';
import type { BenchCoreStatus } from './api/contract';

const healthyStatus: BenchCoreStatus = {
  apiVersion: 'nilo.http/1',
  status: 'healthy',
  profile: 'simulation',
  simulation: true,
  build: { name: 'nilo', version: '0.1.0', integrationProtocol: 'nilo.integration/1' },
};

const ENDPOINT = 'http://127.0.0.1:8090';

const BOUNDARY = /never represent evidence from physical equipment/i;

function stubClient(getStatus: BenchCoreClient['getStatus']): BenchCoreClient {
  return { baseUrl: '', getStatus };
}

function resolving(status: BenchCoreStatus = healthyStatus): BenchCoreClient {
  return stubClient(() => Promise.resolve(status));
}

function failing(
  failure: 'unauthorized' | 'unavailable' | 'malformed',
): BenchCoreClient {
  return stubClient(() =>
    Promise.reject(new BenchCoreError(failure, `simulated ${failure}`)),
  );
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
    const client = resolving();
    const getStatus = vi.spyOn(client, 'getStatus');
    render(<App client={client} apiMode="live" endpoint={ENDPOINT} />);

    expect(tokenField()).toHaveAttribute('type', 'password');
    expect(connectButton()).toBeVisible();
    expect(screen.getByText(/http:\/\/127\.0\.0\.1:8090/)).toBeVisible();
    expect(getStatus).not.toHaveBeenCalled();
  });

  it('refuses an empty token without contacting the backend', async () => {
    const client = resolving();
    const getStatus = vi.spyOn(client, 'getStatus');
    render(<App client={client} apiMode="live" endpoint={ENDPOINT} />);

    fireEvent.click(connectButton());

    expect(await screen.findByText(/enter the bearer token/i)).toBeVisible();
    expect(getStatus).not.toHaveBeenCalled();
  });

  it('sends the entered token as the bearer credential', async () => {
    const client = resolving();
    const getStatus = vi.spyOn(client, 'getStatus');

    connectWith(client, 'issued-development-token');

    await waitFor(() =>
      expect(getStatus).toHaveBeenCalledWith('issued-development-token'),
    );
  });
});

describe('connection states', () => {
  it('shows a distinct in-progress state while the status request is pending', async () => {
    connectWith(stubClient(() => new Promise<BenchCoreStatus>(() => {})));

    expect(await screen.findByText(/contacting bench core/i)).toBeVisible();
    expect(screen.queryByText(/bench core connected/i)).toBeNull();
    expectSimulationIdentity();
  });

  it('shows the reported status once Bench Core answers', async () => {
    connectWith(resolving());

    expect(await screen.findByText(/bench core connected/i)).toBeVisible();
    expect(screen.getByText('simulation')).toBeVisible();
    expect(screen.getByText('healthy')).toBeVisible();
    expect(screen.getByText('0.1.0')).toBeVisible();
    expect(screen.getByRole('button', { name: /refresh status/i })).toBeVisible();
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

  it('shows a distinct unsupported-response state', async () => {
    connectWith(failing('malformed'));

    expect(await screen.findByText(/unsupported response/i)).toBeVisible();
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

  it('retries with the same token when the backend was unreachable', async () => {
    let attempts = 0;
    const client = stubClient(() => {
      attempts += 1;
      return attempts === 1
        ? Promise.reject(new BenchCoreError('unavailable', 'simulated outage'))
        : Promise.resolve(healthyStatus);
    });
    connectWith(client, 'issued-development-token');

    fireEvent.click(await screen.findByRole('button', { name: /try again/i }));

    expect(await screen.findByText(/bench core connected/i)).toBeVisible();
    expect(attempts).toBe(2);
  });

  it('keeps the simulation identity on the unauthenticated screen', () => {
    render(<App client={resolving()} apiMode="live" endpoint={ENDPOINT} />);

    expectSimulationIdentity();
    expect(screen.getByRole('heading', { name: 'NILO' })).toBeVisible();
    expect(screen.getByText(/development interface/i)).toBeVisible();
  });
});

describe('bearer token retention', () => {
  it('never writes the token to a durable browser store', async () => {
    const stored = vi.spyOn(Storage.prototype, 'setItem');
    const cookie = vi.spyOn(Document.prototype, 'cookie', 'set');

    connectWith(resolving(), 'issued-development-token');
    await screen.findByText(/bench core connected/i);

    expect(stored).not.toHaveBeenCalled();
    expect(cookie).not.toHaveBeenCalled();
    expect(JSON.stringify(localStorage)).not.toContain('issued-development-token');
    expect(JSON.stringify(sessionStorage)).not.toContain('issued-development-token');
  });

  it('presents the login screen again after the application is remounted', async () => {
    const { unmount } = render(
      <App client={resolving()} apiMode="live" endpoint={ENDPOINT} />,
    );
    submitToken('issued-development-token');
    await screen.findByText(/bench core connected/i);
    unmount();

    render(<App client={resolving()} apiMode="live" endpoint={ENDPOINT} />);

    expect(tokenField()).toHaveValue('');
    expect(screen.queryByText(/bench core connected/i)).toBeNull();
  });
});

describe('mock mode', () => {
  it('says the interface is answering from its own contract mock', () => {
    render(<App client={resolving()} apiMode="mock" endpoint={ENDPOINT} />);

    expect(screen.getByText(/contract mock/i)).toBeVisible();
    expectSimulationIdentity();
  });
});
