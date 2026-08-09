// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026  iderex

// Package sqlite is the durable implementation of the store port.
//
// It is the only package in this tree that imports a database driver, and
// nothing under internal/orchestration reaches it. That is the arrangement the
// port exists for: the orchestration suite holds a real store through
// internal/store and never links a driver, which is what keeps it runnable on a
// bare runner with no service to start.
//
// The driver is modernc.org/sqlite, a translation of SQLite into Go rather than
// a binding to it. What that buys is a server binary with no CGo, no build
// toolchain on the operator's machine and no shared library to match at
// runtime, which is the whole install experience issue #27 argued the store
// choice from. What it costs is modernc.org/libc, which is the largest single
// thing in the module graph this package adds.
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	// The driver registers itself under the name below. It is imported for
	// that effect only, which is what the blank import says.
	_ "modernc.org/sqlite"

	"github.com/iderex/stammtisch/internal/orchestration"
	"github.com/iderex/stammtisch/internal/store"
)

// driverName is the name modernc.org/sqlite registers with database/sql.
const driverName = "sqlite"

// Store is a store.Store, and this is where that is checked rather than
// assumed. It is what makes "the same interface" in issue #27 a fact the
// compiler holds rather than a claim in a body.
var _ store.Store = (*Store)(nil)

// Store is a store held in one SQLite file.
type Store struct {
	db *sql.DB
}

// Open opens or creates the database at path and brings its schema up to date.
//
// It refuses a database from a later build rather than opening it, which is
// store.ErrNewerSchema and is the third condition on issue #27.
func Open(ctx context.Context, path string) (*Store, error) {
	return open(ctx, driverName, path)
}

// open is Open with the driver named, so the suite can reach the branch where
// database/sql does not know it. Open's own driver name is a constant, so that
// branch is otherwise unreachable and a reader would have to take it on trust.
func open(ctx context.Context, driver, path string) (*Store, error) {
	if path == "" {
		return nil, fmt.Errorf("%w: a store needs a path, and an empty one would be a database that vanishes", store.ErrInvalid)
	}

	db, err := sql.Open(driver, path)
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", path, err)
	}

	// sql.Open does not touch the file. The ping is what turns an
	// unreachable path into an error here rather than into a failure at the
	// first query somebody makes half an hour later.
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("reaching %s: %w", path, err)
	}

	if err := migrate(ctx, db, migrations); err != nil {
		_ = db.Close()
		return nil, err
	}

	return &Store{db: db}, nil
}

// Close closes the database. Closing twice is not an error.
func (s *Store) Close() error { return s.db.Close() }

// PutChannel writes a channel, replacing one with the same identifier.
func (s *Store) PutChannel(ctx context.Context, c *orchestration.Channel) error {
	if c == nil {
		return fmt.Errorf("%w: no channel", store.ErrInvalid)
	}
	const write = `INSERT INTO channel (id, space, name, always_on) VALUES (?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET space = excluded.space, name = excluded.name, always_on = excluded.always_on`
	if _, err := s.db.ExecContext(ctx, write, c.ID().String(), c.Space().String(), c.Name(), boolToInt(c.AlwaysOn())); err != nil {
		return fmt.Errorf("writing channel %s: %w", c.ID(), err)
	}
	return nil
}

// Channel reads a channel back.
func (s *Store) Channel(ctx context.Context, id orchestration.ID) (*orchestration.Channel, error) {
	const read = `SELECT space, name, always_on FROM channel WHERE id = ?`
	var space identifier
	var name string
	var alwaysOn int
	switch err := s.db.QueryRowContext(ctx, read, id.String()).Scan(&space, &name, &alwaysOn); {
	case errors.Is(err, sql.ErrNoRows):
		return nil, fmt.Errorf("%w: no channel %s", store.ErrNotFound, id)
	case err != nil:
		return nil, fmt.Errorf("reading channel %s: %w", id, err)
	}
	return buildChannel(id, space.ID, name, alwaysOn)
}

// ChannelsInSpace returns the channels of one space in identifier order.
func (s *Store) ChannelsInSpace(ctx context.Context, space orchestration.ID) ([]*orchestration.Channel, error) {
	const read = `SELECT id, name, always_on FROM channel WHERE space = ? ORDER BY id`
	var id identifier
	var name string
	var alwaysOn int
	var found []*orchestration.Channel

	err := s.eachRow(ctx, read, []any{space.String()}, []any{&id, &name, &alwaysOn}, func() error {
		c, err := buildChannel(id.ID, space, name, alwaysOn)
		if err != nil {
			return err
		}
		found = append(found, c)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("reading the channels of %s: %w", space, err)
	}
	return found, nil
}

// PutMember records a member of a space.
func (s *Store) PutMember(ctx context.Context, space, member orchestration.ID) error {
	if space.IsZero() || member.IsZero() {
		return fmt.Errorf("%w: a membership needs two identifiers and one of them is the zero value", store.ErrInvalid)
	}
	const write = `INSERT INTO membership (space, member) VALUES (?, ?) ON CONFLICT DO NOTHING`
	if _, err := s.db.ExecContext(ctx, write, space.String(), member.String()); err != nil {
		return fmt.Errorf("recording %s as a member of %s: %w", member, space, err)
	}
	return nil
}

// Members returns the members of a space in identifier order.
func (s *Store) Members(ctx context.Context, space orchestration.ID) ([]orchestration.ID, error) {
	const read = `SELECT member FROM membership WHERE space = ? ORDER BY member`
	var member identifier
	var found []orchestration.ID

	err := s.eachRow(ctx, read, []any{space.String()}, []any{&member}, func() error {
		found = append(found, member.ID)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("reading the members of %s: %w", space, err)
	}
	return found, nil
}

// Grant adds permissions for a principal on a subject.
func (s *Store) Grant(ctx context.Context, principal orchestration.ID, subject orchestration.Subject, granted ...orchestration.Permission) error {
	if principal.IsZero() {
		return fmt.Errorf("%w: a grant needs a principal", store.ErrInvalid)
	}
	if subject.Space().IsZero() {
		return fmt.Errorf("%w: a grant needs a subject and this one names no space", store.ErrInvalid)
	}
	for _, p := range granted {
		if _, err := store.ParsePermission(p.String()); err != nil {
			return err
		}
	}

	const write = `INSERT INTO permission_grant (principal, space, channel, permission) VALUES (?, ?, ?, ?) ON CONFLICT DO NOTHING`
	for _, p := range granted {
		if _, err := s.db.ExecContext(ctx, write, principal.String(), subject.Space().String(), subjectChannel(subject), p.String()); err != nil {
			return fmt.Errorf("granting %s to %s: %w", p, principal, err)
		}
	}
	return nil
}

// Granted answers what a principal holds on a subject.
//
// It returns the empty set where the read fails, which is the port's own
// contract: orchestration.Grantor says "I do not know" and "nothing is granted"
// have to be the same answer at a permission boundary, so a lookup failure
// refuses rather than reaching a caller who might read it as a reason to carry
// on. That direction is safe and it is also silent, and it stays silent until
// there is a logging surface to say so, which is issue #67.
//
// A stored name the model does not declare is skipped and the rest of the row
// set still answers. Dropping every grant on one unrecognised row would let a
// permission removed from the model lock a principal out of everything, and
// returning the unrecognised one would be a grant nothing can name or revoke.
func (s *Store) Granted(p orchestration.Principal, subject orchestration.Subject) orchestration.PermissionSet {
	const read = `SELECT permission FROM permission_grant WHERE principal = ? AND space = ? AND channel = ?`
	args := []any{p.ID().String(), subject.Space().String(), subjectChannel(subject)}

	var name string
	var held []orchestration.Permission

	err := s.eachRow(context.Background(), read, args, []any{&name}, func() error {
		permission, err := store.ParsePermission(name)
		if err != nil {
			return nil
		}
		held = append(held, permission)
		return nil
	})
	if err != nil {
		return orchestration.NewPermissionSet()
	}
	return orchestration.NewPermissionSet(held...)
}

// SetGain records a listener's setting for a speaker.
func (s *Store) SetGain(ctx context.Context, listener, speaker orchestration.ID, percent int) error {
	if listener.IsZero() || speaker.IsZero() {
		return fmt.Errorf("%w: a gain setting needs two identifiers and one of them is the zero value", store.ErrInvalid)
	}
	if percent < store.MinGain || percent > store.MaxGain {
		return fmt.Errorf("%w: a gain is %d to %d percent of unity and %d is outside that", store.ErrInvalid, store.MinGain, store.MaxGain, percent)
	}

	if percent == store.DefaultGain {
		const remove = `DELETE FROM gain WHERE listener = ? AND speaker = ?`
		if _, err := s.db.ExecContext(ctx, remove, listener.String(), speaker.String()); err != nil {
			return fmt.Errorf("clearing the gain %s holds for %s: %w", listener, speaker, err)
		}
		return nil
	}

	const write = `INSERT INTO gain (listener, speaker, percent) VALUES (?, ?, ?)
		ON CONFLICT (listener, speaker) DO UPDATE SET percent = excluded.percent`
	if _, err := s.db.ExecContext(ctx, write, listener.String(), speaker.String(), percent); err != nil {
		return fmt.Errorf("writing the gain %s holds for %s: %w", listener, speaker, err)
	}
	return nil
}

// Gain returns the setting, or store.DefaultGain where there is no row.
func (s *Store) Gain(ctx context.Context, listener, speaker orchestration.ID) (int, error) {
	const read = `SELECT percent FROM gain WHERE listener = ? AND speaker = ?`
	var percent int
	switch err := s.db.QueryRowContext(ctx, read, listener.String(), speaker.String()).Scan(&percent); {
	case errors.Is(err, sql.ErrNoRows):
		return store.DefaultGain, nil
	case err != nil:
		return 0, fmt.Errorf("reading the gain %s holds for %s: %w", listener, speaker, err)
	}
	return percent, nil
}

// eachRow runs a query, scans every row into dest, and calls row once per row.
//
// Every multi-row read goes through it, so the scan, the closing of the rows
// and the check of the error the iteration ended with are written once each. A
// read that forgot the last one would report a truncated result as a complete
// one, which is the failure nothing downstream can detect.
func (s *Store) eachRow(ctx context.Context, query string, args, dest []any, row func() error) error {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		if err := rows.Scan(dest...); err != nil {
			return err
		}
		if err := row(); err != nil {
			return err
		}
	}
	return rows.Err()
}

// identifier reads a stored local@host through the domain's own parse.
//
// It is a sql.Scanner rather than a string the caller parses afterwards so that
// a row carrying something that is not an identifier fails as a scan, at the
// row, naming the column's value. A store that read it into a string and parsed
// it later would have to remember to, once per read.
type identifier struct {
	orchestration.ID
}

// Scan parses the stored text.
func (i *identifier) Scan(src any) error {
	text, isText := src.(string)
	if !isText {
		return fmt.Errorf("%w: an identifier column came back as %T rather than as text", store.ErrInvalid, src)
	}
	parsed, err := orchestration.ParseID(text)
	if err != nil {
		return fmt.Errorf("%w: %s", store.ErrInvalid, err)
	}
	i.ID = parsed
	return nil
}

// buildChannel turns a row into a domain value through the domain's own
// constructor, so a row that cannot be a channel is an error here rather than
// an invalid Channel handed to a caller.
func buildChannel(id, space orchestration.ID, name string, alwaysOn int) (*orchestration.Channel, error) {
	c, err := orchestration.NewChannel(id, space, name, alwaysOn != 0)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", store.ErrInvalid, err)
	}
	return c, nil
}

// subjectChannel writes a subject's channel, or the empty string where it is
// about the space itself. The empty string is not a valid identifier, so the
// two cases cannot collide in the column.
func subjectChannel(s orchestration.Subject) string {
	channel, aboutChannel := s.Channel()
	if !aboutChannel {
		return ""
	}
	return channel.String()
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
