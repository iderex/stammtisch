// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026  Nils Lehnen

package signalling

import (
	"bytes"
	"encoding/binary"
	"math/rand/v2"
	"runtime"
	"testing"
)

// The properties the decoder holds regardless of input, each as its own test
// over the same inputs, so a failure names the property that broke rather than
// the case that happened to reach it.
//
// Every test below draws from inputs(), which produces two families. Arbitrary
// bytes are what an unauthenticated peer can send. Structurally valid but
// semantically absurd messages are the ones that get past a decoder's first
// gate and are where the interesting mistakes are: a well-formed header
// declaring more than the machine has, a header whose payload never arrives, a
// frame followed by rubbish.
//
// The seed is fixed. A property test that draws differently on every run
// reports a failure somebody cannot reproduce, and the coverage-guided search
// over inputs nobody wrote down is the fuzz target beside this file, which is
// where new inputs are supposed to come from.
const propertyDraws = 4000

// inputs returns the draws, structured ones first and arbitrary bytes after.
// The structured half is built from a real header so it reaches past the parts
// of Decode that arbitrary bytes mostly bounce off.
//
// The order is not cosmetic. Every test here stops at its first violation, and
// a violation found in the structured half is one whose declared length is
// capped below. Arbitrary bytes can declare any length a uint32 holds, so a run
// with a guard deleted would otherwise find its first failure by allocating
// whatever four random bytes asked for.
func inputs(random *rand.Rand) [][]byte {
	drawn := make([][]byte, 0, propertyDraws)

	for i := 0; i < propertyDraws/2; i++ {
		// A declared length drawn around the bound rather than across the whole
		// uint32 range. The largest number a header can express has its own
		// test, which allocates nothing either way; here the draws are capped
		// a megabyte above the bound so that the run somebody does with the
		// bound deleted, to watch the allocation property red, costs a
		// megabyte rather than the machine. The comparison being proved does
		// not care which side of it a number falls on by how much.
		var declared uint32
		switch random.IntN(4) {
		case 0:
			declared = uint32(random.IntN(40))
		case 1:
			declared = MaxPayloadSize - uint32(random.IntN(3))
		case 2:
			declared = MaxPayloadSize + uint32(random.IntN(3))
		default:
			declared = MaxPayloadSize + 1 + uint32(random.IntN(1<<20))
		}

		wire := make([]byte, headerSize)
		binary.BigEndian.PutUint32(wire[:4], declared)
		wire[4] = byte(random.UintN(4))

		// The payload that follows is unrelated to what the header declared,
		// which is the case the header is a claim about rather than a fact.
		body := make([]byte, random.IntN(80))
		for j := range body {
			body[j] = byte(random.UintN(256))
		}
		drawn = append(drawn, append(wire, body...))
	}

	for i := 0; i < propertyDraws/2; i++ {
		b := make([]byte, random.IntN(80))
		for j := range b {
			b[j] = byte(random.UintN(256))
		}
		drawn = append(drawn, b)
	}

	return drawn
}

// Property: Decode never panics.
//
// This one has no guard to remove and that is a statement about the decoder
// rather than a gap in the test. Nothing in Decode indexes or slices on a value
// the peer controls: the header is a fixed-size array, the kind is one byte of
// it, and the only quantity the peer chooses is the allocation size, which the
// property below covers. So there is no line whose deletion turns a draw here
// into a panic, and the honest form of the proof is the argument rather than a
// red run. The day the decoder grows a variable-length index, this test starts
// having a guard and the argument stops applying.
func TestPropertyDecodeNeverPanics(t *testing.T) {
	for _, wire := range inputs(rand.New(rand.NewPCG(1, 2))) {
		if _, err := Decode(bytes.NewReader(wire)); err != nil {
			continue
		}
	}
}

// Property: Decode never allocates beyond the declared bound.
//
// The guard is the comparison against MaxPayloadSize sitting ahead of the make
// in Decode. Delete it and this reds on the first draw whose header declares
// more than the slack below, because the allocation is then the peer's to
// choose.
func TestPropertyDecodeNeverAllocatesBeyondTheBound(t *testing.T) {
	// The bound plus room for the frame value, the reader and the error text.
	const slack = MaxPayloadSize + 64<<10

	for _, wire := range inputs(rand.New(rand.NewPCG(3, 4))) {
		r := bytes.NewReader(wire)

		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		_, err := Decode(r)
		runtime.ReadMemStats(&after)

		if grew := after.TotalAlloc - before.TotalAlloc; grew > slack {
			t.Fatalf("decoding % x allocated %d bytes, and the bound plus slack is %d (error was %v)", wire, grew, slack, err)
		}
	}
}

// Property: Decode returns a message or an error and never both.
//
// The guard is the Frame{} on every error return. Replace one of them with a
// partial result, which is what somebody does when they want the header they
// managed to read for a debugging message, and this reds.
func TestPropertyDecodeReturnsAMessageOrAnErrorNeverBoth(t *testing.T) {
	for _, wire := range inputs(rand.New(rand.NewPCG(5, 6))) {
		f, err := Decode(bytes.NewReader(wire))

		if err != nil && (f.Payload != nil || f.Kind != KindReserved) {
			t.Fatalf("decoding % x returned kind %d with a %d byte payload alongside %v", wire, f.Kind, len(f.Payload), err)
		}
		if err == nil && f.Kind == KindReserved {
			t.Fatalf("decoding % x returned neither a usable frame nor an error", wire)
		}
	}
}

// Property: encoding a decoded message reproduces the bytes it was read from.
//
// The guard is the encoding being canonical, which is one length written the
// one way and the kind byte in its place. Drop the kind byte from Encode's
// buffer and this reds, and so does anything that pads, reorders or writes the
// length differently from how Decode reads it.
func TestPropertyEncodingADecodedFrameReproducesItsBytes(t *testing.T) {
	for _, wire := range inputs(rand.New(rand.NewPCG(7, 8))) {
		r := bytes.NewReader(wire)
		f, err := Decode(r)
		if err != nil {
			continue
		}
		consumed := len(wire) - r.Len()

		var round bytes.Buffer
		if err := Encode(&round, f); err != nil {
			t.Fatalf("a frame Decode accepted would not re-encode: %v, from % x", err, wire)
		}
		if !bytes.Equal(round.Bytes(), wire[:consumed]) {
			t.Fatalf("re-encoding gave % x, want the % x it was read from", round.Bytes(), wire[:consumed])
		}
	}
}

// The other direction of the same property, over every message the encoder can
// produce rather than over the ones a random draw happened to decode. A round
// trip that only ever starts from bytes tests the shapes the decoder accepts,
// and this tests the shapes the encoder emits, which is the set a peer will
// actually receive.
func TestPropertyEveryEncodableFrameDecodesBackToItself(t *testing.T) {
	random := rand.New(rand.NewPCG(9, 10))

	for i := 0; i < 400; i++ {
		payload := make([]byte, random.IntN(MaxPayloadSize+1))
		for j := range payload {
			payload[j] = byte(random.UintN(256))
		}
		sent := Frame{Kind: Kind(1 + random.UintN(255)), Payload: payload}

		var wire bytes.Buffer
		if err := Encode(&wire, sent); err != nil {
			t.Fatalf("Encode refused a frame it can produce: %v", err)
		}

		got, err := Decode(bytes.NewReader(wire.Bytes()))
		if err != nil {
			t.Fatalf("Decode refused what Encode wrote: %v", err)
		}
		if got.Kind != sent.Kind || !bytes.Equal(got.Payload, sent.Payload) {
			t.Fatalf("round trip gave kind %d and %d bytes, want kind %d and %d", got.Kind, len(got.Payload), sent.Kind, len(sent.Payload))
		}
	}
}
