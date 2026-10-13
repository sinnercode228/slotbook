import { useCallback, useEffect, useRef, useState } from 'react';

export interface Resource<T> {
  /** The latest value for the current key; kept while a reload runs. */
  data: T | undefined;
  error: unknown;
  loading: boolean;
  reload: () => void;
}

interface Settled<T> {
  key: string;
  run: number;
  data?: T;
  error?: unknown;
}

/**
 * Loads data for a key and reloads when the key changes. A null key loads
 * nothing. reload() fetches again without dropping the data on screen, so
 * background refreshes do not flash a spinner.
 */
export function useResource<T>(key: string | null, load: () => Promise<T>): Resource<T> {
  const [settled, setSettled] = useState<Settled<T> | null>(null);
  const [run, setRun] = useState(0);
  const loadRef = useRef(load);

  useEffect(() => {
    loadRef.current = load;
  });

  useEffect(() => {
    if (key === null) return;
    let current = true;
    loadRef.current().then(
      (data) => current && setSettled({ key, run, data }),
      (error: unknown) => current && setSettled({ key, run, error }),
    );
    return () => {
      current = false;
    };
  }, [key, run]);

  const reload = useCallback(() => setRun((n) => n + 1), []);
  const sameKey = settled !== null && settled.key === key;
  return {
    data: sameKey ? settled.data : undefined,
    error: sameKey && settled.run === run ? settled.error : undefined,
    loading: key !== null && (!sameKey || settled.run !== run),
    reload,
  };
}
