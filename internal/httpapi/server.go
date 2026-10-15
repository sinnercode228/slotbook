// Package httpapi exposes the booking API over HTTP: public endpoints for
// the widget and the manage link, and token-protected admin endpoints.
package httpapi

import (
	"log/slog"
	"net/http"
	"net/netip"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/sinnercode228/slotbook/internal/booking"
	"github.com/sinnercode228/slotbook/internal/catalog"
	"github.com/sinnercode228/slotbook/internal/clock"
	"github.com/sinnercode228/slotbook/internal/ratelimit"
)

type Deps struct {
	DB          *pgxpool.Pool
	Catalog     *catalog.Store
	Bookings    *booking.Manager
	Clock       clock.Clock
	Log         *slog.Logger
	AdminToken  string
	CORSOrigins []string
	// StaticDir, if set, is served at / with index.html as the fallback for
	// client-side routes.
	StaticDir string
	// HoldLimit applies to POST /v1/holds per client address; the zero Limit
	// turns it off. The client address is the TCP peer, or, behind one of
	// TrustedProxies, read from X-Forwarded-For (see clientAddr).
	HoldLimit      ratelimit.Limit
	TrustedProxies []netip.Prefix
}

type server struct {
	Deps
	mux *http.ServeMux
}

// New returns the HTTP handler with all routes and middleware.
func New(d Deps) http.Handler {
	if d.Clock == nil {
		d.Clock = clock.System{}
	}
	s := &server{Deps: d, mux: http.NewServeMux()}
	s.routes()

	var h http.Handler = s.mux
	h = cors(d.CORSOrigins)(h)
	h = recoverPanics(d.Log)(h)
	h = logRequests(d.Log)(h)
	return withRequestID(h)
}

func (s *server) routes() {
	m := s.mux
	m.HandleFunc("GET /healthz", s.healthz)
	m.HandleFunc("GET /readyz", s.readyz)

	m.HandleFunc("GET /v1/locations", s.listLocations)
	m.HandleFunc("GET /v1/locations/{locationID}", s.getLocation)
	m.HandleFunc("GET /v1/locations/{locationID}/services", s.listServices)
	m.HandleFunc("GET /v1/locations/{locationID}/staff", s.listStaff)
	m.HandleFunc("GET /v1/locations/{locationID}/slots", s.listSlots)

	limitHolds := func(h http.Handler) http.Handler { return h }
	if s.HoldLimit.Enabled() {
		limitHolds = limitByClient(ratelimit.New(s.HoldLimit, s.Clock), s.TrustedProxies)
	}
	m.Handle("POST /v1/holds", limitHolds(http.HandlerFunc(s.createHold)))
	m.HandleFunc("DELETE /v1/holds/{holdID}", s.releaseHold)
	m.HandleFunc("POST /v1/bookings", s.confirmHold)
	m.HandleFunc("GET /v1/bookings/{bookingID}", s.getBooking)
	m.HandleFunc("POST /v1/bookings/{bookingID}/cancel", s.cancelBooking)
	m.HandleFunc("POST /v1/bookings/{bookingID}/reschedule", s.rescheduleBooking)

	admin := requireAdmin(s.AdminToken)
	m.Handle("GET /v1/admin/bookings", admin(http.HandlerFunc(s.adminListBookings)))
	m.Handle("POST /v1/admin/bookings/{bookingID}/cancel", admin(http.HandlerFunc(s.adminCancelBooking)))
	m.Handle("POST /v1/admin/blocks", admin(http.HandlerFunc(s.adminCreateBlock)))
	m.Handle("DELETE /v1/admin/blocks/{blockID}", admin(http.HandlerFunc(s.adminDeleteBlock)))
	m.Handle("GET /v1/admin/notifications", admin(http.HandlerFunc(s.adminListNotifications)))

	m.HandleFunc("/v1/", s.noRoute)
	if s.StaticDir != "" {
		m.Handle("/", staticFiles(s.StaticDir))
	} else {
		m.HandleFunc("/", s.noRoute)
	}
}

// noRoute answers in JSON where ServeMux would answer in plain text: 405
// with Allow when the path exists for other methods, 404 otherwise.
func (s *server) noRoute(w http.ResponseWriter, r *http.Request) {
	var allow []string
	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodDelete} {
		probe := r.Clone(r.Context())
		probe.Method = method
		if _, pattern := s.mux.Handler(probe); pattern != "" && pattern != "/" && pattern != "/v1/" {
			allow = append(allow, method)
		}
	}
	if len(allow) > 0 {
		w.Header().Set("Allow", strings.Join(allow, ", "))
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", r.Method+" is not supported here")
		return
	}
	writeError(w, http.StatusNotFound, "not_found", "no such endpoint")
}

func (s *server) logger(r *http.Request) *slog.Logger {
	return s.Log.With("request_id", requestIDFrom(r.Context()))
}

func (s *server) healthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *server) readyz(w http.ResponseWriter, r *http.Request) {
	if err := s.DB.Ping(r.Context()); err != nil {
		s.logger(r).WarnContext(r.Context(), "readiness check failed", "err", err)
		writeError(w, http.StatusServiceUnavailable, "not_ready", "database is not reachable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}
