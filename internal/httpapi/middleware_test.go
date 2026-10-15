package httpapi

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sinnercode228/slotbook/internal/clock"
	"github.com/sinnercode228/slotbook/internal/ratelimit"
)

var quietLog = slog.New(slog.NewTextHandler(io.Discard, nil))

var okHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNoContent)
})

func TestRequireAdmin(t *testing.T) {
	const secret = "0123456789abcdef0123456789abcdef"
	h := requireAdmin(secret)(okHandler)
	cases := map[string]struct {
		header string
		want   int
	}{
		"no header":        {"", http.StatusUnauthorized},
		"wrong token":      {"Bearer 0123456789abcdef0123456789abcdeX", http.StatusUnauthorized},
		"prefix of token":  {"Bearer 0123456789abcdef", http.StatusUnauthorized},
		"basic scheme":     {"Basic " + secret, http.StatusUnauthorized},
		"empty bearer":     {"Bearer ", http.StatusUnauthorized},
		"right token":      {"Bearer " + secret, http.StatusNoContent},
		"lowercase scheme": {"bearer " + secret, http.StatusNoContent},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/v1/admin/bookings", nil)
			if c.header != "" {
				req.Header.Set("Authorization", c.header)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != c.want {
				t.Fatalf("status %d, want %d", rec.Code, c.want)
			}
			if c.want == http.StatusUnauthorized {
				if rec.Header().Get("WWW-Authenticate") == "" {
					t.Error("missing WWW-Authenticate")
				}
				assertError(t, rec, "unauthorized")
			}
		})
	}
}

func TestCORS(t *testing.T) {
	h := cors([]string{"https://widget.example.com"})(okHandler)

	preflight := httptest.NewRequest(http.MethodOptions, "/v1/holds", nil)
	preflight.Header.Set("Origin", "https://widget.example.com")
	preflight.Header.Set("Access-Control-Request-Method", "POST")
	preflight.Header.Set("Access-Control-Request-Headers", "content-type, idempotency-key")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, preflight)
	if rec.Code != http.StatusNoContent || rec.Header().Get("Access-Control-Allow-Origin") != "https://widget.example.com" {
		t.Fatalf("preflight: %d %v", rec.Code, rec.Header())
	}
	allowHeaders := strings.ToLower(rec.Header().Get("Access-Control-Allow-Headers"))
	for _, want := range []string{"idempotency-key", "x-manage-token", "authorization", "content-type"} {
		if !strings.Contains(allowHeaders, want) {
			t.Errorf("Access-Control-Allow-Headers %q lacks %s", allowHeaders, want)
		}
	}

	stranger := httptest.NewRequest(http.MethodGet, "/v1/locations", nil)
	stranger.Header.Set("Origin", "https://evil.example.com")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, stranger)
	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("an unlisted origin got CORS headers")
	}

	simple := httptest.NewRequest(http.MethodGet, "/v1/locations", nil)
	simple.Header.Set("Origin", "https://widget.example.com")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, simple)
	if rec.Code != http.StatusNoContent || rec.Header().Get("Access-Control-Allow-Origin") != "https://widget.example.com" {
		t.Fatalf("simple request: %d %v", rec.Code, rec.Header())
	}
	if !strings.Contains(rec.Header().Get("Vary"), "Origin") {
		t.Fatal("responses that depend on Origin must say so in Vary")
	}
	if expose := rec.Header().Get("Access-Control-Expose-Headers"); !strings.Contains(expose, "Retry-After") {
		t.Fatalf("Access-Control-Expose-Headers %q hides Retry-After from the widget", expose)
	}

	open := cors([]string{"*"})(okHandler)
	rec = httptest.NewRecorder()
	open.ServeHTTP(rec, stranger)
	if rec.Header().Get("Access-Control-Allow-Origin") != "https://evil.example.com" {
		t.Fatal("* should allow any origin")
	}
}

func TestRequestIDIsCreatedOrPassedThrough(t *testing.T) {
	var seen string
	h := withRequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = requestIDFrom(r.Context())
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if seen == "" || rec.Header().Get("X-Request-ID") != seen {
		t.Fatalf("new id %q, header %q", seen, rec.Header().Get("X-Request-ID"))
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Request-ID", "edge-42")
	h.ServeHTTP(httptest.NewRecorder(), req)
	if seen != "edge-42" {
		t.Fatalf("incoming id not kept: %q", seen)
	}

	req.Header.Set("X-Request-ID", "bad id with spaces\n")
	h.ServeHTTP(httptest.NewRecorder(), req)
	if seen == "" || strings.ContainsAny(seen, " \n") {
		t.Fatalf("unsafe incoming id kept: %q", seen)
	}
}

func TestLimitByClient(t *testing.T) {
	clk := clock.NewFake(time.Date(2026, time.October, 12, 8, 0, 0, 0, time.UTC))
	limiter := ratelimit.New(ratelimit.Limit{Count: 2, Per: time.Minute, Burst: 2}, clk)
	h := limitByClient(limiter, prefixes(t, "10.0.0.0/8"))(okHandler)
	send := func(peer string, headers ...string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/v1/holds", nil)
		req.RemoteAddr = peer
		for i := 0; i+1 < len(headers); i += 2 {
			req.Header.Set(headers[i], headers[i+1])
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	for i := 0; i < 2; i++ {
		if rec := send("203.0.113.7:40000"); rec.Code != http.StatusNoContent {
			t.Fatalf("request %d: status %d", i+1, rec.Code)
		}
	}
	rec := send("203.0.113.7:40001")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("third request: status %d, want 429", rec.Code)
	}
	if got := rec.Header().Get("Retry-After"); got != "30" {
		t.Fatalf("Retry-After %q, want 30", got)
	}
	assertError(t, rec, "rate_limited")

	if rec := send("203.0.113.7:40002", "X-Forwarded-For", "198.51.100.20"); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("a made-up X-Forwarded-For from a direct client got status %d", rec.Code)
	}
	if rec := send("198.51.100.20:40000"); rec.Code != http.StatusNoContent {
		t.Fatalf("another address: status %d", rec.Code)
	}

	// Behind the trusted proxy each forwarded client has its own bucket.
	for _, client := range []string{"192.0.2.1", "192.0.2.1", "192.0.2.2", "192.0.2.2"} {
		if rec := send("10.0.0.5:443", "X-Forwarded-For", client); rec.Code != http.StatusNoContent {
			t.Fatalf("%s through the proxy: status %d", client, rec.Code)
		}
	}
	if rec := send("10.0.0.5:443", "X-Forwarded-For", "192.0.2.1"); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("third request of 192.0.2.1 through the proxy: status %d", rec.Code)
	}

	clk.Advance(30 * time.Second)
	if rec := send("203.0.113.7:40003"); rec.Code != http.StatusNoContent {
		t.Fatalf("after Retry-After: status %d", rec.Code)
	}
}

func TestRetryAfterRoundsUp(t *testing.T) {
	clk := clock.NewFake(time.Date(2026, time.October, 12, 8, 0, 0, 0, time.UTC))
	// One token every 8.57 seconds.
	h := limitByClient(ratelimit.New(ratelimit.Limit{Count: 7, Per: time.Minute, Burst: 1}, clk), nil)(okHandler)
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/v1/holds", nil))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/holds", nil))
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") != "9" {
		t.Fatalf("status %d, Retry-After %q; want 429 and 9", rec.Code, rec.Header().Get("Retry-After"))
	}
	clk.Advance(8 * time.Second)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/holds", nil))
	if rec.Header().Get("Retry-After") != "1" {
		t.Fatalf("Retry-After %q with 0.57 s to go, want 1", rec.Header().Get("Retry-After"))
	}
}

func TestRecoverTurnsPanicsIntoJSON(t *testing.T) {
	h := recoverPanics(quietLog)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") }))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status %d", rec.Code)
	}
	assertError(t, rec, "internal")
}

func assertError(t *testing.T, rec *httptest.ResponseRecorder, code string) {
	t.Helper()
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("content type %q", ct)
	}
	var body struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("error body %q: %v", rec.Body.String(), err)
	}
	if body.Error.Code != code || body.Error.Message == "" {
		t.Fatalf("error %+v, want code %q with a message", body.Error, code)
	}
}
