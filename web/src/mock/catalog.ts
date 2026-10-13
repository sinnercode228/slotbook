import demoChain from '../../../seed/demo.json';
import type { Location, LocationSettings, Service, Staff } from '../api/types';
import { parseInterval, parseWeeklyHours, type Interval, type IntervalJSON, type WeeklyHours } from '../schedule/timeOfDay';

// The demo chain is read from seed/demo.json, the file the Go seed command
// loads, and numbered the way a fresh database numbers it: locations,
// services and staff each get ids 1, 2, 3... in file order.

interface SeedChain {
  brand: string;
  locations: {
    slug: string;
    name: string;
    time_zone: string;
    address: string;
    settings: Partial<LocationSettings>;
    services: {
      key: string;
      name: string;
      duration_minutes: number;
      buffer_after_minutes: number;
      price_minor: number;
      currency: string;
    }[];
    staff: {
      key: string;
      name: string;
      services: string[];
      weekly_hours: Record<string, IntervalJSON[] | undefined>;
      overrides: { date: string; intervals: IntervalJSON[] }[];
    }[];
  }[];
}

/** The same defaults as config.LocationDefaults in internal/config. */
export const LOCATION_DEFAULTS: LocationSettings = {
  slot_step_minutes: 15,
  min_notice_minutes: 120,
  horizon_days: 60,
  hold_ttl_seconds: 600,
  cancel_cutoff_minutes: 180,
};

export interface StaffSchedule {
  weekly: WeeklyHours;
  overrides: Map<string, Interval[]>;
}

export interface Catalog {
  brand: string;
  locations: Location[];
  services: Service[];
  staff: Staff[];
  schedules: Map<number, StaffSchedule>;
}

export function buildCatalog(chain: SeedChain): Catalog {
  const catalog: Catalog = { brand: chain.brand, locations: [], services: [], staff: [], schedules: new Map() };
  chain.locations.forEach((loc, index) => {
    const locationId = index + 1;
    catalog.locations.push({
      id: locationId,
      slug: loc.slug,
      name: loc.name,
      time_zone: loc.time_zone,
      address: loc.address,
      settings: { ...LOCATION_DEFAULTS, ...loc.settings },
    });
    const serviceIds = new Map<string, number>();
    for (const svc of loc.services) {
      const id = catalog.services.length + 1;
      serviceIds.set(svc.key, id);
      catalog.services.push({
        id,
        location_id: locationId,
        name: svc.name,
        duration_minutes: svc.duration_minutes,
        buffer_after_minutes: svc.buffer_after_minutes,
        price: { amount_minor: svc.price_minor, currency: svc.currency },
      });
    }
    for (const member of loc.staff) {
      const id = catalog.staff.length + 1;
      catalog.staff.push({
        id,
        location_id: locationId,
        name: member.name,
        service_ids: member.services.map((key) => serviceIds.get(key)!).sort((a, b) => a - b),
      });
      catalog.schedules.set(id, {
        weekly: parseWeeklyHours(member.weekly_hours),
        overrides: new Map(member.overrides.map((o) => [o.date, o.intervals.map(parseInterval)])),
      });
    }
  });
  return catalog;
}

export const demoCatalog: Catalog = buildCatalog(demoChain as SeedChain);
