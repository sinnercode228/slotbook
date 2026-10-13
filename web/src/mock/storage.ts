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

const STORE = 'state';
const KEY = 'current';

function openDatabase(name: string): Promise<IDBDatabase> {
  return new Promise((resolve, reject) => {
    const request = indexedDB.open(name, 1);
    request.onupgradeneeded = () => request.result.createObjectStore(STORE);
    request.onsuccess = () => resolve(request.result);
    request.onerror = () => reject(request.error);
  });
}

/**
 * Keeps the state in IndexedDB so that it survives reloads and is shared by
 * every tab of the demo. Each call is one readwrite transaction, and the
 * browser runs those one at a time across tabs, so two tabs reaching for one
 * slot cannot both get it. localStorage cannot promise that: in Chromium a
 * write in one tab reaches the others asynchronously, even under a Web Lock.
 * A BroadcastChannel tells the other tabs to refresh.
 */
export function browserStorage(name = 'slotbook-demo'): StateStorage {
  if (typeof indexedDB === 'undefined') return memoryStorage();
  const database = openDatabase(name).catch(() => null);
  const fallback = memoryStorage();
  const channel = typeof BroadcastChannel === 'undefined' ? null : new BroadcastChannel(name);

  return {
    async transact<T>(fn: (state: MockState) => T): Promise<T> {
      const db = await database;
      if (!db) return fallback.transact(fn);
      return new Promise<T>((resolve, reject) => {
        const tx = db.transaction(STORE, 'readwrite');
        const store = tx.objectStore(STORE);
        let result: T;
        let changed = false;
        let thrown: { error: unknown } | null = null;
        const read = store.get(KEY);
        read.onsuccess = () => {
          const before = (read.result as string | undefined) ?? null;
          const draft = decodeState(before);
          try {
            result = fn(draft);
          } catch (error) {
            thrown = { error };
            tx.abort();
            return;
          }
          const after = JSON.stringify(draft);
          if (after !== before) {
            store.put(after, KEY);
            changed = true;
          }
        };
        tx.oncomplete = () => {
          if (changed) channel?.postMessage('changed');
          resolve(result);
        };
        tx.onabort = () => reject(thrown ? thrown.error : tx.error);
      });
    },
    subscribe(onChange) {
      if (!channel) return () => {};
      const listener = () => onChange();
      channel.addEventListener('message', listener);
      return () => channel.removeEventListener('message', listener);
    },
    async clear() {
      const db = await database;
      if (!db) return fallback.clear();
      await new Promise<void>((resolve, reject) => {
        const tx = db.transaction(STORE, 'readwrite');
        tx.objectStore(STORE).delete(KEY);
        tx.oncomplete = () => {
          channel?.postMessage('changed');
          resolve();
        };
        tx.onabort = () => reject(tx.error);
      });
    },
  };
}
