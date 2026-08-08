// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026  iderex

package signalling

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// Kind says what a frame's payload is. It is one byte, so the space is 255
// usable values and the protocol will not run out.
//
// Two are declared here and the rest are not. This package owes the framing and
// the rule that an unauthenticated peer may send nothing but an authentication
// frame, so it declares the kind that rule names and one other, which exists
// because a rule about "every kind except that one" cannot be tested against an
// empty set. The message set proper belongs to the issues that define messages,
// and a kind invented here to look complete would be one of them arguing with
// this file later.
type Kind uint8

const (
	// KindReserved is not a kind. It is the value a zero byte decodes to, and
	// refusing it is what stops a run of zero bytes from being read as a valid
	// empty frame of a real kind.
	KindReserved Kind = 0

	// KindAuthenticate carries a peer's credential. It is the only kind a
	// connection accepts before it is authenticated, which Conn enforces.
	KindAuthenticate Kind = 1

	// KindSpaceState carries a full description of a space back to a client.
	// It is declared here as the other side of the authentication rule rather
	// than specified: what is in its payload is not this package's to say.
	KindSpaceState Kind = 2
)

// MaxPayloadSize is the largest payload a frame may declare, in bytes.
//
// The number is chosen rather than inherited from a default. The largest
// message this protocol is expected to carry is a full state transfer of one
// space, which is its channels and the occupancies in them, and 64 KiB holds a
// few thousand of those at the identifier sizes internal/orchestration
// produces. Anything larger is either a message that should have been paged or
// a peer declaring a number to see what happens.
//
// It is deliberately small. The cost of a bound that is too low is a message
// that has to be split, which is a change to this constant argued for with a
// measurement. The cost of one that is too high is a peer that has authenticated
// nothing making the server allocate, which is not a cost anybody notices until
// it is being paid at scale.
const MaxPayloadSize = 64 << 10

// headerSize is four bytes of payload length and one byte of kind.
const headerSize = 5

// ErrFrameTooLarge is returned when a header declares more than MaxPayloadSize,
// and when Encode is handed a payload that would.
var ErrFrameTooLarge = errors.New("signalling: frame larger than the bound")

// ErrKindReserved is returned for a frame whose kind byte is KindReserved.
var ErrKindReserved = errors.New("signalling: frame kind 0 is reserved")

// A Frame is one message on the wire: what it is, and its bytes.
//
// Payload is not copied by Encode and is freshly allocated by Decode, so a
// frame a caller receives is theirs to keep and a frame a caller sends must not
// be mutated while Encode runs.
type Frame struct {
	Kind    Kind
	Payload []byte
}

// Decode reads one frame from r.
//
// The order of the checks is the point of this function. The declared length is
// compared against MaxPayloadSize before anything is allocated for it, so a
// header claiming four gigabytes costs the five bytes of the header and no
// memory at all. A decoder that allocates first and refuses afterwards passes
// every test that asserts an oversized frame is rejected, and is the bug this
// ordering exists against.
func Decode(r io.Reader) (Frame, error) {
	var header [headerSize]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return Frame{}, fmt.Errorf("signalling: reading the frame header: %w", err)
	}

	declared := binary.BigEndian.Uint32(header[:4])
	if declared > MaxPayloadSize {
		return Frame{}, fmt.Errorf("%w: the header declares %d bytes and the bound is %d", ErrFrameTooLarge, declared, MaxPayloadSize)
	}

	kind := Kind(header[4])
	if kind == KindReserved {
		return Frame{}, fmt.Errorf("%w: the header declares a payload of %d bytes", ErrKindReserved, declared)
	}

	payload := make([]byte, declared)
	if _, err := io.ReadFull(r, payload); err != nil {
		return Frame{}, fmt.Errorf("signalling: reading a %d byte payload: %w", declared, err)
	}
	return Frame{Kind: kind, Payload: payload}, nil
}

// Encode writes f to w as one Write, so a writer that fails leaves either the
// whole frame or none of it rather than a header with no payload behind it.
//
// It refuses the same two things Decode refuses. A frame this package will not
// read is not one it will write, or the corpus of what a peer has legitimately
// received stops matching what the decoder accepts.
func Encode(w io.Writer, f Frame) error {
	if len(f.Payload) > MaxPayloadSize {
		return fmt.Errorf("%w: the payload is %d bytes and the bound is %d", ErrFrameTooLarge, len(f.Payload), MaxPayloadSize)
	}
	if f.Kind == KindReserved {
		return fmt.Errorf("%w: the payload is %d bytes", ErrKindReserved, len(f.Payload))
	}

	buf := make([]byte, headerSize+len(f.Payload))
	binary.BigEndian.PutUint32(buf[:4], uint32(len(f.Payload)))
	buf[4] = byte(f.Kind)
	copy(buf[headerSize:], f.Payload)

	if _, err := w.Write(buf); err != nil {
		return fmt.Errorf("signalling: writing a %d byte frame: %w", len(buf), err)
	}
	return nil
}
