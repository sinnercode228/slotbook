import { describe, expect, it } from 'vitest';
import { buildIcs } from './ics';

const event = {
  uid: '6b236920-9d88-44bb-94d1-4fa5b116edeb',
  start: '2026-10-14T10:00:00+02:00',
  end: '2026-10-14T10:45:00+02:00',
  stamp: '2026-10-12T08:02:13Z',
  summary: 'Haircut with Lena H.',
  location: 'Snipwell Berlin, Beispielstrasse 12; Berlin',
  description: 'Change or cancel: https://example.com/#/booking/6b23?token=abc',
};

describe('buildIcs', () => {
  const ics = buildIcs(event);
  const lines = ics.split('\r\n');

  it('is a calendar with one event in UTC, lines ending in CRLF', () => {
    expect(lines[0]).toBe('BEGIN:VCALENDAR');
    expect(lines).toContain('VERSION:2.0');
    expect(lines).toContain('BEGIN:VEVENT');
    expect(lines).toContain('UID:6b236920-9d88-44bb-94d1-4fa5b116edeb@slotbook');
    expect(lines).toContain('DTSTART:20261014T080000Z');
    expect(lines).toContain('DTEND:20261014T084500Z');
    expect(lines).toContain('DTSTAMP:20261012T080213Z');
    expect(ics.endsWith('END:VCALENDAR\r\n')).toBe(true);
  });

  it('escapes commas, semicolons and backslashes in text', () => {
    expect(lines).toContain('LOCATION:Snipwell Berlin\\, Beispielstrasse 12\\; Berlin');
  });

  it('folds lines longer than 75 octets', () => {
    expect(lines.every((line) => new TextEncoder().encode(line).length <= 75)).toBe(true);
    const unfolded = ics.replace(/\r\n /g, '');
    expect(unfolded).toContain('DESCRIPTION:Change or cancel: https://example.com/#/booking/6b23?token=abc');
  });

  it('never splits a multi-byte character when folding', () => {
    const summary = 'Стрижка у мастера '.repeat(6).trim();
    const russian = buildIcs({ ...event, summary });
    expect(russian.split('\r\n').every((line) => new TextEncoder().encode(line).length <= 75)).toBe(true);
    expect(russian.replace(/\r\n /g, '')).toContain(`SUMMARY:${summary}`);
  });
});
