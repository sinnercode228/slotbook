package booking

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/sinnercode228/slotbook/internal/catalog"
	"github.com/sinnercode228/slotbook/internal/db"
	"github.com/sinnercode228/slotbook/internal/slots"
)

// MaxBlockLength bounds a single admin time block.
const MaxBlockLength = 31 * 24 * time.Hour

// BlockRequest takes a staff member out of the schedule for [Start, End),
// for a break, training or a day off.
type BlockRequest struct {
	StaffID int64
	Start   time.Time
	End     time.Time
	Note    string
}

// CreateBlock inserts an admin time block. Blocks live in the bookings
// table, so the same exclusion constraint keeps them clear of appointments.
func (m *Manager) CreateBlock(ctx context.Context, req BlockRequest) (Booking, error) {
	if !req.Start.Before(req.End) {
		return Booking{}, &ValidationError{"end", "must be after start"}
	}
	if req.End.Sub(req.Start) > MaxBlockLength {
		return Booking{}, &ValidationError{"end", "a block can last at most 31 days"}
	}
	member, err := m.catalog.StaffMember(ctx, req.StaffID)
	if errors.Is(err, catalog.ErrNotFound) {
		return Booking{}, ErrInvalidStaff
	}
	if err != nil {
		return Booking{}, err
	}
	now := m.clock.Now()
	var b Booking
	err = pgx.BeginFunc(ctx, m.pool, func(tx pgx.Tx) error {
		if err := expireStaleHolds(ctx, tx, member.ID, req.Start, req.End, now); err != nil {
			return err
		}
		var err error
		b, err = scanBooking(tx.QueryRow(ctx, `
			INSERT INTO bookings (kind, status, location_id, staff_id, starts_at, ends_at, busy_until, note, created_at)
			VALUES ('block', 'confirmed', $1, $2, $3, $4, $4, $5, $6)
			RETURNING `+bookingColumns, member.LocationID, member.ID, req.Start, req.End, req.Note, now))
		return err
	})
	if db.IsExclusionViolation(err) {
		return Booking{}, ErrSlotTaken
	}
	if err != nil {
		return Booking{}, fmt.Errorf("booking: insert block: %w", err)
	}
	return b, nil
}

func (m *Manager) DeleteBlock(ctx context.Context, id string) error {
	if !ValidID(id) {
		return ErrNotFound
	}
	tag, err := m.pool.Exec(ctx, `DELETE FROM bookings WHERE id = $1 AND kind = 'block'`, id)
	if err != nil {
		return fmt.Errorf("booking: delete block: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ListForDay returns appointments (except lapsed holds) and blocks of a
// location that overlap the given local date, ordered by start.
func (m *Manager) ListForDay(ctx context.Context, locationID int64, date slots.Date) ([]Booking, error) {
	loc, err := m.catalog.Location(ctx, locationID)
	if errors.Is(err, catalog.ErrNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	rows, err := m.pool.Query(ctx, `
		SELECT `+bookingColumns+` FROM bookings
		WHERE location_id = $1 AND starts_at < $3 AND busy_until > $2 AND status <> 'expired'
		ORDER BY starts_at, staff_id, created_at`,
		loc.ID, date.Start(loc.Zone), date.AddDays(1).Start(loc.Zone))
	if err != nil {
		return nil, fmt.Errorf("booking: list day: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Booking, error) { return scanBooking(row) })
}
