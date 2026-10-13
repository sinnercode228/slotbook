import type { Customer } from '../api/types';
import { addDays } from '../schedule/date';
import { dateOf, formatInstant } from '../schedule/zone';
import type { Catalog } from './catalog';
import type { Engine } from './engine';

export const SAMPLE_CUSTOMERS: Customer[] = [
  { name: 'Ivan Petrov', phone: '+7 900 123-45-67', email: 'ivan.petrov@example.com' },
  { name: 'Olga Smirnova', phone: '+7 900 234-56-78' },
  { name: 'Dmitry Kozlov', phone: '+7 900 345-67-89', email: 'dmitry.k@example.com' },
  { name: 'Jana Becker', phone: '+7 900 456-78-90', email: 'jana.becker@example.com' },
  { name: 'Lukas Meyer', phone: '+7 900 567-89-01' },
  { name: 'Sofia Romero', phone: '+7 900 678-90-12', email: 'sofia.romero@example.com' },
];

/** A small deterministic generator, so a fresh demo looks the same on every machine that day. */
export function seededRandom(seed: string): () => number {
  let h = 2166136261;
  for (const ch of seed) h = Math.imul(h ^ ch.charCodeAt(0), 16777619);
  return () => {
    h = (h + 0x6d2b79f5) | 0;
    let t = Math.imul(h ^ (h >>> 15), 1 | h);
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

/**
 * Fills the next three days with a few confirmed appointments and one block
 * per location, so the admin calendar and the outbox have something to show.
 * Everything goes through the normal rules, so a sample can never break them.
 */
export function addSampleBookings(engine: Engine, catalog: Catalog, now: number): void {
  for (const loc of catalog.locations) {
    const today = dateOf(now, loc.time_zone);
    const random = seededRandom(`${loc.slug}:${today}`);
    let customer = 0;
    for (let offset = 0; offset < 3; offset++) {
      const date = addDays(today, offset);
      for (const member of engine.staff(loc.id)) {
        const serviceId = member.service_ids[0]!;
        const { days } = engine.listSlots({ locationId: loc.id, serviceId, staffId: member.id, from: date, to: date });
        const starts = days[0]!.slots;
        let booked = 0;
        for (const start of starts) {
          if (booked >= 3 || random() > 0.22) continue;
          try {
            const hold = engine.hold({ serviceId, staffId: member.id, start });
            const who = SAMPLE_CUSTOMERS[customer++ % SAMPLE_CUSTOMERS.length]!;
            engine.confirm(hold.hold_token, who, '');
            booked++;
          } catch {
            // Overlaps an earlier sample; skip it.
          }
        }
        if (offset === 1 && member === engine.staff(loc.id)[0] && starts.length > 0) {
          const start = Date.parse(starts[Math.floor(starts.length / 2)]!);
          try {
            engine.createBlock({
              staffId: member.id,
              start: formatInstant(start, loc.time_zone),
              end: formatInstant(start + 60 * 60_000, loc.time_zone),
              note: 'Team training',
            });
          } catch {
            // The hour is already booked; the demo can do without the block.
          }
        }
      }
    }
  }
}
