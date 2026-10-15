import type { AdminApi, BookingApi, NotificationFilter } from './client';
import { ApiError, type ErrorCode } from './errors';
import type {
  Booking,
  Confirmation,
  DayBookings,
  Hold,
  Location,
  OutboxNotification,
  Service,
  Slots,
  Staff,
} from './types';

type FetchLike = (input: string, init?: RequestInit) => Promise<Response>;

interface RequestOptions {
  method?: 'GET' | 'POST' | 'DELETE';
  body?: unknown;
  headers?: Record<string, string>;
}

function query(params: Record<string, string | number | null | undefined>): string {
  const search = new URLSearchParams();
  for (const [key, value] of Object.entries(params)) {
    if (value !== null && value !== undefined && value !== '') search.set(key, String(value));
  }
  const s = search.toString();
  return s ? `?${s}` : '';
}

/** Retry-After in seconds; the API never sends the HTTP-date form. */
function retryAfterSeconds(res: Response): number | undefined {
  const value = res.headers.get('Retry-After')?.trim();
  return value && /^\d+$/.test(value) ? Number(value) : undefined;
}

async function readError(res: Response): Promise<ApiError> {
  try {
    const body = (await res.json()) as { error?: { code?: string; message?: string } };
    if (body.error?.code) {
      const message = body.error.message ?? body.error.code;
      return new ApiError(res.status, body.error.code as ErrorCode, message, retryAfterSeconds(res));
    }
  } catch {
    // Not JSON: a proxy or gateway answered instead of the API.
  }
  return new ApiError(res.status, 'internal', `unexpected response ${res.status}`);
}

/**
 * The API client for the Go server. With an empty base URL the requests go
 * to /v1 on the page's own origin, which is how the server hosts the widget.
 */
export function createHttpApi(baseUrl = '', fetchImpl: FetchLike = (input, init) => fetch(input, init)): BookingApi {
  const base = baseUrl.replace(/\/+$/, '');

  async function request<T>(path: string, { method = 'GET', body, headers = {} }: RequestOptions = {}): Promise<T> {
    const init: RequestInit = { method, headers: { Accept: 'application/json', ...headers } };
    if (body !== undefined) {
      init.body = JSON.stringify(body);
      init.headers = { ...init.headers, 'Content-Type': 'application/json' };
    }
    let res: Response;
    try {
      res = await fetchImpl(`${base}${path}`, init);
    } catch (err) {
      throw new ApiError(0, 'network', err instanceof Error ? err.message : 'network error');
    }
    if (!res.ok) throw await readError(res);
    if (res.status === 204) return undefined as T;
    return (await res.json()) as T;
  }

  const manage = (token: string) => ({ 'X-Manage-Token': token });

  function admin(token: string): AdminApi {
    const headers = { Authorization: `Bearer ${token}` };
    return {
      listBookings: (locationId, date) =>
        request<DayBookings>(`/v1/admin/bookings${query({ location_id: locationId, date })}`, { headers }),
      cancelBooking: (bookingId) =>
        request<Booking>(`/v1/admin/bookings/${encodeURIComponent(bookingId)}/cancel`, { method: 'POST', headers }),
      createBlock: (req) =>
        request<Booking>('/v1/admin/blocks', {
          method: 'POST',
          headers,
          body: { staff_id: req.staffId, start: req.start, end: req.end, note: req.note },
        }),
      deleteBlock: (blockId) =>
        request<void>(`/v1/admin/blocks/${encodeURIComponent(blockId)}`, { method: 'DELETE', headers }),
      listNotifications: async (filter: NotificationFilter = {}) => {
        const params = query({
          location_id: filter.locationId,
          booking_id: filter.bookingId,
          status: filter.status,
          limit: filter.limit,
        });
        const res = await request<{ notifications: OutboxNotification[] }>(`/v1/admin/notifications${params}`, {
          headers,
        });
        return res.notifications;
      },
    };
  }

  return {
    mode: 'http',
    listLocations: async () => (await request<{ locations: Location[] }>('/v1/locations')).locations,
    listServices: async (locationId) =>
      (await request<{ services: Service[] }>(`/v1/locations/${locationId}/services`)).services,
    listStaff: async (locationId, serviceId) =>
      (await request<{ staff: Staff[] }>(`/v1/locations/${locationId}/staff${query({ service_id: serviceId })}`)).staff,
    listSlots: (q) =>
      request<Slots>(
        `/v1/locations/${q.locationId}/slots${query({ service_id: q.serviceId, staff_id: q.staffId, from: q.from, to: q.to })}`,
      ),
    createHold: (req) =>
      request<Hold>('/v1/holds', {
        method: 'POST',
        body: { service_id: req.serviceId, staff_id: req.staffId, start: req.start },
      }),
    confirmHold: (holdToken, customer, idempotencyKey) =>
      request<Confirmation>('/v1/bookings', {
        method: 'POST',
        headers: { 'Idempotency-Key': idempotencyKey },
        body: { hold_token: holdToken, customer },
      }),
    getBooking: (id, token) => request<Booking>(`/v1/bookings/${encodeURIComponent(id)}`, { headers: manage(token) }),
    cancelBooking: (id, token) =>
      request<Booking>(`/v1/bookings/${encodeURIComponent(id)}/cancel`, { method: 'POST', headers: manage(token) }),
    rescheduleBooking: (id, token, start) =>
      request<Booking>(`/v1/bookings/${encodeURIComponent(id)}/reschedule`, {
        method: 'POST',
        headers: manage(token),
        body: { start },
      }),
    admin,
    // The server has no push channel; screens refresh on focus instead.
    subscribe: () => () => {},
  };
}
