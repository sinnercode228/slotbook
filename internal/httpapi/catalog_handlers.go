package httpapi

import (
	"errors"
	"net/http"

	"github.com/sinnercode228/slotbook/internal/booking"
	"github.com/sinnercode228/slotbook/internal/catalog"
	"github.com/sinnercode228/slotbook/internal/slots"
)

// defaultSlotDays is the range returned when the client gives no "to".
const defaultSlotDays = 7

func (s *server) location(r *http.Request) (catalog.Location, error) {
	id, err := pathID(r, "locationID")
	if err != nil {
		return catalog.Location{}, err
	}
	loc, err := s.Catalog.Location(r.Context(), id)
	if errors.Is(err, catalog.ErrNotFound) {
		return catalog.Location{}, booking.ErrNotFound
	}
	return loc, err
}

func (s *server) listLocations(w http.ResponseWriter, r *http.Request) {
	locs, err := s.Catalog.Locations(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := make([]locationView, len(locs))
	for i, l := range locs {
		out[i] = viewLocation(l)
	}
	writeJSON(w, http.StatusOK, map[string]any{"locations": out})
}

func (s *server) getLocation(w http.ResponseWriter, r *http.Request) {
	loc, err := s.location(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, viewLocation(loc))
}

func (s *server) listServices(w http.ResponseWriter, r *http.Request) {
	loc, err := s.location(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	services, err := s.Catalog.Services(r.Context(), loc.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := make([]serviceView, len(services))
	for i, svc := range services {
		out[i] = viewService(svc)
	}
	writeJSON(w, http.StatusOK, map[string]any{"services": out})
}

func (s *server) listStaff(w http.ResponseWriter, r *http.Request) {
	loc, err := s.location(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	serviceID, err := queryID(r, "service_id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	staff, err := s.Catalog.Staff(r.Context(), loc.ID, serviceID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := make([]staffView, len(staff))
	for i, st := range staff {
		out[i] = viewStaff(st)
	}
	writeJSON(w, http.StatusOK, map[string]any{"staff": out})
}

type slotsResponse struct {
	LocationID int64     `json:"location_id"`
	ServiceID  int64     `json:"service_id"`
	StaffID    *int64    `json:"staff_id"`
	TimeZone   string    `json:"time_zone"`
	Days       []dayView `json:"days"`
}

func (s *server) listSlots(w http.ResponseWriter, r *http.Request) {
	loc, err := s.location(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	q := booking.AvailabilityQuery{LocationID: loc.ID}
	if q.ServiceID, err = requiredQueryID(r, "service_id"); err != nil {
		s.fail(w, r, err)
		return
	}
	if q.StaffID, err = queryID(r, "staff_id"); err != nil {
		s.fail(w, r, err)
		return
	}
	from, ok, err := queryDate(r, "from")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if !ok {
		from = slots.DateOf(s.Clock.Now().In(loc.Zone))
	}
	to, ok, err := queryDate(r, "to")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if !ok {
		to = from.AddDays(defaultSlotDays - 1)
	}
	q.From, q.To = from, to

	avail, err := s.Bookings.Availability(r.Context(), q)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	resp := slotsResponse{
		LocationID: loc.ID,
		ServiceID:  avail.Service.ID,
		TimeZone:   loc.TimeZone,
		Days:       viewDays(avail.Days, loc.Zone),
	}
	if q.StaffID != 0 {
		resp.StaffID = &q.StaffID
	}
	writeJSON(w, http.StatusOK, resp)
}
