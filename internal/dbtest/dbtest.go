// Package dbtest gives each integration test its own freshly migrated schema
// in the database named by SLOTBOOK_TEST_DATABASE_URL. Tests skip when the
// variable is unset.
package dbtest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/url"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/sinnercode228/slotbook/internal/db"
)

const envURL = "SLOTBOOK_TEST_DATABASE_URL"

type options struct {
	maxConns int32
	settings []setting
}

type setting struct{ name, value string }

type Option func(*options)

// WithMaxConns raises the pool size for tests that need real concurrency.
func WithMaxConns(n int32) Option {
	return func(o *options) { o.maxConns = n }
}

// WithSetting sets a configuration parameter on every connection of the
// pool. The test is skipped if the role is not allowed to set it.
func WithSetting(name, value string) Option {
	return func(o *options) { o.settings = append(o.settings, setting{name, value}) }
}

// New returns a pool whose search_path points at a new schema with all
// migrations applied. The schema is dropped when the test ends.
func New(t testing.TB, opts ...Option) *pgxpool.Pool {
	t.Helper()
	o := options{maxConns: 8}
	for _, opt := range opts {
		opt(&o)
	}
	cfg, err := pgxpool.ParseConfig(EmptySchema(t))
	if err != nil {
		t.Fatalf("dbtest: parse url: %v", err)
	}
	cfg.MaxConns = o.maxConns
	if len(o.settings) > 0 {
		cfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
			for _, s := range o.settings {
				if _, err := conn.Exec(ctx, "SELECT set_config($1, $2, false)", s.name, s.value); err != nil {
					return err
				}
			}
			return nil
		}
	}
	ctx := context.Background()
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("dbtest: open pool: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(ctx); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "42501" { // insufficient_privilege
			t.Skipf("dbtest: the role in %s may not change a setting this test needs: %v", envURL, err)
		}
		t.Fatalf("dbtest: connect: %v", err)
	}

	if _, err := db.Migrate(ctx, pool); err != nil {
		t.Fatalf("dbtest: migrate: %v", err)
	}
	return pool
}

// EmptySchema creates a schema that is dropped when the test ends and
// returns a database URL whose search_path points at it.
func EmptySchema(t testing.TB) string {
	t.Helper()
	raw := os.Getenv(envURL)
	if raw == "" {
		t.Skip(envURL + " is not set")
	}
	ctx := context.Background()

	admin, err := pgx.Connect(ctx, raw)
	if err != nil {
		t.Fatalf("dbtest: connect: %v", err)
	}
	t.Cleanup(func() { admin.Close(context.Background()) })

	// btree_gist is database-wide. Installing it in public up front keeps
	// the migration from creating it inside a test schema, where dropping
	// the schema would take the extension (and every other test's
	// constraint) with it.
	err = pgx.BeginFunc(ctx, admin, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtext('slotbook-dbtest-extension'))"); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS btree_gist WITH SCHEMA public")
		return err
	})
	if err != nil {
		t.Fatalf("dbtest: install btree_gist: %v", err)
	}

	schema := "test_" + randomSuffix(t)
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatalf("dbtest: create schema: %v", err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Errorf("dbtest: drop schema %s: %v", schema, err)
		}
	})

	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("dbtest: %s must be a URL: %v", envURL, err)
	}
	q := u.Query()
	q.Set("search_path", schema+",public")
	u.RawQuery = q.Encode()
	return u.String()
}

func randomSuffix(t testing.TB) string {
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(buf)
}
