import { beforeEach, describe, expect, it } from 'vitest';
import type { BookingApi } from '../api/client';
import { ApiError } from '../api/errors';
import type { Customer } from '../api/types';
import { createMockApi, type MockApi } from './mockApi';
import { memoryStorage, type StateStorage } from './storage';

// Berlin from seed/demo.json: location 2, haircut 5 (45 min + 15 buffer),
// Lena 4 (Mon-Fri 09-13, 14-18), Jonas 5 (Tue-Sat), Mila 6 (Mon, Wed, Fri
// from 12:00). One hour of notice, a 24-hour cancellation cutoff.
const BERLIN = 2;
const HAIRCUT = 5;
const BEARD = 6;
const LENA = 4;
const JONAS = 5;
const MILA = 6;
const anna: Customer = { name: 'Anna', phone: '+7 900 123-45-67', email: 'anna@example.com' };

let now: number;
let storage: StateStorage;
let api: MockApi;

function at(iso: string) {
  now = Date.parse(iso);
}

function minutes(n: number) {
  now += n * 60_000;
}

async function apiError(promise: Promise<unknown>): Promise<ApiError> {
  const err = await promise.then(
    () => undefined,
    (e: unknown) => e,
  );
  if (!(err instanceof ApiError)) throw new Error(`expected an ApiError, got ${String(err)}`);
  return err;
}

async function book(start: string, staffId: number | null = LENA, client: BookingApi = api) {
  const hold = await client.createHold({ serviceId: HAIRCUT, staffId, start });
  return client.confirmHold(hold.hold_token, anna, crypto.randomUUID());
}

async function slotsOn(date: string, staffId: number | null = LENA, serviceId = HAIRCUT) {
  const res = await api.listSlots({ locationId: BERLIN, serviceId, staffId, from: date, to: date });
  return res.days[0]!.slots.map((s) => s.slice(11, 16));
}

beforeEach(() => {
  at('2026-10-12T08:00:00Z'); // Monday 10:00 in Berlin
  storage = memoryStorage();
  api = createMockApi({ storage, clock: () => now, samples: false });
});

describe('catalog from the seed file', () => {
  it('numbers locations, services and staff the way a fresh database does', async () => {
    const locations = await api.listLocations();
    expect(locations.map((l) => [l.id, l.slug])).toEqual([
      [1, 'snipwell-moscow'],
      [2, 'snipwell-berlin'],
    ]);
    expect(locations[1]!.settings).toEqual({
      slot_step_minutes: 15,
      min_notice_minutes: 60,
      horizon_days: 60,
      hold_ttl_seconds: 600,
      cancel_cutoff_minutes: 1440,
    });
    const staff = await api.listStaff(BERLIN, HAIRCUT);
    expect(staff.map((s) => [s.id, s.name, s.service_ids])).toEqual([
      [4, 'Lena H.', [5, 8]],
      [5, 'Jonas W.', [5, 6, 7]],
      [6, 'Mila N.', [5, 6, 7]],
    ]);
  });

  it('answers 404 for an unknown location', async () => {
    expect((await apiError(api.listServices(99))).code).toBe('not_found');
  });
});

describe('slots', () => {
  it('lists a staff member\'s starts with service and buffer fitting each working interval', async () => {
    const slots = await slotsOn('2026-10-14');
    expect(slots[0]).toBe('09:00');
    expect(slots).toContain('12:00');
    expect(slots).not.toContain('12:15');
    expect(slots.at(-1)).toBe('17:00');
  });

  it('merges staff for "any available"', async () => {
    const any = await slotsOn('2026-10-14', null);
    expect(any.at(-1)).toBe('19:00');
    expect(any).toContain('12:15');
  });

  it('rejects a range over 31 days', async () => {
    const err = await apiError(
      api.listSlots({ locationId: BERLIN, serviceId: HAIRCUT, staffId: null, from: '2026-10-14', to: '2026-11-14' }),
    );
    expect([err.status, err.code]).toEqual([422, 'validation_failed']);
  });
});

describe('holds and the overlap rule', () => {
  it('lets one hold take a period and refuses an overlapping one, buffer included', async () => {
    await api.createHold({ serviceId: HAIRCUT, staffId: LENA, start: '2026-10-14T10:00:00+02:00' });
    // 10:00 is busy until 11:00 (45 minutes plus 15 of cleanup).
    const err = await apiError(api.createHold({ serviceId: HAIRCUT, staffId: LENA, start: '2026-10-14T10:45:00+02:00' }));
    expect([err.status, err.code]).toEqual([409, 'slot_taken']);
    expect(await slotsOn('2026-10-14')).not.toContain('10:45');
  });

  it('treats periods as half-open, so back-to-back bookings fit', async () => {
    await api.createHold({ serviceId: HAIRCUT, staffId: LENA, start: '2026-10-14T10:00:00+02:00' });
    await expect(
      api.createHold({ serviceId: HAIRCUT, staffId: LENA, start: '2026-10-14T11:00:00+02:00' }),
    ).resolves.toBeDefined();
  });

  it('does not let one staff member\'s booking block another', async () => {
    await api.createHold({ serviceId: HAIRCUT, staffId: JONAS, start: '2026-10-14T10:00:00+02:00' });
    await expect(
      api.createHold({ serviceId: HAIRCUT, staffId: MILA, start: '2026-10-14T13:00:00+02:00' }),
    ).resolves.toBeDefined();
  });

  it('keeps a running hold blocking and frees the period once it lapses', async () => {
    const first = await api.createHold({ serviceId: HAIRCUT, staffId: LENA, start: '2026-10-14T10:00:00+02:00' });
    expect(first.expires_at).toBe('2026-10-12T10:10:00+02:00');

    minutes(9);
    expect((await apiError(api.createHold({ serviceId: HAIRCUT, staffId: LENA, start: '2026-10-14T10:00:00+02:00' }))).code).toBe(
      'slot_taken',
    );

    minutes(1);
    expect(await slotsOn('2026-10-14')).toContain('10:00');
    const second = await api.createHold({ serviceId: HAIRCUT, staffId: LENA, start: '2026-10-14T10:00:00+02:00' });
    expect(second.booking.status).toBe('held');
  });

  it('refuses a start the schedule does not offer, with the server\'s reason', async () => {
    const offGrid = await apiError(api.createHold({ serviceId: HAIRCUT, staffId: LENA, start: '2026-10-14T10:05:00+02:00' }));
    expect([offGrid.status, offGrid.code]).toEqual([422, 'invalid_slot']);

    // Now is Monday 10:00 and Berlin asks for an hour of notice.
    const tooSoon = await apiError(api.createHold({ serviceId: HAIRCUT, staffId: LENA, start: '2026-10-12T10:45:00+02:00' }));
    expect(tooSoon.message).toBe('start is not bookable: bookings need at least 60 minutes notice');
    await expect(api.createHold({ serviceId: HAIRCUT, staffId: LENA, start: '2026-10-12T11:00:00+02:00' })).resolves.toBeDefined();

    const beyond = await apiError(api.createHold({ serviceId: HAIRCUT, staffId: LENA, start: '2026-12-14T10:00:00+01:00' }));
    expect(beyond.message).toMatch(/horizon/);
  });

  it('refuses staff who do not offer the service', async () => {
    const err = await apiError(api.createHold({ serviceId: BEARD, staffId: LENA, start: '2026-10-14T10:00:00+02:00' }));
    expect([err.status, err.code]).toEqual([422, 'invalid_staff']);
  });

  it('gives "any staff" to the least busy person that day, then the lowest id, and moves on when taken', async () => {
    await book('2026-10-14T13:00:00+02:00', JONAS);
    await book('2026-10-14T15:00:00+02:00', LENA);
    // Lena and Jonas have one booking each, Mila none.
    const first = await api.createHold({ serviceId: HAIRCUT, staffId: null, start: '2026-10-14T16:00:00+02:00' });
    expect(first.booking.staff_id).toBe(MILA);
    // Mila is now held at 16:00; Lena and Jonas tie on load, Lena has the lower id.
    const second = await api.createHold({ serviceId: HAIRCUT, staffId: null, start: '2026-10-14T16:00:00+02:00' });
    expect(second.booking.staff_id).toBe(LENA);
    const third = await api.createHold({ serviceId: HAIRCUT, staffId: null, start: '2026-10-14T16:00:00+02:00' });
    expect(third.booking.staff_id).toBe(JONAS);
    expect((await apiError(api.createHold({ serviceId: HAIRCUT, staffId: null, start: '2026-10-14T16:00:00+02:00' }))).code).toBe(
      'slot_taken',
    );
  });
});

describe('confirming a hold', () => {
  it('confirms while the hold runs and answers 410 after it ran out', async () => {
    const hold = await api.createHold({ serviceId: HAIRCUT, staffId: LENA, start: '2026-10-14T10:00:00+02:00' });
    minutes(10);
    const err = await apiError(api.confirmHold(hold.hold_token, anna, 'k1'));
    expect([err.status, err.code]).toEqual([410, 'hold_expired']);

    const fresh = await api.createHold({ serviceId: HAIRCUT, staffId: LENA, start: '2026-10-14T10:00:00+02:00' });
    const confirmed = await api.confirmHold(fresh.hold_token, anna, 'k2');
    expect(confirmed.booking).toMatchObject({
      status: 'confirmed',
      start: '2026-10-14T10:00:00+02:00',
      end: '2026-10-14T10:45:00+02:00',
      busy_until: '2026-10-14T11:00:00+02:00',
      price: { amount_minor: 3800, currency: 'EUR' },
      customer: anna,
    });
    expect(confirmed.manage_token).toMatch(/^[A-Za-z0-9_-]{43}$/);
  });

  it('replays the first result for a repeated idempotency key and refuses the key for another request', async () => {
    const hold = await api.createHold({ serviceId: HAIRCUT, staffId: LENA, start: '2026-10-14T10:00:00+02:00' });
    const first = await api.confirmHold(hold.hold_token, anna, 'same-key');
    const again = await api.confirmHold(hold.hold_token, anna, 'same-key');
    expect(again).toEqual(first);

    const other = await apiError(api.confirmHold(hold.hold_token, { ...anna, name: 'Boris' }, 'same-key'));
    expect([other.status, other.code]).toEqual([422, 'idempotency_key_reused']);

    const newKey = await apiError(api.confirmHold(hold.hold_token, anna, 'another-key'));
    expect([newKey.status, newKey.code]).toEqual([409, 'already_confirmed']);
  });

  it('checks contact details and the hold token', async () => {
    const hold = await api.createHold({ serviceId: HAIRCUT, staffId: LENA, start: '2026-10-14T10:00:00+02:00' });
    const bad = await apiError(api.confirmHold(hold.hold_token, { name: 'Anna', phone: '12' }, 'k'));
    expect([bad.status, bad.code, bad.message]).toEqual([422, 'validation_failed', 'customer.phone: must have 7 to 15 digits']);
    expect((await apiError(api.confirmHold('no-such-token', anna, 'k'))).code).toBe('hold_not_found');
  });
});

describe('releasing a hold', () => {
  it('frees the period at once and drops the hold from the day view', async () => {
    const hold = await api.createHold({ serviceId: HAIRCUT, staffId: LENA, start: '2026-10-14T10:00:00+02:00' });
    expect(await slotsOn('2026-10-14')).not.toContain('10:15');

    await api.releaseHold(hold.booking.id, hold.hold_token);
    expect(await slotsOn('2026-10-14')).toEqual(expect.arrayContaining(['10:00', '10:15']));
    const day = await api.admin('t').listBookings(BERLIN, '2026-10-14');
    expect(day.bookings.map((b) => b.id)).not.toContain(hold.booking.id);
    const next = await api.createHold({ serviceId: HAIRCUT, staffId: LENA, start: '2026-10-14T10:15:00+02:00' });
    expect(next.booking.status).toBe('held');
  });

  it('succeeds again for a hold that is already released', async () => {
    const hold = await api.createHold({ serviceId: HAIRCUT, staffId: LENA, start: '2026-10-14T10:00:00+02:00' });
    await api.releaseHold(hold.booking.id, hold.hold_token);
    await expect(api.releaseHold(hold.booking.id, hold.hold_token)).resolves.toBeUndefined();
    minutes(60);
    await expect(api.releaseHold(hold.booking.id, hold.hold_token)).resolves.toBeUndefined();
  });

  it('needs the hold\'s own token and answers 404 like a missing hold otherwise', async () => {
    const hold = await api.createHold({ serviceId: HAIRCUT, staffId: LENA, start: '2026-10-14T10:00:00+02:00' });
    const other = await api.createHold({ serviceId: HAIRCUT, staffId: JONAS, start: '2026-10-14T10:00:00+02:00' });
    for (const [id, token] of [
      [hold.booking.id, 'not-the-token'],
      [hold.booking.id, other.hold_token],
      ['no-such-hold', hold.hold_token],
    ] as const) {
      const err = await apiError(api.releaseHold(id, token));
      expect([err.status, err.code]).toEqual([404, 'hold_not_found']);
    }
    const empty = await apiError(api.releaseHold(hold.booking.id, ''));
    expect([empty.status, empty.code, empty.message]).toEqual([422, 'validation_failed', 'hold_token: is required']);
    expect(await slotsOn('2026-10-14')).not.toContain('10:00');
  });

  it('refuses a confirmed or expired hold with 409 and leaves it as it was', async () => {
    const hold = await api.createHold({ serviceId: HAIRCUT, staffId: LENA, start: '2026-10-14T10:00:00+02:00' });
    const { booking, manage_token } = await api.confirmHold(hold.hold_token, anna, 'k');
    const confirmed = await apiError(api.releaseHold(hold.booking.id, hold.hold_token));
    expect([confirmed.status, confirmed.code]).toEqual([409, 'already_confirmed']);
    expect((await api.getBooking(booking.id, manage_token)).status).toBe('confirmed');

    const lapsed = await api.createHold({ serviceId: HAIRCUT, staffId: JONAS, start: '2026-10-14T10:00:00+02:00' });
    minutes(10);
    const expired = await apiError(api.releaseHold(lapsed.booking.id, lapsed.hold_token));
    expect([expired.status, expired.code]).toEqual([409, 'hold_expired']);
  });

  it('answers 410 to a confirm of a released hold', async () => {
    const hold = await api.createHold({ serviceId: HAIRCUT, staffId: LENA, start: '2026-10-14T10:00:00+02:00' });
    await api.releaseHold(hold.booking.id, hold.hold_token);
    const err = await apiError(api.confirmHold(hold.hold_token, anna, 'k'));
    expect([err.status, err.code]).toEqual([410, 'hold_expired']);
  });
});

describe('notification outbox', () => {
  it('queues the confirmation and the reminders that are still ahead, and delivers them when due', async () => {
    const { booking } = await book('2026-10-13T09:00:00+02:00');
    const token = api.admin('demo');
    let rows = await token.listNotifications({ bookingId: booking.id });
    // The 24-hour reminder would be due before the booking was made.
    expect(rows.map((r) => [r.kind, r.send_at])).toEqual([
      ['reminder_2h', '2026-10-13T05:00:00Z'],
      ['confirmation', '2026-10-12T08:00:00Z'],
    ]);

    expect(rows.find((r) => r.kind === 'confirmation')?.status).toBe('pending');

    minutes(1);
    rows = await token.listNotifications({ bookingId: booking.id });
    // Like the server's polling worker, the mock sends a few seconds after the due time.
    expect(rows.find((r) => r.kind === 'confirmation')).toMatchObject({
      status: 'sent',
      attempts: 1,
      sent_at: '2026-10-12T08:00:03Z',
    });
    expect(rows.find((r) => r.kind === 'reminder_2h')?.status).toBe('pending');
  });
});

describe('manage link', () => {
  it('shows a booking only with its manage token', async () => {
    const { booking, manage_token } = await book('2026-10-14T10:00:00+02:00');
    await expect(api.getBooking(booking.id, manage_token)).resolves.toMatchObject({ id: booking.id });
    expect((await apiError(api.getBooking(booking.id, 'wrong'))).code).toBe('not_found');
  });

  it('cancels before the cutoff, voids pending reminders and queues a cancellation notice', async () => {
    const { booking, manage_token } = await book('2026-10-14T10:00:00+02:00');
    const cancelled = await api.cancelBooking(booking.id, manage_token);
    expect(cancelled.status).toBe('cancelled');
    expect(await slotsOn('2026-10-14')).toContain('10:00');

    const rows = await api.admin('demo').listNotifications({ bookingId: booking.id });
    expect(rows.find((r) => r.kind === 'reminder_24h')?.status).toBe('voided');
    expect(rows.find((r) => r.kind === 'cancellation')?.status).toBe('pending');

    expect((await apiError(api.cancelBooking(booking.id, manage_token))).code).toBe('not_active');
  });

  it('refuses to cancel or move a booking after the cutoff', async () => {
    const { booking, manage_token } = await book('2026-10-14T10:00:00+02:00');
    at('2026-10-13T08:00:01Z'); // less than 24 hours before the start
    const err = await apiError(api.cancelBooking(booking.id, manage_token));
    expect([err.status, err.code]).toEqual([409, 'too_late']);
    expect((await apiError(api.rescheduleBooking(booking.id, manage_token, '2026-10-15T10:00:00+02:00'))).code).toBe('too_late');
  });

  it('moves a booking onto a period overlapping its own and keeps the manage token', async () => {
    const { booking, manage_token } = await book('2026-10-14T10:00:00+02:00');
    const moved = await api.rescheduleBooking(booking.id, manage_token, '2026-10-14T10:15:00+02:00');
    expect(moved).toMatchObject({ status: 'confirmed', start: '2026-10-14T10:15:00+02:00', rescheduled_from: booking.id });
    expect(moved.id).not.toBe(booking.id);
    expect(moved.rescheduled_to).toBeNull();
    expect(await api.getBooking(booking.id, manage_token)).toMatchObject({ status: 'cancelled', rescheduled_to: moved.id });
    await expect(api.getBooking(moved.id, manage_token)).resolves.toMatchObject({ customer: anna });
  });

  it('leaves the booking untouched when the new time is taken', async () => {
    const { booking, manage_token } = await book('2026-10-14T10:00:00+02:00');
    await book('2026-10-14T14:00:00+02:00');
    const err = await apiError(api.rescheduleBooking(booking.id, manage_token, '2026-10-14T14:30:00+02:00'));
    expect(err.code).toBe('slot_taken');
    expect((await api.getBooking(booking.id, manage_token)).status).toBe('confirmed');
    const rows = await api.admin('demo').listNotifications({ bookingId: booking.id });
    expect(rows.every((r) => r.status !== 'voided')).toBe(true);
  });
});

describe('admin', () => {
  it('blocks time against the same overlap rule and hides it from the slots', async () => {
    const admin = api.admin('anything');
    await book('2026-10-14T10:00:00+02:00');
    const clash = await apiError(
      admin.createBlock({ staffId: LENA, start: '2026-10-14T10:30:00+02:00', end: '2026-10-14T12:00:00+02:00', note: 'training' }),
    );
    expect(clash.code).toBe('slot_taken');

    const block = await admin.createBlock({
      staffId: LENA,
      start: '2026-10-14T11:00:00+02:00',
      end: '2026-10-14T12:00:00+02:00',
      note: 'training',
    });
    expect(block).toMatchObject({ kind: 'block', status: 'confirmed', service_id: null, price: null, note: 'training' });
    expect(await slotsOn('2026-10-14')).not.toContain('11:00');

    const day = await admin.listBookings(BERLIN, '2026-10-14');
    expect(day.bookings.map((b) => [b.kind, b.start])).toEqual([
      ['appointment', '2026-10-14T10:00:00+02:00'],
      ['block', '2026-10-14T11:00:00+02:00'],
    ]);

    await admin.deleteBlock(block.id);
    expect(await slotsOn('2026-10-14')).toContain('11:00');
    expect((await apiError(admin.deleteBlock(block.id))).code).toBe('not_found');
  });

  it('validates block times', async () => {
    const err = await apiError(
      api.admin('t').createBlock({ staffId: LENA, start: '2026-10-14T12:00:00+02:00', end: '2026-10-14T11:00:00+02:00', note: '' }),
    );
    expect([err.status, err.code]).toEqual([422, 'validation_failed']);
  });

  it('cancels a confirmed booking regardless of the cutoff', async () => {
    const { booking } = await book('2026-10-14T10:00:00+02:00');
    at('2026-10-14T07:00:00Z');
    await expect(api.admin('t').cancelBooking(booking.id)).resolves.toMatchObject({ status: 'cancelled' });
  });
});

describe('state shared between tabs', () => {
  it('lets exactly one of two tabs hold the same slot', async () => {
    const otherTab = createMockApi({ storage, clock: () => now, samples: false });
    const start = '2026-10-14T10:00:00+02:00';
    const results = await Promise.allSettled([
      api.createHold({ serviceId: HAIRCUT, staffId: LENA, start }),
      otherTab.createHold({ serviceId: HAIRCUT, staffId: LENA, start }),
    ]);
    expect(results.map((r) => r.status).sort()).toEqual(['fulfilled', 'rejected']);
    const rejected = results.find((r) => r.status === 'rejected') as PromiseRejectedResult;
    expect(rejected.reason).toMatchObject({ code: 'slot_taken' });
  });

  it('books a slot for someone else so the conflict can be shown', async () => {
    const start = '2026-10-14T10:00:00+02:00';
    await api.demo.bookForSomeoneElse({ serviceId: HAIRCUT, staffId: LENA, start });
    expect((await apiError(api.createHold({ serviceId: HAIRCUT, staffId: LENA, start }))).code).toBe('slot_taken');
  });

  it('takes an "any staff" slot from everyone free at that time', async () => {
    const start = '2026-10-14T13:00:00+02:00';
    await api.demo.bookForSomeoneElse({ serviceId: HAIRCUT, staffId: null, start });
    expect(await slotsOn('2026-10-14', null)).not.toContain('13:00');
    expect((await apiError(api.createHold({ serviceId: HAIRCUT, staffId: null, start }))).code).toBe('slot_taken');
  });

  it('starts a fresh demo with sample bookings that follow the rules', async () => {
    const seeded = createMockApi({ storage: memoryStorage(), clock: () => now });
    const day = await seeded.admin('t').listBookings(BERLIN, '2026-10-13');
    expect(day.bookings.length).toBeGreaterThan(0);
    expect(day.bookings.every((b) => b.status === 'confirmed' || b.kind === 'block')).toBe(true);
  });
});
