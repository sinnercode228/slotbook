import type { Money } from '../api/types';
import { dateParts, type LocalDate } from '../schedule/date';
import { offsetAt, wallClock } from '../schedule/zone';

const cache = new Map<string, Intl.DateTimeFormat | Intl.NumberFormat>();

function dateFormat(locale: string, options: Intl.DateTimeFormatOptions): Intl.DateTimeFormat {
  const key = `d|${locale}|${JSON.stringify(options)}`;
  let f = cache.get(key) as Intl.DateTimeFormat | undefined;
  if (!f) {
    f = new Intl.DateTimeFormat(locale, options);
    cache.set(key, f);
  }
  return f;
}

const toDate = (instant: string | number) => new Date(typeof instant === 'number' ? instant : Date.parse(instant));

export function formatMoney(money: Money, locale: string): string {
  const key = `n|${locale}|${money.currency}|${money.amount_minor % 100 === 0}`;
  let f = cache.get(key) as Intl.NumberFormat | undefined;
  if (!f) {
    const whole = money.amount_minor % 100 === 0;
    f = new Intl.NumberFormat(locale, {
      style: 'currency',
      currency: money.currency,
      minimumFractionDigits: whole ? 0 : 2,
      maximumFractionDigits: whole ? 0 : 2,
    });
    cache.set(key, f);
  }
  return f.format(money.amount_minor / 100);
}

/** 24-hour wall-clock time of an instant in a zone. */
export function formatTime(instant: string | number, zone: string, locale: string): string {
  return dateFormat(locale, { timeZone: zone, hour: '2-digit', minute: '2-digit', hourCycle: 'h23' }).format(toDate(instant));
}

/** The date of an instant in a zone, e.g. "Wednesday, 14 October". */
export function formatDay(instant: string | number, zone: string, locale: string): string {
  return dateFormat(locale, { timeZone: zone, weekday: 'long', day: 'numeric', month: 'long' }).format(toDate(instant));
}

/** A short date and time, e.g. "Wed 14 Oct, 10:00". */
export function formatDateTime(instant: string | number, zone: string, locale: string): string {
  return dateFormat(locale, {
    timeZone: zone,
    weekday: 'short',
    day: 'numeric',
    month: 'short',
    hour: '2-digit',
    minute: '2-digit',
    hourCycle: 'h23',
  }).format(toDate(instant));
}

const DATE_LABELS = {
  weekday: { weekday: 'short' },
  day: { day: 'numeric' },
  month: { month: 'long', year: 'numeric' },
  long: { weekday: 'long', day: 'numeric', month: 'long' },
  short: { weekday: 'short', day: 'numeric', month: 'short' },
} satisfies Record<string, Intl.DateTimeFormatOptions>;

/** Formats a calendar date; it is read at UTC noon so no zone can move it. */
export function formatDateLabel(date: LocalDate, locale: string, style: keyof typeof DATE_LABELS): string {
  const { year, month, day } = dateParts(date);
  const noon = new Date(Date.UTC(year, month - 1, day, 12));
  return dateFormat(locale, { ...DATE_LABELS[style], timeZone: 'UTC' }).format(noon);
}

/** A readable name of a zone, e.g. "Central European Time (GMT+2)". */
export function zoneLabel(zone: string, locale: string, at: number = Date.now()): string {
  const part = (style: Intl.DateTimeFormatOptions['timeZoneName']) =>
    dateFormat(locale, { timeZone: zone, timeZoneName: style })
      .formatToParts(at)
      .find((p) => p.type === 'timeZoneName')?.value ?? zone;
  const name = part('longGeneric');
  const offset = part('shortOffset');
  return name === offset ? name : `${name} (${offset})`;
}

export function visitorZone(): string {
  return Intl.DateTimeFormat().resolvedOptions().timeZone;
}

/** Whether clocks in the two zones read differently at that instant. */
export function zonesDiffer(a: string, b: string, at: number): boolean {
  return offsetAt(at, a) !== offsetAt(at, b);
}

export type DayPeriod = 'morning' | 'afternoon' | 'evening';

export function dayPeriod(minutesOfDay: number): DayPeriod {
  if (minutesOfDay < 12 * 60) return 'morning';
  if (minutesOfDay < 17 * 60) return 'afternoon';
  return 'evening';
}

export function minutesOfDay(instant: string | number, zone: string): number {
  return wallClock(typeof instant === 'number' ? instant : Date.parse(instant), zone).minutes;
}
