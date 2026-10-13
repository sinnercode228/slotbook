package booking

import (
	"context"
	"math/rand/v2"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/sinnercode228/slotbook/internal/db"
)

// txAttempts bounds how many times inTx runs a transaction.
const txAttempts = 5

type beginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// inTx runs fn in a transaction like pgx.BeginFunc and, when PostgreSQL
// aborts the transaction as a deadlock victim (40P01), runs fn again from the
// start in a new one, up to txAttempts times in all. When ctx is done it
// stops retrying and returns the last error.
//
// Two inserts of overlapping periods at the same moment can each wait for the
// other's entry in the exclusion constraint's index, and the deadlock
// detector aborts one of them instead of letting it fail with 23P01. With
// more such inserts it aborts one per deadlock_timeout until one is left. A
// victim that came straight back with a new entry would join that chain
// again, so a retry starts by taking an advisory lock and retries go one at
// a time.
func inTx(ctx context.Context, b beginner, fn func(pgx.Tx) error) error {
	for attempt := 1; ; attempt++ {
		err := pgx.BeginFunc(ctx, b, func(tx pgx.Tx) error {
			if attempt > 1 {
				if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('slotbook-deadlock-retry'))`); err != nil {
					return err
				}
			}
			return fn(tx)
		})
		if attempt == txAttempts || !db.IsDeadlock(err) {
			return err
		}
		base := time.Millisecond << attempt
		pause := time.NewTimer(base + rand.N(base))
		select {
		case <-ctx.Done():
			pause.Stop()
			return err
		case <-pause.C:
		}
	}
}
