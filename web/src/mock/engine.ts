import type { BlockRequest, HoldRequest, NotificationFilter, SlotQuery } from '../api/client';
import { ApiError } from '../api/errors';
import type {
  Booking,
  Confirmation,
  Customer,
  DayBookings,
  Hold,
  Location,
  NotificationKind,
  OutboxNotification,
  Service,
  Slots,
  Staff,
} from '../api/types';
import { normalizeCustomer, serverMessage } from '../domain/customer';
import { addDays, daysBetween, parseLocalDate, type LocalDate } from '../schedule/date';
import { generateSlots, offersStart, overlapsBusy, type BusyInterval, type SlotRequest } from '../schedule/generate';
import { dateOf, formatInstant, parseInstant, startOfDate } from '../schedule/zone';
import { randomId } from '../lib/randomId';
import type { Catalog } from './catalog';
import type { MockState, StoredBooking } from './state';

// The server's booking rules (internal/booking and internal/notify) applied
// to the in-browser state. Each method is one API call; a thrown ApiError
// carries the status and code the server would answer with.

const MINUTE = 60_000;
const HOUR = 60 * MINUTE;
const MAX_AVAILABILITY_DAYS = 31;
const MAX_BLOCK_MS = 31 * 24 * HOUR;
const IDEMPOTENCY_KEY_TTL_MS = 24 * HOUR;
const REMINDERS: [NotificationKind, number][] = [
  ['reminder_24h', 24 * HOUR],
  ['reminder_2h', 2 * HOUR],
];

/**
 * The server's worker polls the outbox every few seconds; the mock delivers
 * a row this long after it falls due.
 */
export const DELIVERY_LAG_MS = 3_000;

const badRequest = (message: string) => new ApiError(400, 'bad_request', message);
const notFound = (message: string) => new ApiError(404, 'not_found', message);
const validation = (message: string) => new ApiError(422, 'validation_failed', message);
const slotTaken = () => new ApiError(409, 'slot_taken', 'slot is already taken');

function parseDateParam(name: string, value: string): LocalDate {
  try {
    return parseLocalDate(value);
  } catch {
    throw badRequest(`${name}: invalid date "${value}", want YYYY-MM-DD`);
  }
}

function parseTimeParam(name: string, value: string): number {
  try {
    return parseInstant(value);
  } catch {
    throw badRequest(`${name} must be an RFC 3339 time`);
  }
}

function randomToken(): string {
  const bytes = crypto.getRandomValues(new Uint8Array(32));
  return btoa(String.fromCharCode(...bytes))
    .replace(/\+/g, '-')
    .replace(/\//g, '_')
    .replace(/=+$/, '');
}

function validIdempotencyKey(key: string): boolean {
  return key.length > 0 && key.length <= 255 && /^[\x21-\x7e]+$/.test(key);
}

export class Engine {
  private readonly catalog: Catalog;
  private readonly state: MockState;
  private readonly now: number;

  constructor(catalog: Catalog, state: MockState, now: number) {
    this.catalog = catalog;
    this.state = state;
    this.now = now;
  }

  // Catalog lookups, answering like the server's catalog handlers.

  location(id: number): Location {
    const loc = this.catalog.locations.find((l) => l.id === id);
    if (!loc) throw notFound('booking not found');
    return loc;
  }

  services(locationId: number): Service[] {
    this.location(locationId);
    return this.catalog.services.filter((s) => s.location_id === locationId);
  }

  staff(locationId: number, serviceId?: number): Staff[] {
    this.location(locationId);
    return this.catalog.staff.filter(
      (s) => s.location_id === locationId && (serviceId === undefined || s.service_ids.includes(serviceId)),
    );
  }

  private service(id: number): Service {
    const svc = this.catalog.services.find((s) => s.id === id);
    if (!svc) throw notFound('service not found');
    return svc;
  }

  /** The staff member asked for, or everyone who offers the service when staffId is null. */
  private candidates(svc: Service, staffId: number | null): Staff[] {
    if (staffId === null) return this.staff(svc.location_id, svc.id);
    const member = this.catalog.staff.find((s) => s.id === staffId);
    if (!member || member.location_id !== svc.location_id || !member.service_ids.includes(svc.id)) {
      throw new ApiError(422, 'invalid_staff', 'staff member does not offer this service');
    }
    return [member];
  }

  private slotRequest(loc: Location, svc: Service, staffId: number, busy: BusyInterval[], from: LocalDate, to: LocalDate): SlotRequest {
    const schedule = this.catalog.schedules.get(staffId)!;
    return {
      zone: loc.time_zone,
      weekly: schedule.weekly,
      overrides: schedule.overrides,
      durationMinutes: svc.duration_minutes,
      bufferMinutes: svc.buffer_after_minutes,
      stepMinutes: loc.settings.slot_step_minutes,
      minNoticeMinutes: loc.settings.min_notice_minutes,
      horizonDays: loc.settings.horizon_days,
      busy,
      now: this.now,
      from,
      to,
    };
  }

  // The work the server does on timers: the hold sweeper, the pruning of
  // idempotency keys and the outbox worker. It runs before every call, so a
  // lapsed hold never reaches the overlap check below; the server gets the
  // same effect by expiring stale holds in the transaction that inserts.

  sweep(): void {
    for (const b of this.state.bookings) {
      if (b.status === 'held' && b.holdExpiresAt! <= this.now) b.status = 'expired';
    }
    for (const [key, stored] of Object.entries(this.state.idempotencyKeys)) {
      if (stored.createdAt < this.now - IDEMPOTENCY_KEY_TTL_MS) delete this.state.idempotencyKeys[key];
    }
    this.deliverDue();
  }

  private deliverDue(): void {
    for (const n of this.state.notifications) {
      const deliveredAt = n.nextAttemptAt + DELIVERY_LAG_MS;
      if (n.status !== 'pending' || deliveredAt > this.now) continue;
      const b = this.state.bookings.find((x) => x.id === n.bookingId);
      n.attempts += 1;
      const stale = !b
        ? 'booking is gone'
        : n.kind === 'cancellation'
          ? b.status !== 'cancelled' && `booking is ${b.status}`
          : b.status !== 'confirmed'
            ? `booking is ${b.status}`
            : b.start <= deliveredAt && 'appointment already started';
      if (stale) {
        n.status = 'skipped';
        n.lastError = stale;
      } else {
        n.status = 'sent';
        n.sentAt = deliveredAt;
        n.lastError = null;
      }
    }
  }

  // The exclusion constraint: active rows of one staff member never overlap.

  private busyFor(staffId: number, from: number, to: number): BusyInterval[] {
    return this.state.bookings
      .filter(
        (b) =>
          b.staffId === staffId &&
          (b.status === 'confirmed' || (b.status === 'held' && b.holdExpiresAt! > this.now)) &&
          b.start < to &&
          from < b.busyUntil,
      )
      .map((b) => ({ start: b.start, end: b.busyUntil }));
  }

  private insert(row: StoredBooking): StoredBooking {
    const active = (b: StoredBooking) => b.status === 'held' || b.status === 'confirmed';
    if (active(row)) {
      const clash = this.state.bookings.some(
        (b) => b.staffId === row.staffId && active(b) && overlapsBusy([{ start: b.start, end: b.busyUntil }], row.start, row.busyUntil),
      );
      if (clash) throw slotTaken();
    }
    this.state.bookings.push(row);
    return row;
  }

  // Availability and holds.

  listSlots(q: SlotQuery): Slots {
    const loc = this.location(q.locationId);
    const from = parseDateParam('from', q.from);
    const to = parseDateParam('to', q.to);
    if (to < from || daysBetween(from, to) >= MAX_AVAILABILITY_DAYS) {
      throw validation(`to: the range must cover 1 to ${MAX_AVAILABILITY_DAYS} days`);
    }
    const svc = this.service(q.serviceId);
    if (svc.location_id !== loc.id) throw notFound('service not found');
    // Busy time from the neighbouring days matters too: a buffer can run past midnight.
    const windowStart = startOfDate(addDays(from, -1), loc.time_zone);
    const windowEnd = startOfDate(addDays(to, 2), loc.time_zone);

    const merged = new Map<LocalDate, Set<number>>();
    for (const member of this.candidates(svc, q.staffId)) {
      const busy = this.busyFor(member.id, windowStart, windowEnd);
      for (const day of generateSlots(this.slotRequest(loc, svc, member.id, busy, from, to))) {
        const set = merged.get(day.date) ?? new Set<number>();
        day.slots.forEach((s) => set.add(s));
        merged.set(day.date, set);
      }
    }
    const days = [];
    for (let d = from; d <= to; d = addDays(d, 1)) {
      const starts = [...(merged.get(d) ?? [])].sort((a, b) => a - b);
      days.push({ date: d, slots: starts.map((s) => formatInstant(s, loc.time_zone)) });
    }
    return { location_id: loc.id, service_id: svc.id, staff_id: q.staffId, time_zone: loc.time_zone, days };
  }

  hold(req: HoldRequest): Hold {
    const start = parseTimeParam('start', req.start);
    const svc = this.service(req.serviceId);
    const loc = this.location(svc.location_id);
    let eligible = this.candidates(svc, req.staffId).filter((member) =>
      offersStart(this.slotRequest(loc, svc, member.id, [], dateOf(start, loc.time_zone), dateOf(start, loc.time_zone)), start),
    );
    if (eligible.length === 0) throw this.whyNotBookable(loc, start);
    if (req.staffId === null && eligible.length > 1) {
      eligible = this.orderByLoad(eligible, dateOf(start, loc.time_zone), loc.time_zone);
    }

    const holdToken = randomToken();
    const end = start + svc.duration_minutes * MINUTE;
    for (const member of eligible) {
      try {
        const row = this.insert({
          id: randomId(),
          kind: 'appointment',
          status: 'held',
          locationId: loc.id,
          staffId: member.id,
          serviceId: svc.id,
          start,
          end,
          busyUntil: end + svc.buffer_after_minutes * MINUTE,
          price: { ...svc.price },
          holdToken,
          holdExpiresAt: this.now + loc.settings.hold_ttl_seconds * 1000,
          manageToken: null,
          customer: null,
          note: '',
          rescheduledFrom: null,
          createdAt: this.now,
          confirmedAt: null,
          cancelledAt: null,
        });
        return {
          hold_token: holdToken,
          expires_at: formatInstant(row.holdExpiresAt!, loc.time_zone),
          booking: this.view(row),
        };
      } catch (err) {
        // Lost to another booking; with any staff, try the next person.
        if (err instanceof ApiError && err.code === 'slot_taken') continue;
        throw err;
      }
    }
    throw slotTaken();
  }

  /** Fewest confirmed appointments on that date first, then the lowest id. */
  private orderByLoad(staff: Staff[], date: LocalDate, zone: string): Staff[] {
    const dayStart = startOfDate(date, zone);
    const dayEnd = startOfDate(addDays(date, 1), zone);
    const load = (id: number) =>
      this.state.bookings.filter(
        (b) => b.staffId === id && b.kind === 'appointment' && b.status === 'confirmed' && b.start >= dayStart && b.start < dayEnd,
      ).length;
    return [...staff].sort((a, b) => load(a.id) - load(b.id) || a.id - b.id);
  }

  private whyNotBookable(loc: Location, start: number): ApiError {
    const { min_notice_minutes: notice, horizon_days: horizon } = loc.settings;
    let reason = 'not an available start time for this service and staff';
    if (start < this.now + notice * MINUTE) {
      reason = `bookings need at least ${notice} minutes notice`;
    } else if (dateOf(start, loc.time_zone) > addDays(dateOf(this.now, loc.time_zone), horizon)) {
      reason = `bookings open ${horizon} days ahead (the booking horizon)`;
    }
    return new ApiError(422, 'invalid_slot', `start is not bookable: ${reason}`);
  }

  // Confirmation and the outbox.

  confirm(holdToken: string, input: Customer, idempotencyKey: string): Confirmation {
    const normalized = normalizeCustomer(input);
    if (!normalized.ok) {
      const first = normalized.problems[0]!;
      throw validation(serverMessage(first.field, first.problem));
    }
    if (!holdToken) throw validation('hold_token: is required');
    if (idempotencyKey && !validIdempotencyKey(idempotencyKey)) {
      throw validation('Idempotency-Key: idempotency key must be printable ASCII without spaces');
    }
    const fingerprint = JSON.stringify({ hold_token: holdToken, customer: normalized.customer });
    const stored = idempotencyKey ? this.state.idempotencyKeys[idempotencyKey] : undefined;
    if (stored) {
      if (stored.fingerprint !== fingerprint) {
        throw new ApiError(422, 'idempotency_key_reused', 'idempotency key was used with a different request');
      }
      return structuredClone(stored.response);
    }

    const row = this.state.bookings.find((b) => b.holdToken === holdToken);
    if (!row) throw new ApiError(404, 'hold_not_found', 'hold not found');
    if (row.status !== 'held' || row.holdExpiresAt! <= this.now) {
      if (row.status === 'confirmed' || row.status === 'cancelled') {
        throw new ApiError(409, 'already_confirmed', 'hold is already confirmed');
      }
      throw new ApiError(410, 'hold_expired', 'hold has expired');
    }
    const manageToken = randomToken();
    Object.assign(row, {
      status: 'confirmed',
      customer: normalized.customer,
      manageToken,
      confirmedAt: this.now,
    } satisfies Partial<StoredBooking>);
    this.enqueueForConfirmed(row);
    const result: Confirmation = { booking: this.view(row), manage_token: manageToken };
    if (idempotencyKey) {
      this.state.idempotencyKeys[idempotencyKey] = { fingerprint, response: result, createdAt: this.now };
    }
    return result;
  }

  private enqueue(bookingId: string, kind: NotificationKind, sendAt: number): void {
    this.state.notifications.push({
      id: this.state.nextNotificationId++,
      bookingId,
      kind,
      status: 'pending',
      sendAt,
      nextAttemptAt: sendAt,
      attempts: 0,
      lastError: null,
      createdAt: this.now,
      sentAt: null,
    });
  }

  /** The confirmation now, and each reminder whose time is still ahead. */
  private enqueueForConfirmed(b: StoredBooking): void {
    this.enqueue(b.id, 'confirmation', this.now);
    for (const [kind, before] of REMINDERS) {
      if (b.start - before > this.now) this.enqueue(b.id, kind, b.start - before);
    }
  }

  private voidPending(bookingId: string): void {
    for (const n of this.state.notifications) {
      if (n.bookingId === bookingId && n.status === 'pending') n.status = 'voided';
    }
  }

  private markCancelled(b: StoredBooking): void {
    b.status = 'cancelled';
    b.cancelledAt = this.now;
    this.voidPending(b.id);
  }

  // The manage link.

  private appointment(id: string): StoredBooking {
    const b = this.state.bookings.find((x) => x.id === id && x.kind === 'appointment');
    if (!b) throw notFound('booking not found');
    return b;
  }

  /** A wrong token looks exactly like a missing booking. */
  private owned(id: string, manageToken: string): StoredBooking {
    const b = this.appointment(id);
    if (!manageToken || b.manageToken !== manageToken) throw notFound('booking not found');
    return b;
  }

  private checkCutoff(b: StoredBooking): void {
    const cutoff = this.location(b.locationId).settings.cancel_cutoff_minutes * MINUTE;
    if (this.now > b.start - cutoff) throw new ApiError(409, 'too_late', 'too late to change this booking');
  }

  private requireConfirmed(b: StoredBooking): void {
    if (b.status !== 'confirmed') throw new ApiError(409, 'not_active', 'booking is not active');
  }

  get(id: string, manageToken: string): Booking {
    return this.view(this.owned(id, manageToken));
  }

  cancel(id: string, manageToken: string): Booking {
    const b = this.owned(id, manageToken);
    this.requireConfirmed(b);
    this.checkCutoff(b);
    this.markCancelled(b);
    this.enqueue(b.id, 'cancellation', this.now);
    return this.view(b);
  }

  /**
   * Moves a booking to a new start with the same staff member. The old row
   * leaves the active set before the new one goes in, so a move onto a period
   * overlapping the old one does not conflict with itself.
   */
  reschedule(id: string, manageToken: string, newStart: string): Booking {
    const old = this.owned(id, manageToken);
    this.requireConfirmed(old);
    this.checkCutoff(old);
    const start = parseTimeParam('start', newStart);
    const svc = this.service(old.serviceId!);
    const loc = this.location(old.locationId);
    const date = dateOf(start, loc.time_zone);
    if (!offersStart(this.slotRequest(loc, svc, old.staffId, [], date, date), start)) {
      throw this.whyNotBookable(loc, start);
    }
    this.markCancelled(old);
    const end = start + svc.duration_minutes * MINUTE;
    const moved = this.insert({
      ...old,
      id: randomId(),
      status: 'confirmed',
      start,
      end,
      busyUntil: end + svc.buffer_after_minutes * MINUTE,
      holdToken: null,
      holdExpiresAt: null,
      rescheduledFrom: old.id,
      createdAt: this.now,
      confirmedAt: this.now,
      cancelledAt: null,
    });
    this.enqueueForConfirmed(moved);
    return this.view(moved);
  }

  // Admin.

  listDay(locationId: number, date: string): DayBookings {
    const loc = this.location(locationId);
    const day = parseDateParam('date', date);
    const dayStart = startOfDate(day, loc.time_zone);
    const dayEnd = startOfDate(addDays(day, 1), loc.time_zone);
    const rows = this.state.bookings
      .filter((b) => b.locationId === loc.id && b.start < dayEnd && b.busyUntil > dayStart && b.status !== 'expired')
      .sort((a, b) => a.start - b.start || a.staffId - b.staffId || a.createdAt - b.createdAt);
    return { location_id: loc.id, date: day, time_zone: loc.time_zone, bookings: rows.map((b) => this.view(b)) };
  }

  adminCancel(id: string): Booking {
    const b = this.appointment(id);
    this.requireConfirmed(b);
    this.markCancelled(b);
    this.enqueue(b.id, 'cancellation', this.now);
    return this.view(b);
  }

  createBlock(req: BlockRequest): Booking {
    const start = parseTimeParam('start', req.start);
    const end = parseTimeParam('end', req.end);
    if (end <= start) throw validation('end: must be after start');
    if (end - start > MAX_BLOCK_MS) throw validation('end: a block can last at most 31 days');
    const member = this.catalog.staff.find((s) => s.id === req.staffId);
    if (!member) throw new ApiError(422, 'invalid_staff', 'staff member does not offer this service');
    const row = this.insert({
      id: randomId(),
      kind: 'block',
      status: 'confirmed',
      locationId: member.location_id,
      staffId: member.id,
      serviceId: null,
      start,
      end,
      busyUntil: end,
      price: null,
      holdToken: null,
      holdExpiresAt: null,
      manageToken: null,
      customer: null,
      note: req.note,
      rescheduledFrom: null,
      createdAt: this.now,
      confirmedAt: null,
      cancelledAt: null,
    });
    return this.view(row);
  }

  deleteBlock(id: string): void {
    const index = this.state.bookings.findIndex((b) => b.id === id && b.kind === 'block');
    if (index < 0) throw notFound('booking not found');
    this.state.bookings.splice(index, 1);
  }

  notifications(filter: NotificationFilter = {}): OutboxNotification[] {
    const limit = filter.limit ?? 100;
    if (!Number.isInteger(limit) || limit < 1 || limit > 500) throw badRequest('limit must be between 1 and 500');
    const locationOf = new Map(this.state.bookings.map((b) => [b.id, b.locationId]));
    const utc = (t: number) => formatInstant(t, 'UTC');
    return this.state.notifications
      .filter(
        (n) =>
          (filter.locationId === undefined || locationOf.get(n.bookingId) === filter.locationId) &&
          (filter.bookingId === undefined || n.bookingId === filter.bookingId) &&
          (filter.status === undefined || n.status === filter.status),
      )
      .sort((a, b) => b.sendAt - a.sendAt || b.id - a.id)
      .slice(0, limit)
      .map((n) => ({
        id: n.id,
        booking_id: n.bookingId,
        kind: n.kind,
        status: n.status,
        send_at: utc(n.sendAt),
        next_attempt_at: utc(n.nextAttemptAt),
        attempts: n.attempts,
        ...(n.lastError ? { last_error: n.lastError } : {}),
        created_at: utc(n.createdAt),
        ...(n.sentAt !== null ? { sent_at: utc(n.sentAt) } : {}),
      }));
  }

  private view(b: StoredBooking): Booking {
    const zone = this.location(b.locationId).time_zone;
    const at = (t: number) => formatInstant(t, zone);
    const maybe = (t: number | null) => (t === null ? null : at(t));
    return {
      id: b.id,
      kind: b.kind,
      status: b.status,
      location_id: b.locationId,
      staff_id: b.staffId,
      service_id: b.serviceId,
      start: at(b.start),
      end: at(b.end),
      busy_until: at(b.busyUntil),
      time_zone: zone,
      price: b.price ? { ...b.price } : null,
      hold_expires_at: maybe(b.holdExpiresAt),
      customer: b.customer ? { ...b.customer } : null,
      note: b.note,
      rescheduled_from: b.rescheduledFrom,
      rescheduled_to: this.state.bookings.find((next) => next.rescheduledFrom === b.id)?.id ?? null,
      created_at: at(b.createdAt),
      confirmed_at: maybe(b.confirmedAt),
      cancelled_at: maybe(b.cancelledAt),
    };
  }
}
