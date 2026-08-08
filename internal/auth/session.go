// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026  iderex

package auth

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
)

// Clock is the only way this package learns what time it is, for the reason
// internal/orchestration gives for its own: a suite that waits for a token to
// expire is slow, flaky, and unable to test the expiry at all.
//
// It is declared here rather than imported so neither package depends on the
// other for one method. Anything that satisfies orchestration's Clock satisfies
// this one, so a server builds one implementation and hands it to both.
type Clock interface {
	Now() time.Time
}

// Errors a caller can branch on. A session that does not exist, one that was
// revoked and one that has expired are one error to the peer and three to the
// server, which is why the distinction is in the type and not in the message
// sent back.
var (
	// ErrNoSuchSession covers a token nothing was ever issued for and a token
	// that was revoked. Those are the same statement to a caller: the token
	// buys nothing. Keeping a revoked token distinguishable from an invented
	// one tells a holder of a stolen token that it was real.
	ErrNoSuchSession = errors.New("auth: no session for that token")

	// ErrSessionExpired is separate because it is the one case where the right
	// answer for the client is to authenticate again rather than to stop.
	ErrSessionExpired = errors.New("auth: the session has expired")
)

// TokenLifetime is how long a session token is good for.
//
// Fifteen minutes is short enough that revocation means something, which is
// what issue #28 asks for: a token that outlives the decision to revoke it is a
// decision nobody enforced. It is not a session length. A client renews while
// it is connected, and renewal is what the reconnect and resume work in #34
// carries rather than this file.
const TokenLifetime = 15 * time.Minute

// tokenBytes is the entropy in a token, before encoding. 32 bytes is past the
// point where guessing is the attack anybody would choose. idBytes is the
// session identifier, which is public: it is random rather than sequential so a
// listing does not say how many sessions the server has issued.
const (
	tokenBytes = 32
	idBytes    = 16
)

// A Session is one authenticated connection's right to exist: who it belongs
// to, when it was opened, and when the token stops working.
//
// The token itself is not in here. It is returned once, by Open, and never
// stored in a form the server could hand back. What the store holds is a digest
// of it, so a copy of the session table is not a set of working tokens.
type Session struct {
	ID      string
	Owner   string
	Opened  time.Time
	Expires time.Time
}

// Sessions holds the sessions a server has open.
//
// It is in memory and it says so. What makes a session survive a restart is
// persistence, which is issue #27, and the shape here is the one that issue
// implements behind rather than a decision it has to argue with: open, look up,
// revoke, and list by owner.
type Sessions struct {
	mu    sync.Mutex
	clock Clock
	held  map[string]Session // keyed by the token digest
}

// NewSessions returns an empty store reading time from clock.
func NewSessions(clock Clock) *Sessions {
	return &Sessions{clock: clock, held: map[string]Session{}}
}

// Open starts a session for owner and returns the token exactly once.
//
// The token is the only time the caller sees it. Everything after this point
// works from the digest, so the store cannot hand a token back to anybody,
// including to the person who owns it.
func (s *Sessions) Open(owner string) (Session, string, error) {
	if owner == "" {
		return Session{}, "", fmt.Errorf("auth: a session needs an owner")
	}

	// The token and the identifier are drawn together, so there is one place
	// this can fail and one error path to prove. The identifier is public and
	// appears in a listing; the token is not and is returned once. They are
	// separate bytes of one draw rather than one derived from the other,
	// because an identifier a listing shows must say nothing about the token.
	raw := make([]byte, tokenBytes+idBytes)
	if _, err := randRead(raw); err != nil {
		return Session{}, "", fmt.Errorf("auth: drawing a token: %w", err)
	}
	token := b64.EncodeToString(raw[:tokenBytes])

	now := s.clock.Now()
	session := Session{
		ID:      b64.EncodeToString(raw[tokenBytes:]),
		Owner:   owner,
		Opened:  now,
		Expires: now.Add(TokenLifetime),
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.held[digest(token)] = session

	return session, token, nil
}

// Lookup returns the session token belongs to.
//
// An expired session is reported as expired and left in place. Removing it here
// would make the answer depend on who asked first, and the owner listing their
// own sessions is owed a true picture rather than one the last lookup edited.
func (s *Sessions) Lookup(token string) (Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	session, held := s.held[digest(token)]
	if !held {
		return Session{}, ErrNoSuchSession
	}
	if !s.clock.Now().Before(session.Expires) {
		return Session{}, ErrSessionExpired
	}
	return session, nil
}

// Revoke ends the session with this id, whoever holds its token.
//
// It reports whether anything was revoked, so a caller can tell a revocation
// from a no-op. Revoking twice is not an error: the state afterwards is the
// state that was asked for.
func (s *Sessions) Revoke(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	for key, session := range s.held {
		if subtle.ConstantTimeCompare([]byte(session.ID), []byte(id)) == 1 {
			delete(s.held, key)
			return true
		}
	}
	return false
}

// List returns owner's sessions, oldest first, expired ones included and marked
// by their own Expires field.
//
// A self-hosted service whose owner cannot see their own sessions is worse than
// the commercial one it replaces, which is why this exists and why it hides
// nothing from the person it belongs to. It carries no token, because there is
// none to carry.
func (s *Sessions) List(owner string) []Session {
	s.mu.Lock()
	defer s.mu.Unlock()

	var found []Session
	for _, session := range s.held {
		if session.Owner == owner {
			found = append(found, session)
		}
	}
	sort.Slice(found, func(i, j int) bool {
		if found[i].Opened.Equal(found[j].Opened) {
			return found[i].ID < found[j].ID
		}
		return found[i].Opened.Before(found[j].Opened)
	})
	return found
}

// digest is what the store is keyed on. A token is 32 random bytes, so it has
// no guessable structure and needs no memory-hard function over it; what a
// digest buys is that a copy of the table is not a set of working tokens.
func digest(token string) string {
	sum := sha256.Sum256([]byte(token))
	return base64.RawStdEncoding.EncodeToString(sum[:])
}
