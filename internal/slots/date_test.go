package slots

import (
	"encoding/json"
	"testing"
	"time"
)

func TestParseDate(t *testing.T) {
	d, err := ParseDate("2026-03-29")
	if err != nil {
		t.Fatal(err)
	}
	if d != (Date{2026, time.March, 29}) {
		t.Fatalf("got %+v", d)
	}
	for _, bad := range []string{"", "2026-3-29", "2026-02-30", "29.03.2026", "2026-03-29T00:00:00Z"} {
		if _, err := ParseDate(bad); err == nil {
			t.Errorf("ParseDate(%q) succeeded", bad)
		}
	}
}

func TestDateAddDaysCrossesMonthsAndYears(t *testing.T) {
	cases := []struct {
		from Date
		n    int
		want string
	}{
		{Date{2026, time.March, 28}, 1, "2026-03-29"},
		{Date{2026, time.March, 31}, 1, "2026-04-01"},
		{Date{2026, time.December, 31}, 1, "2027-01-01"},
		{Date{2028, time.February, 28}, 1, "2028-02-29"},
		{Date{2026, time.January, 1}, -1, "2025-12-31"},
		{Date{2026, time.October, 12}, 60, "2026-12-11"},
	}
	for _, c := range cases {
		if got := c.from.AddDays(c.n).String(); got != c.want {
			t.Errorf("%s + %d = %s, want %s", c.from, c.n, got, c.want)
		}
	}
}

func TestDateOfUsesTheLocationCalendar(t *testing.T) {
	moscow := mustZone(t, "Europe/Moscow")
	utc := time.Date(2026, time.October, 19, 22, 30, 0, 0, time.UTC)
	if got := DateOf(utc.In(moscow)).String(); got != "2026-10-20" {
		t.Fatalf("got %s", got)
	}
	if got := DateOf(utc).String(); got != "2026-10-19" {
		t.Fatalf("got %s", got)
	}
}

func TestDateWeekdayAndOrdering(t *testing.T) {
	d := Date{2026, time.October, 25}
	if d.Weekday() != time.Sunday {
		t.Fatalf("weekday %v", d.Weekday())
	}
	next := d.AddDays(1)
	if !d.Before(next) || !next.After(d) || d.After(next) || d.Before(d) {
		t.Fatal("ordering is wrong")
	}
	if d.DaysUntil(Date{2026, time.November, 1}) != 7 || d.DaysUntil(d) != 0 {
		t.Fatal("DaysUntil is wrong")
	}
}

func TestDateJSON(t *testing.T) {
	var v struct {
		D Date `json:"d"`
	}
	if err := json.Unmarshal([]byte(`{"d":"2026-10-25"}`), &v); err != nil {
		t.Fatal(err)
	}
	out, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != `{"d":"2026-10-25"}` {
		t.Fatalf("got %s", out)
	}
	if err := json.Unmarshal([]byte(`{"d":"25.10.2026"}`), &v); err == nil {
		t.Fatal("expected an error for a malformed date")
	}
}

func mustZone(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatal(err)
	}
	return loc
}
