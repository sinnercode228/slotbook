# Slotbook

Online booking for small service businesses such as barbershops and salons: a
booking widget that customers use on their phones, a link to reschedule or
cancel, and a day schedule for the staff. Go and PostgreSQL on the server,
React on the client.

<p>
  <img src="docs/screenshots/widget-time.png" alt="Choosing a time on a phone" width="260">
  <img src="docs/screenshots/widget-taken.png" alt="The slot was taken in the meantime" width="260">
  <img src="docs/screenshots/widget-confirmed.png" alt="Booking confirmed" width="260">
</p>

![The admin day schedule](docs/screenshots/admin-day.png)

The `pages` workflow publishes a demo of the widget to GitHub Pages. It runs
the booking rules in the browser on the same seed data as the server, so it
needs no backend: open it in two tabs and race for one slot, or press
"Someone else books a slot" in the demo bar and then pick that time.

## What it does

- **Booking widget**: salon, service, specialist or "any available", a week of
  free times, then a hold that reserves the slot for ten minutes with a
  visible countdown while the customer enters their details. Times are shown
  in the salon's time zone; a visitor in another zone sees their own time
  too. The confirmation offers an `.ics` file and a manage link.
- **Manage link**: reschedule (same specialist, any offered time) or cancel,
  until the salon's cutoff.
- **Admin**: one column per specialist, with held, confirmed and blocked time
  drawn differently, booking details with cancel, a form to block time, and
  the notification outbox with each message's status and send time.
- **English and Russian**, chosen by `?lang=`, the browser language or the
  switch in the header.

## How it stays correct

- **No double booking.** Holds, confirmed appointments and admin blocks share
  one table, and `EXCLUDE USING gist (staff_id WITH =, period WITH &&)` over
  the active rows rejects any overlap, cleanup buffer included. The API turns
  the violation into `409 slot_taken`; the widget removes the slot, says so
  plainly and returns to the list.
- **Time zones and DST.** Slots are computed from calendar dates and wall-clock
  working hours: a time skipped by a spring-forward jump is not offered, and a
  repeated one means its first occurrence. The rules are pinned by the vectors
  in [`testdata/slots`](testdata/slots/README.md), which the Go generator and
  its TypeScript port in `web/src/schedule` both have to pass.
- **Safe retries.** Confirming takes an `Idempotency-Key`, so a retry after a
  dropped connection returns the first result instead of failing on a hold
  that is already confirmed.
- **Notifications** (confirmation, reminders 24 and 2 hours ahead,
  cancellation) are written to an outbox in the transaction that changes the
  booking, and a worker delivers them with leased claims and retries.
- **The demo** keeps its state in IndexedDB. Every call is one readwrite
  transaction, which the browser runs one at a time across tabs, so two tabs
  cannot both hold a slot. localStorage cannot guarantee that: in Chromium a
  write reaches other tabs asynchronously, even under a Web Lock.

## Layout

| Path | |
|---|---|
| `cmd/slotbook` | `serve`, `migrate` and `seed` |
| `internal/slots` | slot generation, pure Go |
| `internal/booking` | holds, confirmation, cancel, reschedule, admin blocks |
| `internal/notify` | outbox planning and the delivery worker (log or Telegram) |
| `internal/httpapi` | the HTTP API; it also serves the built widget |
| `web/src/widget`, `manage`, `admin` | the three screens |
| `web/src/api` | one `BookingApi` interface with an HTTP and a mock implementation |
| `web/src/mock` | the rules of `internal/booking`, for the in-browser demo |
| `seed/demo.json` | a fictional two-city chain, read by the seed command and the demo |
| `docs/api.md` | the API reference |

## Running it

With Docker:

```sh
docker compose up --build
```

This starts PostgreSQL, applies the migrations, loads the demo chain and
serves the API and the widget on http://localhost:8080. The admin token
defaults to the one in `docker-compose.yml`; set `SLOTBOOK_ADMIN_TOKEN` to
override it.

Without Docker, against a local PostgreSQL 16:

```sh
cp .env.example .env    # then adjust SLOTBOOK_DATABASE_URL
set -a; . ./.env; set +a
go run ./cmd/slotbook migrate
go run ./cmd/slotbook seed
go run ./cmd/slotbook serve

cd web
npm ci
npm run dev             # http://localhost:5173, proxies /v1 to :8080
```

To run the widget without a server, as on GitHub Pages:

```sh
cd web
VITE_API_MODE=mock npm run dev
```

The web settings are listed in [`web/.env.example`](web/.env.example), the
server settings in [`.env.example`](.env.example).

## Tests

```sh
# Go: unit tests, plus integration tests when a database is given
SLOTBOOK_TEST_DATABASE_URL='postgres://postgres:postgres@localhost:5432/slotbook_test?sslmode=disable' \
  go test -race ./...

# Web: ESLint and the type check, then Vitest
cd web
npm run lint
npm test
```

The web tests run every golden slot vector against the TypeScript generator,
check the overlap, expiry, idempotency, cutoff and reschedule rules of the
mock, and render the slot picker and the contact form.

## License

MIT
