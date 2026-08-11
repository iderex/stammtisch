// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026  iderex

package orchestration_test

import (
	"errors"
	"testing"

	"github.com/iderex/stammtisch/internal/orchestration"
)

func alwaysOnFixture(t *testing.T, held ...orchestration.Permission) (*orchestration.Channel, orchestration.Principal, orchestration.Grantor) {
	t.Helper()
	space, channelID, g := fixture(t, held...)
	c, err := orchestration.NewChannel(channelID, space, "general", false)
	if err != nil {
		t.Fatalf("NewChannel: %v", err)
	}
	return c, orchestration.Person(member(t)), g
}

// The flag is settable by a member with the manage permission, which is the
// first done-when line on #33. Both directions are here because a setter that
// only turns a thing on is half a setter, and the half that is missing is the
// one an operator reaches for when a thousand always-on channels turn out to
// cost more than they expected.
func TestAMemberWithManageChannelSetsTheFlagAndClearsItAgain(t *testing.T) {
	c, who, g := alwaysOnFixture(t, orchestration.SeeChannel, orchestration.ManageChannel)

	if c.AlwaysOn() {
		t.Fatal("a channel built with the flag off reported it on")
	}
	if err := orchestration.SetAlwaysOn(g, who, c, true); err != nil {
		t.Fatalf("SetAlwaysOn(true): %v", err)
	}
	if !c.AlwaysOn() {
		t.Fatal("the flag was set and the channel still reports it off")
	}
	if err := orchestration.SetAlwaysOn(g, who, c, false); err != nil {
		t.Fatalf("SetAlwaysOn(false): %v", err)
	}
	if c.AlwaysOn() {
		t.Fatal("the flag was cleared and the channel still reports it on")
	}
}

// A principal who may see, join and speak in a channel still may not decide
// whether its room is held when nobody is in it. The second assertion is the
// one that matters: a refusal that has already changed the value is worse than
// no check, because the caller is told no and the state says yes.
func TestAMemberWithoutManageChannelCannotChangeTheFlag(t *testing.T) {
	c, who, g := alwaysOnFixture(t,
		orchestration.SeeChannel,
		orchestration.JoinChannel,
		orchestration.SpeakInChannel,
	)

	err := orchestration.SetAlwaysOn(g, who, c, true)
	if !errors.Is(err, orchestration.ErrNotPermitted) {
		t.Fatalf("SetAlwaysOn returned %v, want ErrNotPermitted", err)
	}
	if c.AlwaysOn() {
		t.Fatal("the flag changed on a call that was refused")
	}
}

// The near miss beside the case above. ManageChannel is granted, and it is
// granted somewhere else: the same space, a different channel. A check that
// asked the grantor about the space rather than about this channel would pass
// this and would hand every channel in a space to whoever manages one of them.
func TestManageChannelSomewhereElseDoesNotReachThisChannel(t *testing.T) {
	c, who, g := alwaysOnFixture(t, orchestration.SeeChannel, orchestration.ManageChannel)

	elsewhere, err := orchestration.NewChannel(mustID(t, "other@example.test"), c.Space(), "other", false)
	if err != nil {
		t.Fatalf("NewChannel: %v", err)
	}

	if err := orchestration.SetAlwaysOn(g, who, elsewhere, true); !errors.Is(err, orchestration.ErrNotPermitted) {
		t.Fatalf("SetAlwaysOn on another channel returned %v, want ErrNotPermitted", err)
	}
	if elsewhere.AlwaysOn() {
		t.Fatal("the flag changed on a channel the grant does not name")
	}
}

// A grantor that answers nothing is what a caller holds before a store is
// attached, and the answer to a permission question nobody can answer is no.
func TestSetAlwaysOnRefusesWhenThereIsNothingToAskAboutTheGrant(t *testing.T) {
	c, who, _ := alwaysOnFixture(t, orchestration.SeeChannel, orchestration.ManageChannel)

	if err := orchestration.SetAlwaysOn(nil, who, c, true); !errors.Is(err, orchestration.ErrNotPermitted) {
		t.Fatalf("SetAlwaysOn with no grantor returned %v, want ErrNotPermitted", err)
	}
	if c.AlwaysOn() {
		t.Fatal("the flag changed on a call with no grantor to permit it")
	}
}

// The absence of a channel is a malformed call rather than a refusal, and the
// two errors say different things to whoever reads them.
func TestSetAlwaysOnRefusesACallWithNoChannel(t *testing.T) {
	_, who, g := alwaysOnFixture(t, orchestration.SeeChannel, orchestration.ManageChannel)

	err := orchestration.SetAlwaysOn(g, who, nil, true)
	if !errors.Is(err, orchestration.ErrInvariant) {
		t.Fatalf("SetAlwaysOn with no channel returned %v, want ErrInvariant", err)
	}
	if errors.Is(err, orchestration.ErrNotPermitted) {
		t.Fatal("a call with no channel was reported as a permission refusal")
	}
}
