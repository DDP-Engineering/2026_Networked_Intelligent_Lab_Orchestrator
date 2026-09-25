import { screen } from '@testing-library/react';
import { expect, it } from 'vitest';
import { renderInShell } from '../../test/render';
import { InstallationProgress } from './InstallationProgress';

it('announces installation progress and an unavailable helper without claiming failure', () => {
  const view = renderInShell(<InstallationProgress progress="downloading" />);
  expect(screen.getByRole('status')).toHaveTextContent('Downloading wireguard-tools.');
  view.unmount();
  renderInShell(<InstallationProgress progress="helper_unavailable" />);
  expect(screen.getByRole('status')).toHaveTextContent(
    'installation status is unknown',
  );
});
