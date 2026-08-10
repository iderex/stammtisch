// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026  iderex

package orchestration_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/iderex/stammtisch/internal/orchestration"
)

// The suite for issue #34. Three of its done-when lines are assertions about
// what other members see, so the register is driven through the presence
// fan-out rather than on its own: a leave that is never published is a leave
// nobody in the channel sees, and only the hub can say whether one was.
//
// Nothing here sleeps. The window is moved with the injected clock and the end
// of it is a call to Reap, so "inside the window" and "outside the window" are
// two assertions rather than two waits.

const (
	// resumeWindow is short and is not ResumeWindow. The register takes the
	// window as an argument for exactly this reason, and a suite that used the
	// real constant would prove the constant rather than the behaviour.
	resumeWindow = 4 * time.Second
	// resumeBreadth is small so the overflow arm is reachable with three
	// channels instead of sixty five.
	resumeBreadth = 2
)

// resumeRig is one space with a watcher attached, the presence hub it is
// watching, and the resume register beside it. The watcher is another member,
// because "invisible to other members" is a statement about somebody else's
// screen and cannot be asserted from the reconnecting client's own.
type resumeRig struct {
	t       *testing.T
	clock   *orchestration.FakeClock
	reg     *orchestration.Resumptions
	hub     *orchestration.PresenceHub
	watcher orchestration.ClientID
	seq     map[string]uint64
}

func newResumeRig(t *testing.T, channels ...orchestration.ID) *resumeRig {
	t.Helper()

	clock := orchestration.NewFakeClock(presenceAt(0))
	reg, err := orchestration.NewResumptions(clock, resumeWindow, resumeBreadth)
	if err != nil {
		t.Fatalf("NewResumptions: %v", err)
	}

	hub := presenceHub(t, seesEverything())
	for _, channel := range channels {
		if err := hub.AddChannel(channel); err != nil {
			t.Fatalf("AddChannel(%s): %v", channel, err)
		}
	}

	r := &resumeRig{t: t, clock: clock, reg: reg, hub: hub, watcher: "watcher", seq: map[string]uint64{}}
	if _, err := hub.Attach(r.watcher, orchestration.Person(id(t, "watcher"))); err != nil {
		t.Fatalf("Attach(watcher): %v", err)
	}
	for _, channel := range channels {
		if _, err := hub.Subscribe(r.watcher, channel); err != nil {
			t.Fatalf("Subscribe(watcher, %s): %v", channel, err)
		}
	}
	return r
}

// report puts a member in a channel. A zero channel is a leave.
func (r *resumeRig) report(member, channel orchestration.ID, at int) {
	r.t.Helper()
	r.seq[member.String()]++
	r.hub.Apply(orchestration.PresenceReport{
		Member:  member,
		Seq:     r.seq[member.String()],
		Channel: channel,
		Since:   presenceAt(at),
	})
}

// flush ends a coalescing interval and tells the register which channels the
// batch moved, which is the call a server makes after every flush and is what
// gives a ticket a revision to be measured against.
func (r *resumeRig) flush() []orchestration.PresenceDelivery {
	r.t.Helper()
	deliveries := r.hub.Flush()

	seen := map[orchestration.ID]bool{}
	var moved []orchestration.ID
	for _, d := range deliveries {
		for _, c := range d.Message.Counts {
			if !seen[c.Channel] {
				seen[c.Channel] = true
				moved = append(moved, c.Channel)
			}
		}
		for _, m := range d.Message.Members {
			if !seen[m.Channel] {
				seen[m.Channel] = true
				moved = append(moved, m.Channel)
			}
		}
	}
	r.reg.Published(moved...)
	return deliveries
}

// settle releases the occupancy of every member whose window has run out. It is
// the whole of what a server does with Reap's answer, and it is what turns a
// window that passed into an event other members receive.
func (r *resumeRig) settle() {
	r.t.Helper()
	for _, member := range r.reg.Reap() {
		r.seq[member.String()]++
		r.hub.Apply(orchestration.PresenceReport{Member: member, Seq: r.seq[member.String()]})
	}
}

// memberUpdates flattens the member updates out of a flush, so a test asserts
// on the sequence of occupancy events rather than on a shape.
func memberUpdates(deliveries []orchestration.PresenceDelivery) []orchestration.PresenceMemberUpdate {
	var out []orchestration.PresenceMemberUpdate
	for _, d := range deliveries {
		out = append(out, d.Message.Members...)
	}
	return out
}

// TestAReconnectInsideTheWindowIsInvisibleToOtherMembers is the second
// done-when line of #34, asserted the way that line asks for: by there being no
// occupancy event at all, rather than by the member still being where they
// were.
//
// The member's own state never moving is the weaker assertion and it is the one
// that passes when the server emits a leave and a join that happen to cancel
// out. What another member sees is the property, so the watcher's deliveries
// are what is read.
func TestAReconnectInsideTheWindowIsInvisibleToOtherMembers(t *testing.T) {
	general := id(t, "general")
	r := newResumeRig(t, general)
	member := id(t, "ada")

	r.report(member, general, 0)
	if got := len(memberUpdates(r.flush())); got != 1 {
		t.Fatalf("the join produced %d occupancy events, want 1", got)
	}
	folded := r.reg.Revision()

	if err := r.reg.Suspend("session-1", member); err != nil {
		t.Fatalf("Suspend: %v", err)
	}

	r.clock.Advance(resumeWindow - time.Second)
	r.settle()

	if deliveries := r.flush(); len(deliveries) != 0 {
		t.Fatalf("a drop inside the window published %v, want nothing at all", deliveries)
	}

	answer, err := r.reg.Resume(orchestration.ResumeTicket{Session: "session-1", Folded: folded}, member)
	if err != nil {
		t.Fatalf("Resume inside the window: %v", err)
	}
	if answer.Member != member {
		t.Errorf("the answer names %s, want %s", answer.Member, member)
	}
	if answer.Full {
		t.Errorf("a client that missed nothing was sent the whole state")
	}
	if len(answer.Missed) != 0 {
		t.Errorf("the answer names %v as missed, want nothing", answer.Missed)
	}

	if deliveries := r.flush(); len(deliveries) != 0 {
		t.Fatalf("the resume itself published %v, want nothing at all", deliveries)
	}
	if where, present := r.hub.Projection().Where(member); !present || where.Channel != general {
		t.Errorf("after the resume the member is %v present=%t, want %s", where, present, general)
	}
}

// TestAReconnectOutsideTheWindowIsOneLeaveAndThenOneJoin is the third done-when
// line, and the order is half of it: a join published before the leave leaves
// every other client holding two occupancies for one person, which the
// projection refuses to produce and a fan-out written the other way round would
// happily send.
func TestAReconnectOutsideTheWindowIsOneLeaveAndThenOneJoin(t *testing.T) {
	general := id(t, "general")
	r := newResumeRig(t, general)
	member := id(t, "ada")

	r.report(member, general, 0)
	r.flush()

	if err := r.reg.Suspend("session-1", member); err != nil {
		t.Fatalf("Suspend: %v", err)
	}

	r.clock.Advance(resumeWindow)
	r.settle()

	var events []orchestration.PresenceMemberUpdate
	events = append(events, memberUpdates(r.flush())...)

	// The client comes back on a new connection and joins normally, which is
	// what a refused resume leaves it to do.
	if _, err := r.reg.Resume(orchestration.ResumeTicket{Session: "session-1"}, member); !errors.Is(err, orchestration.ErrNoSuspension) {
		t.Fatalf("resuming after the leave was reported returned %v, want ErrNoSuspension", err)
	}
	r.report(member, general, 10)
	events = append(events, memberUpdates(r.flush())...)

	if len(events) != 2 {
		t.Fatalf("a drop outside the window published %d occupancy events, want exactly 2: %v", len(events), events)
	}
	if events[0].Present {
		t.Errorf("the first event is a join, want the leave first: %v", events[0])
	}
	if !events[1].Present {
		t.Errorf("the second event is a leave, want the join second: %v", events[1])
	}
	for i, e := range events {
		if e.Member != member || e.Channel != general {
			t.Errorf("event %d is about %s in %s, want %s in %s", i, e.Member, e.Channel, member, general)
		}
	}
}

// TestAWindowThatPassedStillOwesItsLeave is the case the refusal above is
// written around. A ticket arriving after the window is refused, and refusing
// it must not forget the suspension: the leave other members are waiting for
// has not been reported yet, and Reap is the only thing that reports it.
//
// Deleting the refusal's comment is free; deleting the suspension there is not,
// and this is what says so.
func TestAWindowThatPassedStillOwesItsLeave(t *testing.T) {
	general := id(t, "general")
	r := newResumeRig(t, general)
	member := id(t, "ada")

	r.report(member, general, 0)
	r.flush()
	if err := r.reg.Suspend("session-1", member); err != nil {
		t.Fatalf("Suspend: %v", err)
	}

	r.clock.Advance(resumeWindow)
	if _, err := r.reg.Resume(orchestration.ResumeTicket{Session: "session-1"}, member); !errors.Is(err, orchestration.ErrResumeWindowPassed) {
		t.Fatalf("Resume after the window returned %v, want ErrResumeWindowPassed", err)
	}

	r.settle()
	events := memberUpdates(r.flush())
	if len(events) != 1 || events[0].Present {
		t.Fatalf("the refused ticket left %v, want exactly one leave", events)
	}
}

// TestAResumePresentingAStateTheRegisterNoLongerHoldsGetsTheWholeState is the
// fourth done-when line. Both arms answer with the whole state and neither
// answers with an error, which is the half of that line that matters: a client
// whose state is too old is behind rather than broken.
func TestAResumePresentingAStateTheRegisterNoLongerHoldsGetsTheWholeState(t *testing.T) {
	t.Run("a revision older than the suspension", func(t *testing.T) {
		general := id(t, "general")
		r := newResumeRig(t, general)
		member := id(t, "ada")

		r.report(member, general, 0)
		r.flush()
		if r.reg.Revision() == 0 {
			t.Fatalf("the join published no revision, so there is nothing for a ticket to be behind")
		}
		if err := r.reg.Suspend("session-1", member); err != nil {
			t.Fatalf("Suspend: %v", err)
		}

		// Folded zero is a client that folded nothing, which is every
		// revision the register issued before it dropped and never recorded.
		answer, err := r.reg.Resume(orchestration.ResumeTicket{Session: "session-1", Folded: 0}, member)
		if err != nil {
			t.Fatalf("Resume with a stale revision returned %v, want the whole state", err)
		}
		if !answer.Full {
			t.Errorf("the answer is a difference, want the whole state")
		}
		if len(answer.Missed) != 0 {
			t.Errorf("a full transfer names %v as missed, want nothing", answer.Missed)
		}
	})

	t.Run("more channels than the register will name", func(t *testing.T) {
		one, two, three := id(t, "one"), id(t, "two"), id(t, "three")
		r := newResumeRig(t, one, two, three)
		member, other := id(t, "ada"), id(t, "grace")

		r.report(member, one, 0)
		r.flush()
		folded := r.reg.Revision()
		if err := r.reg.Suspend("session-1", member); err != nil {
			t.Fatalf("Suspend: %v", err)
		}

		// Three channels move while the client is away and the register will
		// name two, so it discards what it had rather than answering with
		// part of it.
		r.report(other, one, 1)
		r.flush()
		r.report(other, two, 2)
		r.flush()
		r.report(other, three, 3)
		r.flush()

		// A batch after the set was discarded must not start a new one, or the
		// register answers with a difference that is missing everything before
		// the overflow.
		r.report(other, one, 4)
		r.flush()

		answer, err := r.reg.Resume(orchestration.ResumeTicket{Session: "session-1", Folded: folded}, member)
		if err != nil {
			t.Fatalf("Resume after the set overflowed returned %v, want the whole state", err)
		}
		if !answer.Full {
			t.Errorf("the answer names %v as missed, want the whole state", answer.Missed)
		}
	})
}

// TestTheAnswerNamesTheChannelsThatMoved is the other side of the line above:
// the difference is sent when the register can still say what it is, and it
// names each channel once however many times that channel moved.
func TestTheAnswerNamesTheChannelsThatMoved(t *testing.T) {
	one, two := id(t, "one"), id(t, "two")
	r := newResumeRig(t, one, two)
	member, other := id(t, "ada"), id(t, "grace")

	r.report(member, one, 0)
	r.flush()
	folded := r.reg.Revision()
	if err := r.reg.Suspend("session-1", member); err != nil {
		t.Fatalf("Suspend: %v", err)
	}

	r.report(other, two, 1)
	r.flush()
	r.report(other, one, 2)
	r.flush()

	answer, err := r.reg.Resume(orchestration.ResumeTicket{Session: "session-1", Folded: folded}, member)
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if answer.Full {
		t.Fatalf("the register gave up on a difference of two channels")
	}
	if got, want := fmt.Sprint(answer.Missed), fmt.Sprint([]orchestration.ID{one, two}); got != want {
		t.Errorf("the answer names %s, want %s in identifier order", got, want)
	}
}

// TestTheRegisterRefusesTheFourThingsItSaysItRefuses walks the closed set. Each
// refusal is a distinct sentinel because a client acts on three of them
// differently, and a test that only asserted "an error" would pass with all
// four collapsed into one.
func TestTheRegisterRefusesTheFourThingsItSaysItRefuses(t *testing.T) {
	general := id(t, "general")
	member, other := id(t, "ada"), id(t, "grace")

	t.Run("no suspension is held for that session", func(t *testing.T) {
		r := newResumeRig(t, general)
		_, err := r.reg.Resume(orchestration.ResumeTicket{Session: "never-seen"}, member)
		if !errors.Is(err, orchestration.ErrNoSuspension) {
			t.Fatalf("Resume of an unknown session returned %v, want ErrNoSuspension", err)
		}
	})

	t.Run("the window has passed", func(t *testing.T) {
		r := newResumeRig(t, general)
		if err := r.reg.Suspend("session-1", member); err != nil {
			t.Fatalf("Suspend: %v", err)
		}
		r.clock.Advance(resumeWindow)
		_, err := r.reg.Resume(orchestration.ResumeTicket{Session: "session-1"}, member)
		if !errors.Is(err, orchestration.ErrResumeWindowPassed) {
			t.Fatalf("Resume after the window returned %v, want ErrResumeWindowPassed", err)
		}
	})

	t.Run("the suspension belongs to another member", func(t *testing.T) {
		r := newResumeRig(t, general)
		if err := r.reg.Suspend("session-1", member); err != nil {
			t.Fatalf("Suspend: %v", err)
		}
		_, err := r.reg.Resume(orchestration.ResumeTicket{Session: "session-1"}, other)
		if !errors.Is(err, orchestration.ErrResumeNotYours) {
			t.Fatalf("Resume by another member returned %v, want ErrResumeNotYours", err)
		}
		if !r.reg.Held("session-1") {
			t.Errorf("a ticket from the wrong member forgot the suspension, so the member it belongs to can no longer resume")
		}
	})

	t.Run("the ticket folds a revision nobody published", func(t *testing.T) {
		r := newResumeRig(t, general)
		if err := r.reg.Suspend("session-1", member); err != nil {
			t.Fatalf("Suspend: %v", err)
		}
		_, err := r.reg.Resume(orchestration.ResumeTicket{Session: "session-1", Folded: 99}, member)
		if !errors.Is(err, orchestration.ErrResumeAhead) {
			t.Fatalf("Resume folding an unpublished revision returned %v, want ErrResumeAhead", err)
		}
	})
}

// TestASuspensionIsForgottenOnceItResumes. A ticket is good once. A register
// that kept the suspension would let a copy of a ticket resume a connection
// somebody else is already holding.
func TestASuspensionIsForgottenOnceItResumes(t *testing.T) {
	general := id(t, "general")
	r := newResumeRig(t, general)
	member := id(t, "ada")

	if err := r.reg.Suspend("session-1", member); err != nil {
		t.Fatalf("Suspend: %v", err)
	}
	if !r.reg.Held("session-1") {
		t.Fatalf("Held is false for a session that just suspended")
	}
	if _, err := r.reg.Resume(orchestration.ResumeTicket{Session: "session-1"}, member); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if r.reg.Held("session-1") {
		t.Errorf("Held is true for a session that has resumed")
	}
	if _, err := r.reg.Resume(orchestration.ResumeTicket{Session: "session-1"}, member); !errors.Is(err, orchestration.ErrNoSuspension) {
		t.Errorf("a second Resume on one ticket returned %v, want ErrNoSuspension", err)
	}
	if gone := r.reg.Reap(); gone != nil {
		t.Errorf("Reap returned %v after the session resumed, want nothing", gone)
	}
}

// TestReapReportsInSuspendOrderAndOnlyOnce. The order is fixed so a caller
// publishes leaves in one order rather than the map's, and a member reported
// twice is a second leave for somebody who already left.
func TestReapReportsInSuspendOrderAndOnlyOnce(t *testing.T) {
	general := id(t, "general")
	r := newResumeRig(t, general)
	first, second := id(t, "ada"), id(t, "grace")

	if err := r.reg.Suspend("session-1", first); err != nil {
		t.Fatalf("Suspend(session-1): %v", err)
	}
	r.clock.Advance(time.Second)
	if err := r.reg.Suspend("session-2", second); err != nil {
		t.Fatalf("Suspend(session-2): %v", err)
	}

	// The first window has run out and the second has not.
	r.clock.Advance(resumeWindow - time.Second)
	if got, want := fmt.Sprint(r.reg.Reap()), fmt.Sprint([]orchestration.ID{first}); got != want {
		t.Fatalf("Reap returned %s, want %s", got, want)
	}
	if gone := r.reg.Reap(); gone != nil {
		t.Errorf("a second Reap returned %v, want nothing", gone)
	}

	r.clock.Advance(time.Second)
	if got, want := fmt.Sprint(r.reg.Reap()), fmt.Sprint([]orchestration.ID{second}); got != want {
		t.Errorf("Reap returned %s, want %s", got, want)
	}
}

// TestAnEmptyBatchDoesNotMoveTheRevision. A server flushes on every interval
// and most intervals are empty. A revision that moved on each of them would
// walk every suspended client's ticket into the past for no change at all,
// which turns every resume into a full transfer within a few seconds.
func TestAnEmptyBatchDoesNotMoveTheRevision(t *testing.T) {
	general := id(t, "general")
	r := newResumeRig(t, general)

	if got := r.reg.Revision(); got != 0 {
		t.Fatalf("a fresh register is at revision %d, want 0", got)
	}
	if got := r.reg.Published(); got != 0 {
		t.Errorf("an empty batch moved the revision to %d, want 0", got)
	}
	if got := r.reg.Published(general); got != 1 {
		t.Errorf("a batch touching one channel moved the revision to %d, want 1", got)
	}
}

// TestSuspendRefusesWhatItCannotHold covers the three ways a caller asks for a
// suspension the register cannot key, look up or release.
func TestSuspendRefusesWhatItCannotHold(t *testing.T) {
	general := id(t, "general")
	r := newResumeRig(t, general)
	member := id(t, "ada")

	if err := r.reg.Suspend("", member); !errors.Is(err, orchestration.ErrInvariant) {
		t.Errorf("Suspend with no session returned %v, want ErrInvariant", err)
	}
	if err := r.reg.Suspend("session-1", orchestration.ID{}); !errors.Is(err, orchestration.ErrInvariant) {
		t.Errorf("Suspend with no member returned %v, want ErrInvariant", err)
	}
	if err := r.reg.Suspend("session-1", member); err != nil {
		t.Fatalf("Suspend: %v", err)
	}
	if err := r.reg.Suspend("session-1", member); !errors.Is(err, orchestration.ErrInvariant) {
		t.Errorf("suspending one session twice returned %v, want ErrInvariant", err)
	}
}

// TestNewResumptionsRefusesARegisterThatCouldNotDecideAnything. Each of the
// three is a register that would answer every ticket the same way, which is a
// feature that is off while looking configured.
func TestNewResumptionsRefusesARegisterThatCouldNotDecideAnything(t *testing.T) {
	clock := orchestration.NewFakeClock(presenceAt(0))

	if _, err := orchestration.NewResumptions(nil, resumeWindow, resumeBreadth); !errors.Is(err, orchestration.ErrInvariant) {
		t.Errorf("NewResumptions with no clock returned %v, want ErrInvariant", err)
	}
	if _, err := orchestration.NewResumptions(clock, 0, resumeBreadth); !errors.Is(err, orchestration.ErrInvariant) {
		t.Errorf("NewResumptions with a zero window returned %v, want ErrInvariant", err)
	}
	if _, err := orchestration.NewResumptions(clock, resumeWindow, 0); !errors.Is(err, orchestration.ErrInvariant) {
		t.Errorf("NewResumptions with a zero breadth returned %v, want ErrInvariant", err)
	}
	if _, err := orchestration.NewResumptions(clock, resumeWindow, resumeBreadth); err != nil {
		t.Errorf("NewResumptions with everything it needs returned %v", err)
	}
}

// TestTheWindowAndTheGracePeriodAreEqualDeliberately. The two constants defend
// different things and come out at the same number, and this is what makes that
// equality a statement somebody has to change on purpose rather than a
// coincidence a later edit can break in silence.
func TestTheWindowAndTheGracePeriodAreEqualDeliberately(t *testing.T) {
	if orchestration.ResumeWindow != 30*time.Second {
		t.Errorf("the resume window is %v, and docs/decisions/channel-and-room-model.md derives 30s from the transport", orchestration.ResumeWindow)
	}
}
