// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026  Nils Lehnen

package logging_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/iderex/stammtisch/internal/logging"
)

// stamp is the instant every record in this file is written at. It is a
// constant rather than a real clock reading, so an assertion on a whole line is
// an assertion on the line.
var stamp = time.Date(2026, time.August, 10, 21, 4, 5, 987654321, time.UTC)

type fixedClock struct{ at time.Time }

func (c fixedClock) Now() time.Time { return c.at }

// failingWriter refuses every write. It exists so the one error path Record has
// that is not a refusal of the caller's arguments is exercised rather than
// assumed.
type failingWriter struct{}

var errWriterGone = errors.New("the writer is gone")

func (failingWriter) Write([]byte) (int, error) { return 0, errWriterGone }

func newLog(t *testing.T, w *strings.Builder) *logging.Log {
	t.Helper()
	l, err := logging.New(w, fixedClock{at: stamp})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return l
}

func identifier(t *testing.T, s string) logging.Identifier {
	t.Helper()
	id, err := logging.NewIdentifier(s)
	if err != nil {
		t.Fatalf("NewIdentifier(%q): %v", s, err)
	}
	return id
}

// TestNewRefusesALogItCannotWrite holds the fail-closed half of construction. A
// server that started with no destination for its log should say so while
// somebody is still watching it start.
func TestNewRefusesALogItCannotWrite(t *testing.T) {
	if _, err := logging.New(nil, fixedClock{at: stamp}); !errors.Is(err, logging.ErrIncomplete) {
		t.Errorf("New with no writer returned %v, and it is ErrIncomplete", err)
	}
	if _, err := logging.New(&strings.Builder{}, nil); !errors.Is(err, logging.ErrIncomplete) {
		t.Errorf("New with no clock returned %v, and it is ErrIncomplete", err)
	}
}

// TestARecordIsOneLineInTheDeclaredOrder is the shape every other assertion
// here is written against.
func TestARecordIsOneLineInTheDeclaredOrder(t *testing.T) {
	var out strings.Builder
	l := newLog(t, &out)

	// Deliberately out of the declared order, so the line below is evidence
	// that Record imposes an order rather than repeating the caller's.
	err := l.Record(logging.MemberEntered,
		logging.Count(3),
		logging.Member(identifier(t, "nils@stammtisch.example")),
		logging.Channel(identifier(t, "allgemein@stammtisch.example")),
		logging.Space(identifier(t, "verein@stammtisch.example")),
	)
	if err != nil {
		t.Fatalf("Record: %v", err)
	}

	want := "at=2026-08-10T21:04:05Z event=member-entered" +
		" space=verein@stammtisch.example" +
		" channel=allgemein@stammtisch.example" +
		" member=nils@stammtisch.example" +
		" count=3\n"
	if out.String() != want {
		t.Errorf("the record is\n%q\nand it should be\n%q", out.String(), want)
	}
}

// TestTheTimestampIsWholeSeconds holds the truncation. A log shipped to
// somebody else's aggregator with nanosecond stamps on it is a correlation
// handle nobody asked for.
func TestTheTimestampIsWholeSeconds(t *testing.T) {
	var out strings.Builder
	l := newLog(t, &out)
	if err := l.Record(logging.SessionClosed); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if strings.Contains(out.String(), "987") {
		t.Errorf("the record carries sub-second precision: %q", out.String())
	}
}

// TestEveryEventInTheSetHasAName covers the whole closed set, and the case
// outside it. An event whose name is empty would be written as a record with no
// subject, which is why Record refuses it instead.
func TestEveryEventInTheSetHasAName(t *testing.T) {
	events := []struct {
		event logging.Event
		name  string
	}{
		{logging.ProtocolAgreed, "protocol-agreed"},
		{logging.ProtocolRefused, "protocol-refused"},
		{logging.SessionOpened, "session-opened"},
		{logging.SessionRefused, "session-refused"},
		{logging.SessionClosed, "session-closed"},
		{logging.ClientAttached, "client-attached"},
		{logging.ClientDetached, "client-detached"},
		{logging.ChannelSubscribed, "channel-subscribed"},
		{logging.ChannelUnsubscribed, "channel-unsubscribed"},
		{logging.MemberEntered, "member-entered"},
		{logging.MemberLeft, "member-left"},
		{logging.PresenceDelivered, "presence-delivered"},
	}
	seen := map[string]bool{}
	for _, e := range events {
		if got := e.event.String(); got != e.name {
			t.Errorf("event %d is named %q and it should be %q", uint8(e.event), got, e.name)
		}
		if seen[e.name] {
			t.Errorf("%q names two events", e.name)
		}
		seen[e.name] = true
	}
	for _, outside := range []logging.Event{0, 200} {
		if got := outside.String(); got != "" {
			t.Errorf("event %d is not in the set and it named itself %q", uint8(outside), got)
		}
	}
}

// TestRecordRefusesWhatWouldMakeALineNobodyCanRead is the guard. Each case here
// is a record that would otherwise be written, and each names a different way
// of writing one that says less than it appears to.
func TestRecordRefusesWhatWouldMakeALineNobodyCanRead(t *testing.T) {
	cases := []struct {
		what   string
		event  logging.Event
		fields []logging.Field
	}{
		{
			what:  "an event this build does not define",
			event: logging.Event(200),
		},
		{
			what:   "a zero Field, which is the only Field a caller can build without a constructor",
			event:  logging.SessionOpened,
			fields: []logging.Field{{}},
		},
		{
			what:   "a field built from the zero Identifier",
			event:  logging.MemberEntered,
			fields: []logging.Field{logging.Channel(logging.Identifier{})},
		},
		{
			what:  "the same field twice, which is a line whose reader has to pick one",
			event: logging.MemberEntered,
			fields: []logging.Field{
				logging.Member(identifier(t, "nils@stammtisch.example")),
				logging.Member(identifier(t, "aylin@stammtisch.example")),
			},
		},
	}

	for _, c := range cases {
		var out strings.Builder
		l := newLog(t, &out)
		err := l.Record(c.event, c.fields...)
		if !errors.Is(err, logging.ErrRefused) {
			t.Errorf("%s: Record returned %v, and it is ErrRefused", c.what, err)
		}
		if out.Len() != 0 {
			t.Errorf("%s: Record refused and wrote %q anyway", c.what, out.String())
		}
	}
}

// TestARefusedWriteIsReturnedRatherThanSwallowed. A log surface that dropped
// its own write error would be the one place in the server where a failure
// leaves no trace anywhere, by construction.
func TestARefusedWriteIsReturnedRatherThanSwallowed(t *testing.T) {
	l, err := logging.New(failingWriter{}, fixedClock{at: stamp})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := l.Record(logging.SessionOpened); !errors.Is(err, errWriterGone) {
		t.Errorf("Record returned %v, and the writer's own error should be under it", err)
	}
}

// TestNewIdentifierAdmitsAnIdentifierAndNothingElse is the runtime half of the
// discipline. The compile-time half is next door: a caller holding free text
// has nothing on this surface to pass it to, and this is what stops them
// turning it into an Identifier first.
func TestNewIdentifierAdmitsAnIdentifierAndNothingElse(t *testing.T) {
	refused := []struct {
		what string
		s    string
	}{
		{"a channel name somebody typed", "Geburtstag von Nils"},
		{"a sentence out of a conversation", "wir sehen uns um acht"},
		{"a display name with no host", "Nils"},
		{"an empty local part", "@stammtisch.example"},
		{"an empty host", "nils@"},
		{"a second separator", "nils@stammtisch@example"},
		{"a plain space", "nils lehnen@stammtisch.example"},
		// The next two are written as escapes rather than as themselves. A
		// literal one in a tracked file is what unicode-guard.yml refuses, and
		// the case here is the same character arriving at runtime instead.
		{"a non-breaking space", "nils\u00a0lehnen@stammtisch.example"},
		{"a bidirectional control", "nils\u202elehnen@stammtisch.example"},
		{"a newline, which would forge a second record", "nils@stammtisch.example\nat=1970-01-01T00:00:00Z"},
		{"a value past the bound", strings.Repeat("a", logging.MaxIdentifierLength) + "@stammtisch.example"},
	}
	for _, c := range refused {
		if _, err := logging.NewIdentifier(c.s); !errors.Is(err, logging.ErrNotAnIdentifier) {
			t.Errorf("%s: NewIdentifier returned %v, and it is ErrNotAnIdentifier", c.what, err)
		}
	}

	for _, s := range []string{
		"nils@stammtisch.example",
		"allgemein@stammtisch.example",
		"c-7f3a@stammtisch.example",
		"grüße@stammtisch.example",
	} {
		id, err := logging.NewIdentifier(s)
		if err != nil {
			t.Errorf("NewIdentifier(%q): %v", s, err)
			continue
		}
		if id.String() != s {
			t.Errorf("NewIdentifier(%q) reads back as %q", s, id.String())
		}
		if id.IsZero() {
			t.Errorf("NewIdentifier(%q) reports itself as the zero identifier", s)
		}
	}

	if !(logging.Identifier{}).IsZero() {
		t.Error("the zero Identifier does not report itself as one")
	}
	if got := (logging.Identifier{}).String(); got != "" {
		t.Errorf("the zero Identifier formats as %q rather than as nothing", got)
	}
}

// TestARefusedIdentifierNeverCarriesTheValueItRefused. An error is a string
// that ends up in somebody's log, so the one place this package handles free
// text is the place it must not repeat it back.
func TestARefusedIdentifierNeverCarriesTheValueItRefused(t *testing.T) {
	secret := "Geburtstag von Nils"
	_, err := logging.NewIdentifier(secret)
	if err == nil {
		t.Fatal("NewIdentifier admitted a channel name")
	}
	if strings.Contains(err.Error(), secret) {
		t.Errorf("the refusal repeats the value back: %q", err.Error())
	}
}

// TestTheNumericFieldsCarryNoSeparator. The format needs no quoting because no
// value can hold a space, and that is a property of every constructor rather
// than of the four that take an Identifier.
func TestTheNumericFieldsCarryNoSeparator(t *testing.T) {
	var out strings.Builder
	l := newLog(t, &out)
	err := l.Record(logging.ProtocolAgreed,
		logging.Version(1),
		logging.Count(-3),
		logging.Held(1500*time.Millisecond),
	)
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	want := "at=2026-08-10T21:04:05Z event=protocol-agreed version=1 count=-3 held=1500ms\n"
	if out.String() != want {
		t.Errorf("the record is\n%q\nand it should be\n%q", out.String(), want)
	}
}

// TestHeldIsWholeMilliseconds. Anything finer measures the machine rather than
// the thing that took the time.
func TestHeldIsWholeMilliseconds(t *testing.T) {
	var out strings.Builder
	l := newLog(t, &out)
	if err := l.Record(logging.SessionClosed, logging.Held(1234567*time.Nanosecond)); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if !strings.HasSuffix(out.String(), "held=1ms\n") {
		t.Errorf("the record is %q and the duration should have been rounded to whole milliseconds", out.String())
	}
}

// TestTheDeclaredSetIsWhatARecordCanCarry. The set is the thing docs/privacy.md
// names, and the two have to move together or the document is describing a
// surface that has changed underneath it.
func TestTheDeclaredSetIsWhatARecordCanCarry(t *testing.T) {
	want := []string{"at", "event", "space", "channel", "member", "client", "version", "count", "held"}
	got := logging.DeclaredFieldNames()
	if len(got) != len(want) {
		t.Fatalf("the declared set is %v and it should be %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("the declared set is %v and it should be %v", got, want)
			break
		}
	}
}
