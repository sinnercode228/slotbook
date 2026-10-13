package notify

import (
	"fmt"
	"time"
)

// Details is what a message says about a booking.
type Details struct {
	Kind         Kind
	CustomerName string
	LocationName string
	ServiceName  string
	StaffName    string
	Start        time.Time
	// Zone is the location's time zone; customers read times in it.
	Zone        *time.Location
	Rescheduled bool
}

// Render produces the plain-text message for a notification.
func Render(d Details) string {
	what := fmt.Sprintf("%s with %s at %s", d.ServiceName, d.StaffName, d.LocationName)
	local := d.Start.In(d.Zone)
	when := local.Format("Mon 2 Jan 2006, 15:04 MST")
	switch d.Kind {
	case KindConfirmation:
		if d.Rescheduled {
			return fmt.Sprintf("Hi %s, your %s has been moved to %s.", d.CustomerName, what, when)
		}
		return fmt.Sprintf("Hi %s, your %s is booked for %s.", d.CustomerName, what, when)
	case KindReminder24h:
		return fmt.Sprintf("Reminder: %s on %s.", what, when)
	case KindReminder2h:
		return fmt.Sprintf("Hi %s, your %s starts at %s on %s.", d.CustomerName, what, local.Format("15:04 MST"), local.Format("Mon 2 Jan"))
	case KindCancellation:
		return fmt.Sprintf("Hi %s, your %s on %s has been cancelled.", d.CustomerName, what, when)
	default:
		return fmt.Sprintf("Update about your %s on %s.", what, when)
	}
}
