// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026  Nils Lehnen

package logging_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/iderex/stammtisch/internal/auth"
	"github.com/iderex/stammtisch/internal/logging"
	"github.com/iderex/stammtisch/internal/orchestration"
	"github.com/iderex/stammtisch/internal/signalling"
)

// host is the one host every identifier in this file is qualified with.
const host = "stammtisch.example"

// channelName is the free text this session carries. It is a channel named
// after a person, which docs/privacy.md names as the case nothing in the
// software can tell from an ordinary channel name, and it is what the
// assertions below look for in the log.
const channelName = "Geburtstag von Nils"

// seesEverything grants the see permission on every channel, so the fan-out
// below is not filtered and the log covers the whole session rather than the
// part one viewer could observe.
type seesEverything struct{}

func (seesEverything) Granted(_ orchestration.Principal, s orchestration.Subject) orchestration.PermissionSet {
	if _, aboutChannel := s.Channel(); !aboutChannel {
		return orchestration.NewPermissionSet()
	}
	return orchestration.NewPermissionSet(orchestration.SeeChannel)
}

func mustID(t *testing.T, local string) orchestration.ID {
	t.Helper()
	id, err := orchestration.NewID(local, host)
	if err != nil {
		t.Fatalf("NewID(%q, %q): %v", local, host, err)
	}
	return id
}

// hello returns a stream carrying one hello frame proposing a version.
//
// The payload is written out as bytes rather than built through this package's
// own encoder, which is unexported. Two bytes of field identifier, two of
// length, two of version.
func hello(t *testing.T, proposed uint16) *bytes.Buffer {
	t.Helper()
	payload := []byte{0, 1, 0, 2, byte(proposed >> 8), byte(proposed)}
	var stream bytes.Buffer
	if err := signalling.Encode(&stream, signalling.Frame{Kind: signalling.KindHello, Payload: payload}); err != nil {
		t.Fatalf("encoding a hello proposing %d: %v", proposed, err)
	}
	return &stream
}

// TestAFullSessionCarriesNoFieldOutsideTheDeclaredSet drives one connection
// from the version negotiation to the revoked session, through the packages
// that actually hold each step, and reads the log it produced.
//
// It drives the packages rather than a server, because there is no server: the
// entry point still prints one line and exits, and wiring one is issue #66's
// configuration and #69's endpoints. So what is asserted here is that a session
// driven through signalling, auth and orchestration produces a log carrying
// only declared fields and none of the free text those packages held while it
// ran. What is not asserted is that a running server logs at these points,
// because there is no running server to log anything.
func TestAFullSessionCarriesNoFieldOutsideTheDeclaredSet(t *testing.T) {
	var out strings.Builder
	l := newLog(t, &out)
	clock := fixedClock{at: stamp}

	space := mustID(t, "verein")
	channel := mustID(t, "allgemein")
	member := mustID(t, "nils")

	room, err := orchestration.NewChannel(channel, space, channelName, true)
	if err != nil {
		t.Fatalf("NewChannel: %v", err)
	}
	if room.Name() != channelName {
		t.Fatalf("the channel is named %q, so this test is not carrying the free text it thinks it is", room.Name())
	}

	// The protocol, both ways.
	agreed, err := signalling.Negotiate(signalling.NewConn(hello(t, 1)))
	if err != nil {
		t.Fatalf("Negotiate: %v", err)
	}
	record(t, l, logging.ProtocolAgreed, logging.Version(uint16(agreed)))

	if _, err := signalling.Negotiate(signalling.NewConn(hello(t, 9))); !errors.Is(err, signalling.ErrVersionUnsupported) {
		t.Fatalf("Negotiate on version 9 returned %v, and it is ErrVersionUnsupported", err)
	}
	record(t, l, logging.ProtocolRefused, logging.Version(9))

	// The session. The client identifier is the session identifier qualified
	// with the host that minted it, which is the form every other identifier
	// in this tree carries and the reason it can be a field at all.
	sessions := auth.NewSessions(clock)
	session, token, err := sessions.Open(member.String())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	client := identifier(t, session.ID+"@"+host)
	record(t, l, logging.SessionOpened,
		logging.Member(identifier(t, member.String())),
		logging.Client(client),
	)

	if _, err := sessions.Lookup("this is not a token"); err == nil {
		t.Fatal("Lookup admitted a token nothing was issued for")
	}
	record(t, l, logging.SessionRefused)

	// Presence, which is where a channel and its occupants meet.
	hub, err := orchestration.NewPresenceHub(space, seesEverything{})
	if err != nil {
		t.Fatalf("NewPresenceHub: %v", err)
	}
	if err := hub.AddChannel(room.ID()); err != nil {
		t.Fatalf("AddChannel: %v", err)
	}

	connection := orchestration.ClientID(client.String())
	if _, err := hub.Attach(connection, orchestration.Person(member)); err != nil {
		t.Fatalf("Attach: %v", err)
	}
	record(t, l, logging.ClientAttached,
		logging.Space(identifier(t, space.String())),
		logging.Client(client),
		logging.Member(identifier(t, member.String())),
	)

	if _, err := hub.Subscribe(connection, room.ID()); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	record(t, l, logging.ChannelSubscribed,
		logging.Channel(identifier(t, room.ID().String())),
		logging.Client(client),
	)

	if !hub.Apply(orchestration.PresenceReport{Member: member, Seq: 1, Channel: channel, Since: stamp}) {
		t.Fatal("Apply reported that entering a channel moved nothing")
	}
	record(t, l, logging.MemberEntered,
		logging.Space(identifier(t, space.String())),
		logging.Channel(identifier(t, channel.String())),
		logging.Member(identifier(t, member.String())),
	)

	deliveries := hub.Flush()
	if len(deliveries) == 0 {
		t.Fatal("Flush delivered nothing after a member entered a channel")
	}
	record(t, l, logging.PresenceDelivered,
		logging.Channel(identifier(t, channel.String())),
		logging.Count(len(deliveries)),
	)

	// Leaving, and the session ending.
	if !hub.Apply(orchestration.PresenceReport{Member: member, Seq: 2, Since: stamp}) {
		t.Fatal("Apply reported that leaving moved nothing")
	}
	record(t, l, logging.MemberLeft,
		logging.Channel(identifier(t, channel.String())),
		logging.Member(identifier(t, member.String())),
	)

	hub.Unsubscribe(connection, room.ID())
	record(t, l, logging.ChannelUnsubscribed,
		logging.Channel(identifier(t, room.ID().String())),
		logging.Client(client),
	)

	hub.Detach(connection)
	record(t, l, logging.ClientDetached, logging.Client(client))

	if !sessions.Revoke(session.ID) {
		t.Fatal("Revoke did not find the session it had just opened")
	}
	record(t, l, logging.SessionClosed,
		logging.Client(client),
		logging.Held(auth.TokenLifetime),
	)

	assertOnlyDeclaredFields(t, out.String())
	assertCarriesNone(t, out.String(), map[string]string{
		"the channel's name":     channelName,
		"a word out of the name": "Geburtstag",
		"the session token":      token,
	})
	assertEveryEventWasDriven(t, out.String())
}

func record(t *testing.T, l *logging.Log, e logging.Event, fields ...logging.Field) {
	t.Helper()
	if err := l.Record(e, fields...); err != nil {
		t.Fatalf("recording %s: %v", e, err)
	}
}

// assertOnlyDeclaredFields is the condition this test exists for. It reads the
// log back as a reader would, splitting on the separator rather than on
// anything it knows about how the line was built.
func assertOnlyDeclaredFields(t *testing.T, log string) {
	t.Helper()
	declared := map[string]bool{}
	for _, name := range logging.DeclaredFieldNames() {
		declared[name] = true
	}

	lines := strings.Split(strings.TrimSuffix(log, "\n"), "\n")
	if len(lines) < 2 {
		t.Fatalf("the session produced %d line(s), which is not a session", len(lines))
	}
	for _, line := range lines {
		for _, pair := range strings.Split(line, " ") {
			name, value, found := strings.Cut(pair, "=")
			if !found {
				t.Errorf("%q is not a name and a value, so the line does not parse: %q", pair, line)
				continue
			}
			if !declared[name] {
				t.Errorf("the log carries the field %q, which is outside the declared set: %q", name, line)
			}
			if value == "" {
				t.Errorf("the field %q carries nothing: %q", name, line)
			}
		}
	}
}

func assertCarriesNone(t *testing.T, log string, forbidden map[string]string) {
	t.Helper()
	for what, value := range forbidden {
		if strings.Contains(log, value) {
			t.Errorf("the log carries %s", what)
		}
	}
}

// assertEveryEventWasDriven is what makes this a full session rather than a
// selection of one. An event this build declares and this test never reaches is
// an event whose fields nothing here has checked.
func assertEveryEventWasDriven(t *testing.T, log string) {
	t.Helper()
	for e := logging.ProtocolAgreed; e <= logging.PresenceDelivered; e++ {
		if !strings.Contains(log, "event="+e.String()+" ") && !strings.Contains(log, "event="+e.String()+"\n") {
			t.Errorf("the session never reached %s, so nothing here read the fields it carries", e)
		}
	}
}
