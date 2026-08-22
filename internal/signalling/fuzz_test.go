// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026  Nils Lehnen

package signalling

import (
	"bytes"
	"testing"
)

// FuzzDecode is the coverage-guided half of driving the decoder with bytes
// nobody chose. Run without -fuzz it executes the seeds below and nothing else,
// which is what makes it safe in the unit job; the scheduled run that lets it
// evolve a corpus is issue #74, and issue #38 owns keeping what it finds.
//
// The seeds are the shapes a header can take that a hand-written test does not
// naturally produce: a length field at its extremes, the reserved kind, and a
// header cut short.
func FuzzDecode(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte{0, 0, 0, 0, 1})
	f.Add([]byte{0, 0, 0, 0, 0})
	f.Add([]byte{0xff, 0xff, 0xff, 0xff, 1})
	f.Add([]byte{0, 0, 0, 1, 2, 0x41})
	f.Add([]byte{0, 0, 0, 4, 2, 0x41})
	f.Add([]byte{0, 1, 0, 0, 2})

	f.Fuzz(func(t *testing.T, wire []byte) {
		r := bytes.NewReader(wire)
		frame, err := Decode(r)
		if err != nil {
			if frame.Payload != nil {
				t.Fatalf("Decode returned both a payload and %v", err)
			}
			return
		}

		var round bytes.Buffer
		if err := Encode(&round, frame); err != nil {
			t.Fatalf("a frame Decode accepted would not re-encode: %v", err)
		}
		consumed := len(wire) - r.Len()
		if !bytes.Equal(round.Bytes(), wire[:consumed]) {
			t.Fatalf("re-encoding gave % x, want the % x it was read from", round.Bytes(), wire[:consumed])
		}
	})
}
