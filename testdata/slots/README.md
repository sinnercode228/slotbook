# Slot generation vectors

Each `*.json` file here is one self-contained case for the slot generator:
the full input and the exact expected output. The Go test in
`internal/slots/golden_test.go` runs every file; any other implementation of
the generator should run the same files and produce the same strings.

## Format

```json
{
  "description": "what the case pins down",
  "input": {
    "time_zone": "Europe/Berlin",
    "now": "2026-03-20T08:00:00Z",
    "from": "2026-03-29",
    "to": "2026-03-29",
    "duration_minutes": 60,
    "buffer_after_minutes": 0,
    "slot_step_minutes": 30,
    "min_notice_minutes": 0,
    "horizon_days": 60,
    "weekly_hours": {"sunday": [{"start": "01:00", "end": "05:00"}]},
    "overrides": [{"date": "2026-03-30", "intervals": []}],
    "busy": [{"start": "2026-03-29T03:00:00+02:00", "end": "2026-03-29T03:30:00+02:00"}]
  },
  "expected": [
    {"date": "2026-03-29", "slots": ["2026-03-29T01:00:00+01:00", "..."]}
  ]
}
```

- `time_zone` is an IANA zone name. All wall-clock values are in this zone.
- `now`, `busy[].start` and `busy[].end` are RFC 3339 instants (any offset),
  with at most millisecond precision: the TypeScript port counts time in
  milliseconds.
- `from` and `to` are local dates (`YYYY-MM-DD`), both inclusive.
- `weekly_hours` maps lowercase English day names (`monday` ... `sunday`) to
  working intervals; a missing day has no hours. Times are `HH:MM`, the end is
  exclusive and may be `24:00`.
- `overrides` replace the weekly hours on their date; `"intervals": []`
  closes the day.
- `busy` intervals are half-open `[start, end)`, and already include the
  buffer of the bookings they come from.
- `expected` has one entry per date from `from` to `to`, in order, with
  `slots: []` when nothing is available. Each slot is an RFC 3339 string with
  seconds, no fraction, and the offset of `time_zone` at that instant.

## Rules

1. Dates are walked as calendar days, never by adding 24 hours.
2. Starts lie on the `slot_step_minutes` grid counted from local midnight
   (step 20 and an interval from 09:10 give 09:20, 09:40, ...).
3. A start whose wall-clock time does not exist (spring-forward gap) is
   skipped. A wall-clock time that happens twice (fall-back) means its first
   occurrence; the repeated hour adds no starts of its own.
4. An interval edge is the first instant at which the local clock shows that
   time or later. The interval therefore has its real length on DST days
   (01:00-05:00 is 3 hours in spring and 5 hours in autumn in Berlin).
5. A start is offered when `[start, start + duration + buffer)` fits inside a
   single working interval, overlaps no busy interval, starts no earlier than
   `now + min_notice_minutes`, and its date lies between today and
   today + `horizon_days` (today taken in `time_zone`).
