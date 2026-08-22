// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026  Nils Lehnen

package signalling

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// A Version is the version of this protocol a connection speaks. It is agreed
// once, on the first frame, and it does not change afterwards.
//
// Two bytes rather than one, and a plain number rather than a triple. A version
// here names the wire contract and nothing else, so there is no second
// component for a change that does not reach the wire, and a second component
// would invite one to be incremented for a change nobody has to negotiate.
type Version uint16

// The versions this build speaks, as a closed range rather than a set.
//
// Both are 1 today, which is the only version that exists. The comparison below
// is still a range comparison, because the day a second version lands the
// change is to these two constants and to nothing else. A negotiation written
// as an equality against one number is one that gets rewritten rather than
// widened, and rewriting it is where a client that should have been refused
// stops being.
//
// How far behind a client is allowed to be, once there is more than one version
// to be behind by, is a policy rather than a property of this code. Issue #91 is
// where that is stated. What this file owes is the refusal, and the refusal is
// against the range above rather than against a number it decides for itself.
const (
	MinSupportedVersion Version = 1
	MaxSupportedVersion Version = 1
)

const (
	// KindHello carries a client's proposed version. It is the first frame on
	// a connection, before the credential, because the credential frame's own
	// shape is part of what a version decides.
	KindHello Kind = 3

	// KindVersionAgreed carries the version the server accepted, so a client
	// reads back what it will be held to rather than assuming its proposal was
	// taken whole.
	KindVersionAgreed Kind = 4

	// KindVersionRefused carries the range this server supports and a sentence
	// naming it. It is the last frame on a connection that sends it.
	KindVersionRefused Kind = 5
)

// The field identifiers inside the messages above. They are scoped to their own
// message, so the same number means different things in a hello and in a
// refusal, and nothing here is a registry of every field in the protocol.
const (
	fieldProposedVersion  uint16 = 1
	fieldAgreedVersion    uint16 = 1
	fieldLowestSupported  uint16 = 1
	fieldHighestSupported uint16 = 2
	fieldReason           uint16 = 3
)

// fieldHeaderSize is two bytes of identifier and two bytes of length.
const fieldHeaderSize = 4

// ErrUnknownMessage is returned for a frame whose kind this build does not
// define. It is the second half of what makes an additive change possible: an
// unknown field inside a message this build knows is ignored, and a message it
// does not know is refused rather than guessed at.
var ErrUnknownMessage = errors.New("signalling: the frame kind is not in this build's message set")

// ErrHelloExpected is returned when the first frame on a connection is a kind
// this build knows but is not the hello.
var ErrHelloExpected = errors.New("signalling: the first frame is not a hello")

// ErrMalformedMessage is returned for a payload whose fields do not parse, and
// for one missing a field the message requires or carrying it at the wrong
// width.
var ErrMalformedMessage = errors.New("signalling: the message payload is malformed")

// ErrVersionUnsupported is returned when the proposed version falls outside the
// range above. The peer has been sent the refusal by the time it is returned.
var ErrVersionUnsupported = errors.New("signalling: the proposed protocol version is not supported")

// Known reports whether this build defines k.
//
// The list is written out rather than derived from a range, so a kind that is
// reserved, or retired, or added between two existing numbers is a line here
// rather than an arithmetic accident. A kind absent from it is refused by
// Conn.ReadFrame.
func (k Kind) Known() bool {
	switch k {
	case KindAuthenticate, KindSpaceState, KindHello, KindVersionAgreed, KindVersionRefused:
		return true
	default:
		return false
	}
}

// A field is one entry in a message payload: what it is, and its bytes.
type field struct {
	id    uint16
	value []byte
}

// encodeFields lays fields out as identifier, length, value, in order.
//
// Nothing here refuses a value too long for the length to express, because
// every caller in this file builds its own fields from a version number or from
// a sentence it composed. A general encoder open to a caller's bytes would owe
// that refusal, and the moment one exists it goes here.
func encodeFields(fields []field) []byte {
	size := 0
	for _, f := range fields {
		size += fieldHeaderSize + len(f.value)
	}

	out := make([]byte, 0, size)
	var header [fieldHeaderSize]byte
	for _, f := range fields {
		binary.BigEndian.PutUint16(header[:2], f.id)
		binary.BigEndian.PutUint16(header[2:], uint16(len(f.value)))
		out = append(out, header[:]...)
		out = append(out, f.value...)
	}
	return out
}

// decodeFields walks payload and returns every field in it, unknown
// identifiers included. Deciding which of them mean anything is the caller's,
// and that split is what makes an unknown field ignorable rather than fatal.
//
// A field whose declared length runs past the end of the payload is refused
// rather than clamped. Clamping would hand a caller a value shorter than the
// sender wrote and no way to tell that from a short value the sender meant.
func decodeFields(payload []byte) ([]field, error) {
	var fields []field
	for offset := 0; offset < len(payload); {
		if len(payload)-offset < fieldHeaderSize {
			return nil, fmt.Errorf("%w: %d trailing bytes are not a field header", ErrMalformedMessage, len(payload)-offset)
		}

		id := binary.BigEndian.Uint16(payload[offset : offset+2])
		length := int(binary.BigEndian.Uint16(payload[offset+2 : offset+fieldHeaderSize]))
		offset += fieldHeaderSize

		if len(payload)-offset < length {
			return nil, fmt.Errorf("%w: field %d declares %d bytes and %d remain", ErrMalformedMessage, id, length, len(payload)-offset)
		}

		fields = append(fields, field{id: id, value: payload[offset : offset+length]})
		offset += length
	}
	return fields, nil
}

// versionField finds the field with the given identifier and reads it as a
// version. A field this build does not know is skipped by never being asked
// for, which is where "an unknown field is ignored" actually happens.
func versionField(fields []field, id uint16) (Version, error) {
	for _, f := range fields {
		if f.id != id {
			continue
		}
		if len(f.value) != 2 {
			return 0, fmt.Errorf("%w: field %d is %d bytes and a version is 2", ErrMalformedMessage, id, len(f.value))
		}
		return Version(binary.BigEndian.Uint16(f.value)), nil
	}
	return 0, fmt.Errorf("%w: field %d is absent", ErrMalformedMessage, id)
}

// Hello builds the first frame a client sends, proposing a version.
func Hello(proposed Version) Frame {
	var value [2]byte
	binary.BigEndian.PutUint16(value[:], uint16(proposed))
	return Frame{
		Kind:    KindHello,
		Payload: encodeFields([]field{{id: fieldProposedVersion, value: value[:]}}),
	}
}

// AgreedVersion reads the version out of the server's acceptance.
func AgreedVersion(f Frame) (Version, error) {
	if f.Kind != KindVersionAgreed {
		return 0, fmt.Errorf("%w: kind %d is not the acceptance", ErrMalformedMessage, f.Kind)
	}
	fields, err := decodeFields(f.Payload)
	if err != nil {
		return 0, err
	}
	return versionField(fields, fieldAgreedVersion)
}

// RefusedVersions reads the server's refusal: the range it supports, and the
// sentence it sent naming that range.
//
// The range is carried as two numbers as well as inside the sentence. The
// sentence is what a person reads out of a client's log and the numbers are
// what a client compares against, and a client that had only the sentence would
// have to parse prose to decide whether to offer a different version.
func RefusedVersions(f Frame) (lowest, highest Version, reason string, err error) {
	if f.Kind != KindVersionRefused {
		return 0, 0, "", fmt.Errorf("%w: kind %d is not the refusal", ErrMalformedMessage, f.Kind)
	}

	fields, err := decodeFields(f.Payload)
	if err != nil {
		return 0, 0, "", err
	}
	if lowest, err = versionField(fields, fieldLowestSupported); err != nil {
		return 0, 0, "", err
	}
	if highest, err = versionField(fields, fieldHighestSupported); err != nil {
		return 0, 0, "", err
	}
	for _, candidate := range fields {
		if candidate.id == fieldReason {
			return lowest, highest, string(candidate.value), nil
		}
	}
	return 0, 0, "", fmt.Errorf("%w: field %d is absent", ErrMalformedMessage, fieldReason)
}

// refusal builds the frame sent to a peer whose proposal is outside the range.
func refusal() Frame {
	var lowest, highest [2]byte
	binary.BigEndian.PutUint16(lowest[:], uint16(MinSupportedVersion))
	binary.BigEndian.PutUint16(highest[:], uint16(MaxSupportedVersion))

	reason := fmt.Sprintf("this server speaks protocol versions %d to %d", MinSupportedVersion, MaxSupportedVersion)

	return Frame{
		Kind: KindVersionRefused,
		Payload: encodeFields([]field{
			{id: fieldLowestSupported, value: lowest[:]},
			{id: fieldHighestSupported, value: highest[:]},
			{id: fieldReason, value: []byte(reason)},
		}),
	}
}

// agreement builds the frame sent to a peer whose proposal is inside the range.
func agreement(agreed Version) Frame {
	var value [2]byte
	binary.BigEndian.PutUint16(value[:], uint16(agreed))
	return Frame{
		Kind:    KindVersionAgreed,
		Payload: encodeFields([]field{{id: fieldAgreedVersion, value: value[:]}}),
	}
}

// Negotiate performs the server's half of the negotiation on c and returns the
// version the connection speaks.
//
// It reads the stream directly rather than through Conn.ReadFrame, and the
// reason is the ordering rather than convenience. ReadFrame's gate is about a
// peer that has proved nothing, and it is written to accept the credential and
// nothing else. The hello arrives before the credential, so putting it through
// that gate would mean widening the one rule on this connection that exists to
// be narrow. Negotiation happens first, on the stream, and the gate is left
// alone.
//
// A refusal is written to the peer before the error is returned, so a client is
// told what happened rather than seeing a connection close on it. The error is
// returned either way: a caller that stopped at the write would carry on
// serving a peer whose version it had just refused.
func Negotiate(c *Conn) (Version, error) {
	f, err := Decode(c.stream)
	if err != nil {
		return 0, err
	}

	if !f.Kind.Known() {
		return 0, fmt.Errorf("%w: kind %d arrived where the hello was expected", ErrUnknownMessage, f.Kind)
	}
	if f.Kind != KindHello {
		return 0, fmt.Errorf("%w: it sent kind %d", ErrHelloExpected, f.Kind)
	}

	fields, err := decodeFields(f.Payload)
	if err != nil {
		return 0, err
	}
	proposed, err := versionField(fields, fieldProposedVersion)
	if err != nil {
		return 0, err
	}

	if proposed < MinSupportedVersion || proposed > MaxSupportedVersion {
		if err := Encode(c.stream, refusal()); err != nil {
			return 0, err
		}
		return 0, fmt.Errorf("%w: it proposed %d and this server speaks %d to %d", ErrVersionUnsupported, proposed, MinSupportedVersion, MaxSupportedVersion)
	}

	if err := Encode(c.stream, agreement(proposed)); err != nil {
		return 0, err
	}
	return proposed, nil
}
