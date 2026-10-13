// A LocalDate is a calendar date with no zone, written YYYY-MM-DD. Slot
// generation walks dates rather than adding 24 hours, so DST days keep their
// 23 or 25 hours. Arithmetic runs on UTC midnights, which have no DST.
export type LocalDate = string;

const DATE_PATTERN = /^(\d{4})-(\d{2})-(\d{2})$/;
const DAY_MS = 86_400_000;

export function parseLocalDate(value: string): LocalDate {
  const m = DATE_PATTERN.exec(value);
  if (!m) throw new Error(`invalid date "${value}", want YYYY-MM-DD`);
  const [year, month, day] = [Number(m[1]), Number(m[2]), Number(m[3])];
  const utc = new Date(Date.UTC(year, month - 1, day));
  if (utc.getUTCFullYear() !== year || utc.getUTCMonth() !== month - 1 || utc.getUTCDate() !== day) {
    throw new Error(`invalid date "${value}"`);
  }
  return value;
}

export function dateParts(date: LocalDate): { year: number; month: number; day: number } {
  const m = DATE_PATTERN.exec(date);
  if (!m) throw new Error(`invalid date "${date}", want YYYY-MM-DD`);
  return { year: Number(m[1]), month: Number(m[2]), day: Number(m[3]) };
}

export function formatLocalDate(year: number, month: number, day: number): LocalDate {
  return `${String(year).padStart(4, '0')}-${String(month).padStart(2, '0')}-${String(day).padStart(2, '0')}`;
}

function utcMidnight(date: LocalDate): number {
  const { year, month, day } = dateParts(date);
  return Date.UTC(year, month - 1, day);
}

function fromUtcMidnight(ms: number): LocalDate {
  const d = new Date(ms);
  return formatLocalDate(d.getUTCFullYear(), d.getUTCMonth() + 1, d.getUTCDate());
}

export function addDays(date: LocalDate, n: number): LocalDate {
  return fromUtcMidnight(utcMidnight(date) + n * DAY_MS);
}

/** Day of the week, 0 for Sunday as in JavaScript and Go. */
export function weekday(date: LocalDate): number {
  return new Date(utcMidnight(date)).getUTCDay();
}

/** Calendar days from a to b, negative when b is earlier. */
export function daysBetween(a: LocalDate, b: LocalDate): number {
  return Math.round((utcMidnight(b) - utcMidnight(a)) / DAY_MS);
}

/** Every date from `from` to `to`, both included. */
export function dateRange(from: LocalDate, to: LocalDate): LocalDate[] {
  const out: LocalDate[] = [];
  for (let d = from; d <= to; d = addDays(d, 1)) out.push(d);
  return out;
}
