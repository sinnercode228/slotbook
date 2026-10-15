package booking_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/sinnercode228/slotbook/internal/booking"
	"github.com/sinnercode228/slotbook/internal/slots"
)

func TestReleaseFreesTheSlotAtOnce(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	ten := at(2026, time.October, 14, 10, 0)
	h := f.hold(t, f.lena(), ten)

	if err := f.manager.Release(ctx, h.Booking.ID, h.Token); err != nil {
		t.Fatalf("release: %v", err)
	}
	if got := f.status(t, h.Booking.ID); got != booking.StatusReleased {
		t.Fatalf("released hold is %s", got)
	}

	// No clock moves: the time and the ones overlapping it are free now, not
	// when the ten minutes would have run out.
	wed := slots.Date{Year: 2026, Month: time.October, Day: 14}
	a, err := f.manager.Availability(ctx, booking.AvailabilityQuery{ServiceID: f.haircut(), StaffID: f.lena(), From: wed, To: wed})
	if err != nil {
		t.Fatal(err)
	}
	offered := func(start time.Time) bool {
		return slices.ContainsFunc(a.Days[0].Slots, start.Equal)
	}
	if !offered(ten) || !offered(ten.Add(15*time.Minute)) {
		t.Fatalf("10:00 and 10:15 are not offered after the release: %v", a.Days[0].Slots)
	}
	f.hold(t, f.lena(), ten.Add(15*time.Minute))

	// Holds have no notifications, and releasing one queues none.
	if got := f.outbox(t, h.Booking.ID); len(got) != 0 {
		t.Fatalf("outbox of a released hold %+v", got)
	}
	day, err := f.manager.ListForDay(ctx, f.location(), wed)
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range day {
		if b.ID == h.Booking.ID {
			t.Fatal("the day view lists the released hold")
		}
	}
}

func TestReleasingTwiceSucceedsBothTimes(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	h := f.hold(t, f.lena(), at(2026, time.October, 14, 10, 0))
	for i := range 2 {
		if err := f.manager.Release(ctx, h.Booking.ID, h.Token); err != nil {
			t.Fatalf("release %d: %v", i+1, err)
		}
	}
	// Even after the ten minutes, a released hold stays released.
	f.clock.Advance(time.Hour)
	if err := f.manager.Release(ctx, h.Booking.ID, h.Token); err != nil {
		t.Fatalf("release after the hold would have run out: %v", err)
	}
	if got := f.status(t, h.Booking.ID); got != booking.StatusReleased {
		t.Fatalf("hold is %s", got)
	}
}

func TestReleaseNeedsTheHoldsOwnToken(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	h := f.hold(t, f.lena(), at(2026, time.October, 14, 10, 0))
	other := f.hold(t, f.jonas(), at(2026, time.October, 14, 10, 0))

	for name, c := range map[string]struct{ id, token string }{
		"wrong token":          {h.Booking.ID, "not-the-token"},
		"another hold's token": {h.Booking.ID, other.Token},
		"unknown id":           {"7c9e6679-7425-40de-944b-e07fc1f90ae7", h.Token},
		"malformed id":         {"not-a-uuid", h.Token},
	} {
		if err := f.manager.Release(ctx, c.id, c.token); !errors.Is(err, booking.ErrHoldNotFound) {
			t.Errorf("%s: got %v, want ErrHoldNotFound", name, err)
		}
	}
	var verr *booking.ValidationError
	if err := f.manager.Release(ctx, h.Booking.ID, ""); !errors.As(err, &verr) || verr.Field != "hold_token" {
		t.Errorf("empty token: got %v, want a hold_token ValidationError", err)
	}
	if got := f.status(t, h.Booking.ID); got != booking.StatusHeld {
		t.Fatalf("hold is %s after failed releases", got)
	}
}

func TestReleaseRefusesConfirmedAndExpiredHolds(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	h := f.hold(t, f.lena(), at(2026, time.October, 14, 10, 0))
	c, _, err := f.manager.Confirm(ctx, booking.ConfirmRequest{HoldToken: h.Token, Customer: anna}, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.manager.Release(ctx, h.Booking.ID, h.Token); !errors.Is(err, booking.ErrAlreadyConfirmed) {
		t.Fatalf("confirmed: got %v, want ErrAlreadyConfirmed", err)
	}
	if got := f.status(t, c.Booking.ID); got != booking.StatusConfirmed {
		t.Fatalf("confirmed booking is %s after a release", got)
	}
	if _, err := f.manager.Cancel(ctx, c.Booking.ID, c.ManageToken); err != nil {
		t.Fatal(err)
	}
	if err := f.manager.Release(ctx, h.Booking.ID, h.Token); !errors.Is(err, booking.ErrAlreadyConfirmed) {
		t.Fatalf("confirmed, then cancelled: got %v, want ErrAlreadyConfirmed", err)
	}

	lapsed := f.hold(t, f.jonas(), at(2026, time.October, 14, 10, 0))
	f.clock.Advance(10 * time.Minute)
	if err := f.manager.Release(ctx, lapsed.Booking.ID, lapsed.Token); !errors.Is(err, booking.ErrHoldExpired) {
		t.Fatalf("ran out, not swept yet: got %v, want ErrHoldExpired", err)
	}
	if _, err := f.manager.ExpireHolds(ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.manager.Release(ctx, lapsed.Booking.ID, lapsed.Token); !errors.Is(err, booking.ErrHoldExpired) {
		t.Fatalf("swept: got %v, want ErrHoldExpired", err)
	}
	if got := f.status(t, lapsed.Booking.ID); got != booking.StatusExpired {
		t.Fatalf("expired hold is %s after a release", got)
	}
}

func TestAReleasedHoldCannotBeConfirmed(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	h := f.hold(t, f.lena(), at(2026, time.October, 14, 10, 0))
	if err := f.manager.Release(ctx, h.Booking.ID, h.Token); err != nil {
		t.Fatal(err)
	}
	_, _, err := f.manager.Confirm(ctx, booking.ConfirmRequest{HoldToken: h.Token, Customer: anna}, "")
	if !errors.Is(err, booking.ErrHoldExpired) {
		t.Fatalf("got %v, want ErrHoldExpired", err)
	}
}

// A release and a confirm of one hold at the same moment: exactly one of
// them takes effect and the other is told why it did not.
func TestSimultaneousReleaseAndConfirmHaveOneWinner(t *testing.T) {
	const rounds = 40
	f := newRaceFixture(t, 2)
	ctx := context.Background()
	start := at(2026, time.October, 14, 10, 0)
	for round := range rounds {
		h := f.hold(t, f.lena(), start)
		errs := race(2, func(i int) error {
			if i == 0 {
				return f.manager.Release(ctx, h.Booking.ID, h.Token)
			}
			_, _, err := f.manager.Confirm(ctx, booking.ConfirmRequest{HoldToken: h.Token, Customer: anna}, "")
			return err
		})
		released, confirmed := errs[0] == nil, errs[1] == nil
		switch {
		case released && !confirmed && errors.Is(errs[1], booking.ErrHoldExpired):
			if got := f.status(t, h.Booking.ID); got != booking.StatusReleased {
				t.Fatalf("round %d: release won but the hold is %s", round, got)
			}
		case confirmed && !released && errors.Is(errs[0], booking.ErrAlreadyConfirmed):
			if got := f.status(t, h.Booking.ID); got != booking.StatusConfirmed {
				t.Fatalf("round %d: confirm won but the booking is %s", round, got)
			}
		default:
			t.Fatalf("round %d: release %v, confirm %v; want exactly one to succeed", round, errs[0], errs[1])
		}
		f.clearBookings(t)
	}
}

// Holds inserted while the same period is being released wait for the
// release and then compete as usual: at most one of them gets it, and none
// fails with anything but ErrSlotTaken.
func TestSimultaneousReleaseAndHoldsOnThePeriod(t *testing.T) {
	const rounds, n = 40, 4
	f := newRaceFixture(t, n)
	ctx := context.Background()
	start := at(2026, time.October, 14, 10, 0)
	for round := range rounds {
		h := f.hold(t, f.lena(), start)
		errs := race(n, func(i int) error {
			if i == 0 {
				return f.manager.Release(ctx, h.Booking.ID, h.Token)
			}
			_, err := f.manager.Hold(ctx, booking.HoldRequest{ServiceID: f.haircut(), StaffID: f.lena(), Start: start})
			return err
		})
		if errs[0] != nil {
			t.Fatalf("round %d: release: %v", round, errs[0])
		}
		won := 0
		for _, err := range errs[1:] {
			switch {
			case err == nil:
				won++
			case !errors.Is(err, booking.ErrSlotTaken):
				t.Fatalf("round %d: hold: %v", round, err)
			}
		}
		if won > 1 {
			t.Fatalf("round %d: %d holds on one period", round, won)
		}
		f.clearBookings(t)
	}
}
