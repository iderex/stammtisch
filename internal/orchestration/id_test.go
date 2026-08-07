// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026  iderex

package orchestration_test

import (
	"errors"
	"testing"

	"github.com/iderex/stammtisch/internal/orchestration"
)

// TestAnIdentifierFromAnotherHostRoundTrips is the federation obligation in
// docs/decisions/federation.md, held as a test rather than as a sentence. The
// form carries a host from the start so it does not have to change later, and
// the proof that it does is a value from a host that is not this one going
// through parse and format unchanged.
func TestAnIdentifierFromAnotherHostRoundTrips(t *testing.T) {
	const written = "general@chat.example.org"

	id, err := orchestration.ParseID(written)
	if err != nil {
		t.Fatalf("ParseID(%q): %v", written, err)
	}
	if got := id.Local(); got != "general" {
		t.Errorf("local part = %q, want %q", got, "general")
	}
	if got := id.Host(); got != "chat.example.org" {
		t.Errorf("host = %q, want %q", got, "chat.example.org")
	}
	if got := id.String(); got != written {
		t.Errorf("round trip gave %q, want %q", got, written)
	}

	again, err := orchestration.ParseID(id.String())
	if err != nil {
		t.Fatalf("re-parsing %q: %v", id.String(), err)
	}
	if again != id {
		t.Errorf("second trip gave %v, want %v", again, id)
	}
}

// TestAnIdentifierWithoutAHostIsRefused holds the same obligation from the other
// side: a bare local part is not an identifier here, however convenient it would
// be while there is one host.
func TestAnIdentifierWithoutAHostIsRefused(t *testing.T) {
	if _, err := orchestration.ParseID("general"); !errors.Is(err, orchestration.ErrInvalidID) {
		t.Errorf("ParseID(\"general\") error = %v, want ErrInvalidID", err)
	}
	if _, err := orchestration.NewID("general", ""); !errors.Is(err, orchestration.ErrInvalidID) {
		t.Errorf("NewID with an empty host error = %v, want ErrInvalidID", err)
	}
	if _, err := orchestration.NewID("", "chat.example.org"); !errors.Is(err, orchestration.ErrInvalidID) {
		t.Errorf("NewID with an empty local part error = %v, want ErrInvalidID", err)
	}
}

// TestAnIdentifierPartCarryingTheSeparatorIsRefused is the near miss the round
// trip depends on. A local part holding an @ would format and parse back as a
// different identifier, and nothing downstream would notice.
func TestAnIdentifierPartCarryingTheSeparatorIsRefused(t *testing.T) {
	if _, err := orchestration.NewID("gene@ral", "chat.example.org"); !errors.Is(err, orchestration.ErrInvalidID) {
		t.Fatalf("NewID with an @ in the local part error = %v, want ErrInvalidID", err)
	}

	// And the string such a value would have formatted to is refused on the way
	// back in rather than cut at the first separator into a different
	// identifier. The cut takes the first @, which leaves the second one in the
	// host, where the same rule refuses it.
	if _, err := orchestration.ParseID("gene@ral@chat.example.org"); !errors.Is(err, orchestration.ErrInvalidID) {
		t.Errorf("ParseID of a doubled separator error = %v, want ErrInvalidID", err)
	}
	if _, err := orchestration.NewID("general", "chat@example.org"); !errors.Is(err, orchestration.ErrInvalidID) {
		t.Errorf("NewID with an @ in the host error = %v, want ErrInvalidID", err)
	}
}

// TestAnIdentifierPartCarryingWhitespaceIsRefused keeps a value that would look
// identical in a log out of the model.
func TestAnIdentifierPartCarryingWhitespaceIsRefused(t *testing.T) {
	for _, part := range []string{"general ", " general", "gen eral", "gen\teral", "gen\neral"} {
		if _, err := orchestration.NewID(part, "chat.example.org"); !errors.Is(err, orchestration.ErrInvalidID) {
			t.Errorf("NewID(%q, ...) error = %v, want ErrInvalidID", part, err)
		}
	}
}

// TestTheZeroIdentifierIsNotOneAndFormatsAsNothing. Every constructor in the
// package refuses the zero ID, so it has to be recognisable as one rather than
// formatting as the plausible-looking "@".
func TestTheZeroIdentifierIsNotOneAndFormatsAsNothing(t *testing.T) {
	var zero orchestration.ID
	if !zero.IsZero() {
		t.Error("the zero ID does not report itself as zero")
	}
	if got := zero.String(); got != "" {
		t.Errorf("the zero ID formats as %q, want the empty string", got)
	}

	id, err := orchestration.NewID("general", "chat.example.org")
	if err != nil {
		t.Fatalf("NewID: %v", err)
	}
	if id.IsZero() {
		t.Error("a constructed ID reports itself as zero")
	}
}
