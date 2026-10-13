package httpapi

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeErrorsNameTheFieldWithoutGoDetails(t *testing.T) {
	cases := []struct {
		body string
		want string
	}{
		{`{"service_id":"1","start":"2026-10-14T10:00:00Z"}`, "service_id must be an integer"},
		{`{"service_id":1e40,"start":"2026-10-14T10:00:00Z"}`, "service_id must be an integer"},
		{`{"service_id":1,"start":42}`, "times must be RFC 3339"},
		{`{"service_id":1,"start":"tomorrow"}`, "times must be RFC 3339"},
		{`{"service_id":1,"start":"2026-10-14T10:00:00+25:00"}`, "times must be RFC 3339"},
		{`[1,2]`, "request body must be a JSON object"},
		{`{"service_id":1,"colour":"red"}`, `unknown field "colour"`},
		{`{"service_id":1,}`, "invalid JSON at byte 17"},
		{`{"service_id":`, "invalid JSON: the body ends early"},
	}
	for _, c := range cases {
		t.Run(c.body, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/v1/holds", strings.NewReader(c.body))
			var dst holdRequest
			err := decodeJSON(httptest.NewRecorder(), r, &dst)
			var apiErr *apiError
			if !errors.As(err, &apiErr) || apiErr.status != http.StatusBadRequest {
				t.Fatalf("got %v, want a 400 apiError", err)
			}
			if !strings.Contains(apiErr.message, c.want) {
				t.Errorf("message %q does not contain %q", apiErr.message, c.want)
			}
			for _, internal := range []string{"Go struct", "holdRequest", "int64", "time.Time", "2006-01-02"} {
				if strings.Contains(apiErr.message, internal) {
					t.Errorf("message %q exposes %q", apiErr.message, internal)
				}
			}
		})
	}
}

func TestDecodeErrorsForNestedFieldsUseTheirPath(t *testing.T) {
	type contact struct {
		Name   string `json:"name"`
		ChatID int64  `json:"telegram_chat_id"`
	}
	var dst struct {
		Customer contact `json:"customer"`
	}
	for body, want := range map[string]string{
		`{"customer":{"telegram_chat_id":"abc"}}`: "customer.telegram_chat_id must be an integer",
		`{"customer":{"name":["A"]}}`:             "customer.name must be a string",
		`{"customer":"Anna"}`:                     "customer must be an object",
	} {
		r := httptest.NewRequest(http.MethodPost, "/v1/bookings", strings.NewReader(body))
		err := decodeJSON(httptest.NewRecorder(), r, &dst)
		if err == nil || err.Error() != want {
			t.Errorf("%s: got %v, want %q", body, err, want)
		}
	}
}
