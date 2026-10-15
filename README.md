# Slotbook

[![CI](https://github.com/sinnercode228/slotbook/actions/workflows/ci.yml/badge.svg)](https://github.com/sinnercode228/slotbook/actions/workflows/ci.yml)

Русская версия: [README.ru.md](README.ru.md)

Slotbook is a booking backend for small service businesses such as barbershops. A customer picks a salon, a service, a specialist and a time, holds the slot for ten minutes while typing their details, and gets a link to reschedule or cancel; staff get a day calendar and a log of outgoing messages. The guarantee the design starts from: two customers can never book the same specialist for overlapping time. PostgreSQL enforces it, not a check in Go.

Demo: https://sinnercode228.github.io/slotbook/ runs the widget and the staff view in the browser on the same seed data, with no server behind it. To see the guarantee, open it in two tabs, choose Snipwell Berlin, Haircut and the same specialist in both, and click the same time in both tabs. With one tab, press "Someone else books a slot" in the dark bar while the times are on screen, then click the time it names. "Staff login" in the footer opens the day view (the demo accepts any token), and "Reset" restores the seed data.

<p>
  <img src="docs/screenshots/race-won.png" alt="Tab A holds Thursday 10:30 with Jonas W. and shows a 9:58 countdown" width="260">
  <img src="docs/screenshots/race-lost.png" alt="Tab B is told 10:30 was just booked by someone else; its morning times are gone" width="260">
</p>

Both tabs clicked 10:30 with Jonas W. at the same moment. Tab B also lost 10:00 and 10:15: a haircut takes 45 minutes plus a 15-minute cleanup buffer, so both would run into the held 10:30.

Stack: Go 1.24 with `net/http` routing and pgx v5 (its only direct dependency), PostgreSQL 16 with `btree_gist`; React 19, TypeScript, Vite and Vitest for the widget, the manage page and the staff view.

Snipwell & Co., its two salons in Moscow and Berlin, the staff and the customers are made up; the code and the tests are real.

## Where double booking is ruled out

Appointments, holds and staff time blocks share one table, `bookings`. Each row stores the time the specialist is taken as `period = [starts_at, busy_until)`, where `busy_until` is the end of the service plus its cleanup buffer. The guarantee is one constraint in [`0001_schema.sql`](internal/db/migrations/0001_schema.sql#L111-L113):

```sql
-- The double-booking guarantee: active rows of one staff member never overlap.
CONSTRAINT bookings_no_overlap EXCLUDE USING gist (staff_id WITH =, period WITH &&)
    WHERE (status IN ('held', 'confirmed'))
```

`btree_gist` lets one GiST index combine equality on `staff_id` with the range overlap operator `&&`. The ranges are half-open, so a booking may start exactly when the previous one's buffer ends. Cancelled and expired rows fall outside the `WHERE`: they stay in the table as history and block nothing. A CHECK, `bookings_period_matches`, keeps `period` equal to `tstzrange(starts_at, busy_until, '[)')`, so no row can carry a period that disagrees with its times.

The Go code never looks for overlaps before inserting. `Hold` checks only that the start is one the slot generator offers for the specialist's working hours ([`offers`](internal/booking/availability.go#L140-L149) passes no busy time), then inserts and lets PostgreSQL decide ([`insertHold`](internal/booking/hold.go#L133-L156)). A conflicting insert fails with SQLSTATE `23P01`, which becomes `ErrSlotTaken` and `409 slot_taken`. A read-then-insert check would race; the constraint does not, because the second insert waits for the transaction that holds the conflicting row:

```mermaid
sequenceDiagram
    participant A as Tab A
    participant B as Tab B
    participant API as POST /v1/holds
    participant PG as PostgreSQL
    A->>API: Jonas W., Thu 15 Oct 10:30
    B->>API: Jonas W., Thu 15 Oct 10:30
    API->>PG: tx A, expire lapsed holds, INSERT held row [10:30, 11:30)
    API->>PG: tx B, expire lapsed holds, INSERT held row [10:30, 11:30)
    Note over PG: tx B overlaps an uncommitted row of tx A and waits for it
    PG-->>API: tx A commits
    API-->>A: 201 with hold_token and expires_at, now + 10 min
    PG-->>API: tx B fails with 23P01 on bookings_no_overlap
    API-->>B: 409 slot_taken
```

Two inserts that arrive at the same moment can also each find the other's uncommitted row and wait for it. So can an insert and the confirmation of a hold it overlaps, because confirming writes a new version of the held row. PostgreSQL's deadlock detector then aborts one of them with `40P01` after `deadlock_timeout` (1 s by default). Holds, confirmations, blocks and reschedules run their transactions through [`inTx`](internal/booking/tx.go#L20-L54), which runs a transaction aborted this way again from the start (at most five runs in all), so instead of a `500` the loser still gets `409 slot_taken` and the confirmation goes through.

The same constraint covers every other path that puts time on a calendar:

- An admin block is a row with `kind = 'block'`, so a block over a booking is a `409` as well ([`admin.go`](internal/booking/admin.go#L28-L64)).
- "Any available specialist" orders the candidates by confirmed bookings that day, then by id, and moves to the next one on `slot_taken` ([`hold.go:58-90`](internal/booking/hold.go#L58-L90)).
- A reschedule cancels the old row and inserts the new one in one transaction. The order matters: the constraint is checked row by row, so moving a booking 15 minutes later onto part of its own period would otherwise conflict with itself ([`manage.go:134-155`](internal/booking/manage.go#L134-L155)).

[`TestConcurrentHoldsOnTheSameSlotHaveOneWinner`](internal/booking/hold_test.go#L76-L121) releases 20 goroutines at once against the same slot through a pool of 25 connections and expects exactly one hold and 19 `ErrSlotTaken`.

The demo has no PostgreSQL, so [`web/src/mock`](web/src/mock/engine.ts) runs a TypeScript port of these rules and keeps its state in IndexedDB. Every API call is one `readwrite` transaction, and the browser runs those one at a time across tabs, which is why the two-tab race above ends the same way as on the server. localStorage would not do: in Chromium a write in one tab reaches the others asynchronously, even under a Web Lock ([`storage.ts`](web/src/mock/storage.ts#L52-L59)).

Write-up on why the constraint decides and not a check in Go, what the second insert waits for, and when a loser gets `500` instead of `409`: [Two tabs, one slot: letting PostgreSQL refuse the second booking](https://github.com/sinnercode228/sinnercode228/blob/main/notes/slotbook-exclusion-constraint.md).

## Holds: ten minutes, then the slot is free again

Picking a time creates a row with `status = 'held'` and `hold_expires_at` ten minutes ahead (`hold_ttl_seconds` per location; the defaults are in [`config.go`](internal/config/config.go#L28-L34)). The client gets a 32-byte random hold token, and the table stores only its SHA-256 hash.

No background job has to run for a hold to expire. Three places compare `hold_expires_at` with the current time:

1. Before inserting a hold, a block or a rescheduled booking, the same transaction marks that specialist's lapsed overlapping holds as expired ([`expireStaleHolds`](internal/booking/store.go#L59-L69)), so a hold that ran out a second ago never causes a `409`.
2. Availability counts a hold as busy only while `hold_expires_at > now` ([`busyIntervals`](internal/booking/store.go#L71-L87)).
3. Confirming is one `UPDATE ... WHERE hold_token_hash = $1 AND status = 'held' AND hold_expires_at > $7` ([`confirm.go:78-84`](internal/booking/confirm.go#L78-L84)). When it matches no row, a second query tells which answer applies: `404 hold_not_found`, `409 already_confirmed` or `410 hold_expired`.

The sweeper in `serve` ([`sweep.go`](internal/booking/sweep.go)) runs every `SLOTBOOK_SWEEP_INTERVAL` (30 s by default) and marks the remaining lapsed holds as expired. That only keeps the staff view tidy; correctness does not depend on it. It also deletes idempotency keys older than 24 hours.

Confirming takes an `Idempotency-Key` header. The key is inserted with `ON CONFLICT DO NOTHING` inside the confirming transaction, so a concurrent retry with the same key waits on that insert until the first request commits, then reads the stored response and returns it with `Idempotent-Replayed: true` ([`confirm.go:126-158`](internal/booking/confirm.go#L126-L158)). The stored response includes the plain manage token, because a client that lost the first response needs it; the 24-hour pruning limits how long it sits in the table. The same key with another body is `422 idempotency_key_reused`. The widget makes one key per hold, so pressing "Confirm booking" again after a dropped connection returns the first result, manage link included, instead of `409 already_confirmed`.

The countdown does not trust the device clock. The widget takes the hold length from the server's `expires_at - created_at` and counts it down from the moment the hold arrived ([`BookingWidget.tsx:190`](web/src/widget/BookingWidget.tsx#L190)). At zero it says the time was released and sends the customer back to the list; a confirm that loses to the clock gets `410` and ends the same way. A reload keeps the step, the running hold and the typed details, because the widget saves its progress in `sessionStorage` ([`progress.ts`](web/src/widget/progress.ts)).

![The staff day view for Snipwell Berlin on Wednesday 14 October: a held slot with a dashed border, confirmed bookings, two blocks, and the outbox with pending reminders](docs/screenshots/admin-day.png)

In the day view a running hold has a dashed border and stripes, confirmed bookings are solid and blocks are hatched. The strip below the dashed line inside a booking is its cleanup buffer, which the constraint covers too.

## Holds per client address

`POST /v1/holds` needs no login, so without a limit a script could keep every slot of a salon held, ten minutes at a time. Each client address therefore gets a token bucket ([`ratelimit.go`](internal/ratelimit/ratelimit.go)): 10 holds at once, then one every 30 seconds (`SLOTBOOK_HOLD_BURST=10`, `SLOTBOOK_HOLD_RATE=2/min`). With ten-minute holds that is at most about 30 live holds per address. Past that the answer is `429 rate_limited` with `Retry-After` in seconds, given before the request reaches the database, and the widget tells the visitor how long to wait. Confirming is not limited: it needs a hold token, so the hold budget already bounds it, and a retry after a lost connection must not be refused.

The client address is the TCP peer. `X-Forwarded-For` counts only when the peer is listed in `SLOTBOOK_TRUSTED_PROXIES`, and then it is read from the right. Each proxy appends the address it saw, so the first entry that is not a trusted proxy is the client, and anything to the left of it was written by the client and is ignored ([`clientaddr.go`](internal/httpapi/clientaddr.go)). An entry that is not an address stops the walk at the proxy that passed it on. IPv6 clients are counted per /64, the block one subscriber usually gets whole.

A bucket is stored as the time at which it will be full again. Past that time it answers like a new bucket, so a sweep once per refill window drops it, and memory holds only the addresses that asked for a hold, whatever the answer, in the last two windows (ten minutes with the defaults). The buckets live in the memory of the API process; Known limitations below says what that costs.

## Berlin on 29 March and 25 October

Working hours are local wall-clock times per weekday, and the generator in [`internal/slots`](internal/slots/slots.go) turns them into instants in the location's zone. It walks calendar dates and never adds 24 hours. Each wall time is resolved explicitly in [`wallclock.go`](internal/slots/wallclock.go): `occurrences` builds candidates from the UTC offsets in effect 15 hours either side and keeps the ones that read back as the same wall time, because `time.Date` does not document which instant it picks inside a gap or an overlap. The rules are pinned by JSON vectors in [`testdata/slots`](testdata/slots/README.md):

- Europe/Berlin, 2026-03-29: clocks jump from 02:00 to 03:00. With hours 01:00-05:00, a 60-minute service and a 30-minute step, the starts are 01:00 and 01:30 (+01:00), then 03:00, 03:30 and 04:00 (+02:00). 02:00 and 02:30 do not exist; 01:30 runs across the jump.
- The same interval lasts three real hours that day, so a three-hour service fits once, at 01:00.
- Europe/Berlin, 2026-10-25: 02:00-03:00 happens twice. A repeated time means its first (summer) occurrence, and the second hour adds no starts: with hours 01:00-04:00 the starts are 01:00, 01:30, 02:00, 02:30 (+02:00) and 03:00 (+01:00).
- America/New_York, 2026-11-01: a booking at the second (EST) 01:30 does not block the first (EDT) 01:30, because busy time is compared as instants.
- Minimum notice is elapsed time: at 01:30 winter time on 2026-03-29, 120 minutes of notice end at 04:30 summer time, not 03:30.
- Australia/Lord_Howe moves its clocks by 30 minutes on 2026-10-04, America/Santiago skips midnight itself on 2026-09-06, and Africa/Cairo goes back at 24:00 on 2026-10-29.

There are 23 vectors. [`golden_test.go`](internal/slots/golden_test.go) runs each file against the Go generator, and [`golden.test.ts`](web/src/schedule/golden.test.ts) runs the same files against the TypeScript port the demo uses, which resolves wall times the same way on top of `Intl.DateTimeFormat` ([`zone.ts`](web/src/schedule/zone.ts)). The API answers in RFC 3339 with the location's offset.

<p>
  <img src="docs/screenshots/zone-en-23.png" alt="Friday 23 October: Berlin is GMT+2, 09:00 in Berlin is 10:00 in Moscow" width="260">
  <img src="docs/screenshots/zone-en-26.png" alt="Monday 26 October: Berlin is GMT+1, 09:00 in Berlin is 11:00 in Moscow" width="260">
</p>

The demo with the browser in Europe/Moscow, booking Lena H. in Berlin. On Friday 23 October 09:00 in Berlin is 10:00 in Moscow; after Berlin's clocks go back, on Monday 26 October, the same 09:00 is 11:00. Lena's first slot is 09:00 Berlin time on both days.

Write-up on how the generator walks dates, how a wall time is resolved and what the vectors pin in each zone: [Berlin on 29 March: booking slots across a DST change without adding 24 hours](https://github.com/sinnercode228/sinnercode228/blob/main/notes/slotbook-dst-slots.md).

## Reminders through the outbox

A booking change and its messages commit together. Confirming inserts rows into `notifications` in the same transaction: the confirmation now, and reminders 24 hours and 2 hours before the start, leaving out any whose time has already passed ([`outbox.go:52-62`](internal/notify/outbox.go#L52-L62)). A cancellation voids the pending rows and queues a cancellation notice. A reschedule voids the old booking's rows and plans a fresh set for the new one. If the transaction rolls back, no message exists; once it commits, the messages are in the database and wait for the worker.

A worker inside `serve` polls every `SLOTBOOK_WORKER_INTERVAL` (5 s by default) and claims up to 20 due rows at a time ([`worker.go:100-127`](internal/notify/worker.go#L100-L127)):

```sql
UPDATE notifications n
SET attempts = n.attempts + 1, next_attempt_at = $2
FROM (
    SELECT id FROM notifications
    WHERE status = 'pending' AND next_attempt_at <= $1
    ORDER BY next_attempt_at, id
    LIMIT $3
    FOR UPDATE SKIP LOCKED
) due
WHERE n.id = due.id
```

`$2` is now plus a two-minute lease. `SKIP LOCKED` lets several workers claim different rows, and the claim commits at once, so the send (a Telegram `sendMessage` call or a log line) holds no row lock and no pooled connection. Right before each send the worker renews its lease with `WHERE id = $1 AND attempts = $2 AND status = 'pending'`, and every later write is fenced on the same attempt number. A worker whose lease ran out therefore neither sends a row that another worker has claimed since nor overwrites that newer attempt's outcome ([`renew`](internal/notify/worker.go#L215-L229), [`record`](internal/notify/worker.go#L265-L280)). Before sending it also re-reads the booking: a reminder for a cancelled booking or for an appointment that has started is marked `skipped`. A failed send is retried after 30 s, 1, 2 and 4 minutes, and the fifth failure marks the row `failed`.

[`TestConcurrentWorkersSendEachNotificationOnce`](internal/notify/worker_test.go#L117-L163) runs three workers over 100 due rows and checks that each message went out exactly once, on the first attempt. The staff view lists the outbox with each row's status and send time.

## Known limitations

- The hold limit is counted per API process, in memory. Two processes behind a load balancer give each address two budgets, and a restart forgets every bucket. It is keyed by address, so customers behind one NAT share a budget, and a script spread over many addresses is slowed down, not stopped.
- There is no call to release a hold. A customer who goes back and picks another time leaves the first one blocked until it expires; if they pick it again, the widget reuses their own hold.
- Delivery is at least once. A worker that dies after a send and before it records the outcome leaves the row to be sent again when its lease ends.
- Messages go to Telegram when the customer gave a chat id and `SLOTBOOK_TELEGRAM_BOT_TOKEN` is set, and to the server log otherwise. Phone and email are stored, but nothing sends to them.
- Locations, services, staff and working hours come only from a seed file; the API cannot edit them. The admin API is guarded by one shared bearer token, not by staff accounts.

## Running it

The fastest way is the [demo](https://sinnercode228.github.io/slotbook/). [`pages.yml`](.github/workflows/pages.yml) builds it with `VITE_API_MODE=mock` and `BASE_PATH=/<repo>/`, and the mock bundles [`seed/demo.json`](seed/demo.json), the file the server's `seed` command loads.

With Docker, `docker compose up --build` starts PostgreSQL 16, runs `migrate` and `seed` once, then serves the API and the widget on http://localhost:8080. The admin token defaults to the one in [`docker-compose.yml`](docker-compose.yml); set `SLOTBOOK_ADMIN_TOKEN` to override it.

Locally, with Go 1.24, Node 22.12 or newer and a PostgreSQL 16 server (the URLs below assume `postgres:postgres@localhost:5432`):

```sh
git clone https://github.com/sinnercode228/slotbook.git && cd slotbook
psql postgres://postgres:postgres@localhost:5432/postgres -c 'CREATE DATABASE slotbook_dev'
cp .env.example .env
set -a; . ./.env; set +a
go run ./cmd/slotbook migrate
go run ./cmd/slotbook seed
(cd web && npm ci && npm run build)
SLOTBOOK_STATIC_DIR=web/dist go run ./cmd/slotbook serve
```

The widget is at http://localhost:8080 and the day view at http://localhost:8080/#/admin, which asks for `SLOTBOOK_ADMIN_TOKEN` from `.env`. For hot reload, run `npm run dev` in `web/` as well and open http://localhost:5173; Vite proxies `/v1` to `SLOTBOOK_API_TARGET` (http://localhost:8080). The race from the top, against the real server:

```sh
# pick a start from GET /v1/locations/2/slots?service_id=5&staff_id=5
start='2026-10-15T10:30:00+02:00'
body='{"service_id":5,"staff_id":5,"start":"'$start'"}'
for i in 1 2; do
  curl -s -o /dev/null -w '%{http_code}\n' -X POST -H 'Content-Type: application/json' \
    -d "$body" http://localhost:8080/v1/holds &
done; wait
# 201
# 409
```

Service 5 is the Berlin haircut and staff 5 is Jonas W. in a freshly seeded database. The rest of the API is in [`docs/api.md`](docs/api.md).

| Variable | Default | Purpose |
|---|---|---|
| `SLOTBOOK_DATABASE_URL` | none, required | PostgreSQL connection string for every command |
| `SLOTBOOK_ADDR` | `:8080` | listen address of `serve` |
| `SLOTBOOK_ADMIN_TOKEN` | none, required by `serve` | bearer token for `/v1/admin/*`, at least 32 characters |
| `SLOTBOOK_DEV` | `false` | `true` accepts a shorter admin token |
| `SLOTBOOK_CORS_ORIGINS` | empty | comma-separated browser origins, `*` for any |
| `SLOTBOOK_TELEGRAM_BOT_TOKEN` | empty | without it every message goes to the log |
| `SLOTBOOK_TELEGRAM_API_URL` | `https://api.telegram.org` | Bot API base URL |
| `SLOTBOOK_STATIC_DIR` | empty | directory with the built widget, served at `/` |
| `SLOTBOOK_LOG_LEVEL` | `info` | `debug`, `info`, `warn` or `error` |
| `SLOTBOOK_WORKER_INTERVAL` | `5s` | outbox polling interval |
| `SLOTBOOK_SWEEP_INTERVAL` | `30s` | hold sweeper interval |
| `SLOTBOOK_HOLD_RATE` | `2/min` | holds per client address once the burst is spent: a count per `s`, `min`, `h` or a duration such as `10m`; `0` turns the limit off |
| `SLOTBOOK_HOLD_BURST` | `10` | holds a client address can make at once |
| `SLOTBOOK_TRUSTED_PROXIES` | empty | comma-separated CIDRs or addresses of the reverse proxies whose `X-Forwarded-For` is read |
| `SLOTBOOK_TEST_DATABASE_URL` | empty | integration tests only; they skip without it |

The widget's build settings (`VITE_API_MODE`, `VITE_API_URL`, `VITE_BRAND_NAME`, `SLOTBOOK_API_TARGET`, `BASE_PATH`) are in [`web/.env.example`](web/.env.example).

## Tests

```sh
psql postgres://postgres:postgres@localhost:5432/postgres -c 'CREATE DATABASE slotbook_test'
SLOTBOOK_TEST_DATABASE_URL='postgres://postgres:postgres@localhost:5432/slotbook_test?sslmode=disable' \
  go test -race -count=1 ./...
cd web && npm run lint && npm test
```

Go: 129 test functions and 122 subtests pass, counted as `pass` events in `go test -json`. 60 of the 129 are integration tests against PostgreSQL; each gets its own freshly migrated schema through `search_path` ([`dbtest.go`](internal/dbtest/dbtest.go)) and skips when `SLOTBOOK_TEST_DATABASE_URL` is unset. They cover the constraint, concurrent holds and confirms, expiry without the sweeper, reschedules onto their own period, the cancellation cutoff, the outbox worker's leases and fencing, and the HTTP API end to end, the hold limit included. Unit tests drive the hold limiter with a fake clock (burst, refill, eviction) and check the client address rules (a made-up `X-Forwarded-For`, proxy chains, IPv6). The subtests include the 23 slot vectors.

Web: 124 Vitest tests in 12 files: the same 23 vectors plus a check that the files were found, 28 for the mock's booking rules, 14 for contact validation, 9 for zone arithmetic, and the rest for the HTTP client, the slot picker, the contact form, the widget (a taken slot, a hold refused for too many attempts, an expired hold, a lost connection, a reload), the manage page, `.ics` files and i18n.

[`ci.yml`](.github/workflows/ci.yml) runs gofmt, `go vet`, staticcheck and the race tests against a PostgreSQL 16 service, then the web lint, tests and build, then builds the Docker image.

---

Built by Грешный Котик (sinnercode). I take freelance work like this: Telegram [@sinnercode](https://t.me/sinnercode). License: [MIT](LICENSE).
