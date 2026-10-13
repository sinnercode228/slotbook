package booking

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/sinnercode228/slotbook/internal/idempotency"
	"github.com/sinnercode228/slotbook/internal/notify"
	"github.com/sinnercode228/slotbook/internal/token"
)

type ConfirmRequest struct {
	HoldToken string   `json:"hold_token"`
	Customer  Customer `json:"customer"`
}

// Confirmation carries the manage token in plain text; it is returned once
// (and again on an idempotent replay) and stored only as a hash.
type Confirmation struct {
	Booking     Booking `json:"booking"`
	ManageToken string  `json:"manage_token"`
}

const confirmScope = "confirm"

// Confirm turns a running hold into a confirmed booking and queues its
// notifications in the same transaction.
//
// With a non-empty idempotencyKey, the first successful result is stored
// with the key: a retry with the same key and request gets that result back
// (replayed is true) and a retry with another request gets
// ErrIdempotencyMismatch. Failures are not stored, so a retry after one runs
// again.
func (m *Manager) Confirm(ctx context.Context, req ConfirmRequest, idempotencyKey string) (result Confirmation, replayed bool, err error) {
	if req.Customer, err = req.Customer.Normalize(); err != nil {
		return Confirmation{}, false, err
	}
	if req.HoldToken == "" {
		return Confirmation{}, false, &ValidationError{"hold_token", "is required"}
	}
	var fingerprint []byte
	if idempotencyKey != "" {
		if err := idempotency.ValidateKey(idempotencyKey); err != nil {
			return Confirmation{}, false, &ValidationError{"Idempotency-Key", err.Error()}
		}
		if fingerprint, err = idempotency.Fingerprint(req); err != nil {
			return Confirmation{}, false, err
		}
	}

	now := m.clock.Now()
	err = pgx.BeginFunc(ctx, m.pool, func(tx pgx.Tx) error {
		if idempotencyKey != "" {
			stored, found, err := reserveKey(ctx, tx, confirmScope, idempotencyKey, fingerprint, now)
			if err != nil {
				return err
			}
			if found {
				if !bytes.Equal(stored.requestHash, fingerprint) {
					return ErrIdempotencyMismatch
				}
				replayed = true
				return json.Unmarshal(stored.response, &result)
			}
		}

		manage, manageHash, err := token.New()
		if err != nil {
			return err
		}
		c := req.Customer
		b, err := scanBooking(tx.QueryRow(ctx, `
			UPDATE bookings
			SET status = 'confirmed', customer_name = $2, customer_phone = $3, customer_email = nullif($4::text, ''),
			    telegram_chat_id = nullif($5::bigint, 0), manage_token_hash = $6, confirmed_at = $7
			WHERE hold_token_hash = $1 AND status = 'held' AND hold_expires_at > $7
			RETURNING `+bookingColumns,
			token.Hash(req.HoldToken), c.Name, c.Phone, c.Email, c.TelegramChatID, manageHash, now))
		if errors.Is(err, pgx.ErrNoRows) {
			return whyNotConfirmable(ctx, tx, req.HoldToken)
		}
		if err != nil {
			return err
		}
		if err := notify.Enqueue(ctx, tx, b.ID, now, notify.ForConfirmed(b.Start, now)...); err != nil {
			return err
		}
		result = Confirmation{Booking: b, ManageToken: manage}
		if idempotencyKey != "" {
			return storeResponse(ctx, tx, confirmScope, idempotencyKey, result)
		}
		return nil
	})
	if err != nil {
		return Confirmation{}, false, err
	}
	return result, replayed, nil
}

func whyNotConfirmable(ctx context.Context, tx pgx.Tx, holdToken string) error {
	var status Status
	err := tx.QueryRow(ctx, `SELECT status FROM bookings WHERE hold_token_hash = $1`, token.Hash(holdToken)).Scan(&status)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return ErrHoldNotFound
	case err != nil:
		return err
	case status == StatusConfirmed || status == StatusCancelled:
		return ErrAlreadyConfirmed
	default:
		return ErrHoldExpired
	}
}

type storedKey struct {
	requestHash []byte
	response    []byte
}

// reserveKey inserts the idempotency key, or returns the stored row if the
// key is already used. A concurrent request with the same key blocks on the
// insert until the first transaction finishes, then sees its result.
func reserveKey(ctx context.Context, tx pgx.Tx, scope, key string, fingerprint []byte, now time.Time) (storedKey, bool, error) {
	tag, err := tx.Exec(ctx, `
		INSERT INTO idempotency_keys (scope, key, request_hash, created_at) VALUES ($1, $2, $3, $4)
		ON CONFLICT (scope, key) DO NOTHING`, scope, key, fingerprint, now)
	if err != nil {
		return storedKey{}, false, fmt.Errorf("booking: reserve idempotency key: %w", err)
	}
	if tag.RowsAffected() == 1 {
		return storedKey{}, false, nil
	}
	var s storedKey
	err = tx.QueryRow(ctx, `SELECT request_hash, response FROM idempotency_keys WHERE scope = $1 AND key = $2`, scope, key).
		Scan(&s.requestHash, &s.response)
	if err != nil {
		return storedKey{}, false, fmt.Errorf("booking: read idempotency key: %w", err)
	}
	return s, true, nil
}

// storeResponse keeps the full result, manage token included, because a
// client that lost the first response needs that token. The sweeper deletes
// keys after a day, which bounds how long the plain token sits in the table.
func storeResponse(ctx context.Context, tx pgx.Tx, scope, key string, v any) error {
	body, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE idempotency_keys SET response = $3 WHERE scope = $1 AND key = $2`, scope, key, body)
	return err
}
