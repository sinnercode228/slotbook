import type { BookingKind, BookingStatus, Confirmation, Customer, Money, NotificationKind, NotificationStatus } from '../api/types';

// What the mock keeps, stored as one JSON document. Instants are epoch
// milliseconds; the API views format them in the location's zone.
// The demo keeps tokens as they are: everything stays in the visitor's own
// browser, unlike the server, which stores only their hashes.

export interface StoredBooking {
  id: string;
  kind: BookingKind;
  status: BookingStatus;
  locationId: number;
  staffId: number;
  serviceId: number | null;
  start: number;
  end: number;
  /** end plus the cleanup buffer: [start, busyUntil) is what the staff member is taken for. */
  busyUntil: number;
  price: Money | null;
  holdToken: string | null;
  holdExpiresAt: number | null;
  manageToken: string | null;
  customer: Customer | null;
  note: string;
  rescheduledFrom: string | null;
  createdAt: number;
  confirmedAt: number | null;
  cancelledAt: number | null;
}

export interface StoredNotification {
  id: number;
  bookingId: string;
  kind: NotificationKind;
  status: NotificationStatus;
  sendAt: number;
  nextAttemptAt: number;
  attempts: number;
  lastError: string | null;
  createdAt: number;
  sentAt: number | null;
}

export interface StoredKey {
  fingerprint: string;
  response: Confirmation;
  createdAt: number;
}

export const STATE_VERSION = 1;

export interface MockState {
  version: typeof STATE_VERSION;
  /** False until the sample bookings of a fresh demo have been added. */
  initialized: boolean;
  bookings: StoredBooking[];
  notifications: StoredNotification[];
  idempotencyKeys: Record<string, StoredKey>;
  nextNotificationId: number;
}

export function emptyState(): MockState {
  return {
    version: STATE_VERSION,
    initialized: false,
    bookings: [],
    notifications: [],
    idempotencyKeys: {},
    nextNotificationId: 1,
  };
}

/** Reads stored JSON; anything unreadable or from another version starts over. */
export function decodeState(raw: string | null): MockState {
  if (!raw) return emptyState();
  try {
    const parsed = JSON.parse(raw) as Partial<MockState>;
    if (parsed.version === STATE_VERSION && Array.isArray(parsed.bookings) && Array.isArray(parsed.notifications)) {
      return parsed as MockState;
    }
  } catch {
    // Fall through to a fresh state.
  }
  return emptyState();
}
