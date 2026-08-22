// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026  Nils Lehnen

package sqlite_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/iderex/stammtisch/internal/orchestration"
	"github.com/iderex/stammtisch/internal/store"
	"github.com/iderex/stammtisch/internal/store/sqlite"
)

// latestSchema is the version the migration history reaches. It is written here
// rather than read from the package so that a migration added without a
// thought for this suite reds it, which is the direction a test should fail in.
const latestSchema = 1

func id(t *testing.T, local string) orchestration.ID {
	t.Helper()
	made, err := orchestration.NewID(local, "example.test")
	if err != nil {
		t.Fatalf("NewID(%q): %v", local, err)
	}
	return made
}

func channel(t *testing.T, name string, space orchestration.ID, alwaysOn bool) *orchestration.Channel {
	t.Helper()
	c, err := orchestration.NewChannel(id(t, name), space, name, alwaysOn)
	if err != nil {
		t.Fatalf("NewChannel(%q): %v", name, err)
	}
	return c
}

// openTemp returns a store on a fresh file in the test's own directory, and the
// path, so a test that wants to look at the file without going through the
// store can.
func openTemp(t *testing.T) (*sqlite.Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stammtisch.db")
	s, err := sqlite.Open(context.Background(), path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s, path
}

// raw opens a second connection to the same file, so a test can plant a row the
// store would refuse to write and then ask the store to read it.
func raw(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("opening %s directly: %v", path, err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func mustExec(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.ExecContext(context.Background(), query, args...); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
}

// ---------------------------------------------------------------------------
// The migration mechanism
// ---------------------------------------------------------------------------

// TestEveryMigrationAppliedFromEmptyProducesTheSchema is the second condition on
// issue #27. It opens a database that does not exist, so every migration in the
// history runs, and then asserts the schema that came out rather than that the
// call returned no error.
//
// The assertion is on the columns the schema constrains rather than on the text
// of the CREATE statements. A comparison against the statement text passes and
// fails for reformatting, which is the wrong thing to be sensitive to; a
// comparison against table_info is a comparison against what the database will
// actually refuse.
func TestEveryMigrationAppliedFromEmptyProducesTheSchema(t *testing.T) {
	_, path := openTemp(t)
	db := raw(t, path)

	wantObjects := []string{
		"index channel_by_space",
		"table channel",
		"table gain",
		"table membership",
		"table permission_grant",
	}
	if got := objects(t, db); !equalStrings(got, wantObjects) {
		t.Errorf("the schema holds\n  %v\nand should hold\n  %v", got, wantObjects)
	}

	wantColumns := map[string][]string{
		"channel": {
			"id TEXT notnull pk1",
			"space TEXT notnull",
			"name TEXT notnull",
			"always_on INTEGER notnull",
		},
		"membership": {
			"space TEXT notnull pk1",
			"member TEXT notnull pk2",
		},
		"permission_grant": {
			"principal TEXT notnull pk1",
			"space TEXT notnull pk2",
			"channel TEXT notnull pk3",
			"permission TEXT notnull pk4",
		},
		"gain": {
			"listener TEXT notnull pk1",
			"speaker TEXT notnull pk2",
			"percent INTEGER notnull",
		},
	}
	for table, want := range wantColumns {
		if got := columns(t, db, table); !equalStrings(got, want) {
			t.Errorf("%s holds\n  %v\nand should hold\n  %v", table, got, want)
		}
	}

	if got := userVersion(t, db); got != latestSchema {
		t.Errorf("the applied schema version is %d, want %d", got, latestSchema)
	}
}

// TestTheTablesAreStrict holds the reason a column declared TEXT means TEXT.
// Without STRICT, SQLite stores whatever it is handed, and the first row nobody
// can scan back is written years before it is read.
func TestTheTablesAreStrict(t *testing.T) {
	_, path := openTemp(t)
	db := raw(t, path)

	_, err := db.ExecContext(context.Background(),
		`INSERT INTO channel (id, space, name, always_on) VALUES (?, ?, ?, ?)`,
		"general@example.test", "space@example.test", "general", "yes")
	if err == nil {
		t.Fatal("a channel with a text always_on was accepted, so the table is not STRICT")
	}
}

// TestReopeningAnUpToDateStoreAppliesNothing walks the branch that skips a
// migration already applied. A mechanism that reran them would fail on the
// second start of every service that ever used it.
func TestReopeningAnUpToDateStoreAppliesNothing(t *testing.T) {
	ctx := context.Background()
	first, path := openTemp(t)
	space := id(t, "space")
	if err := first.PutChannel(ctx, channel(t, "general", space, true)); err != nil {
		t.Fatalf("PutChannel: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	second, err := sqlite.Open(ctx, path)
	if err != nil {
		t.Fatalf("reopening: %v", err)
	}
	defer func() { _ = second.Close() }()

	got, err := second.Channel(ctx, id(t, "general"))
	if err != nil {
		t.Fatalf("Channel after reopening: %v", err)
	}
	if !got.AlwaysOn() {
		t.Error("the channel came back with always_on false, and it was written true")
	}
}

// TestADowngradeIsRefusedRatherThanSilentlyCorrupting is the third condition on
// issue #27.
//
// The refusal has to be both halves: the open fails, and the data is still
// there afterwards. An implementation that refused by truncating, or that
// carried on and wrote a row the newer schema would have refused, would pass a
// test asserting only the error.
func TestADowngradeIsRefusedRatherThanSilentlyCorrupting(t *testing.T) {
	ctx := context.Background()
	first, path := openTemp(t)
	space := id(t, "space")
	if err := first.PutChannel(ctx, channel(t, "general", space, false)); err != nil {
		t.Fatalf("PutChannel: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// A later build wrote this file and stamped it with a schema this build
	// has never heard of.
	db := raw(t, path)
	mustExec(t, db, fmt.Sprintf(`PRAGMA user_version = %d`, latestSchema+1))

	reopened, err := sqlite.Open(ctx, path)
	if !errors.Is(err, store.ErrNewerSchema) {
		t.Fatalf("Open against a newer schema: error = %v, want store.ErrNewerSchema", err)
	}
	if reopened != nil {
		t.Fatal("Open returned a store as well as the refusal")
	}

	if got := userVersion(t, db); got != latestSchema+1 {
		t.Errorf("the refused open moved the schema version to %d", got)
	}
	var held int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM channel`).Scan(&held); err != nil {
		t.Fatalf("counting channels after the refusal: %v", err)
	}
	if held != 1 {
		t.Errorf("the refused open left %d channels, want 1", held)
	}
}

func TestOpenRefusesAPathItCannotReach(t *testing.T) {
	ctx := context.Background()

	if _, err := sqlite.Open(ctx, ""); !errors.Is(err, store.ErrInvalid) {
		t.Errorf(`Open(""): error = %v, want store.ErrInvalid`, err)
	}

	absent := filepath.Join(t.TempDir(), "no-such-directory", "stammtisch.db")
	if _, err := sqlite.Open(ctx, absent); err == nil {
		t.Error("Open in a directory that does not exist returned no error")
	}
}

// ---------------------------------------------------------------------------
// The contract, run against both implementations
// ---------------------------------------------------------------------------

// TestBothImplementationsAnswerTheSame is what "the same interface" in issue
// #27 means in practice. One table of operations, run against the in-memory
// twin and against the durable store, with the same assertions.
//
// It is here rather than in internal/store because this is the package that can
// import both. The in-memory twin exists so the orchestration suite can run
// without a driver, and a twin that answered differently would make that suite
// prove something about a store nobody ships.
func TestBothImplementationsAnswerTheSame(t *testing.T) {
	durable, _ := openTemp(t)
	for _, subject := range []struct {
		name string
		s    store.Store
	}{
		{"memory", store.NewMemory()},
		{"sqlite", durable},
	} {
		t.Run(subject.name, func(t *testing.T) {
			exerciseTheContract(t, subject.s)
		})
	}
}

func exerciseTheContract(t *testing.T, s store.Store) {
	t.Helper()
	ctx := context.Background()
	space := id(t, "space")
	general, lounge := channel(t, "general", space, false), channel(t, "lounge", space, true)
	ada, ben := id(t, "ada"), id(t, "ben")

	for _, c := range []*orchestration.Channel{lounge, general} {
		if err := s.PutChannel(ctx, c); err != nil {
			t.Fatalf("PutChannel(%s): %v", c.Name(), err)
		}
	}
	// Writing the same identifier again replaces rather than duplicating.
	if err := s.PutChannel(ctx, channel(t, "general", space, true)); err != nil {
		t.Fatalf("PutChannel replacing general: %v", err)
	}

	held, err := s.ChannelsInSpace(ctx, space)
	if err != nil {
		t.Fatalf("ChannelsInSpace: %v", err)
	}
	if len(held) != 2 || held[0].Name() != "general" || held[1].Name() != "lounge" {
		t.Fatalf("ChannelsInSpace returned %d channels, in the order %v", len(held), nameOrder(held))
	}
	if !held[0].AlwaysOn() {
		t.Error("the replacing write did not take: general is not always-on")
	}
	if _, err := s.Channel(ctx, id(t, "absent")); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("Channel(absent): error = %v, want store.ErrNotFound", err)
	}
	if none, err := s.ChannelsInSpace(ctx, id(t, "elsewhere")); err != nil || len(none) != 0 {
		t.Errorf("ChannelsInSpace of an unknown space = %d channels, %v", len(none), err)
	}

	for _, member := range []orchestration.ID{ben, ada, ben} {
		if err := s.PutMember(ctx, space, member); err != nil {
			t.Fatalf("PutMember(%s): %v", member, err)
		}
	}
	members, err := s.Members(ctx, space)
	if err != nil {
		t.Fatalf("Members: %v", err)
	}
	if len(members) != 2 || members[0] != ada || members[1] != ben {
		t.Fatalf("Members returned %v, want ada then ben", members)
	}

	subject := orchestration.ChannelSubject(space, general.ID())
	if err := s.Grant(ctx, ada, subject, orchestration.SeeChannel, orchestration.SpeakInChannel); err != nil {
		t.Fatalf("Grant: %v", err)
	}
	// Granting twice is not an error and does not double anything.
	if err := s.Grant(ctx, ada, subject, orchestration.SeeChannel); err != nil {
		t.Fatalf("Grant again: %v", err)
	}
	if !orchestration.Allow(s, orchestration.Person(ada), orchestration.SpeakInChannel, subject) {
		t.Error("speak-in-channel was refused after being granted with see-channel")
	}
	if orchestration.Allow(s, orchestration.Person(ben), orchestration.SeeChannel, subject) {
		t.Error("ben was allowed see-channel and holds no grant")
	}
	if orchestration.Allow(s, orchestration.Person(ada), orchestration.ManageSpace, orchestration.SpaceSubject(space)) {
		t.Error("a grant on a channel reached the space")
	}
	if err := s.Grant(ctx, ada, subject, orchestration.Permission(99)); !errors.Is(err, store.ErrInvalid) {
		t.Errorf("Grant of an undeclared permission: error = %v, want store.ErrInvalid", err)
	}

	if percent, err := s.Gain(ctx, ada, ben); err != nil || percent != store.DefaultGain {
		t.Errorf("an untouched control read %d, %v", percent, err)
	}
	if err := s.SetGain(ctx, ada, ben, 40); err != nil {
		t.Fatalf("SetGain(40): %v", err)
	}
	if percent, err := s.Gain(ctx, ada, ben); err != nil || percent != 40 {
		t.Errorf("Gain after SetGain(40) = %d, %v", percent, err)
	}
	if err := s.SetGain(ctx, ada, ben, store.MaxGain); err != nil {
		t.Fatalf("SetGain(200): %v", err)
	}
	if percent, err := s.Gain(ctx, ada, ben); err != nil || percent != store.MaxGain {
		t.Errorf("Gain after SetGain(200) = %d, %v", percent, err)
	}
	if err := s.SetGain(ctx, ada, ben, store.DefaultGain); err != nil {
		t.Fatalf("SetGain(100): %v", err)
	}
	if percent, err := s.Gain(ctx, ada, ben); err != nil || percent != store.DefaultGain {
		t.Errorf("Gain after SetGain(100) = %d, %v", percent, err)
	}
	for _, refused := range []int{store.MinGain - 1, store.MaxGain + 1} {
		if err := s.SetGain(ctx, ada, ben, refused); !errors.Is(err, store.ErrInvalid) {
			t.Errorf("SetGain(%d): error = %v, want store.ErrInvalid", refused, err)
		}
	}

	if err := s.PutChannel(ctx, nil); !errors.Is(err, store.ErrInvalid) {
		t.Errorf("PutChannel(nil): error = %v, want store.ErrInvalid", err)
	}
	if err := s.PutMember(ctx, orchestration.ID{}, ada); !errors.Is(err, store.ErrInvalid) {
		t.Errorf("PutMember with no space: error = %v, want store.ErrInvalid", err)
	}
	if err := s.PutMember(ctx, space, orchestration.ID{}); !errors.Is(err, store.ErrInvalid) {
		t.Errorf("PutMember with no member: error = %v, want store.ErrInvalid", err)
	}
	if err := s.SetGain(ctx, orchestration.ID{}, ben, 50); !errors.Is(err, store.ErrInvalid) {
		t.Errorf("SetGain with no listener: error = %v, want store.ErrInvalid", err)
	}
	if err := s.SetGain(ctx, ada, orchestration.ID{}, 50); !errors.Is(err, store.ErrInvalid) {
		t.Errorf("SetGain with no speaker: error = %v, want store.ErrInvalid", err)
	}
	if err := s.Grant(ctx, orchestration.ID{}, subject, orchestration.SeeChannel); !errors.Is(err, store.ErrInvalid) {
		t.Errorf("Grant with no principal: error = %v, want store.ErrInvalid", err)
	}
	if err := s.Grant(ctx, ada, orchestration.Subject{}, orchestration.SeeChannel); !errors.Is(err, store.ErrInvalid) {
		t.Errorf("Grant with no subject: error = %v, want store.ErrInvalid", err)
	}
}

func nameOrder(channels []*orchestration.Channel) []string {
	var got []string
	for _, c := range channels {
		got = append(got, c.Name())
	}
	return got
}

// TestASpaceSubjectAndAChannelSubjectDoNotCollide holds the reason the channel
// column is the empty string rather than null for a space subject. If the two
// wrote the same row, a grant on the space would answer a question about a
// channel.
func TestASpaceSubjectAndAChannelSubjectDoNotCollide(t *testing.T) {
	ctx := context.Background()
	s, _ := openTemp(t)
	space, general := id(t, "space"), id(t, "general")
	ada := orchestration.Person(id(t, "ada"))

	if err := s.Grant(ctx, ada.ID(), orchestration.SpaceSubject(space), orchestration.ManageSpace); err != nil {
		t.Fatalf("Grant on the space: %v", err)
	}
	if err := s.Grant(ctx, ada.ID(), orchestration.ChannelSubject(space, general), orchestration.SeeChannel); err != nil {
		t.Fatalf("Grant on the channel: %v", err)
	}

	if !orchestration.Allow(s, ada, orchestration.ManageSpace, orchestration.SpaceSubject(space)) {
		t.Error("manage-space was refused on the space it was granted on")
	}
	if !orchestration.Allow(s, ada, orchestration.SeeChannel, orchestration.ChannelSubject(space, general)) {
		t.Error("see-channel was refused on the channel it was granted on")
	}
	if orchestration.Allow(s, ada, orchestration.ManageSpace, orchestration.ChannelSubject(space, general)) {
		t.Error("a space grant answered a question about a channel")
	}
}

// ---------------------------------------------------------------------------
// Rows this store would not have written
// ---------------------------------------------------------------------------

// TestARowThatCannotBeADomainValueIsAnErrorAndNotAValue plants rows the store's
// own writes cannot produce and asks it to read them.
//
// This is the case a store passes by accident. Nothing here writes a channel
// with no name or an identifier with no host, so a reader that trusted the row
// would look correct forever and hand out a domain value the domain's own
// constructor refuses.
func TestARowThatCannotBeADomainValueIsAnErrorAndNotAValue(t *testing.T) {
	ctx := context.Background()
	s, path := openTemp(t)
	db := raw(t, path)
	const write = `INSERT INTO channel (id, space, name, always_on) VALUES (?, ?, ?, 0)`

	mustExec(t, db, write, "no-host", "space@example.test", "broken")
	if _, err := s.ChannelsInSpace(ctx, id(t, "space")); !errors.Is(err, store.ErrInvalid) {
		t.Errorf("a channel row with an unparseable identifier: error = %v, want store.ErrInvalid", err)
	}
	mustExec(t, db, `DELETE FROM channel`)

	mustExec(t, db, write, "general@example.test", "also-no-host", "general")
	if _, err := s.Channel(ctx, id(t, "general")); !errors.Is(err, store.ErrInvalid) {
		t.Errorf("a channel row with an unparseable space: error = %v, want store.ErrInvalid", err)
	}
	mustExec(t, db, `DELETE FROM channel`)

	mustExec(t, db, write, "general@example.test", "space@example.test", "")
	if _, err := s.Channel(ctx, id(t, "general")); !errors.Is(err, store.ErrInvalid) {
		t.Errorf("a channel row with no name: error = %v, want store.ErrInvalid", err)
	}
	if _, err := s.ChannelsInSpace(ctx, id(t, "space")); !errors.Is(err, store.ErrInvalid) {
		t.Errorf("listing a space holding a channel with no name: error = %v, want store.ErrInvalid", err)
	}
	mustExec(t, db, `DELETE FROM channel`)

	mustExec(t, db, `INSERT INTO membership (space, member) VALUES (?, ?)`, "space@example.test", "no-host")
	if _, err := s.Members(ctx, id(t, "space")); !errors.Is(err, store.ErrInvalid) {
		t.Errorf("a membership row with an unparseable member: error = %v, want store.ErrInvalid", err)
	}
}

// TestAGrantNamingSomethingTheModelDoesNotDeclareIsSkipped is the other
// direction of the same problem. A permission removed from the model leaves its
// rows behind, and a principal holding one must not be locked out of the grants
// that are still real.
func TestAGrantNamingSomethingTheModelDoesNotDeclareIsSkipped(t *testing.T) {
	ctx := context.Background()
	s, path := openTemp(t)
	db := raw(t, path)
	space, general := id(t, "space"), id(t, "general")
	ada := orchestration.Person(id(t, "ada"))
	subject := orchestration.ChannelSubject(space, general)

	if err := s.Grant(ctx, ada.ID(), subject, orchestration.SeeChannel); err != nil {
		t.Fatalf("Grant: %v", err)
	}
	mustExec(t, db, `INSERT INTO permission_grant (principal, space, channel, permission) VALUES (?, ?, ?, ?)`,
		ada.ID().String(), space.String(), general.String(), "manage-everything")

	if !orchestration.Allow(s, ada, orchestration.SeeChannel, subject) {
		t.Error("a row naming an undeclared permission took away a grant that is still declared")
	}
}

// TestAClosedStoreRefusesEveryReadAndWrite walks the error return of every
// method at once. A store whose failures were swallowed would answer a
// permission question with a set built from nothing, which is why Granted is in
// here as well as the errorful methods.
func TestAClosedStoreRefusesEveryReadAndWrite(t *testing.T) {
	ctx := context.Background()
	s, _ := openTemp(t)
	space, ada, ben := id(t, "space"), id(t, "ada"), id(t, "ben")
	subject := orchestration.ChannelSubject(space, id(t, "general"))

	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close twice: %v", err)
	}

	if err := s.PutChannel(ctx, channel(t, "general", space, false)); err == nil {
		t.Error("PutChannel on a closed store returned no error")
	}
	if _, err := s.Channel(ctx, id(t, "general")); err == nil {
		t.Error("Channel on a closed store returned no error")
	}
	if _, err := s.ChannelsInSpace(ctx, space); err == nil {
		t.Error("ChannelsInSpace on a closed store returned no error")
	}
	if err := s.PutMember(ctx, space, ada); err == nil {
		t.Error("PutMember on a closed store returned no error")
	}
	if _, err := s.Members(ctx, space); err == nil {
		t.Error("Members on a closed store returned no error")
	}
	if err := s.Grant(ctx, ada, subject, orchestration.SeeChannel); err == nil {
		t.Error("Grant on a closed store returned no error")
	}
	if err := s.SetGain(ctx, ada, ben, 40); err == nil {
		t.Error("SetGain on a closed store returned no error")
	}
	if err := s.SetGain(ctx, ada, ben, store.DefaultGain); err == nil {
		t.Error("SetGain back to unity on a closed store returned no error")
	}
	if _, err := s.Gain(ctx, ada, ben); err == nil {
		t.Error("Gain on a closed store returned no error")
	}
	if orchestration.Allow(s, orchestration.Person(ada), orchestration.SeeChannel, subject) {
		t.Error("a closed store allowed see-channel, and a failed read has to refuse")
	}
}

// ---------------------------------------------------------------------------
// Reading the schema
// ---------------------------------------------------------------------------

func objects(t *testing.T, db *sql.DB) []string {
	t.Helper()
	rows, err := db.QueryContext(context.Background(),
		`SELECT type, name FROM sqlite_schema WHERE name NOT LIKE 'sqlite_%' ORDER BY type, name`)
	if err != nil {
		t.Fatalf("reading sqlite_schema: %v", err)
	}
	defer func() { _ = rows.Close() }()

	var found []string
	for rows.Next() {
		var kind, name string
		if err := rows.Scan(&kind, &name); err != nil {
			t.Fatalf("scanning sqlite_schema: %v", err)
		}
		found = append(found, kind+" "+name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("reading sqlite_schema: %v", err)
	}
	return found
}

func columns(t *testing.T, db *sql.DB, table string) []string {
	t.Helper()
	// The table name cannot be a bound parameter in a pragma, and it comes
	// from the list in this file rather than from anywhere a caller reaches.
	rows, err := db.QueryContext(context.Background(), fmt.Sprintf(`PRAGMA table_info(%s)`, table))
	if err != nil {
		t.Fatalf("reading the columns of %s: %v", table, err)
	}
	defer func() { _ = rows.Close() }()

	var found []string
	for rows.Next() {
		var index, notNull, primaryKey int
		var name, kind string
		var fallback sql.NullString
		if err := rows.Scan(&index, &name, &kind, &notNull, &fallback, &primaryKey); err != nil {
			t.Fatalf("scanning the columns of %s: %v", table, err)
		}
		described := name + " " + kind
		if notNull != 0 {
			described += " notnull"
		}
		if primaryKey != 0 {
			described += fmt.Sprintf(" pk%d", primaryKey)
		}
		found = append(found, described)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("reading the columns of %s: %v", table, err)
	}
	return found
}

func userVersion(t *testing.T, db *sql.DB) int {
	t.Helper()
	var version int
	if err := db.QueryRowContext(context.Background(), `PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatalf("reading the schema version: %v", err)
	}
	return version
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
