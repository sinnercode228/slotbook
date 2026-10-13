// @vitest-environment node
/// <reference types="node" />
import { readdirSync, readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';
import { generateSlots, type SlotRequest } from './generate';
import { parseInterval, parseWeeklyHours, type IntervalJSON } from './timeOfDay';
import { formatInstant, parseInstant } from './zone';

// The same files drive internal/slots/golden_test.go; see
// testdata/slots/README.md for the format.
interface Vector {
  description: string;
  input: {
    time_zone: string;
    now: string;
    from: string;
    to: string;
    duration_minutes: number;
    buffer_after_minutes: number;
    slot_step_minutes: number;
    min_notice_minutes: number;
    horizon_days: number;
    weekly_hours: Record<string, IntervalJSON[]>;
    overrides: { date: string; intervals: IntervalJSON[] }[];
    busy: { start: string; end: string }[];
  };
  expected: { date: string; slots: string[] }[];
}

// Read at run time rather than through import.meta.glob, whose file list
// would be cached with the transformed test and miss newly added vectors.
const vectorDir = new URL('../../../testdata/slots/', import.meta.url);
const vectors = readdirSync(vectorDir)
  .filter((name) => name.endsWith('.json'))
  .sort()
  .map((name) => [name, JSON.parse(readFileSync(new URL(name, vectorDir), 'utf8')) as Vector] as const);

function toRequest({ input }: Vector): SlotRequest {
  return {
    zone: input.time_zone,
    weekly: parseWeeklyHours(input.weekly_hours),
    overrides: new Map(input.overrides.map((o) => [o.date, o.intervals.map(parseInterval)])),
    durationMinutes: input.duration_minutes,
    bufferMinutes: input.buffer_after_minutes,
    stepMinutes: input.slot_step_minutes,
    minNoticeMinutes: input.min_notice_minutes,
    horizonDays: input.horizon_days,
    busy: input.busy.map((b) => ({ start: parseInstant(b.start), end: parseInstant(b.end) })),
    now: parseInstant(input.now),
    from: input.from,
    to: input.to,
  };
}

describe('golden slot vectors shared with the Go generator', () => {
  it('finds the shared vectors', () => {
    expect(vectors.length).toBeGreaterThanOrEqual(10);
  });

  it.each(vectors)('%s', (_name, vector) => {
    const days = generateSlots(toRequest(vector));
    const got = days.map((d) => ({
      date: d.date,
      slots: d.slots.map((s) => formatInstant(s, vector.input.time_zone)),
    }));
    expect(got).toEqual(vector.expected);
  });
});
