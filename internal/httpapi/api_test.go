package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/sinnercode228/slotbook/internal/booking"
	"github.com/sinnercode228/slotbook/internal/catalog"
	"github.com/sinnercode228/slotbook/internal/clock"
	"github.com/sinnercode228/slotbook/internal/config"
	"github.com/sinnercode228/slotbook/internal/dbtest"
	"github.com/sinnercode228/slotbook/internal/httpapi"
	"github.com/sinnercode228/slotbook/internal/seed"
	"github.com/sinnercode228/slotbook/internal/slots"
)

const adminToken = "test-admin-token-0123456789abcdef"

// Monday 2026-10-12, 10:00 in Berlin.
var now = time.Date(2026, time.October, 12, 8, 0, 0, 0, time.UTC)

type api struct {
	t       *testing.T
	handler http.Handler
	clock   *clock.Fake
	ids     seed.Result
}

func newAPI(t *testing.T, staticDir string) api {
	t.Helper()
	pool := dbtest.New(t)
	var weekdays slots.WeeklyHours
	for d := time.Monday; d <= time.Friday; d++ {
		weekdays[d] = []slots.Interval{{Start: 9 * 60, End: 13 * 60}, {Start: 14 * 60, End: 18 * 60}}
	}
	ids, err := seed.Apply(context.Background(), pool, seed.Chain{Locations: []seed.Location{
		{
			Slug: "berlin", Name: "Snipwell Berlin", TimeZone: "Europe/Berlin",
			Services: []seed.Service{{Key: "cut", Name: "Haircut", DurationMinutes: 45, BufferAfterMinutes: 15, PriceMinor: 3800, Currency: "EUR"}},
			Staff: []seed.Staff{
				{Key: "lena", Name: "Lena H.", Services: []string{"cut"}, WeeklyHours: weekdays},
				{Key: "jonas", Name: "Jonas W.", Services: []string{"cut"}, WeeklyHours: weekdays},
			},
		},
		{
			Slug: "moscow", Name: "Snipwell Moscow", TimeZone: "Europe/Moscow",
			Services: []seed.Service{{Key: "msk-cut", Name: "Men's haircut", DurationMinutes: 60, PriceMinor: 250000, Currency: "RUB"}},
			Staff:    []seed.Staff{{Key: "arina", Name: "Arina V.", Services: []string{"msk-cut"}, WeeklyHours: weekdays}},
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	clk := clock.NewFake(now)
	cat := catalog.NewStore(pool, config.LocationDefaults)
	handler := httpapi.New(httpapi.Deps{
		DB:          pool,
		Catalog:     cat,
		Bookings:    booking.NewManager(pool, cat, clk),
		Clock:       clk,
		Log:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		AdminToken:  adminToken,
		CORSOrigins: []string{"https://widget.example.com"},
		StaticDir:   staticDir,
	})
	return api{t: t, handler: handler, clock: clk, ids: ids}
}

func (a api) do(method, path string, body any, headers ...string) *httptest.ResponseRecorder {
	a.t.Helper()
	var reader io.Reader
	switch b := body.(type) {
	case nil:
	case string:
		reader = strings.NewReader(b)
	default:
		raw, err := json.Marshal(b)
		if err != nil {
			a.t.Fatal(err)
		}
		reader = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, reader)
	if reader != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	rec := httptest.NewRecorder()
	a.handler.ServeHTTP(rec, req)
	return rec
}

func (a api) admin(method, path string, body any) *httptest.ResponseRecorder {
	a.t.Helper()
	return a.do(method, path, body, "Authorization", "Bearer "+adminToken)
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	return v
}

func expectStatus(t *testing.T, rec *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rec.Code != want {
		t.Fatalf("status %d, want %d; body %s", rec.Code, want, rec.Body.String())
	}
}

func expectError(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	expectStatus(t, rec, status)
	body := decode[struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}](t, rec)
	if body.Error.Code != code || body.Error.Message == "" {
		t.Fatalf("error %+v, want code %q", body.Error, code)
	}
}

type bookingView struct {
	ID              string `json:"id"`
	Kind            string `json:"kind"`
	Status          string `json:"status"`
	StaffID         int64  `json:"staff_id"`
	Start           string `json:"start"`
	End             string `json:"end"`
	BusyUntil       string `json:"busy_until"`
	TimeZone        string `json:"time_zone"`
	RescheduledFrom string `json:"rescheduled_from"`
	Price           *struct {
		AmountMinor int64  `json:"amount_minor"`
		Currency    string `json:"currency"`
	} `json:"price"`
	Customer *struct {
		Name  string `json:"name"`
		Phone string `json:"phone"`
	} `json:"customer"`
}

type holdView struct {
	HoldToken string      `json:"hold_token"`
	ExpiresAt string      `json:"expires_at"`
	Booking   bookingView `json:"booking"`
}

type confirmationView struct {
	Booking     bookingView `json:"booking"`
	ManageToken string      `json:"manage_token"`
}

func (a api) hold(serviceKey, staffKey, start string) holdView {
	a.t.Helper()
	body := map[string]any{"service_id": a.ids.Services[serviceKey], "start": start}
	if staffKey != "" {
		body["staff_id"] = a.ids.Staff[staffKey]
	}
	rec := a.do(http.MethodPost, "/v1/holds", body)
	expectStatus(a.t, rec, http.StatusCreated)
	return decode[holdView](a.t, rec)
}

var customer = map[string]any{"name": "Anna", "phone": "+7 900 123-45-67", "email": "anna@example.com"}

func (a api) confirm(holdToken string) confirmationView {
	a.t.Helper()
	rec := a.do(http.MethodPost, "/v1/bookings", map[string]any{"hold_token": holdToken, "customer": customer})
	expectStatus(a.t, rec, http.StatusCreated)
	return decode[confirmationView](a.t, rec)
}

func TestHealthEndpoints(t *testing.T) {
	a := newAPI(t, "")
	expectStatus(t, a.do(http.MethodGet, "/healthz", nil), http.StatusOK)
	rec := a.do(http.MethodGet, "/readyz", nil)
	expectStatus(t, rec, http.StatusOK)
	if rec.Header().Get("X-Request-ID") == "" {
		t.Fatal("responses should carry a request id")
	}
}

func TestUnknownRoutesAndMethodsAnswerInJSON(t *testing.T) {
	a := newAPI(t, "")
	expectError(t, a.do(http.MethodGet, "/v1/nothing-here", nil), http.StatusNotFound, "not_found")
	rec := a.do(http.MethodDelete, "/v1/locations", nil)
	expectError(t, rec, http.StatusMethodNotAllowed, "method_not_allowed")
	if !strings.Contains(rec.Header().Get("Allow"), http.MethodGet) {
		t.Fatalf("Allow header %q", rec.Header().Get("Allow"))
	}
}

func TestCatalogEndpoints(t *testing.T) {
	a := newAPI(t, "")
	berlin := strconv.FormatInt(a.ids.Locations["berlin"], 10)

	locations := decode[struct {
		Locations []struct {
			ID       int64  `json:"id"`
			TimeZone string `json:"time_zone"`
			Settings struct {
				SlotStepMinutes int `json:"slot_step_minutes"`
			} `json:"settings"`
		} `json:"locations"`
	}](t, a.do(http.MethodGet, "/v1/locations", nil))
	if len(locations.Locations) != 2 || locations.Locations[0].TimeZone != "Europe/Berlin" || locations.Locations[0].Settings.SlotStepMinutes != 15 {
		t.Fatalf("locations %+v", locations)
	}
	expectStatus(t, a.do(http.MethodGet, "/v1/locations/"+berlin, nil), http.StatusOK)
	expectError(t, a.do(http.MethodGet, "/v1/locations/999999", nil), http.StatusNotFound, "not_found")
	expectError(t, a.do(http.MethodGet, "/v1/locations/abc", nil), http.StatusBadRequest, "bad_request")

	services := decode[struct {
		Services []struct {
			Name            string `json:"name"`
			DurationMinutes int    `json:"duration_minutes"`
			Price           struct {
				AmountMinor int64  `json:"amount_minor"`
				Currency    string `json:"currency"`
			} `json:"price"`
		} `json:"services"`
	}](t, a.do(http.MethodGet, "/v1/locations/"+berlin+"/services", nil))
	if len(services.Services) != 1 || services.Services[0].Price.Currency != "EUR" || services.Services[0].DurationMinutes != 45 {
		t.Fatalf("services %+v", services)
	}

	staff := decode[struct {
		Staff []struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
		} `json:"staff"`
	}](t, a.do(http.MethodGet, "/v1/locations/"+berlin+"/staff?service_id="+strconv.FormatInt(a.ids.Services["cut"], 10), nil))
	if len(staff.Staff) != 2 {
		t.Fatalf("staff %+v", staff)
	}
}

func TestSlotsEndpoint(t *testing.T) {
	a := newAPI(t, "")
	berlin := strconv.FormatInt(a.ids.Locations["berlin"], 10)
	cut := strconv.FormatInt(a.ids.Services["cut"], 10)

	rec := a.do(http.MethodGet, "/v1/locations/"+berlin+"/slots?service_id="+cut+"&from=2026-10-14&to=2026-10-15", nil)
	expectStatus(t, rec, http.StatusOK)
	got := decode[struct {
		TimeZone string `json:"time_zone"`
		Days     []struct {
			Date  string   `json:"date"`
			Slots []string `json:"slots"`
		} `json:"days"`
	}](t, rec)
	if got.TimeZone != "Europe/Berlin" || len(got.Days) != 2 || got.Days[0].Date != "2026-10-14" {
		t.Fatalf("slots %+v", got)
	}
	if first := got.Days[0].Slots[0]; first != "2026-10-14T09:00:00+02:00" {
		t.Fatalf("first slot %q", first)
	}

	moscowCut := strconv.FormatInt(a.ids.Services["msk-cut"], 10)
	for name, query := range map[string]string{
		"missing service":  "",
		"bad date":         "?service_id=" + cut + "&from=14.10.2026",
		"bad staff":        "?service_id=" + cut + "&staff_id=x",
		"range too long":   "?service_id=" + cut + "&from=2026-10-14&to=2026-12-14",
		"foreign service":  "?service_id=" + moscowCut,
		"staff without it": "?service_id=" + cut + "&staff_id=" + strconv.FormatInt(a.ids.Staff["arina"], 10),
	} {
		rec := a.do(http.MethodGet, "/v1/locations/"+berlin+"/slots"+query, nil)
		if rec.Code < 400 || rec.Code >= 500 {
			t.Errorf("%s: status %d", name, rec.Code)
		}
	}
}

func TestBookingLifecycle(t *testing.T) {
	a := newAPI(t, "")
	ten := "2026-10-14T10:00:00+02:00"

	h := a.hold("cut", "lena", ten)
	if h.HoldToken == "" || h.Booking.Status != "held" || h.ExpiresAt != "2026-10-12T10:10:00+02:00" {
		t.Fatalf("hold %+v", h)
	}
	if h.Booking.End != "2026-10-14T10:45:00+02:00" || h.Booking.BusyUntil != "2026-10-14T11:00:00+02:00" || h.Booking.TimeZone != "Europe/Berlin" {
		t.Fatalf("hold booking %+v", h.Booking)
	}

	taken := a.do(http.MethodPost, "/v1/holds", map[string]any{"service_id": a.ids.Services["cut"], "staff_id": a.ids.Staff["lena"], "start": "2026-10-14T10:30:00+02:00"})
	expectError(t, taken, http.StatusConflict, "slot_taken")
	misaligned := a.do(http.MethodPost, "/v1/holds", map[string]any{"service_id": a.ids.Services["cut"], "staff_id": a.ids.Staff["lena"], "start": "2026-10-14T10:07:00+02:00"})
	expectError(t, misaligned, http.StatusUnprocessableEntity, "invalid_slot")

	c := a.confirm(h.HoldToken)
	if c.ManageToken == "" || c.Booking.Status != "confirmed" || c.Booking.Customer == nil || c.Booking.Price.AmountMinor != 3800 {
		t.Fatalf("confirmation %+v", c)
	}
	path := "/v1/bookings/" + c.Booking.ID

	expectStatus(t, a.do(http.MethodGet, path, nil, "X-Manage-Token", c.ManageToken), http.StatusOK)
	expectError(t, a.do(http.MethodGet, path, nil, "X-Manage-Token", "nope"), http.StatusNotFound, "not_found")
	expectError(t, a.do(http.MethodGet, path, nil), http.StatusNotFound, "not_found")

	rec := a.do(http.MethodPost, path+"/reschedule", map[string]any{"start": "2026-10-14T10:15:00+02:00"}, "X-Manage-Token", c.ManageToken)
	expectStatus(t, rec, http.StatusOK)
	moved := decode[bookingView](t, rec)
	if moved.ID == c.Booking.ID || moved.RescheduledFrom != c.Booking.ID || moved.Start != "2026-10-14T10:15:00+02:00" {
		t.Fatalf("rescheduled %+v", moved)
	}
	old := decode[bookingView](t, a.do(http.MethodGet, path, nil, "X-Manage-Token", c.ManageToken))
	if old.Status != "cancelled" {
		t.Fatalf("old booking is %s", old.Status)
	}

	movedPath := "/v1/bookings/" + moved.ID
	rec = a.do(http.MethodPost, movedPath+"/cancel", nil, "X-Manage-Token", c.ManageToken)
	expectStatus(t, rec, http.StatusOK)
	if got := decode[bookingView](t, rec); got.Status != "cancelled" {
		t.Fatalf("cancelled booking is %s", got.Status)
	}
	expectError(t, a.do(http.MethodPost, movedPath+"/cancel", nil, "X-Manage-Token", c.ManageToken), http.StatusConflict, "not_active")
}

func TestConfirmErrorsAndIdempotency(t *testing.T) {
	a := newAPI(t, "")
	h := a.hold("cut", "lena", "2026-10-14T10:00:00+02:00")
	body := map[string]any{"hold_token": h.HoldToken, "customer": customer}

	first := a.do(http.MethodPost, "/v1/bookings", body, "Idempotency-Key", "k-1")
	expectStatus(t, first, http.StatusCreated)
	replay := a.do(http.MethodPost, "/v1/bookings", body, "Idempotency-Key", "k-1")
	expectStatus(t, replay, http.StatusCreated)
	if replay.Body.String() != first.Body.String() || replay.Header().Get("Idempotent-Replayed") != "true" {
		t.Fatalf("replay differs:\n%s\n%s", first.Body.String(), replay.Body.String())
	}
	other := map[string]any{"hold_token": h.HoldToken, "customer": map[string]any{"name": "Bob", "phone": "+7 900 123-45-67"}}
	expectError(t, a.do(http.MethodPost, "/v1/bookings", other, "Idempotency-Key", "k-1"), http.StatusUnprocessableEntity, "idempotency_key_reused")
	expectError(t, a.do(http.MethodPost, "/v1/bookings", body), http.StatusConflict, "already_confirmed")
	expectError(t, a.do(http.MethodPost, "/v1/bookings", body, "Idempotency-Key", "has spaces"), http.StatusUnprocessableEntity, "validation_failed")

	expired := a.hold("cut", "jonas", "2026-10-14T10:00:00+02:00")
	a.clock.Advance(10 * time.Minute)
	expectError(t, a.do(http.MethodPost, "/v1/bookings", map[string]any{"hold_token": expired.HoldToken, "customer": customer}), http.StatusGone, "hold_expired")
	expectError(t, a.do(http.MethodPost, "/v1/bookings", map[string]any{"hold_token": "unknown", "customer": customer}), http.StatusNotFound, "hold_not_found")

	bad := map[string]any{"hold_token": expired.HoldToken, "customer": map[string]any{"name": "Anna", "phone": "call me"}}
	expectError(t, a.do(http.MethodPost, "/v1/bookings", bad), http.StatusUnprocessableEntity, "validation_failed")
}

func TestCancelAfterTheCutoffIsTooLate(t *testing.T) {
	a := newAPI(t, "")
	c := a.confirm(a.hold("cut", "lena", "2026-10-14T10:00:00+02:00").HoldToken)
	a.clock.Set(time.Date(2026, time.October, 14, 6, 0, 0, 0, time.UTC))
	expectError(t, a.do(http.MethodPost, "/v1/bookings/"+c.Booking.ID+"/cancel", nil, "X-Manage-Token", c.ManageToken), http.StatusConflict, "too_late")
}

func TestMalformedRequests(t *testing.T) {
	a := newAPI(t, "")
	expectError(t, a.do(http.MethodPost, "/v1/holds", `{"service_id":`), http.StatusBadRequest, "bad_request")
	expectError(t, a.do(http.MethodPost, "/v1/holds", `{"service_id":1,"start":"2026-10-14T10:00:00Z","colour":"red"}`), http.StatusBadRequest, "bad_request")
	expectError(t, a.do(http.MethodPost, "/v1/holds", `{"service_id":1,"start":"tomorrow"}`), http.StatusBadRequest, "bad_request")
	expectError(t, a.do(http.MethodPost, "/v1/holds", `{"service_id":1,"start":"2026-10-14T10:00:00Z"} {}`), http.StatusBadRequest, "bad_request")
	expectError(t, a.do(http.MethodPost, "/v1/holds", map[string]any{"service_id": 999999, "start": "2026-10-14T10:00:00Z"}), http.StatusNotFound, "not_found")
	huge := `{"service_id":1,"start":"2026-10-14T10:00:00Z","x":"` + strings.Repeat("a", 70<<10) + `"}`
	expectError(t, a.do(http.MethodPost, "/v1/holds", huge), http.StatusRequestEntityTooLarge, "payload_too_large")
	expectError(t, a.do(http.MethodGet, "/v1/bookings/not-a-uuid", nil, "X-Manage-Token", "t"), http.StatusNotFound, "not_found")
}

func TestAdminRequiresTheToken(t *testing.T) {
	a := newAPI(t, "")
	berlin := strconv.FormatInt(a.ids.Locations["berlin"], 10)
	paths := []struct{ method, path string }{
		{http.MethodGet, "/v1/admin/bookings?location_id=" + berlin + "&date=2026-10-14"},
		{http.MethodGet, "/v1/admin/notifications"},
		{http.MethodPost, "/v1/admin/blocks"},
		{http.MethodDelete, "/v1/admin/blocks/7c9e6679-7425-40de-944b-e07fc1f90ae7"},
		{http.MethodPost, "/v1/admin/bookings/7c9e6679-7425-40de-944b-e07fc1f90ae7/cancel"},
	}
	for _, p := range paths {
		expectError(t, a.do(p.method, p.path, nil), http.StatusUnauthorized, "unauthorized")
		expectError(t, a.do(p.method, p.path, nil, "Authorization", "Bearer wrong"), http.StatusUnauthorized, "unauthorized")
	}
	expectStatus(t, a.admin(http.MethodGet, paths[0].path, nil), http.StatusOK)
	expectStatus(t, a.admin(http.MethodGet, paths[1].path, nil), http.StatusOK)
}

func TestAdminEndpoints(t *testing.T) {
	a := newAPI(t, "")
	berlin := strconv.FormatInt(a.ids.Locations["berlin"], 10)
	c := a.confirm(a.hold("cut", "lena", "2026-10-14T10:00:00+02:00").HoldToken)

	block := map[string]any{"staff_id": a.ids.Staff["lena"], "start": "2026-10-14T10:30:00+02:00", "end": "2026-10-14T12:00:00+02:00", "note": "training"}
	expectError(t, a.admin(http.MethodPost, "/v1/admin/blocks", block), http.StatusConflict, "slot_taken")
	block["start"] = "2026-10-14T14:00:00+02:00"
	block["end"] = "2026-10-14T15:00:00+02:00"
	rec := a.admin(http.MethodPost, "/v1/admin/blocks", block)
	expectStatus(t, rec, http.StatusCreated)
	created := decode[bookingView](t, rec)
	if created.Kind != "block" {
		t.Fatalf("block %+v", created)
	}
	block["end"] = "2026-10-14T13:00:00+02:00"
	expectError(t, a.admin(http.MethodPost, "/v1/admin/blocks", block), http.StatusUnprocessableEntity, "validation_failed")

	day := decode[struct {
		Date     string        `json:"date"`
		TimeZone string        `json:"time_zone"`
		Bookings []bookingView `json:"bookings"`
	}](t, a.admin(http.MethodGet, "/v1/admin/bookings?location_id="+berlin+"&date=2026-10-14", nil))
	if day.TimeZone != "Europe/Berlin" || len(day.Bookings) != 2 || day.Bookings[0].ID != c.Booking.ID || day.Bookings[1].ID != created.ID {
		t.Fatalf("day %+v", day)
	}
	expectError(t, a.admin(http.MethodGet, "/v1/admin/bookings?location_id="+berlin, nil), http.StatusBadRequest, "bad_request")

	expectStatus(t, a.admin(http.MethodDelete, "/v1/admin/blocks/"+created.ID, nil), http.StatusNoContent)
	expectError(t, a.admin(http.MethodDelete, "/v1/admin/blocks/"+created.ID, nil), http.StatusNotFound, "not_found")

	a.clock.Set(time.Date(2026, time.October, 14, 7, 0, 0, 0, time.UTC))
	rec = a.admin(http.MethodPost, "/v1/admin/bookings/"+c.Booking.ID+"/cancel", nil)
	expectStatus(t, rec, http.StatusOK)

	notifications := decode[struct {
		Notifications []struct {
			Kind   string `json:"kind"`
			Status string `json:"status"`
		} `json:"notifications"`
	}](t, a.admin(http.MethodGet, "/v1/admin/notifications?booking_id="+c.Booking.ID, nil))
	statuses := map[string]string{}
	for _, n := range notifications.Notifications {
		statuses[n.Kind] = n.Status
	}
	if statuses["cancellation"] != "pending" || statuses["reminder_2h"] != "voided" || len(statuses) != 4 {
		t.Fatalf("notifications %+v", statuses)
	}
	expectError(t, a.admin(http.MethodGet, "/v1/admin/notifications?status=lost", nil), http.StatusBadRequest, "bad_request")
}

func TestCORSOnTheRealRouter(t *testing.T) {
	a := newAPI(t, "")
	rec := a.do(http.MethodOptions, "/v1/holds", nil,
		"Origin", "https://widget.example.com", "Access-Control-Request-Method", "POST")
	expectStatus(t, rec, http.StatusNoContent)
	if rec.Header().Get("Access-Control-Allow-Origin") != "https://widget.example.com" {
		t.Fatalf("headers %v", rec.Header())
	}
}

func TestStaticFilesWithSPAFallback(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<!doctype html><title>widget</title>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "app.js"), []byte("console.log(1)"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := newAPI(t, dir)
	for _, path := range []string{"/", "/manage/abc"} {
		rec := a.do(http.MethodGet, path, nil)
		expectStatus(t, rec, http.StatusOK)
		if !strings.Contains(rec.Body.String(), "widget") {
			t.Fatalf("%s: %q", path, rec.Body.String())
		}
	}
	expectStatus(t, a.do(http.MethodGet, "/app.js", nil), http.StatusOK)
	expectStatus(t, a.do(http.MethodGet, "/missing.js", nil), http.StatusNotFound)
	expectError(t, a.do(http.MethodGet, "/v1/nothing-here", nil), http.StatusNotFound, "not_found")
}
