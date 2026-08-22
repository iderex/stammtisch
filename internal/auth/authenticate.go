// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026  Nils Lehnen

package auth

import (
	"errors"
	"fmt"

	"golang.org/x/crypto/argon2"

	"github.com/iderex/stammtisch/internal/signalling"
)

// Accounts is where a stored credential comes from. What holds them is issue
// #27's to decide; this is the whole of what authentication needs from it.
type Accounts interface {
	// Stored returns the stored credential for name, or ErrNoSuchAccount.
	Stored(name string) (string, error)
}

var (
	// ErrNoSuchAccount is what an Accounts returns for a name it does not hold.
	// It never reaches a peer: Authenticate turns it into ErrAuthentication.
	ErrNoSuchAccount = errors.New("auth: no account of that name")

	// ErrAuthentication is the one answer a peer gets. An unknown account and a
	// wrong credential are the same sentence, because telling them apart is
	// telling an attacker which names exist.
	ErrAuthentication = errors.New("auth: the name and credential do not match an account")
)

// decoy is what an unknown account is verified against.
//
// This is the whole of the timing property and it is worth being explicit about
// why it is here rather than an early return. Looking a name up and returning
// at once when it is not held makes the unknown case orders of magnitude faster
// than the known one, and an attacker with a list of names and a stopwatch
// reads the account list off the difference. Verifying against a value with the
// current cost parameters makes the two paths do the same work.
//
// It is built as a string rather than by hashing something, because what
// matters is the parameters Matches reads out of it, and hashing at start-up
// would cost every process a verification it never uses. The key is zeroes,
// which no credential hashes to, so this never accidentally admits anybody.
var decoy = fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
	argon2.Version, argonMemory, argonTime, argonThreads,
	b64.EncodeToString(make([]byte, saltLen)),
	b64.EncodeToString(make([]byte, argonKeyLen)))

// Authenticate checks name and credential against accounts and, if they match,
// opens a session.
//
// An unknown name and a wrong credential both come back as ErrAuthentication
// and both take the same work, which is the property the decoy above exists for
// and TestAnUnknownAccountCostsTheSameAsAWrongCredential measures.
func Authenticate(accounts Accounts, sessions *Sessions, name, credential string) (Session, string, error) {
	stored, err := accounts.Stored(name)
	known := true
	switch {
	case errors.Is(err, ErrNoSuchAccount):
		known, stored = false, decoy
	case err != nil:
		return Session{}, "", fmt.Errorf("auth: reading the account: %w", err)
	}

	matched, err := Matches(stored, credential)
	if err != nil {
		return Session{}, "", err
	}
	if !known || !matched {
		return Session{}, "", ErrAuthentication
	}

	return sessions.Open(name)
}

// Admit reads the first frame of a signalling connection and opens the gate if
// it carries a token for a live session.
//
// The frame's kind is not checked here. signalling.Conn refuses every kind but
// the authentication frame before it is authenticated, and that refusal is
// terminal, so a second check in this function would be a copy of a rule that
// already holds and would drift from it.
//
// A connection that is not admitted is left unauthenticated. The caller closes
// it; nothing here writes an answer back, because what a refusal looks like on
// the wire is a message this package does not define.
func Admit(conn *signalling.Conn, sessions *Sessions) (Session, error) {
	frame, err := conn.ReadFrame()
	if err != nil {
		return Session{}, err
	}

	session, err := sessions.Lookup(string(frame.Payload))
	if err != nil {
		return Session{}, err
	}

	conn.MarkAuthenticated()
	return session, nil
}
