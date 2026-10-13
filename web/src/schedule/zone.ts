import { dateParts, formatLocalDate, type LocalDate } from './date';

// Time zone arithmetic on top of Intl, which knows the IANA database but only
// converts instants to wall clocks. The other direction is built the same way
// as in internal/slots/wallclock.go: try the offsets in effect around a wall
// time and keep the candidates that read back as that wall time.

const MINUTE = 60_000;
const HOUR = 60 * MINUTE;

const formatters = new Map<string, Intl.DateTimeFormat>();

function formatter(zone: string): Intl.DateTimeFormat {
  let f = formatters.get(zone);
  if (!f) {
    f = new Intl.DateTimeFormat('en-US', {
      timeZone: zone,
      hourCycle: 'h23',
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
    });
    formatters.set(zone, f);
  }
  return f;
}

export function isValidZone(zone: string): boolean {
  try {
    formatter(zone);
    return true;
  } catch {
    return false;
  }
}

interface WallParts {
  year: number;
  month: number;
  day: number;
  hour: number;
  minute: number;
  second: number;
}

function wallParts(instant: number, zone: string): WallParts {
  const out: Record<string, number> = {};
  for (const part of formatter(zone).formatToParts(new Date(instant))) {
    if (part.type !== 'literal') out[part.type] = Number(part.value);
  }
  return {
    year: out.year ?? 0,
    month: out.month ?? 0,
    day: out.day ?? 0,
    hour: out.hour ?? 0,
    minute: out.minute ?? 0,
    second: out.second ?? 0,
  };
}

/** The UTC offset of zone at instant, in milliseconds. */
export function offsetAt(instant: number, zone: string): number {
  const w = wallParts(instant, zone);
  const asUtc = Date.UTC(w.year, w.month - 1, w.day, w.hour, w.minute, w.second);
  return asUtc - Math.floor(instant / 1000) * 1000;
}

export interface WallClock {
  date: LocalDate;
  /** Minutes since local midnight. */
  minutes: number;
}

export function wallClock(instant: number, zone: string): WallClock {
  const w = wallParts(instant, zone);
  return { date: formatLocalDate(w.year, w.month, w.day), minutes: w.hour * 60 + w.minute };
}

export function dateOf(instant: number, zone: string): LocalDate {
  return wallClock(instant, zone).date;
}

/** The wall time as if it were UTC; only used as a starting point. */
function wallAsUtc(date: LocalDate, minutes: number): number {
  const { year, month, day } = dateParts(date);
  return Date.UTC(year, month - 1, day, 0, minutes);
}

// Real offsets lie within UTC-12 and UTC+14, so probing 15 hours either side
// covers the one transition a day can have.
function nearbyOffsets(wall: number, zone: string): number[] {
  const offsets: number[] = [];
  for (const probe of [-15 * HOUR, 15 * HOUR]) {
    const offset = offsetAt(wall + probe, zone);
    if (!offsets.includes(offset)) offsets.push(offset);
  }
  return offsets;
}

/**
 * Every instant at which a clock in zone reads the given date and time: none
 * inside a spring-forward gap, two inside a repeated fall-back hour, one
 * otherwise. Sorted ascending.
 */
export function occurrences(date: LocalDate, minutes: number, zone: string): number[] {
  const wall = wallAsUtc(date, minutes);
  const found: number[] = [];
  for (const offset of nearbyOffsets(wall, zone)) {
    const candidate = wall - offset;
    if (candidate + offsetAt(candidate, zone) === wall && !found.includes(candidate)) {
      found.push(candidate);
    }
  }
  return found.sort((a, b) => a - b);
}

/**
 * The first instant at which the local clock shows the given time or later.
 * For a repeated time this is its first occurrence; for a time skipped by a
 * spring-forward jump it is the moment of the jump.
 */
export function edge(date: LocalDate, minutes: number, zone: string): number {
  const found = occurrences(date, minutes, zone);
  if (found.length > 0) return found[0]!;
  const wall = wallAsUtc(date, minutes);
  // Read with the offset from before the jump, the wall time lands just after
  // it; the jump itself is where that offset started.
  const landed = wall - nearbyOffsets(wall, zone)[0]!;
  return offsetStart(landed, zone);
}

/** The instant the offset in effect at `instant` began, within a day. */
function offsetStart(instant: number, zone: string): number {
  const target = offsetAt(instant, zone);
  let lo = instant - 24 * HOUR;
  let hi = instant;
  if (offsetAt(lo, zone) === target) return instant;
  while (hi - lo > 1000) {
    const mid = Math.floor((lo + hi) / 2000) * 1000;
    if (offsetAt(mid, zone) === target) hi = mid;
    else lo = mid;
  }
  return hi;
}

/** The first instant of a date in zone, usually its midnight. */
export function startOfDate(date: LocalDate, zone: string): number {
  return edge(date, 0, zone);
}

/** Converts a wall time to an instant, taking its first occurrence. */
export function instantAt(date: LocalDate, minutes: number, zone: string): number {
  return edge(date, minutes, zone);
}

const RFC3339 = /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:\d{2})$/;

export function parseInstant(value: string): number {
  const ms = RFC3339.test(value) ? Date.parse(value) : Number.NaN;
  if (Number.isNaN(ms)) throw new Error(`invalid time "${value}", want RFC 3339`);
  return ms;
}

const pad = (n: number) => String(n).padStart(2, '0');

/**
 * Formats an instant as RFC 3339 with the zone's offset, as Go's
 * time.RFC3339 does: seconds, no fraction, and "Z" for a zero offset.
 */
export function formatInstant(instant: number, zone: string): string {
  const w = wallParts(instant, zone);
  const offsetMinutes = Math.round(offsetAt(instant, zone) / MINUTE);
  const sign = offsetMinutes < 0 ? '-' : '+';
  const abs = Math.abs(offsetMinutes);
  const offset = offsetMinutes === 0 ? 'Z' : `${sign}${pad(Math.floor(abs / 60))}:${pad(abs % 60)}`;
  return `${formatLocalDate(w.year, w.month, w.day)}T${pad(w.hour)}:${pad(w.minute)}:${pad(w.second)}${offset}`;
}
