import { useSyncExternalStore } from 'react';

// The slot list currently on screen, so the demo bar can take one of those
// very slots for "someone else" and the visitor can run into the conflict.

export interface VisibleSlots {
  serviceId: number;
  staffId: number | null;
  zone: string;
  starts: string[];
}

let current: VisibleSlots | null = null;
const listeners = new Set<() => void>();

export function publishVisibleSlots(next: VisibleSlots | null): void {
  current = next;
  listeners.forEach((listener) => listener());
}

function subscribe(listener: () => void): () => void {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

export function useVisibleSlots(): VisibleSlots | null {
  return useSyncExternalStore(subscribe, () => current);
}
