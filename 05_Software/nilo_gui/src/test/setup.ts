import '@testing-library/jest-dom/vitest';
import { cleanup } from '@testing-library/react';
import { afterEach } from 'vitest';

// Vitest runs without injected globals, so Testing Library's automatic
// cleanup is not registered for us; without this, rendered trees from one
// test leak into the next.
afterEach(cleanup);

// jsdom has no native dialog implementation. Component tests observe open/
// closed rendering; focus containment and keyboard use are verified in e2e.
Object.defineProperty(HTMLDialogElement.prototype, 'showModal', {
  configurable: true,
  value: function (this: HTMLDialogElement) {
    this.open = true;
  },
});
Object.defineProperty(HTMLDialogElement.prototype, 'close', {
  configurable: true,
  value: function (this: HTMLDialogElement) {
    this.open = false;
  },
});
