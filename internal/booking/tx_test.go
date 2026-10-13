package booking

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/sinnercode228/slotbook/internal/db"
)

// fakeDB hands out transactions that record their statements and how they
// end; any other method of pgx.Tx panics.
type fakeDB struct {
	begun, committed int
}

type fakeTx struct {
	pgx.Tx
	db     *fakeDB
	execs  []string
	closed bool
}

func (d *fakeDB) Begin(context.Context) (pgx.Tx, error) {
	d.begun++
	return &fakeTx{db: d}, nil
}

func (tx *fakeTx) Exec(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
	tx.execs = append(tx.execs, sql)
	return pgconn.CommandTag{}, nil
}

func (tx *fakeTx) Commit(context.Context) error {
	if tx.closed {
		return pgx.ErrTxClosed
	}
	tx.closed = true
	tx.db.committed++
	return nil
}

func (tx *fakeTx) Rollback(context.Context) error {
	if tx.closed {
		return pgx.ErrTxClosed
	}
	tx.closed = true
	return nil
}

type call struct {
	tx *fakeTx
	// before is what the transaction had run when the function was called.
	before []string
}

// failing returns a transaction function that fails with err the first n
// times and records every call.
func failing(n int, err error, calls *[]call) func(pgx.Tx) error {
	return func(tx pgx.Tx) error {
		ftx := tx.(*fakeTx)
		*calls = append(*calls, call{ftx, append([]string(nil), ftx.execs...)})
		if len(*calls) <= n {
			return err
		}
		return nil
	}
}

var (
	errDeadlock  = fmt.Errorf("insert: %w", &pgconn.PgError{Code: "40P01", Message: "deadlock detected"})
	errExclusion = &pgconn.PgError{Code: "23P01", Message: "conflicting key value violates exclusion constraint"}
)

func TestInTxRunsTheWholeTransactionAgainAfterADeadlock(t *testing.T) {
	var (
		fake  fakeDB
		calls []call
	)
	if err := inTx(context.Background(), &fake, failing(txAttempts-1, errDeadlock, &calls)); err != nil {
		t.Fatalf("got %v after %d deadlocks, want success", err, txAttempts-1)
	}
	if len(calls) != txAttempts || fake.begun != txAttempts || fake.committed != 1 {
		t.Fatalf("%d calls, %d transactions begun, %d committed; want %d, %d, 1", len(calls), fake.begun, fake.committed, txAttempts, txAttempts)
	}
	if len(calls[0].before) != 0 {
		t.Fatalf("the first attempt ran %q before the function", calls[0].before)
	}
	for i, c := range calls[1:] {
		if c.tx == calls[i].tx {
			t.Fatalf("attempt %d reused the aborted transaction", i+2)
		}
		if len(c.before) != 1 || !strings.Contains(c.before[0], "pg_advisory_xact_lock") {
			t.Fatalf("attempt %d ran %q before the function, want the retry lock", i+2, c.before)
		}
	}
}

func TestInTxGivesUpAfterTheLastAttempt(t *testing.T) {
	var (
		fake  fakeDB
		calls []call
	)
	err := inTx(context.Background(), &fake, failing(txAttempts, errDeadlock, &calls))
	if !errors.Is(err, errDeadlock) || !db.IsDeadlock(err) {
		t.Fatalf("got %v, want the deadlock", err)
	}
	if len(calls) != txAttempts || fake.committed != 0 {
		t.Fatalf("%d calls and %d commits, want %d and 0", len(calls), fake.committed, txAttempts)
	}
}

func TestInTxDoesNotRetryOtherErrors(t *testing.T) {
	for name, want := range map[string]error{
		"exclusion violation": errExclusion,
		"domain error":        ErrNotActive,
	} {
		var (
			fake  fakeDB
			calls []call
		)
		if err := inTx(context.Background(), &fake, failing(1, want, &calls)); !errors.Is(err, want) {
			t.Errorf("%s: got %v", name, err)
		}
		if len(calls) != 1 {
			t.Errorf("%s: ran %d times, want once", name, len(calls))
		}
	}
}

func TestInTxStopsRetryingWhenTheContextIsDone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var (
		fake  fakeDB
		calls int
	)
	err := inTx(ctx, &fake, func(pgx.Tx) error {
		calls++
		cancel()
		return errDeadlock
	})
	if !db.IsDeadlock(err) || calls != 1 {
		t.Fatalf("got %v after %d calls, want the deadlock after one", err, calls)
	}
}
