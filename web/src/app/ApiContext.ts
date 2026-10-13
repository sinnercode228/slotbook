import { createContext, useContext } from 'react';
import type { BookingApi } from '../api/client';
import type { MockApi } from '../mock/mockApi';

export const ApiContext = createContext<BookingApi | null>(null);

export function useApi(): BookingApi {
  const api = useContext(ApiContext);
  if (!api) throw new Error('useApi needs an ApiContext provider');
  return api;
}

/** The demo controls when the app runs on the in-browser mock, otherwise null. */
export function useDemo(): MockApi['demo'] | null {
  const api = useApi();
  return api.mode === 'mock' ? (api as MockApi).demo : null;
}
