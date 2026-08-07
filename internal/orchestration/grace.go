// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026  iderex

package orchestration

import "time"

// Grace is a period after which something that has not come back is gone: a
// member who dropped, a session waiting to resume, a room held open for a
// reconnect. It is the shape every deadline in this layer takes, and it is here
// so that shape is measured on the injected clock exactly once rather than
// once per state machine.
//
// It holds no timer and starts no goroutine. Expiry is a question asked of the
// clock, which is what makes a test able to prove one without waiting for it.
type Grace struct {
	clock    Clock
	deadline time.Time
}

// NewGrace starts a grace period of d measured from now on clock.
func NewGrace(clock Clock, d time.Duration) *Grace {
	return &Grace{clock: clock, deadline: clock.Now().Add(d)}
}

// Expired reports whether the period has run out. The deadline itself counts as
// expired, so a grace period of zero is over the moment it starts rather than
// lasting until the clock happens to move.
func (g *Grace) Expired() bool {
	return !g.clock.Now().Before(g.deadline)
}

// Remaining is how much of the period is left, and never less than zero. A
// caller reporting how long a member has to reconnect should not have to guard
// against a negative number.
func (g *Grace) Remaining() time.Duration {
	if left := g.deadline.Sub(g.clock.Now()); left > 0 {
		return left
	}
	return 0
}

// Extend pushes the deadline out by d from now, which is what a member who
// reconnected and dropped again gets. It is measured from now rather than from
// the old deadline, so a member who keeps flapping does not accumulate credit.
func (g *Grace) Extend(d time.Duration) {
	g.deadline = g.clock.Now().Add(d)
}
