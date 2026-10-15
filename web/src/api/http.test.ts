import { describe, expect, it, vi } from 'vitest';
import { ApiError } from './errors';
import { createHttpApi } from './http';

function jsonResponse(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
}

function recordingFetch(response: () => Response | Promise<Response>) {
  const calls: { url: string; init: RequestInit }[] = [];
  const fetchImpl = vi.fn(async (url: string | URL | Request, init?: RequestInit) => {
    calls.push({ url: String(url), init: init ?? {} });
    return response();
  });
  return { calls, fetchImpl };
}

describe('http api client', () => {
  it('uses relative /v1 URLs by default and unwraps list responses', async () => {
    const { calls, fetchImpl } = recordingFetch(() => jsonResponse(200, { staff: [{ id: 4 }] }));
    const api = createHttpApi('', fetchImpl);
    const staff = await api.listStaff(2, 5);
    expect(calls[0]!.url).toBe('/v1/locations/2/staff?service_id=5');
    expect(staff).toEqual([{ id: 4 }]);
  });

  it('prefixes a configured base URL without doubling slashes', async () => {
    const { calls, fetchImpl } = recordingFetch(() => jsonResponse(200, { locations: [] }));
    await createHttpApi('https://api.example.com/', fetchImpl).listLocations();
    expect(calls[0]!.url).toBe('https://api.example.com/v1/locations');
  });

  it('sends "any staff" as a null staff_id and omits it from slot queries', async () => {
    const { calls, fetchImpl } = recordingFetch(() => jsonResponse(201, { hold_token: 't' }));
    const api = createHttpApi('', fetchImpl);
    await api.createHold({ serviceId: 5, staffId: null, start: '2026-10-14T10:00:00+02:00' });
    expect(JSON.parse(String(calls[0]!.init.body))).toEqual({
      service_id: 5,
      staff_id: null,
      start: '2026-10-14T10:00:00+02:00',
    });

    await api.listSlots({ locationId: 2, serviceId: 5, staffId: null, from: '2026-10-14', to: '2026-10-20' });
    expect(calls[1]!.url).toBe('/v1/locations/2/slots?service_id=5&from=2026-10-14&to=2026-10-20');
  });

  it('sends the idempotency key, the manage token and the admin token as headers', async () => {
    const { calls, fetchImpl } = recordingFetch(() => jsonResponse(201, {}));
    const api = createHttpApi('', fetchImpl);
    await api.confirmHold('hold-1', { name: 'Anna', phone: '+7 900 123-45-67' }, 'key-1');
    await api.cancelBooking('b-1', 'manage-1');
    await api.admin('secret').listNotifications({ locationId: 2, limit: 50 });

    expect(new Headers(calls[0]!.init.headers).get('Idempotency-Key')).toBe('key-1');
    expect(new Headers(calls[1]!.init.headers).get('X-Manage-Token')).toBe('manage-1');
    expect(new Headers(calls[2]!.init.headers).get('Authorization')).toBe('Bearer secret');
    expect(calls[2]!.url).toBe('/v1/admin/notifications?location_id=2&limit=50');
  });

  it('turns an error body into an ApiError with its status and code', async () => {
    const { fetchImpl } = recordingFetch(() =>
      jsonResponse(409, { error: { code: 'slot_taken', message: 'slot is already taken' } }),
    );
    const err = await createHttpApi('', fetchImpl)
      .createHold({ serviceId: 5, staffId: 4, start: '2026-10-14T10:00:00+02:00' })
      .catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect(err).toMatchObject({ status: 409, code: 'slot_taken', message: 'slot is already taken' });
  });

  it('keeps the seconds from Retry-After when a hold is refused for too many attempts', async () => {
    const responses = [
      new Response(JSON.stringify({ error: { code: 'rate_limited', message: 'try again in 90 s' } }), {
        status: 429,
        headers: { 'Content-Type': 'application/json', 'Retry-After': '90' },
      }),
      jsonResponse(429, { error: { code: 'rate_limited', message: 'too many requests' } }),
    ];
    const { fetchImpl } = recordingFetch(() => responses.shift()!);
    const api = createHttpApi('', fetchImpl);
    const hold = () => api.createHold({ serviceId: 5, staffId: 4, start: '2026-10-14T10:00:00+02:00' });
    await expect(hold()).rejects.toMatchObject({ status: 429, code: 'rate_limited', retryAfter: 90 });
    await expect(hold()).rejects.toMatchObject({ status: 429, code: 'rate_limited', retryAfter: undefined });
  });

  it('reports a response without a JSON error body as internal', async () => {
    const { fetchImpl } = recordingFetch(() => new Response('<html>Bad gateway</html>', { status: 502 }));
    await expect(createHttpApi('', fetchImpl).listLocations()).rejects.toMatchObject({
      status: 502,
      code: 'internal',
    });
  });

  it('reports a failed request as a network error', async () => {
    const fetchImpl = vi.fn(async () => {
      throw new TypeError('Failed to fetch');
    });
    await expect(createHttpApi('', fetchImpl).listLocations()).rejects.toMatchObject({
      status: 0,
      code: 'network',
    });
  });

  it('accepts 204 without a body', async () => {
    const { fetchImpl } = recordingFetch(() => new Response(null, { status: 204 }));
    await expect(createHttpApi('', fetchImpl).admin('t').deleteBlock('b-1')).resolves.toBeUndefined();
  });
});
