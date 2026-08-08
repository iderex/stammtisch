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
package signalling
