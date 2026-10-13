// Wire types of the Slotbook API, field for field as docs/api.md describes
// them. Times are RFC 3339 strings in the location's zone, except in the
// outbox listing, which uses UTC.

export interface Money {
  amount_minor: number;
  currency: string;
}

export interface LocationSettings {
  slot_step_minutes: number;
  min_notice_minutes: number;
  horizon_days: number;
  hold_ttl_seconds: number;
  cancel_cutoff_minutes: number;
}

export interface Location {
  id: number;
  slug: string;
  name: string;
  time_zone: string;
  address: string;
  settings: LocationSettings;
}

export interface Service {
  id: number;
  location_id: number;
  name: string;
  duration_minutes: number;
  buffer_after_minutes: number;
  price: Money;
}

export interface Staff {
  id: number;
  location_id: number;
  name: string;
  service_ids: number[];
}

export interface SlotDayView {
  date: string;
  slots: string[];
}

export interface Slots {
  location_id: number;
  service_id: number;
  staff_id: number | null;
  time_zone: string;
  days: SlotDayView[];
}

export type BookingKind = 'appointment' | 'block';
export type BookingStatus = 'held' | 'confirmed' | 'cancelled' | 'expired';

export interface Customer {
  name: string;
  phone: string;
  email?: string;
  telegram_chat_id?: number;
}

export interface Booking {
  id: string;
  kind: BookingKind;
  status: BookingStatus;
  location_id: number;
  staff_id: number;
  service_id: number | null;
  start: string;
  end: string;
  busy_until: string;
  time_zone: string;
  price: Money | null;
  hold_expires_at: string | null;
  customer: Customer | null;
  note: string;
  rescheduled_from: string | null;
  created_at: string;
  confirmed_at: string | null;
  cancelled_at: string | null;
}

export interface Hold {
  hold_token: string;
  expires_at: string;
  booking: Booking;
}

export interface Confirmation {
  booking: Booking;
  manage_token: string;
}

export interface DayBookings {
  location_id: number;
  date: string;
  time_zone: string;
  bookings: Booking[];
}

export type NotificationKind = 'confirmation' | 'reminder_24h' | 'reminder_2h' | 'cancellation';
export type NotificationStatus = 'pending' | 'sent' | 'failed' | 'voided' | 'skipped';

export interface OutboxNotification {
  id: number;
  booking_id: string;
  kind: NotificationKind;
  status: NotificationStatus;
  send_at: string;
  next_attempt_at: string;
  attempts: number;
  last_error?: string;
  created_at: string;
  sent_at?: string;
}
