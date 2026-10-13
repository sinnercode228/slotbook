import type { ContactValues } from './contact';

// The booking widget keeps its progress in sessionStorage, so that a reload
// halfway through keeps the running hold and the typed details, and a reload
// after booking still shows the manage link, which the server hands out only
// once. sessionStorage belongs to one tab, and each tab is its own visit.

const KEY = 'slotbook.booking';
const VERSION = 1;

export interface Progress<S> {
  state: S;
  contact: ContactValues;
}

export function loadProgress<S>(): Progress<S> | null {
  try {
    const raw = window.sessionStorage.getItem(KEY);
    if (!raw) return null;
    const saved = JSON.parse(raw) as Partial<Progress<S>> & { version?: number };
    if (saved.version !== VERSION || !saved.state || !saved.contact) return null;
    return { state: saved.state, contact: saved.contact };
  } catch {
    return null;
  }
}

export function saveProgress<S>(progress: Progress<S>): void {
  try {
    window.sessionStorage.setItem(KEY, JSON.stringify({ version: VERSION, ...progress }));
  } catch {
    // Storage can be full or blocked; the widget then works as before, in memory.
  }
}

export function forgetProgress(): void {
  try {
    window.sessionStorage.removeItem(KEY);
  } catch {
    // Nothing was saved then either.
  }
}
