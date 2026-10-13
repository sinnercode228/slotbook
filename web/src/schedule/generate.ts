import { addDays, dateRange, weekday, type LocalDate } from './date';
import type { Interval, WeeklyHours } from './timeOfDay';
import { dateOf, edge, occurrences } from './zone';

// A port of internal/slots.Generate. Both are held to the vectors in
// testdata/slots, so a change in one has to be made in the other.

const MINUTE = 60_000;

/** A half-open interval [start, end) in epoch milliseconds. */
export interface BusyInterval {
  start: number;
  end: number;
}

export interface SlotRequest {
  zone: string;
  weekly: WeeklyHours;
  /** Replace the weekly hours on their date; an empty list closes the day. */
  overrides: ReadonlyMap<LocalDate, Interval[]>;
  durationMinutes: number;
  /** Cleanup time after the service; it has to fit just like the service. */
  bufferMinutes: number;
  stepMinutes: number;
  minNoticeMinutes: number;
  /** Today plus this many days is the last bookable date. */
  horizonDays: number;
  busy: readonly BusyInterval[];
  now: number;
  from: LocalDate;
  to: LocalDate;
}

export interface SlotDay {
  date: LocalDate;
  /** Starts in epoch milliseconds, ascending. */
  slots: number[];
}

function validate(r: SlotRequest): void {
  const problem =
    (r.durationMinutes <= 0 && 'duration must be positive') ||
    (r.bufferMinutes < 0 && 'buffer must not be negative') ||
    ((r.stepMinutes < 1 || !Number.isInteger(r.stepMinutes)) && 'step must be a positive whole number of minutes') ||
    (r.minNoticeMinutes < 0 && 'minimum notice must not be negative') ||
    (r.horizonDays < 0 && 'horizon must not be negative') ||
    (r.to < r.from && 'range ends before it starts');
  if (problem) throw new Error(`slots: ${problem}`);
}

/** One SlotDay per date from r.from to r.to inclusive. */
export function generateSlots(r: SlotRequest): SlotDay[] {
  validate(r);
  const today = dateOf(r.now, r.zone);
  const lastBookable = addDays(today, r.horizonDays);
  return dateRange(r.from, r.to).map((date) => ({
    date,
    slots: date >= today && date <= lastBookable ? startsOn(r, date) : [],
  }));
}

/** Whether start is offered on its own local date, whatever r.from and r.to say. */
export function offersStart(r: SlotRequest, start: number): boolean {
  const date = dateOf(start, r.zone);
  const [day] = generateSlots({ ...r, from: date, to: date });
  return day?.slots.includes(start) ?? false;
}

function hoursOn(r: SlotRequest, date: LocalDate): Interval[] {
  return r.overrides.get(date) ?? r.weekly[weekday(date)] ?? [];
}

function startsOn(r: SlotRequest, date: LocalDate): number[] {
  const span = (r.durationMinutes + r.bufferMinutes) * MINUTE;
  const earliest = r.now + r.minNoticeMinutes * MINUTE;
  const starts = new Set<number>();
  for (const iv of hoursOn(r, date)) {
    const closes = edge(date, iv.end, r.zone);
    for (let at = alignUp(iv.start, r.stepMinutes); at < iv.end; at += r.stepMinutes) {
      const start = occurrences(date, at, r.zone)[0];
      if (start === undefined) continue;
      const end = start + span;
      if (start < earliest || end > closes || overlapsBusy(r.busy, start, end)) continue;
      starts.add(start);
    }
  }
  return [...starts].sort((a, b) => a - b);
}

function alignUp(t: number, step: number): number {
  const rem = t % step;
  return rem === 0 ? t : t + step - rem;
}

export function overlapsBusy(busy: readonly BusyInterval[], start: number, end: number): boolean {
  return busy.some((b) => start < b.end && b.start < end);
}
