// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026  iderex

package orchestration

import (
	"errors"
	"fmt"
	"sort"
)

// The subscription model, which is the half of the presence record that decides
// whether the feature survives a space anybody would actually run.
//
// Two tiers. Counts are pushed unasked, because a count per visible channel is
// what the channel list renders and a client that has the space open needs all
// of them. Identities are subscribed to, because they are the expensive tier and
// a client only needs them for the lists it is actually showing.
//
// The bound the two tiers buy is one message per client per interval, however
// many changes landed in that interval. It is a bound on messages and not on
// changes, which is the whole point: fifty people moving at the end of a
// scheduled event costs the same number of messages as one person moving.

// ErrNoSuchChannel is returned for a channel this hub does not hold and for a
// channel the asking client cannot see. One error for both, with one message,
// deliberately.
//
// A viewer who can tell those two answers apart has learned that the channel
// exists, which is the thing the permission was withholding. The record says a
// channel you cannot view shows nothing including its existence, no placeholder
// and no error a client can tell apart from asking about a channel that was
// never created, and a distinct error here would be exactly that placeholder.
var ErrNoSuchChannel = errors.New("orchestration: no such channel")

// ErrNoSuchClient is returned when a call names a client that is not attached.
// This one is safe to distinguish: a client asking about itself has learned
// nothing it did not already know.
var ErrNoSuchClient = errors.New("orchestration: no such client")

// ClientID names one connected client. It is a connection rather than a person:
// one member on two devices is two clients and one presence.
type ClientID string

// PresenceCountUpdate carries a channel's occupancy count.
//
// It is the resulting count and not a difference. A count converges however
// many updates a client missed or saw twice; a difference has to be accumulated
// against a base the client and the server both believe in, and the first
// dropped message makes that base wrong for as long as the client stays
// connected.
type PresenceCountUpdate struct {
	Channel ID
	Count   int
}

// PresenceMemberUpdate carries one identity in one channel. Present is false
// when the member has left, in which case Now is the zero value.
type PresenceMemberUpdate struct {
	Channel ID
	Member  ID
	Present bool
	Now     Presence
}

// PresenceMessage is what a client receives. Snapshot marks the answer to the
// client's own request, which is the whole of what it asked for rather than the
// changes since it last heard.
type PresenceMessage struct {
	Snapshot bool
	Counts   []PresenceCountUpdate
	Members  []PresenceMemberUpdate
}

// Empty reports whether the message carries nothing. An empty message is not
// sent: a client that would receive one has nothing to learn from it, and
// sending it would break the bound the record states by putting a message on
// the wire per interval per client rather than per interval per client with
// something in it.
func (m PresenceMessage) Empty() bool { return len(m.Counts) == 0 && len(m.Members) == 0 }

// PresenceDelivery is one message for one client.
type PresenceDelivery struct {
	Client  ClientID
	Message PresenceMessage
}

// presenceClient is one attached client and what it is showing.
type presenceClient struct {
	viewer     Principal
	subscribed map[ID]bool
}

// pendingChannel holds what happened to one channel inside the current
// interval, already coalesced.
type pendingChannel struct {
	countMoved bool
	// members is keyed by member, so two changes to one member inside one
	// interval collapse to the last one rather than being sent in order.
	members map[ID]PresenceMemberUpdate
}

// PresenceHub holds the projection, the attached clients and the changes that
// have accumulated since the last flush.
//
// It is not safe for concurrent use and does not pretend to be. Whoever owns
// the transport owns the ordering, and a lock here would let two callers
// interleave a flush with a report and produce a message set neither of them
// asked for.
type PresenceHub struct {
	space    ID
	grants   Grantor
	channels []ID
	known    map[ID]bool

	projection *PresenceProjection

	clients  map[ClientID]*presenceClient
	attached []ClientID

	pending map[ID]*pendingChannel
}

// NewPresenceHub returns a hub for one space.
func NewPresenceHub(space ID, grants Grantor) (*PresenceHub, error) {
	if space.IsZero() {
		return nil, fmt.Errorf("%w: a presence hub needs a space", ErrInvariant)
	}
	if grants == nil {
		return nil, fmt.Errorf("%w: a presence hub needs a grantor, because every channel it publishes is filtered by the see permission", ErrInvariant)
	}
	return &PresenceHub{
		space:      space,
		grants:     grants,
		known:      map[ID]bool{},
		projection: NewPresenceProjection(),
		clients:    map[ClientID]*presenceClient{},
		pending:    map[ID]*pendingChannel{},
	}, nil
}

// Projection returns the state the fan-out reads. It is exposed so a caller can
// answer a question about the whole space without going through a client's
// filtered view, and every path that reaches a client applies the filter.
func (h *PresenceHub) Projection() *PresenceProjection { return h.projection }

// AddChannel tells the hub a channel exists. The hub keeps its own ordered list
// rather than reading one off a Space, because the count list a client receives
// has to arrive in a fixed order and a Space publishes no order today.
func (h *PresenceHub) AddChannel(channel ID) error {
	if channel.IsZero() {
		return fmt.Errorf("%w: a channel needs an identifier", ErrInvariant)
	}
	if h.known[channel] {
		return fmt.Errorf("%w: this hub already holds the channel %s", ErrInvariant, channel)
	}
	h.known[channel] = true
	h.channels = append(h.channels, channel)
	return nil
}

// Attach registers a connected client and returns what it is sent on first
// connect: one message carrying the count of every channel in this space it can
// view.
//
// One message rather than one per channel. A space of two hundred channels
// would otherwise open every connection with two hundred messages, which is the
// case the record names and the reason the snapshot exists at all.
func (h *PresenceHub) Attach(id ClientID, viewer Principal) (PresenceMessage, error) {
	if id == "" {
		return PresenceMessage{}, fmt.Errorf("%w: a client needs an identifier", ErrInvariant)
	}
	if _, taken := h.clients[id]; taken {
		return PresenceMessage{}, fmt.Errorf("%w: the client %s is already attached", ErrInvariant, id)
	}
	if viewer.ID().IsZero() {
		return PresenceMessage{}, fmt.Errorf("%w: a client needs a principal", ErrInvariant)
	}

	c := &presenceClient{viewer: viewer, subscribed: map[ID]bool{}}
	h.clients[id] = c
	h.attached = append(h.attached, id)

	msg := PresenceMessage{Snapshot: true}
	for _, channel := range h.channels {
		if !h.visible(c, channel) {
			continue
		}
		msg.Counts = append(msg.Counts, PresenceCountUpdate{
			Channel: channel,
			Count:   h.projection.Count(channel),
		})
	}
	return msg, nil
}

// Detach forgets a client. Detaching one that is not attached is not an error,
// for the same reason leaving a channel you are not in is not: a connection
// that has already been reaped has to be able to say so.
func (h *PresenceHub) Detach(id ClientID) {
	if _, ok := h.clients[id]; !ok {
		return
	}
	delete(h.clients, id)
	for i, attached := range h.attached {
		if attached == id {
			h.attached = append(h.attached[:i], h.attached[i+1:]...)
			break
		}
	}
}

// Subscribe adds a channel's membership to what a client receives, and returns
// the list as it stands so the client is not waiting an interval for the thing
// it just asked for.
//
// The reply is a snapshot and is not the pushed message the interval bounds.
// The bound the record states is on fan-out, which is the server sending
// without being asked; an answer to a request is neither unasked for nor
// multiplied by the number of connected clients.
func (h *PresenceHub) Subscribe(id ClientID, channel ID) (PresenceMessage, error) {
	c, ok := h.clients[id]
	if !ok {
		return PresenceMessage{}, fmt.Errorf("%w: %s", ErrNoSuchClient, id)
	}
	if !h.visible(c, channel) {
		return PresenceMessage{}, ErrNoSuchChannel
	}
	c.subscribed[channel] = true

	msg := PresenceMessage{
		Snapshot: true,
		Counts:   []PresenceCountUpdate{{Channel: channel, Count: h.projection.Count(channel)}},
	}
	for _, pr := range h.projection.Occupants(channel) {
		msg.Members = append(msg.Members, PresenceMemberUpdate{
			Channel: channel,
			Member:  pr.Member,
			Present: true,
			Now:     pr,
		})
	}
	return msg, nil
}

// Unsubscribe stops sending a client a channel's membership. It reports
// nothing, including for a channel the client cannot see: silence discloses
// nothing either way, and an error here would be the placeholder
// ErrNoSuchChannel exists to avoid.
func (h *PresenceHub) Unsubscribe(id ClientID, channel ID) {
	if c, ok := h.clients[id]; ok {
		delete(c.subscribed, channel)
	}
}

// Apply folds a report into the projection and records what has to go out at
// the end of the interval. It returns whether anything moved.
func (h *PresenceHub) Apply(r PresenceReport) bool {
	change, moved := h.projection.Apply(r)
	if !moved {
		return false
	}

	if change.Moved() {
		if !change.Left.IsZero() {
			h.pendingFor(change.Left).countMoved = true
			h.pendingFor(change.Left).members[change.Member] = PresenceMemberUpdate{
				Channel: change.Left,
				Member:  change.Member,
				Present: false,
			}
		}
		if !change.Joined.IsZero() {
			h.pendingFor(change.Joined).countMoved = true
			h.pendingFor(change.Joined).members[change.Member] = PresenceMemberUpdate{
				Channel: change.Joined,
				Member:  change.Member,
				Present: true,
				Now:     change.Now,
			}
		}
		return true
	}

	// Same channel, so the count did not move and only the subscribers to that
	// channel's membership have anything to learn.
	h.pendingFor(change.Joined).members[change.Member] = PresenceMemberUpdate{
		Channel: change.Joined,
		Member:  change.Member,
		Present: true,
		Now:     change.Now,
	}
	return true
}

func (h *PresenceHub) pendingFor(channel ID) *pendingChannel {
	p, ok := h.pending[channel]
	if !ok {
		p = &pendingChannel{members: map[ID]PresenceMemberUpdate{}}
		h.pending[channel] = p
	}
	return p
}

// Flush ends the coalescing interval and returns at most one message per
// attached client, in attachment order. A client with nothing to learn gets no
// message rather than an empty one.
//
// It is the caller's job to call this once per PresenceCoalesceInterval. That
// is what keeps the timer out of this package and the whole of the fan-out
// bound assertable in a test that never sleeps.
func (h *PresenceHub) Flush() []PresenceDelivery {
	if len(h.pending) == 0 {
		return nil
	}

	// The pending channels in the hub's channel order, so every client's
	// message lists them the same way and a test can assert on a value.
	var changed []ID
	for _, channel := range h.channels {
		if _, ok := h.pending[channel]; ok {
			changed = append(changed, channel)
		}
	}

	var out []PresenceDelivery
	for _, id := range h.attached {
		c := h.clients[id]
		var msg PresenceMessage
		for _, channel := range changed {
			if !h.visible(c, channel) {
				continue
			}
			p := h.pending[channel]
			if p.countMoved {
				msg.Counts = append(msg.Counts, PresenceCountUpdate{
					Channel: channel,
					Count:   h.projection.Count(channel),
				})
			}
			if c.subscribed[channel] {
				msg.Members = append(msg.Members, sortedMemberUpdates(p.members)...)
			}
		}
		if msg.Empty() {
			continue
		}
		out = append(out, PresenceDelivery{Client: id, Message: msg})
	}

	h.pending = map[ID]*pendingChannel{}
	return out
}

// sortedMemberUpdates puts one channel's coalesced updates in identifier order,
// so the message is a value a test can compare rather than a set.
func sortedMemberUpdates(members map[ID]PresenceMemberUpdate) []PresenceMemberUpdate {
	out := make([]PresenceMemberUpdate, 0, len(members))
	for _, u := range members {
		out = append(out, u)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Member.String() < out[j].Member.String() })
	return out
}

// visible answers the only question that filters anything a client receives.
//
// It goes through Allow like every other permission question in this package.
// Occupancy is filtered by channel visibility and by nothing else: if you can
// view the channel you see every occupancy in it, because a list that silently
// omits people misleads the viewer it exists to inform.
func (h *PresenceHub) visible(c *presenceClient, channel ID) bool {
	if !h.known[channel] {
		return false
	}
	return Allow(h.grants, c.viewer, SeeChannel, ChannelSubject(h.space, channel))
}
