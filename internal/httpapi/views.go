package httpapi

import (
	"time"

	"github.com/sinnercode228/slotbook/internal/booking"
	"github.com/sinnercode228/slotbook/internal/catalog"
	"github.com/sinnercode228/slotbook/internal/slots"
)

// Response shapes. Times are RFC 3339 strings in the location's zone so
// that clients can show them without zone arithmetic.

type priceView struct {
	AmountMinor int64  `json:"amount_minor"`
	Currency    string `json:"currency"`
}

type settingsView struct {
	SlotStepMinutes     int `json:"slot_step_minutes"`
	MinNoticeMinutes    int `json:"min_notice_minutes"`
	HorizonDays         int `json:"horizon_days"`
	HoldTTLSeconds      int `json:"hold_ttl_seconds"`
	CancelCutoffMinutes int `json:"cancel_cutoff_minutes"`
}

type locationView struct {
	ID       int64        `json:"id"`
	Slug     string       `json:"slug"`
	Name     string       `json:"name"`
	TimeZone string       `json:"time_zone"`
	Address  string       `json:"address"`
	Settings settingsView `json:"settings"`
}

func viewLocation(l catalog.Location) locationView {
	s := l.Settings
	return locationView{
		ID:       l.ID,
		Slug:     l.Slug,
		Name:     l.Name,
		TimeZone: l.TimeZone,
		Address:  l.Address,
		Settings: settingsView{
			SlotStepMinutes:     int(s.SlotStep / time.Minute),
			MinNoticeMinutes:    int(s.MinNotice / time.Minute),
			HorizonDays:         s.HorizonDays,
			HoldTTLSeconds:      int(s.HoldTTL / time.Second),
			CancelCutoffMinutes: int(s.CancelCutoff / time.Minute),
		},
	}
}

type serviceView struct {
	ID                 int64     `json:"id"`
	LocationID         int64     `json:"location_id"`
	Name               string    `json:"name"`
	DurationMinutes    int       `json:"duration_minutes"`
	BufferAfterMinutes int       `json:"buffer_after_minutes"`
	Price              priceView `json:"price"`
}

func viewService(svc catalog.Service) serviceView {
	return serviceView{
		ID:                 svc.ID,
		LocationID:         svc.LocationID,
		Name:               svc.Name,
		DurationMinutes:    int(svc.Duration / time.Minute),
		BufferAfterMinutes: int(svc.Buffer / time.Minute),
		Price:              priceView{svc.PriceMinor, svc.Currency},
	}
}

type staffView struct {
	ID         int64   `json:"id"`
	LocationID int64   `json:"location_id"`
	Name       string  `json:"name"`
	ServiceIDs []int64 `json:"service_ids"`
}

func viewStaff(st catalog.Staff) staffView {
	return staffView{ID: st.ID, LocationID: st.LocationID, Name: st.Name, ServiceIDs: st.ServiceIDs}
}

type dayView struct {
	Date  string   `json:"date"`
	Slots []string `json:"slots"`
}

func viewDays(days []slots.Day, zone *time.Location) []dayView {
	out := make([]dayView, len(days))
	for i, d := range days {
		starts := make([]string, len(d.Slots))
		for j, s := range d.Slots {
			starts[j] = formatTime(s, zone)
		}
		out[i] = dayView{Date: d.Date.String(), Slots: starts}
	}
	return out
}

type customerView struct {
	Name           string `json:"name"`
	Phone          string `json:"phone"`
	Email          string `json:"email,omitempty"`
	TelegramChatID int64  `json:"telegram_chat_id,omitempty"`
}

type bookingView struct {
	ID              string         `json:"id"`
	Kind            booking.Kind   `json:"kind"`
	Status          booking.Status `json:"status"`
	LocationID      int64          `json:"location_id"`
	StaffID         int64          `json:"staff_id"`
	ServiceID       *int64         `json:"service_id"`
	Start           string         `json:"start"`
	End             string         `json:"end"`
	BusyUntil       string         `json:"busy_until"`
	TimeZone        string         `json:"time_zone"`
	Price           *priceView     `json:"price"`
	HoldExpiresAt   *string        `json:"hold_expires_at"`
	Customer        *customerView  `json:"customer"`
	Note            string         `json:"note"`
	RescheduledFrom *string        `json:"rescheduled_from"`
	CreatedAt       string         `json:"created_at"`
	ConfirmedAt     *string        `json:"confirmed_at"`
	CancelledAt     *string        `json:"cancelled_at"`
}

func viewBooking(b booking.Booking, zone *time.Location) bookingView {
	v := bookingView{
		ID:              b.ID,
		Kind:            b.Kind,
		Status:          b.Status,
		LocationID:      b.LocationID,
		StaffID:         b.StaffID,
		Start:           formatTime(b.Start, zone),
		End:             formatTime(b.End, zone),
		BusyUntil:       formatTime(b.BusyUntil, zone),
		TimeZone:        zone.String(),
		HoldExpiresAt:   formatOptional(b.HoldExpiresAt, zone),
		Note:            b.Note,
		CreatedAt:       formatTime(b.CreatedAt, zone),
		ConfirmedAt:     formatOptional(b.ConfirmedAt, zone),
		CancelledAt:     formatOptional(b.CancelledAt, zone),
		RescheduledFrom: optionalString(b.RescheduledFrom),
	}
	if b.ServiceID != 0 {
		v.ServiceID = &b.ServiceID
		v.Price = &priceView{b.PriceMinor, b.Currency}
	}
	if c := b.Customer; c != nil {
		v.Customer = &customerView{Name: c.Name, Phone: c.Phone, Email: c.Email, TelegramChatID: c.TelegramChatID}
	}
	return v
}

func formatTime(t time.Time, zone *time.Location) string {
	return t.In(zone).Format(time.RFC3339)
}

func formatOptional(t *time.Time, zone *time.Location) *string {
	if t == nil {
		return nil
	}
	s := formatTime(*t, zone)
	return &s
}

func optionalString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
