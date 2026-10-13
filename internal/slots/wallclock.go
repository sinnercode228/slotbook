package slots

import (
	"slices"
	"time"
)

// occurrences returns, in order, every instant at which a clock in loc reads
// the given date and time: none inside a spring-forward gap, two inside a
// repeated fall-back hour, one otherwise.
//
// time.Date does not document which instant it picks in those cases, so the
// candidates are built from the UTC offsets in effect around the wall time
// and kept only if they convert back to the same wall time.
func occurrences(d Date, at TimeOfDay, loc *time.Location) []time.Time {
	wall := time.Date(d.Year, d.Month, d.Day, 0, int(at), 0, 0, time.UTC)
	var found []time.Time
	for _, offset := range nearbyOffsets(wall, loc) {
		t := wall.Add(-offset).In(loc)
		if sameWallTime(t, wall) && !slices.ContainsFunc(found, t.Equal) {
			found = append(found, t)
		}
	}
	slices.SortFunc(found, func(a, b time.Time) int { return a.Compare(b) })
	return found
}

// nearbyOffsets returns the offsets in effect shortly before and after the
// instants that could carry this wall time. Real offsets lie within UTC-12
// and UTC+14, so probing 15 hours either side covers one transition, which
// is the most any zone has within a day.
func nearbyOffsets(wall time.Time, loc *time.Location) []time.Duration {
	var offsets []time.Duration
	for _, probe := range [...]time.Duration{-15 * time.Hour, 15 * time.Hour} {
		_, seconds := wall.Add(probe).In(loc).Zone()
		offset := time.Duration(seconds) * time.Second
		if !slices.Contains(offsets, offset) {
			offsets = append(offsets, offset)
		}
	}
	return offsets
}

func sameWallTime(t, wall time.Time) bool {
	y1, m1, d1 := t.Date()
	y2, m2, d2 := wall.Date()
	return y1 == y2 && m1 == m2 && d1 == d2 && t.Hour() == wall.Hour() && t.Minute() == wall.Minute() && t.Second() == wall.Second()
}

// edge converts a working-interval boundary to an instant: the first moment
// the local clock shows that time or later. For a repeated time this is its
// first occurrence; for a time skipped by a spring-forward jump it is the
// moment of the jump.
func edge(d Date, at TimeOfDay, loc *time.Location) time.Time {
	if found := occurrences(d, at, loc); len(found) > 0 {
		return found[0]
	}
	wall := time.Date(d.Year, d.Month, d.Day, 0, int(at), 0, 0, time.UTC)
	before := nearbyOffsets(wall, loc)[0]
	// Read with the pre-jump offset, the wall time lands just after the
	// jump, and the zone it lands in starts exactly at the jump.
	jumpedTo, _ := wall.Add(-before).In(loc).ZoneBounds()
	return jumpedTo.In(loc)
}
