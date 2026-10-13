package slots

import (
	"testing"
	"time"
)

func baseRequest(t *testing.T) Request {
	t.Helper()
	var weekly WeeklyHours
	weekly[time.Monday] = []Interval{{Start: 9 * 60, End: 12 * 60}}
	return Request{
		Location:    mustZone(t, "Europe/Moscow"),
		Weekly:      weekly,
		Duration:    time.Hour,
		Step:        30 * time.Minute,
		HorizonDays: 60,
		Now:         time.Date(2026, time.October, 12, 6, 0, 0, 0, time.UTC),
		From:        Date{2026, time.October, 19},
		To:          Date{2026, time.October, 19},
	}
}

func TestGenerateRejectsInvalidRequests(t *testing.T) {
	cases := map[string]func(*Request){
		"no location":      func(r *Request) { r.Location = nil },
		"zero duration":    func(r *Request) { r.Duration = 0 },
		"negative buffer":  func(r *Request) { r.Buffer = -time.Minute },
		"zero step":        func(r *Request) { r.Step = 0 },
		"sub-minute step":  func(r *Request) { r.Step = 90 * time.Second },
		"negative notice":  func(r *Request) { r.MinNotice = -time.Minute },
		"negative horizon": func(r *Request) { r.HorizonDays = -1 },
		"reversed range":   func(r *Request) { r.To = r.From.AddDays(-1) },
		"bad interval":     func(r *Request) { r.Weekly[time.Monday] = []Interval{{Start: 600, End: 540}} },
		"bad override": func(r *Request) {
			r.Overrides = map[Date][]Interval{r.From: {{Start: 600, End: 600}}}
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			r := baseRequest(t)
			mutate(&r)
			if _, err := Generate(r); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestGenerateMergesOverlappingIntervalsWithoutDuplicates(t *testing.T) {
	r := baseRequest(t)
	r.Weekly[time.Monday] = []Interval{{Start: 10 * 60, End: 12 * 60}, {Start: 9 * 60, End: 11 * 60}}
	days, err := Generate(r)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, s := range days[0].Slots {
		got = append(got, s.Format("15:04"))
	}
	want := []string{"09:00", "09:30", "10:00", "10:30", "11:00"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestGenerateReturnsSlotsInTheLocationZone(t *testing.T) {
	r := baseRequest(t)
	days, err := Generate(r)
	if err != nil {
		t.Fatal(err)
	}
	if len(days) != 1 || len(days[0].Slots) == 0 {
		t.Fatalf("unexpected result %+v", days)
	}
	if loc := days[0].Slots[0].Location(); loc != r.Location {
		t.Fatalf("slot location %v, want %v", loc, r.Location)
	}
}

func TestEmptyDaysHaveNonNilSlots(t *testing.T) {
	r := baseRequest(t)
	r.From = Date{2026, time.October, 20}
	r.To = r.From
	days, err := Generate(r)
	if err != nil {
		t.Fatal(err)
	}
	if days[0].Slots == nil {
		t.Fatal("an empty day should carry an empty, non-nil slice")
	}
}

func TestContains(t *testing.T) {
	r := baseRequest(t)
	ok, err := Contains(r, time.Date(2026, time.October, 19, 7, 30, 0, 0, time.UTC))
	if err != nil || !ok {
		t.Fatalf("10:30 MSK should be a start: %v %v", ok, err)
	}
	ok, err = Contains(r, time.Date(2026, time.October, 19, 7, 10, 0, 0, time.UTC))
	if err != nil || ok {
		t.Fatalf("10:10 MSK is off the grid: %v %v", ok, err)
	}
	ok, err = Contains(r, time.Date(2026, time.October, 26, 7, 0, 0, 0, time.UTC))
	if err != nil || !ok {
		t.Fatalf("Contains should look at the date of the start, not From/To: %v %v", ok, err)
	}
}
