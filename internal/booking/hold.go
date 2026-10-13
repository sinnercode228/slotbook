package booking

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/sinnercode228/slotbook/internal/catalog"
	"github.com/sinnercode228/slotbook/internal/db"
	"github.com/sinnercode228/slotbook/internal/slots"
	"github.com/sinnercode228/slotbook/internal/token"
)

type HoldRequest struct {
	ServiceID int64
	// StaffID 0 lets the server pick: see orderByLoad.
	StaffID int64
	Start   time.Time
}

// Hold is a temporary reservation. Token is shown to the client once and
// confirms the hold; only its hash is stored.
type Hold struct {
	Token   string
	Booking Booking
}

// Hold reserves a slot until the location's hold TTL runs out. The start
// must be one the slot generator offers (SlotError otherwise); an overlap
// with another active booking is ErrSlotTaken.
func (m *Manager) Hold(ctx context.Context, req HoldRequest) (Hold, error) {
	svc, loc, err := m.serviceAndLocation(ctx, req.ServiceID)
	if err != nil {
		return Hold{}, err
	}
	staff, err := m.candidates(ctx, svc, req.StaffID)
	if err != nil {
		return Hold{}, err
	}
	now := m.clock.Now()
	var eligible []catalog.Staff
	for _, st := range staff {
		ok, err := m.offers(ctx, loc, svc, st.ID, req.Start, now)
		if err != nil {
			return Hold{}, err
		}
		if ok {
			eligible = append(eligible, st)
		}
	}
	if len(eligible) == 0 {
		return Hold{}, whyNotBookable(loc, req.Start, now)
	}
	if req.StaffID == 0 && len(eligible) > 1 {
		if eligible, err = m.orderByLoad(ctx, eligible, slots.DateOf(req.Start.In(loc.Zone)), loc); err != nil {
			return Hold{}, err
		}
	}

	plain, hash, err := token.New()
	if err != nil {
		return Hold{}, err
	}
	end := req.Start.Add(svc.Duration)
	for _, st := range eligible {
		b, err := m.insertHold(ctx, newHold{
			locationID: loc.ID,
			staffID:    st.ID,
			service:    svc,
			start:      req.Start,
			end:        end,
			busyUntil:  end.Add(svc.Buffer),
			tokenHash:  hash,
			expiresAt:  now.Add(loc.Settings.HoldTTL),
			now:        now,
		})
		if errors.Is(err, ErrSlotTaken) {
			// Lost to another booking; with any staff, try the next person.
			continue
		}
		if err != nil {
			return Hold{}, err
		}
		return Hold{Token: plain, Booking: b}, nil
	}
	return Hold{}, ErrSlotTaken
}

// orderByLoad sorts staff for an any-staff hold: fewest confirmed
// appointments on that local date first, then lowest id. It spreads work
// evenly and gives the same answer for the same state.
func (m *Manager) orderByLoad(ctx context.Context, staff []catalog.Staff, date slots.Date, loc catalog.Location) ([]catalog.Staff, error) {
	ids := make([]int64, len(staff))
	for i, st := range staff {
		ids[i] = st.ID
	}
	rows, err := m.pool.Query(ctx, `
		SELECT staff_id, count(*) FROM bookings
		WHERE staff_id = ANY($1) AND kind = 'appointment' AND status = 'confirmed'
		  AND starts_at >= $2 AND starts_at < $3
		GROUP BY staff_id`, ids, date.Start(loc.Zone), date.AddDays(1).Start(loc.Zone))
	if err != nil {
		return nil, fmt.Errorf("booking: count staff load: %w", err)
	}
	load := map[int64]int64{}
	var id, n int64
	if _, err := pgx.ForEachRow(rows, []any{&id, &n}, func() error { load[id] = n; return nil }); err != nil {
		return nil, fmt.Errorf("booking: count staff load: %w", err)
	}
	ordered := slices.Clone(staff)
	slices.SortFunc(ordered, func(a, b catalog.Staff) int {
		if load[a.ID] != load[b.ID] {
			return int(load[a.ID] - load[b.ID])
		}
		return int(a.ID - b.ID)
	})
	return ordered, nil
}

type newHold struct {
	locationID, staffID int64
	service             catalog.Service
	start, end          time.Time
	busyUntil           time.Time
	tokenHash           []byte
	expiresAt, now      time.Time
}

func (m *Manager) insertHold(ctx context.Context, h newHold) (Booking, error) {
	var b Booking
	err := pgx.BeginFunc(ctx, m.pool, func(tx pgx.Tx) error {
		if err := expireStaleHolds(ctx, tx, h.staffID, h.start, h.busyUntil, h.now); err != nil {
			return err
		}
		var err error
		b, err = scanBooking(tx.QueryRow(ctx, `
			INSERT INTO bookings (kind, status, location_id, staff_id, service_id, starts_at, ends_at, busy_until,
			                      price_minor, currency, hold_token_hash, hold_expires_at, created_at)
			VALUES ('appointment', 'held', $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
			RETURNING `+bookingColumns,
			h.locationID, h.staffID, h.service.ID, h.start, h.end, h.busyUntil,
			h.service.PriceMinor, h.service.Currency, h.tokenHash, h.expiresAt, h.now))
		return err
	})
	if db.IsExclusionViolation(err) {
		return Booking{}, ErrSlotTaken
	}
	if err != nil {
		return Booking{}, fmt.Errorf("booking: insert hold: %w", err)
	}
	return b, nil
}
