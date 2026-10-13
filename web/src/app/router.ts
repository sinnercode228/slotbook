import { useSyncExternalStore } from 'react';

// Hash routes (#/booking/<id>?token=...) work on any static host, GitHub
// Pages included, and on the Go server without extra rewrite rules.

export interface Route {
  path: string;
  query: URLSearchParams;
}

export function parseHash(hash: string): Route {
  const raw = hash.replace(/^#/, '') || '/';
  const [path = '/', search = ''] = raw.split('?', 2);
  return { path: path.startsWith('/') ? path : `/${path}`, query: new URLSearchParams(search) };
}

function subscribe(onChange: () => void): () => void {
  window.addEventListener('hashchange', onChange);
  return () => window.removeEventListener('hashchange', onChange);
}

export function useRoute(): Route {
  const hash = useSyncExternalStore(subscribe, () => window.location.hash);
  return parseHash(hash);
}

export function navigate(to: string, { replace = false } = {}): void {
  const hash = `#${to}`;
  if (replace) {
    window.history.replaceState(null, '', hash);
    window.dispatchEvent(new HashChangeEvent('hashchange'));
  } else {
    window.location.hash = hash;
  }
}

/** An absolute link to a hash route of this app, for sharing outside the page. */
export function absoluteUrl(path: string): string {
  const { origin, pathname, search } = window.location;
  return `${origin}${pathname}${search}#${path}`;
}

export function manageRoute(bookingId: string, manageToken: string): string {
  return `/booking/${encodeURIComponent(bookingId)}?token=${encodeURIComponent(manageToken)}`;
}
