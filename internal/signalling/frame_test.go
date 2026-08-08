// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026  iderex

package signalling

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"math/rand/v2"
	"runtime"
	"testing"
)

// countingReader serves n bytes of a header and then stops. It records how much
// was asked of it, which is what turns "did not allocate" into an assertion
// about the bytes that were read rather than about how the function felt.
type countingReader struct {
	remaining []byte
	read      int
}

func (r *countingReader) Read(p []byte) (int, error) {
	if len(r.remaining) == 0 {
		return 0, io.EOF
	}
	n := copy(p, r.remaining)
	r.remaining = r.remaining[n:]
	r.read += n
	return n, nil
}

// failingWriter refuses everything, which is the only way to reach Encode's
// write-error branch without a socket.
type failingWriter struct{ err error }

func (w failingWriter) Write(p []byte) (int, error) { return 0, w.err }

// header builds the five bytes of a frame header declaring the given length and
// kind, without going through Encode, so a test can declare a length that
// Encode would refuse to write.
func header(declared uint32, kind Kind) []byte {
	h := make([]byte, headerSize)
	binary.BigEndian.PutUint32(h[:4], declared)
	h[4] = byte(kind)
	return h
}

func TestEncodeAndDecodeRoundTripEveryShapeOfFrame(t *testing.T) {
	cases := []struct {
		name  string
		frame Frame
	}{
		{"an empty payload", Frame{Kind: KindAuthenticate, Payload: []byte{}}},
		{"one byte", Frame{Kind: KindAuthenticate, Payload: []byte{0x00}}},
		{"a payload of zeroes", Frame{Kind: KindSpaceState, Payload: make([]byte, 300)}},
		{"the largest payload the bound allows", Frame{Kind: KindSpaceState, Payload: bytes.Repeat([]byte{0xff}, MaxPayloadSize)}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := Encode(&buf, c.frame); err != nil {
				t.Fatalf("Encode: %v", err)
			}
			if got, want := buf.Len(), headerSize+len(c.frame.Payload); got != want {
				t.Errorf("encoded %d bytes, want %d", got, want)
			}

			got, err := Decode(&buf)
			if err != nil {
				t.Fatalf("Decode: %v", err)
			}
			if got.Kind != c.frame.Kind {
				t.Errorf("kind %d, want %d", got.Kind, c.frame.Kind)
			}
			if !bytes.Equal(got.Payload, c.frame.Payload) {
				t.Errorf("payload of %d bytes, want %d", len(got.Payload), len(c.frame.Payload))
			}
			if buf.Len() != 0 {
				t.Errorf("%d bytes left after the frame, want none", buf.Len())
			}
		})
	}
}

// The guard this test exists for is the comparison against MaxPayloadSize
// sitting ahead of the make. Delete that comparison and this test reds on the
// heap figure: the declared size below is allocated in full for a frame that
// never arrives.
//
// The declared size is 64 MiB rather than the largest a header can express,
// because this is the test somebody runs with the guard deleted to watch it
// fail, and that run should cost a moment rather than the machine.
func TestDecodeRefusesAnOversizedFrameWithoutAllocatingForIt(t *testing.T) {
	const declared = 64 << 20
	r := &countingReader{remaining: header(declared, KindSpaceState)}

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)

	_, err := Decode(r)

	runtime.ReadMemStats(&after)
	grew := after.TotalAlloc - before.TotalAlloc

	// Errorf rather than Fatalf, so a run with the bound deleted reports the
	// allocation as well as the wrong error. The allocation is the finding; the
	// error is how it is noticed.
	if !errors.Is(err, ErrFrameTooLarge) {
		t.Errorf("Decode returned %v, want ErrFrameTooLarge", err)
	}
	if r.read != headerSize {
		t.Errorf("read %d bytes, want the %d of the header and no more", r.read, headerSize)
	}
	if grew > 1<<20 {
		t.Errorf("the heap grew by %d bytes deciding to refuse a %d byte frame, which is the allocation the bound exists to skip", grew, declared)
	}
}

func TestDecodeRefusesTheLargestLengthAHeaderCanExpress(t *testing.T) {
	r := &countingReader{remaining: header(math.MaxUint32, KindSpaceState)}

	if _, err := Decode(r); !errors.Is(err, ErrFrameTooLarge) {
		t.Fatalf("Decode returned %v, want ErrFrameTooLarge", err)
	}
	if r.read != headerSize {
		t.Errorf("read %d bytes, want the %d of the header and no more", r.read, headerSize)
	}
}

func TestDecodeAcceptsAFrameExactlyOnTheBound(t *testing.T) {
	body := bytes.Repeat([]byte{0x7f}, MaxPayloadSize)
	wire := append(header(MaxPayloadSize, KindSpaceState), body...)

	f, err := Decode(bytes.NewReader(wire))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if len(f.Payload) != MaxPayloadSize {
		t.Errorf("payload of %d bytes, want %d", len(f.Payload), MaxPayloadSize)
	}
}

func TestDecodeRefusesOneByteOverTheBound(t *testing.T) {
	r := &countingReader{remaining: header(MaxPayloadSize+1, KindSpaceState)}

	if _, err := Decode(r); !errors.Is(err, ErrFrameTooLarge) {
		t.Fatalf("Decode returned %v, want ErrFrameTooLarge", err)
	}
}

func TestDecodeRefusesTheReservedKind(t *testing.T) {
	wire := header(0, KindReserved)

	if _, err := Decode(bytes.NewReader(wire)); !errors.Is(err, ErrKindReserved) {
		t.Fatalf("Decode returned %v, want ErrKindReserved", err)
	}
}

func TestDecodeRefusesATruncatedHeader(t *testing.T) {
	wire := header(4, KindAuthenticate)[:headerSize-1]

	_, err := Decode(bytes.NewReader(wire))
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("Decode returned %v, want io.ErrUnexpectedEOF", err)
	}
}

func TestDecodeRefusesAPayloadShorterThanItsHeaderDeclared(t *testing.T) {
	wire := append(header(8, KindAuthenticate), 1, 2, 3)

	_, err := Decode(bytes.NewReader(wire))
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("Decode returned %v, want io.ErrUnexpectedEOF", err)
	}
}

func TestDecodeReportsAnEmptyStreamAsEOF(t *testing.T) {
	_, err := Decode(bytes.NewReader(nil))
	if !errors.Is(err, io.EOF) {
		t.Fatalf("Decode returned %v, want io.EOF", err)
	}
}

func TestEncodeRefusesAPayloadOverTheBound(t *testing.T) {
	f := Frame{Kind: KindSpaceState, Payload: make([]byte, MaxPayloadSize+1)}

	var buf bytes.Buffer
	if err := Encode(&buf, f); !errors.Is(err, ErrFrameTooLarge) {
		t.Fatalf("Encode returned %v, want ErrFrameTooLarge", err)
	}
	if buf.Len() != 0 {
		t.Errorf("wrote %d bytes for a refused frame, want none", buf.Len())
	}
}

func TestEncodeRefusesTheReservedKind(t *testing.T) {
	var buf bytes.Buffer
	if err := Encode(&buf, Frame{Kind: KindReserved}); !errors.Is(err, ErrKindReserved) {
		t.Fatalf("Encode returned %v, want ErrKindReserved", err)
	}
}

func TestEncodeReportsAFailedWrite(t *testing.T) {
	sentinel := errors.New("the stream is gone")

	err := Encode(failingWriter{err: sentinel}, Frame{Kind: KindAuthenticate, Payload: []byte("x")})
	if !errors.Is(err, sentinel) {
		t.Fatalf("Encode returned %v, want the writer's own error", err)
	}
}

// The decoder is what an unauthenticated peer reaches first, so it is driven
// with bytes nobody chose. This is the deterministic half: a fixed seed, so a
// failure here reproduces exactly. The corpus-keeping half and the rest of the
// properties are issue #38.
//
// A panic is a failure because the test runner treats it as one; nothing here
// recovers. What is asserted beyond that is the property that costs most to get
// wrong: either a frame or an error, never both and never neither, and a frame
// that came back re-encodes to the bytes it was read from.
func TestDecodeSurvivesArbitraryBytes(t *testing.T) {
	random := rand.New(rand.NewPCG(0x5eed, 0x51de))

	for i := 0; i < 20000; i++ {
		wire := make([]byte, random.IntN(64))
		for j := range wire {
			wire[j] = byte(random.UintN(256))
		}

		r := bytes.NewReader(wire)
		f, err := Decode(r)
		if err != nil {
			if f.Payload != nil {
				t.Fatalf("Decode returned both a payload and %v for % x", err, wire)
			}
			continue
		}

		var round bytes.Buffer
		if err := Encode(&round, f); err != nil {
			t.Fatalf("a frame Decode accepted would not re-encode: %v for % x", err, wire)
		}
		consumed := len(wire) - r.Len()
		if !bytes.Equal(round.Bytes(), wire[:consumed]) {
			t.Fatalf("re-encoding gave % x, want the % x it was read from", round.Bytes(), wire[:consumed])
		}
	}
}
