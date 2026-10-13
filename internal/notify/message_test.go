package notify

import (
	"strings"
	"testing"
	"time"
)

func details(t *testing.T, zone string, kind Kind) Details {
	t.Helper()
	loc, err := time.LoadLocation(zone)
	if err != nil {
		t.Fatal(err)
	}
	return Details{
		Kind:         kind,
		CustomerName: "Anna",
		LocationName: "Snipwell Berlin",
		ServiceName:  "Haircut",
		StaffName:    "Lena H.",
		Start:        time.Date(2026, time.October, 14, 8, 0, 0, 0, time.UTC),
		Zone:         loc,
	}
}

func TestRenderUsesTheLocationTimeZone(t *testing.T) {
	berlin := Render(details(t, "Europe/Berlin", KindConfirmation))
	if !strings.Contains(berlin, "Wed 14 Oct 2026, 10:00 CEST") {
		t.Fatalf("Berlin message: %q", berlin)
	}
	moscow := Render(details(t, "Europe/Moscow", KindReminder24h))
	if !strings.Contains(moscow, "11:00 MSK") {
		t.Fatalf("Moscow message: %q", moscow)
	}
	winter := details(t, "Europe/Berlin", KindReminder2h)
	winter.Start = time.Date(2026, time.November, 4, 8, 0, 0, 0, time.UTC)
	if msg := Render(winter); !strings.Contains(msg, "09:00 CET") {
		t.Fatalf("winter message: %q", msg)
	}
}

func TestRenderEachKind(t *testing.T) {
	want := map[Kind]string{
		KindConfirmation: "is booked for",
		KindReminder24h:  "Reminder",
		KindReminder2h:   "starts at",
		KindCancellation: "has been cancelled",
	}
	for kind, fragment := range want {
		msg := Render(details(t, "Europe/Berlin", kind))
		if !strings.Contains(msg, fragment) || !strings.Contains(msg, "Haircut with Lena H.") {
			t.Errorf("%s: %q", kind, msg)
		}
	}
	moved := details(t, "Europe/Berlin", KindConfirmation)
	moved.Rescheduled = true
	if msg := Render(moved); !strings.Contains(msg, "has been moved to") {
		t.Errorf("rescheduled confirmation: %q", msg)
	}
}
