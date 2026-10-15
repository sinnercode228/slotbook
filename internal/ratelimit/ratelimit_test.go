package ratelimit

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sinnercode228/slotbook/internal/clock"
)

var t0 = time.Date(2026, time.October, 12, 8, 0, 0, 0, time.UTC)

// Three requests at once, then one every 10 seconds.
var sixPerMinute = Limit{Count: 6, Per: time.Minute, Burst: 3}

func expectAllowed(t *testing.T, l *Limiter, key string, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		if ok, wait := l.Allow(key); !ok {
			t.Fatalf("%s: request %d of %d refused, wait %v", key, i+1, n, wait)
		}
	}
}

func expectRefused(t *testing.T, l *Limiter, key string, wantWait time.Duration) {
	t.Helper()
	ok, wait := l.Allow(key)
	if ok {
		t.Fatalf("%s: allowed, want a refusal with wait %v", key, wantWait)
	}
	if wait != wantWait {
		t.Fatalf("%s: wait %v, want %v", key, wait, wantWait)
	}
}

func TestBurstThenOneTokenPerInterval(t *testing.T) {
	clk := clock.NewFake(t0)
	l := New(sixPerMinute, clk)

	expectAllowed(t, l, "a", 3)
	expectRefused(t, l, "a", 10*time.Second)

	clk.Advance(9 * time.Second)
	expectRefused(t, l, "a", time.Second)

	// The refusals above took nothing, so the token is back on time.
	clk.Advance(time.Second)
	expectAllowed(t, l, "a", 1)
	expectRefused(t, l, "a", 10*time.Second)

	clk.Advance(25 * time.Second)
	expectAllowed(t, l, "a", 2)
	expectRefused(t, l, "a", 5*time.Second)
}

func TestRefillStopsAtTheBurst(t *testing.T) {
	clk := clock.NewFake(t0)
	l := New(sixPerMinute, clk)
	expectAllowed(t, l, "a", 1)

	clk.Advance(time.Hour)
	expectAllowed(t, l, "a", 3)
	expectRefused(t, l, "a", 10*time.Second)
}

func TestKeysHaveTheirOwnBuckets(t *testing.T) {
	l := New(sixPerMinute, clock.NewFake(t0))
	expectAllowed(t, l, "a", 3)
	expectRefused(t, l, "a", 10*time.Second)
	expectAllowed(t, l, "b", 3)
	expectRefused(t, l, "b", 10*time.Second)
}

func TestSimultaneousCallersShareOneBurst(t *testing.T) {
	l := New(sixPerMinute, clock.NewFake(t0))
	var allowed atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if ok, _ := l.Allow("a"); ok {
				allowed.Add(1)
			}
		}()
	}
	wg.Wait()
	if n := allowed.Load(); n != 3 {
		t.Fatalf("%d of 20 simultaneous requests allowed, want 3", n)
	}
}

func TestFullBucketsAreEvicted(t *testing.T) {
	clk := clock.NewFake(t0)
	l := New(sixPerMinute, clk)

	expectAllowed(t, l, "a", 3) // full again at t0+30s
	clk.Advance(20 * time.Second)
	expectAllowed(t, l, "b", 3) // full again at t0+50s
	if n := l.Len(); n != 2 {
		t.Fatalf("%d buckets, want 2", n)
	}

	clk.Advance(10 * time.Second)
	expectAllowed(t, l, "c", 1)
	if n := l.Len(); n != 2 {
		t.Fatalf("%d buckets after the sweep, want b and c", n)
	}
	// Dropping a full bucket changes no answer: a starts over with the burst.
	expectAllowed(t, l, "a", 3)
	expectRefused(t, l, "a", 10*time.Second)
	// b was kept with its state: one token back after 10 seconds.
	expectAllowed(t, l, "b", 1)
	expectRefused(t, l, "b", 10*time.Second)

	clk.Advance(time.Hour)
	expectAllowed(t, l, "d", 1)
	if n := l.Len(); n != 1 {
		t.Fatalf("%d buckets after an idle hour, want only d", n)
	}
}

func TestAClockSteppingBackWaitsNoLongerThanAFullRefill(t *testing.T) {
	clk := clock.NewFake(t0)
	l := New(sixPerMinute, clk)
	expectAllowed(t, l, "a", 3)

	clk.Set(t0.Add(-time.Hour))
	expectRefused(t, l, "a", 10*time.Second)
	clk.Advance(10 * time.Second)
	expectAllowed(t, l, "a", 1)
}

func TestLimitEnabled(t *testing.T) {
	cases := map[Limit]bool{
		sixPerMinute:                           true,
		{}:                                     false,
		{Count: 0, Per: time.Minute, Burst: 3}: false,
		{Count: 6, Per: 0, Burst: 3}:           false,
		{Count: 6, Per: time.Minute, Burst: 0}: false,
	}
	for limit, want := range cases {
		if got := limit.Enabled(); got != want {
			t.Errorf("%+v.Enabled() = %v, want %v", limit, got, want)
		}
	}
}
