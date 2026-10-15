// Package ratelimit is an in-memory token-bucket limiter keyed by client.
//
// A bucket holds up to Burst tokens, a request takes one, and one comes back
// every Per/Count. Each bucket is stored as the instant at which it will be
// full again: a bucket past that instant answers exactly like a new one, so
// it can be dropped, and that is how idle clients are evicted.
package ratelimit

import (
	"sync"
	"time"

	"github.com/sinnercode228/slotbook/internal/clock"
)

// Limit allows Count requests per Per in the long run and Burst at once.
type Limit struct {
	Count int
	Per   time.Duration
	Burst int
}

// Enabled reports whether the limit restricts anything; the zero Limit
// does not.
func (l Limit) Enabled() bool { return l.Count > 0 && l.Per > 0 && l.Burst > 0 }

// Limiter applies one Limit to each key separately. It is safe for
// concurrent use.
type Limiter struct {
	clock    clock.Clock
	interval time.Duration // to earn one token back
	window   time.Duration // to refill an empty bucket

	mu        sync.Mutex
	full      map[string]time.Time
	lastSweep time.Time
}

// New returns a limiter for an enabled limit.
func New(limit Limit, clk clock.Clock) *Limiter {
	interval := max(limit.Per/time.Duration(limit.Count), time.Nanosecond)
	return &Limiter{
		clock:     clk,
		interval:  interval,
		window:    interval * time.Duration(limit.Burst),
		full:      make(map[string]time.Time),
		lastSweep: clk.Now(),
	}
}

// Allow takes a token from key's bucket. When the bucket is empty it takes
// nothing and returns how long until a token is back.
func (l *Limiter) Allow(key string) (bool, time.Duration) {
	now := l.clock.Now()
	l.mu.Lock()
	defer l.mu.Unlock()

	if now.Sub(l.lastSweep) >= l.window || now.Before(l.lastSweep) {
		l.sweep(now)
	}
	full, ok := l.full[key]
	if !ok || full.Before(now) {
		full = now
	}
	// A bucket is full again at most one window from now, unless the wall
	// clock stepped back; without this it would stay empty until the clock
	// caught up.
	if latest := now.Add(l.window); full.After(latest) {
		full = latest
		l.full[key] = full
	}
	next := full.Add(l.interval)
	if wait := next.Sub(now) - l.window; wait > 0 {
		return false, wait
	}
	l.full[key] = next
	return true, 0
}

// sweep drops the buckets that are full by now. Running it once per window
// keeps only keys seen in the last two windows.
func (l *Limiter) sweep(now time.Time) {
	for key, full := range l.full {
		if !full.After(now) {
			delete(l.full, key)
		}
	}
	l.lastSweep = now
}

// Len returns the number of buckets held in memory.
func (l *Limiter) Len() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.full)
}
