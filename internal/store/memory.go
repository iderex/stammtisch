// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026  iderex

package store

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/iderex/stammtisch/internal/orchestration"
)

// Memory is the in-memory implementation of Store.
//
// It is not a stub with the methods filled in to compile. It holds the same
// refusals the durable implementation holds, because a suite that runs against
// a permissive fake proves the code under test against rules nothing will
// enforce at runtime. Where the two implementations are allowed to differ is
// nowhere: the contract suite in internal/store/sqlite runs one table against
// both and asserts the answers match.
//
// It is safe for concurrent use. The durable store is, because a database
// handle is, and an in-memory twin that was not would turn a race into a
// failure that only reproduces without the database.
type Memory struct {
	mu       sync.RWMutex
	channels map[string]*orchestration.Channel
	members  map[string]map[string]orchestration.ID
	grants   map[grantKey]struct{}
	gains    map[gainKey]int
}

type grantKey struct {
	principal  string
	space      string
	channel    string
	permission string
}

type gainKey struct {
	listener string
	speaker  string
}

// Memory is a Store, and this is where that is checked rather than assumed.
var _ Store = (*Memory)(nil)

// NewMemory returns an empty in-memory store.
func NewMemory() *Memory {
	return &Memory{
		channels: map[string]*orchestration.Channel{},
		members:  map[string]map[string]orchestration.ID{},
		grants:   map[grantKey]struct{}{},
		gains:    map[gainKey]int{},
	}
}

// PutChannel writes a channel.
func (m *Memory) PutChannel(_ context.Context, c *orchestration.Channel) error {
	if c == nil {
		return fmt.Errorf("%w: no channel", ErrInvalid)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.channels[c.ID().String()] = c
	return nil
}

// Channel reads a channel back.
func (m *Memory) Channel(_ context.Context, id orchestration.ID) (*orchestration.Channel, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	c, held := m.channels[id.String()]
	if !held {
		return nil, fmt.Errorf("%w: no channel %s", ErrNotFound, id)
	}
	return c, nil
}

// ChannelsInSpace returns the channels of one space in identifier order.
func (m *Memory) ChannelsInSpace(_ context.Context, space orchestration.ID) ([]*orchestration.Channel, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var found []*orchestration.Channel
	for _, c := range m.channels {
		if c.Space() == space {
			found = append(found, c)
		}
	}
	sort.Slice(found, func(i, j int) bool { return found[i].ID().String() < found[j].ID().String() })
	return found, nil
}

// PutMember records a member of a space.
func (m *Memory) PutMember(_ context.Context, space, member orchestration.ID) error {
	if err := checkPair("a membership", space, member); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	held, known := m.members[space.String()]
	if !known {
		held = map[string]orchestration.ID{}
		m.members[space.String()] = held
	}
	held[member.String()] = member
	return nil
}

// Members returns the members of a space in identifier order.
func (m *Memory) Members(_ context.Context, space orchestration.ID) ([]orchestration.ID, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var found []orchestration.ID
	for _, member := range m.members[space.String()] {
		found = append(found, member)
	}
	sort.Slice(found, func(i, j int) bool { return found[i].String() < found[j].String() })
	return found, nil
}

// Grant adds permissions for a principal on a subject.
func (m *Memory) Grant(_ context.Context, principal orchestration.ID, subject orchestration.Subject, granted ...orchestration.Permission) error {
	if principal.IsZero() {
		return fmt.Errorf("%w: a grant needs a principal", ErrInvalid)
	}
	if subject.Space().IsZero() {
		return fmt.Errorf("%w: a grant needs a subject and this one names no space", ErrInvalid)
	}
	for _, p := range granted {
		if _, err := ParsePermission(p.String()); err != nil {
			return err
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, p := range granted {
		m.grants[grantKey{principal.String(), subject.Space().String(), subjectChannel(subject), p.String()}] = struct{}{}
	}
	return nil
}

// Granted answers what a principal holds on a subject.
//
// It keys on the principal's identifier alone, because that is all
// orchestration.Principal exposes. The comment on the unexported kind field
// there says the field exists so that a store can tell two principals with the
// same identifier apart, and no store can: there is no accessor. That is
// recorded on issue #27 rather than worked around here, and it is safe today
// only because an identifier is local@host and nothing mints the same one for a
// person and a bot.
//
// A permission this model does not declare cannot come back out, because the
// answer is built by asking the declared set rather than by reading whatever
// happens to be stored.
func (m *Memory) Granted(p orchestration.Principal, s orchestration.Subject) orchestration.PermissionSet {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var held []orchestration.Permission
	for _, perm := range orchestration.Permissions() {
		key := grantKey{p.ID().String(), s.Space().String(), subjectChannel(s), perm.String()}
		if _, ok := m.grants[key]; ok {
			held = append(held, perm)
		}
	}
	return orchestration.NewPermissionSet(held...)
}

// SetGain records a listener's setting for a speaker.
func (m *Memory) SetGain(_ context.Context, listener, speaker orchestration.ID, percent int) error {
	if err := checkPair("a gain setting", listener, speaker); err != nil {
		return err
	}
	if err := checkGain(percent); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	key := gainKey{listener.String(), speaker.String()}
	if percent == DefaultGain {
		delete(m.gains, key)
		return nil
	}
	m.gains[key] = percent
	return nil
}

// Gain returns the setting, or DefaultGain where there is no row.
func (m *Memory) Gain(_ context.Context, listener, speaker orchestration.ID) (int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if percent, set := m.gains[gainKey{listener.String(), speaker.String()}]; set {
		return percent, nil
	}
	return DefaultGain, nil
}

// Close releases nothing and says so.
func (m *Memory) Close() error { return nil }
