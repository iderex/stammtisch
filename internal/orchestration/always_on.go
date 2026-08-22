// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026  Nils Lehnen

package orchestration

import (
	"errors"
	"fmt"
)

// ErrNotPermitted is returned when a well formed request is refused because the
// principal making it does not hold the permission it needs.
//
// It is separate from ErrInvariant on purpose. An invariant failure says the
// request could not have been valid whoever made it; this says the request was
// valid and the answer is no. A caller that collapsed the two would report a
// missing grant as a malformed call, and the person on the other end would go
// looking for the wrong mistake.
var ErrNotPermitted = errors.New("orchestration: not permitted")

// SetAlwaysOn turns a channel's always-on flag on or off on behalf of a
// principal.
//
// The flag is what docs/decisions/channel-and-room-model.md separates an
// ordinary channel from: an ordinary channel's room is destroyed after the last
// leave and a grace period, and an always-on channel's is not destroyed at all.
// Issue #33 is where the rest of that behaviour lives. This is the part of it
// that decides who may change the setting, and it is a permission question
// rather than a field assignment.
//
// The permission is ManageChannel on the channel itself, which carries the
// see-the-channel rule with it because Allow answers it that way: a principal
// who cannot see a channel cannot manage one, so a grant that reached this
// through a space-wide answer would be a way to learn a channel exists by
// changing it.
//
// Nothing here writes anything down. The domain does not reach the store, so
// persistence is the caller's PutChannel, and the restart behaviour this flag
// exists for is a property of that write rather than of this call.
func SetAlwaysOn(g Grantor, actor Principal, c *Channel, on bool) error {
	if c == nil {
		return fmt.Errorf("%w: there is no channel to set the always-on flag on", ErrInvariant)
	}
	if !Allow(g, actor, ManageChannel, ChannelSubject(c.space, c.id)) {
		return fmt.Errorf("%w: %s may not manage channel %s", ErrNotPermitted, actor.ID(), c.id)
	}
	c.alwaysOn = on
	return nil
}
