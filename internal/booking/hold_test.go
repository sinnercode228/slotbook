package booking_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sinnercode228/slotbook/internal/booking"
	"github.com/sinnercode228/slotbook/internal/dbtest"
)

func TestHoldRejectsStartsTheGeneratorDoesNotOffer(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	cases := map[string]struct {
		start  time.Time
		reason string
	}{
		"off the step grid":    {at(2026, time.October, 14, 10, 5), "not an available start"},
		"outside hours":        {at(2026, time.October, 14, 17, 30), "not an available start"},
		"weekend":              {at(2026, time.October, 17, 10, 0), "not an available start"},
		"inside notice period": {mondayMorning.Add(90 * time.Minute), "notice"},
		"beyond horizon":       {at(2026, time.December, 14, 10, 0), "horizon"},
		"in the past":          {at(2026, time.October, 9, 10, 0), "notice"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := f.manager.Hold(ctx, booking.HoldRequest{ServiceID: f.haircut(), StaffID: f.lena(), Start: c.start})
			var slotErr *booking.SlotError
			if !errors.As(err, &slotErr) {
				t.Fatalf("got %v, want a SlotError", err)
			}
			if !strings.Contains(slotErr.Reason, c.reason) {
				t.Fatalf("reason %q should mention %q", slotErr.Reason, c.reason)
			}
		})
	}
}

func TestHoldRejectsUnknownServiceAndWrongStaff(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	start := at(2026, time.October, 14, 10, 0)
	if _, err := f.manager.Hold(ctx, booking.HoldRequest{ServiceID: 999999, Start: start}); !errors.Is(err, booking.ErrServiceNotFound) {
		t.Fatalf("unknown service: %v", err)
	}
	_, err := f.manager.Hold(ctx, booking.HoldRequest{ServiceID: f.shave(), StaffID: f.lena(), Start: start})
	if !errors.Is(err, booking.ErrInvalidStaff) {
		t.Fatalf("staff without the service: %v", err)
	}
}

func TestOverlappingHoldIsSlotTaken(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	first := f.hold(t, f.lena(), at(2026, time.October, 14, 10, 0))
	if first.Token == "" || first.Booking.Status != booking.StatusHeld {
		t.Fatalf("unexpected hold %+v", first)
	}
	if want := mondayMorning.Add(10 * time.Minute); first.Booking.HoldExpiresAt == nil || !first.Booking.HoldExpiresAt.Equal(want) {
		t.Fatalf("hold expires at %v, want %v", first.Booking.HoldExpiresAt, want)
	}

	// 10:00 + 45 minutes + 15 minutes of cleanup keeps Lena busy until 11:00.
	_, err := f.manager.Hold(ctx, booking.HoldRequest{ServiceID: f.haircut(), StaffID: f.lena(), Start: at(2026, time.October, 14, 10, 45)})
	if !errors.Is(err, booking.ErrSlotTaken) {
		t.Fatalf("overlapping hold: got %v, want ErrSlotTaken", err)
	}
	f.hold(t, f.lena(), at(2026, time.October, 14, 11, 0))
	f.hold(t, f.jonas(), at(2026, time.October, 14, 10, 0))
}

func TestConcurrentHoldsOnTheSameSlotHaveOneWinner(t *testing.T) {
	f := newFixture(t, dbtest.WithMaxConns(25))
	ctx := context.Background()
	start := at(2026, time.October, 14, 10, 0)

	const n = 20
	var (
		wg      sync.WaitGroup
		ready   = make(chan struct{})
		results = make(chan error, n)
	)
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-ready
			_, err := f.manager.Hold(ctx, booking.HoldRequest{ServiceID: f.haircut(), StaffID: f.lena(), Start: start})
			results <- err
		}()
	}
	close(ready)
	wg.Wait()
	close(results)

	won, taken := 0, 0
	for err := range results {
		switch {
		case err == nil:
			won++
		case errors.Is(err, booking.ErrSlotTaken):
			taken++
		default:
			t.Errorf("unexpected error: %v", err)
		}
	}
	if won != 1 || taken != n-1 {
		t.Fatalf("%d holds succeeded and %d got slot_taken; want 1 and %d", won, taken, n-1)
	}
	var active int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM bookings WHERE status = 'held'`).Scan(&active); err != nil {
		t.Fatal(err)
	}
	if active != 1 {
		t.Fatalf("%d active holds in the table", active)
	}
}

func TestExpiredHoldDoesNotBlockANewOne(t *testing.T) {
	f := newFixture(t)
	start := at(2026, time.October, 14, 10, 0)
	stale := f.hold(t, f.lena(), start)

	f.clock.Advance(10 * time.Minute)
	fresh := f.hold(t, f.lena(), start.Add(15*time.Minute))

	if got := f.status(t, stale.Booking.ID); got != booking.StatusExpired {
		t.Fatalf("stale hold is %s, want expired", got)
	}
	if got := f.status(t, fresh.Booking.ID); got != booking.StatusHeld {
		t.Fatalf("new hold is %s", got)
	}
}

func TestAnyStaffPrefersFewestConfirmedBookingsThenLowestID(t *testing.T) {
	f := newFixture(t)
	if f.lena() > f.jonas() {
		t.Fatal("fixture assumes Lena has the lower id")
	}
	ten := at(2026, time.October, 14, 10, 0)

	// A tie on bookings goes to the lower id.
	h := f.hold(t, 0, ten)
	if h.Booking.StaffID != f.lena() {
		t.Fatalf("tie went to staff %d, want Lena", h.Booking.StaffID)
	}

	// Lena: one booking on the 15th, two on the 16th. Jonas: two on the 15th.
	f.book(t, f.lena(), at(2026, time.October, 15, 14, 0))
	f.book(t, f.lena(), at(2026, time.October, 16, 14, 0))
	f.book(t, f.jonas(), at(2026, time.October, 15, 9, 0))
	f.book(t, f.jonas(), at(2026, time.October, 15, 11, 0))
	f.book(t, f.lena(), at(2026, time.October, 16, 9, 0))
	got := f.hold(t, 0, at(2026, time.October, 16, 11, 0))
	if got.Booking.StaffID != f.jonas() {
		t.Fatalf("picked staff %d, want Jonas who has fewer bookings on the 16th", got.Booking.StaffID)
	}
	got = f.hold(t, 0, at(2026, time.October, 15, 15, 0))
	if got.Booking.StaffID != f.lena() {
		t.Fatalf("picked staff %d, want Lena who has fewer bookings on the 15th", got.Booking.StaffID)
	}
}

func TestAnyStaffFallsThroughToTheNextFreeStaffMember(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	ten := at(2026, time.October, 14, 10, 0)

	// Lena is first in line (no bookings, lower id) but already holds 10:00.
	f.hold(t, f.lena(), ten)
	got := f.hold(t, 0, ten)
	if got.Booking.StaffID != f.jonas() {
		t.Fatalf("picked staff %d, want Jonas", got.Booking.StaffID)
	}
	_, err := f.manager.Hold(ctx, booking.HoldRequest{ServiceID: f.haircut(), Start: ten})
	if !errors.Is(err, booking.ErrSlotTaken) {
		t.Fatalf("with everyone busy: got %v, want ErrSlotTaken", err)
	}
}

func TestSweeperExpiresOnlyStaleHolds(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	stale := f.hold(t, f.lena(), at(2026, time.October, 14, 10, 0))
	f.clock.Advance(6 * time.Minute)
	fresh := f.hold(t, f.lena(), at(2026, time.October, 14, 12, 0))
	f.clock.Advance(5 * time.Minute)

	n, err := f.manager.ExpireHolds(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 || f.status(t, stale.Booking.ID) != booking.StatusExpired || f.status(t, fresh.Booking.ID) != booking.StatusHeld {
		t.Fatalf("expired %d; stale %s, fresh %s", n, f.status(t, stale.Booking.ID), f.status(t, fresh.Booking.ID))
	}
}
