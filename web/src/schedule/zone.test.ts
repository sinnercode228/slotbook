// @vitest-environment node
import { describe, expect, it } from 'vitest';
import { addDays, daysBetween, parseLocalDate, weekday } from './date';
import { dateOf, formatInstant, instantAt, occurrences, parseInstant, startOfDate } from './zone';

describe('formatInstant', () => {
  it('writes a zero offset as Z, like Go time.RFC3339', () => {
    expect(formatInstant(Date.parse('2026-01-15T09:00:00Z'), 'Europe/London')).toBe('2026-01-15T09:00:00Z');
    expect(formatInstant(Date.parse('2026-07-15T09:00:00Z'), 'Europe/London')).toBe('2026-07-15T10:00:00+01:00');
  });

  it('writes negative and half-hour offsets', () => {
    expect(formatInstant(Date.parse('2026-01-15T15:00:00Z'), 'America/New_York')).toBe('2026-01-15T10:00:00-05:00');
    expect(formatInstant(Date.parse('2026-01-15T04:30:00Z'), 'Asia/Kolkata')).toBe('2026-01-15T10:00:00+05:30');
  });
});

describe('wall time to instant', () => {
  it('finds both instants of a repeated wall time', () => {
    const found = occurrences('2026-10-25', 2 * 60 + 30, 'Europe/Berlin').map((t) => new Date(t).toISOString());
    expect(found).toEqual(['2026-10-25T00:30:00.000Z', '2026-10-25T01:30:00.000Z']);
  });

  it('maps a skipped wall time to the moment of the jump', () => {
    expect(new Date(instantAt('2026-03-29', 2 * 60 + 30, 'Europe/Berlin')).toISOString()).toBe('2026-03-29T01:00:00.000Z');
  });

  it('starts a day at the first instant of the date', () => {
    expect(new Date(startOfDate('2026-10-14', 'Europe/Moscow')).toISOString()).toBe('2026-10-13T21:00:00.000Z');
  });

  it('reads the local date of an instant', () => {
    expect(dateOf(Date.parse('2026-10-13T22:30:00Z'), 'Europe/Moscow')).toBe('2026-10-14');
    expect(dateOf(Date.parse('2026-10-13T22:30:00Z'), 'America/New_York')).toBe('2026-10-13');
  });
});

describe('input checks', () => {
  it('accepts only RFC 3339 instants', () => {
    expect(parseInstant('2026-10-14T10:00:00+02:00')).toBe(Date.parse('2026-10-14T08:00:00Z'));
    expect(() => parseInstant('2026-10-14 10:00')).toThrow(/RFC 3339/);
  });

  it('rejects impossible dates', () => {
    expect(parseLocalDate('2028-02-29')).toBe('2028-02-29');
    expect(() => parseLocalDate('2026-02-29')).toThrow();
  });
});

describe('calendar arithmetic', () => {
  it('walks calendar days across DST changes and year ends', () => {
    expect(addDays('2026-03-28', 1)).toBe('2026-03-29');
    expect(addDays('2026-12-31', 1)).toBe('2027-01-01');
    expect(daysBetween('2026-10-01', '2026-10-31')).toBe(30);
    expect(weekday('2026-10-14')).toBe(3);
  });
});
