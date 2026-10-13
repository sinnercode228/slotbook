// A minimal iCalendar (RFC 5545) file with one event, built in the browser
// so the confirmation screen can offer "add to calendar" without a server.

export interface CalendarEvent {
  uid: string;
  start: string;
  end: string;
  /** When the file was made; defaults to now. */
  stamp?: string;
  summary: string;
  location: string;
  description: string;
}

function utcStamp(iso: string): string {
  return new Date(iso).toISOString().replace(/[-:]/g, '').replace(/\.\d{3}/, '');
}

function escapeText(value: string): string {
  return value.replace(/\\/g, '\\\\').replace(/;/g, '\\;').replace(/,/g, '\\,').replace(/\r?\n/g, '\\n');
}

// Lines are folded at 75 octets; continuation lines start with a space.
function fold(line: string): string {
  const encoder = new TextEncoder();
  const parts: string[] = [];
  let current = '';
  let size = 0;
  for (const ch of line) {
    const width = encoder.encode(ch).length;
    const limit = parts.length === 0 ? 75 : 74;
    if (size + width > limit) {
      parts.push(current);
      current = '';
      size = 0;
    }
    current += ch;
    size += width;
  }
  parts.push(current);
  return parts.join('\r\n ');
}

export function buildIcs(event: CalendarEvent): string {
  const lines = [
    'BEGIN:VCALENDAR',
    'VERSION:2.0',
    'PRODID:-//Slotbook//Booking widget//EN',
    'CALSCALE:GREGORIAN',
    'METHOD:PUBLISH',
    'BEGIN:VEVENT',
    `UID:${event.uid}@slotbook`,
    `DTSTAMP:${utcStamp(event.stamp ?? new Date().toISOString())}`,
    `DTSTART:${utcStamp(event.start)}`,
    `DTEND:${utcStamp(event.end)}`,
    `SUMMARY:${escapeText(event.summary)}`,
    `LOCATION:${escapeText(event.location)}`,
    `DESCRIPTION:${escapeText(event.description)}`,
    'END:VEVENT',
    'END:VCALENDAR',
  ];
  return lines.map(fold).join('\r\n') + '\r\n';
}

/** Offers the event as a file download. */
export function downloadIcs(event: CalendarEvent, filename: string): void {
  const blob = new Blob([buildIcs(event)], { type: 'text/calendar;charset=utf-8' });
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url;
  link.download = filename;
  document.body.append(link);
  link.click();
  link.remove();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}
