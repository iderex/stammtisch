// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026  iderex

// Package signalling holds the framing every message between a client, a bot
// and the server travels inside, and the connection state that decides which of
// them a peer is allowed to send.
//
// # The transport this framing rides on
//
// A page has to be able to open the connection. That follows from
// docs/decisions/client-platform.md, which makes the first client a browser
// application, and it removes every transport a browser cannot open: a raw TCP
// stream, a QUIC stream opened directly, and anything that needs a socket the
// page does not get. What is left and satisfies the rest of the requirement is
// WebSocket, which carries a long-lived bidirectional connection, delivers in
// order within it, and lets the server send without being asked, so a
// participant list updates rather than refreshes.
//
// The framing below does not depend on that choice and does not import it. It
// reads and writes a byte stream, which is what a WebSocket connection, a pipe
// in a test and a future transport all are. Nothing in this package opens a
// socket, listens on a port or speaks HTTP, and the binding onto a WebSocket is
// not here yet.
//
// # Why a length and a kind rather than the transport's own message boundary
//
// WebSocket already carries message boundaries, so a frame header looks like a
// second one. It is here because the boundary a transport gives is the
// transport's, and the bound this package enforces has to be ours: a peer that
// has authenticated nothing is the first thing to send bytes, and the size it
// is allowed to declare is a security property rather than a tuning knob. A
// header we read is one we can refuse before allocating for it. A boundary the
// transport hands us has already been allocated by the time we see it.
//
// It also keeps the framing usable over any ordered stream, which is what lets
// the whole of this package be tested with no network at all.
//
// # The version negotiation, as a client author has to implement it
//
// A connection speaks one version of this protocol and agrees it on the first
// frame. Clients update when their user lets them and bots update when nobody
// makes them, so a protocol with no version is one that can never change or one
// that breaks people without saying so, and which of the two happens is decided
// by accident rather than by anyone.
//
// The exchange is three frames and at most two of them travel:
//
//  1. The client sends KindHello carrying the version it proposes. This is the
//     first frame on the connection, before the credential, because what a
//     credential frame looks like is one of the things a version decides.
//  2. If the proposal is inside the range this server speaks, the server sends
//     KindVersionAgreed carrying the agreed version, and the connection carries
//     on to the credential. A client reads that frame rather than assuming its
//     proposal was taken whole.
//  3. If it is outside the range, the server sends KindVersionRefused and stops.
//     That frame carries the lowest and the highest version this server speaks,
//     as numbers a client compares against, and a sentence naming the same two
//     so a person reading a log is not left parsing a number out of an error
//     code. A client that can speak one of them may open a new connection and
//     propose it.
//
// A payload here is a sequence of fields, each two bytes of identifier, two
// bytes of length, and that many bytes of value. Two rules run over that shape
// and they only work as a pair:
//
//   - An unknown field inside a message this build knows is ignored. That is
//     what lets a later version add a field to the hello without every older
//     server refusing it.
//   - An unknown message is an error, and a terminal one. That is what stops a
//     later version's message from being silently dropped by an older server
//     that then carries on as though the peer had said nothing.
//
// Kind.Known is the list a message is unknown against, and Conn.ReadFrame
// refuses one on every read rather than only during the negotiation.
//
// What is not decided here is how far behind a client is allowed to be once
// there is more than one version to be behind by. That is a policy about
// support rather than a property of this code, and issue #91 is where it is
// stated. This package holds the refusal and the range it is made against.
package signalling
