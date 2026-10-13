package db_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/sinnercode228/slotbook/internal/db"
	"github.com/sinnercode228/slotbook/internal/dbtest"
)

func TestMigrateIsIdempotent(t *testing.T) {
	pool := dbtest.New(t)
	applied, err := db.Migrate(context.Background(), pool)
	if err != nil {
		t.Fatal(err)
	}
	if len(applied) != 0 {
		t.Fatalf("second run applied %v", applied)
	}
	var n int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM schema_migrations").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		t.Fatal("no versions recorded")
	}
}

// insertStaff creates the minimum rows a booking refers to.
func insertStaff(t *testing.T, pool *pgxpool.Pool) (locationID, staffID int64) {
	t.Helper()
	ctx := context.Background()
	err := pool.QueryRow(ctx, `INSERT INTO locations (slug, name, time_zone) VALUES ('l', 'L', 'Europe/Berlin') RETURNING id`).Scan(&locationID)
	if err != nil {
		t.Fatal(err)
	}
	err = pool.QueryRow(ctx, `INSERT INTO staff (location_id, name) VALUES ($1, 'S') RETURNING id`, locationID).Scan(&staffID)
	if err != nil {
		t.Fatal(err)
	}
	return locationID, staffID
}

func insertBlock(pool *pgxpool.Pool, locationID, staffID int64, start, end time.Time, status string) error {
	_, err := pool.Exec(context.Background(), `
		INSERT INTO bookings (kind, status, location_id, staff_id, starts_at, ends_at, busy_until, period, created_at)
		VALUES ('block', $1, $2, $3, $4, $5, $5, tstzrange($4, $5, '[)'), $4)`, status, locationID, staffID, start, end)
	return err
}

func TestExclusionConstraintRejectsOverlap(t *testing.T) {
	pool := dbtest.New(t)
	locationID, staffID := insertStaff(t, pool)
	nine := time.Date(2026, time.October, 20, 7, 0, 0, 0, time.UTC)

	if err := insertBlock(pool, locationID, staffID, nine, nine.Add(time.Hour), "confirmed"); err != nil {
		t.Fatal(err)
	}
	err := insertBlock(pool, locationID, staffID, nine.Add(30*time.Minute), nine.Add(90*time.Minute), "confirmed")
	if !db.IsExclusionViolation(err) {
		t.Fatalf("overlapping insert: got %v, want an exclusion violation", err)
	}
	if err := insertBlock(pool, locationID, staffID, nine.Add(time.Hour), nine.Add(2*time.Hour), "confirmed"); err != nil {
		t.Fatalf("adjacent period rejected: %v", err)
	}
}

func TestExclusionConstraintIgnoresInactiveRows(t *testing.T) {
	pool := dbtest.New(t)
	locationID, staffID := insertStaff(t, pool)
	ctx := context.Background()
	start := time.Date(2026, time.October, 20, 7, 0, 0, 0, time.UTC)

	var serviceID int64
	err := pool.QueryRow(ctx, `
		INSERT INTO services (location_id, name, duration_minutes, price_minor, currency)
		VALUES ($1, 'Haircut', 60, 3800, 'EUR') RETURNING id`, locationID).Scan(&serviceID)
	if err != nil {
		t.Fatal(err)
	}
	insertHold := func(status string) error {
		_, err := pool.Exec(ctx, `
			INSERT INTO bookings (kind, status, location_id, staff_id, service_id, starts_at, ends_at, busy_until,
			                      period, hold_expires_at, created_at)
			VALUES ('appointment', $1, $2, $3, $4, $5, $6, $6, tstzrange($5, $6, '[)'), $5, $5)`,
			status, locationID, staffID, serviceID, start, start.Add(time.Hour))
		return err
	}
	if err := insertHold("expired"); err != nil {
		t.Fatal(err)
	}
	if err := insertHold("held"); err != nil {
		t.Fatalf("an expired hold blocked a new one: %v", err)
	}
	if err := insertHold("held"); !db.IsExclusionViolation(err) {
		t.Fatalf("second active hold: got %v, want an exclusion violation", err)
	}
}

func TestPeriodMustMatchStartAndBusyUntil(t *testing.T) {
	pool := dbtest.New(t)
	locationID, staffID := insertStaff(t, pool)
	start := time.Date(2026, time.October, 20, 7, 0, 0, 0, time.UTC)
	_, err := pool.Exec(context.Background(), `
		INSERT INTO bookings (kind, status, location_id, staff_id, starts_at, ends_at, busy_until, period, created_at)
		VALUES ('block', 'confirmed', $1, $2, $3, $4, $4, tstzrange($3::timestamptz, $3::timestamptz + interval '1 minute', '[)'), $3)`,
		locationID, staffID, start, start.Add(time.Hour))
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.ConstraintName != "bookings_period_matches" {
		t.Fatalf("got %v, want a bookings_period_matches violation", err)
	}
}
