// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026  Nils Lehnen

package orchestration

import (
	"errors"
	"fmt"
	"sort"
)

// The three things docs/decisions/channel-and-room-model.md keeps apart, as
// types rather than as prose.
//
// A channel is durable and belongs to exactly one space. A room is the live
// media session backing at most one channel, and it exists only while that
// channel needs it. An occupancy is one member present in one channel right now,
// and a member holds at most one across the whole space, because a person cannot
// be in two voice channels at once. That last one is the reason switching has to
// be atomic, so it is held by the space rather than by the channel: a rule about
// "across the whole space" cannot be checked by anything smaller.
//
// Every field here is unexported and every value arrives through a constructor
// that refuses an invalid one, so an invalid value cannot be built rather than
// being merely discouraged.

// ErrInvariant is returned when a constructor or a state change is refused. The
// message names the invariant.
var ErrInvariant = errors.New("orchestration: invariant")

// Channel is a durable, named, permissioned place. It exists whether or not
// anybody is in it, and it survives a restart. Which store holds it is #27.
type Channel struct {
	id       ID
	space    ID
	name     string
	alwaysOn bool
}

// NewChannel returns a channel belonging to exactly one space.
//
// The space is required rather than optional, which is the whole of "belongs to
// exactly one space": there is no way to build a channel without one and no way
// to move it afterwards, because the field is unexported and there is no setter.
func NewChannel(id, space ID, name string, alwaysOn bool) (*Channel, error) {
	if id.IsZero() {
		return nil, fmt.Errorf("%w: a channel needs an identifier", ErrInvariant)
	}
	if space.IsZero() {
		return nil, fmt.Errorf("%w: a channel belongs to exactly one space and this one names none", ErrInvariant)
	}
	if name == "" {
		return nil, fmt.Errorf("%w: a channel is a named place and %s has no name", ErrInvariant, id)
	}
	return &Channel{id: id, space: space, name: name, alwaysOn: alwaysOn}, nil
}

// ID returns the channel's identifier.
func (c *Channel) ID() ID { return c.id }

// Space returns the identifier of the space this channel belongs to.
func (c *Channel) Space() ID { return c.space }

// Name returns the channel's name.
func (c *Channel) Name() string { return c.name }

// AlwaysOn reports whether this channel holds its room when empty. It is a
// per-channel setting and never a global mode.
func (c *Channel) AlwaysOn() bool { return c.alwaysOn }

// Room is the live media session backing a channel. It does not survive a
// restart of either side, and a channel with no room is a normal state.
type Room struct {
	channel ID
}

// NewRoom returns a room backing exactly one channel. A room that backed none,
// or that could be pointed at a second channel later, would make the room the
// durable thing and the channel the session, which is the conflation the model
// exists to prevent.
func NewRoom(channel ID) (*Room, error) {
	if channel.IsZero() {
		return nil, fmt.Errorf("%w: a room backs exactly one channel and this one names none", ErrInvariant)
	}
	return &Room{channel: channel}, nil
}

// Channel returns the identifier of the channel this room backs.
func (r *Room) Channel() ID { return r.channel }

// Occupancy is one member present in one channel. It is a fact about right now
// and it does not survive a restart: the participant list is rebuilt as clients
// reconnect.
type Occupancy struct {
	member  ID
	channel ID
}

// Member returns the occupying member.
func (o Occupancy) Member() ID { return o.member }

// Channel returns the channel occupied.
func (o Occupancy) Channel() ID { return o.channel }

// Space owns channels and holds the occupancies across all of them.
//
// It is the smallest thing that can enforce the one-voice-occupancy rule, and it
// is also where a room is bound to a channel, because "at most one room per
// channel" is a statement about a set of channels rather than about one.
type Space struct {
	id          ID
	channels    map[ID]*Channel
	rooms       map[ID]*Room // keyed by channel
	occupancies map[ID]Occupancy
}

// NewSpace returns an empty space.
func NewSpace(id ID) (*Space, error) {
	if id.IsZero() {
		return nil, fmt.Errorf("%w: a space needs an identifier", ErrInvariant)
	}
	return &Space{
		id:          id,
		channels:    map[ID]*Channel{},
		rooms:       map[ID]*Room{},
		occupancies: map[ID]Occupancy{},
	}, nil
}

// ID returns the space's identifier.
func (s *Space) ID() ID { return s.id }

// AddChannel puts a channel in this space.
//
// It refuses a channel built for another space, which is the other half of
// "belongs to exactly one space": the channel names its space and the space
// checks that the channel named it. It also refuses a second channel with the
// same identifier, because two durable places sharing an identity is the defect
// every later lookup inherits.
func (s *Space) AddChannel(c *Channel) error {
	if c == nil {
		return fmt.Errorf("%w: no channel", ErrInvariant)
	}
	if c.space != s.id {
		return fmt.Errorf("%w: channel %s belongs to space %s and not to %s", ErrInvariant, c.id, c.space, s.id)
	}
	if _, taken := s.channels[c.id]; taken {
		return fmt.Errorf("%w: this space already holds a channel with the identifier %s", ErrInvariant, c.id)
	}
	s.channels[c.id] = c
	return nil
}

// Channel returns the channel with this identifier, and whether there is one.
func (s *Space) Channel(id ID) (*Channel, bool) {
	c, ok := s.channels[id]
	return c, ok
}

// Channels returns how many channels this space holds.
func (s *Space) Channels() int { return len(s.channels) }

// OpenRoom starts the room backing a channel and returns it.
//
// It refuses a channel this space does not hold, and it refuses a second room
// for a channel that already has one. Reopening returns the room already there
// rather than a second one, so a caller that races another cannot end up with
// two live sessions behind one channel. When the room is torn down is the
// lifecycle in #33 and is not decided here.
func (s *Space) OpenRoom(channel ID) (*Room, error) {
	if _, held := s.channels[channel]; !held {
		return nil, fmt.Errorf("%w: this space holds no channel %s, so there is nothing to back", ErrInvariant, channel)
	}
	if r, running := s.rooms[channel]; running {
		return r, nil
	}
	r, err := NewRoom(channel)
	if err != nil {
		return nil, err
	}
	s.rooms[channel] = r
	return r, nil
}

// CloseRoom ends the room backing a channel. Closing a channel that has none is
// not an error: a channel with no room is a normal state and not a fault, so
// the caller does not have to ask first.
func (s *Space) CloseRoom(channel ID) {
	delete(s.rooms, channel)
}

// Room returns the room backing a channel, and whether one is running.
func (s *Space) Room(channel ID) (*Room, bool) {
	r, ok := s.rooms[channel]
	return r, ok
}

// Enter records a member as present in a channel.
//
// It refuses a member who already occupies a channel anywhere in this space,
// including the one being entered. A person cannot be in two voice channels at
// once, and the refusal is what makes a switch have to be atomic rather than a
// leave and a join that can half-fail. Switch is the operation that does both.
func (s *Space) Enter(member, channel ID) (Occupancy, error) {
	if member.IsZero() {
		return Occupancy{}, fmt.Errorf("%w: an occupancy needs a member", ErrInvariant)
	}
	if _, held := s.channels[channel]; !held {
		return Occupancy{}, fmt.Errorf("%w: this space holds no channel %s", ErrInvariant, channel)
	}
	if held, occupied := s.occupancies[member]; occupied {
		return Occupancy{}, fmt.Errorf("%w: %s already occupies %s and a member holds at most one occupancy across the space", ErrInvariant, member, held.channel)
	}
	o := Occupancy{member: member, channel: channel}
	s.occupancies[member] = o
	return o, nil
}

// Leave removes a member's occupancy. Leaving when not present is not an error,
// because a reconnecting client that has already been reaped has to be able to
// say so without the server treating it as a fault.
func (s *Space) Leave(member ID) {
	delete(s.occupancies, member)
}

// Switch moves a member from wherever they are to another channel, as one
// operation. It refuses before it removes anything, so a failed switch leaves
// the member where they were rather than nowhere.
func (s *Space) Switch(member, channel ID) (Occupancy, error) {
	if member.IsZero() {
		return Occupancy{}, fmt.Errorf("%w: an occupancy needs a member", ErrInvariant)
	}
	if _, held := s.channels[channel]; !held {
		return Occupancy{}, fmt.Errorf("%w: this space holds no channel %s", ErrInvariant, channel)
	}
	o := Occupancy{member: member, channel: channel}
	s.occupancies[member] = o
	return o, nil
}

// Occupancy returns where a member is, and whether they are anywhere.
func (s *Space) Occupancy(member ID) (Occupancy, bool) {
	o, ok := s.occupancies[member]
	return o, ok
}

// Occupants returns the members present in a channel, in identifier order. The
// order is fixed rather than the map's, so a test can assert on a value instead
// of on a set and a projection built from this cannot depend on iteration luck.
func (s *Space) Occupants(channel ID) []ID {
	var found []ID
	for member, o := range s.occupancies {
		if o.channel == channel {
			found = append(found, member)
		}
	}
	sort.Slice(found, func(i, j int) bool { return found[i].String() < found[j].String() })
	return found
}
