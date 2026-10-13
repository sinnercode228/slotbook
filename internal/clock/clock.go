// Package clock lets the application take the current time from a single
// injectable source, so tests can move time instead of sleeping.
package clock

import (
	"sync"
	"time"
)

type Clock interface {
	Now() time.Time
}

// System is the wall clock, in UTC.
type System struct{}

func (System) Now() time.Time { return time.Now().UTC() }

// Fake is a manually driven clock for tests. It is safe for concurrent use.
type Fake struct {
	mu  sync.Mutex
	now time.Time
}

func NewFake(now time.Time) *Fake { return &Fake{now: now} }

func (f *Fake) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

func (f *Fake) Set(now time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = now
}

func (f *Fake) Advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = f.now.Add(d)
}
