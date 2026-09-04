// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026  Nils Lehnen

package fake_test

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/iderex/stammtisch/internal/media"
	"github.com/iderex/stammtisch/internal/media/fake"
	"github.com/iderex/stammtisch/internal/orchestration"
)

const (
	theRoom = media.RoomID("channel-1")
	ada     = media.MemberID("ada@example.org")
	grace   = media.MemberID("grace@example.org")
)

var adasVoice = media.Source{Member: ada, Kind: media.Audio}

// newFake returns a fake on a clock a test moves by hand, and the clock.
//
// The clock is orchestration's, which is the point of media.Clock being
// declared as one method: a server builds one implementation and hands it to
// both layers, and this test is where that is shown to hold rather than
// asserted in a comment.
func newFake(t *testing.T) (*fake.Fake, *orchestration.FakeClock) {
	t.Helper()
	clock := orchestration.NewFakeClock(time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC))
	f, err := fake.New(clock)
	if err != nil {
		t.Fatalf("building the fake: %v", err)
	}
	return f, clock
}

// admitted is a room with two members who may do everything, which is the
// arrangement most of the tests below start from.
func admitted(t *testing.T, f *fake.Fake) {
	t.Helper()
	ctx := context.Background()
	caps := media.Capabilities(0).With(media.MayPublishAudio).With(media.MayPublishVideo).With(media.MaySubscribe)
	if err := f.CreateRoom(ctx, theRoom); err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	for _, m := range []media.MemberID{ada, grace} {
		if _, err := f.AdmitMember(ctx, theRoom, m, caps); err != nil {
			t.Fatalf("AdmitMember %s: %v", m, err)
		}
	}
}

func TestAFakeWithNoClockIsRefused(t *testing.T) {
	if _, err := fake.New(nil); err == nil {
		t.Fatal("a fake was built with no clock, and it would have read the real one")
	}
}

// TestEveryPortOperationIsImplementedAndRecorded is the first Done-when line of
// issue #36.
//
// Compilation carries half of it: `var _ media.Port = (*Fake)(nil)` in the
// package means an operation with no implementation is not a failing test, it is
// a build that does not happen. What compilation cannot see is an operation that
// answers without recording, and that is the half this test holds. It drives all
// twelve through reflection off media.Operations(), so an operation added to the
// port and implemented without its record reds here rather than going quiet.
func TestEveryPortOperationIsImplementedAndRecorded(t *testing.T) {
	f, _ := newFake(t)
	ctx := context.Background()
	admitted(t, f)

	// Every operation, called once with arguments that reach it. What each one
	// answers is other tests' business; this one is about the record.
	if err := f.PublishSource(ctx, theRoom, ada, media.Audio); err != nil {
		t.Fatalf("PublishSource: %v", err)
	}
	if err := f.Subscribe(ctx, theRoom, grace, adasVoice); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	_ = f.SetSubscriberGain(ctx, theRoom, grace, adasVoice, 1)
	if _, err := f.SpeakingState(ctx, theRoom); err != nil {
		t.Fatalf("SpeakingState: %v", err)
	}
	if _, err := f.HopMetrics(ctx, theRoom); err != nil {
		t.Fatalf("HopMetrics: %v", err)
	}
	if err := f.Unsubscribe(ctx, theRoom, grace, adasVoice); err != nil {
		t.Fatalf("Unsubscribe: %v", err)
	}
	if err := f.UnpublishSource(ctx, theRoom, ada, media.Audio); err != nil {
		t.Fatalf("UnpublishSource: %v", err)
	}
	if err := f.RevokeMember(ctx, theRoom, grace); err != nil {
		t.Fatalf("RevokeMember: %v", err)
	}
	_ = f.DrainRoom(ctx, theRoom)
	if _, err := f.DestroyRoom(ctx, theRoom); err != nil {
		t.Fatalf("DestroyRoom: %v", err)
	}

	recorded := map[string]bool{}
	for _, op := range f.Operations() {
		recorded[op] = true
	}

	var missing []string
	for _, op := range media.Operations() {
		if !recorded[op] {
			missing = append(missing, op)
		}
	}
	if len(missing) > 0 {
		t.Errorf("these port operations were driven and recorded nothing: %v", missing)
	}

	for op := range recorded {
		found := false
		for _, declared := range media.Operations() {
			if declared == op {
				found = true
			}
		}
		if !found {
			t.Errorf("the fake recorded %q, which is not an operation the port declares", op)
		}
	}
}

// TestTheRecordIsTheSequenceAndNotASet. A test asserting that a switch was one
// transition compares an order, so a record that lost the order would pass every
// assertion this fake exists to make possible.
func TestTheRecordIsTheSequenceAndNotASet(t *testing.T) {
	f, _ := newFake(t)
	ctx := context.Background()
	admitted(t, f)

	want := []string{"CreateRoom", "AdmitMember", "AdmitMember"}
	if got := f.Operations(); !reflect.DeepEqual(got, want) {
		t.Errorf("the record is %v, want %v", got, want)
	}

	if err := f.RevokeMember(ctx, theRoom, ada); err != nil {
		t.Fatalf("RevokeMember: %v", err)
	}
	if got := f.Operations(); !reflect.DeepEqual(got, append(want, "RevokeMember")) {
		t.Errorf("the record is %v after a revoke", got)
	}
}

// TestARefusalIsInTheRecordWithItsError is the test above, written against the
// port rather than a helper.
func TestARefusalIsInTheRecordWithItsError(t *testing.T) {
	f, _ := newFake(t)
	ctx := context.Background()

	if _, err := f.AdmitMember(ctx, theRoom, ada, 0); !errors.Is(err, media.ErrNoSuchRoom) {
		t.Fatalf("admitting into a room that does not exist gave %v", err)
	}
	calls := f.Calls()
	if len(calls) != 1 {
		t.Fatalf("a refused admission produced %d calls", len(calls))
	}
	if !errors.Is(calls[0].Err, media.ErrNoSuchRoom) {
		t.Errorf("the recorded call carries %v rather than the refusal", calls[0].Err)
	}
	if calls[0].Op != "AdmitMember" || calls[0].Room != theRoom || calls[0].Member != ada {
		t.Errorf("the recorded call is %s", calls[0])
	}
}

// ---- the misbehaviours -----------------------------------------------------
//
// Each of the five the issue names is inducible and has at least one test using
// it, and each test asserts the consequence a caller would meet rather than
// that the flag was set.

// TestAnAdmissionThatTimesOut. The unit took the request and the caller's
// deadline went before an answer came. Nothing waits, because the fake is on an
// injected clock and a fake that slept would put the sleep in every suite above
// it.
func TestAnAdmissionThatTimesOut(t *testing.T) {
	f, _ := newFake(t)
	ctx := context.Background()
	if err := f.CreateRoom(ctx, theRoom); err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}

	f.TimeOutAdmission(theRoom, ada, true)
	cred, err := f.AdmitMember(ctx, theRoom, ada, media.Capabilities(0).With(media.MaySubscribe))
	if !errors.Is(err, media.ErrCancelled) {
		t.Fatalf("the admission gave %v, want ErrCancelled", err)
	}
	if !cred.IsZero() {
		t.Error("a timed-out admission returned a credential, which a caller would present")
	}
	if f.Admitted(theRoom, ada) {
		t.Error("a timed-out admission put the member in the room anyway")
	}

	// And it stops when the test says so, which is what makes it a condition
	// rather than a property of the fake.
	f.TimeOutAdmission(theRoom, ada, false)
	if _, err := f.AdmitMember(ctx, theRoom, ada, media.Capabilities(0).With(media.MaySubscribe)); err != nil {
		t.Fatalf("the admission still fails after the fault was cleared: %v", err)
	}
}

// TestARoomThatFailsToCreate.
func TestARoomThatFailsToCreate(t *testing.T) {
	f, _ := newFake(t)
	ctx := context.Background()

	f.FailRoomCreation(theRoom, true)
	if err := f.CreateRoom(ctx, theRoom); !errors.Is(err, media.ErrRoomLimitReached) {
		t.Fatalf("CreateRoom gave %v, want ErrRoomLimitReached", err)
	}
	if _, err := f.AdmitMember(ctx, theRoom, ada, 0); !errors.Is(err, media.ErrNoSuchRoom) {
		t.Errorf("the room a failed creation left behind admitted with %v", err)
	}

	f.FailRoomCreation(theRoom, false)
	if err := f.CreateRoom(ctx, theRoom); err != nil {
		t.Fatalf("CreateRoom still fails after the fault was cleared: %v", err)
	}
}

// TestATrackThatNeverArrives is the sharpest of the five, because the operation
// succeeds. The unit said yes and nothing came, so a caller that trusted the
// answer subscribes to a source that is not there.
func TestATrackThatNeverArrives(t *testing.T) {
	f, _ := newFake(t)
	ctx := context.Background()
	admitted(t, f)

	f.NeverDeliverSource(adasVoice, true)
	if err := f.PublishSource(ctx, theRoom, ada, media.Audio); err != nil {
		t.Fatalf("PublishSource gave %v, and this fault is the one where it succeeds", err)
	}
	if f.Published(theRoom, adasVoice) {
		t.Fatal("the source arrived, so this test is not about a track that never did")
	}
	if err := f.Subscribe(ctx, theRoom, grace, adasVoice); !errors.Is(err, media.ErrNoSuchSource) {
		t.Errorf("subscribing to a track that never arrived gave %v, want ErrNoSuchSource", err)
	}
}

// TestASubscriptionThatIsDropped. The unit forgets a subscription and tells
// nobody, so the caller finds out from the next operation that needed it.
func TestASubscriptionThatIsDropped(t *testing.T) {
	f, _ := newFake(t)
	ctx := context.Background()
	admitted(t, f)
	f.SetGainSupported(true)

	if err := f.PublishSource(ctx, theRoom, ada, media.Audio); err != nil {
		t.Fatalf("PublishSource: %v", err)
	}
	if err := f.Subscribe(ctx, theRoom, grace, adasVoice); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	if !f.Subscribed(theRoom, grace, adasVoice) {
		t.Fatal("the subscription was not made, so dropping it proves nothing")
	}
	if err := f.SetSubscriberGain(ctx, theRoom, grace, adasVoice, 2); err != nil {
		t.Fatalf("SetSubscriberGain before the drop: %v", err)
	}

	f.DropSubscription(theRoom, grace, adasVoice)

	if f.Subscribed(theRoom, grace, adasVoice) {
		t.Error("the subscription survived being dropped")
	}
	if err := f.SetSubscriberGain(ctx, theRoom, grace, adasVoice, 2); !errors.Is(err, media.ErrNoSuchSubscription) {
		t.Errorf("gain on a dropped subscription gave %v, want ErrNoSuchSubscription", err)
	}
}

// TestAUnitThatBecomesUnreachableAndComesBack. Every operation answers
// ErrUnavailable while it is gone, and the state is untouched, which is what
// makes a unit that comes back a unit with its rooms still in it.
func TestAUnitThatBecomesUnreachableAndComesBack(t *testing.T) {
	f, _ := newFake(t)
	ctx := context.Background()
	admitted(t, f)

	f.SetUnreachable(true)

	for _, tc := range []struct {
		op  string
		run func() error
	}{
		{"CreateRoom", func() error { return f.CreateRoom(ctx, "channel-2") }},
		{"DestroyRoom", func() error { _, err := f.DestroyRoom(ctx, theRoom); return err }},
		{"AdmitMember", func() error { _, err := f.AdmitMember(ctx, theRoom, "new@example.org", 0); return err }},
		{"RevokeMember", func() error { return f.RevokeMember(ctx, theRoom, ada) }},
		{"PublishSource", func() error { return f.PublishSource(ctx, theRoom, ada, media.Audio) }},
		{"UnpublishSource", func() error { return f.UnpublishSource(ctx, theRoom, ada, media.Audio) }},
		{"Subscribe", func() error { return f.Subscribe(ctx, theRoom, grace, adasVoice) }},
		{"Unsubscribe", func() error { return f.Unsubscribe(ctx, theRoom, grace, adasVoice) }},
		{"SetSubscriberGain", func() error { return f.SetSubscriberGain(ctx, theRoom, grace, adasVoice, 1) }},
		{"SpeakingState", func() error { _, err := f.SpeakingState(ctx, theRoom); return err }},
		{"HopMetrics", func() error { _, err := f.HopMetrics(ctx, theRoom); return err }},
		{"DrainRoom", func() error { return f.DrainRoom(ctx, theRoom) }},
	} {
		if err := tc.run(); !errors.Is(err, media.ErrUnavailable) {
			t.Errorf("%s gave %v while the unit was unreachable, want ErrUnavailable", tc.op, err)
		}
	}

	f.SetUnreachable(false)

	if !f.Admitted(theRoom, ada) {
		t.Error("the unit came back without its room")
	}
	if _, err := f.SpeakingState(ctx, theRoom); err != nil {
		t.Errorf("the unit that came back answers %v", err)
	}
}

// ---- the port's own behaviour ----------------------------------------------

// TestCreateRoomIsIdempotentAndDestroyRoomSaysWhichHappened. Both are the
// specification's own words, and the second is why DestroyRoom returns a bool:
// the caller's intent is a state, and a caller that wanted to know whether it
// did anything can still find out.
func TestCreateRoomIsIdempotentAndDestroyRoomSaysWhichHappened(t *testing.T) {
	f, _ := newFake(t)
	ctx := context.Background()

	if err := f.CreateRoom(ctx, theRoom); err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if err := f.CreateRoom(ctx, theRoom); err != nil {
		t.Errorf("creating a room that exists gave %v, and it is idempotent", err)
	}

	alreadyGone, err := f.DestroyRoom(ctx, theRoom)
	if err != nil {
		t.Fatalf("DestroyRoom: %v", err)
	}
	if alreadyGone {
		t.Error("destroying a room that was there reported it already gone")
	}

	alreadyGone, err = f.DestroyRoom(ctx, theRoom)
	if err != nil {
		t.Errorf("destroying a room that is gone gave %v, and it is idempotent", err)
	}
	if !alreadyGone {
		t.Error("destroying a room that was gone did not report it")
	}
}

// TestARoomLimitIsReachable. The specification says the fake's limit is
// whatever a test sets and the real one's is the unit's, and that the error has
// to be reachable in both.
func TestARoomLimitIsReachable(t *testing.T) {
	f, _ := newFake(t)
	ctx := context.Background()

	f.SetRoomLimit(1)
	if err := f.CreateRoom(ctx, "channel-1"); err != nil {
		t.Fatalf("the first room: %v", err)
	}
	if err := f.CreateRoom(ctx, "channel-2"); !errors.Is(err, media.ErrRoomLimitReached) {
		t.Errorf("the second room gave %v, want ErrRoomLimitReached", err)
	}
	// The limit does not make an existing room unreachable, because CreateRoom
	// is idempotent and a retry at the limit is the common case.
	if err := f.CreateRoom(ctx, "channel-1"); err != nil {
		t.Errorf("creating the existing room at the limit gave %v", err)
	}
}

// TestAMemberIsAdmittedOnceAndTheSecondTimeIsRefused.
func TestAMemberIsAdmittedOnceAndTheSecondTimeIsRefused(t *testing.T) {
	f, _ := newFake(t)
	ctx := context.Background()
	admitted(t, f)

	if _, err := f.AdmitMember(ctx, theRoom, ada, 0); !errors.Is(err, media.ErrMemberAlreadyAdmitted) {
		t.Errorf("a second admission gave %v, want ErrMemberAlreadyAdmitted", err)
	}
}

// TestEveryAdmissionMintsItsOwnCredential. Two members holding one credential
// would make a revocation ambiguous, and the credential is opaque so nothing
// above the port could tell.
func TestEveryAdmissionMintsItsOwnCredential(t *testing.T) {
	f, _ := newFake(t)
	ctx := context.Background()
	if err := f.CreateRoom(ctx, theRoom); err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}

	first, err := f.AdmitMember(ctx, theRoom, ada, 0)
	if err != nil {
		t.Fatalf("admitting ada: %v", err)
	}
	second, err := f.AdmitMember(ctx, theRoom, grace, 0)
	if err != nil {
		t.Fatalf("admitting grace: %v", err)
	}
	if first.IsZero() || second.IsZero() {
		t.Fatal("an admission returned no credential")
	}
	if first.String() == second.String() {
		t.Errorf("two members hold the same credential: %q", first.String())
	}
}

// TestACapabilityTheUnitCannotEnforceRefusesTheAdmission. The caller's answer is
// to refuse rather than to admit with a weaker set: a permission silently
// downgraded here is a permission the layer above believes it has.
func TestACapabilityTheUnitCannotEnforceRefusesTheAdmission(t *testing.T) {
	f, _ := newFake(t)
	ctx := context.Background()
	if err := f.CreateRoom(ctx, theRoom); err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}

	f.RefuseCapability(media.MayPublishVideo, true)

	wanted := media.Capabilities(0).With(media.MaySubscribe).With(media.MayPublishVideo)
	if _, err := f.AdmitMember(ctx, theRoom, ada, wanted); !errors.Is(err, media.ErrPermissionsNotExpressible) {
		t.Fatalf("the admission gave %v, want ErrPermissionsNotExpressible", err)
	}
	if f.Admitted(theRoom, ada) {
		t.Error("the member was admitted with a weaker set, which is the outcome this error exists against")
	}

	// A set that does not carry the refused capability still gets in, so the
	// refusal is about the capability rather than about the member.
	if _, err := f.AdmitMember(ctx, theRoom, ada, media.Capabilities(0).With(media.MaySubscribe)); err != nil {
		t.Errorf("a set without the refused capability gave %v", err)
	}
}

// TestAMemberRevokedIsAbsentFromTheNextRead. The specification names this as
// something the fake and the real unit must agree on.
func TestAMemberRevokedIsAbsentFromTheNextRead(t *testing.T) {
	f, _ := newFake(t)
	ctx := context.Background()
	admitted(t, f)

	if err := f.PublishSource(ctx, theRoom, ada, media.Audio); err != nil {
		t.Fatalf("PublishSource: %v", err)
	}
	if err := f.Subscribe(ctx, theRoom, grace, adasVoice); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	if err := f.RevokeMember(ctx, theRoom, ada); err != nil {
		t.Fatalf("RevokeMember: %v", err)
	}

	if f.Admitted(theRoom, ada) {
		t.Error("the revoked member is still in the room")
	}
	if f.Published(theRoom, adasVoice) {
		t.Error("the revoked member's source is still published")
	}
	if f.Subscribed(theRoom, grace, adasVoice) {
		t.Error("a subscription to the revoked member's source survived")
	}

	speaking, err := f.SpeakingState(ctx, theRoom)
	if err != nil {
		t.Fatalf("SpeakingState: %v", err)
	}
	for _, s := range speaking {
		if s.Member == ada {
			t.Error("the revoked member is in the next speaking state")
		}
	}

	// Idempotent for a member who was never admitted.
	if err := f.RevokeMember(ctx, theRoom, "nobody@example.org"); err != nil {
		t.Errorf("revoking a member who was never admitted gave %v", err)
	}
}

// TestPublishingWithoutTheCapabilityIsRefused, per kind, because a set carrying
// audio and not video is the arrangement a voice-only room actually has.
func TestPublishingWithoutTheCapabilityIsRefused(t *testing.T) {
	f, _ := newFake(t)
	ctx := context.Background()
	if err := f.CreateRoom(ctx, theRoom); err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := f.AdmitMember(ctx, theRoom, ada, media.Capabilities(0).With(media.MayPublishAudio)); err != nil {
		t.Fatalf("AdmitMember: %v", err)
	}

	if err := f.PublishSource(ctx, theRoom, ada, media.Audio); err != nil {
		t.Errorf("publishing audio with MayPublishAudio gave %v", err)
	}
	if err := f.PublishSource(ctx, theRoom, ada, media.Video); !errors.Is(err, media.ErrNotPermitted) {
		t.Errorf("publishing video without MayPublishVideo gave %v, want ErrNotPermitted", err)
	}
}

// TestPublishingTwiceIsRefusedAndUnpublishingTwiceIsNot.
func TestPublishingTwiceIsRefusedAndUnpublishingTwiceIsNot(t *testing.T) {
	f, _ := newFake(t)
	ctx := context.Background()
	admitted(t, f)

	if err := f.PublishSource(ctx, theRoom, ada, media.Audio); err != nil {
		t.Fatalf("PublishSource: %v", err)
	}
	if err := f.PublishSource(ctx, theRoom, ada, media.Audio); !errors.Is(err, media.ErrSourceAlreadyPublished) {
		t.Errorf("publishing twice gave %v, want ErrSourceAlreadyPublished", err)
	}
	if err := f.UnpublishSource(ctx, theRoom, ada, media.Audio); err != nil {
		t.Fatalf("UnpublishSource: %v", err)
	}
	if err := f.UnpublishSource(ctx, theRoom, ada, media.Audio); err != nil {
		t.Errorf("unpublishing twice gave %v, and it is idempotent", err)
	}
}

// TestUnpublishingTakesTheSubscriptionsWithIt. A subscription to a source that
// is gone is a subscription the next gain call would answer about, and the
// answer would be about a source nobody is sending.
func TestUnpublishingTakesTheSubscriptionsWithIt(t *testing.T) {
	f, _ := newFake(t)
	ctx := context.Background()
	admitted(t, f)

	if err := f.PublishSource(ctx, theRoom, ada, media.Audio); err != nil {
		t.Fatalf("PublishSource: %v", err)
	}
	if err := f.Subscribe(ctx, theRoom, grace, adasVoice); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	if err := f.UnpublishSource(ctx, theRoom, ada, media.Audio); err != nil {
		t.Fatalf("UnpublishSource: %v", err)
	}
	if f.Subscribed(theRoom, grace, adasVoice) {
		t.Error("a subscription to an unpublished source survived")
	}
}

// TestSubscribingWithoutTheCapabilityIsRefused, and unsubscribing from
// something nobody subscribed to is not.
func TestSubscribingWithoutTheCapabilityIsRefused(t *testing.T) {
	f, _ := newFake(t)
	ctx := context.Background()
	if err := f.CreateRoom(ctx, theRoom); err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := f.AdmitMember(ctx, theRoom, ada, media.Capabilities(0).With(media.MayPublishAudio)); err != nil {
		t.Fatalf("admitting ada: %v", err)
	}
	if _, err := f.AdmitMember(ctx, theRoom, grace, 0); err != nil {
		t.Fatalf("admitting grace: %v", err)
	}
	if err := f.PublishSource(ctx, theRoom, ada, media.Audio); err != nil {
		t.Fatalf("PublishSource: %v", err)
	}

	if err := f.Subscribe(ctx, theRoom, grace, adasVoice); !errors.Is(err, media.ErrNotPermitted) {
		t.Errorf("subscribing without MaySubscribe gave %v, want ErrNotPermitted", err)
	}
	if err := f.Unsubscribe(ctx, theRoom, grace, adasVoice); err != nil {
		t.Errorf("unsubscribing from nothing gave %v, and it is idempotent", err)
	}
}

// TestSubscribingToAMemberWhoIsNotInTheRoomIsRefused, which is a different
// answer from the source not being published.
func TestSubscribingToAMemberWhoIsNotInTheRoomIsRefused(t *testing.T) {
	f, _ := newFake(t)
	ctx := context.Background()
	admitted(t, f)

	stranger := media.Source{Member: "nobody@example.org", Kind: media.Audio}
	if err := f.Subscribe(ctx, theRoom, grace, stranger); !errors.Is(err, media.ErrNoSuchMember) {
		t.Errorf("subscribing to a stranger gave %v, want ErrNoSuchMember", err)
	}
	if err := f.Subscribe(ctx, theRoom, grace, adasVoice); !errors.Is(err, media.ErrNoSuchSource) {
		t.Errorf("subscribing to a member who publishes nothing gave %v, want ErrNoSuchSource", err)
	}
}

// TestGainNotSupportedIsTheAnswerAForwardingUnitGives, and it comes before
// every other answer, because a unit that does not apply gain has nothing to say
// about whether the subscription exists.
func TestGainNotSupportedIsTheAnswerAForwardingUnitGives(t *testing.T) {
	f, _ := newFake(t)
	ctx := context.Background()
	admitted(t, f)

	if err := f.SetSubscriberGain(ctx, theRoom, grace, adasVoice, 1); !errors.Is(err, media.ErrGainNotSupported) {
		t.Errorf("a new fake answered %v, want ErrGainNotSupported", err)
	}
	if err := f.SetSubscriberGain(ctx, "channel-does-not-exist", grace, adasVoice, 1); !errors.Is(err, media.ErrGainNotSupported) {
		t.Errorf("the capability answer did not come first: %v", err)
	}
}

// TestAGainOutsideTheRangeIsRefusedRatherThanClamped. A clamped gain is a
// setting the layer above believes it applied.
func TestAGainOutsideTheRangeIsRefusedRatherThanClamped(t *testing.T) {
	f, _ := newFake(t)
	ctx := context.Background()
	admitted(t, f)
	f.SetGainSupported(true)
	f.SetGainRange(0, 2)

	if err := f.PublishSource(ctx, theRoom, ada, media.Audio); err != nil {
		t.Fatalf("PublishSource: %v", err)
	}
	if err := f.Subscribe(ctx, theRoom, grace, adasVoice); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	if err := f.SetSubscriberGain(ctx, theRoom, grace, adasVoice, 2); err != nil {
		t.Errorf("the top of the range gave %v, and the range is inclusive", err)
	}
	if err := f.SetSubscriberGain(ctx, theRoom, grace, adasVoice, 2.5); !errors.Is(err, media.ErrGainOutOfRange) {
		t.Errorf("a gain above the range gave %v, want ErrGainOutOfRange", err)
	}
	if err := f.SetSubscriberGain(ctx, theRoom, grace, adasVoice, -1); !errors.Is(err, media.ErrGainOutOfRange) {
		t.Errorf("a negative gain gave %v, want ErrGainOutOfRange", err)
	}
}

// TestSpeakingStateAndHopMetricsReturnWhatATestSet, in a fixed order, so an
// assertion is on a value rather than on a set.
func TestSpeakingStateAndHopMetricsReturnWhatATestSet(t *testing.T) {
	f, clock := newFake(t)
	ctx := context.Background()
	admitted(t, f)

	f.SetSpeaking(theRoom, ada, true)
	speaking, err := f.SpeakingState(ctx, theRoom)
	if err != nil {
		t.Fatalf("SpeakingState: %v", err)
	}
	want := []media.Speaking{{Member: ada, Speaking: true}, {Member: grace, Speaking: false}}
	if !reflect.DeepEqual(speaking, want) {
		t.Errorf("SpeakingState is %v, want %v", speaking, want)
	}

	clock.Advance(90 * time.Second)
	f.SetHop(theRoom, media.Hop{Member: ada, Delay: 12 * time.Millisecond, Loss: 0.01})

	hops, err := f.HopMetrics(ctx, theRoom)
	if err != nil {
		t.Fatalf("HopMetrics: %v", err)
	}
	if len(hops) != 2 {
		t.Fatalf("HopMetrics returned %d hops for two members", len(hops))
	}
	sort.Slice(hops, func(i, j int) bool { return hops[i].Member < hops[j].Member })
	if hops[0].Member != ada || hops[0].Delay != 12*time.Millisecond || hops[0].Loss != 0.01 {
		t.Errorf("the hop set is %+v", hops[0])
	}
	if !hops[0].Observed.Equal(clock.Now()) {
		t.Errorf("the observation is stamped %v and the clock reads %v", hops[0].Observed, clock.Now())
	}
}

// TestAUnitThatObservesNothingSaysSo. ErrNotObservable is a capability answer
// rather than a fault, and a fake that could not produce it would leave the
// handling of it untested.
func TestAUnitThatObservesNothingSaysSo(t *testing.T) {
	f, _ := newFake(t)
	ctx := context.Background()
	admitted(t, f)

	f.SetObservable(false)
	if _, err := f.SpeakingState(ctx, theRoom); !errors.Is(err, media.ErrNotObservable) {
		t.Errorf("SpeakingState gave %v, want ErrNotObservable", err)
	}
	if _, err := f.HopMetrics(ctx, theRoom); !errors.Is(err, media.ErrNotObservable) {
		t.Errorf("HopMetrics gave %v, want ErrNotObservable", err)
	}
	if _, err := f.SpeakingState(ctx, "channel-does-not-exist"); !errors.Is(err, media.ErrNoSuchRoom) {
		t.Errorf("a room that does not exist gave %v, want ErrNoSuchRoom", err)
	}
	if _, err := f.HopMetrics(ctx, "channel-does-not-exist"); !errors.Is(err, media.ErrNoSuchRoom) {
		t.Errorf("a room that does not exist gave %v, want ErrNoSuchRoom", err)
	}
}

// TestDrainingRoomStopsAdmittingAndSaysWhoIsStillIn. Drain is the graceful path
// and Destroy is the abrupt one, and a drain that gave up and then let somebody
// back in would be a shutdown that never finishes.
func TestDrainingRoomStopsAdmittingAndSaysWhoIsStillIn(t *testing.T) {
	f, _ := newFake(t)
	ctx := context.Background()
	admitted(t, f)

	if err := f.DrainRoom(ctx, theRoom); !errors.Is(err, media.ErrDrainDeadlineExceeded) {
		t.Fatalf("draining a room with members in it gave %v, want ErrDrainDeadlineExceeded", err)
	}
	if _, err := f.AdmitMember(ctx, theRoom, "late@example.org", 0); err == nil {
		t.Error("a draining room admitted a new member")
	}

	for _, m := range []media.MemberID{ada, grace} {
		if err := f.RevokeMember(ctx, theRoom, m); err != nil {
			t.Fatalf("RevokeMember %s: %v", m, err)
		}
	}
	if err := f.DrainRoom(ctx, theRoom); err != nil {
		t.Errorf("draining an empty room gave %v", err)
	}
	if err := f.DrainRoom(ctx, "channel-does-not-exist"); !errors.Is(err, media.ErrNoSuchRoom) {
		t.Errorf("draining a room that does not exist gave %v, want ErrNoSuchRoom", err)
	}
}

// TestACallersExpiredDeadlineIsAnsweredBeforeAnythingElse. It is the caller's
// own deadline rather than the unit's, so the answer is the same whatever state
// the fake is in.
func TestACallersExpiredDeadlineIsAnsweredBeforeAnythingElse(t *testing.T) {
	f, _ := newFake(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := f.CreateRoom(ctx, theRoom); !errors.Is(err, media.ErrCancelled) {
		t.Errorf("CreateRoom with an expired deadline gave %v, want ErrCancelled", err)
	}
	if _, err := f.DestroyRoom(ctx, theRoom); !errors.Is(err, media.ErrCancelled) {
		t.Errorf("DestroyRoom with an expired deadline gave %v, want ErrCancelled", err)
	}
	if got := f.Operations(); !reflect.DeepEqual(got, []string{"CreateRoom", "DestroyRoom"}) {
		t.Errorf("a cancelled call was not recorded: %v", got)
	}
}

// TestAnIdentityThisPortWillNotNameAnythingByIsRefusedByTheFakeToo, which is the
// shared refusal in media rather than a second copy of it here.
func TestAnIdentityThisPortWillNotNameAnythingByIsRefusedByTheFakeToo(t *testing.T) {
	f, _ := newFake(t)
	ctx := context.Background()
	admitted(t, f)

	if err := f.CreateRoom(ctx, ""); !errors.Is(err, media.ErrInvalidIdentity) {
		t.Errorf("CreateRoom(\"\") gave %v", err)
	}
	if _, err := f.DestroyRoom(ctx, ""); !errors.Is(err, media.ErrInvalidIdentity) {
		t.Errorf("DestroyRoom(\"\") gave %v", err)
	}
	if _, err := f.AdmitMember(ctx, "", ada, 0); !errors.Is(err, media.ErrInvalidIdentity) {
		t.Errorf("AdmitMember with no room gave %v", err)
	}
	if _, err := f.AdmitMember(ctx, theRoom, "", 0); !errors.Is(err, media.ErrInvalidIdentity) {
		t.Errorf("AdmitMember with no member gave %v", err)
	}
	if err := f.PublishSource(ctx, theRoom, ada, 0); !errors.Is(err, media.ErrInvalidIdentity) {
		t.Errorf("PublishSource with no kind gave %v", err)
	}
	if err := f.UnpublishSource(ctx, theRoom, ada, 0); !errors.Is(err, media.ErrInvalidIdentity) {
		t.Errorf("UnpublishSource with no kind gave %v", err)
	}
	if err := f.Subscribe(ctx, theRoom, grace, media.Source{Member: ada}); !errors.Is(err, media.ErrInvalidIdentity) {
		t.Errorf("Subscribe with no kind gave %v", err)
	}
	if err := f.Unsubscribe(ctx, theRoom, grace, media.Source{Member: ada}); !errors.Is(err, media.ErrInvalidIdentity) {
		t.Errorf("Unsubscribe with no kind gave %v", err)
	}
}

// TestAMemberScopedOperationOnARoomThatIsNotThereSaysSo, and says the member is
// missing rather than the room when the room is there.
func TestAMemberScopedOperationOnARoomThatIsNotThereSaysSo(t *testing.T) {
	f, _ := newFake(t)
	ctx := context.Background()
	admitted(t, f)

	const gone = media.RoomID("channel-does-not-exist")
	for name, err := range map[string]error{
		"PublishSource":   f.PublishSource(ctx, gone, ada, media.Audio),
		"UnpublishSource": f.UnpublishSource(ctx, gone, ada, media.Audio),
		"Subscribe":       f.Subscribe(ctx, gone, grace, adasVoice),
		"Unsubscribe":     f.Unsubscribe(ctx, gone, grace, adasVoice),
		"RevokeMember":    f.RevokeMember(ctx, gone, ada),
	} {
		if !errors.Is(err, media.ErrNoSuchRoom) {
			t.Errorf("%s on a room that does not exist gave %v, want ErrNoSuchRoom", name, err)
		}
	}

	const stranger = media.MemberID("nobody@example.org")
	for name, err := range map[string]error{
		"PublishSource":   f.PublishSource(ctx, theRoom, stranger, media.Audio),
		"UnpublishSource": f.UnpublishSource(ctx, theRoom, stranger, media.Audio),
		"Subscribe":       f.Subscribe(ctx, theRoom, stranger, adasVoice),
		"Unsubscribe":     f.Unsubscribe(ctx, theRoom, stranger, adasVoice),
	} {
		if !errors.Is(err, media.ErrNoSuchMember) {
			t.Errorf("%s for a member who is not in the room gave %v, want ErrNoSuchMember", name, err)
		}
	}
}

// TestTheReadsAnswerFalseForARoomThatIsNotThere, so a test using them to assert
// an absence is not reading a room it never made.
func TestTheReadsAnswerFalseForARoomThatIsNotThere(t *testing.T) {
	f, _ := newFake(t)
	if f.Admitted("channel-does-not-exist", ada) {
		t.Error("Admitted answered true for a room that does not exist")
	}
	if f.Published("channel-does-not-exist", adasVoice) {
		t.Error("Published answered true for a room that does not exist")
	}
	if f.Subscribed("channel-does-not-exist", grace, adasVoice) {
		t.Error("Subscribed answered true for a room that does not exist")
	}
}

// TestACallPrintsItselfForAFailingAssertion.
func TestACallPrintsItselfForAFailingAssertion(t *testing.T) {
	f, _ := newFake(t)
	ctx := context.Background()
	admitted(t, f)
	if err := f.Subscribe(ctx, theRoom, grace, adasVoice); err == nil {
		t.Fatal("the fixture subscribed, so there is no refusal to print")
	}

	last := f.Calls()[len(f.Calls())-1]
	printed := last.String()
	for _, want := range []string{"Subscribe", string(theRoom), string(grace), "audio", "no such source"} {
		if !strings.Contains(printed, want) {
			t.Errorf("the printed call %q does not carry %q", printed, want)
		}
	}

	plain := fake.Call{Op: "CreateRoom", Room: theRoom}
	if plain.String() != "CreateRoom(channel-1)" {
		t.Errorf("a call with no member and no source prints as %q", plain.String())
	}
}
