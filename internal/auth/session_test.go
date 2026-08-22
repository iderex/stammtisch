// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026  Nils Lehnen

package auth

import (
	"errors"
	"testing"
	"time"
)

// fakeClock is moved by hand. It never advances on its own, so a test that
// forgets to move it hangs on an assertion rather than passing because enough
// real time went by.
type fakeClock struct{ now time.Time }

func (c *fakeClock) Now() time.Time { return c.now }

func (c *fakeClock) advance(d time.Duration) { c.now = c.now.Add(d) }

func newFakeClock() *fakeClock {
	return &fakeClock{now: time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)}
}

func TestAnOpenSessionIsFoundByItsToken(t *testing.T) {
	clock := newFakeClock()
	sessions := NewSessions(clock)

	opened, token, err := sessions.Open("someone")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if token == "" {
		t.Fatal("Open returned no token")
	}
	if opened.Owner != "someone" {
		t.Errorf("owner is %q", opened.Owner)
	}
	if want := clock.now.Add(TokenLifetime); !opened.Expires.Equal(want) {
		t.Errorf("expires at %v, want %v", opened.Expires, want)
	}

	found, err := sessions.Lookup(token)
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if found.ID != opened.ID {
		t.Errorf("looked up %q, want %q", found.ID, opened.ID)
	}
}

func TestTwoSessionsGetDifferentTokensAndIdentifiers(t *testing.T) {
	sessions := NewSessions(newFakeClock())

	first, firstToken, err := sessions.Open("someone")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	second, secondToken, err := sessions.Open("someone")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	if firstToken == secondToken {
		t.Error("two sessions share a token")
	}
	if first.ID == second.ID {
		t.Error("two sessions share an identifier")
	}
	if firstToken == first.ID || secondToken == second.ID {
		t.Error("the identifier a listing shows is the token")
	}
}

func TestAnUnknownTokenIsNotASession(t *testing.T) {
	sessions := NewSessions(newFakeClock())

	if _, err := sessions.Lookup("a token nobody issued"); !errors.Is(err, ErrNoSuchSession) {
		t.Fatalf("Lookup returned %v, want ErrNoSuchSession", err)
	}
}

func TestARevokedSessionIsGoneAndReadsAsUnknown(t *testing.T) {
	sessions := NewSessions(newFakeClock())

	opened, token, err := sessions.Open("someone")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	if !sessions.Revoke(opened.ID) {
		t.Fatal("Revoke reported nothing to revoke")
	}
	if _, err := sessions.Lookup(token); !errors.Is(err, ErrNoSuchSession) {
		t.Fatalf("Lookup returned %v, want ErrNoSuchSession", err)
	}
	if sessions.Revoke(opened.ID) {
		t.Error("revoking twice reported a second revocation")
	}
	if sessions.Revoke("an identifier nobody was issued") {
		t.Error("revoking an unknown identifier reported a revocation")
	}
}

// The expiry is asserted at the boundary and one instant past it, on a clock a
// test moves, so neither leg waits for real time.
func TestATokenStopsWorkingWhenItExpires(t *testing.T) {
	clock := newFakeClock()
	sessions := NewSessions(clock)

	_, token, err := sessions.Open("someone")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	clock.advance(TokenLifetime - time.Nanosecond)
	if _, err := sessions.Lookup(token); err != nil {
		t.Fatalf("a token one instant inside its lifetime returned %v", err)
	}

	clock.advance(time.Nanosecond)
	if _, err := sessions.Lookup(token); !errors.Is(err, ErrSessionExpired) {
		t.Fatalf("Lookup returned %v at the expiry instant, want ErrSessionExpired", err)
	}
}

func TestAnOwnerSeesTheirOwnSessionsOldestFirstAndNobodyElseSees(t *testing.T) {
	clock := newFakeClock()
	sessions := NewSessions(clock)

	first, _, err := sessions.Open("someone")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	clock.advance(time.Minute)
	second, _, err := sessions.Open("someone")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, _, err := sessions.Open("somebody else"); err != nil {
		t.Fatalf("Open: %v", err)
	}

	listed := sessions.List("someone")
	if len(listed) != 2 {
		t.Fatalf("listed %d sessions, want 2", len(listed))
	}
	if listed[0].ID != first.ID || listed[1].ID != second.ID {
		t.Errorf("listed %q then %q, want %q then %q", listed[0].ID, listed[1].ID, first.ID, second.ID)
	}

	if n := len(sessions.List("somebody else")); n != 1 {
		t.Errorf("the other owner sees %d sessions, want 1", n)
	}
	if n := len(sessions.List("nobody")); n != 0 {
		t.Errorf("an owner with no sessions sees %d", n)
	}
}

// An expired session stays in the listing rather than being swept by whoever
// looked it up last. The owner is owed a true picture of what exists.
func TestAnExpiredSessionIsStillListed(t *testing.T) {
	clock := newFakeClock()
	sessions := NewSessions(clock)

	if _, _, err := sessions.Open("someone"); err != nil {
		t.Fatalf("Open: %v", err)
	}
	clock.advance(2 * TokenLifetime)

	listed := sessions.List("someone")
	if len(listed) != 1 {
		t.Fatalf("listed %d sessions, want the expired one", len(listed))
	}
	if listed[0].Expires.After(clock.now) {
		t.Error("the listed session does not read as expired")
	}
}

// Two sessions opened at one instant still list in a stated order, so a listing
// is not a set whose order changes between calls.
func TestTwoSessionsOpenedAtOneInstantListInAStatedOrder(t *testing.T) {
	sessions := NewSessions(newFakeClock())

	if _, _, err := sessions.Open("someone"); err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, _, err := sessions.Open("someone"); err != nil {
		t.Fatalf("Open: %v", err)
	}

	first := sessions.List("someone")
	for i := 0; i < 8; i++ {
		again := sessions.List("someone")
		if again[0].ID != first[0].ID || again[1].ID != first[1].ID {
			t.Fatalf("the listing moved between calls: %q then %q", again[0].ID, again[1].ID)
		}
	}
	if first[0].ID >= first[1].ID {
		t.Error("the tie is not broken by identifier")
	}
}

func TestASessionNeedsAnOwner(t *testing.T) {
	sessions := NewSessions(newFakeClock())

	if _, _, err := sessions.Open(""); err == nil {
		t.Fatal("Open accepted an empty owner")
	}
}

func TestOpenReportsAFailedDraw(t *testing.T) {
	sentinel := errors.New("no entropy")
	original := randRead
	randRead = func([]byte) (int, error) { return 0, sentinel }
	t.Cleanup(func() { randRead = original })

	sessions := NewSessions(newFakeClock())
	if _, _, err := sessions.Open("someone"); !errors.Is(err, sentinel) {
		t.Fatalf("Open returned %v, want the reader's own error", err)
	}
}
