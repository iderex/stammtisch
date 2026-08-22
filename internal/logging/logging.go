// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026  Nils Lehnen

package logging

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
)

// Clock is the only way this package learns what time it is.
//
// It is declared here rather than imported so that this package depends on
// nothing else in the module, which is what lets every package under internal/
// import it. Anything satisfying orchestration's Clock satisfies this one, so a
// server builds one implementation and hands it to all three.
type Clock interface {
	Now() time.Time
}

// MaxIdentifierLength bounds what NewIdentifier will admit.
//
// The bound is here because an unbounded field is how a payload reaches a log
// line whole. Nothing this tree produces comes near it: an identifier is
// local@host, both parts named by a person or minted by the server.
const MaxIdentifierLength = 128

// ErrNotAnIdentifier is returned by NewIdentifier for a value that is not
// local@host.
var ErrNotAnIdentifier = errors.New("logging: not an identifier")

// ErrRefused is returned by Record for a record it will not write.
var ErrRefused = errors.New("logging: the record is refused")

// ErrIncomplete is returned by New when it is not given everything a log needs.
var ErrIncomplete = errors.New("logging: the log is incomplete")

// An Identifier is a value this surface will accept as a field.
//
// It is local@host, with no space, no character that does not print, one
// separator and a bounded length. That is the grammar
// docs/decisions/federation.md fixes for an identifier in this tree, and
// holding a field to it is what stops free text being laundered into a log by
// a caller who has a string and needs one of these.
//
// The zero Identifier is not one. Every constructor below turns it into the
// zero Field, which Record refuses.
type Identifier struct {
	s string
}

// NewIdentifier returns s as an identifier, or refuses it.
//
// The refusal names the rule and never the value. An error is a string that
// ends up in somebody's log, and the value here is exactly the thing this
// package exists to keep out of one.
func NewIdentifier(s string) (Identifier, error) {
	if len(s) > MaxIdentifierLength {
		return Identifier{}, fmt.Errorf("%w: it is %d bytes and the bound is %d", ErrNotAnIdentifier, len(s), MaxIdentifierLength)
	}
	local, host, found := strings.Cut(s, "@")
	if !found {
		return Identifier{}, fmt.Errorf("%w: it carries no @ separating the local part from the host", ErrNotAnIdentifier)
	}
	if err := checkPart("local part", local); err != nil {
		return Identifier{}, err
	}
	if err := checkPart("host", host); err != nil {
		return Identifier{}, err
	}
	return Identifier{s: s}, nil
}

// String returns the identifier. The zero Identifier formats as the empty
// string rather than as "@", so a zero value is recognisable as one.
func (i Identifier) String() string { return i.s }

// IsZero reports whether this is the zero Identifier, which NewIdentifier does
// not produce.
func (i Identifier) IsZero() bool { return i.s == "" }

// checkPart holds one half of the grammar.
//
// The character rule is stricter than a control-character scan and deliberately
// so. A record is written as space-separated pairs, so a value carrying any
// kind of space would produce a line that parses as more fields than it has,
// and a value carrying a bidirectional or invisible control would render in an
// operator's terminal as something other than what it is. That is the same
// attack .github/workflows/unicode-guard.yml refuses in tracked text, arriving
// at runtime instead.
func checkPart(what, s string) error {
	if s == "" {
		return fmt.Errorf("%w: the %s is empty", ErrNotAnIdentifier, what)
	}
	if strings.Contains(s, "@") {
		return fmt.Errorf("%w: the %s carries an @, which is the separator", ErrNotAnIdentifier, what)
	}
	for _, r := range s {
		if unicode.IsSpace(r) || !unicode.IsPrint(r) {
			return fmt.Errorf("%w: the %s carries a space or a character that does not print", ErrNotAnIdentifier, what)
		}
	}
	return nil
}

// An Event is what happened. The set is closed, and it is closed because a
// record whose subject is free text is a record carrying whatever the caller
// had to hand.
type Event uint8

// The events this build records. The zero Event is not one of them, so a
// forgotten argument is refused rather than written as an empty subject.
//
// Each pair that can go either way is two events rather than one event and an
// outcome field. A reader grepping for a refusal should find it by name.
const (
	ProtocolAgreed Event = iota + 1
	ProtocolRefused
	SessionOpened
	SessionRefused
	SessionClosed
	ClientAttached
	ClientDetached
	ChannelSubscribed
	ChannelUnsubscribed
	MemberEntered
	MemberLeft
	PresenceDelivered
)

// String returns the name written into a record, and the empty string for an
// event this build does not define. Record refuses that case, so the empty
// string never reaches a line.
func (e Event) String() string {
	switch e {
	case ProtocolAgreed:
		return "protocol-agreed"
	case ProtocolRefused:
		return "protocol-refused"
	case SessionOpened:
		return "session-opened"
	case SessionRefused:
		return "session-refused"
	case SessionClosed:
		return "session-closed"
	case ClientAttached:
		return "client-attached"
	case ClientDetached:
		return "client-detached"
	case ChannelSubscribed:
		return "channel-subscribed"
	case ChannelUnsubscribed:
		return "channel-unsubscribed"
	case MemberEntered:
		return "member-entered"
	case MemberLeft:
		return "member-left"
	case PresenceDelivered:
		return "presence-delivered"
	}
	return ""
}

// A key is one field's name. It is unexported, and that is the whole of why a
// caller cannot invent a field: the only values of this type are the constants
// below and the only way to attach one to a value is a constructor here.
type key uint8

const (
	keyAt key = iota + 1
	keyEvent
	keySpace
	keyChannel
	keyMember
	keyClient
	keyVersion
	keyCount
	keyHeld
)

// keyNames is what each key is written as.
var keyNames = map[key]string{
	keyAt:      "at",
	keyEvent:   "event",
	keySpace:   "space",
	keyChannel: "channel",
	keyMember:  "member",
	keyClient:  "client",
	keyVersion: "version",
	keyCount:   "count",
	keyHeld:    "held",
}

func (k key) String() string { return keyNames[k] }

// declaredKeys is every field a record may carry, in the order Record writes
// them. The order is fixed here rather than taken from the caller, so two
// callers recording the same event produce the same line and a reader comparing
// two lines is comparing the events rather than the argument order.
var declaredKeys = []key{
	keyAt,
	keyEvent,
	keySpace,
	keyChannel,
	keyMember,
	keyClient,
	keyVersion,
	keyCount,
	keyHeld,
}

// A Field is one name and its value. It carries no exported field and has no
// exported constructor beyond the ones below, so the zero Field is the only one
// a caller can build directly, and Record refuses it.
type Field struct {
	k key
	v string
}

// Space records which space the event happened in.
func Space(id Identifier) Field { return identifierField(keySpace, id) }

// Channel records which channel. It is the channel's identifier and never its
// name, which is free text a person typed.
func Channel(id Identifier) Field { return identifierField(keyChannel, id) }

// Member records which person or bot. It is their identifier and never their
// display name.
func Member(id Identifier) Field { return identifierField(keyMember, id) }

// Client records which of a member's connections. A member may hold several and
// an operator reading a log has to be able to tell them apart.
func Client(id Identifier) Field { return identifierField(keyClient, id) }

// Version records a protocol version.
func Version(v uint16) Field {
	return Field{k: keyVersion, v: strconv.FormatUint(uint64(v), 10)}
}

// Count records how many of something. It is a number about a set and never a
// member of one.
func Count(n int) Field { return Field{k: keyCount, v: strconv.Itoa(n)} }

// Held records a duration, in whole milliseconds. Anything finer is a
// measurement of the machine rather than of the thing that took the time, and
// it is a correlation handle in a log that is shipped somewhere else.
func Held(d time.Duration) Field {
	return Field{k: keyHeld, v: strconv.FormatInt(d.Round(time.Millisecond).Milliseconds(), 10) + "ms"}
}

// identifierField turns a zero Identifier into a zero Field rather than
// refusing here. The refusal belongs at Record, where it is one check covering
// both a zero Identifier and a Field a caller built by writing Field{}.
func identifierField(k key, id Identifier) Field {
	if id.IsZero() {
		return Field{}
	}
	return Field{k: k, v: id.s}
}

// A Log is the surface. Everything this server writes to an operator's log goes
// through Record and there is no second way.
type Log struct {
	mu    sync.Mutex
	w     io.Writer
	clock Clock
}

// New returns a Log writing to w and stamping from clock.
//
// It refuses either being absent rather than accepting them and failing at the
// first record, because a server that started without a log destination should
// say so while somebody is still watching it start.
func New(w io.Writer, clock Clock) (*Log, error) {
	if w == nil {
		return nil, fmt.Errorf("%w: there is no writer to record to", ErrIncomplete)
	}
	if clock == nil {
		return nil, fmt.Errorf("%w: there is no clock to stamp a record with", ErrIncomplete)
	}
	return &Log{w: w, clock: clock}, nil
}

// Record writes one line: the time, the event, and the fields given, in the
// declared order.
//
// It refuses an event this build does not define, a zero Field, and the same
// field twice. The last is not tidiness: two values under one name is a line
// whose reader has to choose which one to believe, and the choice is silent.
func (l *Log) Record(e Event, fields ...Field) error {
	name := e.String()
	if name == "" {
		return fmt.Errorf("%w: event %d is not in this build's set", ErrRefused, uint8(e))
	}

	given := make(map[key]string, len(fields))
	for _, f := range fields {
		if f.k == 0 {
			return fmt.Errorf("%w: a field carries no name, so it came from a zero value rather than from a constructor", ErrRefused)
		}
		if _, already := given[f.k]; already {
			return fmt.Errorf("%w: %s is given twice and a record carries each field once", ErrRefused, f.k)
		}
		given[f.k] = f.v
	}

	given[keyAt] = l.clock.Now().UTC().Truncate(time.Second).Format(time.RFC3339)
	given[keyEvent] = name

	var b strings.Builder
	for _, k := range declaredKeys {
		v, present := given[k]
		if !present {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(k.String())
		b.WriteByte('=')
		b.WriteString(v)
	}
	b.WriteByte('\n')

	l.mu.Lock()
	defer l.mu.Unlock()
	if _, err := fmt.Fprint(l.w, b.String()); err != nil {
		return fmt.Errorf("writing the record: %w", err)
	}
	return nil
}
