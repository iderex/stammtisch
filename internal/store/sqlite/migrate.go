// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026  iderex

package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/iderex/stammtisch/internal/store"
)

// A migration is one numbered step and the statements that take the schema from
// the step before it to this one.
//
// The version is where it is written down. SQLite carries a four-byte integer
// in the database header for exactly this, readable and writable as
// `PRAGMA user_version`, so the schema version travels inside the file rather
// than in a table the first migration would have to create before it could
// record that it had run.
type migration struct {
	version    int
	name       string
	statements []string
}

// migrations is the whole history, oldest first. It is append-only: a landed
// migration is never edited, because a tree that edits one produces two
// databases at the same version with different schemas and no way to tell them
// apart afterwards.
var migrations = []migration{
	{
		version: 1,
		name:    "the durable half of the domain",
		statements: []string{
			// Every table is STRICT, so a column declared TEXT
			// refuses an integer instead of quietly storing one.
			// SQLite's default is the opposite and it is the
			// reason a store like this one grows rows nothing can
			// scan back.
			//
			// Identifiers are stored as local@host, which is what
			// orchestration.ID.String writes and what
			// orchestration.ParseID reads. The host is carried per
			// docs/decisions/federation.md even though this
			// release does not federate: an identifier that grows
			// a host component later is a migration of every row
			// that holds one.
			`CREATE TABLE channel (
				id        TEXT    NOT NULL PRIMARY KEY,
				space     TEXT    NOT NULL,
				name      TEXT    NOT NULL,
				always_on INTEGER NOT NULL CHECK (always_on IN (0, 1))
			) STRICT`,
			`CREATE INDEX channel_by_space ON channel (space)`,

			`CREATE TABLE membership (
				space  TEXT NOT NULL,
				member TEXT NOT NULL,
				PRIMARY KEY (space, member)
			) STRICT`,

			// channel is the empty string where the subject is the
			// space itself. The empty string is not a valid
			// identifier, so a space subject and a channel subject
			// cannot collide, and the column stays NOT NULL rather
			// than growing a nullable case every read has to
			// handle.
			`CREATE TABLE permission_grant (
				principal  TEXT NOT NULL,
				space      TEXT NOT NULL,
				channel    TEXT NOT NULL,
				permission TEXT NOT NULL,
				PRIMARY KEY (principal, space, channel, permission)
			) STRICT`,

			// Percent of unity amplitude, per
			// docs/decisions/per-person-volume.md. The CHECK is the
			// record's range, refused by the store as well as by
			// the port, because a row written by anything but this
			// code is still a row this code will read back.
			// A setting equal to the default is not stored, which
			// is why there is no default column and no row for a
			// listener who has never touched the control.
			`CREATE TABLE gain (
				listener TEXT    NOT NULL,
				speaker  TEXT    NOT NULL,
				percent  INTEGER NOT NULL CHECK (percent BETWEEN 0 AND 200),
				PRIMARY KEY (listener, speaker)
			) STRICT`,
		},
	},
}

// migrate brings a database up to the last version in the list.
//
// It refuses a database written by a later build rather than opening it. The
// alternative is the failure this whole mechanism exists against: an older
// binary writing rows a newer schema's constraints were meant to refuse, with
// the corruption found later by the newer binary and no way back.
//
// The list is a parameter rather than the package variable so the suite can
// hand it a faulty one and watch the refusal happen. A migration mechanism
// whose refusals are only reachable by editing the real history is one nobody
// can prove.
func migrate(ctx context.Context, db *sql.DB, list []migration) error {
	if err := checkList(list); err != nil {
		return err
	}

	var current int
	if err := db.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&current); err != nil {
		return fmt.Errorf("reading the schema version: %w", err)
	}

	latest := list[len(list)-1].version
	if current > latest {
		return fmt.Errorf("%w: the data is at schema %d and this build knows %d", store.ErrNewerSchema, current, latest)
	}

	for _, m := range list {
		if m.version <= current {
			continue
		}
		if err := apply(ctx, db, m); err != nil {
			return fmt.Errorf("applying schema %d, %s: %w", m.version, m.name, err)
		}
	}
	return nil
}

// apply runs one migration and its version stamp in a single transaction, so a
// half-applied step cannot be left behind by a statement that fails in the
// middle of one.
//
// The stamp is the last statement rather than a separate write for the same
// reason: a transaction that applied the statements and then failed to record
// the version would leave a database whose schema and whose recorded version
// disagree, which is the state no later run can reason about.
func apply(ctx context.Context, db *sql.DB, m migration) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	statements := make([]string, 0, len(m.statements)+1)
	statements = append(statements, m.statements...)
	// The version is an int from the list above and never from input, so
	// there is nothing here for a parameter to protect. PRAGMA takes no
	// bound parameter, which is why this is formatted rather than bound.
	statements = append(statements, fmt.Sprintf(`PRAGMA user_version = %d`, m.version))

	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("%w: %s", err, firstLine(statement))
		}
	}
	return tx.Commit()
}

// checkList refuses a history that is not one, two, three and so on from the
// start.
//
// A gap or a repeat means a version somewhere is applied twice or never, and
// the failure surfaces as a schema that does not match its stamp on somebody
// else's machine. Refusing at the top of migrate makes it a failure of every
// run instead.
func checkList(list []migration) error {
	if len(list) == 0 {
		return fmt.Errorf("%w: there are no migrations, so there is no schema to reach", store.ErrInvalid)
	}
	for i, m := range list {
		if m.version != i+1 {
			return fmt.Errorf("%w: migration %d of the list is numbered %d, and the history has to run from 1 without a gap", store.ErrInvalid, i+1, m.version)
		}
		if len(m.statements) == 0 {
			return fmt.Errorf("%w: migration %d carries no statements", store.ErrInvalid, m.version)
		}
	}
	return nil
}

// firstLine trims a statement to something a failure message can carry without
// pasting a whole table definition into it.
func firstLine(statement string) string {
	for i, r := range statement {
		if r == '\n' {
			return statement[:i]
		}
	}
	return statement
}
