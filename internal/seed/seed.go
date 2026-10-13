// Package seed loads a chain of locations, services, staff and schedules
// from a JSON file. Locations are matched by slug, and one that already
// exists is left untouched, so seeding twice is harmless.
package seed

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/sinnercode228/slotbook/internal/slots"
)

type Chain struct {
	Brand     string     `json:"brand"`
	Locations []Location `json:"locations"`
}

type Location struct {
	Slug     string    `json:"slug"`
	Name     string    `json:"name"`
	TimeZone string    `json:"time_zone"`
	Address  string    `json:"address"`
	Settings Settings  `json:"settings"`
	Services []Service `json:"services"`
	Staff    []Staff   `json:"staff"`
}

// Settings override the defaults from internal/config; omitted fields keep them.
type Settings struct {
	SlotStepMinutes     *int `json:"slot_step_minutes,omitempty"`
	MinNoticeMinutes    *int `json:"min_notice_minutes,omitempty"`
	HorizonDays         *int `json:"horizon_days,omitempty"`
	HoldTTLSeconds      *int `json:"hold_ttl_seconds,omitempty"`
	CancelCutoffMinutes *int `json:"cancel_cutoff_minutes,omitempty"`
}

type Service struct {
	Key                string `json:"key"`
	Name               string `json:"name"`
	DurationMinutes    int    `json:"duration_minutes"`
	BufferAfterMinutes int    `json:"buffer_after_minutes"`
	PriceMinor         int64  `json:"price_minor"`
	Currency           string `json:"currency"`
}

type Staff struct {
	Key         string            `json:"key"`
	Name        string            `json:"name"`
	Services    []string          `json:"services"`
	WeeklyHours slots.WeeklyHours `json:"weekly_hours"`
	Overrides   []Override        `json:"overrides"`
}

type Override struct {
	Date      slots.Date       `json:"date"`
	Intervals []slots.Interval `json:"intervals"`
}

// Result maps the keys used in the chain to the ids they received.
type Result struct {
	Locations map[string]int64
	Services  map[string]int64
	Staff     map[string]int64
	// Skipped lists slugs of locations that already existed.
	Skipped []string
}

func LoadFile(path string) (Chain, error) {
	f, err := os.Open(path)
	if err != nil {
		return Chain{}, err
	}
	defer f.Close()
	return Parse(f)
}

// Parse decodes and validates a chain; unknown fields are an error so that
// typos in a seed file do not pass silently.
func Parse(r io.Reader) (Chain, error) {
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	var c Chain
	if err := dec.Decode(&c); err != nil {
		return Chain{}, fmt.Errorf("seed: decode: %w", err)
	}
	if err := c.Validate(); err != nil {
		return Chain{}, err
	}
	return c, nil
}

var currencyCode = regexp.MustCompile(`^[A-Z]{3}$`)

func (c Chain) Validate() error {
	slugs := map[string]bool{}
	serviceKeys := map[string]bool{}
	staffKeys := map[string]bool{}
	for _, l := range c.Locations {
		if l.Slug == "" || l.Name == "" {
			return errors.New("seed: every location needs a slug and a name")
		}
		if slugs[l.Slug] {
			return fmt.Errorf("seed: duplicate location slug %q", l.Slug)
		}
		slugs[l.Slug] = true
		if _, err := time.LoadLocation(l.TimeZone); err != nil || l.TimeZone == "" {
			return fmt.Errorf("seed: location %s: unknown time zone %q", l.Slug, l.TimeZone)
		}
		local := map[string]bool{}
		for _, s := range l.Services {
			if s.Key == "" || serviceKeys[s.Key] {
				return fmt.Errorf("seed: location %s: missing or duplicate service key %q", l.Slug, s.Key)
			}
			if s.DurationMinutes <= 0 || s.BufferAfterMinutes < 0 || s.PriceMinor < 0 {
				return fmt.Errorf("seed: service %s: duration must be positive, buffer and price not negative", s.Key)
			}
			if !currencyCode.MatchString(s.Currency) {
				return fmt.Errorf("seed: service %s: currency %q is not an ISO 4217 code", s.Key, s.Currency)
			}
			serviceKeys[s.Key] = true
			local[s.Key] = true
		}
		for _, st := range l.Staff {
			if st.Key == "" || staffKeys[st.Key] {
				return fmt.Errorf("seed: location %s: missing or duplicate staff key %q", l.Slug, st.Key)
			}
			staffKeys[st.Key] = true
			for _, key := range st.Services {
				if !local[key] {
					return fmt.Errorf("seed: staff %s: service %q is not offered at %s", st.Key, key, l.Slug)
				}
			}
			if err := st.WeeklyHours.Validate(); err != nil {
				return fmt.Errorf("seed: staff %s: %w", st.Key, err)
			}
			for _, o := range st.Overrides {
				for _, iv := range o.Intervals {
					if err := iv.Validate(); err != nil {
						return fmt.Errorf("seed: staff %s on %s: %w", st.Key, o.Date, err)
					}
				}
			}
		}
	}
	return nil
}

// Apply inserts the chain in one transaction.
func Apply(ctx context.Context, pool *pgxpool.Pool, c Chain) (Result, error) {
	if err := c.Validate(); err != nil {
		return Result{}, err
	}
	res := Result{
		Locations: map[string]int64{},
		Services:  map[string]int64{},
		Staff:     map[string]int64{},
	}
	err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		for _, l := range c.Locations {
			inserted, err := applyLocation(ctx, tx, l, &res)
			if err != nil {
				return fmt.Errorf("seed: location %s: %w", l.Slug, err)
			}
			if !inserted {
				res.Skipped = append(res.Skipped, l.Slug)
			}
		}
		return nil
	})
	if err != nil {
		return Result{}, err
	}
	return res, nil
}

func applyLocation(ctx context.Context, tx pgx.Tx, l Location, res *Result) (bool, error) {
	var locationID int64
	err := tx.QueryRow(ctx, `
		INSERT INTO locations (slug, name, time_zone, address, slot_step_minutes, min_notice_minutes,
		                       horizon_days, hold_ttl_seconds, cancel_cutoff_minutes)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (slug) DO NOTHING
		RETURNING id`,
		l.Slug, l.Name, l.TimeZone, l.Address, l.Settings.SlotStepMinutes, l.Settings.MinNoticeMinutes,
		l.Settings.HorizonDays, l.Settings.HoldTTLSeconds, l.Settings.CancelCutoffMinutes,
	).Scan(&locationID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	res.Locations[l.Slug] = locationID

	for _, s := range l.Services {
		var id int64
		err := tx.QueryRow(ctx, `
			INSERT INTO services (location_id, name, duration_minutes, buffer_after_minutes, price_minor, currency)
			VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
			locationID, s.Name, s.DurationMinutes, s.BufferAfterMinutes, s.PriceMinor, s.Currency,
		).Scan(&id)
		if err != nil {
			return false, fmt.Errorf("service %s: %w", s.Key, err)
		}
		res.Services[s.Key] = id
	}

	for _, st := range l.Staff {
		var id int64
		err := tx.QueryRow(ctx, `INSERT INTO staff (location_id, name) VALUES ($1, $2) RETURNING id`, locationID, st.Name).Scan(&id)
		if err != nil {
			return false, fmt.Errorf("staff %s: %w", st.Key, err)
		}
		res.Staff[st.Key] = id
		if err := applyStaff(ctx, tx, id, st, res.Services); err != nil {
			return false, fmt.Errorf("staff %s: %w", st.Key, err)
		}
	}
	return true, nil
}

func applyStaff(ctx context.Context, tx pgx.Tx, staffID int64, st Staff, serviceIDs map[string]int64) error {
	for _, key := range st.Services {
		if _, err := tx.Exec(ctx, `INSERT INTO staff_services (staff_id, service_id) VALUES ($1, $2)`, staffID, serviceIDs[key]); err != nil {
			return err
		}
	}
	for day, intervals := range st.WeeklyHours {
		for _, iv := range intervals {
			_, err := tx.Exec(ctx, `INSERT INTO working_hours (staff_id, weekday, start_time, end_time) VALUES ($1, $2, $3, $4)`,
				staffID, day, pgTime(iv.Start), pgTime(iv.End))
			if err != nil {
				return err
			}
		}
	}
	for _, o := range st.Overrides {
		date := pgtype.Date{Time: o.Date.Start(time.UTC), Valid: true}
		if _, err := tx.Exec(ctx, `INSERT INTO schedule_overrides (staff_id, on_date) VALUES ($1, $2)`, staffID, date); err != nil {
			return err
		}
		for _, iv := range o.Intervals {
			_, err := tx.Exec(ctx, `
				INSERT INTO schedule_override_intervals (staff_id, on_date, start_time, end_time)
				VALUES ($1, $2, $3, $4)`, staffID, date, pgTime(iv.Start), pgTime(iv.End))
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func pgTime(t slots.TimeOfDay) pgtype.Time {
	return pgtype.Time{Microseconds: int64(t) * int64(time.Minute/time.Microsecond), Valid: true}
}
