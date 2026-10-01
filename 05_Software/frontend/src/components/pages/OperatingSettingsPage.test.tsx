import { fireEvent, screen, waitFor } from '@testing-library/react';
import { expect, it, vi } from 'vitest';

import { renderInShell } from '../../test/render';
import { BenchCoreError } from '../../api/client';
import { OperatingSettingsPage } from './OperatingSettingsPage';

const initial = {
  revision: 1,
  settings: {
    executing_jobs: 2,
    outstanding_jobs: 32,
    serial_connections: 4,
    camera_streams: 2,
    default_job_timeout_seconds: 60,
    max_job_timeout_seconds: 300,
    serial_lifetime_minutes: 60,
    serial_idle_minutes: 1,
    camera_lifetime_minutes: 55,
  },
  usage: {
    executing_jobs: 1,
    outstanding_jobs: 3,
    serial_connections: 2,
    camera_streams: 1,
  },
};

it('shows platform settings as editable table rows with help and saves from the top', async () => {
  const save = vi.fn().mockResolvedValue({ ...initial, revision: 2 });
  renderInShell(
    <OperatingSettingsPage read={() => Promise.resolve(initial)} save={save} />,
  );

  expect(
    await screen.findByRole('heading', { name: 'Nilo Settings' }),
  ).toBeInTheDocument();
  expect(screen.getByRole('table')).toBeInTheDocument();
  expect(
    screen.getByRole('columnheader', { name: 'Allowed range' }),
  ).toBeInTheDocument();
  expect(screen.getByRole('columnheader', { name: 'Usage' })).toBeInTheDocument();
  expect(screen.getAllByRole('row')).toHaveLength(5);
  expect(screen.getByLabelText('Executing finite jobs')).toHaveValue(2);
  expect(screen.getAllByText(/1 of 2 in use/).length).toBeGreaterThan(0);
  expect(screen.getByText(/1–8 jobs/)).toBeInTheDocument();
  expect(screen.queryByLabelText('Active serial connections')).not.toBeInTheDocument();
  expect(screen.queryByLabelText('Active camera streams')).not.toBeInTheDocument();
  fireEvent.click(
    screen.getByRole('button', { name: 'Help for Executing finite jobs' }),
  );
  expect(
    screen.getByText(/Finite jobs executing across all plugins/),
  ).toBeInTheDocument();
  const saveButton = screen.getByRole('button', { name: 'Save settings' });
  expect(
    saveButton.compareDocumentPosition(screen.getByRole('table')) &
      Node.DOCUMENT_POSITION_FOLLOWING,
  ).toBeTruthy();
  fireEvent.change(screen.getByLabelText('Executing finite jobs'), {
    target: { value: '3' },
  });
  fireEvent.click(saveButton);
  await waitFor(() =>
    expect(save).toHaveBeenCalledWith(1, { ...initial.settings, executing_jobs: 3 }),
  );
});

it('shows a stale revision conflict without replacing the displayed values', async () => {
  const save = vi
    .fn()
    .mockRejectedValue(
      new BenchCoreError('rejected', 'conflict', 'operating_settings_conflict'),
    );
  renderInShell(
    <OperatingSettingsPage read={() => Promise.resolve(initial)} save={save} />,
  );
  expect(await screen.findByLabelText('Executing finite jobs')).toHaveValue(2);
  fireEvent.change(screen.getByLabelText('Executing finite jobs'), {
    target: { value: '3' },
  });
  fireEvent.click(screen.getByRole('button', { name: 'Save settings' }));
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'Settings changed in another session',
  );
  expect(screen.getByLabelText('Executing finite jobs')).toHaveValue(3);
});

it('rejects a default timeout above the maximum without saving', async () => {
  const save = vi.fn();
  renderInShell(
    <OperatingSettingsPage read={() => Promise.resolve(initial)} save={save} />,
  );
  const field = await screen.findByLabelText('Default job timeout');
  fireEvent.change(field, { target: { value: '301' } });
  fireEvent.submit(field.closest('form')!);
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'The default job timeout cannot exceed the maximum',
  );
  expect(save).not.toHaveBeenCalled();
});
