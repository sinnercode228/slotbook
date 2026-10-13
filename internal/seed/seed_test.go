package seed

import (
	"context"
	"strings"
	"testing"

	"github.com/sinnercode228/slotbook/internal/dbtest"
)

const demoPath = "../../seed/demo.json"

func TestDemoChainIsValid(t *testing.T) {
	chain, err := LoadFile(demoPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(chain.Locations) != 2 {
		t.Fatalf("demo chain has %d locations, want 2", len(chain.Locations))
	}
	zones := map[string]bool{}
	staff, services := 0, 0
	for _, l := range chain.Locations {
		zones[l.TimeZone] = true
		staff += len(l.Staff)
		services += len(l.Services)
	}
	if !zones["Europe/Moscow"] || !zones["Europe/Berlin"] {
		t.Fatalf("demo zones %v", zones)
	}
	if staff < 5 || staff > 6 || services < 6 || services > 8 {
		t.Fatalf("demo chain has %d staff and %d services", staff, services)
	}
}

func TestParseRejectsInconsistentChains(t *testing.T) {
	cases := map[string]string{
		"unknown zone":        `{"locations":[{"slug":"a","name":"A","time_zone":"Mars/Olympus"}]}`,
		"unknown field":       `{"locations":[{"slug":"a","name":"A","time_zone":"UTC","color":"red"}]}`,
		"duplicate slug":      `{"locations":[{"slug":"a","name":"A","time_zone":"UTC"},{"slug":"a","name":"B","time_zone":"UTC"}]}`,
		"bad currency":        `{"locations":[{"slug":"a","name":"A","time_zone":"UTC","services":[{"key":"s","name":"S","duration_minutes":30,"price_minor":1,"currency":"rub"}]}]}`,
		"zero duration":       `{"locations":[{"slug":"a","name":"A","time_zone":"UTC","services":[{"key":"s","name":"S","duration_minutes":0,"price_minor":1,"currency":"RUB"}]}]}`,
		"unknown service key": `{"locations":[{"slug":"a","name":"A","time_zone":"UTC","staff":[{"key":"p","name":"P","services":["nope"]}]}]}`,
		"bad hours":           `{"locations":[{"slug":"a","name":"A","time_zone":"UTC","staff":[{"key":"p","name":"P","weekly_hours":{"monday":[{"start":"18:00","end":"09:00"}]}}]}]}`,
		"empty slug":          `{"locations":[{"slug":"","name":"A","time_zone":"UTC"}]}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse(strings.NewReader(body)); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestApplyLoadsTheDemoChainOnce(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	chain, err := LoadFile(demoPath)
	if err != nil {
		t.Fatal(err)
	}
	res, err := Apply(ctx, pool, chain)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Locations) != 2 || len(res.Skipped) != 0 {
		t.Fatalf("first apply: %+v", res)
	}

	counts := map[string]int{}
	for _, table := range []string{"locations", "staff", "services", "staff_services", "working_hours", "schedule_overrides"} {
		var n int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&n); err != nil {
			t.Fatal(err)
		}
		counts[table] = n
	}
	if counts["locations"] != 2 || counts["staff"] < 5 || counts["services"] < 6 || counts["working_hours"] == 0 || counts["staff_services"] == 0 {
		t.Fatalf("unexpected row counts %v", counts)
	}

	again, err := Apply(ctx, pool, chain)
	if err != nil {
		t.Fatal(err)
	}
	if len(again.Skipped) != 2 {
		t.Fatalf("second apply should skip both locations: %+v", again)
	}
	var n int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM staff").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != counts["staff"] {
		t.Fatalf("second apply added staff: %d -> %d", counts["staff"], n)
	}
}
