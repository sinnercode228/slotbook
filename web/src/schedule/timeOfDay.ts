// Wall-clock times are minutes since local midnight; 24:00 (1440) is valid as
// the end of an interval.
export const END_OF_DAY = 24 * 60;

export interface Interval {
  start: number;
  end: number;
}

export interface IntervalJSON {
  start: string;
  end: string;
}

/** Working intervals per weekday, indexed like Date#getDay (0 is Sunday). */
export type WeeklyHours = Interval[][];

const DAY_NAMES = ['sunday', 'monday', 'tuesday', 'wednesday', 'thursday', 'friday', 'saturday'];

export function parseTimeOfDay(value: string): number {
  const m = /^(\d{2}):(\d{2})$/.exec(value);
  if (!m) throw new Error(`invalid time "${value}", want HH:MM`);
  const minutes = Number(m[1]) * 60 + Number(m[2]);
  if (Number(m[2]) > 59 || minutes > END_OF_DAY) {
    throw new Error(`invalid time "${value}", want 00:00 to 24:00`);
  }
  return minutes;
}

export function formatTimeOfDay(minutes: number): string {
  return `${String(Math.floor(minutes / 60)).padStart(2, '0')}:${String(minutes % 60).padStart(2, '0')}`;
}

export function parseInterval(raw: IntervalJSON): Interval {
  const interval = { start: parseTimeOfDay(raw.start), end: parseTimeOfDay(raw.end) };
  if (interval.start >= interval.end) {
    throw new Error(`invalid interval ${raw.start}-${raw.end}`);
  }
  return interval;
}

/** Reads the JSON form used by the seed file and the golden vectors. */
export function parseWeeklyHours(raw: Record<string, IntervalJSON[]>): WeeklyHours {
  const weekly: WeeklyHours = DAY_NAMES.map(() => []);
  for (const [name, intervals] of Object.entries(raw)) {
    const day = DAY_NAMES.indexOf(name);
    if (day < 0) throw new Error(`unknown weekday "${name}"`);
    weekly[day] = intervals.map(parseInterval);
  }
  return weekly;
}
