// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026  Nils Lehnen

package auth

import "testing"

// FuzzStoredCredential drives the reader of a stored credential with arbitrary
// bytes.
//
// What is fuzzed is parse and not Matches. parse is the whole of the validation:
// it is the code that decides what a stored value means, and every branch that
// can be wrong about a value is in it. Matches adds one call to argon2 at the
// cost parameters the value carries, which is tens of milliseconds per
// execution, and a coverage-guided search needs millions of executions. Putting
// argon2 inside the loop would turn a search into a benchmark and find nothing.
//
// The properties asserted are the ones that cost most to get wrong. Bytes are
// never returned alongside an error, because a caller that reads them would be
// verifying against a value the parser rejected. And an accepted value never
// carries a zero cost or an empty salt or key, because those are the shapes that
// turn a memory-hard comparison into a fast one without failing.
//
// A panic is a failure because nothing here recovers.
func FuzzStoredCredential(f *testing.F) {
	good, err := Store("a credential")
	if err != nil {
		f.Fatalf("Store: %v", err)
	}
	f.Add(good)
	f.Add(decoy)
	f.Add("")
	f.Add("$argon2id$v=19$m=65536,t=3,p=4$c2FsdA$a2V5")
	f.Add("$argon2id$v=19$m=0,t=3,p=4$c2FsdA$a2V5")
	f.Add("$argon2i$v=19$m=65536,t=3,p=4$c2FsdA$a2V5")
	f.Add("$argon2id$v=19$m=65536,t=3,p=4$$")
	f.Add("$$$$$")

	f.Fuzz(func(t *testing.T, stored string) {
		p, salt, key, err := parse(stored)
		if err != nil {
			if salt != nil || key != nil {
				t.Fatalf("parse returned %d salt bytes and %d key bytes alongside %v", len(salt), len(key), err)
			}
			return
		}

		if p.memory == 0 || p.time == 0 || p.threads == 0 {
			t.Fatalf("parse accepted a cost of m=%d,t=%d,p=%d, which is not a cost", p.memory, p.time, p.threads)
		}
		if len(salt) == 0 || len(key) == 0 {
			t.Fatalf("parse accepted a value with %d salt bytes and %d key bytes", len(salt), len(key))
		}
	})
}
