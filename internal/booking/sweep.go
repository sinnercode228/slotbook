package booking

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// IdempotencyKeyTTL is how long a confirm result can be replayed.
const IdempotencyKeyTTL = 24 * time.Hour

// ExpireHolds marks every lapsed hold as expired. Holds would not block new
// bookings anyway (see expireStaleHolds); this keeps the table honest for
// admins and reports.
func (m *Manager) ExpireHolds(ctx context.Context) (int64, error) {
	tag, err := m.pool.Exec(ctx,
		`UPDATE bookings SET status = 'expired' WHERE status = 'held' AND hold_expires_at <= $1`, m.clock.Now())
	if err != nil {
		return 0, fmt.Errorf("booking: expire holds: %w", err)
	}
	return tag.RowsAffected(), nil
}

// PruneIdempotencyKeys deletes keys older than IdempotencyKeyTTL.
func (m *Manager) PruneIdempotencyKeys(ctx context.Context) (int64, error) {
	tag, err := m.pool.Exec(ctx,
		`DELETE FROM idempotency_keys WHERE created_at < $1`, m.clock.Now().Add(-IdempotencyKeyTTL))
	if err != nil {
		return 0, fmt.Errorf("booking: prune idempotency keys: %w", err)
	}
	return tag.RowsAffected(), nil
}

// RunSweeper expires holds and prunes idempotency keys every interval until
// ctx is cancelled.
func (m *Manager) RunSweeper(ctx context.Context, interval time.Duration, log *slog.Logger) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if n, err := m.ExpireHolds(ctx); err != nil {
			if ctx.Err() == nil {
				log.Error("sweeper", "err", err)
			}
		} else if n > 0 {
			log.Info("expired stale holds", "count", n)
		}
		if _, err := m.PruneIdempotencyKeys(ctx); err != nil && ctx.Err() == nil {
			log.Error("sweeper", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
