// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026  Nils Lehnen

package sqlite_test

import (
	"database/sql"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// docs/privacy.md is the material an operator needs to answer for the personal
// data this software holds, and issue #81 asks that every item in it trace to
// code rather than to somebody's memory of the schema. This file is the trace.
//
// The comparison runs against the schema a migration run actually produces
// rather than against the statements in migrate.go, so a column that arrives
// through a statement nobody read is still caught, and so is one that arrives
// through a migration added later.
//
// It refuses in both directions on purpose. A column missing from the document
// is an operator answering a subject access request from a list that is short.
// A column in the document that the schema does not have is the opposite
// defect and reads exactly the same from outside: a document nobody can tell
// has drifted.
const privacyDocument = "../../../docs/privacy.md"

// inventoryRow matches one row of the document's inventory table and nothing
// else in it. The anchor is the row opening rather than a bare `table.column`
// anywhere in the prose, because the prose names files and packages in the same
// backticks and a looser pattern would read `docs/layout.md` as a column.
var inventoryRow = regexp.MustCompile("(?m)^\\| `([a-z_]+)\\.([a-z_]+)` \\|")

func TestThePrivacyDocumentNamesEveryStoredColumn(t *testing.T) {
	_, path := openTemp(t)
	db := raw(t, path)

	text, err := os.ReadFile(privacyDocument)
	if err != nil {
		t.Fatalf("reading %s: %v", privacyDocument, err)
	}

	if err := compareInventory(documented(string(text)), stored(t, db)); err != nil {
		t.Errorf("%s and the applied schema disagree:\n%v", privacyDocument, err)
	}
}

// TestTheInventoryComparisonRefusesTheRealDocumentWithOneItemRemoved deletes
// the guard's subject and requires the guard to notice.
//
// It works on the real document rather than on a made-up list, because the
// thing worth proving is that this comparison bites the artefact it is pointed
// at. The removal is the one-character-scale mistake somebody actually makes:
// a column added to a migration and a row left out of the table.
func TestTheInventoryComparisonRefusesTheRealDocumentWithOneItemRemoved(t *testing.T) {
	_, path := openTemp(t)
	db := raw(t, path)

	text, err := os.ReadFile(privacyDocument)
	if err != nil {
		t.Fatalf("reading %s: %v", privacyDocument, err)
	}

	for _, victim := range []string{"gain.percent", "membership.member", "channel.always_on"} {
		t.Run(victim, func(t *testing.T) {
			var kept []string
			for _, line := range strings.Split(string(text), "\n") {
				if !strings.HasPrefix(line, "| `"+victim+"` |") {
					kept = append(kept, line)
				}
			}
			if len(kept) == len(strings.Split(string(text), "\n")) {
				t.Fatalf("no inventory row for %s, so nothing was removed and this proves nothing", victim)
			}

			if err := compareInventory(documented(strings.Join(kept, "\n")), stored(t, db)); err == nil {
				t.Errorf("the comparison passed a document with the row for %s taken out", victim)
			}
		})
	}
}

// TestTheInventoryComparisonRefusesAColumnTheSchemaDoesNotHave is the other
// direction, and it is the one a near miss would slip through. A document that
// keeps describing a column dropped by a later migration is wrong in a way
// nobody reading it can see.
func TestTheInventoryComparisonRefusesAColumnTheSchemaDoesNotHave(t *testing.T) {
	schema := []string{"channel.id", "channel.name"}

	if err := compareInventory([]string{"channel.id", "channel.name"}, schema); err != nil {
		t.Errorf("an exact match was refused: %v", err)
	}
	if err := compareInventory([]string{"channel.id", "channel.name", "channel.retired"}, schema); err == nil {
		t.Error("a document naming a column the schema does not have was accepted")
	}
	if err := compareInventory([]string{"channel.id"}, schema); err == nil {
		t.Error("a document short one column was accepted")
	}
}

// documented returns the `table.column` items the document's inventory names,
// sorted and with duplicates kept, so a row written twice is visible as a
// difference rather than absorbed.
func documented(text string) []string {
	var found []string
	for _, match := range inventoryRow.FindAllStringSubmatch(text, -1) {
		found = append(found, match[1]+"."+match[2])
	}
	sort.Strings(found)
	return found
}

// stored returns every column of every table in the applied schema, in the same
// form. Indexes are not in it: an index holds no column of its own and the
// document is an inventory of what is held rather than of how it is reached.
func stored(t *testing.T, db *sql.DB) []string {
	t.Helper()

	var found []string
	for _, object := range objects(t, db) {
		table, isTable := strings.CutPrefix(object, "table ")
		if !isTable {
			continue
		}
		for _, described := range columns(t, db, table) {
			name, _, ok := strings.Cut(described, " ")
			if !ok {
				t.Fatalf("the column description %q carries no name", described)
			}
			found = append(found, table+"."+name)
		}
	}
	sort.Strings(found)
	return found
}

// compareInventory reports the difference in both directions, or nil where
// there is none. It names every item on both sides rather than the first,
// because a schema change touches several columns at once and a report of one
// sends the reader back for the rest.
func compareInventory(documented, schema []string) error {
	inSchema := map[string]bool{}
	for _, item := range schema {
		inSchema[item] = true
	}
	inDocument := map[string]bool{}
	for _, item := range documented {
		inDocument[item] = true
	}

	var missing, invented []string
	for _, item := range schema {
		if !inDocument[item] {
			missing = append(missing, item)
		}
	}
	for _, item := range documented {
		if !inSchema[item] {
			invented = append(invented, item)
		}
	}

	if len(missing) == 0 && len(invented) == 0 {
		return nil
	}

	var said []string
	if len(missing) > 0 {
		said = append(said, fmt.Sprintf("the schema holds %v and the document does not name them", missing))
	}
	if len(invented) > 0 {
		said = append(said, fmt.Sprintf("the document names %v and the schema does not hold them", invented))
	}
	return fmt.Errorf("%s", strings.Join(said, "\n"))
}
