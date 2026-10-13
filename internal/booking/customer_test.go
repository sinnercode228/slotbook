package booking

import (
	"errors"
	"strings"
	"testing"
)

func TestCustomerNormalize(t *testing.T) {
	c, err := Customer{Name: "  Anna  ", Phone: " +7 900 123-45-67 ", Email: " anna@example.com "}.Normalize()
	if err != nil {
		t.Fatal(err)
	}
	if c.Name != "Anna" || c.Phone != "+7 900 123-45-67" || c.Email != "anna@example.com" {
		t.Fatalf("normalized %+v", c)
	}
	if _, err := (Customer{Name: "Jonas", Phone: "(030) 1234 5678", TelegramChatID: -1001234}).Normalize(); err != nil {
		t.Fatalf("valid customer rejected: %v", err)
	}
}

func TestCustomerValidation(t *testing.T) {
	cases := map[string]Customer{
		"missing name":    {Phone: "+7 900 123-45-67"},
		"long name":       {Name: strings.Repeat("a", 101), Phone: "+7 900 123-45-67"},
		"missing phone":   {Name: "Anna"},
		"letters":         {Name: "Anna", Phone: "+7 900 CALL-ME"},
		"too few digits":  {Name: "Anna", Phone: "12-34"},
		"too many digits": {Name: "Anna", Phone: "+7 900 123 45 67 89 01 23"},
		"bad email":       {Name: "Anna", Phone: "+7 900 123-45-67", Email: "anna@"},
		"display name":    {Name: "Anna", Phone: "+7 900 123-45-67", Email: "Anna <anna@example.com>"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := c.Normalize()
			var verr *ValidationError
			if !errors.As(err, &verr) {
				t.Fatalf("got %v, want a ValidationError", err)
			}
		})
	}
}
