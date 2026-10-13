package booking

import (
	"net/mail"
	"strings"
	"unicode/utf8"
)

// Customer holds the contact details given when a hold is confirmed.
type Customer struct {
	Name           string `json:"name"`
	Phone          string `json:"phone"`
	Email          string `json:"email,omitempty"`
	TelegramChatID int64  `json:"telegram_chat_id,omitempty"`
}

// Normalize trims the fields and checks them. Phone numbers are kept as
// typed; the check only makes sure they look like one (7 to 15 digits, the
// E.164 maximum, plus common separators).
func (c Customer) Normalize() (Customer, error) {
	c.Name = strings.TrimSpace(c.Name)
	c.Phone = strings.TrimSpace(c.Phone)
	c.Email = strings.TrimSpace(c.Email)

	if c.Name == "" || utf8.RuneCountInString(c.Name) > 100 {
		return Customer{}, &ValidationError{"customer.name", "must be 1 to 100 characters"}
	}
	digits := 0
	for _, r := range c.Phone {
		switch {
		case r >= '0' && r <= '9':
			digits++
		case strings.ContainsRune("+-() ", r):
		default:
			return Customer{}, &ValidationError{"customer.phone", "may contain only digits, spaces, +, -, ( and )"}
		}
	}
	if digits < 7 || digits > 15 {
		return Customer{}, &ValidationError{"customer.phone", "must have 7 to 15 digits"}
	}
	if c.Email != "" {
		addr, err := mail.ParseAddress(c.Email)
		if err != nil || addr.Address != c.Email || len(c.Email) > 254 {
			return Customer{}, &ValidationError{"customer.email", "is not a valid address"}
		}
	}
	return c, nil
}
