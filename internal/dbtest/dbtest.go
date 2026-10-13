// Package dbtest gives each integration test its own freshly migrated schema
// in the database named by SLOTBOOK_TEST_DATABASE_URL. Tests skip when the
// variable is unset.
package dbtest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/sinnercode228/slotbook/internal/db"
)

const envURL = "SLOTBOOK_TEST_DATABASE_URL"

type options struct {
	maxConns int32
}

type Option func(*options)

// WithMaxConns raises the pool size for tests that need real concurrency.
func WithMaxConns(n int32) Option {
	return func(o *options) { o.maxConns = n }
}

// New returns a pool whose search_path points at a new schema with all
// migrations applied. The schema is dropped when the test ends.
func New(t testing.TB, opts ...Option) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv(envURL)
	if url == "" {
		t.Skip(envURL + " is not set")
	}
	o := options{maxConns: 8}
	for _, opt := range opts {
		opt(&o)
	}
	ctx := context.Background()

	admin, err := pgx.Connect(ctx, url)
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

	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatalf("dbtest: parse url: %v", err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	cfg.MaxConns = o.maxConns
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("dbtest: open pool: %v", err)
	}
	t.Cleanup(pool.Close)

	if _, err := db.Migrate(ctx, pool); err != nil {
		t.Fatalf("dbtest: migrate: %v", err)
	}
	return pool
}

func randomSuffix(t testing.TB) string {
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(buf)
}
