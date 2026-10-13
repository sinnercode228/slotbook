// Package idempotency holds the helpers behind the Idempotency-Key header:
// key validation and a stable fingerprint of the request it was sent with.
package idempotency

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
)

// MaxKeyLength bounds client-chosen keys; a UUID needs 36 characters.
const MaxKeyLength = 255

// ValidateKey accepts 1 to MaxKeyLength printable ASCII characters without spaces.
func ValidateKey(key string) error {
	if key == "" {
		return errors.New("idempotency key is empty")
	}
	if len(key) > MaxKeyLength {
		return fmt.Errorf("idempotency key is longer than %d characters", MaxKeyLength)
	}
	for i := 0; i < len(key); i++ {
		if key[i] <= ' ' || key[i] > '~' {
			return errors.New("idempotency key must be printable ASCII without spaces")
		}
	}
	return nil
}

// Fingerprint hashes the decoded request rather than the raw body, so the
// same request sent with other whitespace or field order still matches.
// v should be a struct: encoding/json writes struct fields in a fixed order.
func Fingerprint(v any) ([]byte, error) {
	canonical, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("idempotency: encode request: %w", err)
	}
	sum := sha256.Sum256(canonical)
	return sum[:], nil
}
