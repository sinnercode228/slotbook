import '@testing-library/jest-dom/vitest';
import { cleanup } from '@testing-library/react';
import { afterEach } from 'vitest';

afterEach(() => {
  cleanup();
  // Files that run in the node environment have no browser storage.
  globalThis.sessionStorage?.clear();
});
