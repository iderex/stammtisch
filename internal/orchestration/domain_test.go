// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026  iderex

package orchestration_test

import (
	"errors"
	"testing"

	"github.com/iderex/stammtisch/internal/orchestration"
)

const host = "chat.example.org"

func id(t *testing.T, local string) orchestration.ID {
	t.Helper()
	v, err := orchestration.NewID(local, host)
	if err != nil {
		t.Fatalf("NewID(%q, %q): %v", local, host, err)
	}
	return v
}

func space(t *testing.T) *orchestration.Space {
	t.Helper()
	s, err := orchestration.NewSpace(id(t, "space"))
	if err != nil {
		t.Fatalf("NewSpace: %v", err)
	}
	return s
}

// channelIn builds a channel for a space and adds it, failing the test if either
// step is refused.
func channelIn(t *testing.T, s *orchestration.Space, local string) orchestration.ID {
	t.Helper()
	cid := id(t, local)
	c, err := orchestration.NewChannel(cid, s.ID(), local, false)
	if err != nil {
		t.Fatalf("NewChannel(%s): %v", cid, err)
	}
	if err := s.AddChannel(c); err != nil {
		t.Fatalf("AddChannel(%s): %v", cid, err)
	}
	return cid
}

// TestAChannelBelongsToExactlyOneSpace. The space is required at construction
// and there is no way to set it afterwards, so a channel that belongs to none
// cannot be built.
func TestAChannelBelongsToExactlyOneSpace(t *testing.T) {
	var noSpace orchestration.ID
	if _, err := orchestration.NewChannel(id(t, "general"), noSpace, "general", false); !errors.Is(err, orchestration.ErrInvariant) {
		t.Errorf("a channel with no space was built, error = %v", err)
	}

	s := space(t)
	other, err := orchestration.NewSpace(id(t, "other-space"))
	if err != nil {
		t.Fatalf("NewSpace: %v", err)
	}
	c, err := orchestration.NewChannel(id(t, "general"), other.ID(), "general", false)
	if err != nil {
		t.Fatalf("NewChannel: %v", err)
	}
	if err := s.AddChannel(c); !errors.Is(err, orchestration.ErrInvariant) {
		t.Errorf("a space took a channel built for another space, error = %v", err)
	}
	if s.Channels() != 0 {
		t.Errorf("the refused channel was kept: %d channel(s) in the space", s.Channels())
	}
}

// TestAChannelIsANamedPlaceWithAnIdentifier. Both are what makes a channel
// durable rather than a session, so neither is optional.
func TestAChannelIsANamedPlaceWithAnIdentifier(t *testing.T) {
	s := space(t)
	var none orchestration.ID
	if _, err := orchestration.NewChannel(none, s.ID(), "general", false); !errors.Is(err, orchestration.ErrInvariant) {
		t.Errorf("a channel with no identifier was built, error = %v", err)
	}
	if _, err := orchestration.NewChannel(id(t, "general"), s.ID(), "", false); !errors.Is(err, orchestration.ErrInvariant) {
		t.Errorf("a channel with no name was built, error = %v", err)
	}
}

// TestTwoChannelsCannotShareAnIdentifier. Two durable places under one identity
// is the defect every later lookup inherits, and the second one wins silently
// in a map.
func TestTwoChannelsCannotShareAnIdentifier(t *testing.T) {
	s := space(t)
	first := channelIn(t, s, "general")

	again, err := orchestration.NewChannel(first, s.ID(), "general-again", false)
	if err != nil {
		t.Fatalf("NewChannel: %v", err)
	}
	if err := s.AddChannel(again); !errors.Is(err, orchestration.ErrInvariant) {
		t.Fatalf("a second channel with the identifier %s was accepted, error = %v", first, err)
	}

	held, ok := s.Channel(first)
	if !ok {
		t.Fatalf("the first channel is gone")
	}
	if held.Name() != "general" {
		t.Errorf("the refused channel replaced the first: name = %q, want %q", held.Name(), "general")
	}
}

// TestARoomBacksExactlyOneChannel. A room that backed none, or that could be
// pointed at a second channel, would make the room the durable thing.
func TestARoomBacksExactlyOneChannel(t *testing.T) {
	var none orchestration.ID
	if _, err := orchestration.NewRoom(none); !errors.Is(err, orchestration.ErrInvariant) {
		t.Errorf("a room backing no channel was built, error = %v", err)
	}

	s := space(t)
	general := channelIn(t, s, "general")

	first, err := s.OpenRoom(general)
	if err != nil {
		t.Fatalf("OpenRoom: %v", err)
	}
	second, err := s.OpenRoom(general)
	if err != nil {
		t.Fatalf("OpenRoom a second time: %v", err)
	}
	if first != second {
		t.Error("opening a room twice produced two rooms behind one channel")
	}
	if first.Channel() != general {
		t.Errorf("the room backs %s, want %s", first.Channel(), general)
	}
}

// TestARoomExistsOnlyForAChannelTheSpaceHolds. A room for a channel nobody has
// is a live media session nothing can reach or tear down.
func TestARoomExistsOnlyForAChannelTheSpaceHolds(t *testing.T) {
	s := space(t)
	unknown := id(t, "nowhere")

	if _, err := s.OpenRoom(unknown); !errors.Is(err, orchestration.ErrInvariant) {
		t.Fatalf("a room was opened for a channel the space does not hold, error = %v", err)
	}
	if _, running := s.Room(unknown); running {
		t.Error("the refused room was kept")
	}
}

// TestAChannelWithNoRoomIsANormalState. Closing a room that is not running is
// not a fault, because a caller reconnecting after a restart should not have to
// ask first.
func TestAChannelWithNoRoomIsANormalState(t *testing.T) {
	s := space(t)
	general := channelIn(t, s, "general")

	if _, running := s.Room(general); running {
		t.Error("a fresh channel already has a room")
	}
	s.CloseRoom(general)

	if _, err := s.OpenRoom(general); err != nil {
		t.Fatalf("OpenRoom after a close of nothing: %v", err)
	}
	s.CloseRoom(general)
	if _, running := s.Room(general); running {
		t.Error("the room survived being closed")
	}
}

// TestAMemberHoldsAtMostOneOccupancyAcrossTheSpace. A person cannot be in two
// voice channels at once. The rule is held by the space because it is about
// every channel at once, and it is the reason a switch has to be one operation.
func TestAMemberHoldsAtMostOneOccupancyAcrossTheSpace(t *testing.T) {
	s := space(t)
	general := channelIn(t, s, "general")
	second := channelIn(t, s, "second")
	member := id(t, "ada")

	if _, err := s.Enter(member, general); err != nil {
		t.Fatalf("Enter: %v", err)
	}
	if _, err := s.Enter(member, second); !errors.Is(err, orchestration.ErrInvariant) {
		t.Fatalf("the same member entered a second channel, error = %v", err)
	}
	if _, err := s.Enter(member, general); !errors.Is(err, orchestration.ErrInvariant) {
		t.Errorf("the same member entered the channel they are already in, error = %v", err)
	}

	held, occupied := s.Occupancy(member)
	if !occupied {
		t.Fatal("the member's occupancy is gone")
	}
	if held.Channel() != general {
		t.Errorf("the refused entry moved the member to %s, want %s", held.Channel(), general)
	}
	if got := s.Occupants(second); len(got) != 0 {
		t.Errorf("the second channel has occupants %v, want none", got)
	}
}

// TestASwitchIsOneOperationAndRefusesBeforeItMoves. This is the half the
// one-occupancy rule exists for: a leave and a join that can half-fail leaves a
// member nowhere, which is the state the participant list cannot describe.
func TestASwitchIsOneOperationAndRefusesBeforeItMoves(t *testing.T) {
	s := space(t)
	general := channelIn(t, s, "general")
	second := channelIn(t, s, "second")
	member := id(t, "ada")

	if _, err := s.Enter(member, general); err != nil {
		t.Fatalf("Enter: %v", err)
	}

	if _, err := s.Switch(member, id(t, "nowhere")); !errors.Is(err, orchestration.ErrInvariant) {
		t.Fatalf("a switch to a channel the space does not hold was allowed, error = %v", err)
	}
	held, occupied := s.Occupancy(member)
	if !occupied || held.Channel() != general {
		t.Fatalf("the failed switch moved the member: occupied=%v channel=%s, want %s", occupied, held.Channel(), general)
	}

	moved, err := s.Switch(member, second)
	if err != nil {
		t.Fatalf("Switch: %v", err)
	}
	if moved.Channel() != second {
		t.Errorf("the switch landed in %s, want %s", moved.Channel(), second)
	}
	if got := s.Occupants(general); len(got) != 0 {
		t.Errorf("the member is still in %s: %v", general, got)
	}
	if got := s.Occupants(second); len(got) != 1 || got[0] != member {
		t.Errorf("occupants of %s = %v, want exactly %s", second, got, member)
	}
}

// TestAnOccupancyNeedsAMemberAndAChannelTheSpaceHolds.
func TestAnOccupancyNeedsAMemberAndAChannelTheSpaceHolds(t *testing.T) {
	s := space(t)
	general := channelIn(t, s, "general")

	var nobody orchestration.ID
	if _, err := s.Enter(nobody, general); !errors.Is(err, orchestration.ErrInvariant) {
		t.Errorf("an occupancy with no member was built, error = %v", err)
	}
	if _, err := s.Enter(id(t, "ada"), id(t, "nowhere")); !errors.Is(err, orchestration.ErrInvariant) {
		t.Errorf("an occupancy in a channel the space does not hold was built, error = %v", err)
	}
}

// TestLeavingWhenNotPresentIsNotAFault. A client that reconnects after its
// occupancy was reaped has to be able to say it is leaving.
func TestLeavingWhenNotPresentIsNotAFault(t *testing.T) {
	s := space(t)
	general := channelIn(t, s, "general")
	member := id(t, "ada")

	s.Leave(member)

	if _, err := s.Enter(member, general); err != nil {
		t.Fatalf("Enter after a leave of nothing: %v", err)
	}
	s.Leave(member)
	if _, occupied := s.Occupancy(member); occupied {
		t.Error("the occupancy survived the leave")
	}
}

// TestOccupantsAreReturnedInAFixedOrder. A projection built from a map's
// iteration order is a test that passes until it does not.
func TestOccupantsAreReturnedInAFixedOrder(t *testing.T) {
	s := space(t)
	general := channelIn(t, s, "general")

	for _, who := range []string{"grace", "ada", "edsger", "barbara"} {
		if _, err := s.Enter(id(t, who), general); err != nil {
			t.Fatalf("Enter(%s): %v", who, err)
		}
	}

	want := []string{"ada@" + host, "barbara@" + host, "edsger@" + host, "grace@" + host}
	for run := range 3 {
		got := s.Occupants(general)
		if len(got) != len(want) {
			t.Fatalf("run %d: %d occupant(s), want %d", run, len(got), len(want))
		}
		for i := range want {
			if got[i].String() != want[i] {
				t.Fatalf("run %d: occupant %d = %s, want %s", run, i, got[i], want[i])
			}
		}
	}
}

// TestASpaceNeedsAnIdentifier.
func TestASpaceNeedsAnIdentifier(t *testing.T) {
	var none orchestration.ID
	if _, err := orchestration.NewSpace(none); !errors.Is(err, orchestration.ErrInvariant) {
		t.Errorf("a space with no identifier was built, error = %v", err)
	}
}

// TestAlwaysOnIsPerChannel. docs/decisions/channel-and-room-model.md makes it a
// per-channel setting and never a global mode, because the idle cost is per
// held room.
func TestAlwaysOnIsPerChannel(t *testing.T) {
	s := space(t)
	held, err := orchestration.NewChannel(id(t, "lobby"), s.ID(), "lobby", true)
	if err != nil {
		t.Fatalf("NewChannel: %v", err)
	}
	normal, err := orchestration.NewChannel(id(t, "general"), s.ID(), "general", false)
	if err != nil {
		t.Fatalf("NewChannel: %v", err)
	}
	if !held.AlwaysOn() {
		t.Error("an always-on channel does not report itself as one")
	}
	if normal.AlwaysOn() {
		t.Error("a normal channel reports itself as always-on")
	}
}
