package token

import (
	"bytes"
	"testing"
)

func TestNewTokensAreRandomAndURLSafe(t *testing.T) {
	a, hashA, err := New()
	if err != nil {
		t.Fatal(err)
	}
	b, _, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatal("two tokens are equal")
	}
	if len(a) != 43 {
		t.Fatalf("token length %d, want 43 (32 bytes, unpadded base64url)", len(a))
	}
	for _, r := range a {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			t.Fatalf("token %q is not URL safe", a)
		}
	}
	if !bytes.Equal(hashA, Hash(a)) {
		t.Fatal("returned hash does not match Hash(token)")
	}
	if bytes.Contains(hashA, []byte(a)) {
		t.Fatal("hash contains the token")
	}
}

func TestMatches(t *testing.T) {
	plain, stored, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if !Matches(plain, stored) {
		t.Fatal("token does not match its own hash")
	}
	if Matches(plain+"x", stored) {
		t.Fatal("a different token matched")
	}
	if Matches("", stored) || Matches(plain, nil) || Matches("", nil) {
		t.Fatal("an empty token or hash matched")
	}
}

func TestEqual(t *testing.T) {
	if !Equal("s3cret-admin-token", "s3cret-admin-token") {
		t.Fatal("equal strings reported unequal")
	}
	for _, other := range []string{"", "s3cret-admin-toke", "s3cret-admin-token!", "S3cret-admin-token"} {
		if Equal("s3cret-admin-token", other) {
			t.Fatalf("matched %q", other)
		}
	}
	if Equal("", "") {
		t.Fatal("empty secrets must never match")
	}
}
