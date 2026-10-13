package notify

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/sinnercode228/slotbook/internal/clock"
	"github.com/sinnercode228/slotbook/internal/dbtest"
	"github.com/sinnercode228/slotbook/internal/seed"
	"github.com/sinnercode228/slotbook/internal/slots"
)

var testNow = time.Date(2026, time.October, 12, 8, 0, 0, 0, time.UTC)

type outboxFixture struct {
	pool     *pgxpool.Pool
	clock    *clock.Fake
	bookings []string
}

// newOutboxFixture creates n confirmed bookings, one hour apart starting
// three days from now, each with a confirmation that is already due.
func newOutboxFixture(t *testing.T, n int) outboxFixture {
	t.Helper()
	pool := dbtest.New(t, dbtest.WithMaxConns(12))
	ctx := context.Background()
	var weekly slots.WeeklyHours
	res, err := seed.Apply(ctx, pool, seed.Chain{Locations: []seed.Location{{
		Slug: "berlin", Name: "Snipwell Berlin", TimeZone: "Europe/Berlin",
		Services: []seed.Service{{Key: "cut", Name: "Haircut", DurationMinutes: 45, PriceMinor: 3800, Currency: "EUR"}},
		Staff:    []seed.Staff{{Key: "lena", Name: "Lena H.", Services: []string{"cut"}, WeeklyHours: weekly}},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := pool.Query(ctx, `
		INSERT INTO bookings (kind, status, location_id, staff_id, service_id, starts_at, ends_at, busy_until, period,
		                      customer_name, customer_phone, manage_token_hash, created_at, confirmed_at)
		SELECT 'appointment', 'confirmed', $1, $2, $3, s.start, s.start + interval '45 minutes',
		       s.start + interval '45 minutes', tstzrange(s.start, s.start + interval '45 minutes', '[)'),
		       'Anna', '+7 900 123-45-67', '\x00'::bytea, $5, $5
		FROM (SELECT $4::timestamptz + make_interval(hours => g) AS start FROM generate_series(0, $6 - 1) AS g) AS s
		ORDER BY s.start
		RETURNING id::text`,
		res.Locations["berlin"], res.Staff["lena"], res.Services["cut"], testNow.Add(72*time.Hour), testNow, n)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	for _, id := range ids {
		if err := Enqueue(ctx, pool, id, testNow.Add(-time.Minute), Planned{KindConfirmation, testNow.Add(-time.Minute)}); err != nil {
			t.Fatal(err)
		}
	}
	return outboxFixture{pool: pool, clock: clock.NewFake(testNow), bookings: ids}
}

func (f outboxFixture) worker(s Sender) *Worker {
	w := NewWorker(f.pool, s, f.clock, slog.New(slog.NewTextHandler(io.Discard, nil)))
	w.BatchSize = 5
	return w
}

func (f outboxFixture) statusOf(t *testing.T, bookingID string, kind Kind) (Status, int, string) {
	t.Helper()
	var (
		status   Status
		attempts int
		lastErr  string
	)
	err := f.pool.QueryRow(context.Background(), `
		SELECT status, attempts, coalesce(last_error, '') FROM notifications WHERE booking_id = $1 AND kind = $2`,
		bookingID, kind).Scan(&status, &attempts, &lastErr)
	if err != nil {
		t.Fatal(err)
	}
	return status, attempts, lastErr
}

type countingSender struct {
	mu    sync.Mutex
	sends map[int64]int
}

func (c *countingSender) Send(_ context.Context, m Message) error {
	time.Sleep(time.Millisecond)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sends[m.NotificationID]++
	return nil
}

type failingSender struct{}

func (failingSender) Send(context.Context, Message) error {
	return errors.New("smtp relay unavailable")
}

func TestConcurrentWorkersSendEachNotificationOnce(t *testing.T) {
	f := newOutboxFixture(t, 100)
	sender := &countingSender{sends: map[int64]int{}}
	ctx := context.Background()

	var wg sync.WaitGroup
	errs := make(chan error, 3)
	for range 3 {
		w := f.worker(sender)
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				n, err := w.RunOnce(ctx)
				if err != nil {
					errs <- err
					return
				}
				if n == 0 {
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}

	if len(sender.sends) != 100 {
		t.Fatalf("%d distinct notifications sent, want 100", len(sender.sends))
	}
	for id, n := range sender.sends {
		if n != 1 {
			t.Errorf("notification %d sent %d times", id, n)
		}
	}
	var sent, attempts int
	err := f.pool.QueryRow(ctx, `SELECT count(*) FILTER (WHERE status = 'sent'), coalesce(max(attempts), 0) FROM notifications`).Scan(&sent, &attempts)
	if err != nil {
		t.Fatal(err)
	}
	if sent != 100 || attempts != 1 {
		t.Fatalf("%d rows marked sent (want 100), max attempts %d (want 1)", sent, attempts)
	}
}

func TestWorkerRechecksTheBookingBeforeSending(t *testing.T) {
	f := newOutboxFixture(t, 2)
	ctx := context.Background()
	cancelled, started := f.bookings[0], f.bookings[1]
	if _, err := f.pool.Exec(ctx, `UPDATE bookings SET status = 'cancelled', cancelled_at = $2 WHERE id = $1`, cancelled, testNow); err != nil {
		t.Fatal(err)
	}
	if err := Enqueue(ctx, f.pool, started, testNow, Planned{KindReminder2h, testNow.Add(70 * time.Hour)}); err != nil {
		t.Fatal(err)
	}

	sender := &countingSender{sends: map[int64]int{}}
	w := f.worker(sender)
	if _, err := w.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	status, _, reason := f.statusOf(t, cancelled, KindConfirmation)
	if status != StatusSkipped || !strings.Contains(reason, "cancelled") {
		t.Fatalf("confirmation of a cancelled booking: %s (%s)", status, reason)
	}

	// The worker was down past the appointment: the reminder is pointless now.
	f.clock.Set(testNow.Add(80 * time.Hour))
	if _, err := w.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	status, _, reason = f.statusOf(t, started, KindReminder2h)
	if status != StatusSkipped || !strings.Contains(reason, "started") {
		t.Fatalf("late reminder: %s (%s)", status, reason)
	}
	if status, _, _ := f.statusOf(t, started, KindConfirmation); status != StatusSent || len(sender.sends) != 1 {
		t.Fatalf("only the live booking's confirmation should go out: %s, %v", status, sender.sends)
	}
}

func TestWorkerRetriesWithBackoffAndThenGivesUp(t *testing.T) {
	f := newOutboxFixture(t, 1)
	ctx := context.Background()
	w := f.worker(failingSender{})
	w.MaxAttempts = 3
	id := f.bookings[0]

	for attempt := 1; attempt <= 3; attempt++ {
		n, err := w.RunOnce(ctx)
		if err != nil || n != 1 {
			t.Fatalf("attempt %d: claimed %d, %v", attempt, n, err)
		}
		status, attempts, lastErr := f.statusOf(t, id, KindConfirmation)
		if attempts != attempt || !strings.Contains(lastErr, "smtp relay unavailable") {
			t.Fatalf("attempt %d: attempts %d, last error %q", attempt, attempts, lastErr)
		}
		if attempt < 3 {
			if status != StatusPending {
				t.Fatalf("attempt %d: status %s, want pending", attempt, status)
			}
			if n, _ := w.RunOnce(ctx); n != 0 {
				t.Fatalf("attempt %d: retried before the backoff elapsed", attempt)
			}
			f.clock.Advance(Backoff(attempt))
		} else if status != StatusFailed {
			t.Fatalf("after the last attempt: status %s, want failed", status)
		}
	}
}

func TestExpiredLeaseCannotOverwriteANewerAttempt(t *testing.T) {
	f := newOutboxFixture(t, 1)
	ctx := context.Background()
	slow := f.worker(failingSender{})
	claimed, err := slow.claim(ctx)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim: %v, %v", claimed, err)
	}

	f.clock.Advance(slow.Lease + time.Second)
	fresh := f.worker(&countingSender{sends: map[int64]int{}})
	if n, err := fresh.RunOnce(ctx); err != nil || n != 1 {
		t.Fatalf("second worker claimed %d, %v", n, err)
	}

	// The first worker finally finishes its (failed) attempt.
	if err := slow.deliver(ctx, claimed[0]); err != nil {
		t.Fatal(err)
	}
	status, attempts, _ := f.statusOf(t, f.bookings[0], KindConfirmation)
	if status != StatusSent || attempts != 2 {
		t.Fatalf("status %s after %d attempts, want sent after 2", status, attempts)
	}
}

func TestUnrenderableNotificationsUseUpTheirAttempts(t *testing.T) {
	f := newOutboxFixture(t, 1)
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `UPDATE locations SET time_zone = 'Mars/Olympus_Mons'`); err != nil {
		t.Fatal(err)
	}
	w := f.worker(&countingSender{sends: map[int64]int{}})
	w.MaxAttempts = 1
	if _, err := w.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	status, _, lastErr := f.statusOf(t, f.bookings[0], KindConfirmation)
	if status != StatusFailed || !strings.Contains(lastErr, "Mars/Olympus_Mons") {
		t.Fatalf("status %s, last error %q", status, lastErr)
	}
}
