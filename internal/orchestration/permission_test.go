// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026  iderex

package orchestration_test

import (
	"testing"

	"github.com/iderex/stammtisch/internal/orchestration"
)

// grants is a Grantor built from a table. It is the whole of what a store owes
// the permission model, which is the point: #27 can put the rows anywhere and
// the model does not learn about it.
type grants struct {
	// keyed by principal identifier, then by space and channel written out.
	rows map[string]orchestration.PermissionSet
}

func (g grants) Granted(p orchestration.Principal, s orchestration.Subject) orchestration.PermissionSet {
	channel, _ := s.Channel()
	return g.rows[p.ID().String()+"|"+s.Space().String()+"|"+channel.String()]
}

func mustID(t *testing.T, s string) orchestration.ID {
	t.Helper()
	id, err := orchestration.ParseID(s)
	if err != nil {
		t.Fatalf("parsing %q: %v", s, err)
	}
	return id
}

// fixture returns a space, a channel, and a grantor holding the permissions
// named, for both a person and a bot under the same identifier local part.
func fixture(t *testing.T, held ...orchestration.Permission) (orchestration.ID, orchestration.ID, grants) {
	t.Helper()
	space := mustID(t, "space@example.test")
	channel := mustID(t, "general@example.test")
	who := mustID(t, "member@example.test")

	set := orchestration.NewPermissionSet(held...)
	return space, channel, grants{rows: map[string]orchestration.PermissionSet{
		who.String() + "|" + space.String() + "|" + channel.String(): set,
		who.String() + "|" + space.String() + "|":                    set,
	}}
}

func member(t *testing.T) orchestration.ID { return mustID(t, "member@example.test") }

// TestAnUndeclaredPermissionIsRefused is the totality condition on #29. The
// default arm of the switch in Allow is a refusal, so a value nobody declared
// cannot be answered by whatever case happens to sit last.
//
// The zero value is in the table because it is the one an undeclared variable
// carries, which makes it the value a forgotten assignment produces.
func TestAnUndeclaredPermissionIsRefused(t *testing.T) {
	space, channel, g := fixture(t,
		orchestration.SeeChannel,
		orchestration.JoinChannel,
		orchestration.SpeakInChannel,
		orchestration.MuteMember,
		orchestration.MoveMember,
		orchestration.ManageChannel,
		orchestration.ManageSpace,
	)
	who := orchestration.Person(member(t))

	for _, perm := range []orchestration.Permission{
		orchestration.Permission(0),
		orchestration.Permission(-1),
		orchestration.Permission(99),
	} {
		if orchestration.Allow(g, who, perm, orchestration.ChannelSubject(space, channel)) {
			t.Errorf("Allow said yes to %v on a channel, and it is not a permission this model declares", perm)
		}
		if orchestration.Allow(g, who, perm, orchestration.SpaceSubject(space)) {
			t.Errorf("Allow said yes to %v on a space, and it is not a permission this model declares", perm)
		}
	}
}

// TestAnUndeclaredPermissionCannotBeGranted closes the other half of the same
// hole. Refusing an undeclared question is worth little if an undeclared value
// can sit in a grant set waiting to match one.
func TestAnUndeclaredPermissionCannotBeGranted(t *testing.T) {
	space := mustID(t, "space@example.test")
	channel := mustID(t, "general@example.test")
	who := orchestration.Person(member(t))

	set := orchestration.NewPermissionSet(orchestration.Permission(99), orchestration.SeeChannel)
	g := grants{rows: map[string]orchestration.PermissionSet{
		who.ID().String() + "|" + space.String() + "|" + channel.String(): set,
	}}

	if orchestration.Allow(g, who, orchestration.Permission(99), orchestration.ChannelSubject(space, channel)) {
		t.Error("an undeclared permission was granted and then allowed")
	}
	if !orchestration.Allow(g, who, orchestration.SeeChannel, orchestration.ChannelSubject(space, channel)) {
		t.Error("the declared permission in the same set was dropped along with the undeclared one")
	}
}

// TestAPrincipalWhoCannotSeeAChannelCanDoNothingInIt is the gate the presence
// decision requires. A channel you cannot view shows nothing, including its
// existence, and a principal who could join or moderate one they cannot see has
// learned it exists by trying.
func TestAPrincipalWhoCannotSeeAChannelCanDoNothingInIt(t *testing.T) {
	// Everything except the one that gates the rest.
	space, channel, g := fixture(t,
		orchestration.JoinChannel,
		orchestration.SpeakInChannel,
		orchestration.MuteMember,
		orchestration.MoveMember,
		orchestration.ManageChannel,
	)
	subject := orchestration.ChannelSubject(space, channel)

	for _, perm := range orchestration.Permissions() {
		if perm == orchestration.ManageSpace {
			continue
		}
		for _, who := range []orchestration.Principal{
			orchestration.Person(member(t)),
			orchestration.Bot(member(t)),
		} {
			if orchestration.Allow(g, who, perm, subject) {
				t.Errorf("%v was allowed to a principal holding it but not see-channel", perm)
			}
		}
	}
}

// TestSeeChannelOnItsOwnAllowsOnlySeeing is the other direction. The gate lets
// the rest through, it does not stand in for them.
func TestSeeChannelOnItsOwnAllowsOnlySeeing(t *testing.T) {
	space, channel, g := fixture(t, orchestration.SeeChannel)
	subject := orchestration.ChannelSubject(space, channel)
	who := orchestration.Person(member(t))

	if !orchestration.Allow(g, who, orchestration.SeeChannel, subject) {
		t.Error("see-channel was granted and refused")
	}
	for _, perm := range orchestration.Permissions() {
		if perm == orchestration.SeeChannel || perm == orchestration.ManageSpace {
			continue
		}
		if orchestration.Allow(g, who, perm, subject) {
			t.Errorf("%v was allowed off a see-channel grant alone", perm)
		}
	}
}

// TestABotIsEvaluatedByTheSameFunctionAsAPerson is the condition #29 states in
// those words. Same identifier, same grants, same subject, every declared
// permission, and the two answers have to agree.
//
// It is a real check rather than a restatement because Person and Bot do build
// different values: the kind field distinguishes them and a branch on it would
// change this result. The test is what says no branch exists.
func TestABotIsEvaluatedByTheSameFunctionAsAPerson(t *testing.T) {
	for _, held := range [][]orchestration.Permission{
		{},
		{orchestration.SeeChannel},
		{orchestration.SeeChannel, orchestration.JoinChannel},
		{orchestration.SeeChannel, orchestration.SpeakInChannel, orchestration.MuteMember},
		{orchestration.ManageSpace},
		orchestration.Permissions(),
	} {
		space, channel, g := fixture(t, held...)
		person := orchestration.Person(member(t))
		bot := orchestration.Bot(member(t))

		for _, subject := range []orchestration.Subject{
			orchestration.ChannelSubject(space, channel),
			orchestration.SpaceSubject(space),
		} {
			for _, perm := range orchestration.Permissions() {
				forPerson := orchestration.Allow(g, person, perm, subject)
				forBot := orchestration.Allow(g, bot, perm, subject)
				if forPerson != forBot {
					t.Errorf("holding %v: %v on %v gave %t for a person and %t for a bot",
						held, perm, subject.Space(), forPerson, forBot)
				}
			}
		}
	}
}

// TestAQuestionAboutTheWrongShapeOfSubjectIsRefused covers the arm that has no
// correct answer. Guessing here is how a space-wide grant eventually answers a
// question about one channel.
func TestAQuestionAboutTheWrongShapeOfSubjectIsRefused(t *testing.T) {
	space, channel, g := fixture(t, orchestration.Permissions()...)
	who := orchestration.Person(member(t))

	for _, perm := range orchestration.Permissions() {
		if perm == orchestration.ManageSpace {
			continue
		}
		if orchestration.Allow(g, who, perm, orchestration.SpaceSubject(space)) {
			t.Errorf("%v is about a channel and was allowed on a subject naming none", perm)
		}
	}
	if orchestration.Allow(g, who, orchestration.ManageSpace, orchestration.ChannelSubject(space, channel)) {
		t.Error("manage-space is about a space and was allowed on a subject naming a channel")
	}
	if !orchestration.Allow(g, who, orchestration.ManageSpace, orchestration.SpaceSubject(space)) {
		t.Error("manage-space was granted on the space and refused there")
	}
}

// TestAnIncompleteQuestionIsRefused covers the values a caller reaches Allow
// with when something upstream failed and was not checked. A nil grantor is the
// one that matters most: a decision taken with nothing to read has to be no.
func TestAnIncompleteQuestionIsRefused(t *testing.T) {
	space, channel, g := fixture(t, orchestration.Permissions()...)
	subject := orchestration.ChannelSubject(space, channel)
	who := orchestration.Person(member(t))

	if orchestration.Allow(nil, who, orchestration.SeeChannel, subject) {
		t.Error("a nil grantor allowed a permission")
	}
	if orchestration.Allow(g, orchestration.Person(orchestration.ID{}), orchestration.SeeChannel, subject) {
		t.Error("a principal with the zero identifier was allowed a permission")
	}
	if orchestration.Allow(g, who, orchestration.SeeChannel, orchestration.ChannelSubject(orchestration.ID{}, channel)) {
		t.Error("a subject with no space was allowed a permission")
	}
}

// TestAGrantorThatKnowsNothingRefuses covers the zero PermissionSet, which is
// what a store returns for a principal it has no rows for. It has a nil map
// inside it and has to answer no rather than panic.
func TestAGrantorThatKnowsNothingRefuses(t *testing.T) {
	space := mustID(t, "space@example.test")
	channel := mustID(t, "general@example.test")
	empty := grants{}

	for _, perm := range orchestration.Permissions() {
		if orchestration.Allow(empty, orchestration.Person(member(t)), perm, orchestration.ChannelSubject(space, channel)) {
			t.Errorf("%v was allowed by a grantor holding no rows at all", perm)
		}
	}
}

// TestEveryDeclaredPermissionIsReachable is the totality condition read from
// the other end. Each permission, granted alongside the see-channel gate, has
// to come back allowed. A constant added to the model and left out of the
// switch in Allow falls into the default arm and reds here.
func TestEveryDeclaredPermissionIsReachable(t *testing.T) {
	for _, perm := range orchestration.Permissions() {
		space, channel, g := fixture(t, orchestration.SeeChannel, perm)
		subject := orchestration.ChannelSubject(space, channel)
		if perm == orchestration.ManageSpace {
			subject = orchestration.SpaceSubject(space)
		}
		if !orchestration.Allow(g, orchestration.Person(member(t)), perm, subject) {
			t.Errorf("%v was granted on the subject it is about and Allow refused it, "+
				"which is what an unnamed case in the switch looks like", perm)
		}
	}
}

// TestPermissionNamesAreDistinctAndUndeclaredValuesSaySo keeps the strings
// usable in a refusal message. Two permissions sharing a name make a log line
// that reads correctly and names the wrong rule.
func TestPermissionNamesAreDistinctAndUndeclaredValuesSaySo(t *testing.T) {
	seen := map[string]orchestration.Permission{}
	for _, perm := range orchestration.Permissions() {
		name := perm.String()
		if name == "" {
			t.Errorf("permission %d has no name", int(perm))
		}
		if other, taken := seen[name]; taken {
			t.Errorf("permissions %d and %d are both called %q", int(other), int(perm), name)
		}
		seen[name] = perm
	}
	if got := orchestration.Permission(99).String(); got != "undeclared-permission(99)" {
		t.Errorf("an undeclared permission named itself %q", got)
	}
}
