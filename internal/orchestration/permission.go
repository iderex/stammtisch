// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026  Nils Lehnen

package orchestration

import "fmt"

// The permission model. One place, total, and the same for a person and a bot.
//
// Permission questions decided where they happen to come up are how a
// moderation bypass ships: every site is correct on the day it is written and
// one of them is not updated on the day a rule changes. So there is exactly one
// function here that answers a permission question, Allow, and
// TestOnlyOnePlaceDecidesAPermission refuses the vocabulary a second decision
// site would have to use.
//
// A bot is a principal holding grants. It is not a kind that is checked
// specially, and there is no branch anywhere below on which kind a principal
// is: the kind is unexported, has no accessor, and Allow never reads it. That
// is why "evaluated identically" is a fact about the code rather than a
// promise about it.

// Permission is one thing a principal may be allowed to do.
//
// The set is small on purpose and is meant to stay small. Every permission is a
// row in a matrix that has to be covered against every principal kind and every
// subject state, so the cost of one more is paid in the suite as well as here.
type Permission int

// The permissions this project needs. The zero value is deliberately not one of
// them: a Permission nobody set is not a permission that happens to be readable
// as "see a channel".
const (
	// SeeChannel is the channel existing for you at all. It is the one the
	// presence decision separated out, and it gates every other permission
	// on a channel. docs/decisions/presence-model.md calls it view.
	SeeChannel Permission = iota + 1
	// JoinChannel is entering it.
	JoinChannel
	// SpeakInChannel is publishing audio into it.
	SpeakInChannel
	// MuteMember is the server-side mute, which is #39 and is a moderation
	// action rather than the comfort control in
	// docs/decisions/per-person-volume.md.
	MuteMember
	// MoveMember is moving somebody out of a channel.
	MoveMember
	// ManageChannel is changing the channel itself.
	ManageChannel
	// ManageSpace is changing the space, and it is the one permission whose
	// subject is a space rather than a channel.
	ManageSpace
)

// Permissions returns every permission this model declares, in declaration
// order.
//
// It is the list the matrix suite in #37 generates its cases from, so a
// permission added to the constants above and left out of here is a permission
// the suite would never test. TestPermissionsMatchesTheDeclaredConstants reads
// the constants out of this file and refuses that.
func Permissions() []Permission {
	return []Permission{
		SeeChannel,
		JoinChannel,
		SpeakInChannel,
		MuteMember,
		MoveMember,
		ManageChannel,
		ManageSpace,
	}
}

// String names the permission. An undeclared value says so and carries its
// number, because a refusal a reader cannot place is a refusal they will work
// around.
func (p Permission) String() string {
	switch p {
	case SeeChannel:
		return "see-channel"
	case JoinChannel:
		return "join-channel"
	case SpeakInChannel:
		return "speak-in-channel"
	case MuteMember:
		return "mute-member"
	case MoveMember:
		return "move-member"
	case ManageChannel:
		return "manage-channel"
	case ManageSpace:
		return "manage-space"
	default:
		return fmt.Sprintf("undeclared-permission(%d)", int(p))
	}
}

// principalKind is person or bot. It is unexported, it has no accessor, and
// nothing reads it. Its only purpose is to make two principals with the same
// identifier distinguishable to a store, and it is here rather than absent so
// that the claim "a bot is evaluated by the same function as a person" is made
// about a model that can actually tell them apart.
type principalKind int

const (
	kindPerson principalKind = iota + 1
	kindBot
)

// Principal is whoever is asking. A person and a bot are both one.
type Principal struct {
	id   ID
	kind principalKind
}

// Person returns a principal for a human member.
func Person(id ID) Principal { return Principal{id: id, kind: kindPerson} }

// Bot returns a principal for a bot. It is the same type, carries the same
// grants and goes through the same function.
func Bot(id ID) Principal { return Principal{id: id, kind: kindBot} }

// ID returns the principal's identifier. There is no accessor for the kind,
// which is what stops a caller writing the special case this model exists to
// prevent.
func (p Principal) ID() ID { return p.id }

// Subject is what a permission question is about: a channel in a space, or the
// space itself.
//
// Both are one type because the answer to "may this principal do this here" has
// to be one function, and a second type would be a second function with a
// second set of rules to keep in step.
type Subject struct {
	space   ID
	channel ID
}

// SpaceSubject is a question about a space. Its channel is the zero ID, which
// is not a valid identifier, so it cannot be mistaken for a question about a
// channel.
func SpaceSubject(space ID) Subject { return Subject{space: space} }

// ChannelSubject is a question about one channel in one space.
func ChannelSubject(space, channel ID) Subject {
	return Subject{space: space, channel: channel}
}

// Space returns the space the question is about.
func (s Subject) Space() ID { return s.space }

// Channel returns the channel the question is about, and whether it is about
// one at all.
func (s Subject) Channel() (ID, bool) { return s.channel, !s.channel.IsZero() }

// PermissionSet is what a principal has been granted on one subject.
//
// Membership is unexported on purpose. A caller that could ask a set whether it
// holds a permission would be a second place a permission is decided, and this
// type would be the thing that let it happen quietly.
type PermissionSet struct {
	held map[Permission]struct{}
}

// NewPermissionSet returns the set holding exactly these permissions. A value
// that is not a declared permission is dropped rather than stored, so a grant
// nobody can name cannot later be matched by an undeclared question.
func NewPermissionSet(granted ...Permission) PermissionSet {
	held := make(map[Permission]struct{}, len(granted))
	for _, p := range granted {
		if !declared(p) {
			continue
		}
		held[p] = struct{}{}
	}
	return PermissionSet{held: held}
}

// has reports membership. Unexported, and the guard test refuses a call to it
// from any file but this one.
func (s PermissionSet) has(p Permission) bool {
	if s.held == nil {
		return false
	}
	_, ok := s.held[p]
	return ok
}

// declared reports whether a value is one of the constants above.
func declared(p Permission) bool {
	for _, known := range Permissions() {
		if p == known {
			return true
		}
	}
	return false
}

// Grantor answers what a principal has been granted on a subject. Where those
// grants are stored, and how they are administered, is #27 and is not decided
// here.
//
// A Grantor that knows nothing about a principal returns the empty set. It does
// not report an error, because "I do not know" and "nothing is granted" have to
// be the same answer at a permission boundary: a lookup failure that reached
// the caller as an error would eventually be handled by somebody as a reason to
// carry on.
type Grantor interface {
	Granted(p Principal, s Subject) PermissionSet
}

// Allow is the only function in this package that answers a permission
// question. Everything that needs a decision calls it and nothing reimplements
// what it does.
//
// It is total in the sense #29 asks for: every declared permission is named in
// the switch below and reaches an explicit answer, and the default arm is a
// refusal rather than a fallthrough that inherits whatever the last case
// decided. A permission this model does not declare is refused, so a constant
// added above and forgotten here fails closed. It fails loudly as well, because
// TestEveryDeclaredPermissionIsNamedInAllow reds when it happens.
//
// Two structural rules hold across the whole model, and both come out of
// decision records rather than out of convenience:
//
// A permission whose subject is a channel is refused when the question names no
// channel, and ManageSpace is refused when it names one. A question asked about
// the wrong shape of subject has no correct answer, and the shape that guesses
// is the one that eventually answers a channel question with a space-wide
// grant.
//
// Nothing on a channel is allowed to a principal who cannot see the channel.
// docs/decisions/presence-model.md settles that a channel you cannot view shows
// nothing including its existence, and a principal who could join, speak in or
// moderate a channel they cannot see would have learned it exists by trying.
//
// No other implication is built in. Whether ManageSpace should carry the
// channel permissions inside that space is a policy question no record on this
// board has answered, so a space manager here holds exactly what they were
// granted and nothing more. That is stated rather than hidden: it is a live
// question and not a settled design.
func Allow(g Grantor, p Principal, perm Permission, s Subject) bool {
	if g == nil {
		return false
	}
	if p.id.IsZero() || s.space.IsZero() {
		return false
	}

	_, aboutChannel := s.Channel()

	switch perm {
	case ManageSpace:
		if aboutChannel {
			return false
		}
		return g.Granted(p, s).has(ManageSpace)

	case SeeChannel:
		if !aboutChannel {
			return false
		}
		return g.Granted(p, s).has(SeeChannel)

	case JoinChannel, SpeakInChannel, MuteMember, MoveMember, ManageChannel:
		if !aboutChannel {
			return false
		}
		held := g.Granted(p, s)
		if !held.has(SeeChannel) {
			return false
		}
		return held.has(perm)

	default:
		return false
	}
}
