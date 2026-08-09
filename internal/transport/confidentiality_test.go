// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026  iderex

package transport_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/iderex/stammtisch/internal/signalling"
	"github.com/iderex/stammtisch/internal/transport"
)

// This file keeps the property the rest of the package's suite has: nothing
// binds an address. The connection that carries TLS below is the same in-memory
// pipe every other test uses, wrapped in a certificate made in the process that
// checks it, so a run needs no port, no privilege and no file on disk.

// serveOverTLS runs h behind a TLS connection reached only through memory, and
// returns a client that trusts the certificate it presents. What this buys over
// setting a field on a request by hand is that r.TLS is populated by net/http
// off a handshake that actually happened, so a guard reading it is being shown
// the thing it will see in a deployment.
func serveOverTLS(t *testing.T, h http.Handler) *http.Client {
	t.Helper()

	certificate, roots := selfSigned(t)

	listener := &pipeNet{conns: make(chan net.Conn), closed: make(chan struct{})}
	server := &http.Server{Handler: h, ReadHeaderTimeout: dialTimeout}

	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = server.Serve(tls.NewListener(listener, &tls.Config{
			Certificates: []tls.Certificate{certificate},
			MinVersion:   tls.VersionTLS13,
		}))
	}()

	t.Cleanup(func() {
		_ = listener.Close()
		_ = server.Close()
		<-done
	})

	return &http.Client{Transport: &http.Transport{
		DialContext: listener.dial,
		TLSClientConfig: &tls.Config{
			RootCAs:    roots,
			ServerName: pipeAddr{}.String(),
			MinVersion: tls.VersionTLS13,
		},
	}}
}

// selfSigned returns a certificate for the host the in-memory dialler answers
// to, and the pool that trusts it. It is generated per test rather than checked
// in, because a private key in the tree is a private key in the tree whatever
// the comment beside it says.
func selfSigned(t *testing.T) (tls.Certificate, *x509.CertPool) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generating a key: %v", err)
	}

	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: pipeAddr{}.String()},
		DNSNames:              []string{pipeAddr{}.String()},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("creating the certificate: %v", err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parsing the certificate back: %v", err)
	}

	roots := x509.NewCertPool()
	roots.AddCert(leaf)

	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: leaf}, roots
}

// dialSecure opens a WebSocket connection over TLS to a handler served in
// memory. It is dial's neighbour and differs only in the scheme, which is what
// decides whether net/http performs a handshake before the request.
func dialSecure(t *testing.T, client *http.Client) (*websocket.Conn, error) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), dialTimeout)
	t.Cleanup(cancel)

	peer, resp, err := websocket.Dial(ctx, "wss://"+pipeAddr{}.String()+"/", &websocket.DialOptions{
		HTTPClient: client,
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

// TestARequestThatDidNotArriveOverTLSIsRefused stands up the arrangement the
// requirement refuses and asserts the refusal, which is the deployment where a
// conversation crosses a network in the clear and nothing says so. The
// assertion that matters is the second one: a status code is a courtesy, and
// what the requirement is about is that no signalling connection was made.
func TestARequestThatDidNotArriveOverTLSIsRefused(t *testing.T) {
	t.Parallel()

	served := make(chan struct{}, 1)
	client := serveInMemory(t, transport.Handler(func(*signalling.Conn) {
		served <- struct{}{}
	}, transport.TLSHere))

	peer, err := dial(t, client, nil)
	if err == nil {
		_ = peer.CloseNow()
		t.Fatal("a handshake over a connection that is not confidential was accepted")
	}

	select {
	case <-served:
		t.Fatal("a request that did not arrive over TLS reached serve")
	default:
	}
}

// TestTheRefusalOfANonConfidentialRequestIsAnswered is the other half of the
// refusal. A guard that dropped the request without a response would pass the
// test above and leave a client author with a connection that failed for no
// stated reason.
func TestTheRefusalOfANonConfidentialRequestIsAnswered(t *testing.T) {
	t.Parallel()

	client := serveInMemory(t, transport.Handler(func(*signalling.Conn) {
		t.Error("serve was reached by a request that is not confidential")
	}, transport.TLSHere))

	ctx, cancel := context.WithTimeout(context.Background(), dialTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+pipeAddr{}.String()+"/", nil)
	if err != nil {
		t.Fatalf("building the request: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("making the request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("a request that is not confidential was answered with %d, want %d", resp.StatusCode, http.StatusForbidden)
	}
}

// TestARequestOverTLSReachesServe is the near miss that stops the guard above
// from being satisfied by refusing everything. The connection is a real TLS
// connection and r.TLS is set by net/http rather than by this test.
func TestARequestOverTLSReachesServe(t *testing.T) {
	t.Parallel()

	served := make(chan struct{}, 1)
	client := serveOverTLS(t, transport.Handler(func(*signalling.Conn) {
		served <- struct{}{}
	}, transport.TLSHere))

	if _, err := dialSecure(t, client); err != nil {
		t.Fatalf("a handshake over TLS was refused: %v", err)
	}

	read(t, served, "serve being reached by a handshake over TLS")
}

// TestTLSTerminatedAheadAdmitsARequestThatDidNotArriveOverTLS is the declared
// arrangement, and it is here because the value has to work for the deployment
// it describes: a reverse proxy terminating TLS and forwarding to this process.
// What it also fixes is how far the declaration reaches. Nothing here inspects
// where the request came from, so this test passing is the whole of what the
// value promises, and the record says the same thing in prose.
func TestTLSTerminatedAheadAdmitsARequestThatDidNotArriveOverTLS(t *testing.T) {
	t.Parallel()

	served := make(chan struct{}, 1)
	client := serveInMemory(t, transport.Handler(func(*signalling.Conn) {
		served <- struct{}{}
	}, transport.TLSTerminatedAhead))

	if _, err := dial(t, client, nil); err != nil {
		t.Fatalf("a forwarded handshake was refused under TLSTerminatedAhead: %v", err)
	}

	read(t, served, "serve being reached under TLSTerminatedAhead")
}

// TestATransitValueNobodyHandledRefuses is the one-character mistake: a
// constant added to the block later, or a conversion from an integer, arriving
// at a comparison written for the two values that existed when it was written.
// The refusal is what makes that mistake a red run rather than an open door.
func TestATransitValueNobodyHandledRefuses(t *testing.T) {
	t.Parallel()

	served := make(chan struct{}, 1)
	client := serveInMemory(t, transport.Handler(func(*signalling.Conn) {
		served <- struct{}{}
	}, transport.TLSTerminatedAhead+1))

	peer, err := dial(t, client, nil)
	if err == nil {
		_ = peer.CloseNow()
		t.Fatal("a Transit value no constant names was treated as a guarantee")
	}

	select {
	case <-served:
		t.Fatal("a Transit value no constant names reached serve")
	default:
	}
}
