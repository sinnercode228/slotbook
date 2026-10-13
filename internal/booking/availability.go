package booking

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/sinnercode228/slotbook/internal/catalog"
	"github.com/sinnercode228/slotbook/internal/slots"
)

// MaxAvailabilityDays bounds one availability request.
const MaxAvailabilityDays = 31

type AvailabilityQuery struct {
	// LocationID, when set, must be the location of the service.
	LocationID int64
	ServiceID  int64
	// StaffID 0 means any staff member who offers the service.
	StaffID  int64
	From, To slots.Date
}

type Availability struct {
	Location catalog.Location
	Service  catalog.Service
	Days     []slots.Day
}

// Availability lists bookable starts per local date. With any staff, a
// start is listed if at least one staff member can take it.
func (m *Manager) Availability(ctx context.Context, q AvailabilityQuery) (Availability, error) {
	if q.To.Before(q.From) || q.From.DaysUntil(q.To) >= MaxAvailabilityDays {
		return Availability{}, &ValidationError{"to", fmt.Sprintf("the range must cover 1 to %d days", MaxAvailabilityDays)}
	}
	svc, loc, err := m.serviceAndLocation(ctx, q.ServiceID)
	if err != nil {
		return Availability{}, err
	}
	if q.LocationID != 0 && svc.LocationID != q.LocationID {
		return Availability{}, ErrServiceNotFound
	}
	staff, err := m.candidates(ctx, svc, q.StaffID)
	if err != nil {
		return Availability{}, err
	}

	now := m.clock.Now()
	// Busy time from the neighbouring days matters too: a buffer can run
	// past midnight.
	windowStart, windowEnd := q.From.AddDays(-1).Start(loc.Zone), q.To.AddDays(2).Start(loc.Zone)
	days := emptyDays(q.From, q.To)
	for _, st := range staff {
		sched, err := m.catalog.Schedule(ctx, st.ID, q.From, q.To)
		if err != nil {
			return Availability{}, err
		}
		busy, err := busyIntervals(ctx, m.pool, st.ID, windowStart, windowEnd, now)
		if err != nil {
			return Availability{}, fmt.Errorf("booking: busy intervals: %w", err)
		}
		own, err := slots.Generate(slotRequest(loc, svc, sched, busy, now, q.From, q.To))
		if err != nil {
			return Availability{}, err
		}
		for i := range days {
			days[i].Slots = mergeStarts(days[i].Slots, own[i].Slots)
		}
	}
	return Availability{Location: loc, Service: svc, Days: days}, nil
}

func emptyDays(from, to slots.Date) []slots.Day {
	var days []slots.Day
	for d := from; !d.After(to); d = d.AddDays(1) {
		days = append(days, slots.Day{Date: d, Slots: []time.Time{}})
	}
	return days
}

func mergeStarts(a, b []time.Time) []time.Time {
	out := append(a, b...)
	slices.SortFunc(out, func(x, y time.Time) int { return x.Compare(y) })
	return slices.CompactFunc(out, time.Time.Equal)
}

func slotRequest(loc catalog.Location, svc catalog.Service, sched catalog.Schedule, busy []slots.Busy, now time.Time, from, to slots.Date) slots.Request {
	return slots.Request{
		Location:    loc.Zone,
		Weekly:      sched.Weekly,
		Overrides:   sched.Overrides,
		Duration:    svc.Duration,
		Buffer:      svc.Buffer,
		Step:        loc.Settings.SlotStep,
		MinNotice:   loc.Settings.MinNotice,
		HorizonDays: loc.Settings.HorizonDays,
		Busy:        busy,
		Now:         now,
		From:        from,
		To:          to,
	}
}

func (m *Manager) serviceAndLocation(ctx context.Context, serviceID int64) (catalog.Service, catalog.Location, error) {
	svc, err := m.catalog.Service(ctx, serviceID)
	if errors.Is(err, catalog.ErrNotFound) {
		return catalog.Service{}, catalog.Location{}, ErrServiceNotFound
	}
	if err != nil {
		return catalog.Service{}, catalog.Location{}, err
	}
	loc, err := m.catalog.Location(ctx, svc.LocationID)
	if err != nil {
		return catalog.Service{}, catalog.Location{}, err
	}
	return svc, loc, nil
}

// candidates returns the staff member asked for, or everyone at the
// location who offers the service when staffID is 0.
func (m *Manager) candidates(ctx context.Context, svc catalog.Service, staffID int64) ([]catalog.Staff, error) {
	if staffID == 0 {
		return m.catalog.Staff(ctx, svc.LocationID, svc.ID)
	}
	member, err := m.catalog.StaffMember(ctx, staffID)
	if errors.Is(err, catalog.ErrNotFound) {
		return nil, ErrInvalidStaff
	}
	if err != nil {
		return nil, err
	}
	if member.LocationID != svc.LocationID || !member.Offers(svc.ID) {
		return nil, ErrInvalidStaff
	}
	return []catalog.Staff{member}, nil
}

// offers reports whether start is a slot of the staff member's schedule,
// ignoring existing bookings: overlaps are left to the database.
func (m *Manager) offers(ctx context.Context, loc catalog.Location, svc catalog.Service, staffID int64, start, now time.Time) (bool, error) {
	date := slots.DateOf(start.In(loc.Zone))
	sched, err := m.catalog.Schedule(ctx, staffID, date, date)
	if err != nil {
		return false, err
	}
	return slots.Contains(slotRequest(loc, svc, sched, nil, now, date, date), start)
}

// whyNotBookable explains a start that offers rejected.
func whyNotBookable(loc catalog.Location, start, now time.Time) *SlotError {
	s := loc.Settings
	if start.Before(now.Add(s.MinNotice)) {
		return &SlotError{fmt.Sprintf("bookings need at least %d minutes notice", int(s.MinNotice.Minutes()))}
	}
	last := slots.DateOf(now.In(loc.Zone)).AddDays(s.HorizonDays)
	if slots.DateOf(start.In(loc.Zone)).After(last) {
		return &SlotError{fmt.Sprintf("bookings open %d days ahead (the booking horizon)", s.HorizonDays)}
	}
	return &SlotError{"not an available start time for this service and staff"}
}
