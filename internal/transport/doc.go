// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026  iderex

// Package transport binds the signalling framing onto a WebSocket connection.
//
// It is the reader and the writer that internal/signalling deliberately does
// not have. That package reads and writes an ordered byte stream and says so in
// its own comment; this one produces such a stream from an HTTP request and
// hands it over, and it holds nothing else. No message is defined here, no
// permission is decided here, and nothing here looks at a payload.
//
// # The means, and why it is a dependency rather than a file
//
// WebSocket is the transport, and the reason is in internal/signalling/doc.go
// rather than repeated here: a page has to be able to open the connection, and
// that removes everything a browser cannot open.
//
// The handshake and RFC 6455's own framing are the third-party part. Writing
// them here would put a parser reachable by an unauthenticated peer into this
// tree, which is the one place a homemade parser is least worth having, and it
// would be a parser with no corpus behind it. github.com/coder/websocket is
// Go, carries no dependencies of its own, and answers the four questions the
// contribution guide asks of a means: the property this package owes stays
// refusable because the framing bound is ours and sits above it, the proof is
// executed by the suite below with no socket bound, the claims here carry the
// commands that produced them, and the runtime the tree already has is the only
// one it needs.
//
// # What the library's read limit does here, and why that is not a hole
//
// websocket.NetConn documents that it sets the read limit to -1, so the
// per-message bound the library would otherwise apply is off on this path. That
// is the correct behaviour and not something worked around: NetConn hands back
// a stream, so a message arrives a read at a time and nothing allocates for the
// size a peer declared. The bound that matters is signalling.MaxPayloadSize,
// which is compared against the header before the payload is allocated for, and
// TestTheFrameBoundIsRefusedThroughTheTransport is where that is shown to hold
// through this package rather than only over a pipe in a test.
//
// This is the same argument internal/signalling/doc.go already makes for
// carrying a length of its own over a transport that has message boundaries.
// The boundary a transport gives is the transport's; the bound is ours.
//
// # Origin
//
// websocket.Accept refuses a request whose Origin names a host other than the
// one the request was made to, and this package takes that default rather than
// widening it. A browser client served from the operator's own host is
// therefore the only page that can open a connection, and a page on somebody
// else's host carrying a visitor's cookies is refused before any frame is read.
// TestARequestFromAnotherOriginIsRefused is the proof, and it goes red if
// InsecureSkipVerify or an OriginPatterns entry is added.
//
// # Confidentiality
//
// Handler refuses a request that did not arrive over a connection its Transit
// argument says is confidential, before the handshake and before any connection
// exists. The default value of that argument is the refusing one, so the
// arrangement in which a conversation crosses a network in the clear has to be
// declared by name at the call site rather than reached by leaving something
// out. docs/decisions/transport-confidentiality.md is where the requirement is
// argued and where what the declaration cannot check is written down.
//
// TestARequestThatDidNotArriveOverTLSIsRefused is the proof, and it asserts on
// the refused arrangement rather than on the working one. Its neighbour
// TestARequestOverTLSReachesServe is what stops a guard that refused every
// request from passing it.
//
// # What is not here
//
// Nothing in this package listens on a port. Handler returns an http.Handler
// and a caller decides where it is mounted and what address it is served on,
// which is the entry point's decision and not this package's. Nothing in this
// tree calls it yet.
package transport
