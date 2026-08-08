// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026  iderex

package orchestration_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/iderex/stammtisch/internal/orchestration"
)

// presenceGrants grants the see permission on every channel except the ones it
// is told to hide from a given principal. It is the whole of what the presence
// fan-out asks a store for, and hiding is per principal because the case that
// matters is one viewer being kept out of one channel while everybody else sees
// it.
type presenceGrants struct {
	hiddenFrom map[string]map[orchestration.ID]bool
}

func (g presenceGrants) Granted(p orchestration.Principal, s orchestration.Subject) orchestration.PermissionSet {
	channel, aboutChannel := s.Channel()
	if !aboutChannel {
		return orchestration.NewPermissionSet()
	}
	if g.hiddenFrom[p.ID().String()][channel] {
		return orchestration.NewPermissionSet()
	}
	return orchestration.NewPermissionSet(orchestration.SeeChannel)
}

func seesEverything() presenceGrants {
	return presenceGrants{hiddenFrom: map[string]map[orchestration.ID]bool{}}
}

// presenceAt is a fixed reading of a server clock. Nothing in this package may
// read the operating system clock, tests included, and
// TestNothingInOrchestrationReadsTheClockDirectly refuses it, so the time an
// occupancy began is a value the test names.
func presenceAt(seconds int) time.Time {
	return time.Date(2026, 8, 8, 12, 0, seconds, 0, time.UTC)
}

func presenceHub(t *testing.T, g orchestration.Grantor) *orchestration.PresenceHub {
	t.Helper()
	h, err := orchestration.NewPresenceHub(id(t, "space"), g)
	if err != nil {
		t.Fatalf("NewPresenceHub: %v", err)
	}
	return h
}

// TestOneMoveCostsOneMessagePerClientAndNoMore is the measurement the first
// done-when condition of #35 asks for, at the size the presence record works
// through: a space of two hundred channels and a thousand connected clients,
// one person moving from one channel to another, every channel viewable by
// everybody so nothing is filtered out by permissions.
//
// The figures it asserts, and they are the record's:
//
//   - 1000 messages, which is one per connected client and is the bound. The
//     naive design sends one message per change per client, which here is 2000.
//   - 2000 count deltas, two per message, because both channels moved.
//   - 8 of those messages also carry the identity of the person who moved: the
//     five subscribed to the membership of the channel they left plus the three
//     subscribed to the one they joined. The other 992 clients learn that a
//     count changed and never learn who.
func TestOneMoveCostsOneMessagePerClientAndNoMore(t *testing.T) {
	const (
		channelCount = 200
		clientCount  = 1000
		inA          = 5
		inB          = 3
	)

	hub := presenceHub(t, seesEverything())

	channels := make([]orchestration.ID, channelCount)
	for i := range channels {
		channels[i] = id(t, fmt.Sprintf("channel-%03d", i+1))
		if err := hub.AddChannel(channels[i]); err != nil {
			t.Fatalf("AddChannel(%s): %v", channels[i], err)
		}
	}
	a, b := channels[0], channels[1]

	members := make([]orchestration.ID, clientCount)
	for i := range members {
		members[i] = id(t, fmt.Sprintf("member-%04d", i+1))
	}

	// Eight people are already in two channels before anybody connects.
	for i := 0; i < inA; i++ {
		applyPresence(t, hub, members[i], 1, a, presenceAt(i))
	}
	for i := inA; i < inA+inB; i++ {
		applyPresence(t, hub, members[i], 1, b, presenceAt(i))
	}
	// Clear the interval those reports landed in. Nobody is attached, so there
	// is nothing to deliver and this only resets the accumulator.
	if got := hub.Flush(); len(got) != 0 {
		t.Fatalf("flushing before anybody attached produced %d deliveries", len(got))
	}

	clients := make([]orchestration.ClientID, clientCount)
	for i := range clients {
		clients[i] = orchestration.ClientID(fmt.Sprintf("client-%04d", i+1))
		snapshot, err := hub.Attach(clients[i], orchestration.Person(members[i]))
		if err != nil {
			t.Fatalf("Attach(%s): %v", clients[i], err)
		}
		if len(snapshot.Counts) != channelCount {
			t.Fatalf("the first-connect snapshot for %s carried %d counts, want %d",
				clients[i], len(snapshot.Counts), channelCount)
		}
	}

	// A client subscribes to the membership of the channel it is in, which is
	// what the record says a client does.
	for i := 0; i < inA; i++ {
		subscribePresence(t, hub, clients[i], a)
	}
	for i := inA; i < inA+inB; i++ {
		subscribePresence(t, hub, clients[i], b)
	}

	// One person moves from A to B.
	mover := members[0]
	applyPresence(t, hub, mover, 2, b, presenceAt(100))

	deliveries := hub.Flush()

	// The bound, derived from the model rather than restated as a number: at
	// most one message per client per interval, whatever happened in it.
	if len(deliveries) > clientCount {
		t.Fatalf("one move produced %d messages for %d clients, which breaks the one-per-client bound",
			len(deliveries), clientCount)
	}
	// And the worked count, which is every client because every channel is
	// viewable by everybody here.
	if len(deliveries) != clientCount {
		t.Fatalf("one move produced %d messages, want %d", len(deliveries), clientCount)
	}

	wantCounts := []orchestration.PresenceCountUpdate{
		{Channel: a, Count: inA - 1},
		{Channel: b, Count: inB + 1},
	}
	deltas := 0
	withIdentity := map[orchestration.ClientID]bool{}
	for _, d := range deliveries {
		if d.Message.Snapshot {
			t.Fatalf("%s received a pushed message marked as a snapshot", d.Client)
		}
		if len(d.Message.Counts) != len(wantCounts) {
			t.Fatalf("%s received %d count updates, want %d", d.Client, len(d.Message.Counts), len(wantCounts))
		}
		for i, got := range d.Message.Counts {
			if got != wantCounts[i] {
				t.Fatalf("%s count update %d is %+v, want %+v", d.Client, i, got, wantCounts[i])
			}
		}
		deltas += len(d.Message.Counts)
		if len(d.Message.Members) > 0 {
			withIdentity[d.Client] = true
		}
	}

	if want := 2 * clientCount; deltas != want {
		t.Fatalf("the flush carried %d count deltas, want %d", deltas, want)
	}
	if want := inA + inB; len(withIdentity) != want {
		t.Fatalf("%d messages carried an identity, want %d", len(withIdentity), want)
	}
	for i := 0; i < inA+inB; i++ {
		if !withIdentity[clients[i]] {
			t.Fatalf("%s is subscribed to a membership that changed and received no identity", clients[i])
		}
	}

	// The five who were in A are told the mover left it; the three in B are
	// told who arrived, with the whole published record.
	for i := 0; i < inA; i++ {
		got := membersFor(t, deliveries, clients[i])
		want := []orchestration.PresenceMemberUpdate{{Channel: a, Member: mover, Present: false}}
		assertMemberUpdates(t, clients[i], got, want)
	}
	arrived := orchestration.Presence{
		Channel:   b,
		Member:    mover,
		SinceUnix: presenceAt(100).Unix(),
	}
	for i := inA; i < inA+inB; i++ {
		got := membersFor(t, deliveries, clients[i])
		want := []orchestration.PresenceMemberUpdate{{Channel: b, Member: mover, Present: true, Now: arrived}}
		assertMemberUpdates(t, clients[i], got, want)
	}

	t.Logf("measured: %d channels, %d clients, 1 move: %d messages, %d count deltas, %d messages carrying an identity; the naive design sends %d messages",
		channelCount, clientCount, len(deliveries), deltas, len(withIdentity), 2*clientCount)
}

// TestFiftyMovesInOneIntervalCostTheSameNumberOfMessages is the claim the bound
// is actually for. The record says the message count is bounded by the number
// of clients per interval and not by the number of changes, and fifty people
// moving at the end of a scheduled event is where that stops being a detail:
// one hundred changes is 100,000 messages naively and one per client here.
func TestFiftyMovesInOneIntervalCostTheSameNumberOfMessages(t *testing.T) {
	const (
		channelCount = 200
		clientCount  = 1000
		movers       = 50
	)

	hub := presenceHub(t, seesEverything())
	channels := make([]orchestration.ID, channelCount)
	for i := range channels {
		channels[i] = id(t, fmt.Sprintf("channel-%03d", i+1))
		if err := hub.AddChannel(channels[i]); err != nil {
			t.Fatalf("AddChannel(%s): %v", channels[i], err)
		}
	}

	members := make([]orchestration.ID, clientCount)
	clients := make([]orchestration.ClientID, clientCount)
	for i := range members {
		members[i] = id(t, fmt.Sprintf("member-%04d", i+1))
		clients[i] = orchestration.ClientID(fmt.Sprintf("client-%04d", i+1))
		if _, err := hub.Attach(clients[i], orchestration.Person(members[i])); err != nil {
			t.Fatalf("Attach(%s): %v", clients[i], err)
		}
	}

	// Fifty people each land in a channel of their own, so a hundred channels
	// move inside one interval.
	for i := 0; i < movers; i++ {
		applyPresence(t, hub, members[i], 1, channels[i], presenceAt(i))
	}
	if got := hub.Flush(); len(got) != clientCount {
		t.Fatalf("the first interval produced %d messages, want %d", len(got), clientCount)
	}
	for i := 0; i < movers; i++ {
		applyPresence(t, hub, members[i], 2, channels[movers+i], presenceAt(100+i))
	}

	deliveries := hub.Flush()
	if len(deliveries) != clientCount {
		t.Fatalf("%d moves in one interval produced %d messages, want %d", movers, len(deliveries), clientCount)
	}
	changes := 0
	for _, d := range deliveries {
		changes += len(d.Message.Counts)
	}
	if want := clientCount * movers * 2; changes != want {
		t.Fatalf("the flush carried %d count deltas, want %d", changes, want)
	}
	t.Logf("measured: %d moves in one interval: %d messages carrying %d count deltas; the naive design sends %d messages",
		movers, len(deliveries), changes, movers*2*clientCount)
}

// TestAChannelAViewerCannotSeeIsNotDisclosedAtAll is the third done-when
// condition. A channel you cannot view shows nothing including its existence,
// so there is no count, no placeholder, no message when it changes, and no
// error a client can tell apart from asking about a channel that was never
// created.
//
// A second client that can see the channel is attached throughout. Without it
// this test would pass on a hub that published nothing to anybody.
func TestAChannelAViewerCannotSeeIsNotDisclosedAtAll(t *testing.T) {
	open := id(t, "open")
	closed := id(t, "closed")
	// Never added to the hub, so it is a channel that was never created.
	ghost := id(t, "ghost")

	blind := id(t, "blind-member")
	sighted := id(t, "sighted-member")
	g := presenceGrants{hiddenFrom: map[string]map[orchestration.ID]bool{
		blind.String(): {closed: true},
	}}

	hub := presenceHub(t, g)
	for _, c := range []orchestration.ID{open, closed} {
		if err := hub.AddChannel(c); err != nil {
			t.Fatalf("AddChannel(%s): %v", c, err)
		}
	}

	blindSnapshot, err := hub.Attach("blind", orchestration.Person(blind))
	if err != nil {
		t.Fatalf("Attach(blind): %v", err)
	}
	if len(blindSnapshot.Counts) != 1 || blindSnapshot.Counts[0].Channel != open {
		t.Fatalf("the snapshot for a viewer who cannot see %s carried %+v", closed, blindSnapshot.Counts)
	}
	sightedSnapshot, err := hub.Attach("sighted", orchestration.Person(sighted))
	if err != nil {
		t.Fatalf("Attach(sighted): %v", err)
	}
	if len(sightedSnapshot.Counts) != 2 {
		t.Fatalf("the snapshot for a viewer who can see both carried %+v", sightedSnapshot.Counts)
	}

	// The two refusals are one refusal. Same error and the same message, so
	// nothing distinguishes a channel withheld from one that is not there.
	_, hiddenErr := hub.Subscribe("blind", closed)
	_, ghostErr := hub.Subscribe("blind", ghost)
	for name, err := range map[string]error{"a channel withheld": hiddenErr, "a channel that does not exist": ghostErr} {
		if !errors.Is(err, orchestration.ErrNoSuchChannel) {
			t.Fatalf("subscribing to %s returned %v, want ErrNoSuchChannel", name, err)
		}
	}
	if hiddenErr.Error() != ghostErr.Error() {
		t.Fatalf("the two refusals read differently: %q against %q", hiddenErr, ghostErr)
	}

	// Somebody walks into the channel the first viewer cannot see.
	applyPresence(t, hub, id(t, "walker"), 1, closed, presenceAt(0))
	deliveries := hub.Flush()

	if len(deliveries) != 1 {
		t.Fatalf("a change in a channel one of two viewers can see produced %d messages, want 1", len(deliveries))
	}
	if deliveries[0].Client != "sighted" {
		t.Fatalf("the message went to %s, want sighted", deliveries[0].Client)
	}
	if got := deliveries[0].Message.Counts; len(got) != 1 || got[0].Channel != closed || got[0].Count != 1 {
		t.Fatalf("the sighted viewer received %+v", got)
	}
}

// TestTheProjectionConvergesWhateverOrderReportsArriveIn is the fourth
// done-when condition. Every ordering of the same reports has to leave the same
// list, which is what a register per member buys and what a stream of
// differences could not give without a reordering buffer.
//
// It enumerates every permutation rather than shuffling, so the case that
// breaks it cannot be one this run happened not to draw.
func TestTheProjectionConvergesWhateverOrderReportsArriveIn(t *testing.T) {
	a, b := id(t, "channel-a"), id(t, "channel-b")
	one, two, three := id(t, "member-one"), id(t, "member-two"), id(t, "member-three")

	reports := []orchestration.PresenceReport{
		{Member: one, Seq: 1, Channel: a, Since: presenceAt(1)},
		{Member: one, Seq: 2, Channel: b, Since: presenceAt(2)},
		{Member: two, Seq: 1, Channel: a, Since: presenceAt(3)},
		{Member: two, Seq: 2},
		{Member: three, Seq: 1, Channel: b, Since: presenceAt(4), SelfMuted: true},
	}

	// The state every ordering has to reach: one moved to B, two left, three is
	// in B and has muted themselves.
	wantB := []orchestration.Presence{
		{Channel: b, Member: one, SinceUnix: presenceAt(2).Unix()},
		{Channel: b, Member: three, SinceUnix: presenceAt(4).Unix(), SelfMuted: true},
	}

	orderings := 0
	permute(reports, func(order []orchestration.PresenceReport) {
		orderings++
		p := orchestration.NewPresenceProjection()
		for _, r := range order {
			p.Apply(r)
		}
		if got := p.Count(a); got != 0 {
			t.Fatalf("order %s left %d in channel a, want 0", describe(order), got)
		}
		if got := p.Count(b); got != 2 {
			t.Fatalf("order %s left %d in channel b, want 2", describe(order), got)
		}
		got := p.Occupants(b)
		if len(got) != len(wantB) {
			t.Fatalf("order %s left %d occupants in channel b, want %d", describe(order), len(got), len(wantB))
		}
		for i := range got {
			if got[i] != wantB[i] {
				t.Fatalf("order %s left occupant %d as %s, want %s", describe(order), i, got[i], wantB[i])
			}
		}
		if _, present := p.Where(two); present {
			t.Fatalf("order %s left a member who reported leaving still present", describe(order))
		}
	})
	if orderings != 120 {
		t.Fatalf("the test walked %d orderings of five reports, want 120", orderings)
	}
}

// TestAReportBehindTheOneHeldChangesNothing is the leg the convergence above
// rests on. A late report is the common case rather than an error, and it has
// to be discarded rather than applied, because applying it would move a member
// backwards and no later report would correct it.
func TestAReportBehindTheOneHeldChangesNothing(t *testing.T) {
	a, b := id(t, "channel-a"), id(t, "channel-b")
	who := id(t, "member-one")

	p := orchestration.NewPresenceProjection()
	if _, moved := p.Apply(orchestration.PresenceReport{Member: who, Seq: 2, Channel: b, Since: presenceAt(2)}); !moved {
		t.Fatal("the first report was not applied")
	}
	for _, late := range []orchestration.PresenceReport{
		{Member: who, Seq: 1, Channel: a, Since: presenceAt(1)},
		{Member: who, Seq: 2, Channel: a, Since: presenceAt(1)},
		{Member: who, Seq: 1},
	} {
		if _, moved := p.Apply(late); moved {
			t.Fatalf("a report at sequence %d was applied over one at sequence 2", late.Seq)
		}
	}
	if got := p.Count(a); got != 0 {
		t.Fatalf("channel a holds %d after three late reports, want 0", got)
	}
	if got, present := p.Where(who); !present || got.Channel != b {
		t.Fatalf("the member is at %+v, want channel b", got)
	}
}

// TestAReportThatChangesNothingSendsNothing holds the record's rule that a
// message with no changes in it is not sent. The report is well formed and
// carries a new sequence; it just says what the projection already holds.
func TestAReportThatChangesNothingSendsNothing(t *testing.T) {
	channel := id(t, "channel-a")
	who := id(t, "member-one")

	hub := presenceHub(t, seesEverything())
	if err := hub.AddChannel(channel); err != nil {
		t.Fatalf("AddChannel: %v", err)
	}
	if _, err := hub.Attach("watcher", orchestration.Person(id(t, "watcher-member"))); err != nil {
		t.Fatalf("Attach: %v", err)
	}

	applyPresence(t, hub, who, 1, channel, presenceAt(0))
	if got := hub.Flush(); len(got) != 1 {
		t.Fatalf("the first report produced %d messages, want 1", len(got))
	}
	if hub.Apply(orchestration.PresenceReport{Member: who, Seq: 2, Channel: channel, Since: presenceAt(0)}) {
		t.Fatal("a report identical to the state already held was reported as a change")
	}
	if got := hub.Flush(); len(got) != 0 {
		t.Fatalf("a report that changed nothing produced %d messages, want 0", len(got))
	}
}

// TestASelfMuteMovesNoCountAndReachesOnlySubscribers is the split between the
// two tiers at its narrowest. A self-mute changes nothing a count would show,
// so a client that is only watching counts has nothing to receive.
func TestASelfMuteMovesNoCountAndReachesOnlySubscribers(t *testing.T) {
	channel := id(t, "channel-a")
	who := id(t, "member-one")

	hub := presenceHub(t, seesEverything())
	if err := hub.AddChannel(channel); err != nil {
		t.Fatalf("AddChannel: %v", err)
	}
	for _, c := range []orchestration.ClientID{"counter", "watcher"} {
		if _, err := hub.Attach(c, orchestration.Person(id(t, string(c)+"-member"))); err != nil {
			t.Fatalf("Attach(%s): %v", c, err)
		}
	}
	subscribePresence(t, hub, "watcher", channel)

	applyPresence(t, hub, who, 1, channel, presenceAt(0))
	hub.Flush()

	if !hub.Apply(orchestration.PresenceReport{Member: who, Seq: 2, Channel: channel, Since: presenceAt(0), SelfMuted: true}) {
		t.Fatal("a self-mute was not reported as a change")
	}
	deliveries := hub.Flush()
	if len(deliveries) != 1 || deliveries[0].Client != "watcher" {
		t.Fatalf("a self-mute produced %+v, want one message to the subscriber", deliveries)
	}
	if got := deliveries[0].Message.Counts; len(got) != 0 {
		t.Fatalf("a self-mute carried %d count updates, want 0", len(got))
	}
	got := deliveries[0].Message.Members
	want := []orchestration.PresenceMemberUpdate{{
		Channel: channel,
		Member:  who,
		Present: true,
		Now:     orchestration.Presence{Channel: channel, Member: who, SinceUnix: presenceAt(0).Unix(), SelfMuted: true},
	}}
	assertMemberUpdates(t, "watcher", got, want)
}

// applyPresence folds one report in and fails the test if it changed nothing,
// because every call here is written to change something.
func applyPresence(t *testing.T, h *orchestration.PresenceHub, member orchestration.ID, seq uint64, channel orchestration.ID, since time.Time) {
	t.Helper()
	if !h.Apply(orchestration.PresenceReport{Member: member, Seq: seq, Channel: channel, Since: since}) {
		t.Fatalf("the report putting %s in %s at sequence %d changed nothing", member, channel, seq)
	}
}

func subscribePresence(t *testing.T, h *orchestration.PresenceHub, client orchestration.ClientID, channel orchestration.ID) {
	t.Helper()
	if _, err := h.Subscribe(client, channel); err != nil {
		t.Fatalf("Subscribe(%s, %s): %v", client, channel, err)
	}
}

func membersFor(t *testing.T, deliveries []orchestration.PresenceDelivery, client orchestration.ClientID) []orchestration.PresenceMemberUpdate {
	t.Helper()
	for _, d := range deliveries {
		if d.Client == client {
			return d.Message.Members
		}
	}
	t.Fatalf("no delivery for %s", client)
	return nil
}

func assertMemberUpdates(t *testing.T, client orchestration.ClientID, got, want []orchestration.PresenceMemberUpdate) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s received %d member updates, want %d", client, len(got), len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("%s member update %d is %+v, want %+v", client, i, got[i], want[i])
		}
	}
}

// permute calls fn with every ordering of in, on a fresh slice each time.
func permute(in []orchestration.PresenceReport, fn func([]orchestration.PresenceReport)) {
	order := make([]orchestration.PresenceReport, len(in))
	copy(order, in)
	var walk func(int)
	walk = func(k int) {
		if k == len(order) {
			out := make([]orchestration.PresenceReport, len(order))
			copy(out, order)
			fn(out)
			return
		}
		for i := k; i < len(order); i++ {
			order[k], order[i] = order[i], order[k]
			walk(k + 1)
			order[k], order[i] = order[i], order[k]
		}
	}
	walk(0)
}

// describe names an ordering so a failure says which one broke.
func describe(order []orchestration.PresenceReport) string {
	out := ""
	for _, r := range order {
		out += fmt.Sprintf("[%s@%d]", r.Member, r.Seq)
	}
	return out
}

// TestEveryPresenceRefusalIsReached covers the constructors' and the hub's
// refusals, which the fan-out tests above walk straight past.
//
// A refusal nothing executes is a refusal nobody has read the message of, and
// this surface has eight of them. #93 is the issue that asks for the layer to
// hold no unexecuted path; this is the presence half of that, and it was the
// half the fan-out change left behind.
func TestEveryPresenceRefusalIsReached(t *testing.T) {
	g := seesEverything()

	if _, err := orchestration.NewPresenceHub(orchestration.ID{}, g); !errors.Is(err, orchestration.ErrInvariant) {
		t.Fatalf("a hub with no space returned %v, want ErrInvariant", err)
	}
	if _, err := orchestration.NewPresenceHub(id(t, "space"), nil); !errors.Is(err, orchestration.ErrInvariant) {
		t.Fatalf("a hub with no grantor returned %v, want ErrInvariant", err)
	}

	hub := presenceHub(t, g)
	channel := id(t, "channel-a")

	if err := hub.AddChannel(orchestration.ID{}); !errors.Is(err, orchestration.ErrInvariant) {
		t.Fatalf("adding a channel with no identifier returned %v, want ErrInvariant", err)
	}
	if err := hub.AddChannel(channel); err != nil {
		t.Fatalf("AddChannel: %v", err)
	}
	if err := hub.AddChannel(channel); !errors.Is(err, orchestration.ErrInvariant) {
		t.Fatalf("adding the same channel twice returned %v, want ErrInvariant", err)
	}

	if _, err := hub.Attach("", orchestration.Person(id(t, "someone"))); !errors.Is(err, orchestration.ErrInvariant) {
		t.Fatalf("attaching a client with no identifier returned %v, want ErrInvariant", err)
	}
	if _, err := hub.Attach("nobody", orchestration.Person(orchestration.ID{})); !errors.Is(err, orchestration.ErrInvariant) {
		t.Fatalf("attaching a client with no principal returned %v, want ErrInvariant", err)
	}
	if _, err := hub.Attach("watcher", orchestration.Person(id(t, "watcher-member"))); err != nil {
		t.Fatalf("Attach: %v", err)
	}
	if _, err := hub.Attach("watcher", orchestration.Person(id(t, "watcher-member"))); !errors.Is(err, orchestration.ErrInvariant) {
		t.Fatalf("attaching the same client twice returned %v, want ErrInvariant", err)
	}

	if _, err := hub.Subscribe("stranger", channel); !errors.Is(err, orchestration.ErrNoSuchClient) {
		t.Fatalf("subscribing an unattached client returned %v, want ErrNoSuchClient", err)
	}

	// A member who was never present reporting that they are in no channel is
	// not a change, and it is a report a reconnecting client can genuinely
	// send.
	if hub.Apply(orchestration.PresenceReport{Member: id(t, "ghost-member"), Seq: 1}) {
		t.Fatal("a leave from a member who was never present was reported as a change")
	}
	// A report naming no member at all is refused rather than folded in under
	// the zero identifier.
	if hub.Apply(orchestration.PresenceReport{Seq: 1, Channel: channel, Since: presenceAt(0)}) {
		t.Fatal("a report naming no member was applied")
	}
	if hub.Apply(orchestration.PresenceReport{Member: id(t, "someone"), Channel: channel, Since: presenceAt(0)}) {
		t.Fatal("a report at sequence zero was applied")
	}
	if got := hub.Projection().Count(channel); got != 0 {
		t.Fatalf("three refused reports left %d in the channel, want 0", got)
	}
}

// TestDetachAndUnsubscribeStopTheFanOut covers the two calls that take a client
// back out again. Both are total: neither reports anything, and a caller that
// names something it does not hold is not a fault.
func TestDetachAndUnsubscribeStopTheFanOut(t *testing.T) {
	channel := id(t, "channel-a")
	hub := presenceHub(t, seesEverything())
	if err := hub.AddChannel(channel); err != nil {
		t.Fatalf("AddChannel: %v", err)
	}
	for _, c := range []orchestration.ClientID{"staying", "leaving"} {
		if _, err := hub.Attach(c, orchestration.Person(id(t, string(c)+"-member"))); err != nil {
			t.Fatalf("Attach(%s): %v", c, err)
		}
		subscribePresence(t, hub, c, channel)
	}

	// Neither call reports anything, including for a client and a channel
	// nobody holds.
	hub.Detach("never-attached")
	hub.Unsubscribe("never-attached", channel)
	hub.Unsubscribe("staying", id(t, "channel-never-added"))

	hub.Unsubscribe("staying", channel)
	hub.Detach("leaving")

	applyPresence(t, hub, id(t, "walker"), 1, channel, presenceAt(0))
	deliveries := hub.Flush()
	if len(deliveries) != 1 || deliveries[0].Client != "staying" {
		t.Fatalf("after one detach and one unsubscribe the flush produced %+v, want one message to staying", deliveries)
	}
	if got := deliveries[0].Message.Members; len(got) != 0 {
		t.Fatalf("an unsubscribed client received %d member updates, want 0", len(got))
	}
	if got := deliveries[0].Message.Counts; len(got) != 1 {
		t.Fatalf("the remaining client received %d count updates, want 1", len(got))
	}
}

// TestTwoArrivalsInOneIntervalAreOrderedAndTheListIsOrderedByArrival covers the
// two orderings the fan-out depends on and that a single-member case cannot
// reach: the coalesced updates for one channel, and the occupant list.
//
// The list is longest present first, so a viewer reading it sees the people who
// were already talking above the person who has just walked in, and the order
// does not jump when somebody unrelated arrives.
func TestTwoArrivalsInOneIntervalAreOrderedAndTheListIsOrderedByArrival(t *testing.T) {
	channel := id(t, "channel-a")
	early, late := id(t, "member-early"), id(t, "member-late")

	hub := presenceHub(t, seesEverything())
	if err := hub.AddChannel(channel); err != nil {
		t.Fatalf("AddChannel: %v", err)
	}
	if _, err := hub.Attach("watcher", orchestration.Person(id(t, "watcher-member"))); err != nil {
		t.Fatalf("Attach: %v", err)
	}
	subscribePresence(t, hub, "watcher", channel)

	// Applied late first, so the ordering in the message cannot come from the
	// order the reports arrived in.
	applyPresence(t, hub, late, 1, channel, presenceAt(30))
	applyPresence(t, hub, early, 1, channel, presenceAt(10))

	deliveries := hub.Flush()
	if len(deliveries) != 1 {
		t.Fatalf("two arrivals in one interval produced %d messages, want 1", len(deliveries))
	}
	got := deliveries[0].Message.Members
	want := []orchestration.PresenceMemberUpdate{
		{Channel: channel, Member: early, Present: true, Now: orchestration.Presence{Channel: channel, Member: early, SinceUnix: presenceAt(10).Unix()}},
		{Channel: channel, Member: late, Present: true, Now: orchestration.Presence{Channel: channel, Member: late, SinceUnix: presenceAt(30).Unix()}},
	}
	assertMemberUpdates(t, "watcher", got, want)
	if counts := deliveries[0].Message.Counts; len(counts) != 1 || counts[0].Count != 2 {
		t.Fatalf("two arrivals coalesced to %+v, want one update reading 2", counts)
	}

	occupants := hub.Projection().Occupants(channel)
	if len(occupants) != 2 || occupants[0].Member != early || occupants[1].Member != late {
		t.Fatalf("the occupant list is %v, want the earlier arrival first", occupants)
	}

	// Two people arriving in the same second is the tie the list still has to
	// break, because the published field is whole seconds and two people
	// walking in together is the ordinary case rather than a coincidence.
	together := id(t, "member-alongside")
	applyPresence(t, hub, together, 1, channel, presenceAt(10))
	hub.Flush()
	occupants = hub.Projection().Occupants(channel)
	if len(occupants) != 3 {
		t.Fatalf("the occupant list holds %d, want 3", len(occupants))
	}
	if occupants[0].Member != together || occupants[1].Member != early || occupants[2].Member != late {
		t.Fatalf("the occupant list is %v, want the same-second arrivals in identifier order ahead of the later one", occupants)
	}

	// The rendering used in a failure message, so a broken one is not
	// discovered by a test that was already failing.
	if got, want := occupants[1].String(), fmt.Sprintf("%s in %s since %d muted=false", early, channel, presenceAt(10).Unix()); got != want {
		t.Fatalf("a presence renders as %q, want %q", got, want)
	}
}
