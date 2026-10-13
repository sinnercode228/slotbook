// Package db opens the PostgreSQL pool, applies the embedded migrations and
// classifies the database errors the application reacts to.
package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Querier is satisfied by *pgxpool.Pool and pgx.Tx, so queries can run
// either on their own or inside a caller's transaction.
type Querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Open connects a pool and checks that the database answers.
func Open(ctx context.Context, url string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("db: parse database url: %w", err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("db: connect: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("db: ping: %w", err)
	}
	return pool, nil
}

// SQLSTATE codes the application handles explicitly.
const (
	codeUniqueViolation    = "23505"
	codeExclusionViolation = "23P01"
	codeDeadlockDetected   = "40P01"
)

// IsExclusionViolation reports whether err comes from an exclusion
// constraint, which for bookings means the period is already taken.
func IsExclusionViolation(err error) bool { return hasCode(err, codeExclusionViolation) }

// IsDeadlock reports whether the deadlock detector aborted the transaction.
// Two inserts of overlapping periods at the same moment can end this way
// instead of with an exclusion violation; the transaction can run again.
func IsDeadlock(err error) bool { return hasCode(err, codeDeadlockDetected) }

func IsUniqueViolation(err error) bool { return hasCode(err, codeUniqueViolation) }

func hasCode(err error, code string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == code
}
