// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026  iderex

package orchestration

import (
	"fmt"
	"sort"
	"time"
)

// The participant list a viewer sees for a channel they have not joined.
//
// docs/decisions/presence-model.md decides what is published, to whom, and at
// what rate. This file implements that record and adds nothing to it. The four
// fields of the published record are the four fields the type admits, and there
// is no fifth for a caller to fill in later, because every field here is
// disclosed to everybody who can see the channel and a field that can be added
// quietly is a disclosure that can be widened quietly.
//
// Nothing here starts a goroutine or holds a timer. An interval boundary is a
// call to Flush made by whoever owns the transport, which is the same shape
// Grace takes and for the same reason: a fan-out bound proved by waiting half a
// second per case is a bound nobody measures twice.

// PresenceCoalesceInterval is the window the server holds changes for before
// sending, and it is the record's number rather than a tuning knob.
//
// It is a constant here and a call to Flush at the transport, so this package
// never reads a clock. TestNothingInOrchestrationReadsTheClockDirectly refuses
// the alternative.
const PresenceCoalesceInterval = 500 * time.Millisecond

// Presence is one member present in one channel, as it is published.
//
// Four fields, and the record argues each one. The identity is the person
// rather than the device or the session. The display name and the avatar are
// not here: the client resolves them from the member directory it already
// holds, so a field that can be looked up separately is not broadcast at the
// width of a channel's whole audience.
//
// SinceUnix is whole seconds and is an integer rather than a time.Time for that
// same reason. This field says how long a named person has been sitting in a
// room, to everybody who can see the room, and a time.Time would carry the
// nanosecond the event happened to land on out to all of them. Truncating at
// the type is what makes "whole seconds" a property of the value instead of a
// habit of whoever formats it.
type Presence struct {
	Channel   ID
	Member    ID
	SinceUnix int64
	SelfMuted bool
}

// PresenceReport is one member's whole occupancy state at one point in that
// member's own sequence. A zero Channel means the member is in no channel,
// which is how a leave is reported.
//
// It carries the whole state rather than a difference, and that is what holds
// convergence under reordering. A stream of differences cannot be folded out of
// order without keeping every gap until the missing piece arrives, which is a
// buffer with its own eviction policy and its own failure mode. A register per
// member needs none of that: the highest sequence wins and everything behind it
// is discardable the moment it is seen.
//
// Seq is per member and starts at one. Zero is not a sequence number, so
// "nothing has been reported about this member" and "the first report" cannot
// be confused, and a report left at the zero value is refused rather than
// treated as the earliest one.
type PresenceReport struct {
	Member    ID
	Seq       uint64
	Channel   ID
	Since     time.Time
	SelfMuted bool
}

// PresenceChange is what one accepted report did. Left is the channel the
// member was in and Joined is the channel they are in now, either of which may
// be the zero ID. A change inside one channel, which today is a self-mute,
// carries the same identifier in both.
type PresenceChange struct {
	Member ID
	Left   ID
	Joined ID
	// Now is the member's published presence after the report. It is only
	// meaningful when Joined is set.
	Now Presence
}

// Moved reports whether the member changed channel, which is the only case that
// moves a count. A self-mute inside one channel does not.
func (c PresenceChange) Moved() bool { return c.Left != c.Joined }

// PresenceProjection is the server-side occupancy state the fan-out is derived
// from. It is maintained and pushed, never polled: the staleness bound in the
// budget is p95 at or below two seconds, and polling at that rate across a
// space is the same fan-out problem with extra steps.
type PresenceProjection struct {
	seq     map[ID]uint64
	present map[ID]Presence
	counts  map[ID]int
}

// NewPresenceProjection returns an empty projection.
func NewPresenceProjection() *PresenceProjection {
	return &PresenceProjection{
		seq:     map[ID]uint64{},
		present: map[ID]Presence{},
		counts:  map[ID]int{},
	}
}

// Apply folds a report in and reports what it did and whether it did anything.
//
// It returns false for a report it refused and for a report that changed
// nothing, and those are the same answer to the only caller that matters: a
// message with no changes in it is not sent. A report is refused when it names
// no member, when its sequence is zero, and when its sequence is at or behind
// the one already held for that member. The last of those is the reordering
// case and it is the common one rather than an error.
func (p *PresenceProjection) Apply(r PresenceReport) (PresenceChange, bool) {
	if r.Member.IsZero() || r.Seq == 0 {
		return PresenceChange{}, false
	}
	if held, seen := p.seq[r.Member]; seen && r.Seq <= held {
		return PresenceChange{}, false
	}
	p.seq[r.Member] = r.Seq

	was, wasPresent := p.present[r.Member]

	change := PresenceChange{Member: r.Member}
	if wasPresent {
		change.Left = was.Channel
	}

	if r.Channel.IsZero() {
		if !wasPresent {
			return PresenceChange{}, false
		}
		delete(p.present, r.Member)
		p.decrement(was.Channel)
		return change, true
	}

	now := Presence{
		Channel:   r.Channel,
		Member:    r.Member,
		SinceUnix: r.Since.Unix(),
		SelfMuted: r.SelfMuted,
	}
	if wasPresent && was == now {
		return PresenceChange{}, false
	}

	if wasPresent && was.Channel != r.Channel {
		p.decrement(was.Channel)
	}
	if !wasPresent || was.Channel != r.Channel {
		p.counts[r.Channel]++
	}
	p.present[r.Member] = now

	change.Joined = r.Channel
	change.Now = now
	return change, true
}

// decrement removes one occupancy from a channel and removes the channel from
// the count map when it empties, so a count of zero and a channel nobody has
// ever been in are one state rather than two.
func (p *PresenceProjection) decrement(channel ID) {
	if p.counts[channel] <= 1 {
		delete(p.counts, channel)
		return
	}
	p.counts[channel]--
}

// Count returns how many members are in a channel.
func (p *PresenceProjection) Count(channel ID) int { return p.counts[channel] }

// Where returns a member's presence, and whether they are in a channel at all.
func (p *PresenceProjection) Where(member ID) (Presence, bool) {
	pr, ok := p.present[member]
	return pr, ok
}

// Occupants returns the members in a channel, longest present first and then in
// identifier order.
//
// The order is fixed rather than the map's for the reason the record gives: a
// list that reorders itself as unrelated changes arrive is a list a person
// cannot read. Ties are broken on the identifier so that two people who arrived
// in the same second still have one order rather than whichever one the runtime
// felt like.
func (p *PresenceProjection) Occupants(channel ID) []Presence {
	var found []Presence
	for _, pr := range p.present {
		if pr.Channel == channel {
			found = append(found, pr)
		}
	}
	sort.Slice(found, func(i, j int) bool {
		if found[i].SinceUnix != found[j].SinceUnix {
			return found[i].SinceUnix < found[j].SinceUnix
		}
		return found[i].Member.String() < found[j].Member.String()
	})
	return found
}

// String renders a presence for a failure message. It is not a wire format and
// nothing parses it.
func (pr Presence) String() string {
	return fmt.Sprintf("%s in %s since %d muted=%t", pr.Member, pr.Channel, pr.SinceUnix, pr.SelfMuted)
}
