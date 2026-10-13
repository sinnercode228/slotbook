package slots

import (
	"encoding/json"
	"testing"
	"time"
)

func TestParseTimeOfDay(t *testing.T) {
	cases := map[string]TimeOfDay{"00:00": 0, "09:05": 545, "23:59": 1439, "24:00": 1440}
	for in, want := range cases {
		got, err := ParseTimeOfDay(in)
		if err != nil || got != want {
			t.Errorf("ParseTimeOfDay(%q) = %d, %v; want %d", in, got, err, want)
		}
		if got.String() != in {
			t.Errorf("String() = %q, want %q", got.String(), in)
		}
	}
	for _, bad := range []string{"", "9:00", "24:01", "25:00", "12:60", "12-30", "aa:bb"} {
		if _, err := ParseTimeOfDay(bad); err == nil {
			t.Errorf("ParseTimeOfDay(%q) succeeded", bad)
		}
	}
}

func TestIntervalValidation(t *testing.T) {
	if err := (Interval{Start: 600, End: 600}).Validate(); err == nil {
		t.Error("empty interval accepted")
	}
	if err := (Interval{Start: 700, End: 600}).Validate(); err == nil {
		t.Error("reversed interval accepted")
	}
	if err := (Interval{Start: 0, End: 1440}).Validate(); err != nil {
		t.Errorf("whole day rejected: %v", err)
	}
}

func TestWeeklyHoursJSONRoundTrip(t *testing.T) {
	in := `{"monday":[{"start":"09:00","end":"13:00"},{"start":"14:00","end":"18:00"}],"sunday":[{"start":"10:00","end":"24:00"}]}`
	var w WeeklyHours
	if err := json.Unmarshal([]byte(in), &w); err != nil {
		t.Fatal(err)
	}
	if len(w[time.Monday]) != 2 || w[time.Monday][1] != (Interval{Start: 840, End: 1080}) {
		t.Fatalf("monday = %+v", w[time.Monday])
	}
	if len(w[time.Tuesday]) != 0 || w[time.Sunday][0].End != 1440 {
		t.Fatalf("unexpected days: %+v", w)
	}
	out, err := json.Marshal(w)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != in {
		t.Fatalf("round trip:\n got  %s\n want %s", out, in)
	}
}

func TestWeeklyHoursRejectsBadInput(t *testing.T) {
	for _, in := range []string{
		`{"funday":[{"start":"09:00","end":"10:00"}]}`,
		`{"monday":[{"start":"10:00","end":"09:00"}]}`,
		`{"monday":[{"start":"9","end":"10:00"}]}`,
	} {
		var w WeeklyHours
		if err := json.Unmarshal([]byte(in), &w); err == nil {
			t.Errorf("accepted %s", in)
		}
	}
}
