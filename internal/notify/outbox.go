// Package notify implements the notification outbox: rows written in the
// same transaction as the booking change they describe, and a worker that
// delivers them.
package notify

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/sinnercode228/slotbook/internal/db"
)

type Kind string

const (
	KindConfirmation Kind = "confirmation"
	KindReminder24h  Kind = "reminder_24h"
	KindReminder2h   Kind = "reminder_2h"
	KindCancellation Kind = "cancellation"
)

type Status string

const (
	StatusPending Status = "pending"
	StatusSent    Status = "sent"
	StatusFailed  Status = "failed"
	// StatusVoided marks rows cancelled by a later booking change.
	StatusVoided Status = "voided"
	// StatusSkipped marks rows the worker dropped because the booking no
	// longer matched them when they came due.
	StatusSkipped Status = "skipped"
)

// Planned is a notification to be written to the outbox.
type Planned struct {
	Kind   Kind
	SendAt time.Time
}

var reminders = []struct {
	kind   Kind
	before time.Duration
}{
	{KindReminder24h, 24 * time.Hour},
	{KindReminder2h, 2 * time.Hour},
}

// ForConfirmed plans the messages for a booking confirmed at now: the
// confirmation itself and each reminder whose send time is still ahead.
func ForConfirmed(start, now time.Time) []Planned {
	planned := []Planned{{KindConfirmation, now}}
	for _, r := range reminders {
		if at := start.Add(-r.before); at.After(now) {
			planned = append(planned, Planned{r.kind, at})
		}
	}
	return planned
}

// Enqueue writes planned notifications for a booking. Call it inside the
// transaction that changes the booking.
func Enqueue(ctx context.Context, q db.Querier, bookingID string, now time.Time, planned ...Planned) error {
	for _, p := range planned {
		_, err := q.Exec(ctx, `
			INSERT INTO notifications (booking_id, kind, send_at, next_attempt_at, created_at)
			VALUES ($1, $2, $3, $3, $4)`, bookingID, p.Kind, p.SendAt, now)
		if err != nil {
			return fmt.Errorf("notify: enqueue %s: %w", p.Kind, err)
		}
	}
	return nil
}

// VoidPending cancels every notification of a booking that has not been
// delivered yet.
func VoidPending(ctx context.Context, q db.Querier, bookingID string) error {
	_, err := q.Exec(ctx, `UPDATE notifications SET status = 'voided' WHERE booking_id = $1 AND status = 'pending'`, bookingID)
	if err != nil {
		return fmt.Errorf("notify: void pending: %w", err)
	}
	return nil
}

// Notification is an outbox row as shown to admins. Times are in UTC.
type Notification struct {
	ID            int64      `json:"id"`
	BookingID     string     `json:"booking_id"`
	Kind          Kind       `json:"kind"`
	Status        Status     `json:"status"`
	SendAt        time.Time  `json:"send_at"`
	NextAttemptAt time.Time  `json:"next_attempt_at"`
	Attempts      int        `json:"attempts"`
	LastError     string     `json:"last_error,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	SentAt        *time.Time `json:"sent_at,omitempty"`
}

// Filter narrows List; zero values mean no restriction.
type Filter struct {
	LocationID int64
	BookingID  string
	Status     Status
	Limit      int
}

// List returns outbox rows, newest send time first.
func List(ctx context.Context, q db.Querier, f Filter) ([]Notification, error) {
	if f.Limit <= 0 || f.Limit > 500 {
		f.Limit = 100
	}
	var bookingID *string
	if f.BookingID != "" {
		bookingID = &f.BookingID
	}
	rows, err := q.Query(ctx, `
		SELECT n.id, n.booking_id::text, n.kind, n.status, n.send_at, n.next_attempt_at, n.attempts,
		       coalesce(n.last_error, ''), n.created_at, n.sent_at
		FROM notifications n
		JOIN bookings b ON b.id = n.booking_id
		WHERE ($1::bigint = 0 OR b.location_id = $1)
		  AND ($2::uuid IS NULL OR n.booking_id = $2)
		  AND ($3::text = '' OR n.status = $3)
		ORDER BY n.send_at DESC, n.id DESC
		LIMIT $4`, f.LocationID, bookingID, string(f.Status), f.Limit)
	if err != nil {
		return nil, fmt.Errorf("notify: list: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Notification, error) {
		var n Notification
		err := row.Scan(&n.ID, &n.BookingID, &n.Kind, &n.Status, &n.SendAt, &n.NextAttemptAt, &n.Attempts,
			&n.LastError, &n.CreatedAt, &n.SentAt)
		if err != nil {
			return Notification{}, err
		}
		n.SendAt, n.NextAttemptAt, n.CreatedAt = n.SendAt.UTC(), n.NextAttemptAt.UTC(), n.CreatedAt.UTC()
		if n.SentAt != nil {
			sent := n.SentAt.UTC()
			n.SentAt = &sent
		}
		return n, nil
	})
}

// Backoff is the wait before retry number attempt+1: 30s, doubling, capped
// at one hour.
func Backoff(attempt int) time.Duration {
	const base, ceiling = 30 * time.Second, time.Hour
	if attempt < 1 {
		attempt = 1
	}
	d := base
	for i := 1; i < attempt; i++ {
		d *= 2
		if d >= ceiling {
			return ceiling
		}
	}
	return d
}
