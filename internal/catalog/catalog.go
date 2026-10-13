// Package catalog reads what a chain offers: locations with their booking
// settings, services, staff and staff schedules.
package catalog

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinnercode228/slotbook/internal/config"
	"github.com/sinnercode228/slotbook/internal/db"
	"github.com/sinnercode228/slotbook/internal/slots"
)

var ErrNotFound = errors.New("catalog: not found")

type Location struct {
	ID       int64
	Slug     string
	Name     string
	TimeZone string
	Address  string
	Zone     *time.Location
	Settings config.LocationSettings
}

type Service struct {
	ID         int64
	LocationID int64
	Name       string
	Duration   time.Duration
	Buffer     time.Duration
	PriceMinor int64
	Currency   string
}

type Staff struct {
	ID         int64
	LocationID int64
	Name       string
	ServiceIDs []int64
}

func (s Staff) Offers(serviceID int64) bool { return slices.Contains(s.ServiceIDs, serviceID) }

// Schedule is one staff member's working hours: the weekly pattern plus the
// date overrides inside the requested range.
type Schedule struct {
	Weekly    slots.WeeklyHours
	Overrides map[slots.Date][]slots.Interval
}

type Store struct {
	db       db.Querier
	defaults config.LocationSettings
	zones    sync.Map // zone name -> *time.Location
}

func NewStore(q db.Querier, defaults config.LocationSettings) *Store {
	return &Store{db: q, defaults: defaults}
}

const locationColumns = `id, slug, name, time_zone, address, slot_step_minutes, min_notice_minutes,
	horizon_days, hold_ttl_seconds, cancel_cutoff_minutes`

func (s *Store) Locations(ctx context.Context) ([]Location, error) {
	rows, err := s.db.Query(ctx, `SELECT `+locationColumns+` FROM locations ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("catalog: list locations: %w", err)
	}
	defer rows.Close()
	var out []Location
	for rows.Next() {
		l, err := s.scanLocation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (s *Store) Location(ctx context.Context, id int64) (Location, error) {
	l, err := s.scanLocation(s.db.QueryRow(ctx, `SELECT `+locationColumns+` FROM locations WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Location{}, ErrNotFound
	}
	return l, err
}

func (s *Store) scanLocation(row pgx.Row) (Location, error) {
	var (
		l                                      Location
		step, notice, horizon, holdTTL, cutoff *int32
	)
	if err := row.Scan(&l.ID, &l.Slug, &l.Name, &l.TimeZone, &l.Address, &step, &notice, &horizon, &holdTTL, &cutoff); err != nil {
		return Location{}, err
	}
	zone, err := s.zone(l.TimeZone)
	if err != nil {
		return Location{}, fmt.Errorf("catalog: location %d: %w", l.ID, err)
	}
	l.Zone = zone
	l.Settings = s.defaults
	if step != nil {
		l.Settings.SlotStep = time.Duration(*step) * time.Minute
	}
	if notice != nil {
		l.Settings.MinNotice = time.Duration(*notice) * time.Minute
	}
	if horizon != nil {
		l.Settings.HorizonDays = int(*horizon)
	}
	if holdTTL != nil {
		l.Settings.HoldTTL = time.Duration(*holdTTL) * time.Second
	}
	if cutoff != nil {
		l.Settings.CancelCutoff = time.Duration(*cutoff) * time.Minute
	}
	return l, nil
}

func (s *Store) zone(name string) (*time.Location, error) {
	if z, ok := s.zones.Load(name); ok {
		return z.(*time.Location), nil
	}
	z, err := time.LoadLocation(name)
	if err != nil {
		return nil, err
	}
	s.zones.Store(name, z)
	return z, nil
}

const serviceColumns = `id, location_id, name, duration_minutes, buffer_after_minutes, price_minor, currency`

func scanService(row pgx.Row) (Service, error) {
	var (
		svc              Service
		duration, buffer int32
	)
	if err := row.Scan(&svc.ID, &svc.LocationID, &svc.Name, &duration, &buffer, &svc.PriceMinor, &svc.Currency); err != nil {
		return Service{}, err
	}
	svc.Duration = time.Duration(duration) * time.Minute
	svc.Buffer = time.Duration(buffer) * time.Minute
	return svc, nil
}

func (s *Store) Services(ctx context.Context, locationID int64) ([]Service, error) {
	rows, err := s.db.Query(ctx, `SELECT `+serviceColumns+` FROM services WHERE location_id = $1 AND active ORDER BY id`, locationID)
	if err != nil {
		return nil, fmt.Errorf("catalog: list services: %w", err)
	}
	defer rows.Close()
	var out []Service
	for rows.Next() {
		svc, err := scanService(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, svc)
	}
	return out, rows.Err()
}

func (s *Store) Service(ctx context.Context, id int64) (Service, error) {
	svc, err := scanService(s.db.QueryRow(ctx, `SELECT `+serviceColumns+` FROM services WHERE id = $1 AND active`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Service{}, ErrNotFound
	}
	return svc, err
}

const staffQuery = `
	SELECT st.id, st.location_id, st.name,
	       coalesce(array_agg(ss.service_id ORDER BY ss.service_id) FILTER (WHERE ss.service_id IS NOT NULL), '{}')
	FROM staff st
	LEFT JOIN staff_services ss ON ss.staff_id = st.id
	WHERE st.active AND %s
	GROUP BY st.id
	ORDER BY st.id`

// Staff lists active staff of a location; a non-zero serviceID keeps only
// those who offer that service.
func (s *Store) Staff(ctx context.Context, locationID, serviceID int64) ([]Staff, error) {
	rows, err := s.db.Query(ctx, fmt.Sprintf(staffQuery, `st.location_id = $1
		AND ($2::bigint = 0 OR EXISTS (SELECT 1 FROM staff_services x WHERE x.staff_id = st.id AND x.service_id = $2))`),
		locationID, serviceID)
	if err != nil {
		return nil, fmt.Errorf("catalog: list staff: %w", err)
	}
	return pgx.CollectRows(rows, scanStaff)
}

func (s *Store) StaffMember(ctx context.Context, id int64) (Staff, error) {
	rows, err := s.db.Query(ctx, fmt.Sprintf(staffQuery, `st.id = $1`), id)
	if err != nil {
		return Staff{}, fmt.Errorf("catalog: get staff: %w", err)
	}
	member, err := pgx.CollectExactlyOneRow(rows, scanStaff)
	if errors.Is(err, pgx.ErrNoRows) {
		return Staff{}, ErrNotFound
	}
	return member, err
}

func scanStaff(row pgx.CollectableRow) (Staff, error) {
	var st Staff
	err := row.Scan(&st.ID, &st.LocationID, &st.Name, &st.ServiceIDs)
	return st, err
}

// Schedule loads the weekly hours of a staff member and the overrides on
// dates from..to inclusive.
func (s *Store) Schedule(ctx context.Context, staffID int64, from, to slots.Date) (Schedule, error) {
	sched := Schedule{Overrides: map[slots.Date][]slots.Interval{}}

	rows, err := s.db.Query(ctx, `
		SELECT weekday, start_time, end_time FROM working_hours
		WHERE staff_id = $1 ORDER BY weekday, start_time`, staffID)
	if err != nil {
		return Schedule{}, fmt.Errorf("catalog: working hours: %w", err)
	}
	var (
		weekday    int16
		start, end pgtype.Time
	)
	_, err = pgx.ForEachRow(rows, []any{&weekday, &start, &end}, func() error {
		sched.Weekly[weekday] = append(sched.Weekly[weekday], slots.Interval{Start: timeOfDay(start), End: timeOfDay(end)})
		return nil
	})
	if err != nil {
		return Schedule{}, fmt.Errorf("catalog: working hours: %w", err)
	}

	rows, err = s.db.Query(ctx, `
		SELECT o.on_date, i.start_time, i.end_time
		FROM schedule_overrides o
		LEFT JOIN schedule_override_intervals i USING (staff_id, on_date)
		WHERE o.staff_id = $1 AND o.on_date BETWEEN $2 AND $3
		ORDER BY o.on_date, i.start_time`,
		staffID, pgDate(from), pgDate(to))
	if err != nil {
		return Schedule{}, fmt.Errorf("catalog: overrides: %w", err)
	}
	var day pgtype.Date
	_, err = pgx.ForEachRow(rows, []any{&day, &start, &end}, func() error {
		d := slots.DateOf(day.Time)
		intervals := sched.Overrides[d]
		if start.Valid {
			intervals = append(intervals, slots.Interval{Start: timeOfDay(start), End: timeOfDay(end)})
		}
		if intervals == nil {
			intervals = []slots.Interval{}
		}
		sched.Overrides[d] = intervals
		return nil
	})
	if err != nil {
		return Schedule{}, fmt.Errorf("catalog: overrides: %w", err)
	}
	return sched, nil
}

func timeOfDay(t pgtype.Time) slots.TimeOfDay {
	return slots.TimeOfDay(t.Microseconds / int64(time.Minute/time.Microsecond))
}

func pgDate(d slots.Date) pgtype.Date {
	return pgtype.Date{Time: d.Start(time.UTC), Valid: true}
}
