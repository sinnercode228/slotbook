package booking

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/sinnercode228/slotbook/internal/db"
	"github.com/sinnercode228/slotbook/internal/slots"
)

const bookingColumns = `id::text, kind, status, location_id, staff_id, coalesce(service_id, 0), starts_at, ends_at,
	busy_until, coalesce(price_minor, 0), coalesce(currency, ''), hold_expires_at, customer_name,
	coalesce(customer_phone, ''), coalesce(customer_email, ''), coalesce(telegram_chat_id, 0), note,
	coalesce(rescheduled_from::text, ''), created_at, confirmed_at, cancelled_at, manage_token_hash`

func scanBooking(row pgx.Row) (Booking, error) {
	var (
		b            Booking
		name         *string
		phone, email string
		chatID       int64
	)
	err := row.Scan(&b.ID, &b.Kind, &b.Status, &b.LocationID, &b.StaffID, &b.ServiceID, &b.Start, &b.End,
		&b.BusyUntil, &b.PriceMinor, &b.Currency, &b.HoldExpiresAt, &name, &phone, &email, &chatID, &b.Note,
		&b.RescheduledFrom, &b.CreatedAt, &b.ConfirmedAt, &b.CancelledAt, &b.manageTokenHash)
	if err != nil {
		return Booking{}, err
	}
	if name != nil {
		b.Customer = &Customer{Name: *name, Phone: phone, Email: email, TelegramChatID: chatID}
	}
	return b, nil
}

// lockAppointment loads an appointment and locks its row until the
// transaction ends.
func lockAppointment(ctx context.Context, tx pgx.Tx, id string) (Booking, error) {
	b, err := scanBooking(tx.QueryRow(ctx,
		`SELECT `+bookingColumns+` FROM bookings WHERE id = $1 AND kind = 'appointment' FOR UPDATE`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Booking{}, ErrNotFound
	}
	return b, err
}

func markCancelled(ctx context.Context, tx pgx.Tx, id string, now time.Time) (Booking, error) {
	return scanBooking(tx.QueryRow(ctx,
		`UPDATE bookings SET status = 'cancelled', cancelled_at = $2 WHERE id = $1 RETURNING `+bookingColumns, id, now))
}

// expireStaleHolds marks the held rows of a staff member that overlap
// [start, end) and have run out as expired. Doing this in the transaction
// that inserts the new row keeps a lapsed hold from blocking it even when
// the sweeper has not run yet.
func expireStaleHolds(ctx context.Context, q db.Querier, staffID int64, start, end, now time.Time) error {
	_, err := q.Exec(ctx, `
		UPDATE bookings SET status = 'expired'
		WHERE staff_id = $1 AND status = 'held' AND hold_expires_at <= $4
		  AND period && tstzrange($2, $3, '[)')`, staffID, start, end, now)
	return err
}

// busyIntervals returns the periods in [from, to) that a staff member cannot
// take: confirmed bookings, blocks and holds that are still running.
func busyIntervals(ctx context.Context, q db.Querier, staffID int64, from, to, now time.Time) ([]slots.Busy, error) {
	rows, err := q.Query(ctx, `
		SELECT starts_at, busy_until FROM bookings
		WHERE staff_id = $1 AND period && tstzrange($2, $3, '[)')
		  AND (status = 'confirmed' OR (status = 'held' AND hold_expires_at > $4))`,
		staffID, from, to, now)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (slots.Busy, error) {
		var b slots.Busy
		err := row.Scan(&b.Start, &b.End)
		return b, err
	})
}

func validUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		default:
			if !('0' <= c && c <= '9' || 'a' <= c && c <= 'f' || 'A' <= c && c <= 'F') {
				return false
			}
		}
	}
	return true
}
