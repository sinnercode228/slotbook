// Package token creates the secrets handed to customers (hold and manage
// tokens) and compares secrets without leaking timing information.
package token

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
)

// New returns a random URL-safe token and the SHA-256 hash to store in its
// place. Only the hash is persisted.
func New() (string, []byte, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", nil, fmt.Errorf("token: read random bytes: %w", err)
	}
	plain := base64.RawURLEncoding.EncodeToString(buf)
	return plain, Hash(plain), nil
}

// Hash returns the SHA-256 digest of a token. Tokens carry 256 bits of
// entropy, so a fast unsalted hash is enough to make a leaked table useless.
func Hash(plain string) []byte {
	sum := sha256.Sum256([]byte(plain))
	return sum[:]
}

// Matches reports whether plain is the token behind stored, in constant time.
func Matches(plain string, stored []byte) bool {
	if plain == "" || len(stored) != sha256.Size {
		return false
	}
	return subtle.ConstantTimeCompare(Hash(plain), stored) == 1
}

// Equal compares two secrets in constant time. Both sides are hashed first so
// that the comparison does not reveal the expected length either.
func Equal(given, expected string) bool {
	if given == "" || expected == "" {
		return false
	}
	return subtle.ConstantTimeCompare(Hash(given), Hash(expected)) == 1
}
