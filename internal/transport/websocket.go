// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026  Nils Lehnen

package transport

import (
	"net/http"

	"github.com/coder/websocket"

	"github.com/iderex/stammtisch/internal/signalling"
)

// A Serve is handed one peer's signalling connection and keeps it for as long
// as that peer is around. When it returns, the WebSocket connection is closed.
//
// It takes the connection rather than a stream so that the rule about an
// unauthenticated peer is already wrapped around the bytes by the time anybody
// downstream sees them. A caller that was handed the raw stream could read a
// frame without that gate, which is the arrangement this signature refuses.
type Serve func(*signalling.Conn)

// Handler returns an http.Handler that accepts a WebSocket request and hands
// the resulting stream to serve as a signalling connection.
//
// A request that is not a WebSocket handshake, or whose Origin names another
// host, never reaches serve: websocket.Accept writes the refusal itself and
// this function returns without having made a connection. That is the reason
// the error is not wrapped or reported anywhere here. There is one response and
// the library has already written it, and a second one written from this
// function would be a header written after the body.
//
// A request that did not arrive over a connection transit says is confidential
// is refused before any of that, so no handshake is completed and no connection
// is made for it. transit has no default because a caller that forgot it would
// get the arrangement this refusal exists against, and 403 rather than a
// redirect because the client is opening a socket and has nowhere to be sent.
func Handler(serve Serve, transit Transit) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !transit.confidential(r) {
			http.Error(w, "this connection is not confidential", http.StatusForbidden)
			return
		}

		ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{})
		if err != nil {
			return
		}
		defer func() { _ = ws.CloseNow() }()

		// MessageBinary, and not text. The framing carries a length and a kind
		// in front of bytes this package never interprets, and a text message
		// is one a peer's runtime is entitled to validate as UTF-8 and refuse.
		stream := websocket.NetConn(r.Context(), ws, websocket.MessageBinary)
		serve(signalling.NewConn(stream))
	})
}
