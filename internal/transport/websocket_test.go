// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026  iderex

package transport_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/iderex/stammtisch/internal/signalling"
	"github.com/iderex/stammtisch/internal/transport"
)

// The whole suite runs over net.Pipe and never over a socket. Nothing here
// binds an address, so no test in this package can be blocked by a firewall,
// ask for a privilege, or fail on a machine where a port is already taken. That
// is the same property internal/signalling has for the framing, kept one layer
// further out.

// dialTimeout bounds every exchange below. net.Pipe has no buffer and no
// deadline of its own, so a test that got the handshake wrong would otherwise
// hang until the package timeout rather than say what it was waiting for.
const dialTimeout = 10 * time.Second

// pipeNet is a net.Listener whose connections are made in memory. Dial hands
// one end to the caller and the other to Accept.
type pipeNet struct {
	conns  chan net.Conn
	closed chan struct{}
}

func (p *pipeNet) Accept() (net.Conn, error) {
	select {
	case c := <-p.conns:
		return c, nil
	case <-p.closed:
		return nil, net.ErrClosed
	}
}

func (p *pipeNet) Close() error {
	select {
	case <-p.closed:
	default:
		close(p.closed)
	}
	return nil
}

func (p *pipeNet) Addr() net.Addr { return pipeAddr{} }

func (p *pipeNet) dial(_ context.Context, _, _ string) (net.Conn, error) {
	client, server := net.Pipe()
	select {
	case p.conns <- server:
		return client, nil
	case <-p.closed:
		return nil, net.ErrClosed
	}
}

type pipeAddr struct{}

func (pipeAddr) Network() string { return "pipe" }
func (pipeAddr) String() string  { return "stammtisch.invalid" }

// serveInMemory runs h on an HTTP server reached only through memory, and
// returns a client that reaches it.
func serveInMemory(t *testing.T, h http.Handler) *http.Client {
	t.Helper()

	listener := &pipeNet{conns: make(chan net.Conn), closed: make(chan struct{})}
	server := &http.Server{Handler: h, ReadHeaderTimeout: dialTimeout}

	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = server.Serve(listener)
	}()

	t.Cleanup(func() {
		_ = listener.Close()
		_ = server.Close()
		<-done
	})

	return &http.Client{Transport: &http.Transport{DialContext: listener.dial}}
}

// dial opens a WebSocket connection to a handler served in memory.
func dial(t *testing.T, client *http.Client, header http.Header) (*websocket.Conn, error) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), dialTimeout)
	t.Cleanup(cancel)

	peer, resp, err := websocket.Dial(ctx, "ws://stammtisch.invalid/", &websocket.DialOptions{
		HTTPClient: client,
		HTTPHeader: header,
	})
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		return nil, err
	}
	t.Cleanup(func() { _ = peer.CloseNow() })
	return peer, nil
}

// writeMessage sends bytes as exactly one WebSocket message, which is what
// lets a test decide how many frames share one message.
func writeMessage(t *testing.T, peer *websocket.Conn, payload []byte) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), dialTimeout)
	defer cancel()

	if err := peer.Write(ctx, websocket.MessageBinary, payload); err != nil {
		t.Fatalf("writing a %d byte message: %v", len(payload), err)
	}
}

// encodeFrames returns the bytes of frames laid end to end, which is what a
// peer using the framing writes.
func encodeFrames(t *testing.T, frames ...signalling.Frame) []byte {
	t.Helper()

	var buf bytes.Buffer
	for _, f := range frames {
		if err := signalling.Encode(&buf, f); err != nil {
			t.Fatalf("encoding a frame of kind %d: %v", f.Kind, err)
		}
	}
	return buf.Bytes()
}

// read waits for one value from ch, so a test that never gets there says what
// it was waiting for instead of hanging.
func read[T any](t *testing.T, ch <-chan T, what string) T {
	t.Helper()

	select {
	case v := <-ch:
		return v
	case <-time.After(dialTimeout):
		var zero T
		t.Fatalf("nothing arrived on %s", what)
		return zero
	}
}

// TestAFrameSentByAPeerArrivesThroughTheTransport is the round trip: the
// framing goes out over a real WebSocket handshake and comes back to a
// signalling.Conn with its kind and its bytes intact.
func TestAFrameSentByAPeerArrivesThroughTheTransport(t *testing.T) {
	t.Parallel()

	arrived := make(chan signalling.Frame, 1)
	client := serveInMemory(t, transport.Handler(func(conn *signalling.Conn) {
		f, err := conn.ReadFrame()
		if err != nil {
			t.Errorf("reading the first frame: %v", err)
			return
		}
		arrived <- f
		if err := conn.WriteFrame(signalling.Frame{Kind: signalling.KindSpaceState, Payload: []byte("back")}); err != nil {
			t.Errorf("writing the reply: %v", err)
		}
	}))

	peer, err := dial(t, client, nil)
	if err != nil {
		t.Fatalf("dialling: %v", err)
	}

	sent := signalling.Frame{Kind: signalling.KindAuthenticate, Payload: []byte("a credential")}
	writeMessage(t, peer, encodeFrames(t, sent))

	got := read(t, arrived, "the frame the server read")
	if got.Kind != sent.Kind || !bytes.Equal(got.Payload, sent.Payload) {
		t.Fatalf("the server read kind %d payload %q, want kind %d payload %q", got.Kind, got.Payload, sent.Kind, sent.Payload)
	}

	ctx, cancel := context.WithTimeout(context.Background(), dialTimeout)
	defer cancel()
	kind, reply, err := peer.Read(ctx)
	if err != nil {
		t.Fatalf("reading the reply: %v", err)
	}
	if kind != websocket.MessageBinary {
		t.Fatalf("the reply arrived as message type %v, want %v", kind, websocket.MessageBinary)
	}
	decoded, err := signalling.Decode(bytes.NewReader(reply))
	if err != nil {
		t.Fatalf("decoding the reply: %v", err)
	}
	if decoded.Kind != signalling.KindSpaceState || !bytes.Equal(decoded.Payload, []byte("back")) {
		t.Fatalf("the reply decoded to kind %d payload %q", decoded.Kind, decoded.Payload)
	}
}

// TestAPeerThatHasProvedNothingIsRefusedThroughTheTransport is the rule
// internal/signalling owes, asserted from the far end of a real connection
// rather than over an io.Pipe. Handler hands over a signalling.Conn and not the
// stream underneath it, and this is what that signature is for: there is no
// arrangement in which the first frame reaches a caller without the gate.
func TestAPeerThatHasProvedNothingIsRefusedThroughTheTransport(t *testing.T) {
	t.Parallel()

	refusal := make(chan error, 1)
	client := serveInMemory(t, transport.Handler(func(conn *signalling.Conn) {
		_, err := conn.ReadFrame()
		refusal <- err
	}))

	peer, err := dial(t, client, nil)
	if err != nil {
		t.Fatalf("dialling: %v", err)
	}

	writeMessage(t, peer, encodeFrames(t, signalling.Frame{Kind: signalling.KindSpaceState, Payload: []byte("before authenticating")}))

	err = read(t, refusal, "the server's verdict on the first frame")
	if !errors.Is(err, signalling.ErrUnauthenticated) {
		t.Fatalf("the server returned %v, want ErrUnauthenticated", err)
	}
}

// TestTheFrameBoundIsRefusedThroughTheTransport sends a header declaring more
// than the bound and no payload behind it. The refusal has to come from the
// framing's comparison rather than from the transport running out of bytes, so
// the assertion is on ErrFrameTooLarge and not on an error of any kind.
func TestTheFrameBoundIsRefusedThroughTheTransport(t *testing.T) {
	t.Parallel()

	verdict := make(chan error, 1)
	client := serveInMemory(t, transport.Handler(func(conn *signalling.Conn) {
		_, err := conn.ReadFrame()
		verdict <- err
	}))

	peer, err := dial(t, client, nil)
	if err != nil {
		t.Fatalf("dialling: %v", err)
	}

	// Written by hand rather than through Encode, because Encode refuses this
	// frame too and a peer that means harm is not using it.
	header := make([]byte, 5)
	binary.BigEndian.PutUint32(header[:4], signalling.MaxPayloadSize+1)
	header[4] = byte(signalling.KindAuthenticate)
	writeMessage(t, peer, header)

	err = read(t, verdict, "the server's verdict on the oversized header")
	if !errors.Is(err, signalling.ErrFrameTooLarge) {
		t.Fatalf("the server returned %v, want ErrFrameTooLarge", err)
	}
}

// TestOneMessageMayCarryMoreThanOneFrameAndMoreBytesThanTheBound is what says
// the stream is a stream. websocket.NetConn documents that it turns the
// library's read limit off, and this is the behaviour that makes that correct:
// a message is read a piece at a time, so its size is not a quantity anything
// allocates for, and the bound that does bite is the one the test above
// asserts. It reds if this package ever reads whole messages, and it reds if a
// read limit is set on the connection at or below the frame bound.
func TestOneMessageMayCarryMoreThanOneFrameAndMoreBytesThanTheBound(t *testing.T) {
	t.Parallel()

	const frames = 3
	kinds := make(chan []signalling.Kind, 1)
	client := serveInMemory(t, transport.Handler(func(conn *signalling.Conn) {
		conn.MarkAuthenticated()
		var read []signalling.Kind
		for range frames {
			f, err := conn.ReadFrame()
			if err != nil {
				t.Errorf("reading frame %d of %d: %v", len(read)+1, frames, err)
				break
			}
			read = append(read, f.Kind)
		}
		kinds <- read
	}))

	peer, err := dial(t, client, nil)
	if err != nil {
		t.Fatalf("dialling: %v", err)
	}

	// Each payload is half the bound, so the three together are half again as
	// many bytes as the largest frame this protocol will ever carry.
	payload := bytes.Repeat([]byte{0x5a}, signalling.MaxPayloadSize/2)
	message := encodeFrames(t,
		signalling.Frame{Kind: signalling.KindSpaceState, Payload: payload},
		signalling.Frame{Kind: signalling.KindSpaceState, Payload: payload},
		signalling.Frame{Kind: signalling.KindSpaceState, Payload: payload},
	)
	if len(message) <= signalling.MaxPayloadSize {
		t.Fatalf("the message is %d bytes and the bound is %d, so this test is not about what it says it is", len(message), signalling.MaxPayloadSize)
	}
	writeMessage(t, peer, message)

	got := read(t, kinds, "the kinds the server read out of one message")
	if len(got) != frames {
		t.Fatalf("the server read %d frames out of one %d byte message, want %d", len(got), len(message), frames)
	}
}

// TestARequestThatIsNotAHandshakeNeverReachesServe covers the plain GET
// somebody makes by opening the address in a browser. The refusal is
// websocket.Accept's and the property this asserts is that no signalling
// connection was made for it.
func TestARequestThatIsNotAHandshakeNeverReachesServe(t *testing.T) {
	t.Parallel()

	served := make(chan struct{}, 1)
	client := serveInMemory(t, transport.Handler(func(*signalling.Conn) {
		served <- struct{}{}
	}))

	ctx, cancel := context.WithTimeout(context.Background(), dialTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://stammtisch.invalid/", nil)
	if err != nil {
		t.Fatalf("building the request: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("making a plain request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusSwitchingProtocols {
		t.Fatalf("a plain GET was answered with %d", resp.StatusCode)
	}
	select {
	case <-served:
		t.Fatal("a plain GET reached serve, so a request that proved nothing got a signalling connection")
	default:
	}
}

// TestARequestFromAnotherOriginIsRefused is the cross-origin guard, and it is
// websocket.Accept's default rather than a line in this package. It reds if
// InsecureSkipVerify is set or if an OriginPatterns entry is added, which is
// the one-line widening somebody makes to get a client on another host working.
func TestARequestFromAnotherOriginIsRefused(t *testing.T) {
	t.Parallel()

	served := make(chan struct{}, 1)
	client := serveInMemory(t, transport.Handler(func(*signalling.Conn) {
		served <- struct{}{}
	}))

	_, err := dial(t, client, http.Header{"Origin": []string{"http://elsewhere.invalid"}})
	if err == nil {
		t.Fatal("a handshake carrying another host's Origin was accepted")
	}

	select {
	case <-served:
		t.Fatal("a cross-origin request reached serve")
	default:
	}
}

// TestTheSameHostOriginIsAccepted is the other half of the pair. Without it a
// guard that refused every Origin, including the operator's own page, would
// pass the test above and break the only client this project plans to have.
func TestTheSameHostOriginIsAccepted(t *testing.T) {
	t.Parallel()

	served := make(chan struct{}, 1)
	client := serveInMemory(t, transport.Handler(func(*signalling.Conn) {
		served <- struct{}{}
	}))

	if _, err := dial(t, client, http.Header{"Origin": []string{"http://stammtisch.invalid"}}); err != nil {
		t.Fatalf("a handshake from the request's own host was refused: %v", err)
	}

	read(t, served, "serve being reached by a same-host handshake")
}
