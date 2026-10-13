package booking_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sinnercode228/slotbook/internal/booking"
	"github.com/sinnercode228/slotbook/internal/slots"
)

func TestBlocksShareTheOverlapGuarantee(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	f.book(t, f.lena(), at(2026, time.October, 14, 10, 0))

	_, err := f.manager.CreateBlock(ctx, booking.BlockRequest{
		StaffID: f.lena(), Start: at(2026, time.October, 14, 10, 30), End: at(2026, time.October, 14, 12, 0), Note: "training",
	})
	if !errors.Is(err, booking.ErrSlotTaken) {
		t.Fatalf("block over a booking: got %v, want ErrSlotTaken", err)
	}
	block, err := f.manager.CreateBlock(ctx, booking.BlockRequest{
		StaffID: f.lena(), Start: at(2026, time.October, 14, 13, 0), End: at(2026, time.October, 14, 14, 0), Note: "dentist",
	})
	if err != nil {
		t.Fatal(err)
	}
	if block.Kind != booking.KindBlock || block.Note != "dentist" {
		t.Fatalf("block %+v", block)
	}
	_, err = f.manager.Hold(ctx, booking.HoldRequest{ServiceID: f.haircut(), StaffID: f.lena(), Start: at(2026, time.October, 14, 12, 30)})
	if !errors.Is(err, booking.ErrSlotTaken) {
		t.Fatalf("hold running into a block: got %v, want ErrSlotTaken", err)
	}

	if err := f.manager.DeleteBlock(ctx, block.ID); err != nil {
		t.Fatal(err)
	}
	f.hold(t, f.lena(), at(2026, time.October, 14, 12, 30))
	if err := f.manager.DeleteBlock(ctx, block.ID); !errors.Is(err, booking.ErrNotFound) {
		t.Fatalf("deleting twice: %v", err)
	}
}

func TestCreateBlockValidatesInput(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	start := at(2026, time.October, 14, 13, 0)
	for name, req := range map[string]booking.BlockRequest{
		"end before start": {StaffID: f.lena(), Start: start, End: start.Add(-time.Hour)},
		"empty":            {StaffID: f.lena(), Start: start, End: start},
		"too long":         {StaffID: f.lena(), Start: start, End: start.Add(32 * 24 * time.Hour)},
	} {
		var verr *booking.ValidationError
		if _, err := f.manager.CreateBlock(ctx, req); !errors.As(err, &verr) {
			t.Errorf("%s: got %v, want a ValidationError", name, err)
		}
	}
	if _, err := f.manager.CreateBlock(ctx, booking.BlockRequest{StaffID: 999999, Start: start, End: start.Add(time.Hour)}); !errors.Is(err, booking.ErrInvalidStaff) {
		t.Errorf("unknown staff: %v", err)
	}
}

func TestListForDayUsesTheLocalCalendarDay(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	wed := f.book(t, f.lena(), at(2026, time.October, 14, 9, 0))
	f.book(t, f.lena(), at(2026, time.October, 15, 9, 0))
	held := f.hold(t, f.jonas(), at(2026, time.October, 14, 17, 0))
	block, err := f.manager.CreateBlock(ctx, booking.BlockRequest{
		StaffID: f.jonas(), Start: at(2026, time.October, 14, 23, 0), End: at(2026, time.October, 15, 1, 0),
	})
	if err != nil {
		t.Fatal(err)
	}

	list, err := f.manager.ListForDay(ctx, f.location(), slots.Date{Year: 2026, Month: time.October, Day: 14})
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, b := range list {
		ids = append(ids, b.ID)
	}
	want := []string{wed.Booking.ID, held.Booking.ID, block.ID}
	if len(ids) != len(want) {
		t.Fatalf("listed %v, want %v", ids, want)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("listed %v, want %v (ordered by start)", ids, want)
		}
	}
	if _, err := f.manager.ListForDay(ctx, 999999, slots.Date{Year: 2026, Month: time.October, Day: 14}); !errors.Is(err, booking.ErrNotFound) {
		t.Fatalf("unknown location: %v", err)
	}
}

func TestAdminCancelIgnoresTheCutoff(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	start := at(2026, time.October, 14, 10, 0)
	c := f.book(t, f.lena(), start)
	f.clock.Set(start.Add(-time.Hour))

	b, err := f.manager.AdminCancel(ctx, c.Booking.ID)
	if err != nil || b.Status != booking.StatusCancelled {
		t.Fatalf("admin cancel: %+v, %v", b, err)
	}
	if got := f.outbox(t, c.Booking.ID)["cancellation"].Status; got != "pending" {
		t.Fatalf("cancellation notice is %q", got)
	}
	if _, err := f.manager.AdminCancel(ctx, c.Booking.ID); !errors.Is(err, booking.ErrNotActive) {
		t.Fatalf("second admin cancel: %v", err)
	}
}

func TestAvailabilityHonoursBusyTimeAndAnyStaff(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	wed := slots.Date{Year: 2026, Month: time.October, Day: 14}
	ten := at(2026, time.October, 14, 10, 0)
	f.book(t, f.lena(), ten)

	contains := func(a booking.Availability, start time.Time) bool {
		for _, d := range a.Days {
			for _, s := range d.Slots {
				if s.Equal(start) {
					return true
				}
			}
		}
		return false
	}
	lena, err := f.manager.Availability(ctx, booking.AvailabilityQuery{ServiceID: f.haircut(), StaffID: f.lena(), From: wed, To: wed})
	if err != nil {
		t.Fatal(err)
	}
	if contains(lena, ten) || contains(lena, ten.Add(45*time.Minute)) || !contains(lena, ten.Add(time.Hour)) {
		t.Fatal("Lena's availability ignores her booking and its buffer")
	}
	anyone, err := f.manager.Availability(ctx, booking.AvailabilityQuery{ServiceID: f.haircut(), From: wed, To: wed})
	if err != nil {
		t.Fatal(err)
	}
	if !contains(anyone, ten) {
		t.Fatal("Jonas is free at 10:00, so any-staff availability should include it")
	}
	if len(anyone.Days) != 1 || anyone.Location.ID != f.location() {
		t.Fatalf("unexpected availability %+v", anyone)
	}

	var verr *booking.ValidationError
	_, err = f.manager.Availability(ctx, booking.AvailabilityQuery{ServiceID: f.haircut(), From: wed, To: wed.AddDays(booking.MaxAvailabilityDays)})
	if !errors.As(err, &verr) {
		t.Fatalf("over-long range: %v", err)
	}
}
