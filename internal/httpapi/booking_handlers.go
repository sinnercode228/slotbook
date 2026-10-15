package httpapi

import (
	"errors"
	"net/http"

	"github.com/sinnercode228/slotbook/internal/booking"
)

const manageTokenHeader = "X-Manage-Token"

// render writes a booking in its location's time zone.
func (s *server) render(w http.ResponseWriter, r *http.Request, status int, b booking.Booking) {
	loc, err := s.Catalog.Location(r.Context(), b.LocationID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, status, viewBooking(b, loc.Zone))
}

type holdRequest struct {
	ServiceID int64     `json:"service_id"`
	StaffID   *int64    `json:"staff_id"`
	Start     timestamp `json:"start"`
}

type holdResponse struct {
	HoldToken string      `json:"hold_token"`
	ExpiresAt string      `json:"expires_at"`
	Booking   bookingView `json:"booking"`
}

func (s *server) createHold(w http.ResponseWriter, r *http.Request) {
	var req holdRequest
	if err := decodeJSON(w, r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	if req.ServiceID <= 0 || req.Start.IsZero() {
		s.fail(w, r, badRequest("service_id and start are required"))
		return
	}
	hr := booking.HoldRequest{ServiceID: req.ServiceID, Start: req.Start.Time}
	if req.StaffID != nil {
		hr.StaffID = *req.StaffID
	}
	hold, err := s.Bookings.Hold(r.Context(), hr)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	loc, err := s.Catalog.Location(r.Context(), hold.Booking.LocationID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, holdResponse{
		HoldToken: hold.Token,
		ExpiresAt: formatTime(*hold.Booking.HoldExpiresAt, loc.Zone),
		Booking:   viewBooking(hold.Booking, loc.Zone),
	})
}

type confirmationResponse struct {
	Booking     bookingView `json:"booking"`
	ManageToken string      `json:"manage_token"`
}

func (s *server) confirmHold(w http.ResponseWriter, r *http.Request) {
	var req booking.ConfirmRequest
	if err := decodeJSON(w, r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	c, replayed, err := s.Bookings.Confirm(r.Context(), req, r.Header.Get("Idempotency-Key"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	loc, err := s.Catalog.Location(r.Context(), c.Booking.LocationID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if replayed {
		w.Header().Set("Idempotent-Replayed", "true")
	}
	writeJSON(w, http.StatusCreated, confirmationResponse{Booking: viewBooking(c.Booking, loc.Zone), ManageToken: c.ManageToken})
}

type releaseRequest struct {
	HoldToken string `json:"hold_token"`
}

func (s *server) releaseHold(w http.ResponseWriter, r *http.Request) {
	var req releaseRequest
	if err := decodeJSON(w, r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	err := s.Bookings.Release(r.Context(), r.PathValue("holdID"), req.HoldToken)
	if errors.Is(err, booking.ErrHoldExpired) {
		// A hold that ran out can no more be released than a confirmed one,
		// so both are conflicts here; confirm reports the expiry as 410.
		writeError(w, http.StatusConflict, "hold_expired", err.Error())
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) getBooking(w http.ResponseWriter, r *http.Request) {
	b, err := s.Bookings.Get(r.Context(), r.PathValue("bookingID"), r.Header.Get(manageTokenHeader))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, b)
}

func (s *server) cancelBooking(w http.ResponseWriter, r *http.Request) {
	b, err := s.Bookings.Cancel(r.Context(), r.PathValue("bookingID"), r.Header.Get(manageTokenHeader))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, b)
}

type rescheduleRequest struct {
	Start timestamp `json:"start"`
}

func (s *server) rescheduleBooking(w http.ResponseWriter, r *http.Request) {
	var req rescheduleRequest
	if err := decodeJSON(w, r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	if req.Start.IsZero() {
		s.fail(w, r, badRequest("start is required"))
		return
	}
	b, err := s.Bookings.Reschedule(r.Context(), r.PathValue("bookingID"), r.Header.Get(manageTokenHeader), req.Start.Time)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, b)
}
