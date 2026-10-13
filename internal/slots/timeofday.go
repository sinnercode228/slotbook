package slots

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// TimeOfDay is a local wall-clock time in minutes since midnight.
// 24:00 (1440) is valid as the end of an interval.
type TimeOfDay int

const endOfDay TimeOfDay = 24 * 60

// ParseTimeOfDay parses "HH:MM" with 00:00 <= value <= 24:00.
func ParseTimeOfDay(s string) (TimeOfDay, error) {
	if len(s) != 5 || s[2] != ':' || !isDigits(s[:2]) || !isDigits(s[3:]) {
		return 0, fmt.Errorf("invalid time %q, want HH:MM", s)
	}
	h := int(s[0]-'0')*10 + int(s[1]-'0')
	m := int(s[3]-'0')*10 + int(s[4]-'0')
	t := TimeOfDay(h*60 + m)
	if m > 59 || t > endOfDay {
		return 0, fmt.Errorf("invalid time %q, want 00:00 to 24:00", s)
	}
	return t, nil
}

func isDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func (t TimeOfDay) String() string {
	return fmt.Sprintf("%02d:%02d", int(t)/60, int(t)%60)
}

func (t TimeOfDay) MarshalText() ([]byte, error) {
	return []byte(t.String()), nil
}

func (t *TimeOfDay) UnmarshalText(b []byte) error {
	parsed, err := ParseTimeOfDay(string(b))
	if err != nil {
		return err
	}
	*t = parsed
	return nil
}

// Interval is a half-open working interval [Start, End) in local wall time.
type Interval struct {
	Start TimeOfDay `json:"start"`
	End   TimeOfDay `json:"end"`
}

func (i Interval) Validate() error {
	if i.Start < 0 || i.End > endOfDay || i.Start >= i.End {
		return fmt.Errorf("invalid interval %s-%s", i.Start, i.End)
	}
	return nil
}

// WeeklyHours holds working intervals per weekday, indexed by time.Weekday.
// In JSON it is an object keyed by lowercase English day names; missing days
// have no hours.
type WeeklyHours [7][]Interval

func (w WeeklyHours) Validate() error {
	for day, intervals := range w {
		for _, iv := range intervals {
			if err := iv.Validate(); err != nil {
				return fmt.Errorf("%s: %w", dayName(time.Weekday(day)), err)
			}
		}
	}
	return nil
}

// jsonDayOrder lists days Monday first, the way people read a week.
var jsonDayOrder = [7]time.Weekday{
	time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday, time.Saturday, time.Sunday,
}

func dayName(d time.Weekday) string { return strings.ToLower(d.String()) }

func (w WeeklyHours) MarshalJSON() ([]byte, error) {
	var b strings.Builder
	b.WriteByte('{')
	first := true
	for _, day := range jsonDayOrder {
		if len(w[day]) == 0 {
			continue
		}
		if !first {
			b.WriteByte(',')
		}
		first = false
		intervals, err := json.Marshal(w[day])
		if err != nil {
			return nil, err
		}
		fmt.Fprintf(&b, "%q:%s", dayName(day), intervals)
	}
	b.WriteByte('}')
	return []byte(b.String()), nil
}

func (w *WeeklyHours) UnmarshalJSON(b []byte) error {
	var raw map[string][]Interval
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	var out WeeklyHours
	for name, intervals := range raw {
		day, ok := weekdayByName(name)
		if !ok {
			return fmt.Errorf("unknown weekday %q", name)
		}
		out[day] = intervals
	}
	if err := out.Validate(); err != nil {
		return err
	}
	*w = out
	return nil
}

func weekdayByName(name string) (time.Weekday, bool) {
	for _, day := range jsonDayOrder {
		if dayName(day) == name {
			return day, true
		}
	}
	return 0, false
}
