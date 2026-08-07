// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026  iderex

package orchestration

import (
	"errors"
	"fmt"
	"strings"
)

// ID is a globally addressable identifier: a local part and a host, written
// local@host.
//
// The host is carried even though this release does not federate and there is
// only ever one of them. docs/decisions/federation.md places that obligation
// here by name: the form is fixed now so it does not have to change later and
// every stored identifier does not have to be rewritten. An identifier that
// grows a host component after the fact is a migration of every row that holds
// one.
//
// The zero ID is not a valid identifier. Both fields are unexported and the only
// way to build one is NewID or ParseID, so a value that reaches a caller has
// been through the checks below or is the zero value, which every constructor in
// this package refuses.
type ID struct {
	local string
	host  string
}

// ErrInvalidID is returned by NewID and ParseID. It is one error rather than one
// per rule because a caller either has a usable identifier or does not; the
// detail is in the message and the tests assert on the message.
var ErrInvalidID = errors.New("orchestration: invalid identifier")

// NewID returns the identifier local@host.
//
// It refuses an empty part, a part carrying the separator, and a part carrying
// a space or any other ASCII control or whitespace character. The separator rule
// is what makes ParseID and String round-trip: an identifier whose local part
// contained an @ would parse back as a different identifier, silently.
func NewID(local, host string) (ID, error) {
	if err := checkIDPart("local part", local); err != nil {
		return ID{}, err
	}
	if err := checkIDPart("host", host); err != nil {
		return ID{}, err
	}
	return ID{local: local, host: host}, nil
}

// ParseID reads local@host.
func ParseID(s string) (ID, error) {
	local, host, found := strings.Cut(s, "@")
	if !found {
		return ID{}, fmt.Errorf("%w: %q carries no @ separating the local part from the host", ErrInvalidID, s)
	}
	return NewID(local, host)
}

// String returns local@host. The zero ID formats as the empty string rather than
// as "@", so a zero value that reaches a log is recognisable as one.
func (id ID) String() string {
	if id.IsZero() {
		return ""
	}
	return id.local + "@" + id.host
}

// Local returns the part before the @.
func (id ID) Local() string { return id.local }

// Host returns the part after the @.
func (id ID) Host() string { return id.host }

// IsZero reports whether this is the zero ID, which no constructor produces.
func (id ID) IsZero() bool { return id.local == "" && id.host == "" }

func checkIDPart(what, s string) error {
	if s == "" {
		return fmt.Errorf("%w: the %s is empty", ErrInvalidID, what)
	}
	if strings.Contains(s, "@") {
		return fmt.Errorf("%w: the %s %q carries an @, which is the separator", ErrInvalidID, what, s)
	}
	for _, r := range s {
		if r <= ' ' || r == 0x7f {
			return fmt.Errorf("%w: the %s %q carries a control or space character", ErrInvalidID, what, s)
		}
	}
	return nil
}
