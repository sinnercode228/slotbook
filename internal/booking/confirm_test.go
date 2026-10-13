package booking_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sinnercode228/slotbook/internal/booking"
)

func TestConfirmTurnsAHoldIntoABooking(t *testing.T) {
	f := newFixture(t)
	start := at(2026, time.October, 14, 10, 0)
	c := f.book(t, f.lena(), start)

	b := c.Booking
	if b.Status != booking.StatusConfirmed || !b.Start.Equal(start) || !b.End.Equal(start.Add(45*time.Minute)) {
		t.Fatalf("booking %+v", b)
	}
	if b.PriceMinor != 3800 || b.Currency != "EUR" || b.Customer == nil || b.Customer.Name != "Anna" {
		t.Fatalf("booking %+v", b)
	}
	if c.ManageToken == "" {
		t.Fatal("no manage token returned")
	}
	got, err := f.manager.Get(context.Background(), b.ID, c.ManageToken)
	if err != nil || got.ID != b.ID {
		t.Fatalf("Get with the manage token: %+v, %v", got, err)
	}
}

func TestConfirmAfterTheHoldExpired(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	h := f.hold(t, f.lena(), at(2026, time.October, 14, 10, 0))
	f.clock.Advance(10 * time.Minute)

	_, _, err := f.manager.Confirm(ctx, booking.ConfirmRequest{HoldToken: h.Token, Customer: anna}, "")
	if !errors.Is(err, booking.ErrHoldExpired) {
		t.Fatalf("got %v, want ErrHoldExpired", err)
	}
	// Once another hold has replaced it, the old one is expired for good.
	f.hold(t, f.lena(), at(2026, time.October, 14, 10, 0))
	_, _, err = f.manager.Confirm(ctx, booking.ConfirmRequest{HoldToken: h.Token, Customer: anna}, "")
	if !errors.Is(err, booking.ErrHoldExpired) {
		t.Fatalf("after replacement: got %v, want ErrHoldExpired", err)
	}
}

func TestConfirmRejectsUnknownAndUsedHolds(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	_, _, err := f.manager.Confirm(ctx, booking.ConfirmRequest{HoldToken: "no-such-token", Customer: anna}, "")
	if !errors.Is(err, booking.ErrHoldNotFound) {
		t.Fatalf("unknown token: %v", err)
	}
	h := f.hold(t, f.lena(), at(2026, time.October, 14, 10, 0))
	req := booking.ConfirmRequest{HoldToken: h.Token, Customer: anna}
	if _, _, err := f.manager.Confirm(ctx, req, ""); err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.manager.Confirm(ctx, req, ""); !errors.Is(err, booking.ErrAlreadyConfirmed) {
		t.Fatalf("second confirm without a key: %v", err)
	}
	bad := booking.ConfirmRequest{HoldToken: h.Token, Customer: booking.Customer{Name: "Anna"}}
	var verr *booking.ValidationError
	if _, _, err := f.manager.Confirm(ctx, bad, ""); !errors.As(err, &verr) {
		t.Fatalf("missing phone: %v", err)
	}
}

func TestConfirmIsIdempotentPerKey(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	h := f.hold(t, f.lena(), at(2026, time.October, 14, 10, 0))
	req := booking.ConfirmRequest{HoldToken: h.Token, Customer: anna}
	const key = "0b7d6c1e-4f5a-4c1b-9d2e-3f4a5b6c7d8e"

	first, replayed, err := f.manager.Confirm(ctx, req, key)
	if err != nil || replayed {
		t.Fatalf("first confirm: replayed=%v err=%v", replayed, err)
	}
	f.clock.Advance(time.Hour)
	second, replayed, err := f.manager.Confirm(ctx, req, key)
	if err != nil || !replayed {
		t.Fatalf("retry: replayed=%v err=%v", replayed, err)
	}
	if second.Booking.ID != first.Booking.ID || second.ManageToken != first.ManageToken {
		t.Fatalf("retry returned a different result:\n%+v\n%+v", first, second)
	}

	other := req
	other.Customer.Name = "Someone else"
	if _, _, err := f.manager.Confirm(ctx, other, key); !errors.Is(err, booking.ErrIdempotencyMismatch) {
		t.Fatalf("same key, different body: %v", err)
	}
	if n := len(f.outbox(t, first.Booking.ID)); n != 3 {
		t.Fatalf("%d notifications after a replayed confirm, want 3", n)
	}
}

func TestConfirmWritesTheOutbox(t *testing.T) {
	f := newFixture(t)
	start := at(2026, time.October, 14, 10, 0)
	c := f.book(t, f.lena(), start)
	got := f.outbox(t, c.Booking.ID)
	want := map[string]time.Time{
		"confirmation": mondayMorning,
		"reminder_24h": start.Add(-24 * time.Hour),
		"reminder_2h":  start.Add(-2 * time.Hour),
	}
	if len(got) != len(want) {
		t.Fatalf("outbox %+v", got)
	}
	for kind, sendAt := range want {
		if got[kind].Status != "pending" || !got[kind].SendAt.Equal(sendAt) {
			t.Errorf("%s: %+v, want pending at %s", kind, got[kind], sendAt)
		}
	}

	// Booked this afternoon: the 24h reminder would already be in the past.
	soon := f.book(t, f.lena(), at(2026, time.October, 12, 15, 0))
	got = f.outbox(t, soon.Booking.ID)
	if _, ok := got["reminder_24h"]; ok || len(got) != 2 {
		t.Fatalf("same-day booking outbox %+v", got)
	}
}

func TestIdempotencyKeysAreForgottenAfterADay(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	h := f.hold(t, f.lena(), at(2026, time.October, 14, 10, 0))
	req := booking.ConfirmRequest{HoldToken: h.Token, Customer: anna}
	if _, _, err := f.manager.Confirm(ctx, req, "key-1"); err != nil {
		t.Fatal(err)
	}

	f.clock.Advance(booking.IdempotencyKeyTTL - time.Minute)
	if n, err := f.manager.PruneIdempotencyKeys(ctx); err != nil || n != 0 {
		t.Fatalf("pruned %d keys before the TTL, err %v", n, err)
	}
	f.clock.Advance(2 * time.Minute)
	if n, err := f.manager.PruneIdempotencyKeys(ctx); err != nil || n != 1 {
		t.Fatalf("pruned %d keys after the TTL, err %v", n, err)
	}
	// Without the stored result the retry runs again and finds the hold used.
	if _, _, err := f.manager.Confirm(ctx, req, "key-1"); !errors.Is(err, booking.ErrAlreadyConfirmed) {
		t.Fatalf("retry after pruning: %v", err)
	}
}
