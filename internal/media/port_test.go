// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026  Nils Lehnen

package media_test

import (
	"errors"
	"reflect"
	"sort"
	"testing"

	"github.com/iderex/stammtisch/internal/media"
)

// TestOperationsIsThePortsOwnMethodSet is what makes media.Operations() worth
// having.
//
// An implementation is judged against that list, and a list maintained by hand
// beside an interface drifts the first time an operation is added. This reads
// the method set off the interface type itself, so the two cannot disagree: an
// operation added to Port without the list reds here, and a name left in the
// list after an operation went reds here too.
func TestOperationsIsThePortsOwnMethodSet(t *testing.T) {
	port := reflect.TypeOf((*media.Port)(nil)).Elem()
	if port.NumMethod() == 0 {
		t.Fatal("the Port interface has no methods, so this test is reading the wrong type")
	}

	var declared []string
	for i := 0; i < port.NumMethod(); i++ {
		declared = append(declared, port.Method(i).Name)
	}
	sort.Strings(declared)

	listed := append([]string(nil), media.Operations()...)
	sort.Strings(listed)

	if !reflect.DeepEqual(declared, listed) {
		t.Errorf("Operations() and the interface disagree.\n  interface: %v\n  Operations: %v", declared, listed)
	}
}

// TestTheOperationListIsInTheOrderTheRecordUses. The sorted comparison above
// cannot see an order, and the order is what a reader of a call sequence
// compares against the specification.
func TestTheOperationListIsInTheOrderTheRecordUses(t *testing.T) {
	want := []string{
		"CreateRoom", "DestroyRoom", "AdmitMember", "RevokeMember",
		"PublishSource", "UnpublishSource", "Subscribe", "Unsubscribe",
		"SetSubscriberGain", "SpeakingState", "HopMetrics", "DrainRoom",
	}
	if got := media.Operations(); !reflect.DeepEqual(got, want) {
		t.Errorf("Operations() is %v, and docs/decisions/media-plane-port.md lists them as %v", got, want)
	}
}

// TestOperationsCannotBeChangedByACaller. It returns a slice, and a caller that
// held on to one and wrote into it would move what every implementation is
// judged against.
func TestOperationsCannotBeChangedByACaller(t *testing.T) {
	first := media.Operations()
	first[0] = "NotAnOperation"
	if second := media.Operations(); second[0] == "NotAnOperation" {
		t.Error("writing into the returned slice changed what the next caller gets")
	}
}

func TestAnIdentityThisPortWillNotNameAnythingByIsRefused(t *testing.T) {
	if err := media.ValidRoom(""); !errors.Is(err, media.ErrInvalidIdentity) {
		t.Errorf("an empty room identity gave %v, want ErrInvalidIdentity", err)
	}
	if err := media.ValidRoom("room-1"); err != nil {
		t.Errorf("a room identity was refused: %v", err)
	}
	if err := media.ValidMember(""); !errors.Is(err, media.ErrInvalidIdentity) {
		t.Errorf("an empty member identity gave %v, want ErrInvalidIdentity", err)
	}
	if err := media.ValidMember("ada@example.org"); err != nil {
		t.Errorf("a member identity was refused: %v", err)
	}
}

// TestASourceKindThisBuildDoesNotDefineIsRefusedRatherThanTreatedAsAudio. The
// near miss is the zero value, which is what a struct literal that forgot the
// kind carries, and treating it as audio is how a video source would be
// published as a voice one.
func TestASourceKindThisBuildDoesNotDefineIsRefusedRatherThanTreatedAsAudio(t *testing.T) {
	for _, kind := range []media.SourceKind{0, 3, 200} {
		if err := media.ValidKind(kind); !errors.Is(err, media.ErrInvalidIdentity) {
			t.Errorf("source kind %d gave %v, want ErrInvalidIdentity", kind, err)
		}
	}
	for _, kind := range []media.SourceKind{media.Audio, media.Video} {
		if err := media.ValidKind(kind); err != nil {
			t.Errorf("%v was refused: %v", kind, err)
		}
	}
	if media.Audio.String() != "audio" || media.Video.String() != "video" {
		t.Errorf("the kinds are named %q and %q", media.Audio.String(), media.Video.String())
	}
	if media.SourceKind(9).String() != "" {
		t.Errorf("a kind this build does not define is named %q", media.SourceKind(9).String())
	}
}

// TestTheEmptyCapabilitySetAllowsNothing. The zero value is what an admission
// with a forgotten argument carries, and it has to be the member who may do
// nothing rather than the member who may do everything.
func TestTheEmptyCapabilitySetAllowsNothing(t *testing.T) {
	var none media.Capabilities
	for _, c := range []media.Capability{media.MayPublishAudio, media.MayPublishVideo, media.MaySubscribe} {
		if none.Allows(c) {
			t.Errorf("the empty capability set allows %d", c)
		}
	}

	one := none.With(media.MaySubscribe)
	if !one.Allows(media.MaySubscribe) {
		t.Error("a set carrying MaySubscribe does not allow it")
	}
	if one.Allows(media.MayPublishAudio) || one.Allows(media.MayPublishVideo) {
		t.Errorf("adding MaySubscribe allowed something else as well: %d", one)
	}
}

// TestACredentialCarriesNothingACallerCanRead is the opacity rule at the one
// place a test can hold it: the type has no exported field and one accessor,
// which returns what a client presents and nothing about it.
func TestACredentialCarriesNothingACallerCanRead(t *testing.T) {
	var zero media.Credential
	if !zero.IsZero() {
		t.Error("the credential no admission produced does not report itself as one")
	}
	if zero.String() != "" {
		t.Errorf("the zero credential presents %q", zero.String())
	}

	cred := media.NewCredential("opaque-token")
	if cred.IsZero() {
		t.Error("a minted credential reports itself as the zero one")
	}
	if cred.String() != "opaque-token" {
		t.Errorf("the credential presents %q", cred.String())
	}

	if n := reflect.TypeOf(cred).NumField(); n != 1 {
		t.Errorf("Credential has %d fields, and the opacity rests on it having one unexported one", n)
	}
	if f := reflect.TypeOf(cred).Field(0); f.IsExported() {
		t.Errorf("Credential's field %s is exported, so a caller can read the token apart", f.Name)
	}
}

// TestEveryErrorThisPortDeclaresIsDistinct. The error set is what an
// implementation is held to, and two sentinels that compared equal would let a
// caller branch on the wrong one without any test noticing.
func TestEveryErrorThisPortDeclaresIsDistinct(t *testing.T) {
	all := []error{
		media.ErrUnavailable, media.ErrCancelled, media.ErrInternal,
		media.ErrRoomLimitReached, media.ErrInvalidIdentity, media.ErrNoSuchRoom,
		media.ErrMemberAlreadyAdmitted, media.ErrPermissionsNotExpressible,
		media.ErrNoSuchMember, media.ErrNotPermitted, media.ErrSourceAlreadyPublished,
		media.ErrNoSuchSource, media.ErrGainNotSupported, media.ErrNoSuchSubscription,
		media.ErrGainOutOfRange, media.ErrNotObservable, media.ErrDrainDeadlineExceeded,
	}
	seen := map[string]bool{}
	for i, a := range all {
		if a == nil {
			t.Fatalf("error %d is nil", i)
		}
		if seen[a.Error()] {
			t.Errorf("two errors share the message %q", a.Error())
		}
		seen[a.Error()] = true
		for j, b := range all {
			if i != j && errors.Is(a, b) {
				t.Errorf("%v matches %v, so a caller branching on one catches the other", a, b)
			}
		}
	}
}
