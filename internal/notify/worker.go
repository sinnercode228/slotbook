package notify

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/sinnercode228/slotbook/internal/clock"
)

// Worker delivers due outbox rows.
//
// Claiming is a short transaction: due rows are picked with FOR UPDATE SKIP
// LOCKED, so concurrent workers never pick the same row, and their
// next_attempt_at is pushed to the end of a lease before the transaction
// commits. Sending then happens outside any transaction, so a slow Telegram
// call holds neither row locks nor a pooled connection. If a worker dies,
// its rows become due again when the lease ends, which makes delivery
// at-least-once. Each claim bumps attempts, and every write after a send is
// conditional on that attempt number, so a worker whose lease ran out cannot
// overwrite the outcome of a newer attempt.
type Worker struct {
	pool   *pgxpool.Pool
	sender Sender
	clock  clock.Clock
	log    *slog.Logger

	// Lease must comfortably exceed SendTimeout.
	Lease       time.Duration
	SendTimeout time.Duration
	MaxAttempts int
	BatchSize   int
}

func NewWorker(pool *pgxpool.Pool, sender Sender, clk clock.Clock, log *slog.Logger) *Worker {
	return &Worker{
		pool:        pool,
		sender:      sender,
		clock:       clk,
		log:         log,
		Lease:       2 * time.Minute,
		SendTimeout: 30 * time.Second,
		MaxAttempts: 5,
		BatchSize:   20,
	}
}

// Run processes due notifications every interval until ctx is cancelled.
func (w *Worker) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		for {
			n, err := w.RunOnce(ctx)
			if err != nil {
				if ctx.Err() == nil {
					w.log.Error("notification worker", "err", err)
				}
				break
			}
			if n < w.BatchSize {
				break
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// RunOnce claims one batch of due notifications, delivers it and returns
// how many rows it claimed.
func (w *Worker) RunOnce(ctx context.Context) (int, error) {
	batch, err := w.claim(ctx)
	if err != nil {
		return 0, err
	}
	for _, c := range batch {
		if err := w.deliver(ctx, c); err != nil {
			return len(batch), err
		}
	}
	return len(batch), nil
}

type claimed struct {
	id        int64
	bookingID string
	kind      Kind
	attempts  int
}

func (w *Worker) claim(ctx context.Context) ([]claimed, error) {
	now := w.clock.Now()
	rows, err := w.pool.Query(ctx, `
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
		RETURNING n.id, n.booking_id::text, n.kind, n.attempts`,
		now, now.Add(w.Lease), w.BatchSize)
	if err != nil {
		return nil, fmt.Errorf("notify: claim: %w", err)
	}
	batch, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (claimed, error) {
		var c claimed
		err := row.Scan(&c.id, &c.bookingID, &c.kind, &c.attempts)
		return c, err
	})
	if err != nil {
		return nil, fmt.Errorf("notify: claim: %w", err)
	}
	return batch, nil
}

type bookingState struct {
	status         string
	start          time.Time
	rescheduled    bool
	customerName   string
	phone          string
	email          string
	telegramChatID int64
	locationName   string
	timeZone       string
	serviceName    string
	staffName      string
}

func (w *Worker) loadBooking(ctx context.Context, id string) (bookingState, error) {
	var b bookingState
	err := w.pool.QueryRow(ctx, `
		SELECT b.status, b.starts_at, b.rescheduled_from IS NOT NULL,
		       coalesce(b.customer_name, ''), coalesce(b.customer_phone, ''), coalesce(b.customer_email, ''),
		       coalesce(b.telegram_chat_id, 0), l.name, l.time_zone, s.name, st.name
		FROM bookings b
		JOIN locations l ON l.id = b.location_id
		JOIN services s ON s.id = b.service_id
		JOIN staff st ON st.id = b.staff_id
		WHERE b.id = $1`, id).Scan(
		&b.status, &b.start, &b.rescheduled, &b.customerName, &b.phone, &b.email,
		&b.telegramChatID, &b.locationName, &b.timeZone, &b.serviceName, &b.staffName)
	return b, err
}

// staleReason explains why a notification no longer fits its booking, or
// returns "" if it should still go out.
func staleReason(kind Kind, b bookingState, now time.Time) string {
	if kind == KindCancellation {
		if b.status != "cancelled" {
			return "booking is " + b.status
		}
		return ""
	}
	if b.status != "confirmed" {
		return "booking is " + b.status
	}
	if !b.start.After(now) {
		return "appointment already started"
	}
	return ""
}

func (w *Worker) deliver(ctx context.Context, c claimed) error {
	log := w.log.With("notification_id", c.id, "booking_id", c.bookingID, "kind", c.kind, "attempt", c.attempts)
	b, err := w.loadBooking(ctx, c.bookingID)
	if err != nil {
		return fmt.Errorf("notify: load booking %s: %w", c.bookingID, err)
	}
	if reason := staleReason(c.kind, b, w.clock.Now()); reason != "" {
		log.Info("notification skipped", "reason", reason)
		return w.record(ctx, c, `status = 'skipped', last_error = $3`, reason)
	}
	zone, err := time.LoadLocation(b.timeZone)
	if err != nil {
		return fmt.Errorf("notify: booking %s: %w", c.bookingID, err)
	}
	msg := Message{
		NotificationID: c.id,
		BookingID:      c.bookingID,
		Kind:           c.kind,
		Phone:          b.phone,
		Email:          b.email,
		TelegramChatID: b.telegramChatID,
		Text: Render(Details{
			Kind:         c.kind,
			CustomerName: b.customerName,
			LocationName: b.locationName,
			ServiceName:  b.serviceName,
			StaffName:    b.staffName,
			Start:        b.start,
			Zone:         zone,
			Rescheduled:  b.rescheduled,
		}),
	}

	sendCtx, cancel := context.WithTimeout(ctx, w.SendTimeout)
	sendErr := w.sender.Send(sendCtx, msg)
	cancel()

	now := w.clock.Now()
	switch {
	case sendErr == nil:
		return w.record(ctx, c, `status = 'sent', sent_at = $3, last_error = NULL`, now)
	case c.attempts >= w.MaxAttempts:
		log.Warn("notification failed for good", "err", sendErr)
		return w.record(ctx, c, `status = 'failed', last_error = $3`, sendErr.Error())
	default:
		retryAt := now.Add(Backoff(c.attempts))
		log.Info("notification will be retried", "err", sendErr, "retry_at", retryAt)
		return w.record(ctx, c, `next_attempt_at = $3, last_error = $4`, retryAt, sendErr.Error())
	}
}

// record applies an update to the claimed row, fenced by the attempt number
// of the claim. A fenced-out update means another worker took the row over
// after this worker's lease ended; that is logged and not an error.
func (w *Worker) record(ctx context.Context, c claimed, set string, args ...any) error {
	tag, err := w.pool.Exec(ctx,
		`UPDATE notifications SET `+set+` WHERE id = $1 AND attempts = $2 AND status = 'pending'`,
		append([]any{c.id, c.attempts}, args...)...)
	if err != nil {
		return fmt.Errorf("notify: record outcome of %d: %w", c.id, err)
	}
	if tag.RowsAffected() == 0 {
		w.log.Warn("notification outcome discarded: lease lost to a newer attempt",
			"notification_id", c.id, "attempt", c.attempts)
	}
	return nil
}
