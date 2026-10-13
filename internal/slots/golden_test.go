package slots

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

// vector mirrors the JSON format documented in testdata/slots/README.md.
type vector struct {
	Description string `json:"description"`
	Input       struct {
		TimeZone           string      `json:"time_zone"`
		Now                time.Time   `json:"now"`
		From               Date        `json:"from"`
		To                 Date        `json:"to"`
		DurationMinutes    int         `json:"duration_minutes"`
		BufferAfterMinutes int         `json:"buffer_after_minutes"`
		SlotStepMinutes    int         `json:"slot_step_minutes"`
		MinNoticeMinutes   int         `json:"min_notice_minutes"`
		HorizonDays        int         `json:"horizon_days"`
		WeeklyHours        WeeklyHours `json:"weekly_hours"`
		Overrides          []struct {
			Date      Date       `json:"date"`
			Intervals []Interval `json:"intervals"`
		} `json:"overrides"`
		Busy []Busy `json:"busy"`
	} `json:"input"`
	Expected []struct {
		Date  string   `json:"date"`
		Slots []string `json:"slots"`
	} `json:"expected"`
}

func (v vector) request(t *testing.T) Request {
	t.Helper()
	loc, err := time.LoadLocation(v.Input.TimeZone)
	if err != nil {
		t.Fatalf("load zone: %v", err)
	}
	in := v.Input
	req := Request{
		Location:    loc,
		Weekly:      in.WeeklyHours,
		Overrides:   map[Date][]Interval{},
		Duration:    time.Duration(in.DurationMinutes) * time.Minute,
		Buffer:      time.Duration(in.BufferAfterMinutes) * time.Minute,
		Step:        time.Duration(in.SlotStepMinutes) * time.Minute,
		MinNotice:   time.Duration(in.MinNoticeMinutes) * time.Minute,
		HorizonDays: in.HorizonDays,
		Busy:        in.Busy,
		Now:         in.Now,
		From:        in.From,
		To:          in.To,
	}
	for _, o := range in.Overrides {
		req.Overrides[o.Date] = o.Intervals
	}
	return req
}

func TestGoldenVectors(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("..", "..", "testdata", "slots", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) < 10 {
		t.Fatalf("expected at least 10 golden vectors, found %d", len(files))
	}
	for _, file := range files {
		t.Run(filepath.Base(file), func(t *testing.T) {
			raw, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			var v vector
			if err := json.Unmarshal(raw, &v); err != nil {
				t.Fatalf("decode: %v", err)
			}
			days, err := Generate(v.request(t))
			if err != nil {
				t.Fatalf("Generate: %v", err)
			}
			if len(days) != len(v.Expected) {
				t.Fatalf("got %d days, want %d", len(days), len(v.Expected))
			}
			for i, want := range v.Expected {
				got := days[i]
				if got.Date.String() != want.Date {
					t.Errorf("day %d: date %s, want %s", i, got.Date, want.Date)
					continue
				}
				gotSlots := make([]string, len(got.Slots))
				for j, s := range got.Slots {
					gotSlots[j] = s.Format(time.RFC3339)
				}
				if !slices.Equal(gotSlots, want.Slots) {
					t.Errorf("%s:\n got  %v\n want %v", want.Date, gotSlots, want.Slots)
				}
			}
		})
	}
}
