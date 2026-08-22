// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026  Nils Lehnen

package orchestration_test

import (
	"errors"
	"testing"

	"github.com/iderex/stammtisch/internal/orchestration"
)

// The paths in domain.go that no test reached, which is what #93 asks for: a
// line covered, or listed in the tree with a reason it is not.
//
// Two of them were accessors and one was a refusal, and none of those is a
// coverage number for its own sake. A channel that never reports the space it
// was built for is half of "belongs to exactly one space" left unasserted, and
// a refusal nothing executes is a refusal whose message nobody has read.
//
// One is genuinely unreachable and is not covered. It is named at the bottom of
// this file, with the property that makes it unreachable turned into an
// assertion rather than left as a claim.

// TestAChannelReportsWhatItWasBuiltFrom covers the two accessors that carry the
// invariant. There is no setter for either field, so what a channel reports at
// the end of its life is what it was given at the start.
func TestAChannelReportsWhatItWasBuiltFrom(t *testing.T) {
	spaceID := id(t, "space")
	channelID := id(t, "general")

	c, err := orchestration.NewChannel(channelID, spaceID, "general", true)
	if err != nil {
		t.Fatalf("NewChannel: %v", err)
	}
	if got := c.ID(); got != channelID {
		t.Errorf("the channel reports the identifier %s, want %s", got, channelID)
	}
	if got := c.Space(); got != spaceID {
		t.Errorf("the channel reports the space %s, want %s", got, spaceID)
	}
	if got := c.Name(); got != "general" {
		t.Errorf("the channel reports the name %q, want %q", got, "general")
	}
	if !c.AlwaysOn() {
		t.Error("a channel built always-on does not report itself as always-on")
	}
}

// TestAnOccupancyReportsWhoAndWhere covers the accessor on the value the
// participant list is built from. A projection reading the wrong member out of
// an occupancy would put the right count in the wrong room.
func TestAnOccupancyReportsWhoAndWhere(t *testing.T) {
	s := space(t)
	channel := channelIn(t, s, "general")
	who := id(t, "member")

	o, err := s.Enter(who, channel)
	if err != nil {
		t.Fatalf("Enter: %v", err)
	}
	if got := o.Member(); got != who {
		t.Errorf("the occupancy reports the member %s, want %s", got, who)
	}
	if got := o.Channel(); got != channel {
		t.Errorf("the occupancy reports the channel %s, want %s", got, channel)
	}
}

// TestASpaceRefusesNothingAndNobody covers the two refusals that guard against
// a zero value arriving where a value was expected.
//
// Both are the shape a caller produces by forgetting rather than by trying: a
// constructor whose error was ignored hands on a nil channel, and an
// identifier nobody assigned is the zero one.
func TestASpaceRefusesNothingAndNobody(t *testing.T) {
	s := space(t)
	channel := channelIn(t, s, "general")

	if err := s.AddChannel(nil); !errors.Is(err, orchestration.ErrInvariant) {
		t.Errorf("adding no channel at all returned %v, want ErrInvariant", err)
	}
	if s.Channels() != 1 {
		t.Errorf("the space holds %d channels after a refused add, want 1", s.Channels())
	}

	if _, err := s.Switch(orchestration.ID{}, channel); !errors.Is(err, orchestration.ErrInvariant) {
		t.Errorf("switching nobody into a channel returned %v, want ErrInvariant", err)
	}
	if got := s.Occupants(channel); len(got) != 0 {
		t.Errorf("a refused switch left %d occupants, want 0", len(got))
	}
}

// TestEveryChannelASpaceHoldsCarriesAnIdentifier is the reason the one
// remaining uncovered block in domain.go stays uncovered.
//
// OpenRoom calls NewRoom only after finding the channel in the space, and
// NewRoom fails on exactly one condition, a zero channel identifier. So the
// error arm of that call is unreachable through the type's own surface, and
// there is no fixture that reaches it: NewChannel refuses a zero identifier, so
// no channel a space can hold has one.
//
// That is an argument, and an argument is what drifts. This test asserts the
// property it rests on, so a later change that lets a channel into a space
// without going through NewChannel reddens here rather than quietly making an
// untested line reachable.
func TestEveryChannelASpaceHoldsCarriesAnIdentifier(t *testing.T) {
	if _, err := orchestration.NewChannel(orchestration.ID{}, id(t, "space"), "general", false); !errors.Is(err, orchestration.ErrInvariant) {
		t.Fatalf("a channel with no identifier was built, returning %v", err)
	}

	s := space(t)
	channel := channelIn(t, s, "general")
	held, ok := s.Channel(channel)
	if !ok {
		t.Fatal("the space does not hold the channel it was just given")
	}
	if held.ID().IsZero() {
		t.Fatal("a space holds a channel with no identifier, so OpenRoom can now fail where nothing tests it")
	}

	// And the room opens, which is the arm that is reached.
	room, err := s.OpenRoom(channel)
	if err != nil {
		t.Fatalf("OpenRoom: %v", err)
	}
	if room.Channel() != channel {
		t.Fatalf("the room backs %s, want %s", room.Channel(), channel)
	}
}
