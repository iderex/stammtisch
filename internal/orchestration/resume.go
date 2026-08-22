// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026  Nils Lehnen

package orchestration

import (
	"errors"
	"fmt"
	"sort"
	"time"
)

// Reconnect and resume, which is issue #34.
//
// Connections drop. On a service people leave open all day they drop
// constantly, and the difference between a good and a bad implementation is
// whether a five second blip costs somebody their place in a conversation.
// Below the window the member's occupancy is never released, so nobody in the
// channel sees them leave. Above it the occupancy is released and the client
// rejoins normally, which is one leave and then one join rather than a flicker.
//
// # The exchange, as a client author has to implement it
//
// A resume is not a second way in. The client opens a new connection, agrees a
// version and authenticates exactly as it would on a first connection, and only
// then presents a ticket. That keeps one authentication path rather than two,
// and it means the member this register compares against is the authenticated
// one rather than a field the peer filled in.
//
// What the client presents is a ResumeTicket: the session it is claiming
// continuity with, and the highest revision it folded in before the connection
// went. Revisions come from Published below, one per batch of change the server
// sent, so the number a client echoes back is a number the server issued rather
// than one the client kept for itself.
//
// What the server may refuse is a closed set of four, and each is a distinct
// sentence because a client can act on three of them differently:
//
//   - ErrNoSuspension. Nothing is held for that session. The client has never
//     been here, or it already resumed, or the leave has been reported and the
//     server has forgotten it. Join normally.
//   - ErrResumeWindowPassed. The suspension is held but the window ran out
//     before the ticket arrived. Join normally. It is separate from the case
//     above because the two say different things about how long the client was
//     away, and a client that logs the difference can tell a slow reconnect
//     from a lost session.
//   - ErrResumeNotYours. The session names a suspension belonging to a
//     different member. Nothing about this is recoverable by retrying.
//   - ErrResumeAhead. The ticket folds a revision this server never published.
//     A client cannot have seen more than was sent, so this is a bug or a
//     forgery, and answering it as though the client were merely stale would
//     hide both.
//
// What the server answers otherwise is a ResumeAnswer, and it has two arms. The
// register either names the channels whose membership moved while the client
// was away, which is the difference, or it says it can no longer tell and
// everything follows. The second arm is a fallback rather than a failure: a
// client that presents a state the server no longer holds gets the whole state
// back, and never an error.
//
// # What no part of this is carried over yet
//
// Nothing on the wire carries any of it. internal/signalling defines the
// framing, the version negotiation and the authentication frame, and the
// message set beyond those is not in the tree: KindSpaceState is declared with
// its payload left to whoever defines it, and the presence messages
// PresenceHub produces have no encoding at all. So both arms of the answer name
// bytes nothing can write today, and a frame kind added here would sequence
// messages that do not exist. The register is the decision and the window; the
// frames are owed by whichever change first gives a space state a payload, and
// no issue on the board carries that today.
//
// This file is the record as well as the code. Issue #34 declares
// `Scope: internal/`, so a decision record under docs/ would be a path outside
// it, and the two numbers argued below are written here for that reason.
//
// # Nothing here holds a timer
//
// Expiry is a question asked of the injected clock, and a caller ends the
// window by calling Reap rather than by a goroutine firing. That is the shape
// Grace and PresenceHub already take, and it is what makes "inside the window"
// and "outside the window" two assertions instead of two sleeps.

// ResumeWindow is how long the server will accept a client claiming to be the
// same member.
//
// Thirty seconds, which is the same number as the room grace period in
// docs/decisions/channel-and-room-model.md, and it is the same number by
// argument rather than by inheritance. The two defend different things. The
// grace period keeps a room alive so a session survives a blip; this window is
// how long the server will believe a client that says it is the same member.
// They come out equal because both are bounded by one transport fact: the ICE
// agent under the chosen engine gives up after five seconds to disconnected
// plus twenty five to failed, and past that the client is establishing a new
// connection, which is a join whatever this register would have allowed.
//
// It is its own constant rather than an alias of the grace period, because the
// day one of the two moves for a reason of its own, an alias makes the other
// move silently with it.
const ResumeWindow = 30 * time.Second

// ResumeMissedChannels is how many distinct channels the register will name for
// one suspended client before it stops trying and answers with everything.
//
// The bound exists because the alternative is a register that grows with how
// long a client is away, which is a memory cost an absent peer chooses. Sixty
// four is above what a blip in a busy space produces and far below what would
// be worth sending as a difference: past that the difference is most of the
// state, and the full transfer is both smaller to describe and simpler to be
// right about. The number is a choice rather than a measurement, and what would
// move it is a measurement of how many channels actually move inside thirty
// seconds in a space anybody runs.
const ResumeMissedChannels = 64

// The four refusals. Each is a sentinel rather than a formatted string, so a
// caller branches on the answer instead of reading it.
var (
	// ErrNoSuspension is returned for a session this register holds nothing
	// for, which covers a session that never suspended, one that has already
	// resumed, and one whose leave has been reported and forgotten.
	ErrNoSuspension = errors.New("orchestration: no suspension is held for that session")

	// ErrResumeWindowPassed is returned when the suspension is still held but
	// its window ran out before the ticket arrived. The suspension is left
	// where it is rather than dropped here, because the leave it still owes is
	// Reap's to report and a refusal that quietly forgot it would lose the
	// only event other members are waiting for.
	ErrResumeWindowPassed = errors.New("orchestration: the resume window has passed")

	// ErrResumeNotYours is returned when the ticket names a suspension held for
	// a different member. The comparison is against the member the connection
	// authenticated as, not against anything in the ticket.
	ErrResumeNotYours = errors.New("orchestration: the suspension belongs to another member")

	// ErrResumeAhead is returned for a ticket folding a revision this server
	// never published.
	ErrResumeAhead = errors.New("orchestration: the ticket folds a revision this server never published")
)

// ResumeTicket is what a client presents on a reconnect.
//
// Two fields and neither of them names a member. Who is asking is the
// authenticated identity of the connection the ticket arrived on, and a member
// field here would be a second answer to that question for a peer to disagree
// with.
type ResumeTicket struct {
	// Session is the session the client is claiming continuity with.
	Session string
	// Folded is the highest revision the client folded in.
	Folded uint64
}

// ResumeAnswer is what the server sends back when it accepts a ticket.
//
// Full and Missed are not both meaningful. When Full is true the register could
// no longer say what the client missed and the whole state follows, so Missed
// is empty and would be misleading if it were not.
type ResumeAnswer struct {
	// Member is who the register held the suspension for. It is returned so a
	// caller has the answer in one place rather than carrying it alongside.
	Member ID
	// Missed names the channels whose membership moved while the client was
	// away, in identifier order.
	Missed []ID
	// Full says the difference is gone and everything follows instead.
	Full bool
}

// suspension is one client's held place: who it belongs to, when the window
// runs out, and what has happened since it dropped.
type suspension struct {
	member ID
	grace  *Grace
	// from is the revision current when the client dropped. A ticket folding
	// anything below it is asking about change this register never recorded.
	from uint64
	// missed is a set rather than a list, because a channel that moved forty
	// times while a client was away is one channel to resubscribe to.
	//
	// A nil set is the whole of "this suspension can no longer say what was
	// missed". It is one field rather than a set beside a flag saying the set
	// is meaningless, because two fields holding one fact can disagree and the
	// disagreement reads as a difference that is missing everything before the
	// overflow.
	missed map[ID]bool
}

// Resumptions holds the suspended clients of one space and the revision counter
// their tickets are measured against.
//
// It is not safe for concurrent use, for the reason PresenceHub gives for the
// same choice: whoever owns the transport owns the ordering, and a lock here
// would let a suspend interleave with a reap and produce an answer neither
// caller asked for.
type Resumptions struct {
	clock    Clock
	window   time.Duration
	breadth  int
	revision uint64
	held     map[string]*suspension
	// order is the sessions in the order they suspended, so Reap reports in a
	// fixed order and a test asserts on a value rather than on a set.
	order []string
}

// NewResumptions returns an empty register.
//
// window and breadth are arguments rather than the constants above, so a test
// can hold a one second window without waiting one second of anything and the
// overflow arm can be reached without building sixty four channels. The
// constants are what a server passes.
func NewResumptions(clock Clock, window time.Duration, breadth int) (*Resumptions, error) {
	if clock == nil {
		return nil, fmt.Errorf("%w: a resume register needs a clock, because the window is the whole of what it decides", ErrInvariant)
	}
	if window <= 0 {
		return nil, fmt.Errorf("%w: a resume window of %v accepts nothing, so a caller asking for one is asking for the feature to be off rather than configured", ErrInvariant, window)
	}
	if breadth <= 0 {
		return nil, fmt.Errorf("%w: a register that may name %d channels can never answer with a difference", ErrInvariant, breadth)
	}
	return &Resumptions{
		clock:   clock,
		window:  window,
		breadth: breadth,
		held:    map[string]*suspension{},
	}, nil
}

// Revision returns the highest revision this register has published.
func (r *Resumptions) Revision() uint64 { return r.revision }

// Published records that the server sent a batch of change touching channels,
// and returns the revision that batch carries.
//
// A caller calls it once per flush, with the channels that flush moved, and
// sends the returned number alongside the message. That number is what a client
// echoes back in a ticket, which is what makes Folded a number the server
// issued rather than one the client invented.
//
// A batch touching no channel is not a batch. It leaves the revision where it
// is and records nothing, so a caller that flushes an empty interval does not
// walk every suspension away from the state its client actually has.
func (r *Resumptions) Published(channels ...ID) uint64 {
	if len(channels) == 0 {
		return r.revision
	}
	r.revision++
	for _, s := range r.held {
		// A suspension that has already given up records nothing more. Reading
		// a nil set is fine and writing to one panics, so the day this line
		// goes the next batch after an overflow takes the process down.
		if s.missed == nil {
			continue
		}
		for _, channel := range channels {
			s.missed[channel] = true
		}
		if len(s.missed) > r.breadth {
			s.missed = nil
		}
	}
	return r.revision
}

// Suspend records that a client's connection went and starts its window.
//
// It touches no occupancy and emits no event, which is the whole of "invisible
// to other members": the member stays where they were, the participant list
// does not move, and nothing is published. What ends the silence is Reap.
func (r *Resumptions) Suspend(session string, member ID) error {
	if session == "" {
		return fmt.Errorf("%w: a suspension is keyed on a session and this one names none", ErrInvariant)
	}
	if member.IsZero() {
		return fmt.Errorf("%w: a suspension needs the member it is holding a place for", ErrInvariant)
	}
	if _, taken := r.held[session]; taken {
		return fmt.Errorf("%w: the session %s is already suspended", ErrInvariant, session)
	}
	r.held[session] = &suspension{
		member: member,
		grace:  NewGrace(r.clock, r.window),
		from:   r.revision,
		missed: map[ID]bool{},
	}
	r.order = append(r.order, session)
	return nil
}

// Held reports whether a session is suspended. Nothing about the suspension is
// returned, because a caller that needs its contents is resuming it.
func (r *Resumptions) Held(session string) bool {
	_, ok := r.held[session]
	return ok
}

// Resume answers a ticket presented on a connection authenticated as member.
//
// The four refusals are the closed set the file comment names. A ticket that
// passes all four is accepted, the suspension is forgotten because the client
// is back, and the answer is a difference or a full transfer.
func (r *Resumptions) Resume(t ResumeTicket, member ID) (ResumeAnswer, error) {
	s, ok := r.held[t.Session]
	if !ok {
		return ResumeAnswer{}, fmt.Errorf("%w: %s", ErrNoSuspension, t.Session)
	}
	if s.grace.Expired() {
		return ResumeAnswer{}, fmt.Errorf("%w: %s has been gone longer than %v", ErrResumeWindowPassed, s.member, r.window)
	}
	if s.member != member {
		return ResumeAnswer{}, fmt.Errorf("%w: the session is held for %s and the connection authenticated as %s", ErrResumeNotYours, s.member, member)
	}
	if t.Folded > r.revision {
		return ResumeAnswer{}, fmt.Errorf("%w: it folds %d and the highest published is %d", ErrResumeAhead, t.Folded, r.revision)
	}

	answer := ResumeAnswer{Member: s.member}
	switch {
	case s.missed == nil, t.Folded < s.from:
		answer.Full = true
	default:
		answer.Missed = sortedIDs(s.missed)
	}
	r.forget(t.Session)
	return answer, nil
}

// Reap forgets every suspension whose window has run out and returns the
// members they were holding a place for, in the order they suspended.
//
// Those are the members whose occupancy the caller now releases, and each one
// is exactly one leave. Reaping is what turns the silence Suspend started into
// an event, and it is a call rather than a timer so a test moves the clock and
// asks rather than waiting.
func (r *Resumptions) Reap() []ID {
	var gone []ID
	for _, session := range append([]string(nil), r.order...) {
		s := r.held[session]
		if !s.grace.Expired() {
			continue
		}
		gone = append(gone, s.member)
		r.forget(session)
	}
	return gone
}

// forget removes a suspension from both the map and the order.
func (r *Resumptions) forget(session string) {
	delete(r.held, session)
	for i, held := range r.order {
		if held == session {
			r.order = append(r.order[:i], r.order[i+1:]...)
			break
		}
	}
}

// sortedIDs puts a set of identifiers in identifier order, so an answer is a
// value a test can compare rather than a set whose order is the runtime's.
func sortedIDs(set map[ID]bool) []ID {
	out := make([]ID, 0, len(set))
	for id := range set {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].String() < out[j].String() })
	return out
}
