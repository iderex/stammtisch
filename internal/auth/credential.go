// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026  iderex

package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// The cost parameters. They are a floor rather than a setting, and
// credential_test.go refuses a change that lowers any of them.
//
// Argon2id is the variant, because it is the one that resists both a
// side-channel attack on the memory access pattern and a time-memory trade-off
// on a graphics card, and an attacker who has the stored values is assumed to
// have both kinds of hardware.
//
// The memory figure is what makes the function memory-hard, so it is the one
// that matters most and the one an operator is tempted to lower on a small box.
// 64 MiB per verification at four lanes is the widely published floor for a
// server-side interactive login, and it is the number this project would have
// to argue against rather than the number it has to argue for. Three passes is
// the same source's companion figure at that memory. The key is 32 bytes
// because a comparison is only as strong as the shorter side and 32 bytes is
// past the point where the comparison is the weak part.
//
// Raising any of them is a change to these constants and nothing else. A stored
// credential carries the parameters it was made with, so raising the cost
// leaves every existing value verifiable at the cost it was written at, and
// re-hashing on the next successful verification is what moves it. That
// re-hashing is not here; it belongs with the account that owns the credential.
const (
	argonTime    uint32 = 3
	argonMemory  uint32 = 64 * 1024 // KiB, so 64 MiB
	argonThreads uint8  = 4
	argonKeyLen  uint32 = 32
	saltLen             = 16
)

// ErrMalformedCredential is returned when a stored value cannot be read as one
// this package wrote. It is deliberately one error: a caller either has a
// credential it can compare against or does not, and telling a caller which
// field was wrong tells an attacker the same thing.
var ErrMalformedCredential = errors.New("auth: the stored credential cannot be read")

// randRead is crypto/rand.Read, indirected so the test can fail it. A salt that
// silently comes back short is the failure this indirection exists to let a
// test reach, and there is no other way to reach it.
var randRead = rand.Read

// b64 is the encoding the stored form uses: standard alphabet, no padding, so a
// stored value carries no character that needs escaping wherever it is put.
var b64 = base64.RawStdEncoding

// Store returns the value to hold for credential. It is not reversible and the
// same credential stored twice gives two different values, because the salt is
// drawn fresh each time.
//
// The form is the one Argon2's own reference implementation writes, so a stored
// value is readable by something other than this package:
//
//	$argon2id$v=19$m=65536,t=3,p=4$<salt>$<key>
func Store(credential string) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := randRead(salt); err != nil {
		return "", fmt.Errorf("auth: drawing a salt: %w", err)
	}

	key := argon2.IDKey([]byte(credential), salt, argonTime, argonMemory, argonThreads, argonKeyLen)

	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads,
		b64.EncodeToString(salt), b64.EncodeToString(key)), nil
}

// Matches reports whether credential is the one stored is a record of.
//
// It reads the cost parameters out of the stored value rather than using the
// constants above, so a value written before the cost was raised still
// verifies. The comparison is constant time in the two keys, because a
// comparison that stops at the first differing byte tells an attacker how much
// of a guess was right.
//
// A malformed stored value is an error and never a false. Those are different
// statements: one says the credential was wrong and the other says the server
// cannot tell, and collapsing them turns a corrupted record into a login that
// merely fails.
func Matches(stored, credential string) (bool, error) {
	params, salt, key, err := parse(stored)
	if err != nil {
		return false, err
	}

	candidate := argon2.IDKey([]byte(credential), salt, params.time, params.memory, params.threads, uint32(len(key)))

	return subtle.ConstantTimeCompare(candidate, key) == 1, nil
}

type params struct {
	memory  uint32
	time    uint32
	threads uint8
}

// parse reads the stored form back. Every failure returns
// ErrMalformedCredential with a detail, and the detail is for a log rather than
// for the peer.
func parse(stored string) (params, []byte, []byte, error) {
	field := strings.Split(stored, "$")
	// A leading $ makes the first field empty, so the six are: "", the variant,
	// the version, the costs, the salt and the key.
	if len(field) != 6 || field[0] != "" {
		return params{}, nil, nil, fmt.Errorf("%w: it has %d fields", ErrMalformedCredential, len(field))
	}
	if field[1] != "argon2id" {
		return params{}, nil, nil, fmt.Errorf("%w: the variant is %q and this package writes argon2id", ErrMalformedCredential, field[1])
	}

	var version int
	if _, err := fmt.Sscanf(field[2], "v=%d", &version); err != nil {
		return params{}, nil, nil, fmt.Errorf("%w: the version field is %q", ErrMalformedCredential, field[2])
	}
	if version != argon2.Version {
		return params{}, nil, nil, fmt.Errorf("%w: the version is %d and this build implements %d", ErrMalformedCredential, version, argon2.Version)
	}

	var p params
	if _, err := fmt.Sscanf(field[3], "m=%d,t=%d,p=%d", &p.memory, &p.time, &p.threads); err != nil {
		return params{}, nil, nil, fmt.Errorf("%w: the cost field is %q", ErrMalformedCredential, field[3])
	}
	if p.memory == 0 || p.time == 0 || p.threads == 0 {
		return params{}, nil, nil, fmt.Errorf("%w: a cost of zero is not a cost", ErrMalformedCredential)
	}

	salt, err := b64.DecodeString(field[4])
	if err != nil {
		return params{}, nil, nil, fmt.Errorf("%w: the salt is not base64", ErrMalformedCredential)
	}
	key, err := b64.DecodeString(field[5])
	if err != nil {
		return params{}, nil, nil, fmt.Errorf("%w: the key is not base64", ErrMalformedCredential)
	}
	if len(salt) == 0 || len(key) == 0 {
		return params{}, nil, nil, fmt.Errorf("%w: the salt is %d bytes and the key is %d", ErrMalformedCredential, len(salt), len(key))
	}

	return p, salt, key, nil
}
