import { describe, expect, it } from 'vitest';
import { dayPeriod, formatDateLabel, formatMoney, formatTime, zonesDiffer } from './format';

describe('formatMoney', () => {
  it('drops zero minor units and keeps real ones', () => {
    expect(formatMoney({ amount_minor: 3800, currency: 'EUR' }, 'en-GB')).toBe('€38');
    expect(formatMoney({ amount_minor: 3850, currency: 'EUR' }, 'en-GB')).toBe('€38.50');
    expect(formatMoney({ amount_minor: 250000, currency: 'RUB' }, 'ru-RU').replace(/\s/g, ' ')).toBe('2 500 ₽');
  });
});

describe('formatTime', () => {
  it('shows the wall clock of the given zone in 24-hour form', () => {
    expect(formatTime('2026-10-14T10:00:00+02:00', 'Europe/Berlin', 'en-GB')).toBe('10:00');
    expect(formatTime('2026-10-14T10:00:00+02:00', 'Europe/Moscow', 'en-GB')).toBe('11:00');
    expect(formatTime('2026-10-14T21:30:00+02:00', 'America/New_York', 'en-GB')).toBe('15:30');
  });
});

describe('formatDateLabel', () => {
  it('formats a calendar date without shifting it through a zone', () => {
    expect(formatDateLabel('2026-10-14', 'en-GB', 'weekday')).toBe('Wed');
    expect(formatDateLabel('2026-10-14', 'ru-RU', 'long')).toBe('среда, 14 октября');
  });
});

describe('dayPeriod', () => {
  it('splits the day at noon and at five in the evening', () => {
    expect(dayPeriod(11 * 60 + 45)).toBe('morning');
    expect(dayPeriod(12 * 60)).toBe('afternoon');
    expect(dayPeriod(16 * 60 + 59)).toBe('afternoon');
    expect(dayPeriod(17 * 60)).toBe('evening');
  });
});

describe('zonesDiffer', () => {
  const at = Date.parse('2026-10-14T08:00:00Z');
  it('compares offsets, not names', () => {
    expect(zonesDiffer('Europe/Berlin', 'Europe/Paris', at)).toBe(false);
    expect(zonesDiffer('Europe/Berlin', 'Europe/Moscow', at)).toBe(true);
  });
});
