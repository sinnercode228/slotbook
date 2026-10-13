# Slotbook API

Base path `/v1`. Requests and responses are JSON (`Content-Type: application/json`).
Request bodies are limited to 64 KB and must not contain unknown fields.

Times in responses are RFC 3339 strings in the location's time zone
(`2026-10-14T10:00:00+02:00`), except in the admin outbox listing, which uses
UTC. Times in requests may use any offset. Dates are `YYYY-MM-DD` in the
location's calendar. Money is `{"amount_minor": 3800, "currency": "EUR"}`
(minor units: cents, kopecks).

Every response carries `X-Request-ID` (an incoming one is kept if it is up to
64 characters of `[A-Za-z0-9._-]`).

## Errors

```json
{"error": {"code": "slot_taken", "message": "slot is already taken"}}
```

| Status | Code | When |
|---|---|---|
| 400 | `bad_request` | Malformed JSON, unknown field, missing or unparseable parameter |
| 401 | `unauthorized` | Admin route without a valid bearer token |
| 404 | `not_found` | Unknown location, service, booking, block or route; wrong manage token |
| 404 | `hold_not_found` | Confirm with an unknown hold token |
| 405 | `method_not_allowed` | Known path, other method (see `Allow`) |
| 409 | `slot_taken` | The period overlaps an active booking, hold or block |
| 409 | `already_confirmed` | Confirm on a hold that was already confirmed |
| 409 | `too_late` | Cancel or reschedule after the location's cancellation cutoff |
| 409 | `not_active` | Cancel or reschedule a booking that is not confirmed |
| 410 | `hold_expired` | Confirm after the hold ran out |
| 413 | `payload_too_large` | Body over 64 KB |
| 422 | `invalid_slot` | Start is not an offered slot (off the step grid, outside hours, inside the minimum notice, beyond the horizon); the message says which |
| 422 | `invalid_staff` | The staff member does not offer the service |
| 422 | `validation_failed` | A field is invalid (contact details, block times, Idempotency-Key) |
| 422 | `idempotency_key_reused` | Same Idempotency-Key with a different request |
| 500 | `internal` | Anything else |

## Health

- `GET /healthz` → `200 {"status":"ok"}` while the process runs.
- `GET /readyz` → `200 {"status":"ready"}`, or `503 not_ready` when the database does not answer.

## Catalog

### `GET /v1/locations`

```json
{
  "locations": [
    {
      "id": 2,
      "slug": "snipwell-berlin",
      "name": "Snipwell Berlin, Beispielstrasse",
      "time_zone": "Europe/Berlin",
      "address": "Beispielstrasse 12, Berlin",
      "settings": {
        "slot_step_minutes": 15,
        "min_notice_minutes": 60,
        "horizon_days": 60,
        "hold_ttl_seconds": 600,
        "cancel_cutoff_minutes": 1440
      }
    }
  ]
}
```

`GET /v1/locations/{locationID}` returns one location object.

### `GET /v1/locations/{locationID}/services`

```json
{
  "services": [
    {"id": 5, "location_id": 2, "name": "Haircut", "duration_minutes": 45,
     "buffer_after_minutes": 15, "price": {"amount_minor": 3800, "currency": "EUR"}}
  ]
}
```

### `GET /v1/locations/{locationID}/staff?service_id=5`

`service_id` is optional and keeps only staff who offer that service.

```json
{"staff": [{"id": 4, "location_id": 2, "name": "Lena H.", "service_ids": [5, 8]}]}
```

### `GET /v1/locations/{locationID}/slots`

| Parameter | |
|---|---|
| `service_id` | required |
| `staff_id` | optional; without it a start is listed when any staff member who offers the service is free |
| `from` | local date, default today in the location zone |
| `to` | local date, inclusive, default `from` + 6 days; at most 31 days in total |

```json
{
  "location_id": 2,
  "service_id": 5,
  "staff_id": null,
  "time_zone": "Europe/Berlin",
  "days": [
    {"date": "2026-10-14", "slots": ["2026-10-14T09:00:00+02:00", "2026-10-14T09:15:00+02:00"]},
    {"date": "2026-10-15", "slots": []}
  ]
}
```

Each date in the range is present; `slots` may be empty. The rules (DST,
buffers, notice, horizon) are those of `testdata/slots/README.md`.

## Booking flow

### `POST /v1/holds`

Reserves a slot for the location's hold TTL (10 minutes by default).

```json
{"service_id": 5, "staff_id": 4, "start": "2026-10-14T10:00:00+02:00"}
```

`staff_id` may be omitted or `null` for "any staff": the server picks the
staff member with the fewest confirmed appointments that day (then the lowest
id) and moves on to the next one if that slot was just taken.

`201 Created`:

```json
{
  "hold_token": "i9BYK3fK_BoJgUH8l2gClZX8zof7jij7-uwz5pFmOTQ",
  "expires_at": "2026-10-12T10:10:00+02:00",
  "booking": { "...": "a booking object with status held" }
}
```

Errors: `invalid_slot`, `invalid_staff`, `slot_taken`, `not_found`.

### `POST /v1/bookings`

Confirms a hold. Optional header `Idempotency-Key` (up to 255 printable ASCII
characters, a UUID is a good choice): repeating the request with the same key
and body returns the stored `201` response with `Idempotent-Replayed: true`;
the same key with another body is `422 idempotency_key_reused`. Keys are kept
for 24 hours.

```json
{
  "hold_token": "i9BYK3fK_BoJgUH8l2gClZX8zof7jij7-uwz5pFmOTQ",
  "customer": {
    "name": "Anna",
    "phone": "+7 900 123-45-67",
    "email": "anna@example.com",
    "telegram_chat_id": 123456789
  }
}
```

`name` (1-100 characters) and `phone` (7-15 digits, may contain spaces,
`+`, `-`, parentheses) are required; `email` and `telegram_chat_id` are
optional. With a chat id and a configured bot, notifications go through
Telegram; otherwise they go to the server log.

`201 Created`:

```json
{
  "booking": {
    "id": "6b236920-9d88-44bb-94d1-4fa5b116edeb",
    "kind": "appointment",
    "status": "confirmed",
    "location_id": 2,
    "staff_id": 4,
    "service_id": 5,
    "start": "2026-10-14T10:00:00+02:00",
    "end": "2026-10-14T10:45:00+02:00",
    "busy_until": "2026-10-14T11:00:00+02:00",
    "time_zone": "Europe/Berlin",
    "price": {"amount_minor": 3800, "currency": "EUR"},
    "hold_expires_at": "2026-10-12T10:10:00+02:00",
    "customer": {"name": "Anna", "phone": "+7 900 123-45-67", "email": "anna@example.com"},
    "note": "",
    "rescheduled_from": null,
    "created_at": "2026-10-12T10:00:00+02:00",
    "confirmed_at": "2026-10-12T10:02:13+02:00",
    "cancelled_at": null
  },
  "manage_token": "6UIR8Tawz-sa4X4JsPUweOI6ObLuxaZmAHsrvGrzpGk"
}
```

`end` is when the service ends; `busy_until` adds the cleanup buffer.
The manage token is shown only here; the server stores a hash of it.

Errors: `hold_not_found`, `hold_expired`, `already_confirmed`,
`validation_failed`, `idempotency_key_reused`.

Confirming queues a confirmation message, a reminder 24 hours before and one
2 hours before the start (reminders whose time has already passed are not
created).

## Manage link

These routes take the manage token in the `X-Manage-Token` header. A wrong
token answers `404 not_found`, the same as a missing booking.

- `GET /v1/bookings/{bookingID}` → the booking object.
- `POST /v1/bookings/{bookingID}/cancel` (no body) → the cancelled booking.
  Allowed until the cancellation cutoff before the start (3 hours by
  default); after that `409 too_late`. Pending reminders are voided and a
  cancellation notice is queued.
- `POST /v1/bookings/{bookingID}/reschedule` with `{"start": "2026-10-14T10:15:00+02:00"}`
  → the new booking. Same staff member and service; the start must be an
  offered slot (the booking's own time does not count as busy). The old
  booking becomes `cancelled`, the new one has a new `id`,
  `rescheduled_from` pointing at the old one, and the same manage token.
  Errors: `too_late`, `not_active`, `invalid_slot`, `slot_taken`.

## Admin

Every admin route needs `Authorization: Bearer <SLOTBOOK_ADMIN_TOKEN>`.

### `GET /v1/admin/bookings?location_id=2&date=2026-10-14`

Appointments (held, confirmed, cancelled) and blocks overlapping that local
date, ordered by start.

```json
{
  "location_id": 2,
  "date": "2026-10-14",
  "time_zone": "Europe/Berlin",
  "bookings": [ { "...": "booking objects; blocks have kind block, service_id null, price null" } ]
}
```

### `POST /v1/admin/bookings/{bookingID}/cancel`

Cancels a confirmed appointment regardless of the cutoff. Returns the
booking; `409 not_active` if it is not confirmed.

### `POST /v1/admin/blocks`

```json
{"staff_id": 4, "start": "2026-10-14T13:00:00+02:00", "end": "2026-10-14T14:00:00+02:00", "note": "training"}
```

`201 Created` with a booking object of kind `block`. A block overlapping an
active booking or block is `409 slot_taken`; `end` must be after `start` and
at most 31 days later (`422 validation_failed`).

### `DELETE /v1/admin/blocks/{blockID}`

`204 No Content`, or `404 not_found`.

### `GET /v1/admin/notifications`

Optional filters: `location_id`, `booking_id`, `status` (`pending`, `sent`,
`failed`, `voided`, `skipped`), `limit` (1-500, default 100). Newest send
time first.

```json
{
  "notifications": [
    {
      "id": 1,
      "booking_id": "6b236920-9d88-44bb-94d1-4fa5b116edeb",
      "kind": "confirmation",
      "status": "sent",
      "send_at": "2026-10-12T08:02:13Z",
      "next_attempt_at": "2026-10-12T08:04:18Z",
      "attempts": 1,
      "created_at": "2026-10-12T08:02:13Z",
      "sent_at": "2026-10-12T08:02:18Z"
    }
  ]
}
```

`kind` is one of `confirmation`, `reminder_24h`, `reminder_2h`,
`cancellation`. `voided` rows were cancelled by a later change to the
booking; `skipped` rows were dropped by the worker because the booking no
longer matched them when they came due; `failed` rows ran out of retries
(`last_error` says why).

## CORS

Browsers may call the API from the origins in `SLOTBOOK_CORS_ORIGINS`
(comma-separated, `*` for any). Allowed request headers: `Authorization`,
`Content-Type`, `Idempotency-Key`, `X-Manage-Token`, `X-Request-ID`. Exposed
response headers: `Idempotent-Replayed`, `X-Request-ID`.
