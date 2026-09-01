import '@testing-library/jest-dom/vitest';
import { cleanup } from '@testing-library/react';
import { afterEach } from 'vitest';

// Vitest runs without injected globals, so Testing Library's automatic
// cleanup is not registered for us; without this, rendered trees from one
// test leak into the next.
afterEach(cleanup);
