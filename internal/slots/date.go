package slots

import (
	"fmt"
	"time"
)

// Date is a calendar date with no time zone attached. Slot generation walks
// dates rather than adding 24 hours, so DST days keep their 23 or 25 hours.
type Date struct {
	Year  int
	Month time.Month
	Day   int
}

const dateLayout = "2006-01-02"

// ParseDate parses a date in YYYY-MM-DD form.
func ParseDate(s string) (Date, error) {
	t, err := time.Parse(dateLayout, s)
	if err != nil {
		return Date{}, fmt.Errorf("invalid date %q, want YYYY-MM-DD", s)
	}
	return DateOf(t), nil
}

// DateOf returns the calendar date of t in t's own location.
func DateOf(t time.Time) Date {
	y, m, d := t.Date()
	return Date{y, m, d}
}

func (d Date) String() string {
	return fmt.Sprintf("%04d-%02d-%02d", d.Year, d.Month, d.Day)
}

// midnightUTC is used only for calendar arithmetic: UTC has no DST, so
// AddDate on it moves exactly one calendar day per day.
func (d Date) midnightUTC() time.Time {
	return time.Date(d.Year, d.Month, d.Day, 0, 0, 0, 0, time.UTC)
}

func (d Date) AddDays(n int) Date {
	return DateOf(d.midnightUTC().AddDate(0, 0, n))
}

func (d Date) Weekday() time.Weekday {
	return d.midnightUTC().Weekday()
}

func (d Date) Before(o Date) bool { return d.midnightUTC().Before(o.midnightUTC()) }
func (d Date) After(o Date) bool  { return d.midnightUTC().After(o.midnightUTC()) }

// DaysUntil returns the number of calendar days from d to o (negative if o is earlier).
func (d Date) DaysUntil(o Date) int {
	return int(o.midnightUTC().Sub(d.midnightUTC()) / (24 * time.Hour))
}

func (d Date) MarshalText() ([]byte, error) {
	return []byte(d.String()), nil
}

func (d *Date) UnmarshalText(b []byte) error {
	parsed, err := ParseDate(string(b))
	if err != nil {
		return err
	}
	*d = parsed
	return nil
}
