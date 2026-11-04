package kftime

import (
	"sync"
	"time"
)

// FakeClock is a concurrency-safe clock for deterministic tests.
type FakeClock struct {
	mu  sync.RWMutex
	now time.Time
}

// NewFakeClock creates a FakeClock initialized to t.
func NewFakeClock(t time.Time) *FakeClock { return &FakeClock{now: t} }

// Now returns the clock's current time.
func (c *FakeClock) Now() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.now
}

// Set replaces the clock's current time.
func (c *FakeClock) Set(t time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = t
}

// Advance moves the clock forward by d (or backward when d is negative).
func (c *FakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}
