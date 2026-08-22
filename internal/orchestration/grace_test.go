// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026  Nils Lehnen

package orchestration_test

import (
	"testing"
	"time"

	"github.com/iderex/stammtisch/internal/orchestration"
)

// epoch is an arbitrary fixed instant. It is written out rather than taken from
// the operating system clock so that a failure names the same numbers every
// time it happens.
var epoch = time.Date(2026, time.August, 6, 12, 0, 0, 0, time.UTC)

// TestAGracePeriodExpiresWithoutWaitingForIt is the test issue #24 asks for by
// name. It proves a grace period runs out, and it contains no sleep: the clock
// is moved by hand and the code under test cannot tell the difference.
func TestAGracePeriodExpiresWithoutWaitingForIt(t *testing.T) {
	clock := orchestration.NewFakeClock(epoch)
	grace := orchestration.NewGrace(clock, 30*time.Second)

	if grace.Expired() {
		t.Fatal("a grace period of 30s was over the moment it started")
	}
	if got := grace.Remaining(); got != 30*time.Second {
		t.Fatalf("remaining is %v at the start, want 30s", got)
	}

	clock.Advance(29 * time.Second)
	if grace.Expired() {
		t.Fatal("a 30s grace period was over after 29s")
	}
	if got := grace.Remaining(); got != time.Second {
		t.Fatalf("remaining is %v after 29s, want 1s", got)
	}

	clock.Advance(time.Second)
	if !grace.Expired() {
		t.Fatal("a 30s grace period was still running after 30s")
	}
	if got := grace.Remaining(); got != 0 {
		t.Fatalf("remaining is %v after the deadline, want 0", got)
	}
}

func TestAGracePeriodOfZeroIsOverAtOnce(t *testing.T) {
	clock := orchestration.NewFakeClock(epoch)
	if !orchestration.NewGrace(clock, 0).Expired() {
		t.Fatal("a grace period of zero was still running")
	}
}

func TestExtendingAGracePeriodMeasuresFromNow(t *testing.T) {
	// A member who keeps dropping and returning does not accumulate credit.
	clock := orchestration.NewFakeClock(epoch)
	grace := orchestration.NewGrace(clock, 30*time.Second)
	clock.Advance(20 * time.Second)
	grace.Extend(30 * time.Second)

	if got := grace.Remaining(); got != 30*time.Second {
		t.Fatalf("remaining is %v after extending by 30s at t+20s, want 30s", got)
	}
	clock.Advance(30 * time.Second)
	if !grace.Expired() {
		t.Fatal("the extended period was still running at t+50s")
	}
}

func TestTheFakeClockOnlyMovesWhenItIsMoved(t *testing.T) {
	clock := orchestration.NewFakeClock(epoch)
	first := clock.Now()
	for i := 0; i < 1000; i++ {
		_ = clock.Now()
	}
	if !clock.Now().Equal(first) {
		t.Fatalf("the clock moved on its own, from %v to %v", first, clock.Now())
	}
	clock.Advance(time.Hour)
	if got := clock.Now().Sub(first); got != time.Hour {
		t.Fatalf("advancing by an hour moved the clock by %v", got)
	}
}

func TestAdvancingTheFakeClockBackwardsPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("advancing the clock by -1s was allowed")
		}
	}()
	orchestration.NewFakeClock(epoch).Advance(-time.Second)
}

func TestFixedIDsAreThePointOfInjectingThem(t *testing.T) {
	ids := orchestration.NewFixedIDs("member")
	for _, want := range []string{"member-1", "member-2", "member-3"} {
		if got := ids.Next(); got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	}
}
