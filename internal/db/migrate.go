package db

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

// migrationLock is an arbitrary advisory lock key shared by every migrator,
// so replicas starting at the same time apply migrations one after another.
const migrationLock int64 = 7_243_901_552

// Migrate applies the embedded migrations that have not been applied yet,
// each in its own transaction, and returns the versions it applied.
func Migrate(ctx context.Context, pool *pgxpool.Pool) ([]string, error) {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("migrate: acquire connection: %w", err)
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", migrationLock); err != nil {
		return nil, fmt.Errorf("migrate: take lock: %w", err)
	}
	defer func() {
		// The lock belongs to the session; if unlocking fails the connection
		// is closed instead so the lock cannot outlive this call.
		if _, err := conn.Exec(context.WithoutCancel(ctx), "SELECT pg_advisory_unlock($1)", migrationLock); err != nil {
			conn.Conn().Close(context.WithoutCancel(ctx))
		}
	}()

	if _, err := conn.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    text PRIMARY KEY,
			applied_at timestamptz NOT NULL DEFAULT now()
		)`); err != nil {
		return nil, fmt.Errorf("migrate: create schema_migrations: %w", err)
	}

	rows, err := conn.Query(ctx, "SELECT version FROM schema_migrations")
	if err != nil {
		return nil, fmt.Errorf("migrate: read applied versions: %w", err)
	}
	applied, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, fmt.Errorf("migrate: read applied versions: %w", err)
	}

	names, err := fs.Glob(migrationFiles, "migrations/*.sql")
	if err != nil {
		return nil, err
	}
	slices.Sort(names)

	var done []string
	for _, name := range names {
		version := strings.TrimSuffix(path.Base(name), ".sql")
		if slices.Contains(applied, version) {
			continue
		}
		body, err := migrationFiles.ReadFile(name)
		if err != nil {
			return done, err
		}
		err = pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, string(body)); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, "INSERT INTO schema_migrations (version) VALUES ($1)", version)
			return err
		})
		if err != nil {
			return done, fmt.Errorf("migrate: apply %s: %w", version, err)
		}
		done = append(done, version)
	}
	return done, nil
}
