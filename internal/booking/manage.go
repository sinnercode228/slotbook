package booking

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/sinnercode228/slotbook/internal/catalog"
	"github.com/sinnercode228/slotbook/internal/db"
	"github.com/sinnercode228/slotbook/internal/notify"
	"github.com/sinnercode228/slotbook/internal/token"
)

// Get returns an appointment to the holder of its manage token. A wrong
// token looks exactly like a missing booking.
func (m *Manager) Get(ctx context.Context, id, manageToken string) (Booking, error) {
	if !ValidID(id) || manageToken == "" {
		return Booking{}, ErrNotFound
	}
	b, err := scanBooking(m.pool.QueryRow(ctx,
		`SELECT `+bookingColumns+` FROM bookings WHERE id = $1 AND kind = 'appointment'`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Booking{}, ErrNotFound
	}
	if err != nil {
		return Booking{}, err
	}
	if !token.Matches(manageToken, b.manageTokenHash) {
		return Booking{}, ErrNotFound
	}
	return b, nil
}

// Cancel cancels a confirmed appointment for its customer, up to the
// location's cancellation cutoff before the start.
func (m *Manager) Cancel(ctx context.Context, id, manageToken string) (Booking, error) {
	return m.cancel(ctx, id, func(b Booking, now time.Time) error {
		if !token.Matches(manageToken, b.manageTokenHash) {
			return ErrNotFound
		}
		if b.Status != StatusConfirmed {
			return ErrNotActive
		}
		return m.checkCutoff(ctx, b, now)
	})
}

// AdminCancel cancels a confirmed appointment regardless of the cutoff.
func (m *Manager) AdminCancel(ctx context.Context, id string) (Booking, error) {
	return m.cancel(ctx, id, func(b Booking, _ time.Time) error {
		if b.Status != StatusConfirmed {
			return ErrNotActive
		}
		return nil
	})
}

func (m *Manager) cancel(ctx context.Context, id string, allowed func(Booking, time.Time) error) (Booking, error) {
	if !ValidID(id) {
		return Booking{}, ErrNotFound
	}
	now := m.clock.Now()
	var out Booking
	err := pgx.BeginFunc(ctx, m.pool, func(tx pgx.Tx) error {
		b, err := lockAppointment(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := allowed(b, now); err != nil {
			return err
		}
		if out, err = markCancelled(ctx, tx, b.ID, now); err != nil {
			return err
		}
		if err := notify.VoidPending(ctx, tx, b.ID); err != nil {
			return err
		}
		return notify.Enqueue(ctx, tx, b.ID, now, notify.Planned{Kind: notify.KindCancellation, SendAt: now})
	})
	return out, err
}

func (m *Manager) checkCutoff(ctx context.Context, b Booking, now time.Time) error {
	loc, err := m.catalog.Location(ctx, b.LocationID)
	if err != nil {
		return err
	}
	if now.After(b.Start.Add(-loc.Settings.CancelCutoff)) {
		return ErrTooLate
	}
	return nil
}

// Reschedule moves a confirmed appointment to newStart with the same staff
// member. The old row is cancelled and a new confirmed row inserted in one
// transaction; the new booking keeps the customer, the price and the manage
// token, and points back with RescheduledFrom.
func (m *Manager) Reschedule(ctx context.Context, id, manageToken string, newStart time.Time) (Booking, error) {
	current, err := m.Get(ctx, id, manageToken)
	if err != nil {
		return Booking{}, err
	}
	if current.Status != StatusConfirmed {
		return Booking{}, ErrNotActive
	}
	now := m.clock.Now()
	if err := m.checkCutoff(ctx, current, now); err != nil {
		return Booking{}, err
	}
	svc, loc, err := m.serviceAndLocation(ctx, current.ServiceID)
	if err != nil {
		return Booking{}, err
	}
	if err := m.checkStart(ctx, loc, svc, current.StaffID, newStart, now); err != nil {
		return Booking{}, err
	}

	end := newStart.Add(svc.Duration)
	busyUntil := end.Add(svc.Buffer)
	var moved Booking
	err = pgx.BeginFunc(ctx, m.pool, func(tx pgx.Tx) error {
		old, err := lockAppointment(ctx, tx, current.ID)
		if err != nil {
			return err
		}
		if old.Status != StatusConfirmed {
			return ErrNotActive
		}
		// The exclusion constraint is checked row by row, so the old row has
		// to leave the active set before the new one goes in; otherwise a
		// move onto a period overlapping the old one would conflict with
		// itself.
		if _, err := markCancelled(ctx, tx, old.ID, now); err != nil {
			return err
		}
		if err := notify.VoidPending(ctx, tx, old.ID); err != nil {
			return err
		}
		if err := expireStaleHolds(ctx, tx, old.StaffID, newStart, busyUntil, now); err != nil {
			return err
		}
		moved, err = scanBooking(tx.QueryRow(ctx, `
			INSERT INTO bookings (kind, status, location_id, staff_id, service_id, starts_at, ends_at, busy_until,
			                      price_minor, currency, manage_token_hash, customer_name, customer_phone,
			                      customer_email, telegram_chat_id, rescheduled_from, created_at, confirmed_at)
			SELECT 'appointment', 'confirmed', location_id, staff_id, service_id, $2, $3, $4,
			       price_minor, currency, manage_token_hash, customer_name, customer_phone,
			       customer_email, telegram_chat_id, id, $5, $5
			FROM bookings WHERE id = $1
			RETURNING `+bookingColumns, old.ID, newStart, end, busyUntil, now))
		if err != nil {
			return err
		}
		return notify.Enqueue(ctx, tx, moved.ID, now, notify.ForConfirmed(newStart, now)...)
	})
	if db.IsExclusionViolation(err) {
		return Booking{}, ErrSlotTaken
	}
	if err != nil {
		return Booking{}, err
	}
	return moved, nil
}

func (m *Manager) checkStart(ctx context.Context, loc catalog.Location, svc catalog.Service, staffID int64, start, now time.Time) error {
	ok, err := m.offers(ctx, loc, svc, staffID, start, now)
	if err != nil {
		return err
	}
	if !ok {
		return whyNotBookable(loc, start, now)
	}
	return nil
}
