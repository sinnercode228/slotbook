package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"github.com/sinnercode228/slotbook/internal/booking"
	"github.com/sinnercode228/slotbook/internal/notify"
)

type dayBookingsResponse struct {
	LocationID int64         `json:"location_id"`
	Date       string        `json:"date"`
	TimeZone   string        `json:"time_zone"`
	Bookings   []bookingView `json:"bookings"`
}

func (s *server) adminListBookings(w http.ResponseWriter, r *http.Request) {
	locationID, err := requiredQueryID(r, "location_id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	date, ok, err := queryDate(r, "date")
	if err == nil && !ok {
		err = badRequest("date is required")
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	list, err := s.Bookings.ListForDay(r.Context(), locationID, date)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	loc, err := s.Catalog.Location(r.Context(), locationID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := make([]bookingView, len(list))
	for i, b := range list {
		out[i] = viewBooking(b, loc.Zone)
	}
	writeJSON(w, http.StatusOK, dayBookingsResponse{LocationID: loc.ID, Date: date.String(), TimeZone: loc.TimeZone, Bookings: out})
}

func (s *server) adminCancelBooking(w http.ResponseWriter, r *http.Request) {
	b, err := s.Bookings.AdminCancel(r.Context(), r.PathValue("bookingID"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, b)
}

type blockRequest struct {
	StaffID int64     `json:"staff_id"`
	Start   time.Time `json:"start"`
	End     time.Time `json:"end"`
	Note    string    `json:"note"`
}

func (s *server) adminCreateBlock(w http.ResponseWriter, r *http.Request) {
	var req blockRequest
	if err := decodeJSON(w, r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	if req.StaffID <= 0 || req.Start.IsZero() || req.End.IsZero() {
		s.fail(w, r, badRequest("staff_id, start and end are required"))
		return
	}
	b, err := s.Bookings.CreateBlock(r.Context(), booking.BlockRequest{StaffID: req.StaffID, Start: req.Start, End: req.End, Note: req.Note})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, http.StatusCreated, b)
}

func (s *server) adminDeleteBlock(w http.ResponseWriter, r *http.Request) {
	if err := s.Bookings.DeleteBlock(r.Context(), r.PathValue("blockID")); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

var notificationStatuses = map[string]bool{
	string(notify.StatusPending): true,
	string(notify.StatusSent):    true,
	string(notify.StatusFailed):  true,
	string(notify.StatusVoided):  true,
	string(notify.StatusSkipped): true,
}

func (s *server) adminListNotifications(w http.ResponseWriter, r *http.Request) {
	var (
		f   notify.Filter
		err error
	)
	q := r.URL.Query()
	if f.LocationID, err = queryID(r, "location_id"); err != nil {
		s.fail(w, r, err)
		return
	}
	if f.BookingID = q.Get("booking_id"); f.BookingID != "" && !booking.ValidID(f.BookingID) {
		s.fail(w, r, badRequest("booking_id must be a booking id"))
		return
	}
	if status := q.Get("status"); status != "" {
		if !notificationStatuses[status] {
			s.fail(w, r, badRequest("unknown status %q", status))
			return
		}
		f.Status = notify.Status(status)
	}
	if raw := q.Get("limit"); raw != "" {
		if f.Limit, err = strconv.Atoi(raw); err != nil || f.Limit < 1 || f.Limit > 500 {
			s.fail(w, r, badRequest("limit must be between 1 and 500"))
			return
		}
	}
	list, err := notify.List(r.Context(), s.DB, f)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if list == nil {
		list = []notify.Notification{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"notifications": list})
}
