package booking

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/sinnercode228/slotbook/internal/token"
)

// Release gives a running hold back before it runs out, so that its time is
// free for others at once. The hold token authorizes it, and a wrong token
// looks like a missing hold (ErrHoldNotFound). Releasing a released hold
// again succeeds, so a client may retry freely; a hold that was confirmed is
// ErrAlreadyConfirmed and one that ran out is ErrHoldExpired.
func (m *Manager) Release(ctx context.Context, id, holdToken string) error {
	if holdToken == "" {
		return &ValidationError{"hold_token", "is required"}
	}
	if !ValidID(id) {
		return ErrHoldNotFound
	}
	now := m.clock.Now()
	return inTx(ctx, m.pool, func(tx pgx.Tx) error {
		var (
			status    Status
			tokenHash []byte
			expiresAt *time.Time
		)
		// The row lock puts a release and a confirm of the same hold in
		// order, so the later one sees what the earlier one did.
		err := tx.QueryRow(ctx, `
			SELECT status, hold_token_hash, hold_expires_at FROM bookings
			WHERE id = $1 AND kind = 'appointment' FOR UPDATE`, id).Scan(&status, &tokenHash, &expiresAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrHoldNotFound
		}
		if err != nil {
			return err
		}
		if !token.Matches(holdToken, tokenHash) {
			return ErrHoldNotFound
		}
		switch {
		case status == StatusReleased:
			return nil
		case status == StatusConfirmed || status == StatusCancelled:
			return ErrAlreadyConfirmed
		case status != StatusHeld || !expiresAt.After(now):
			return ErrHoldExpired
		}
		_, err = tx.Exec(ctx, `UPDATE bookings SET status = 'released' WHERE id = $1`, id)
		return err
	})
}
