// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026  Nils Lehnen

package signalling

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

// stream is a byte stream with a separate read side and write side, which is
// what a connection is. No socket, no pipe, no goroutine.
type stream struct {
	in  *bytes.Reader
	out bytes.Buffer
}

func newStream(peerSends ...Frame) *stream {
	var wire bytes.Buffer
	for _, f := range peerSends {
		if err := Encode(&wire, f); err != nil {
			panic(err)
		}
	}
	return &stream{in: bytes.NewReader(wire.Bytes())}
}

func (s *stream) Read(p []byte) (int, error)  { return s.in.Read(p) }
func (s *stream) Write(p []byte) (int, error) { return s.out.Write(p) }

func TestAnUnauthenticatedConnectionAcceptsTheAuthenticationFrame(t *testing.T) {
	c := NewConn(newStream(Frame{Kind: KindAuthenticate, Payload: []byte("credential")}))

	if c.Authenticated() {
		t.Fatal("a new connection reports itself authenticated")
	}

	f, err := c.ReadFrame()
	if err != nil {
		t.Fatalf("ReadFrame: %v", err)
	}
	if f.Kind != KindAuthenticate {
		t.Errorf("kind %d, want %d", f.Kind, KindAuthenticate)
	}
}

func TestAnUnauthenticatedConnectionAcceptsNothingElse(t *testing.T) {
	c := NewConn(newStream(Frame{Kind: KindSpaceState, Payload: []byte("state")}))

	_, err := c.ReadFrame()
	if !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("ReadFrame returned %v, want ErrUnauthenticated", err)
	}
}

// The frame after the refused one is a valid authentication frame, so a gate
// that refuses one frame and then reads the next would return it and this test
// would pass with the connection wide open. That is the whole reason the
// refusal is sticky, and it is what this asserts.
func TestARefusalIsTerminalRatherThanPerFrame(t *testing.T) {
	c := NewConn(newStream(
		Frame{Kind: KindSpaceState, Payload: []byte("state")},
		Frame{Kind: KindAuthenticate, Payload: []byte("credential")},
	))

	first, err := c.ReadFrame()
	if !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("the first ReadFrame returned %v, want ErrUnauthenticated", err)
	}
	if first.Payload != nil {
		t.Error("a refused read returned a payload")
	}

	second, err := c.ReadFrame()
	if !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("the second ReadFrame returned %v, want the same refusal", err)
	}
	if second.Kind == KindAuthenticate {
		t.Fatal("the connection went on reading after refusing a peer that had proved nothing")
	}
	if c.Authenticated() {
		t.Error("a refused connection reports itself authenticated")
	}
}

func TestAnAuthenticatedConnectionAcceptsEveryKind(t *testing.T) {
	c := NewConn(newStream(Frame{Kind: KindSpaceState, Payload: []byte("state")}))
	c.MarkAuthenticated()

	if !c.Authenticated() {
		t.Fatal("MarkAuthenticated did not open the gate")
	}

	f, err := c.ReadFrame()
	if err != nil {
		t.Fatalf("ReadFrame: %v", err)
	}
	if f.Kind != KindSpaceState {
		t.Errorf("kind %d, want %d", f.Kind, KindSpaceState)
	}
}

func TestReadFrameReportsADecodeFailure(t *testing.T) {
	c := NewConn(newStream())

	if _, err := c.ReadFrame(); !errors.Is(err, io.EOF) {
		t.Fatalf("ReadFrame returned %v, want io.EOF", err)
	}
}

func TestWriteFramePutsTheFrameOnTheStream(t *testing.T) {
	s := newStream()
	c := NewConn(s)

	if err := c.WriteFrame(Frame{Kind: KindSpaceState, Payload: []byte("state")}); err != nil {
		t.Fatalf("WriteFrame: %v", err)
	}

	f, err := Decode(bytes.NewReader(s.out.Bytes()))
	if err != nil {
		t.Fatalf("decoding what was written: %v", err)
	}
	if f.Kind != KindSpaceState || !bytes.Equal(f.Payload, []byte("state")) {
		t.Errorf("wrote kind %d payload %q", f.Kind, f.Payload)
	}
}

func TestWriteFrameReportsARefusedFrame(t *testing.T) {
	c := NewConn(newStream())

	if err := c.WriteFrame(Frame{Kind: KindReserved}); !errors.Is(err, ErrKindReserved) {
		t.Fatalf("WriteFrame returned %v, want ErrKindReserved", err)
	}
}
