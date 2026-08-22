// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026  Nils Lehnen

package store_test

import (
	"context"
	"errors"
	"testing"

	"github.com/iderex/stammtisch/internal/orchestration"
	"github.com/iderex/stammtisch/internal/store"
)

func id(t *testing.T, local string) orchestration.ID {
	t.Helper()
	made, err := orchestration.NewID(local, "example.test")
	if err != nil {
		t.Fatalf("NewID(%q): %v", local, err)
	}
	return made
}

func channel(t *testing.T, name string, space orchestration.ID, alwaysOn bool) *orchestration.Channel {
	t.Helper()
	c, err := orchestration.NewChannel(id(t, name), space, name, alwaysOn)
	if err != nil {
		t.Fatalf("NewChannel(%q): %v", name, err)
	}
	return c
}

// TestParsePermissionRoundTripsEveryDeclaredPermission holds the reason grants
// are stored by name. If a name did not come back as the permission that wrote
// it, a stored grant would decode as a different permission or as none.
func TestParsePermissionRoundTripsEveryDeclaredPermission(t *testing.T) {
	for _, want := range orchestration.Permissions() {
		got, err := store.ParsePermission(want.String())
		if err != nil {
			t.Errorf("ParsePermission(%q): %v", want, err)
			continue
		}
		if got != want {
			t.Errorf("ParsePermission(%q) = %v, want %v", want, got, want)
		}
	}
}

// TestParsePermissionRefusesANameTheModelDoesNotDeclare is the other direction,
// and it is the one that matters at a permission boundary: a row naming
// something nobody declares must not become a grant.
func TestParsePermissionRefusesANameTheModelDoesNotDeclare(t *testing.T) {
	for _, name := range []string{"", "manage-everything", "SeeChannel", "see-channel "} {
		if _, err := store.ParsePermission(name); !errors.Is(err, store.ErrInvalid) {
			t.Errorf("ParsePermission(%q) error = %v, want store.ErrInvalid", name, err)
		}
	}
}

func TestMemoryHoldsAndReturnsAChannel(t *testing.T) {
	ctx := context.Background()
	s := store.NewMemory()
	space := id(t, "space")
	general := channel(t, "general", space, false)

	if err := s.PutChannel(ctx, general); err != nil {
		t.Fatalf("PutChannel: %v", err)
	}
	got, err := s.Channel(ctx, general.ID())
	if err != nil {
		t.Fatalf("Channel: %v", err)
	}
	if got.Name() != "general" || got.Space() != space || got.AlwaysOn() {
		t.Errorf("read back %s in %s alwaysOn=%v", got.Name(), got.Space(), got.AlwaysOn())
	}
}

func TestMemoryRefusesNoChannelAndReportsAMissingOne(t *testing.T) {
	ctx := context.Background()
	s := store.NewMemory()

	if err := s.PutChannel(ctx, nil); !errors.Is(err, store.ErrInvalid) {
		t.Errorf("PutChannel(nil) error = %v, want store.ErrInvalid", err)
	}
	if _, err := s.Channel(ctx, id(t, "absent")); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("Channel(absent) error = %v, want store.ErrNotFound", err)
	}
}

func TestMemoryReturnsOneSpacesChannelsInIdentifierOrder(t *testing.T) {
	ctx := context.Background()
	s := store.NewMemory()
	here, elsewhere := id(t, "here"), id(t, "elsewhere")

	for _, c := range []*orchestration.Channel{
		channel(t, "zulu", here, false),
		channel(t, "alpha", here, true),
		channel(t, "mike", elsewhere, false),
	} {
		if err := s.PutChannel(ctx, c); err != nil {
			t.Fatalf("PutChannel: %v", err)
		}
	}

	found, err := s.ChannelsInSpace(ctx, here)
	if err != nil {
		t.Fatalf("ChannelsInSpace: %v", err)
	}
	if len(found) != 2 || found[0].Name() != "alpha" || found[1].Name() != "zulu" {
		t.Fatalf("ChannelsInSpace returned %s, want alpha then zulu", names(found))
	}
}

func names(channels []*orchestration.Channel) []string {
	var got []string
	for _, c := range channels {
		got = append(got, c.Name())
	}
	return got
}

func TestMemoryRecordsMembershipsOnceAndInOrder(t *testing.T) {
	ctx := context.Background()
	s := store.NewMemory()
	space := id(t, "space")

	for _, member := range []string{"zoe", "ada", "zoe"} {
		if err := s.PutMember(ctx, space, id(t, member)); err != nil {
			t.Fatalf("PutMember(%s): %v", member, err)
		}
	}

	members, err := s.Members(ctx, space)
	if err != nil {
		t.Fatalf("Members: %v", err)
	}
	if len(members) != 2 || members[0].Local() != "ada" || members[1].Local() != "zoe" {
		t.Fatalf("Members returned %v, want ada then zoe", members)
	}

	empty, err := s.Members(ctx, id(t, "nowhere"))
	if err != nil {
		t.Fatalf("Members of an unknown space: %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("Members of an unknown space returned %v, want none", empty)
	}
}

func TestMemoryRefusesAMembershipWithAZeroIdentifier(t *testing.T) {
	ctx := context.Background()
	s := store.NewMemory()

	if err := s.PutMember(ctx, orchestration.ID{}, id(t, "ada")); !errors.Is(err, store.ErrInvalid) {
		t.Errorf("PutMember with no space: error = %v, want store.ErrInvalid", err)
	}
	if err := s.PutMember(ctx, id(t, "space"), orchestration.ID{}); !errors.Is(err, store.ErrInvalid) {
		t.Errorf("PutMember with no member: error = %v, want store.ErrInvalid", err)
	}
}

// TestMemoryAnswersAPermissionQuestionThroughAllow is the shape every caller
// uses. The store is the Grantor and orchestration.Allow is the only thing that
// reads it, which is why the assertions here are on Allow and not on the set.
func TestMemoryAnswersAPermissionQuestionThroughAllow(t *testing.T) {
	ctx := context.Background()
	s := store.NewMemory()
	space, general := id(t, "space"), id(t, "general")
	ada := orchestration.Person(id(t, "ada"))
	subject := orchestration.ChannelSubject(space, general)

	if orchestration.Allow(s, ada, orchestration.SeeChannel, subject) {
		t.Fatal("an empty store allowed see-channel")
	}

	if err := s.Grant(ctx, ada.ID(), subject, orchestration.SeeChannel, orchestration.JoinChannel); err != nil {
		t.Fatalf("Grant: %v", err)
	}
	if !orchestration.Allow(s, ada, orchestration.JoinChannel, subject) {
		t.Error("join-channel was refused after being granted with see-channel")
	}
	if orchestration.Allow(s, ada, orchestration.SpeakInChannel, subject) {
		t.Error("speak-in-channel was allowed and was never granted")
	}

	// A grant on one channel is not a grant on another, and a grant on a
	// channel is not a grant on the space.
	other := orchestration.ChannelSubject(space, id(t, "other"))
	if orchestration.Allow(s, ada, orchestration.SeeChannel, other) {
		t.Error("a grant on one channel reached another")
	}
	if orchestration.Allow(s, ada, orchestration.ManageSpace, orchestration.SpaceSubject(space)) {
		t.Error("a grant on a channel reached the space")
	}
}

func TestMemoryGrantsOnASpaceSubject(t *testing.T) {
	ctx := context.Background()
	s := store.NewMemory()
	space := id(t, "space")
	ada := orchestration.Person(id(t, "ada"))
	subject := orchestration.SpaceSubject(space)

	if err := s.Grant(ctx, ada.ID(), subject, orchestration.ManageSpace); err != nil {
		t.Fatalf("Grant: %v", err)
	}
	if !orchestration.Allow(s, ada, orchestration.ManageSpace, subject) {
		t.Error("manage-space was refused after being granted on the space")
	}
}

// TestMemoryEvaluatesABotThroughTheSameStore is the store's half of the claim
// that a bot is not a special kind. Nothing here branches on the principal.
func TestMemoryEvaluatesABotThroughTheSameStore(t *testing.T) {
	ctx := context.Background()
	s := store.NewMemory()
	space, general := id(t, "space"), id(t, "general")
	subject := orchestration.ChannelSubject(space, general)
	transcriber := id(t, "transcriber")

	if err := s.Grant(ctx, transcriber, subject, orchestration.SeeChannel); err != nil {
		t.Fatalf("Grant: %v", err)
	}
	if !orchestration.Allow(s, orchestration.Bot(transcriber), orchestration.SeeChannel, subject) {
		t.Error("a bot was refused a grant written for its identifier")
	}
}

func TestMemoryRefusesAGrantItCannotStore(t *testing.T) {
	ctx := context.Background()
	s := store.NewMemory()
	space := id(t, "space")
	subject := orchestration.SpaceSubject(space)

	if err := s.Grant(ctx, orchestration.ID{}, subject, orchestration.ManageSpace); !errors.Is(err, store.ErrInvalid) {
		t.Errorf("Grant with no principal: error = %v, want store.ErrInvalid", err)
	}
	if err := s.Grant(ctx, id(t, "ada"), orchestration.Subject{}, orchestration.ManageSpace); !errors.Is(err, store.ErrInvalid) {
		t.Errorf("Grant with no subject: error = %v, want store.ErrInvalid", err)
	}
	// A value that is not one of the declared constants. Storing it would
	// put a row in the store that nothing can name and nothing can revoke.
	if err := s.Grant(ctx, id(t, "ada"), subject, orchestration.Permission(99)); !errors.Is(err, store.ErrInvalid) {
		t.Errorf("Grant of an undeclared permission: error = %v, want store.ErrInvalid", err)
	}
}

func TestMemoryHoldsAGainAndTreatsUnityAsNoRow(t *testing.T) {
	ctx := context.Background()
	s := store.NewMemory()
	ada, ben := id(t, "ada"), id(t, "ben")

	percent, err := s.Gain(ctx, ada, ben)
	if err != nil {
		t.Fatalf("Gain: %v", err)
	}
	if percent != store.DefaultGain {
		t.Errorf("an untouched control read %d, want %d", percent, store.DefaultGain)
	}

	if err := s.SetGain(ctx, ada, ben, store.MinGain); err != nil {
		t.Fatalf("SetGain(0): %v", err)
	}
	if percent, err = s.Gain(ctx, ada, ben); err != nil || percent != store.MinGain {
		t.Errorf("Gain after SetGain(0) = %d, %v", percent, err)
	}

	// Back to unity removes the row rather than writing 100, which is the
	// record's rule. The reading is the same either way, so the assertion
	// that it happened is in the sqlite suite where the row is visible.
	if err := s.SetGain(ctx, ada, ben, store.DefaultGain); err != nil {
		t.Fatalf("SetGain(100): %v", err)
	}
	if percent, err = s.Gain(ctx, ada, ben); err != nil || percent != store.DefaultGain {
		t.Errorf("Gain after SetGain(100) = %d, %v", percent, err)
	}
}

func TestMemoryRefusesAGainOutsideTheRecordedRange(t *testing.T) {
	ctx := context.Background()
	s := store.NewMemory()
	ada, ben := id(t, "ada"), id(t, "ben")

	for _, percent := range []int{store.MinGain - 1, store.MaxGain + 1} {
		if err := s.SetGain(ctx, ada, ben, percent); !errors.Is(err, store.ErrInvalid) {
			t.Errorf("SetGain(%d) error = %v, want store.ErrInvalid", percent, err)
		}
	}
	if err := s.SetGain(ctx, orchestration.ID{}, ben, store.MaxGain); !errors.Is(err, store.ErrInvalid) {
		t.Errorf("SetGain with no listener: error = %v, want store.ErrInvalid", err)
	}
}

func TestMemoryClosesWithoutComplaint(t *testing.T) {
	s := store.NewMemory()
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close twice: %v", err)
	}
}
