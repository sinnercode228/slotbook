package idempotency

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

type confirmBody struct {
	HoldToken string `json:"hold_token"`
	Name      string `json:"name"`
	Phone     string `json:"phone"`
}

func TestFingerprintIgnoresFieldOrderAndWhitespace(t *testing.T) {
	var a, b confirmBody
	if err := json.Unmarshal([]byte(`{"hold_token":"h","name":"Anna","phone":"+7 900 123-45-67"}`), &a); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte("{\n  \"phone\": \"+7 900 123-45-67\",\n  \"name\": \"Anna\",\n  \"hold_token\": \"h\"\n}"), &b); err != nil {
		t.Fatal(err)
	}
	fa, err := Fingerprint(a)
	if err != nil {
		t.Fatal(err)
	}
	fb, err := Fingerprint(b)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(fa, fb) {
		t.Fatal("same request produced different fingerprints")
	}
}

func TestFingerprintChangesWithAnyField(t *testing.T) {
	base := confirmBody{HoldToken: "h", Name: "Anna", Phone: "+7 900 123-45-67"}
	f0, _ := Fingerprint(base)
	for _, changed := range []confirmBody{
		{HoldToken: "h2", Name: "Anna", Phone: "+7 900 123-45-67"},
		{HoldToken: "h", Name: "Anya", Phone: "+7 900 123-45-67"},
		{HoldToken: "h", Name: "Anna", Phone: "+7 900 123-45-68"},
	} {
		f, _ := Fingerprint(changed)
		if bytes.Equal(f0, f) {
			t.Fatalf("%+v has the same fingerprint as %+v", changed, base)
		}
	}
}

func TestValidateKey(t *testing.T) {
	for _, ok := range []string{"a", "0b7d6c1e-4f5a-4c1b-9d2e-3f4a5b6c7d8e", strings.Repeat("k", MaxKeyLength)} {
		if err := ValidateKey(ok); err != nil {
			t.Errorf("ValidateKey(%q): %v", ok, err)
		}
	}
	for _, bad := range []string{"", strings.Repeat("k", MaxKeyLength+1), "has space", "tab\tkey", "ключ"} {
		if err := ValidateKey(bad); err == nil {
			t.Errorf("ValidateKey(%q) accepted", bad)
		}
	}
}
