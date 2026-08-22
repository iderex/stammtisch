// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026  Nils Lehnen

package auth

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"golang.org/x/crypto/argon2"
)

// The floor. This is not a test that pins the current values, which is the
// shape that reds when somebody strengthens the parameters and so gets relaxed
// the first time it is in the way. It refuses a lowering and passes a raising,
// which is what issue #28 asks for in those words.
//
// The numbers below are written out rather than read from the constants,
// because a test comparing a constant to itself proves nothing. Moving one of
// these is the deliberate act of arguing the floor down, in a change whose
// whole subject is that argument.
func TestTheCostParametersAreNeverLowered(t *testing.T) {
	if argonMemory < 64*1024 {
		t.Errorf("memory is %d KiB and the floor is %d KiB", argonMemory, 64*1024)
	}
	if argonTime < 3 {
		t.Errorf("passes is %d and the floor is 3", argonTime)
	}
	if argonThreads < 4 {
		t.Errorf("lanes is %d and the floor is 4", argonThreads)
	}
	if argonKeyLen < 32 {
		t.Errorf("the key is %d bytes and the floor is 32", argonKeyLen)
	}
	if saltLen < 16 {
		t.Errorf("the salt is %d bytes and the floor is 16", saltLen)
	}
}

func TestAStoredCredentialMatchesItselfAndNothingElse(t *testing.T) {
	stored, err := Store("a passphrase nobody else has")
	if err != nil {
		t.Fatalf("Store: %v", err)
	}

	ok, err := Matches(stored, "a passphrase nobody else has")
	if err != nil {
		t.Fatalf("Matches: %v", err)
	}
	if !ok {
		t.Error("the credential did not match itself")
	}

	for _, wrong := range []string{
		"",
		"a passphrase nobody else has ",
		"A passphrase nobody else has",
		"a passphrase nobody else ha",
	} {
		ok, err := Matches(stored, wrong)
		if err != nil {
			t.Fatalf("Matches(%q): %v", wrong, err)
		}
		if ok {
			t.Errorf("%q matched", wrong)
		}
	}
}

// The salt is what makes two records of one credential different. Without it a
// stored value is a lookup key, and two accounts sharing a passphrase are
// visible to anybody holding the file.
func TestTheSameCredentialStoresDifferentlyEachTime(t *testing.T) {
	first, err := Store("the same credential")
	if err != nil {
		t.Fatalf("Store: %v", err)
	}
	second, err := Store("the same credential")
	if err != nil {
		t.Fatalf("Store: %v", err)
	}

	if first == second {
		t.Fatal("two stores of one credential produced the same value, so no salt is being drawn")
	}

	for _, stored := range []string{first, second} {
		ok, err := Matches(stored, "the same credential")
		if err != nil || !ok {
			t.Errorf("a stored value did not verify: %v, %v", ok, err)
		}
	}
}

// The stored form carries the cost it was written at, so raising the floor does
// not invalidate what is already held. This builds a value at a cost below the
// current constants and requires it to verify.
func TestAValueStoredAtALowerCostStillVerifies(t *testing.T) {
	salt := []byte("sixteen byte salt")[:saltLen]
	const old = uint32(1)
	key := argon2.IDKey([]byte("an old credential"), salt, old, 8*1024, 1, argonKeyLen)
	stored := fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, 8*1024, old, 1, b64.EncodeToString(salt), b64.EncodeToString(key))

	ok, err := Matches(stored, "an old credential")
	if err != nil {
		t.Fatalf("Matches: %v", err)
	}
	if !ok {
		t.Error("a value stored at a lower cost did not verify, so raising the cost would lock everybody out")
	}
}

func TestTheStoredFormIsTheReferenceOne(t *testing.T) {
	stored, err := Store("anything")
	if err != nil {
		t.Fatalf("Store: %v", err)
	}

	want := fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$", argon2.Version, argonMemory, argonTime, argonThreads)
	if !strings.HasPrefix(stored, want) {
		t.Errorf("stored value begins %q, want the prefix %q", stored, want)
	}
	if n := strings.Count(stored, "$"); n != 5 {
		t.Errorf("stored value has %d separators, want 5", n)
	}
}

// A malformed stored value is an error and never a false, because those are
// different statements and collapsing them turns a corrupted record into a
// login that merely fails.
func TestAMalformedStoredValueIsAnErrorRatherThanAMismatch(t *testing.T) {
	good, err := Store("anything")
	if err != nil {
		t.Fatalf("Store: %v", err)
	}
	field := strings.Split(good, "$")

	cases := []struct {
		name   string
		stored string
	}{
		{"empty", ""},
		{"not the stored form at all", "argon2id"},
		{"too few fields", "$argon2id$v=19$m=65536,t=3,p=4$c2FsdA"},
		{"no leading separator", strings.TrimPrefix(good, "$")},
		{"another variant", "$argon2i$" + strings.Join(field[2:], "$")},
		{"a version field that is not one", "$argon2id$version$" + strings.Join(field[3:], "$")},
		{"a version this build does not implement", "$argon2id$v=1$" + strings.Join(field[3:], "$")},
		{"a cost field that is not one", "$argon2id$" + field[2] + "$cheap$" + strings.Join(field[4:], "$")},
		{"a cost of zero", "$argon2id$" + field[2] + "$m=0,t=3,p=4$" + strings.Join(field[4:], "$")},
		{"a salt that is not base64", "$argon2id$" + field[2] + "$" + field[3] + "$not base64!$" + field[5]},
		{"a key that is not base64", "$argon2id$" + field[2] + "$" + field[3] + "$" + field[4] + "$not base64!"},
		{"an empty salt", "$argon2id$" + field[2] + "$" + field[3] + "$$" + field[5]},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ok, err := Matches(c.stored, "anything")
			if !errors.Is(err, ErrMalformedCredential) {
				t.Fatalf("Matches returned %v, want ErrMalformedCredential", err)
			}
			if ok {
				t.Error("a malformed value reported a match")
			}
		})
	}
}

func TestStoreReportsAFailedSalt(t *testing.T) {
	sentinel := errors.New("no entropy")
	original := randRead
	randRead = func([]byte) (int, error) { return 0, sentinel }
	t.Cleanup(func() { randRead = original })

	if _, err := Store("anything"); !errors.Is(err, sentinel) {
		t.Fatalf("Store returned %v, want the reader's own error", err)
	}
}
