// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026  iderex

package auth

import (
	"bytes"
	"errors"
	"sort"
	"testing"
	"time"

	"github.com/iderex/stammtisch/internal/signalling"
)

// accounts is an Accounts a test fills by hand. What holds accounts on a real
// server is #27 and nothing here waits on it.
type accounts map[string]string

func (a accounts) Stored(name string) (string, error) {
	stored, held := a[name]
	if !held {
		return "", ErrNoSuchAccount
	}
	return stored, nil
}

// failingAccounts is the third case, which is neither a hit nor a miss: the
// store itself could not answer.
type failingAccounts struct{ err error }

func (f failingAccounts) Stored(string) (string, error) { return "", f.err }

func newAccount(t *testing.T, name, credential string) accounts {
	t.Helper()
	stored, err := Store(credential)
	if err != nil {
		t.Fatalf("Store: %v", err)
	}
	return accounts{name: stored}
}

func TestTheRightCredentialOpensASession(t *testing.T) {
	held := newAccount(t, "someone", "the right credential")
	sessions := NewSessions(newFakeClock())

	session, token, err := Authenticate(held, sessions, "someone", "the right credential")
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if session.Owner != "someone" {
		t.Errorf("session owner is %q", session.Owner)
	}
	if _, err := sessions.Lookup(token); err != nil {
		t.Errorf("the token Authenticate returned does not look up: %v", err)
	}
}

func TestAWrongCredentialAndAnUnknownNameGiveOneAnswer(t *testing.T) {
	held := newAccount(t, "someone", "the right credential")
	sessions := NewSessions(newFakeClock())

	for _, c := range []struct{ name, credential string }{
		{"someone", "the wrong credential"},
		{"nobody", "the right credential"},
		{"", ""},
	} {
		_, _, err := Authenticate(held, sessions, c.name, c.credential)
		if !errors.Is(err, ErrAuthentication) {
			t.Errorf("Authenticate(%q, %q) returned %v, want ErrAuthentication", c.name, c.credential, err)
		}
	}

	if n := len(sessions.List("someone")) + len(sessions.List("nobody")); n != 0 {
		t.Errorf("%d sessions were opened by refused attempts", n)
	}
}

func TestAFailingAccountStoreIsNeitherAMatchNorARefusal(t *testing.T) {
	sentinel := errors.New("the store is unreachable")
	sessions := NewSessions(newFakeClock())

	_, _, err := Authenticate(failingAccounts{err: sentinel}, sessions, "someone", "anything")
	if !errors.Is(err, sentinel) {
		t.Fatalf("Authenticate returned %v, want the store's own error", err)
	}
	if errors.Is(err, ErrAuthentication) {
		t.Error("a store that could not answer was reported as a wrong credential")
	}
}

func TestAMalformedStoredCredentialIsAnError(t *testing.T) {
	sessions := NewSessions(newFakeClock())

	_, _, err := Authenticate(accounts{"someone": "not a stored credential"}, sessions, "someone", "anything")
	if !errors.Is(err, ErrMalformedCredential) {
		t.Fatalf("Authenticate returned %v, want ErrMalformedCredential", err)
	}
}

// The timing property, and the bound is stated here rather than tuned until it
// passes.
//
// What this catches is an early return on an unknown account, which is the
// mistake the decoy exists against. That mistake is not a small difference: an
// early return skips a memory-hard function at 64 MiB and three passes, so the
// unknown path becomes faster by three or four orders of magnitude. The bound
// is therefore set at a factor of two, which is enormous margin against a
// shared runner's noise and still leaves no room at all for the failure being
// measured. A tighter bound would buy nothing and would red on somebody else's
// change.
//
// Medians rather than means, because one scheduling stall in one iteration
// moves a mean and does not move a median.
func TestAnUnknownAccountCostsTheSameAsAWrongCredential(t *testing.T) {
	held := newAccount(t, "someone", "the right credential")
	sessions := NewSessions(newFakeClock())

	const rounds = 5
	median := func(name, credential string) time.Duration {
		taken := make([]time.Duration, 0, rounds)
		for i := 0; i < rounds; i++ {
			start := time.Now()
			if _, _, err := Authenticate(held, sessions, name, credential); !errors.Is(err, ErrAuthentication) {
				t.Fatalf("Authenticate(%q) returned %v, want ErrAuthentication", name, err)
			}
			taken = append(taken, time.Since(start))
		}
		sort.Slice(taken, func(i, j int) bool { return taken[i] < taken[j] })
		return taken[len(taken)/2]
	}

	unknown := median("nobody at all", "anything")
	wrong := median("someone", "the wrong credential")

	slower, faster := unknown, wrong
	if faster > slower {
		slower, faster = faster, slower
	}
	if faster <= 0 {
		t.Fatalf("a verification took no measurable time: unknown %v, wrong %v", unknown, wrong)
	}
	if slower > 2*faster {
		t.Fatalf("unknown account took %v and a wrong credential took %v, and the bound is a factor of two", unknown, wrong)
	}
}

func TestALiveTokenAdmitsAConnection(t *testing.T) {
	sessions := NewSessions(newFakeClock())
	opened, token, err := sessions.Open("someone")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	conn := signalling.NewConn(peerSending(t, token))
	admitted, err := Admit(conn, sessions)
	if err != nil {
		t.Fatalf("Admit: %v", err)
	}
	if admitted.ID != opened.ID {
		t.Errorf("admitted %q, want %q", admitted.ID, opened.ID)
	}
	if !conn.Authenticated() {
		t.Error("the connection was not opened")
	}
}

// The line issue #28 asks for by name: a revoked session cannot open a
// signalling connection.
func TestARevokedSessionCannotOpenAConnection(t *testing.T) {
	sessions := NewSessions(newFakeClock())
	opened, token, err := sessions.Open("someone")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if !sessions.Revoke(opened.ID) {
		t.Fatal("Revoke reported nothing to revoke")
	}

	conn := signalling.NewConn(peerSending(t, token))
	if _, err := Admit(conn, sessions); !errors.Is(err, ErrNoSuchSession) {
		t.Fatalf("Admit returned %v, want ErrNoSuchSession", err)
	}
	if conn.Authenticated() {
		t.Fatal("a revoked session opened a connection")
	}
}

// The second half of that line: an expired token cannot either.
func TestAnExpiredTokenCannotOpenAConnection(t *testing.T) {
	clock := newFakeClock()
	sessions := NewSessions(clock)
	if _, _, err := sessions.Open("someone"); err != nil {
		t.Fatalf("Open: %v", err)
	}
	_, token, err := sessions.Open("someone")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	clock.advance(TokenLifetime)

	conn := signalling.NewConn(peerSending(t, token))
	if _, err := Admit(conn, sessions); !errors.Is(err, ErrSessionExpired) {
		t.Fatalf("Admit returned %v, want ErrSessionExpired", err)
	}
	if conn.Authenticated() {
		t.Fatal("an expired token opened a connection")
	}
}

func TestAdmitReportsAConnectionThatSaidNothing(t *testing.T) {
	sessions := NewSessions(newFakeClock())
	conn := signalling.NewConn(&stream{})

	if _, err := Admit(conn, sessions); err == nil {
		t.Fatal("Admit accepted a connection that sent nothing")
	}
	if conn.Authenticated() {
		t.Error("a connection that sent nothing was opened")
	}
}

// stream is a byte stream with a read side and a write side, which is what a
// connection is. No socket and no goroutine.
type stream struct {
	in  bytes.Reader
	out bytes.Buffer
}

func (s *stream) Read(p []byte) (int, error)  { return s.in.Read(p) }
func (s *stream) Write(p []byte) (int, error) { return s.out.Write(p) }

// peerSending returns a stream carrying one authentication frame with token in
// it, which is what a client sends first.
func peerSending(t *testing.T, token string) *stream {
	t.Helper()

	var wire bytes.Buffer
	if err := signalling.Encode(&wire, signalling.Frame{
		Kind:    signalling.KindAuthenticate,
		Payload: []byte(token),
	}); err != nil {
		t.Fatalf("Encode: %v", err)
	}

	s := &stream{}
	s.in.Reset(wire.Bytes())
	return s
}
