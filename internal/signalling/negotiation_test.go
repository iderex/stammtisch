// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026  Nils Lehnen

package signalling

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"strconv"
	"strings"
	"testing"
)

// duplex is a connection's two halves with the read side loaded up front, and a
// write side that can be told to fail. No socket and no goroutine.
type duplex struct {
	read     *bytes.Reader
	written  bytes.Buffer
	writeErr error
}

// errWriteRefused is what a duplex returns when it has been told to fail, and
// it stands for a peer that went away between the read and the answer.
var errWriteRefused = errors.New("the write side refused")

func peerSending(frames ...Frame) *duplex {
	var wire bytes.Buffer
	for _, f := range frames {
		if err := Encode(&wire, f); err != nil {
			panic(err)
		}
	}
	return &duplex{read: bytes.NewReader(wire.Bytes())}
}

func (d *duplex) Read(p []byte) (int, error) { return d.read.Read(p) }

func (d *duplex) Write(p []byte) (int, error) {
	if d.writeErr != nil {
		return 0, d.writeErr
	}
	return d.written.Write(p)
}

// answer decodes the one frame the server wrote back.
func (d *duplex) answer(t *testing.T) Frame {
	t.Helper()
	f, err := Decode(bytes.NewReader(d.written.Bytes()))
	if err != nil {
		t.Fatalf("decoding what the server wrote back: %v", err)
	}
	return f
}

// payloadOf builds a message payload out of the fields given, so a test can
// write a hello or a refusal this package would never produce.
func payloadOf(fields ...field) []byte { return encodeFields(fields) }

func versionValue(v Version) []byte {
	var b [2]byte
	binary.BigEndian.PutUint16(b[:], uint16(v))
	return b[:]
}

// The three cases the negotiation exists for. The version this build speaks is
// a range of one today, so older is every value below it and newer every value
// above it, and both tests are written against the constants rather than
// against the literal 1, so they keep asking the same question when the range
// widens.

func TestAClientProposingTheServersVersionIsAccepted(t *testing.T) {
	d := peerSending(Hello(MaxSupportedVersion))

	agreed, err := Negotiate(NewConn(d))
	if err != nil {
		t.Fatalf("Negotiate: %v", err)
	}
	if agreed != MaxSupportedVersion {
		t.Errorf("agreed on %d, want %d", agreed, MaxSupportedVersion)
	}

	read, err := AgreedVersion(d.answer(t))
	if err != nil {
		t.Fatalf("AgreedVersion: %v", err)
	}
	if read != MaxSupportedVersion {
		t.Errorf("the server sent back %d, want %d", read, MaxSupportedVersion)
	}
}

func TestAClientOlderThanTheServerIsRefused(t *testing.T) {
	d := peerSending(Hello(MinSupportedVersion - 1))

	if _, err := Negotiate(NewConn(d)); !errors.Is(err, ErrVersionUnsupported) {
		t.Fatalf("Negotiate returned %v, want ErrVersionUnsupported", err)
	}
	if kind := d.answer(t).Kind; kind != KindVersionRefused {
		t.Errorf("the server answered with kind %d, want the refusal %d", kind, KindVersionRefused)
	}
}

func TestAClientNewerThanTheServerIsRefused(t *testing.T) {
	d := peerSending(Hello(MaxSupportedVersion + 1))

	if _, err := Negotiate(NewConn(d)); !errors.Is(err, ErrVersionUnsupported) {
		t.Fatalf("Negotiate returned %v, want ErrVersionUnsupported", err)
	}
	if kind := d.answer(t).Kind; kind != KindVersionRefused {
		t.Errorf("the server answered with kind %d, want the refusal %d", kind, KindVersionRefused)
	}
}

// The refusal has to be something a client author can act on, so this asserts
// on what the message carries rather than on the error the server returned to
// itself. Both halves: the numbers a client compares against, and the sentence
// a person reads out of a log.
func TestTheRefusalNamesTheVersionsTheServerSupports(t *testing.T) {
	d := peerSending(Hello(MaxSupportedVersion + 1))

	if _, err := Negotiate(NewConn(d)); !errors.Is(err, ErrVersionUnsupported) {
		t.Fatalf("Negotiate returned %v, want ErrVersionUnsupported", err)
	}

	lowest, highest, reason, err := RefusedVersions(d.answer(t))
	if err != nil {
		t.Fatalf("RefusedVersions: %v", err)
	}
	if lowest != MinSupportedVersion || highest != MaxSupportedVersion {
		t.Errorf("the refusal names %d to %d, want %d to %d", lowest, highest, MinSupportedVersion, MaxSupportedVersion)
	}

	for _, want := range []Version{MinSupportedVersion, MaxSupportedVersion} {
		if !strings.Contains(reason, strconv.Itoa(int(want))) {
			t.Errorf("the reason %q does not name version %d", reason, want)
		}
	}
}

// An unknown field in a known message is ignored. The hello below carries the
// proposed version and a field identifier this build has never heard of, and
// the negotiation has to succeed on the half it understands.
func TestAnUnknownFieldInAKnownMessageIsIgnored(t *testing.T) {
	d := peerSending(Frame{
		Kind: KindHello,
		Payload: payloadOf(
			field{id: 4096, value: []byte("a field from a later version")},
			field{id: fieldProposedVersion, value: versionValue(MaxSupportedVersion)},
			field{id: 4097, value: nil},
		),
	})

	agreed, err := Negotiate(NewConn(d))
	if err != nil {
		t.Fatalf("Negotiate: %v", err)
	}
	if agreed != MaxSupportedVersion {
		t.Errorf("agreed on %d, want %d", agreed, MaxSupportedVersion)
	}
}

// An unknown message is an error. Kind 200 is in no build's message set, and
// the rule is asserted in both places it holds: at the negotiation, where it is
// the first thing a peer sends, and on an authenticated connection, where a
// decoder that switched on the kind and fell through would otherwise carry on.

func TestAnUnknownMessageIsRefusedAtTheNegotiation(t *testing.T) {
	d := peerSending(Frame{Kind: 200, Payload: []byte("whatever this is")})

	if _, err := Negotiate(NewConn(d)); !errors.Is(err, ErrUnknownMessage) {
		t.Fatalf("Negotiate returned %v, want ErrUnknownMessage", err)
	}
	if d.written.Len() != 0 {
		t.Errorf("the server answered a message it does not know with %d bytes", d.written.Len())
	}
}

func TestAnUnknownMessageIsRefusedOnAnAuthenticatedConnection(t *testing.T) {
	c := NewConn(peerSending(
		Frame{Kind: 200, Payload: []byte("whatever this is")},
		Frame{Kind: KindSpaceState, Payload: []byte("state")},
	))
	c.MarkAuthenticated()

	if _, err := c.ReadFrame(); !errors.Is(err, ErrUnknownMessage) {
		t.Fatalf("the first ReadFrame returned %v, want ErrUnknownMessage", err)
	}

	// The frame after it is a kind this build does know, so a refusal that was
	// not terminal would return it here and this test would pass with the
	// connection still reading.
	second, err := c.ReadFrame()
	if !errors.Is(err, ErrUnknownMessage) {
		t.Fatalf("the second ReadFrame returned %v, want the same refusal", err)
	}
	if second.Kind == KindSpaceState {
		t.Fatal("the connection went on reading after a message it does not know")
	}
}

func TestEveryKindThisBuildDefinesIsKnown(t *testing.T) {
	for _, k := range []Kind{KindAuthenticate, KindSpaceState, KindHello, KindVersionAgreed, KindVersionRefused} {
		if !k.Known() {
			t.Errorf("kind %d is declared and Known reports it is not", k)
		}
	}
	for _, k := range []Kind{KindReserved, 6, 200, 255} {
		if k.Known() {
			t.Errorf("kind %d is not declared and Known reports it is", k)
		}
	}
}

// A hello that is not a hello. Both are frames the negotiation has to tell
// apart from each other, because "I do not know this message" and "I know this
// message and it is in the wrong place" are different things to a client
// author.
func TestAKnownMessageThatIsNotTheHelloIsRefused(t *testing.T) {
	d := peerSending(Frame{Kind: KindAuthenticate, Payload: []byte("credential")})

	if _, err := Negotiate(NewConn(d)); !errors.Is(err, ErrHelloExpected) {
		t.Fatalf("Negotiate returned %v, want ErrHelloExpected", err)
	}
}

func TestNegotiateReportsAReadFailure(t *testing.T) {
	if _, err := Negotiate(NewConn(peerSending())); !errors.Is(err, io.EOF) {
		t.Fatalf("Negotiate returned %v, want io.EOF", err)
	}
}

// The write halves. A peer that goes away between its hello and the answer is
// the case, and the error has to reach the caller rather than being swallowed
// by a function whose last statement is a successful return.

func TestNegotiateReportsAFailureWritingTheAgreement(t *testing.T) {
	d := peerSending(Hello(MaxSupportedVersion))
	d.writeErr = errWriteRefused

	if _, err := Negotiate(NewConn(d)); !errors.Is(err, errWriteRefused) {
		t.Fatalf("Negotiate returned %v, want the write failure", err)
	}
}

func TestNegotiateReportsAFailureWritingTheRefusal(t *testing.T) {
	d := peerSending(Hello(MaxSupportedVersion + 1))
	d.writeErr = errWriteRefused

	if _, err := Negotiate(NewConn(d)); !errors.Is(err, errWriteRefused) {
		t.Fatalf("Negotiate returned %v, want the write failure", err)
	}
}

// The payload parser. A field structure a peer can send has to be refused
// rather than clamped, and the two ways it can be broken are a trailing stub
// too short to be a header and a length that runs past the end.

func TestAPayloadEndingInsideAFieldHeaderIsRefused(t *testing.T) {
	d := peerSending(Frame{Kind: KindHello, Payload: []byte{0, 1, 0}})

	if _, err := Negotiate(NewConn(d)); !errors.Is(err, ErrMalformedMessage) {
		t.Fatalf("Negotiate returned %v, want ErrMalformedMessage", err)
	}
}

func TestAFieldDeclaringMoreThanRemainsIsRefused(t *testing.T) {
	d := peerSending(Frame{Kind: KindHello, Payload: []byte{0, 1, 0, 9, 1, 2, 3}})

	if _, err := Negotiate(NewConn(d)); !errors.Is(err, ErrMalformedMessage) {
		t.Fatalf("Negotiate returned %v, want ErrMalformedMessage", err)
	}
}

func TestAHelloWithNoVersionFieldIsRefused(t *testing.T) {
	d := peerSending(Frame{Kind: KindHello, Payload: payloadOf(field{id: 4096, value: []byte("only this")})})

	if _, err := Negotiate(NewConn(d)); !errors.Is(err, ErrMalformedMessage) {
		t.Fatalf("Negotiate returned %v, want ErrMalformedMessage", err)
	}
}

// A version field of the wrong width is its own refusal rather than being
// treated as absent. A peer that sent four bytes where two were specified has
// made a different mistake from one that sent nothing, and collapsing the two
// would send a client author looking for a field they had already written.
func TestAVersionFieldOfTheWrongWidthIsRefused(t *testing.T) {
	d := peerSending(Frame{
		Kind:    KindHello,
		Payload: payloadOf(field{id: fieldProposedVersion, value: []byte{0, 0, 0, 1}}),
	})

	if _, err := Negotiate(NewConn(d)); !errors.Is(err, ErrMalformedMessage) {
		t.Fatalf("Negotiate returned %v, want ErrMalformedMessage", err)
	}
}

// The reading half of the two answers, driven from a client's side. A client
// handed the wrong frame, or a broken one, has to be told rather than handed a
// zero version that reads as a successful negotiation on version zero.

func TestAgreedVersionRefusesAFrameThatIsNotTheAcceptance(t *testing.T) {
	if _, err := AgreedVersion(Frame{Kind: KindSpaceState}); !errors.Is(err, ErrMalformedMessage) {
		t.Fatalf("AgreedVersion returned %v, want ErrMalformedMessage", err)
	}
}

func TestAgreedVersionRefusesABrokenPayload(t *testing.T) {
	if _, err := AgreedVersion(Frame{Kind: KindVersionAgreed, Payload: []byte{0}}); !errors.Is(err, ErrMalformedMessage) {
		t.Fatalf("AgreedVersion returned %v, want ErrMalformedMessage", err)
	}
}

func TestRefusedVersionsRefusesAFrameThatIsNotTheRefusal(t *testing.T) {
	if _, _, _, err := RefusedVersions(Frame{Kind: KindVersionAgreed}); !errors.Is(err, ErrMalformedMessage) {
		t.Fatalf("RefusedVersions returned %v, want ErrMalformedMessage", err)
	}
}

func TestRefusedVersionsRefusesABrokenPayload(t *testing.T) {
	if _, _, _, err := RefusedVersions(Frame{Kind: KindVersionRefused, Payload: []byte{0}}); !errors.Is(err, ErrMalformedMessage) {
		t.Fatalf("RefusedVersions returned %v, want ErrMalformedMessage", err)
	}
}

// Each of the three fields the refusal carries, missing on its own. A reader
// that returned what it found and left the rest zero would hand a client a
// range of nought to nought, which is a range it can never satisfy.
func TestRefusedVersionsRefusesAMessageMissingAnyOfItsFields(t *testing.T) {
	complete := []field{
		{id: fieldLowestSupported, value: versionValue(MinSupportedVersion)},
		{id: fieldHighestSupported, value: versionValue(MaxSupportedVersion)},
		{id: fieldReason, value: []byte("a reason")},
	}

	for missing := range complete {
		var kept []field
		kept = append(kept, complete[:missing]...)
		kept = append(kept, complete[missing+1:]...)

		f := Frame{Kind: KindVersionRefused, Payload: payloadOf(kept...)}
		if _, _, _, err := RefusedVersions(f); !errors.Is(err, ErrMalformedMessage) {
			t.Errorf("without field %d, RefusedVersions returned %v, want ErrMalformedMessage", complete[missing].id, err)
		}
	}
}

// The whole exchange over one stream, so the two halves are shown to agree with
// each other rather than each with a fixture written beside it.
func TestTheClientAndServerHalvesAgree(t *testing.T) {
	d := peerSending(Hello(MinSupportedVersion))

	agreed, err := Negotiate(NewConn(d))
	if err != nil {
		t.Fatalf("Negotiate: %v", err)
	}

	read, err := AgreedVersion(d.answer(t))
	if err != nil {
		t.Fatalf("AgreedVersion: %v", err)
	}
	if read != agreed {
		t.Errorf("the server returned %d to itself and sent %d to the client", agreed, read)
	}
}
