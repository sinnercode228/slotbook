package catalog_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sinnercode228/slotbook/internal/catalog"
	"github.com/sinnercode228/slotbook/internal/config"
	"github.com/sinnercode228/slotbook/internal/dbtest"
	"github.com/sinnercode228/slotbook/internal/seed"
	"github.com/sinnercode228/slotbook/internal/slots"
)

func intp(v int) *int { return &v }

func fixture(t *testing.T) (*catalog.Store, seed.Result) {
	t.Helper()
	pool := dbtest.New(t)
	var weekly slots.WeeklyHours
	weekly[time.Monday] = []slots.Interval{{Start: 9 * 60, End: 13 * 60}, {Start: 14 * 60, End: 18 * 60}}
	chain := seed.Chain{Locations: []seed.Location{{
		Slug: "berlin", Name: "Berlin", TimeZone: "Europe/Berlin",
		Settings: seed.Settings{MinNoticeMinutes: intp(30), CancelCutoffMinutes: intp(1440)},
		Services: []seed.Service{
			{Key: "cut", Name: "Haircut", DurationMinutes: 45, BufferAfterMinutes: 15, PriceMinor: 3800, Currency: "EUR"},
			{Key: "shave", Name: "Shave", DurationMinutes: 30, PriceMinor: 2500, Currency: "EUR"},
		},
		Staff: []seed.Staff{
			{Key: "lena", Name: "Lena H.", Services: []string{"cut"}, WeeklyHours: weekly, Overrides: []seed.Override{
				{Date: slots.Date{Year: 2026, Month: time.November, Day: 16}},
				{Date: slots.Date{Year: 2026, Month: time.November, Day: 21}, Intervals: []slots.Interval{{Start: 600, End: 720}}},
				{Date: slots.Date{Year: 2026, Month: time.December, Day: 24}},
			}},
			{Key: "jonas", Name: "Jonas W.", Services: []string{"cut", "shave"}},
		},
	}}}
	res, err := seed.Apply(context.Background(), pool, chain)
	if err != nil {
		t.Fatal(err)
	}
	return catalog.NewStore(pool, config.LocationDefaults), res
}

func TestLocationMergesSettingsWithDefaults(t *testing.T) {
	store, ids := fixture(t)
	loc, err := store.Location(context.Background(), ids.Locations["berlin"])
	if err != nil {
		t.Fatal(err)
	}
	if loc.Zone.String() != "Europe/Berlin" {
		t.Fatalf("zone %v", loc.Zone)
	}
	want := config.LocationDefaults
	want.MinNotice = 30 * time.Minute
	want.CancelCutoff = 24 * time.Hour
	if loc.Settings != want {
		t.Fatalf("settings %+v, want %+v", loc.Settings, want)
	}
	all, err := store.Locations(context.Background())
	if err != nil || len(all) != 1 || all[0].Settings != want {
		t.Fatalf("Locations = %+v, %v", all, err)
	}
}

func TestServiceLookup(t *testing.T) {
	store, ids := fixture(t)
	ctx := context.Background()
	svc, err := store.Service(ctx, ids.Services["cut"])
	if err != nil {
		t.Fatal(err)
	}
	if svc.Duration != 45*time.Minute || svc.Buffer != 15*time.Minute || svc.PriceMinor != 3800 || svc.Currency != "EUR" {
		t.Fatalf("service %+v", svc)
	}
	if _, err := store.Service(ctx, 999999); !errors.Is(err, catalog.ErrNotFound) {
		t.Fatalf("missing service: %v", err)
	}
	list, err := store.Services(ctx, ids.Locations["berlin"])
	if err != nil || len(list) != 2 {
		t.Fatalf("Services = %+v, %v", list, err)
	}
}

func TestStaffFilteredByService(t *testing.T) {
	store, ids := fixture(t)
	ctx := context.Background()
	all, err := store.Staff(ctx, ids.Locations["berlin"], 0)
	if err != nil || len(all) != 2 {
		t.Fatalf("all staff = %+v, %v", all, err)
	}
	shave, err := store.Staff(ctx, ids.Locations["berlin"], ids.Services["shave"])
	if err != nil || len(shave) != 1 || shave[0].ID != ids.Staff["jonas"] {
		t.Fatalf("shave staff = %+v, %v", shave, err)
	}
	if got := shave[0].ServiceIDs; len(got) != 2 {
		t.Fatalf("jonas service ids %v", got)
	}
	member, err := store.StaffMember(ctx, ids.Staff["lena"])
	if err != nil || member.Name != "Lena H." || !member.Offers(ids.Services["cut"]) || member.Offers(ids.Services["shave"]) {
		t.Fatalf("StaffMember = %+v, %v", member, err)
	}
	if _, err := store.StaffMember(ctx, 999999); !errors.Is(err, catalog.ErrNotFound) {
		t.Fatalf("missing staff: %v", err)
	}
}

func TestScheduleReturnsWeeklyHoursAndOverridesInRange(t *testing.T) {
	store, ids := fixture(t)
	from := slots.Date{Year: 2026, Month: time.November, Day: 16}
	sched, err := store.Schedule(context.Background(), ids.Staff["lena"], from, from.AddDays(6))
	if err != nil {
		t.Fatal(err)
	}
	if len(sched.Weekly[time.Monday]) != 2 || sched.Weekly[time.Monday][1] != (slots.Interval{Start: 840, End: 1080}) {
		t.Fatalf("weekly %+v", sched.Weekly)
	}
	closed, ok := sched.Overrides[from]
	if !ok || len(closed) != 0 {
		t.Fatalf("closed override missing: %+v", sched.Overrides)
	}
	custom := sched.Overrides[from.AddDays(5)]
	if len(custom) != 1 || custom[0] != (slots.Interval{Start: 600, End: 720}) {
		t.Fatalf("custom override %+v", custom)
	}
	if len(sched.Overrides) != 2 {
		t.Fatalf("override outside the range returned: %+v", sched.Overrides)
	}
}
