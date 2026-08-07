// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026  iderex

package orchestration

import (
	"fmt"
	"sync"
	"time"
)

// Clock is the only way anything in this package learns what time it is.
//
// It is injected from the first line of code rather than retrofitted, because
// this layer is made of timers, grace periods and reconnect windows, and every
// one of them is untestable in a suite that waits for real time to pass. A
// suite that sleeps is slow, flaky, and unable to test a timeout at all.
//
// The implementation that reads the operating system clock is deliberately not
// here. Nothing under internal/orchestration may read the clock directly, and
// TestNothingInOrchestrationReadsTheClockDirectly refuses it, so the real one
// is built at the edge and passed in.
type Clock interface {
	Now() time.Time
}

// IDs hands out identifiers, and is injected for the same reason the clock is.
// A test that cannot predict an identifier asserts on a shape instead of a
// value, and an assertion on a shape is how a bug survives a suite.
type IDs interface {
	Next() string
}

// FakeClock is a Clock a test moves by hand. It never advances on its own, so a
// test that forgets to advance it hangs on an assertion rather than passing
// because enough real time happened to go by.
type FakeClock struct {
	mu  sync.Mutex
	now time.Time
}

// NewFakeClock returns a clock reading start.
func NewFakeClock(start time.Time) *FakeClock {
	return &FakeClock{now: start}
}

// Now implements Clock.
func (c *FakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// Advance moves the clock forward. A negative duration panics: time running
// backwards is a bug in the test rather than a condition the code under test
// is expected to survive.
func (c *FakeClock) Advance(d time.Duration) {
	if d < 0 {
		panic(fmt.Sprintf("orchestration: FakeClock advanced by %v", d))
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// FixedIDs hands out prefix-1, prefix-2 and so on, so a test can name the
// identifier it expects instead of matching a pattern.
type FixedIDs struct {
	mu     sync.Mutex
	prefix string
	n      int
}

// NewFixedIDs returns a generator producing identifiers under prefix.
func NewFixedIDs(prefix string) *FixedIDs {
	return &FixedIDs{prefix: prefix}
}

// Next implements IDs.
func (g *FixedIDs) Next() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.n++
	return fmt.Sprintf("%s-%d", g.prefix, g.n)
}
