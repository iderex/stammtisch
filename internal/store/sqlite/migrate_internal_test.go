// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026  iderex

package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iderex/stammtisch/internal/store"
)

// This file is inside the package on purpose. Everything below is a refusal
// that cannot be reached from outside: a driver name Open never passes, a
// migration history the tree does not carry, and a scan of a value no column
// holds. A guard nobody can reach is a guard nobody has proved.

func tempDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open(driverName, filepath.Join(t.TempDir(), "stammtisch.db"))
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestOpenReportsADriverDatabaseSQLDoesNotKnow(t *testing.T) {
	_, err := open(context.Background(), "no-such-driver", filepath.Join(t.TempDir(), "stammtisch.db"))
	if err == nil {
		t.Fatal("opening with an unregistered driver returned no error")
	}
	if !strings.Contains(err.Error(), "no-such-driver") {
		t.Errorf("the error does not name the driver: %v", err)
	}
}

// TestTheMigrationHistoryIsContiguousFromOne judges the list the tree actually
// carries. A history with a gap applies a version twice or never, and the
// damage shows up on somebody else's machine.
func TestTheMigrationHistoryIsContiguousFromOne(t *testing.T) {
	if err := checkList(migrations); err != nil {
		t.Fatalf("the migration history in this package is refused by its own check: %v", err)
	}
}

func TestCheckListRefusesAHistoryThatIsNotOne(t *testing.T) {
	for _, faulty := range []struct {
		why  string
		list []migration
	}{
		{"no migrations at all", nil},
		{"starting at two", []migration{{version: 2, name: "late", statements: []string{"SELECT 1"}}}},
		{"a gap in the middle", []migration{
			{version: 1, name: "first", statements: []string{"SELECT 1"}},
			{version: 3, name: "third", statements: []string{"SELECT 1"}},
		}},
		{"a repeated version", []migration{
			{version: 1, name: "first", statements: []string{"SELECT 1"}},
			{version: 1, name: "first again", statements: []string{"SELECT 1"}},
		}},
		{"a migration with nothing in it", []migration{{version: 1, name: "empty"}}},
	} {
		if err := checkList(faulty.list); !errors.Is(err, store.ErrInvalid) {
			t.Errorf("%s: error = %v, want store.ErrInvalid", faulty.why, err)
		}
	}
}

// TestMigrateRefusesAFaultyHistoryBeforeItTouchesTheDatabase is the check above
// reached through the function that runs it. The database here is closed, so a
// run that reached the first query would fail for that reason instead, and the
// assertion on store.ErrInvalid is what tells the two apart.
func TestMigrateRefusesAFaultyHistoryBeforeItTouchesTheDatabase(t *testing.T) {
	db := tempDB(t)
	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := migrate(context.Background(), db, nil); !errors.Is(err, store.ErrInvalid) {
		t.Errorf("migrate with no history: error = %v, want store.ErrInvalid", err)
	}
}

// TestMigrateStopsOnAStatementThatFails is the proof that a failing migration
// is a failure rather than a partial schema. The transaction is what makes the
// second half true, and the assertion after the refusal is what proves it.
func TestMigrateStopsOnAStatementThatFails(t *testing.T) {
	ctx := context.Background()
	db := tempDB(t)

	faulty := []migration{{
		version: 1,
		name:    "one good statement and one that is not sql",
		statements: []string{
			"CREATE TABLE landed (x TEXT NOT NULL) STRICT",
			"THIS IS NOT SQL",
		},
	}}
	err := migrate(ctx, db, faulty)
	if err == nil {
		t.Fatal("a migration carrying a statement that is not SQL was applied")
	}
	if !strings.Contains(err.Error(), "THIS IS NOT SQL") {
		t.Errorf("the error does not name the statement that failed: %v", err)
	}

	var landed int
	if err := db.QueryRowContext(ctx,
		`SELECT count(*) FROM sqlite_schema WHERE name = 'landed'`).Scan(&landed); err != nil {
		t.Fatalf("reading sqlite_schema: %v", err)
	}
	if landed != 0 {
		t.Error("the table created by the statement before the failure is still there")
	}
	var version int
	if err := db.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatalf("reading the schema version: %v", err)
	}
	if version != 0 {
		t.Errorf("the failed migration stamped the version anyway: %d", version)
	}
}

// TestAFailingStatementIsReportedByItsFirstLine keeps a table definition out of
// a one-line failure message. The statement above is a single line and this one
// is not, which is the other arm.
func TestAFailingStatementIsReportedByItsFirstLine(t *testing.T) {
	db := tempDB(t)
	faulty := []migration{{
		version:    1,
		name:       "a statement that spans lines",
		statements: []string{"CREATE TABLE broken (\n\tx NOT A TYPE\n)"},
	}}

	err := migrate(context.Background(), db, faulty)
	if err == nil {
		t.Fatal("a migration carrying a statement that is not SQL was applied")
	}
	if strings.Contains(err.Error(), "NOT A TYPE") {
		t.Errorf("the whole statement reached the message rather than its first line: %v", err)
	}
	if !strings.Contains(err.Error(), "CREATE TABLE broken (") {
		t.Errorf("the message does not carry the statement's first line: %v", err)
	}
}

// TestMigrateReportsADatabaseItCannotRead covers the two places a migration run
// depends on the handle working: reading the version it starts from, and
// opening the transaction it applies in.
func TestMigrateReportsADatabaseItCannotRead(t *testing.T) {
	ctx := context.Background()
	db := tempDB(t)
	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if err := migrate(ctx, db, migrations); err == nil {
		t.Error("migrate against a closed database returned no error")
	}
	if err := apply(ctx, db, migrations[0]); err == nil {
		t.Error("apply against a closed database returned no error")
	}
}

// TestAnIdentifierColumnThatIsNotTextIsRefused is the arm of the scan no column
// in this schema can produce today. Every identifier column is TEXT under a
// STRICT table, so the only way to reach it is to call the scanner, and a
// scanner that trusted its input would be the thing that broke on the day a
// column stopped being TEXT.
func TestAnIdentifierColumnThatIsNotTextIsRefused(t *testing.T) {
	var read identifier
	if err := read.Scan(int64(42)); !errors.Is(err, store.ErrInvalid) {
		t.Errorf("Scan of an integer: error = %v, want store.ErrInvalid", err)
	}
	if err := read.Scan("ada@example.test"); err != nil {
		t.Fatalf("Scan of a valid identifier: %v", err)
	}
	if read.ID.Local() != "ada" || read.ID.Host() != "example.test" {
		t.Errorf("Scan read %s", read.ID)
	}
}
