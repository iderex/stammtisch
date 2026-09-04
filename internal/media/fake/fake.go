// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026  Nils Lehnen

// Package fake is the in-memory media plane, and the whole discipline of this
// board rests on it.
//
// If the fake is weak the unit suite proves nothing and the tests that matter
// migrate into a hardware-gated harness where they run rarely and slowly. So it
// implements the port completely, it needs no network, no device and no
// elevation, and it can be told to misbehave in the specific ways a real unit
// does. Those paths are the ones that cannot be reached against a real unit on
// demand, and they are exactly where the orchestration bugs are.
//
// # It records what it was asked to do
//
// Every operation appends a Call before it returns, whatever it returns. A test
// asserts on the sequence rather than on an outcome that could have been reached
// another way, which is the difference between proving that a switch was one
// transition and proving that the member ended up somewhere.
//
// The recording is complete by construction and by a test. Every operation goes
// through record, and TestEveryPortOperationIsImplementedAndRecorded drives all
// twelve and compares what came back against media.Operations(), so an operation
// added without its recording reds rather than going quiet.
//
// # What it is not
//
// It is not a simulation. It returns what a test set for speaking state and hop
// metrics, because the alternative is a fake that models a network, which is a
// second media plane nobody will maintain. It returns instantly on a fake clock,
// and nothing in the orchestration layer may depend on how long an operation
// takes: a test that passes only because this is instant is asserting something
// the port does not promise.
//
// It is not proof that the engine is replaceable either. It never has to make a
// real engine's API fit, which is where the tendrils would come from.
package fake

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"sync"

	"github.com/iderex/stammtisch/internal/media"
)

// Fake is a media plane in memory. It is safe for concurrent use, because the
// orchestration layer above it is not required to hold a lock to talk to a
// media unit and a fake that demanded one would be testing the wrong contract.
var _ media.Port = (*Fake)(nil)

// A Call is one operation as it was asked for and as it answered.
//
// The error is kept as well as the arguments, because a sequence of calls that
// does not say which of them failed is a sequence a test can misread in the one
// direction that matters: a caller that carried on after a refusal looks the
// same as one that never got it.
type Call struct {
	Op     string
	Room   media.RoomID
	Member media.MemberID
	Source media.Source
	Gain   media.Gain
	Caps   media.Capabilities
	Err    error
}

// String is what a failing assertion prints. It names the operation and its
// subject and stops there, because a test comparing sequences compares the
// values rather than this.
func (c Call) String() string {
	s := c.Op + "(" + string(c.Room)
	if c.Member != "" {
		s += ", " + string(c.Member)
	}
	if c.Source.Kind != 0 {
		s += ", " + string(c.Source.Member) + "/" + c.Source.Kind.String()
	}
	s += ")"
	if c.Err != nil {
		s += " -> " + c.Err.Error()
	}
	return s
}

// A source is one member's stream inside a room.
type room struct {
	members     map[media.MemberID]media.Capabilities
	sources     map[media.Source]bool
	subscribers map[subscription]bool
	speaking    map[media.MemberID]bool
	hops        map[media.MemberID]media.Hop
	draining    bool
}

// A subscription is the pair the port names: one subscriber and one source.
type subscription struct {
	subscriber media.MemberID
	source     media.Source
}

// A seat is one member in one room, which is what an admission is about.
type seat struct {
	room   media.RoomID
	member media.MemberID
}

// Fake is the in-memory port. Build one with New.
type Fake struct {
	mu    sync.Mutex
	clock media.Clock
	calls []Call
	rooms map[media.RoomID]*room

	minted int

	// What a test set. Each of these is a capability answer or a limit rather
	// than a fault; the faults are below.
	roomLimit     int
	gainSupported bool
	gainRange     [2]media.Gain
	observable    bool

	// The misbehaviours, each inducible from a test.
	unreachable        bool
	admissionTimesOut  map[seat]bool
	roomCreationFails  map[media.RoomID]bool
	sourceNeverArives  map[media.Source]bool
	capsNotExpressible map[media.Capability]bool
}

// New returns a fake driven by clock.
//
// It refuses a nil clock rather than reading the operating system's, because a
// fake that quietly used real time is a fake whose users' tests sleep.
func New(clock media.Clock) (*Fake, error) {
	if clock == nil {
		return nil, fmt.Errorf("fake: there is no clock, and a media plane that read the real one would make every test above it wait")
	}
	return &Fake{
		clock:              clock,
		rooms:              map[media.RoomID]*room{},
		roomLimit:          -1,
		gainRange:          [2]media.Gain{0, 4},
		observable:         true,
		admissionTimesOut:  map[seat]bool{},
		roomCreationFails:  map[media.RoomID]bool{},
		sourceNeverArives:  map[media.Source]bool{},
		capsNotExpressible: map[media.Capability]bool{},
	}, nil
}

// ---- what a test reads -----------------------------------------------------

// Calls is the sequence of operations this fake was asked for, in order.
func (f *Fake) Calls() []Call {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]Call, len(f.calls))
	copy(out, f.calls)
	return out
}

// Operations is the names of the calls in order, which is what a test asserting
// on a sequence usually wants.
func (f *Fake) Operations() []string {
	out := []string{}
	for _, c := range f.Calls() {
		out = append(out, c.Op)
	}
	return out
}

// Subscribed reports whether the subscriber is receiving that source right now.
// It is a read of the state rather than of the call log, so a test can tell a
// subscription that was made and then dropped from one that was never made.
func (f *Fake) Subscribed(id media.RoomID, subscriber media.MemberID, source media.Source) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.rooms[id]
	return ok && r.subscribers[subscription{subscriber, source}]
}

// Published reports whether that source is present in the room.
func (f *Fake) Published(id media.RoomID, source media.Source) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.rooms[id]
	return ok && r.sources[source]
}

// Admitted reports whether the member is in the room.
func (f *Fake) Admitted(id media.RoomID, member media.MemberID) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.rooms[id]
	if !ok {
		return false
	}
	_, in := r.members[member]
	return in
}

// ---- what a test sets ------------------------------------------------------

// SetRoomLimit caps how many rooms this fake will hold. A negative limit is no
// limit, which is the state a new fake is in.
func (f *Fake) SetRoomLimit(n int) { f.set(func() { f.roomLimit = n }) }

// SetGainSupported says whether the unit applies gain per subscriber. A new
// fake says it does not, because that is the answer a forwarding unit gives and
// a fake defaulting to the rarer answer would leave the common path untested.
func (f *Fake) SetGainSupported(ok bool) { f.set(func() { f.gainSupported = ok }) }

// SetGainRange fixes what SetSubscriberGain will accept, inclusive.
func (f *Fake) SetGainRange(low, high media.Gain) {
	f.set(func() { f.gainRange = [2]media.Gain{low, high} })
}

// SetObservable says whether the unit reports speaking state and hop metrics at
// all.
func (f *Fake) SetObservable(ok bool) { f.set(func() { f.observable = ok }) }

// SetSpeaking is what SpeakingState will report for that member.
func (f *Fake) SetSpeaking(id media.RoomID, member media.MemberID, speaking bool) {
	f.set(func() {
		if r, ok := f.rooms[id]; ok {
			r.speaking[member] = speaking
		}
	})
}

// SetHop is what HopMetrics will report for that member. The observation moment
// is stamped from the injected clock rather than taken from the caller, so a
// test that forgot to advance the clock sees the clock it has rather than a time
// it typed.
func (f *Fake) SetHop(id media.RoomID, hop media.Hop) {
	f.set(func() {
		if r, ok := f.rooms[id]; ok {
			hop.Observed = f.clock.Now()
			r.hops[hop.Member] = hop
		}
	})
}

// ---- the misbehaviours -----------------------------------------------------

// SetUnreachable makes the unit unreachable, or reachable again. Every
// operation answers media.ErrUnavailable while it is set, and the state the fake
// holds is untouched, which is what makes a unit that comes back a unit with its
// rooms still in it.
func (f *Fake) SetUnreachable(unreachable bool) { f.set(func() { f.unreachable = unreachable }) }

// FailRoomCreation makes CreateRoom refuse that room with
// media.ErrRoomLimitReached until it is called again with false.
func (f *Fake) FailRoomCreation(id media.RoomID, fail bool) {
	f.set(func() { f.roomCreationFails[id] = fail })
}

// TimeOutAdmission makes AdmitMember answer media.ErrCancelled for that member
// in that room. It is the admission that hangs and then gives up, without
// anything hanging.
func (f *Fake) TimeOutAdmission(id media.RoomID, member media.MemberID, timeOut bool) {
	f.set(func() { f.admissionTimesOut[seat{id, member}] = timeOut })
}

// NeverDeliverSource makes PublishSource succeed and the source never appear.
// That is the track that never arrives, and it is the shape that matters: the
// unit said yes and nothing came, so a caller that trusted the answer is
// subscribing to something that is not there.
func (f *Fake) NeverDeliverSource(source media.Source, never bool) {
	f.set(func() { f.sourceNeverArives[source] = never })
}

// DropSubscription removes a subscription the unit had made, without telling
// anybody. A caller learns about it from the next operation that needs it.
func (f *Fake) DropSubscription(id media.RoomID, subscriber media.MemberID, source media.Source) {
	f.set(func() {
		if r, ok := f.rooms[id]; ok {
			delete(r.subscribers, subscription{subscriber, source})
		}
	})
}

// RefuseCapability makes AdmitMember answer media.ErrPermissionsNotExpressible
// for any capability set carrying c. It is how a unit says a permission the
// layer above wants is not one it can enforce.
func (f *Fake) RefuseCapability(c media.Capability, refuse bool) {
	f.set(func() { f.capsNotExpressible[c] = refuse })
}

// set runs fn under the lock. Every setter goes through it so that a test
// arranging a fault while another goroutine is calling the port is a race the
// fake handles rather than one it reports.
func (f *Fake) set(fn func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn()
}

// ---- the port --------------------------------------------------------------

// record appends the call and returns its error, so every operation ends in one
// statement and none of them can return without being recorded.
func (f *Fake) record(c Call) error {
	f.calls = append(f.calls, c)
	return c.Err
}

// begin takes the lock and answers the two things every operation asks first:
// whether the caller's deadline has already gone, and whether the unit is
// reachable. It returns the error to record, or nil.
//
// The caller holds the lock when this returns either way. Unlocking is the
// operation's own deferred job, which is why this does not do it.
func (f *Fake) begin(ctx context.Context) error {
	f.mu.Lock()
	if ctx.Err() != nil {
		return media.ErrCancelled
	}
	if f.unreachable {
		return media.ErrUnavailable
	}
	return nil
}

// CreateRoom implements media.Port.
func (f *Fake) CreateRoom(ctx context.Context, id media.RoomID) error {
	if err := f.begin(ctx); err != nil {
		defer f.mu.Unlock()
		return f.record(Call{Op: "CreateRoom", Room: id, Err: err})
	}
	defer f.mu.Unlock()

	if err := media.ValidRoom(id); err != nil {
		return f.record(Call{Op: "CreateRoom", Room: id, Err: err})
	}
	if _, exists := f.rooms[id]; exists {
		// Idempotent: the existing room is the answer, not a refusal.
		return f.record(Call{Op: "CreateRoom", Room: id})
	}
	if f.roomCreationFails[id] {
		return f.record(Call{Op: "CreateRoom", Room: id, Err: media.ErrRoomLimitReached})
	}
	if f.roomLimit >= 0 && len(f.rooms) >= f.roomLimit {
		return f.record(Call{Op: "CreateRoom", Room: id, Err: media.ErrRoomLimitReached})
	}

	f.rooms[id] = &room{
		members:     map[media.MemberID]media.Capabilities{},
		sources:     map[media.Source]bool{},
		subscribers: map[subscription]bool{},
		speaking:    map[media.MemberID]bool{},
		hops:        map[media.MemberID]media.Hop{},
	}
	return f.record(Call{Op: "CreateRoom", Room: id})
}

// DestroyRoom implements media.Port.
func (f *Fake) DestroyRoom(ctx context.Context, id media.RoomID) (bool, error) {
	if err := f.begin(ctx); err != nil {
		defer f.mu.Unlock()
		return false, f.record(Call{Op: "DestroyRoom", Room: id, Err: err})
	}
	defer f.mu.Unlock()

	if err := media.ValidRoom(id); err != nil {
		return false, f.record(Call{Op: "DestroyRoom", Room: id, Err: err})
	}
	_, exists := f.rooms[id]
	delete(f.rooms, id)
	return !exists, f.record(Call{Op: "DestroyRoom", Room: id})
}

// AdmitMember implements media.Port.
func (f *Fake) AdmitMember(ctx context.Context, id media.RoomID, member media.MemberID, caps media.Capabilities) (media.Credential, error) {
	call := Call{Op: "AdmitMember", Room: id, Member: member, Caps: caps}
	if err := f.begin(ctx); err != nil {
		defer f.mu.Unlock()
		call.Err = err
		return media.Credential{}, f.record(call)
	}
	defer f.mu.Unlock()

	if err := media.ValidRoom(id); err != nil {
		call.Err = err
		return media.Credential{}, f.record(call)
	}
	if err := media.ValidMember(member); err != nil {
		call.Err = err
		return media.Credential{}, f.record(call)
	}
	if f.admissionTimesOut[seat{id, member}] {
		call.Err = media.ErrCancelled
		return media.Credential{}, f.record(call)
	}
	r, ok := f.rooms[id]
	if !ok {
		call.Err = media.ErrNoSuchRoom
		return media.Credential{}, f.record(call)
	}
	if _, in := r.members[member]; in {
		call.Err = media.ErrMemberAlreadyAdmitted
		return media.Credential{}, f.record(call)
	}
	for c, refused := range f.capsNotExpressible {
		if refused && caps.Allows(c) {
			call.Err = media.ErrPermissionsNotExpressible
			return media.Credential{}, f.record(call)
		}
	}
	if r.draining {
		// A draining room admits nobody. The specification says no new
		// admission succeeds, and the nearest error in this operation's set is
		// that the room is not there to be joined.
		call.Err = media.ErrNoSuchRoom
		return media.Credential{}, f.record(call)
	}

	r.members[member] = caps
	f.minted++
	cred := media.NewCredential("fake-" + string(id) + "-" + string(member) + "-" + strconv.Itoa(f.minted))
	return cred, f.record(call)
}

// RevokeMember implements media.Port.
func (f *Fake) RevokeMember(ctx context.Context, id media.RoomID, member media.MemberID) error {
	call := Call{Op: "RevokeMember", Room: id, Member: member}
	if err := f.begin(ctx); err != nil {
		defer f.mu.Unlock()
		call.Err = err
		return f.record(call)
	}
	defer f.mu.Unlock()

	r, ok := f.rooms[id]
	if !ok {
		call.Err = media.ErrNoSuchRoom
		return f.record(call)
	}

	// Idempotent for a member who was never admitted, and the removals below
	// are what "a member revoked is a member absent from the next read" means:
	// their sources go and so does everything anybody had subscribed to of
	// theirs.
	delete(r.members, member)
	delete(r.speaking, member)
	delete(r.hops, member)
	for s := range r.sources {
		if s.Member == member {
			delete(r.sources, s)
		}
	}
	for sub := range r.subscribers {
		if sub.subscriber == member || sub.source.Member == member {
			delete(r.subscribers, sub)
		}
	}
	return f.record(call)
}

// PublishSource implements media.Port.
func (f *Fake) PublishSource(ctx context.Context, id media.RoomID, member media.MemberID, kind media.SourceKind) error {
	source := media.Source{Member: member, Kind: kind}
	call := Call{Op: "PublishSource", Room: id, Member: member, Source: source}
	if err := f.begin(ctx); err != nil {
		defer f.mu.Unlock()
		call.Err = err
		return f.record(call)
	}
	defer f.mu.Unlock()

	if err := media.ValidKind(kind); err != nil {
		call.Err = err
		return f.record(call)
	}
	r, caps, err := f.memberOf(id, member)
	if err != nil {
		call.Err = err
		return f.record(call)
	}
	if !caps.Allows(publishing(kind)) {
		call.Err = media.ErrNotPermitted
		return f.record(call)
	}
	if r.sources[source] {
		call.Err = media.ErrSourceAlreadyPublished
		return f.record(call)
	}
	if !f.sourceNeverArives[source] {
		r.sources[source] = true
	}
	return f.record(call)
}

// UnpublishSource implements media.Port.
func (f *Fake) UnpublishSource(ctx context.Context, id media.RoomID, member media.MemberID, kind media.SourceKind) error {
	source := media.Source{Member: member, Kind: kind}
	call := Call{Op: "UnpublishSource", Room: id, Member: member, Source: source}
	if err := f.begin(ctx); err != nil {
		defer f.mu.Unlock()
		call.Err = err
		return f.record(call)
	}
	defer f.mu.Unlock()

	if err := media.ValidKind(kind); err != nil {
		call.Err = err
		return f.record(call)
	}
	r, _, err := f.memberOf(id, member)
	if err != nil {
		call.Err = err
		return f.record(call)
	}
	delete(r.sources, source)
	for sub := range r.subscribers {
		if sub.source == source {
			delete(r.subscribers, sub)
		}
	}
	return f.record(call)
}

// Subscribe implements media.Port.
func (f *Fake) Subscribe(ctx context.Context, id media.RoomID, subscriber media.MemberID, source media.Source) error {
	call := Call{Op: "Subscribe", Room: id, Member: subscriber, Source: source}
	if err := f.begin(ctx); err != nil {
		defer f.mu.Unlock()
		call.Err = err
		return f.record(call)
	}
	defer f.mu.Unlock()

	if err := media.ValidKind(source.Kind); err != nil {
		call.Err = err
		return f.record(call)
	}
	r, caps, err := f.memberOf(id, subscriber)
	if err != nil {
		call.Err = err
		return f.record(call)
	}
	if _, in := r.members[source.Member]; !in {
		call.Err = media.ErrNoSuchMember
		return f.record(call)
	}
	if !caps.Allows(media.MaySubscribe) {
		call.Err = media.ErrNotPermitted
		return f.record(call)
	}
	if !r.sources[source] {
		call.Err = media.ErrNoSuchSource
		return f.record(call)
	}
	r.subscribers[subscription{subscriber, source}] = true
	return f.record(call)
}

// Unsubscribe implements media.Port.
func (f *Fake) Unsubscribe(ctx context.Context, id media.RoomID, subscriber media.MemberID, source media.Source) error {
	call := Call{Op: "Unsubscribe", Room: id, Member: subscriber, Source: source}
	if err := f.begin(ctx); err != nil {
		defer f.mu.Unlock()
		call.Err = err
		return f.record(call)
	}
	defer f.mu.Unlock()

	if err := media.ValidKind(source.Kind); err != nil {
		call.Err = err
		return f.record(call)
	}
	r, _, err := f.memberOf(id, subscriber)
	if err != nil {
		call.Err = err
		return f.record(call)
	}
	delete(r.subscribers, subscription{subscriber, source})
	return f.record(call)
}

// SetSubscriberGain implements media.Port.
func (f *Fake) SetSubscriberGain(ctx context.Context, id media.RoomID, subscriber media.MemberID, source media.Source, gain media.Gain) error {
	call := Call{Op: "SetSubscriberGain", Room: id, Member: subscriber, Source: source, Gain: gain}
	if err := f.begin(ctx); err != nil {
		defer f.mu.Unlock()
		call.Err = err
		return f.record(call)
	}
	defer f.mu.Unlock()

	// The capability answer comes first, and that ordering is the specification
	// rather than a preference: a unit that does not apply gain at all has
	// nothing to say about whether the subscription exists or the number is in
	// range, and a caller expecting this answer would otherwise get one of the
	// other two from a fake and neither from a forwarding unit.
	if !f.gainSupported {
		call.Err = media.ErrGainNotSupported
		return f.record(call)
	}
	r, ok := f.rooms[id]
	if !ok || !r.subscribers[subscription{subscriber, source}] {
		call.Err = media.ErrNoSuchSubscription
		return f.record(call)
	}
	if gain < f.gainRange[0] || gain > f.gainRange[1] {
		call.Err = media.ErrGainOutOfRange
		return f.record(call)
	}
	return f.record(call)
}

// SpeakingState implements media.Port.
func (f *Fake) SpeakingState(ctx context.Context, id media.RoomID) ([]media.Speaking, error) {
	call := Call{Op: "SpeakingState", Room: id}
	if err := f.begin(ctx); err != nil {
		defer f.mu.Unlock()
		call.Err = err
		return nil, f.record(call)
	}
	defer f.mu.Unlock()

	r, ok := f.rooms[id]
	if !ok {
		call.Err = media.ErrNoSuchRoom
		return nil, f.record(call)
	}
	if !f.observable {
		call.Err = media.ErrNotObservable
		return nil, f.record(call)
	}

	out := make([]media.Speaking, 0, len(r.members))
	for member := range r.members {
		out = append(out, media.Speaking{Member: member, Speaking: r.speaking[member]})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Member < out[j].Member })
	return out, f.record(call)
}

// HopMetrics implements media.Port.
func (f *Fake) HopMetrics(ctx context.Context, id media.RoomID) ([]media.Hop, error) {
	call := Call{Op: "HopMetrics", Room: id}
	if err := f.begin(ctx); err != nil {
		defer f.mu.Unlock()
		call.Err = err
		return nil, f.record(call)
	}
	defer f.mu.Unlock()

	r, ok := f.rooms[id]
	if !ok {
		call.Err = media.ErrNoSuchRoom
		return nil, f.record(call)
	}
	if !f.observable {
		call.Err = media.ErrNotObservable
		return nil, f.record(call)
	}

	out := make([]media.Hop, 0, len(r.members))
	for member := range r.members {
		hop, set := r.hops[member]
		if !set {
			hop = media.Hop{Member: member, Observed: f.clock.Now()}
		}
		out = append(out, hop)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Member < out[j].Member })
	return out, f.record(call)
}

// DrainRoom implements media.Port.
func (f *Fake) DrainRoom(ctx context.Context, id media.RoomID) error {
	call := Call{Op: "DrainRoom", Room: id}
	if err := f.begin(ctx); err != nil {
		defer f.mu.Unlock()
		call.Err = err
		return f.record(call)
	}
	defer f.mu.Unlock()

	r, ok := f.rooms[id]
	if !ok {
		call.Err = media.ErrNoSuchRoom
		return f.record(call)
	}

	// The room stops admitting whether or not the drain completes, because a
	// drain that gave up and then let somebody back in would be a shutdown that
	// never finishes.
	r.draining = true
	if len(r.members) > 0 {
		call.Err = media.ErrDrainDeadlineExceeded
		return f.record(call)
	}
	return f.record(call)
}

// memberOf answers the two lookups every member-scoped operation makes, so the
// order of ErrNoSuchRoom and ErrNoSuchMember is decided once.
func (f *Fake) memberOf(id media.RoomID, member media.MemberID) (*room, media.Capabilities, error) {
	r, ok := f.rooms[id]
	if !ok {
		return nil, 0, media.ErrNoSuchRoom
	}
	caps, in := r.members[member]
	if !in {
		return nil, 0, media.ErrNoSuchMember
	}
	return r, caps, nil
}

// publishing is the capability a source of that kind needs.
func publishing(kind media.SourceKind) media.Capability {
	if kind == media.Video {
		return media.MayPublishVideo
	}
	return media.MayPublishAudio
}
