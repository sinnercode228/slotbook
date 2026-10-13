import type { BookingApi } from '../api/client';
import { createHttpApi } from '../api/http';

/**
 * Picks the API from the build: VITE_API_MODE=mock bundles the in-browser
 * mock (and the seed file); any other build talks to VITE_API_URL, or to the
 * page's own origin when it is empty.
 */
export async function createApi(): Promise<BookingApi> {
  if (import.meta.env.VITE_API_MODE === 'mock') {
    const { createMockApi } = await import('../mock/mockApi');
    return createMockApi({ latencyMs: 200 });
  }
  return createHttpApi(import.meta.env.VITE_API_URL ?? '');
}
