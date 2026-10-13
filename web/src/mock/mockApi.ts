import type { BookingApi, HoldRequest } from '../api/client';
import type { Booking } from '../api/types';
import { demoCatalog, type Catalog } from './catalog';
import { Engine } from './engine';
import { addSampleBookings, SAMPLE_CUSTOMERS } from './samples';
import { browserStorage, type StateStorage } from './storage';

export interface MockApiOptions {
  storage?: StateStorage;
  clock?: () => number;
  catalog?: Catalog;
  /** Add sample bookings to a fresh demo (default true). */
  samples?: boolean;
  /** Delay every answer, so that loading states show as they would over a network. */
  latencyMs?: number;
}

export interface DemoControls {
  /** Forgets every booking and starts a fresh demo. */
  reset(): Promise<void>;
  /** Holds and confirms a slot as another visitor would, to show a lost race. */
  bookForSomeoneElse(req: HoldRequest): Promise<Booking>;
}

export interface MockApi extends BookingApi {
  readonly mode: 'mock';
  readonly demo: DemoControls;
}

/** The booking API running entirely in the browser, on the seed file's chain. */
export function createMockApi(options: MockApiOptions = {}): MockApi {
  const catalog = options.catalog ?? demoCatalog;
  const storage = options.storage ?? browserStorage();
  const clock = options.clock ?? Date.now;
  const samples = options.samples ?? true;
  const latency = options.latencyMs ?? 0;

  const pause = () => (latency > 0 ? new Promise((resolve) => setTimeout(resolve, latency)) : Promise.resolve());

  /** One API call: a transaction on the shared state, answered after the simulated latency. */
  async function call<T>(fn: (engine: Engine) => T): Promise<T> {
    try {
      const result = await storage.transact((state) => {
        const engine = new Engine(catalog, state, clock());
        if (!state.initialized) {
          if (samples) addSampleBookings(engine, catalog, clock());
          state.initialized = true;
        }
        engine.sweep();
        return fn(engine);
      });
      await pause();
      return result;
    } catch (err) {
      await pause();
      throw err;
    }
  }

  return {
    mode: 'mock',
    listLocations: () => call(() => structuredClone(catalog.locations)),
    listServices: (locationId) => call((e) => structuredClone(e.services(locationId))),
    listStaff: (locationId, serviceId) => call((e) => structuredClone(e.staff(locationId, serviceId))),
    listSlots: (query) => call((e) => e.listSlots(query)),
    createHold: (req) => call((e) => e.hold(req)),
    confirmHold: (holdToken, customer, key) => call((e) => e.confirm(holdToken, customer, key)),
    getBooking: (id, token) => call((e) => e.get(id, token)),
    cancelBooking: (id, token) => call((e) => e.cancel(id, token)),
    rescheduleBooking: (id, token, start) => call((e) => e.reschedule(id, token, start)),
    // Any token opens the demo's admin screens.
    admin: () => ({
      listBookings: (locationId, date) => call((e) => e.listDay(locationId, date)),
      cancelBooking: (id) => call((e) => e.adminCancel(id)),
      createBlock: (req) => call((e) => e.createBlock(req)),
      deleteBlock: (id) => call((e) => e.deleteBlock(id)),
      listNotifications: (filter) => call((e) => e.notifications(filter)),
    }),
    subscribe: (onChange) => storage.subscribe(onChange),
    demo: {
      reset: () => storage.clear(),
      bookForSomeoneElse: (req) =>
        call((e) => {
          const hold = e.hold(req);
          const who = SAMPLE_CUSTOMERS[Math.floor(Math.random() * SAMPLE_CUSTOMERS.length)]!;
          return e.confirm(hold.hold_token, who, '').booking;
        }),
    },
  };
}
