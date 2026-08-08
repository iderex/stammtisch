// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026  iderex

package signalling

import (
	"errors"
	"fmt"
	"io"
)

// ErrUnauthenticated is returned when a peer sends any kind but
// KindAuthenticate before it has been authenticated.
var ErrUnauthenticated = errors.New("signalling: the connection is not authenticated")

// A Conn is one peer's signalling connection: the framing above, plus the one
// rule that has to hold before anything else is read.
//
// It takes an io.ReadWriter rather than a network connection. What that stream
// is belongs to the transport, and holding it at this width is what lets the
// whole of this package be tested with no socket, no device and no elevation.
//
// A Conn is used by one reader and one writer. Nothing here is safe for
// concurrent readers, and nothing here pretends to be: two goroutines reading
// one signalling connection is a design mistake rather than a case to lock for.
type Conn struct {
	stream        io.ReadWriter
	authenticated bool
	refused       error
}

// NewConn wraps stream. The connection starts unauthenticated, which is the
// only state a peer that has proved nothing may be in.
func NewConn(stream io.ReadWriter) *Conn {
	return &Conn{stream: stream}
}

// Authenticated reports whether the gate below is open.
func (c *Conn) Authenticated() bool { return c.authenticated }

// MarkAuthenticated opens the gate.
//
// What makes that decision is issue #28 and is not here. This method exists so
// the rule this package owes, that nothing but an authentication frame crosses
// an unauthenticated connection, is enforced in one place and can be proved
// without the credential machinery existing yet.
func (c *Conn) MarkAuthenticated() { c.authenticated = true }

// ReadFrame reads the next frame from the peer.
//
// Before MarkAuthenticated, every kind but KindAuthenticate is refused, and the
// refusal is terminal: the connection stays refused and every later read
// returns the same error. A gate that refuses one frame and reads the next lets
// a peer that has proved nothing keep talking, which is the property this rule
// is about rather than the single frame it rejects.
func (c *Conn) ReadFrame() (Frame, error) {
	if c.refused != nil {
		return Frame{}, c.refused
	}

	f, err := Decode(c.stream)
	if err != nil {
		return Frame{}, err
	}

	if !c.authenticated && f.Kind != KindAuthenticate {
		c.refused = fmt.Errorf("%w: it sent kind %d and only kind %d is accepted first", ErrUnauthenticated, f.Kind, KindAuthenticate)
		return Frame{}, c.refused
	}

	// A kind this build does not define is refused here rather than passed on
	// for somebody downstream to switch on and fall through. The refusal is
	// terminal for the same reason the one above is: a peer speaking a message
	// set this server does not have is in a state neither side can reason
	// about, and reading the next frame is guessing which of the two of them
	// was wrong.
	if !f.Kind.Known() {
		c.refused = fmt.Errorf("%w: it sent kind %d", ErrUnknownMessage, f.Kind)
		return Frame{}, c.refused
	}
	return f, nil
}

// WriteFrame writes f to the peer.
//
// It carries no gate. The server is the side that decides what a peer may be
// told, and a connection refusing its own writes would be this package holding
// an opinion about a message it did not define.
func (c *Conn) WriteFrame(f Frame) error {
	return Encode(c.stream, f)
}
