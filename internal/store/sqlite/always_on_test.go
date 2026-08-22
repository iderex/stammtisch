// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026  Nils Lehnen

package sqlite_test

import (
	"context"
	"errors"
	"testing"

	"github.com/iderex/stammtisch/internal/orchestration"
	"github.com/iderex/stammtisch/internal/store/sqlite"
)

// A restart, from the store's side, is a second Open on the same file. These
// two tests are the pair of done-when lines on #33 that do not need a media
// plane: the flag survives one, and a channel carrying it comes back present
// and joinable without anybody having joined it first.
//
// Both reopen rather than reuse the handle. A test that asserted against the
// store it had just written through would pass on a build that never wrote the
// column at all, which is the failure the flag exists to rule out: an always-on
// channel that quietly becomes an ordinary one the first time the process
// stops.

func TestTheAlwaysOnFlagSurvivesARestart(t *testing.T) {
	ctx := context.Background()
	first, path := openTemp(t)
	space := id(t, "space")
	ada := orchestration.Person(id(t, "ada"))
	general := channel(t, "general", space, false)

	if err := first.Grant(ctx, ada.ID(), orchestration.ChannelSubject(space, general.ID()),
		orchestration.SeeChannel, orchestration.ManageChannel); err != nil {
		t.Fatalf("Grant: %v", err)
	}
	if err := orchestration.SetAlwaysOn(first, ada, general, true); err != nil {
		t.Fatalf("SetAlwaysOn: %v", err)
	}
	if err := first.PutChannel(ctx, general); err != nil {
		t.Fatalf("PutChannel: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	second, err := sqlite.Open(ctx, path)
	if err != nil {
		t.Fatalf("reopening %s: %v", path, err)
	}
	t.Cleanup(func() { _ = second.Close() })

	read, err := second.Channel(ctx, general.ID())
	if err != nil {
		t.Fatalf("Channel after the restart: %v", err)
	}
	if !read.AlwaysOn() {
		t.Fatal("the channel came back from the restart with the always-on flag off")
	}
}

// The line this one holds is that an always-on channel is present and joinable
// after a restart with no prior join. Nothing joins anything here, which is the
// condition: an always-on channel is not brought back by somebody arriving, so
// it has to be there before anybody does.
func TestAnAlwaysOnChannelIsPresentAndJoinableAfterARestartWithNoPriorJoin(t *testing.T) {
	ctx := context.Background()
	first, path := openTemp(t)
	space := id(t, "space")
	ada := orchestration.Person(id(t, "ada"))
	general := channel(t, "general", space, true)

	if err := first.PutChannel(ctx, general); err != nil {
		t.Fatalf("PutChannel: %v", err)
	}
	if err := first.Grant(ctx, ada.ID(), orchestration.ChannelSubject(space, general.ID()),
		orchestration.SeeChannel, orchestration.JoinChannel); err != nil {
		t.Fatalf("Grant: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	second, err := sqlite.Open(ctx, path)
	if err != nil {
		t.Fatalf("reopening %s: %v", path, err)
	}
	t.Cleanup(func() { _ = second.Close() })

	listed, err := second.ChannelsInSpace(ctx, space)
	if err != nil {
		t.Fatalf("ChannelsInSpace after the restart: %v", err)
	}
	if len(listed) != 1 || listed[0].ID() != general.ID() {
		t.Fatalf("the space came back holding %v, want the one always-on channel", nameOrder(listed))
	}
	if !listed[0].AlwaysOn() {
		t.Fatal("the listed channel came back with the always-on flag off")
	}
	if !orchestration.Allow(second, ada, orchestration.JoinChannel,
		orchestration.ChannelSubject(space, general.ID())) {
		t.Fatal("a member who may join the channel was refused after the restart")
	}
}

// A channel read back out of a reopened store is a domain value and not a row,
// so the flag cannot be changed on it by anybody who could not change it
// before. The refusal travels with the value rather than with the handle it
// came from.
func TestTheFlagOnAChannelReadBackAfterARestartIsStillPermissioned(t *testing.T) {
	ctx := context.Background()
	first, path := openTemp(t)
	space := id(t, "space")
	ben := orchestration.Person(id(t, "ben"))
	general := channel(t, "general", space, true)

	if err := first.PutChannel(ctx, general); err != nil {
		t.Fatalf("PutChannel: %v", err)
	}
	if err := first.Grant(ctx, ben.ID(), orchestration.ChannelSubject(space, general.ID()),
		orchestration.SeeChannel, orchestration.JoinChannel); err != nil {
		t.Fatalf("Grant: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	second, err := sqlite.Open(ctx, path)
	if err != nil {
		t.Fatalf("reopening %s: %v", path, err)
	}
	t.Cleanup(func() { _ = second.Close() })

	read, err := second.Channel(ctx, general.ID())
	if err != nil {
		t.Fatalf("Channel after the restart: %v", err)
	}
	if err := orchestration.SetAlwaysOn(second, ben, read, false); !errors.Is(err, orchestration.ErrNotPermitted) {
		t.Fatalf("SetAlwaysOn returned %v, want ErrNotPermitted", err)
	}
	if !read.AlwaysOn() {
		t.Fatal("the flag was cleared by a call that was refused")
	}
}
