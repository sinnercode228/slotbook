package slots

import (
	"testing"
	"time"
)

func TestOccurrences(t *testing.T) {
	cases := []struct {
		name string
		zone string
		date Date
		at   string
		want []string
	}{
		{"ordinary time", "Europe/Berlin", Date{2026, time.October, 24}, "09:00", []string{"2026-10-24T07:00:00Z"}},
		{"berlin gap", "Europe/Berlin", Date{2026, time.March, 29}, "02:30", nil},
		{"berlin repeated hour", "Europe/Berlin", Date{2026, time.October, 25}, "02:30", []string{"2026-10-25T00:30:00Z", "2026-10-25T01:30:00Z"}},
		{"berlin hour after the repeat", "Europe/Berlin", Date{2026, time.October, 25}, "03:00", []string{"2026-10-25T02:00:00Z"}},
		{"new york gap", "America/New_York", Date{2026, time.March, 8}, "02:00", nil},
		{"new york repeated hour", "America/New_York", Date{2026, time.November, 1}, "01:00", []string{"2026-11-01T05:00:00Z", "2026-11-01T06:00:00Z"}},
		{"moscow", "Europe/Moscow", Date{2026, time.March, 29}, "02:30", []string{"2026-03-28T23:30:00Z"}},
		{"end of day", "Europe/Moscow", Date{2026, time.October, 21}, "24:00", []string{"2026-10-21T21:00:00Z"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			at, err := ParseTimeOfDay(c.at)
			if err != nil {
				t.Fatal(err)
			}
			got := occurrences(c.date, at, mustZone(t, c.zone))
			if len(got) != len(c.want) {
				t.Fatalf("got %v, want %v", got, c.want)
			}
			for i := range got {
				if got[i].UTC().Format(time.RFC3339) != c.want[i] {
					t.Fatalf("got %v, want %v", got, c.want)
				}
			}
		})
	}
}

func TestEdgeInsideGapIsTheMomentClocksJump(t *testing.T) {
	cases := []struct {
		zone string
		date Date
		at   TimeOfDay
		want string
	}{
		{"Europe/Berlin", Date{2026, time.March, 29}, 2*60 + 30, "2026-03-29T01:00:00Z"},
		{"Europe/Berlin", Date{2026, time.March, 29}, 2 * 60, "2026-03-29T01:00:00Z"},
		{"America/New_York", Date{2026, time.March, 8}, 2*60 + 15, "2026-03-08T07:00:00Z"},
	}
	for _, c := range cases {
		got := edge(c.date, c.at, mustZone(t, c.zone))
		if got.UTC().Format(time.RFC3339) != c.want {
			t.Errorf("%s %s %s: got %s, want %s", c.zone, c.date, c.at, got.UTC().Format(time.RFC3339), c.want)
		}
	}
}

func TestEdgeOnRepeatedHourIsTheFirstOccurrence(t *testing.T) {
	got := edge(Date{2026, time.October, 25}, 2*60+30, mustZone(t, "Europe/Berlin"))
	if got.UTC().Format(time.RFC3339) != "2026-10-25T00:30:00Z" {
		t.Fatalf("got %s", got.UTC())
	}
}
