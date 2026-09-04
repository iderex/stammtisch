// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026  Nils Lehnen

package media

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Clock is the only way an implementation of Port learns what time it is.
//
// It is declared here rather than imported so that this package depends on
// nothing else in the module, which is what lets the orchestration layer hold a
// port without the import graph guard in that layer having anything to refuse.
// Anything satisfying orchestration's Clock satisfies this one, so a server
// builds one implementation and hands it to both.
type Clock interface {
	Now() time.Time
}

// A RoomID names one live session. It is the room from
// docs/decisions/channel-and-room-model.md, which is the only thing this port
// creates: a channel is durable and the port never sees one.
type RoomID string

// A MemberID names one person or bot inside a room. Which of the two it is,
// this port cannot ask and does not need to know.
type MemberID string

// A SourceKind is what a member is sending. The port names a source by member
// and kind, and never by a track identifier, because a track identifier is the
// engine's vocabulary and the specification puts it under the port.
type SourceKind uint8

const (
	// Audio is a member's voice.
	Audio SourceKind = iota + 1
	// Video is a member's camera or screen.
	Video
)

// String returns the name a report writes, and the empty string for a kind this
// build does not define. Every operation taking a SourceKind refuses that case,
// so the empty string never reaches an implementation.
func (k SourceKind) String() string {
	switch k {
	case Audio:
		return "audio"
	case Video:
		return "video"
	}
	return ""
}

// A Source is one member's stream of one kind. Subscription is per pair,
// subscriber and source, which is what makes per-person gain a question this
// port can ask at all.
type Source struct {
	Member MemberID
	Kind   SourceKind
}

// A Capability is one thing a member may do with media. It is a small closed
// set of this port's own rather than the orchestration layer's permission
// model, and that is deliberate: a port that took the domain's permission type
// would make the media plane depend on the domain, and the mapping between the
// two is exactly where ErrPermissionsNotExpressible is raised.
type Capability uint8

const (
	// MayPublishAudio lets the member send voice.
	MayPublishAudio Capability = 1 << iota
	// MayPublishVideo lets the member send video.
	MayPublishVideo
	// MaySubscribe lets the member receive what others send.
	MaySubscribe
)

// A Capabilities is a set of them. The zero value is a member who may receive
// nothing and send nothing, which is the direction an admission with a
// forgotten argument should fail in.
type Capabilities uint8

// Allows reports whether the set carries c.
func (s Capabilities) Allows(c Capability) bool { return s&Capabilities(c) != 0 }

// With returns the set with c added.
func (s Capabilities) With(c Capability) Capabilities { return s | Capabilities(c) }

// A Credential is what a unit requires a client to present, and it is opaque
// above this port. Nothing above the port parses one, reads an expiry out of
// one, or constructs one.
//
// NOT ENFORCED for the constructing half, and no check refuses it today. The
// value has one unexported field, so nothing outside this package can build one
// by writing a literal, and NewCredential exists because an implementation in a
// package beside this one has to mint them. Go has no visibility that admits an
// implementation and refuses a caller, so the same constructor is reachable from
// above. What would refuse it is a pattern rule keyed on a path prefix, which is
// what `.github/workflows/invariants.yml` is for and which is outside the
// `Scope:` issue #36 declares.
type Credential struct {
	opaque string
}

// NewCredential is how an implementation of Port mints one. See the type's own
// comment for what it does not stop.
func NewCredential(opaque string) Credential { return Credential{opaque: opaque} }

// String returns the credential as the client will present it. It is the one
// thing a caller may do with one: pass it on.
func (c Credential) String() string { return c.opaque }

// IsZero reports whether this is the credential no admission produced.
func (c Credential) IsZero() bool { return c.opaque == "" }

// A Gain is a subscriber's volume for one source, where 1 is unchanged. It is a
// ratio rather than a decibel figure because
// docs/decisions/per-person-volume.md fixes the scale above this port and the
// port carries the number rather than re-deciding it.
type Gain float64

// A Speaking is what the unit currently believes about one member.
type Speaking struct {
	Member   MemberID
	Speaking bool
}

// A Hop is what the unit observes on one member's leg.
//
// The clock is carried with the figures rather than assumed, because a delay
// with an unstated clock is not a figure that can be held against a budget
// line. It is the moment the unit made the observation, on the unit's own
// clock, and a caller comparing two hops from two units is comparing two
// clocks.
type Hop struct {
	Member   MemberID
	Delay    time.Duration
	Loss     float64
	Observed time.Time
}

// The three errors every operation can return. They are common rather than
// listed per operation, which is what the specification does.
var (
	// ErrUnavailable means the unit could not be reached at all, and the caller
	// learns nothing about whether the work happened. The orchestration layer's
	// answer to it is already written: the channel stays, the join fails, and
	// the failure says the media plane is unavailable rather than saying the
	// channel does not exist.
	ErrUnavailable = errors.New("media: the unit could not be reached")

	// ErrCancelled means the caller's own deadline expired.
	ErrCancelled = errors.New("media: the caller's deadline expired")

	// ErrInternal means the unit answered with something this port has no
	// vocabulary for. Every time it is returned it means the record this port
	// is declared from was incomplete, rather than that the unit misbehaved.
	ErrInternal = errors.New("media: the unit answered outside this port's vocabulary")
)

// The errors an operation may return beyond the three above. An error not in an
// operation's list cannot come out of that operation, and an implementation
// that needs one that is not here is a change to
// docs/decisions/media-plane-port.md rather than a detail of a binding.
var (
	// ErrRoomLimitReached is CreateRoom's, and the limit is the unit's.
	ErrRoomLimitReached = errors.New("media: the unit will hold no further room")

	// ErrInvalidIdentity is for an identity this port will not name a room or a
	// member by. It is the empty one, and a kind this build does not define.
	ErrInvalidIdentity = errors.New("media: that is not an identity this port names anything by")

	// ErrNoSuchRoom is for an operation on a room that does not exist. Note that
	// DestroyRoom does not return it: destroying a room that is already gone is
	// the caller's intent already satisfied.
	ErrNoSuchRoom = errors.New("media: there is no such room")

	// ErrMemberAlreadyAdmitted is AdmitMember's. Admission is not idempotent,
	// because a second admission would mint a second credential and the first
	// one's fate would be unstated.
	ErrMemberAlreadyAdmitted = errors.New("media: that member is already admitted to that room")

	// ErrPermissionsNotExpressible is how a unit says a capability the caller
	// wants is not something it can enforce. The caller's answer is to refuse
	// the admission rather than to admit with a weaker set: a permission
	// silently downgraded here is a permission the layer above believes it has.
	ErrPermissionsNotExpressible = errors.New("media: the unit cannot enforce those capabilities")

	// ErrNoSuchMember is for a member who is not admitted to the room.
	ErrNoSuchMember = errors.New("media: there is no such member in that room")

	// ErrNotPermitted is for an operation the member's capabilities do not
	// allow.
	ErrNotPermitted = errors.New("media: that member's capabilities do not allow it")

	// ErrSourceAlreadyPublished is PublishSource's. Unpublishing is idempotent
	// and publishing is not, for AdmitMember's reason one level down.
	ErrSourceAlreadyPublished = errors.New("media: that source is already published")

	// ErrNoSuchSource is for a subscription to a source nobody is publishing.
	ErrNoSuchSource = errors.New("media: there is no such source in that room")

	// ErrGainNotSupported is not a failure. It is the unit reporting a
	// capability, and it is the expected answer from a forwarding unit, because
	// docs/decisions/per-person-volume.md puts the gain at the client. The
	// operation exists so that a unit which can do it server side needs no new
	// port, and so that the answer is asked for rather than assumed.
	ErrGainNotSupported = errors.New("media: the unit does not apply gain per subscriber")

	// ErrNoSuchSubscription is SetSubscriberGain's.
	ErrNoSuchSubscription = errors.New("media: there is no such subscription")

	// ErrGainOutOfRange is for a gain the unit will not apply.
	ErrGainOutOfRange = errors.New("media: that gain is outside what the unit will apply")

	// ErrNotObservable is how a unit says it does not report speaking state or
	// hop metrics at all. It is a capability answer of the same kind as
	// ErrGainNotSupported rather than a fault.
	ErrNotObservable = errors.New("media: the unit does not report that")

	// ErrDrainDeadlineExceeded is DrainRoom's, and it means members were still
	// in the room when the caller's deadline passed. The room is still draining.
	ErrDrainDeadlineExceeded = errors.New("media: the room did not empty before the deadline")
)

// A Port is the media plane, and the engine underneath is reachable no other
// way. It is specified in docs/decisions/media-plane-port.md, which fixes each
// operation's preconditions, postconditions and error set, and this declaration
// carries that specification rather than restating it.
//
// Two things this port deliberately cannot express, because both are load
// bearing elsewhere. There is no operation that decodes, mixes, transcodes,
// records or reads a payload, and the property that the server never looks
// inside one is what makes the architecture cheap. And there is no codec or
// bitrate: those live inside the unit, docs/decisions/codec-and-bitrate.md
// fixes them, and a layer that could set a codec is a layer that has to know
// which codecs the unit has.
//
// Every operation takes a context. That is where the caller's deadline lives,
// and ErrCancelled is what an expired one comes back as.
type Port interface {
	// CreateRoom makes a room addressable by the identity passed. It is
	// idempotent: a room that already exists is returned rather than refused,
	// because the alternative is every caller racing against its own retry.
	//
	// Errors: ErrRoomLimitReached, ErrInvalidIdentity.
	CreateRoom(ctx context.Context, room RoomID) error

	// DestroyRoom removes the room and disconnects every member of it. It is
	// idempotent, and alreadyGone says which of the two happened, because the
	// caller's intent is a state and not an event.
	//
	// Errors: ErrInvalidIdentity.
	DestroyRoom(ctx context.Context, room RoomID) (alreadyGone bool, err error)

	// AdmitMember prepares the unit to accept a connection from member with
	// exactly caps, and returns the credential the client presents.
	//
	// Errors: ErrNoSuchRoom, ErrMemberAlreadyAdmitted,
	// ErrPermissionsNotExpressible, ErrInvalidIdentity.
	AdmitMember(ctx context.Context, room RoomID, member MemberID, caps Capabilities) (Credential, error)

	// RevokeMember takes the member out of the room, stops the credential
	// minted for them from admitting anybody, and disconnects them if they were
	// connected. Idempotent for a member who was never admitted.
	//
	// Errors: ErrNoSuchRoom.
	RevokeMember(ctx context.Context, room RoomID, member MemberID) error

	// PublishSource puts the member's source of that kind into the room.
	//
	// Errors: ErrNoSuchRoom, ErrNoSuchMember, ErrNotPermitted,
	// ErrSourceAlreadyPublished, ErrInvalidIdentity.
	PublishSource(ctx context.Context, room RoomID, member MemberID, kind SourceKind) error

	// UnpublishSource takes it out again, and is idempotent.
	//
	// Errors: ErrNoSuchRoom, ErrNoSuchMember, ErrInvalidIdentity.
	UnpublishSource(ctx context.Context, room RoomID, member MemberID, kind SourceKind) error

	// Subscribe makes the subscriber receive that source.
	//
	// Errors: ErrNoSuchRoom, ErrNoSuchMember, ErrNoSuchSource,
	// ErrNotPermitted, ErrInvalidIdentity.
	Subscribe(ctx context.Context, room RoomID, subscriber MemberID, source Source) error

	// Unsubscribe stops that, and is idempotent.
	//
	// Errors: ErrNoSuchRoom, ErrNoSuchMember, ErrInvalidIdentity.
	Unsubscribe(ctx context.Context, room RoomID, subscriber MemberID, source Source) error

	// SetSubscriberGain asks the unit to apply gain to one subscriber's copy of
	// one source. Either it is applied or ErrGainNotSupported comes back and
	// nothing was applied. There is no third outcome.
	//
	// Errors: ErrGainNotSupported, ErrNoSuchSubscription, ErrGainOutOfRange.
	SetSubscriberGain(ctx context.Context, room RoomID, subscriber MemberID, source Source, gain Gain) error

	// SpeakingState reports, per member, whether the unit believes that member
	// is speaking. It changes nothing.
	//
	// Errors: ErrNoSuchRoom, ErrNotObservable.
	SpeakingState(ctx context.Context, room RoomID) ([]Speaking, error)

	// HopMetrics reports, per member, the delay and loss the unit observes on
	// that member's hop, and the clock it measured them against.
	//
	// Errors: ErrNoSuchRoom, ErrNotObservable.
	HopMetrics(ctx context.Context, room RoomID) ([]Hop, error)

	// DrainRoom closes a room gracefully: no new admission succeeds, existing
	// members are told it is closing, and it returns once they have left or the
	// caller's deadline expires. DestroyRoom is the abrupt path, and both exist
	// because a shutdown with only the abrupt one drops conversations.
	//
	// Errors: ErrNoSuchRoom, ErrDrainDeadlineExceeded.
	DrainRoom(ctx context.Context, room RoomID) error
}

// Operations is every method name on Port, in the order the specification lists
// them.
//
// It is here rather than in a test because it is what an implementation is
// judged against, and a list living in one implementation's suite would be a
// list the next implementation does not meet. TestOperationsIsThePortsOwnMethodSet
// refuses it drifting from the interface.
func Operations() []string {
	return []string{
		"CreateRoom",
		"DestroyRoom",
		"AdmitMember",
		"RevokeMember",
		"PublishSource",
		"UnpublishSource",
		"Subscribe",
		"Unsubscribe",
		"SetSubscriberGain",
		"SpeakingState",
		"HopMetrics",
		"DrainRoom",
	}
}

// ValidRoom refuses a room identity this port will not name a room by, and is
// exported because every implementation owes the same refusal and two copies of
// it would be two answers.
func ValidRoom(room RoomID) error {
	if room == "" {
		return fmt.Errorf("%w: a room identity is empty", ErrInvalidIdentity)
	}
	return nil
}

// ValidMember is the same for a member identity.
func ValidMember(member MemberID) error {
	if member == "" {
		return fmt.Errorf("%w: a member identity is empty", ErrInvalidIdentity)
	}
	return nil
}

// ValidKind refuses a source kind this build does not define, so a value
// arriving from a conversion or from a constant nobody handled is refused
// rather than treated as audio.
func ValidKind(kind SourceKind) error {
	if kind.String() == "" {
		return fmt.Errorf("%w: source kind %d is not in this build's set", ErrInvalidIdentity, uint8(kind))
	}
	return nil
}
