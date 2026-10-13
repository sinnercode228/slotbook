// Package slots computes bookable start times. It is pure: everything it
// needs, including the current time, comes in through Request.
package slots

import (
	"errors"
	"fmt"
	"slices"
	"time"
)

// Busy is an absolute half-open interval [Start, End) that a new booking must
// not overlap, such as an existing booking including its buffer.
type Busy struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// Request describes one staff member's availability for one service.
type Request struct {
	Location *time.Location
	Weekly   WeeklyHours
	// Overrides replace the weekly hours on their date; a date mapped to no
	// intervals is closed.
	Overrides map[Date][]Interval
	Duration  time.Duration
	// Buffer is cleanup time after the service; it must fit in working hours
	// and stay clear of busy intervals just like the service itself.
	Buffer    time.Duration
	Step      time.Duration
	MinNotice time.Duration
	// HorizonDays is how many days past today (in the location zone) can be
	// booked: today plus HorizonDays is the last bookable date.
	HorizonDays int
	Busy        []Busy
	Now         time.Time
	From, To    Date
}

// Day lists the available starts on one local date, in order. Slots is never
// nil so that an empty day encodes as [] rather than null.
type Day struct {
	Date  Date
	Slots []time.Time
}

func (r Request) validate() error {
	switch {
	case r.Location == nil:
		return errors.New("slots: location is required")
	case r.Duration <= 0:
		return errors.New("slots: duration must be positive")
	case r.Buffer < 0:
		return errors.New("slots: buffer must not be negative")
	case r.Step < time.Minute || r.Step%time.Minute != 0:
		return errors.New("slots: step must be a positive whole number of minutes")
	case r.MinNotice < 0:
		return errors.New("slots: minimum notice must not be negative")
	case r.HorizonDays < 0:
		return errors.New("slots: horizon must not be negative")
	case r.To.Before(r.From):
		return errors.New("slots: range ends before it starts")
	}
	if err := r.Weekly.Validate(); err != nil {
		return fmt.Errorf("slots: weekly hours: %w", err)
	}
	for date, intervals := range r.Overrides {
		for _, iv := range intervals {
			if err := iv.Validate(); err != nil {
				return fmt.Errorf("slots: override on %s: %w", date, err)
			}
		}
	}
	return nil
}

// Generate returns one Day per date from r.From to r.To inclusive.
//
// A start is offered when it lies on the step grid counted from local
// midnight, its wall-clock time exists on that date (times skipped by a
// spring-forward jump are not offered; a repeated time means its first
// occurrence), [start, start+Duration+Buffer) fits inside a single working
// interval measured in real elapsed time, it overlaps no busy interval, it is
// at least MinNotice after Now, and its date is within the horizon.
func Generate(r Request) ([]Day, error) {
	if err := r.validate(); err != nil {
		return nil, err
	}
	today := DateOf(r.Now.In(r.Location))
	lastBookable := today.AddDays(r.HorizonDays)
	days := make([]Day, 0, r.From.DaysUntil(r.To)+1)
	for d := r.From; !d.After(r.To); d = d.AddDays(1) {
		day := Day{Date: d, Slots: []time.Time{}}
		if !d.Before(today) && !d.After(lastBookable) {
			day.Slots = r.startsOn(d)
		}
		days = append(days, day)
	}
	return days, nil
}

// Contains reports whether start is one of the starts Generate offers on the
// local date of start, regardless of r.From and r.To.
func Contains(r Request, start time.Time) (bool, error) {
	d := DateOf(start.In(r.Location))
	r.From, r.To = d, d
	days, err := Generate(r)
	if err != nil {
		return false, err
	}
	return slices.ContainsFunc(days[0].Slots, start.Equal), nil
}

func (r Request) hoursOn(d Date) []Interval {
	if intervals, ok := r.Overrides[d]; ok {
		return intervals
	}
	return r.Weekly[d.Weekday()]
}

func (r Request) startsOn(d Date) []time.Time {
	step := TimeOfDay(r.Step / time.Minute)
	span := r.Duration + r.Buffer
	earliest := r.Now.Add(r.MinNotice)
	starts := []time.Time{}
	for _, iv := range r.hoursOn(d) {
		closes := edge(d, iv.End, r.Location)
		for at := alignUp(iv.Start, step); at < iv.End; at += step {
			found := occurrences(d, at, r.Location)
			if len(found) == 0 {
				continue
			}
			start := found[0]
			end := start.Add(span)
			if start.Before(earliest) || end.After(closes) || r.overlapsBusy(start, end) {
				continue
			}
			starts = append(starts, start)
		}
	}
	slices.SortFunc(starts, func(a, b time.Time) int { return a.Compare(b) })
	return slices.CompactFunc(starts, time.Time.Equal)
}

func alignUp(t, step TimeOfDay) TimeOfDay {
	if rem := t % step; rem != 0 {
		return t + step - rem
	}
	return t
}

func (r Request) overlapsBusy(start, end time.Time) bool {
	for _, b := range r.Busy {
		if start.Before(b.End) && b.Start.Before(end) {
			return true
		}
	}
	return false
}
