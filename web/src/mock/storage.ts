import { decodeState, type MockState } from './state';

export interface StateStorage {
  /**
   * Runs fn on the latest state, one call at a time across all tabs. Changes
   * fn makes are saved only if it returns normally; a throw discards them,
   * which gives each call the all-or-nothing behaviour of a transaction.
   */
  transact<T>(fn: (state: MockState) => T): Promise<T>;
  /** Calls onChange when another tab saved a change. */
  subscribe(onChange: () => void): () => void;
  clear(): Promise<void>;
}

/** Keeps the state in this JavaScript context only; used in tests and as a fallback. */
export function memoryStorage(): StateStorage {
  let saved: string | null = null;
  let queue: Promise<unknown> = Promise.resolve();
  const serialize = <T>(fn: () => T): Promise<T> => {
    const run = queue.then(fn);
    queue = run.catch(() => undefined);
    return run;
  };
  return {
    transact: (fn) =>
      serialize(() => {
        const draft = decodeState(saved);
        const result = fn(draft);
        saved = JSON.stringify(draft);
        return result;
      }),
    subscribe: () => () => {},
    clear: () =>
      serialize(() => {
        saved = null;
      }),
  };
}

function availableLocalStorage(): Storage | null {
  try {
    const probe = '__slotbook_probe__';
    window.localStorage.setItem(probe, probe);
    window.localStorage.removeItem(probe);
    return window.localStorage;
  } catch {
    return null;
  }
}

/**
 * Keeps the state in localStorage so that it survives reloads and is shared
 * by every tab of the demo. The Web Locks API makes each read-modify-write
 * exclusive across tabs, so two tabs reaching for one slot cannot both get
 * it; the storage event tells the other tabs to refresh.
 */
export function browserStorage(key = 'slotbook.demo'): StateStorage {
  const local = availableLocalStorage();
  if (!local) return memoryStorage();

  const exclusive = <T>(fn: () => T): Promise<T> =>
    navigator.locks ? navigator.locks.request(key, () => fn()) : Promise.resolve().then(fn);

  return {
    transact: (fn) =>
      exclusive(() => {
        const before = local.getItem(key);
        const draft = decodeState(before);
        const result = fn(draft);
        const after = JSON.stringify(draft);
        if (after !== before) local.setItem(key, after);
        return result;
      }),
    subscribe(onChange) {
      const handler = (event: StorageEvent) => {
        if (event.key === key || event.key === null) onChange();
      };
      window.addEventListener('storage', handler);
      return () => window.removeEventListener('storage', handler);
    },
    clear: () => exclusive(() => local.removeItem(key)),
  };
}
