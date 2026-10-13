package booking_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/sinnercode228/slotbook/internal/booking"
	"github.com/sinnercode228/slotbook/internal/catalog"
	"github.com/sinnercode228/slotbook/internal/clock"
	"github.com/sinnercode228/slotbook/internal/config"
	"github.com/sinnercode228/slotbook/internal/dbtest"
	"github.com/sinnercode228/slotbook/internal/seed"
	"github.com/sinnercode228/slotbook/internal/slots"
)

// Monday 2026-10-12, 10:00 in Berlin.
var mondayMorning = time.Date(2026, time.October, 12, 8, 0, 0, 0, time.UTC)

var berlin = mustZone("Europe/Berlin")

func mustZone(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return loc
}

// at returns a wall-clock time in Berlin.
func at(year int, month time.Month, day, hour, minute int) time.Time {
	return time.Date(year, month, day, hour, minute, 0, 0, berlin)
}

type fixture struct {
	pool    *pgxpool.Pool
	clock   *clock.Fake
	manager *booking.Manager
	ids     seed.Result
}

func (f fixture) location() int64 { return f.ids.Locations["berlin"] }
func (f fixture) haircut() int64  { return f.ids.Services["cut"] }
func (f fixture) shave() int64    { return f.ids.Services["shave"] }
func (f fixture) lena() int64     { return f.ids.Staff["lena"] }
func (f fixture) jonas() int64    { return f.ids.Staff["jonas"] }

// newFixture seeds one Berlin location with the default settings (15 minute
// step, 2h notice, 60 day horizon, 10 minute holds, 3h cancellation cutoff).
// Lena and Jonas both cut hair (45 minutes plus 15 minutes of cleanup) on
// weekdays 09:00-18:00; only Jonas shaves.
func newFixture(t *testing.T, opts ...dbtest.Option) fixture {
	t.Helper()
	pool := dbtest.New(t, opts...)
	var weekdays slots.WeeklyHours
	for d := time.Monday; d <= time.Friday; d++ {
		weekdays[d] = []slots.Interval{{Start: 9 * 60, End: 18 * 60}}
	}
	ids, err := seed.Apply(context.Background(), pool, seed.Chain{Locations: []seed.Location{{
		Slug: "berlin", Name: "Snipwell Berlin", TimeZone: "Europe/Berlin",
		Services: []seed.Service{
			{Key: "cut", Name: "Haircut", DurationMinutes: 45, BufferAfterMinutes: 15, PriceMinor: 3800, Currency: "EUR"},
			{Key: "shave", Name: "Shave", DurationMinutes: 30, PriceMinor: 2500, Currency: "EUR"},
		},
		Staff: []seed.Staff{
			{Key: "lena", Name: "Lena H.", Services: []string{"cut"}, WeeklyHours: weekdays},
			{Key: "jonas", Name: "Jonas W.", Services: []string{"cut", "shave"}, WeeklyHours: weekdays},
		},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	clk := clock.NewFake(mondayMorning)
	return fixture{
		pool:    pool,
		clock:   clk,
		manager: booking.NewManager(pool, catalog.NewStore(pool, config.LocationDefaults), clk),
		ids:     ids,
	}
}

// newRaceFixture is newFixture with a connection for each of n goroutines
// and a short deadlock_timeout, so that PostgreSQL resolves a deadlock
// between simultaneous inserts in milliseconds instead of a second.
func newRaceFixture(t *testing.T, n int32) fixture {
	t.Helper()
	return newFixture(t, dbtest.WithMaxConns(n), dbtest.WithSetting("deadlock_timeout", "10ms"))
}

// race runs fn in n goroutines released at the same moment and returns
// their errors in goroutine order.
func race(n int, fn func(i int) error) []error {
	var (
		wg    sync.WaitGroup
		ready = make(chan struct{})
		errs  = make([]error, n)
	)
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-ready
			errs[i] = fn(i)
		}()
	}
	close(ready)
	wg.Wait()
	return errs
}

// checkOneWinner fails the test unless exactly one of errs is nil, all the
// others are ErrSlotTaken and Lena has exactly one active row over
// [start, end).
func (f fixture) checkOneWinner(t *testing.T, round int, errs []error, start, end time.Time) {
	t.Helper()
	won := 0
	for _, err := range errs {
		switch {
		case err == nil:
			won++
		case !errors.Is(err, booking.ErrSlotTaken):
			t.Fatalf("round %d: got %v, want ErrSlotTaken", round, err)
		}
	}
	if won != 1 {
		t.Fatalf("round %d: %d of %d calls succeeded, want 1", round, won, len(errs))
	}
	var active int
	err := f.pool.QueryRow(context.Background(), `
		SELECT count(*) FROM bookings
		WHERE staff_id = $1 AND status IN ('held', 'confirmed') AND period && tstzrange($2, $3, '[)')`,
		f.lena(), start, end).Scan(&active)
	if err != nil {
		t.Fatal(err)
	}
	if active != 1 {
		t.Fatalf("round %d: %d active rows over the slot, want 1", round, active)
	}
}

func (f fixture) clearBookings(t *testing.T) {
	t.Helper()
	if _, err := f.pool.Exec(context.Background(), `DELETE FROM bookings`); err != nil {
		t.Fatal(err)
	}
}

var anna = booking.Customer{Name: "Anna", Phone: "+7 900 123-45-67", Email: "anna@example.com"}

func (f fixture) hold(t *testing.T, staffID int64, start time.Time) booking.Hold {
	t.Helper()
	h, err := f.manager.Hold(context.Background(), booking.HoldRequest{ServiceID: f.haircut(), StaffID: staffID, Start: start})
	if err != nil {
		t.Fatalf("hold %s with staff %d: %v", start, staffID, err)
	}
	return h
}

func (f fixture) book(t *testing.T, staffID int64, start time.Time) booking.Confirmation {
	t.Helper()
	h := f.hold(t, staffID, start)
	c, _, err := f.manager.Confirm(context.Background(), booking.ConfirmRequest{HoldToken: h.Token, Customer: anna}, "")
	if err != nil {
		t.Fatalf("confirm: %v", err)
	}
	return c
}

type outboxRow struct {
	Kind   string
	Status string
	SendAt time.Time
}

func (f fixture) outbox(t *testing.T, bookingID string) map[string]outboxRow {
	t.Helper()
	rows, err := f.pool.Query(context.Background(),
		`SELECT kind, status, send_at FROM notifications WHERE booking_id = $1`, bookingID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]outboxRow{}
	for rows.Next() {
		var r outboxRow
		if err := rows.Scan(&r.Kind, &r.Status, &r.SendAt); err != nil {
			t.Fatal(err)
		}
		out[r.Kind] = r
	}
	return out
}

func (f fixture) status(t *testing.T, bookingID string) booking.Status {
	t.Helper()
	var s booking.Status
	if err := f.pool.QueryRow(context.Background(), `SELECT status FROM bookings WHERE id = $1`, bookingID).Scan(&s); err != nil {
		t.Fatal(err)
	}
	return s
}
