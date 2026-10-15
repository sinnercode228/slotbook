import type {
  Booking,
  Confirmation,
  Customer,
  DayBookings,
  Hold,
  Location,
  NotificationStatus,
  OutboxNotification,
  Service,
  Slots,
  Staff,
} from './types';

export interface SlotQuery {
  locationId: number;
  serviceId: number;
  /** null asks for any staff member who offers the service. */
  staffId: number | null;
  from: string;
  to: string;
}

export interface HoldRequest {
  serviceId: number;
  staffId: number | null;
  start: string;
}

export interface BlockRequest {
  staffId: number;
  start: string;
  end: string;
  note: string;
}

export interface NotificationFilter {
  locationId?: number;
  bookingId?: string;
  status?: NotificationStatus;
  limit?: number;
}

export interface AdminApi {
  listBookings(locationId: number, date: string): Promise<DayBookings>;
  cancelBooking(bookingId: string): Promise<Booking>;
  createBlock(req: BlockRequest): Promise<Booking>;
  deleteBlock(blockId: string): Promise<void>;
  listNotifications(filter?: NotificationFilter): Promise<OutboxNotification[]>;
}

/**
 * Everything the web app needs from the backend. The http implementation
 * talks to the Go server; the mock runs the same rules in the browser.
 */
export interface BookingApi {
  readonly mode: 'http' | 'mock';
  listLocations(): Promise<Location[]>;
  listServices(locationId: number): Promise<Service[]>;
  listStaff(locationId: number, serviceId?: number): Promise<Staff[]>;
  listSlots(query: SlotQuery): Promise<Slots>;
  createHold(req: HoldRequest): Promise<Hold>;
  /** Gives a hold back so that its time is free at once. Releasing it again also succeeds. */
  releaseHold(bookingId: string, holdToken: string): Promise<void>;
  /** Retrying with the same idempotency key is safe and returns the first result. */
  confirmHold(holdToken: string, customer: Customer, idempotencyKey: string): Promise<Confirmation>;
  getBooking(bookingId: string, manageToken: string): Promise<Booking>;
  cancelBooking(bookingId: string, manageToken: string): Promise<Booking>;
  rescheduleBooking(bookingId: string, manageToken: string, start: string): Promise<Booking>;
  admin(token: string): AdminApi;
  /**
   * Calls onChange when the data may have changed elsewhere, such as in
   * another tab. Returns a function that stops the calls.
   */
  subscribe(onChange: () => void): () => void;
}
