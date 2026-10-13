package clock

import (
	"testing"
	"time"
)

func TestFakeClock(t *testing.T) {
	start := time.Date(2026, time.October, 12, 8, 0, 0, 0, time.UTC)
	c := NewFake(start)
	if !c.Now().Equal(start) {
		t.Fatalf("Now = %v", c.Now())
	}
	c.Advance(90 * time.Minute)
	if want := start.Add(90 * time.Minute); !c.Now().Equal(want) {
		t.Fatalf("after Advance Now = %v, want %v", c.Now(), want)
	}
	later := start.Add(48 * time.Hour)
	c.Set(later)
	if !c.Now().Equal(later) {
		t.Fatalf("after Set Now = %v", c.Now())
	}
}

func TestSystemClockIsUTC(t *testing.T) {
	if loc := (System{}).Now().Location(); loc != time.UTC {
		t.Fatalf("location %v", loc)
	}
}
