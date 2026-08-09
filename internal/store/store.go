// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026  iderex

package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/iderex/stammtisch/internal/orchestration"
)

// ErrNotFound is returned when a lookup names something the store does not
// hold. It is distinct from an error, because "there is no such channel" is an
// answer and a failed read is not.
var ErrNotFound = errors.New("store: not found")

// ErrInvalid is returned when a value offered to the store, or read back out of
// it, cannot be a domain value. A row that cannot become a Channel is refused
// at this boundary rather than handed on as one, because a domain type in this
// tree carries its invariants in its constructor and a store that bypassed the
// constructor would be the one place they do not hold.
var ErrInvalid = errors.New("store: invalid")

// ErrNewerSchema is returned when a store is opened against data written by a
// later version of this software. The refusal is the point: an older binary
// that carried on would write rows the newer schema's constraints were meant to
// refuse, and the corruption would be discovered by the newer binary later.
var ErrNewerSchema = errors.New("store: the data is from a newer schema than this build knows")

// Gain is a per-listener, per-speaker setting in percent of unity amplitude.
//
// The scale and its ends are docs/decisions/per-person-volume.md's, quoted here
// as constants rather than as prose so a caller cannot disagree with the record
// silently. DefaultGain is unity, and a setting equal to it is not stored, so a
// listener who has never touched the control has no rows.
const (
	MinGain     = 0
	DefaultGain = 100
	MaxGain     = 200
)

// Store is the durable half of the domain.
//
// It embeds orchestration.Grantor, so a store is what a permission question is
// answered against. That embedding is the reason this port exists in the shape
// it does: orchestration.Allow takes a Grantor, and the whole point of the
// in-memory implementation below is that the orchestration suite can hand it
// one without a database.
//
// Every method takes a context. Nothing in the in-memory implementation uses
// it, and that is not an argument for leaving it out: an interface whose
// durable implementation needs cancellation and whose fake does not is an
// interface with the fake's requirements.
type Store interface {
	orchestration.Grantor

	// PutChannel writes a channel, replacing one with the same identifier.
	PutChannel(ctx context.Context, c *orchestration.Channel) error
	// Channel reads one back. It returns ErrNotFound when there is none.
	Channel(ctx context.Context, id orchestration.ID) (*orchestration.Channel, error)
	// ChannelsInSpace returns every channel in a space, in identifier
	// order. The order is fixed rather than the store's, so a caller cannot
	// come to depend on insertion order in one implementation and break
	// against the other.
	ChannelsInSpace(ctx context.Context, space orchestration.ID) ([]*orchestration.Channel, error)

	// PutMember records that a member belongs to a space. Recording one
	// twice is not an error.
	PutMember(ctx context.Context, space, member orchestration.ID) error
	// Members returns the members of a space, in identifier order.
	Members(ctx context.Context, space orchestration.ID) ([]orchestration.ID, error)

	// Grant adds permissions for a principal on a subject. It is additive:
	// granting something already granted is not an error, and nothing here
	// revokes, because revocation is a moderation action with an ordering
	// requirement of its own and it is issue #39 rather than this port.
	Grant(ctx context.Context, principal orchestration.ID, subject orchestration.Subject, granted ...orchestration.Permission) error

	// SetGain records a listener's setting for one speaker. A value equal to
	// DefaultGain removes the row rather than writing unity, which is the
	// record's rule and not an optimisation.
	SetGain(ctx context.Context, listener, speaker orchestration.ID, percent int) error
	// Gain returns the setting, or DefaultGain where there is no row. There
	// is no ErrNotFound here for the same reason: no row is the default and
	// not an absence a caller has to handle.
	Gain(ctx context.Context, listener, speaker orchestration.ID) (int, error)

	// Close releases whatever the implementation holds. Closing twice is not
	// an error.
	Close() error
}

// ParsePermission returns the permission with this name, as Permission.String
// writes it.
//
// Grants are stored by name rather than by the constant's number. The numbers
// are declaration order in orchestration.Permission, so inserting a permission
// in the middle of that block would silently repoint every stored row; the
// names do not move. It reads the declared set from the model rather than
// carrying a second copy of it, so a permission added there is parseable here
// without anybody editing this function.
func ParsePermission(name string) (orchestration.Permission, error) {
	for _, p := range orchestration.Permissions() {
		if p.String() == name {
			return p, nil
		}
	}
	return 0, fmt.Errorf("%w: %q is not a permission this model declares", ErrInvalid, name)
}

// checkGain refuses a setting outside the record's range.
func checkGain(percent int) error {
	if percent < MinGain || percent > MaxGain {
		return fmt.Errorf("%w: a gain is %d to %d percent of unity and %d is outside that", ErrInvalid, MinGain, MaxGain, percent)
	}
	return nil
}

// checkPair refuses a zero identifier in either position. The zero ID is not a
// valid identifier anywhere in the domain, and a store that accepted one would
// hold a row no lookup built from a real identifier could ever reach.
func checkPair(what string, a, b orchestration.ID) error {
	if a.IsZero() || b.IsZero() {
		return fmt.Errorf("%w: %s needs two identifiers and one of them is the zero value", ErrInvalid, what)
	}
	return nil
}

// subjectChannel returns the channel a subject is about, or the empty string
// where it is about a space. The empty string is how a space subject is written
// in a store: the zero ID formats as the empty string already, and no valid
// identifier can, so the two cases cannot collide.
func subjectChannel(s orchestration.Subject) string {
	channel, aboutChannel := s.Channel()
	if !aboutChannel {
		return ""
	}
	return channel.String()
}
