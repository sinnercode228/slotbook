package booking_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sinnercode228/slotbook/internal/booking"
)

func TestGetNeedsTheRightToken(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	c := f.book(t, f.lena(), at(2026, time.October, 14, 10, 0))
	for name, args := range map[string][2]string{
		"wrong token":  {c.Booking.ID, c.ManageToken + "x"},
		"empty token":  {c.Booking.ID, ""},
		"malformed id": {"not-a-uuid", c.ManageToken},
		"unknown id":   {"7c9e6679-7425-40de-944b-e07fc1f90ae7", c.ManageToken},
	} {
		if _, err := f.manager.Get(ctx, args[0], args[1]); !errors.Is(err, booking.ErrNotFound) {
			t.Errorf("%s: got %v, want ErrNotFound", name, err)
		}
	}
}

func TestCancelRespectsTheCutoff(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	start := at(2026, time.October, 14, 10, 0)
	early := f.book(t, f.lena(), start)
	late := f.book(t, f.jonas(), start)

	f.clock.Set(start.Add(-3 * time.Hour))
	b, err := f.manager.Cancel(ctx, early.Booking.ID, early.ManageToken)
	if err != nil || b.Status != booking.StatusCancelled {
		t.Fatalf("cancel exactly at the cutoff: %+v, %v", b, err)
	}
	f.clock.Advance(time.Minute)
	if _, err := f.manager.Cancel(ctx, late.Booking.ID, late.ManageToken); !errors.Is(err, booking.ErrTooLate) {
		t.Fatalf("cancel after the cutoff: got %v, want ErrTooLate", err)
	}
	if _, err := f.manager.Cancel(ctx, early.Booking.ID, early.ManageToken); !errors.Is(err, booking.ErrNotActive) {
		t.Fatalf("cancel twice: got %v, want ErrNotActive", err)
	}
}

func TestCancelVoidsRemindersAndQueuesANotice(t *testing.T) {
	f := newFixture(t)
	c := f.book(t, f.lena(), at(2026, time.October, 14, 10, 0))
	if _, err := f.manager.Cancel(context.Background(), c.Booking.ID, c.ManageToken); err != nil {
		t.Fatal(err)
	}
	got := f.outbox(t, c.Booking.ID)
	for kind, status := range map[string]string{
		"confirmation": "voided",
		"reminder_24h": "voided",
		"reminder_2h":  "voided",
		"cancellation": "pending",
	} {
		if got[kind].Status != status {
			t.Errorf("%s is %q, want %q", kind, got[kind].Status, status)
		}
	}
}

func TestRescheduleOntoAPeriodOverlappingItsOwn(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	start := at(2026, time.October, 14, 10, 0)
	c := f.book(t, f.lena(), start)

	// 10:15-11:15 overlaps the old 10:00-11:00; it works only because the old
	// booking is cancelled before the new one is inserted.
	moved, err := f.manager.Reschedule(ctx, c.Booking.ID, c.ManageToken, start.Add(15*time.Minute))
	if err != nil {
		t.Fatalf("reschedule by 15 minutes: %v", err)
	}
	if moved.ID == c.Booking.ID || moved.Status != booking.StatusConfirmed || !moved.Start.Equal(start.Add(15*time.Minute)) {
		t.Fatalf("new booking %+v", moved)
	}
	if moved.RescheduledFrom != c.Booking.ID || moved.StaffID != f.lena() || moved.Customer == nil || moved.Customer.Phone != anna.Phone {
		t.Fatalf("new booking does not carry over the old one: %+v", moved)
	}
	if got := f.status(t, c.Booking.ID); got != booking.StatusCancelled {
		t.Fatalf("old booking is %s", got)
	}
	if _, err := f.manager.Get(ctx, moved.ID, c.ManageToken); err != nil {
		t.Fatalf("the manage token should work for the new booking: %v", err)
	}

	old := f.outbox(t, c.Booking.ID)
	for kind, row := range old {
		if row.Status != "voided" {
			t.Errorf("old %s is %s, want voided", kind, row.Status)
		}
	}
	if _, ok := old["cancellation"]; ok {
		t.Error("a reschedule should not send a cancellation notice")
	}
	if fresh := f.outbox(t, moved.ID); len(fresh) != 3 || fresh["confirmation"].Status != "pending" {
		t.Fatalf("new booking outbox %+v", fresh)
	}
}

func TestRescheduleIntoSomeoneElsesBookingChangesNothing(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	mine := f.book(t, f.lena(), at(2026, time.October, 14, 10, 0))
	f.book(t, f.lena(), at(2026, time.October, 14, 12, 0))

	_, err := f.manager.Reschedule(ctx, mine.Booking.ID, mine.ManageToken, at(2026, time.October, 14, 11, 30))
	if !errors.Is(err, booking.ErrSlotTaken) {
		t.Fatalf("got %v, want ErrSlotTaken", err)
	}
	if got := f.status(t, mine.Booking.ID); got != booking.StatusConfirmed {
		t.Fatalf("original booking is %s after a failed reschedule", got)
	}
	if got := f.outbox(t, mine.Booking.ID)["reminder_2h"].Status; got != "pending" {
		t.Fatalf("reminder is %s after a failed reschedule", got)
	}
}

func TestRescheduleChecksTheNewStartAndTheCutoff(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	start := at(2026, time.October, 14, 10, 0)
	c := f.book(t, f.lena(), start)

	var slotErr *booking.SlotError
	if _, err := f.manager.Reschedule(ctx, c.Booking.ID, c.ManageToken, start.Add(7*time.Minute)); !errors.As(err, &slotErr) {
		t.Fatalf("misaligned start: %v", err)
	}
	f.clock.Set(start.Add(-2 * time.Hour))
	if _, err := f.manager.Reschedule(ctx, c.Booking.ID, c.ManageToken, start.Add(24*time.Hour)); !errors.Is(err, booking.ErrTooLate) {
		t.Fatalf("inside the cutoff: %v", err)
	}
}
