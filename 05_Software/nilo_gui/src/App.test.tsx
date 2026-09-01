import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import App from './App';

describe('NILO GUI foundation', () => {
  it('identifies the development interface as simulation', () => {
    render(<App />);

    expect(screen.getByRole('heading', { name: 'NILO' })).toBeVisible();
    expect(screen.getByText('Networked Intelligent Lab Orchestrator')).toBeVisible();
    expect(screen.getByText(/development interface/i)).toBeVisible();
    expect(screen.getByRole('status', { name: 'Runtime profile' })).toHaveTextContent(
      'SIMULATION',
    );
  });
});
