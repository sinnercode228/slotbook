package httpapi

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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

	open := cors([]string{"*"})(okHandler)
	rec = httptest.NewRecorder()
	open.ServeHTTP(rec, stranger)
	if rec.Header().Get("Access-Control-Allow-Origin") != "https://evil.example.com" {
		t.Fatal("* should allow any origin")
	}
}

func TestRequestIDIsGeneratedOrPassedThrough(t *testing.T) {
	var seen string
	h := withRequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = requestIDFrom(r.Context())
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if seen == "" || rec.Header().Get("X-Request-ID") != seen {
		t.Fatalf("generated id %q, header %q", seen, rec.Header().Get("X-Request-ID"))
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
