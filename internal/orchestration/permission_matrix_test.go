// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026  Nils Lehnen

package orchestration_test

import (
	"fmt"
	"sort"
	"testing"

	"github.com/iderex/stammtisch/internal/orchestration"
)

// The permission matrix.
//
// A matrix is where partial coverage hides most comfortably: half the cells
// tested reads identically to all of them in a passing run. So the cases here
// are not written out. They are generated from orchestration.Permissions(),
// crossed with every grant state, every subject shape and both principal kinds,
// and the suite refuses to run at all if a permission has no declared
// expectation. Adding a permission to the model therefore adds cases, and
// adding one without saying what it should do reds the suite instead of passing
// untested.
//
// What is written out by hand is the expectation, and only the expectation.
// Deriving it from the same rules Allow applies would make every cell agree
// with itself and the suite would prove nothing, so each answer below is a
// literal true or false somebody had to mean.

// grantState is the set of permissions the principal holds, described rather
// than enumerated, so one name covers every permission the loop is currently
// on.
type grantState int

const (
	// nothingGranted is the empty grant set. It is the state a store returns
	// for a principal it has no rows for, which is the state that matters
	// most: it is what a misconfiguration looks like.
	nothingGranted grantState = iota
	// seeOnly is the see-channel gate and nothing else.
	seeOnly
	// theOneAsked is the permission under test and nothing else, so the gate
	// is absent.
	theOneAsked
	// seeAndTheOneAsked is the gate plus the permission under test.
	seeAndTheOneAsked
	// everything is every declared permission at once.
	everything
)

func (g grantState) String() string {
	switch g {
	case nothingGranted:
		return "nothing granted"
	case seeOnly:
		return "see-channel only"
	case theOneAsked:
		return "the permission asked for, without see-channel"
	case seeAndTheOneAsked:
		return "see-channel and the permission asked for"
	case everything:
		return "every declared permission"
	default:
		return fmt.Sprintf("undeclared grant state(%d)", int(g))
	}
}

// grantStates is every state a case is generated for. A state added here
// widens every expectation below, and an expectation that does not cover it
// reds in TestEveryDeclaredPermissionHasAnExpectationForEveryCase.
func grantStates() []grantState {
	return []grantState{nothingGranted, seeOnly, theOneAsked, seeAndTheOneAsked, everything}
}

// subjectShape is what the question is asked about.
type subjectShape int

const (
	aboutAChannel subjectShape = iota
	aboutASpace
)

func (s subjectShape) String() string {
	switch s {
	case aboutAChannel:
		return "a channel"
	case aboutASpace:
		return "a space"
	default:
		return fmt.Sprintf("undeclared subject shape(%d)", int(s))
	}
}

func subjectShapes() []subjectShape { return []subjectShape{aboutAChannel, aboutASpace} }

// expectation is what Allow must answer for one permission, in every state, on
// each shape of subject. Both maps have to be total over grantStates().
type expectation struct {
	onChannel map[grantState]bool
	onSpace   map[grantState]bool
}

// alwaysNo is the expectation for a permission on the subject shape it is not
// about. It is written as a helper rather than repeated because six of the
// seven permissions share it on a space and the seventh shares it on a channel,
// and six copies of the same five false entries is where a typo lives.
func alwaysNo() map[grantState]bool {
	no := map[grantState]bool{}
	for _, state := range grantStates() {
		no[state] = false
	}
	return no
}

// expectations is the declared answer per permission. Every entry was written
// by hand against what the model is supposed to do, not derived from what it
// does.
//
// A permission missing from this map is the failure #37 asks to be caught: the
// suite says so by name rather than generating no case for it and passing.
func expectations() map[orchestration.Permission]expectation {
	return map[orchestration.Permission]expectation{
		// The gate. It answers off its own grant and nothing else gates it.
		orchestration.SeeChannel: {
			onChannel: map[grantState]bool{
				nothingGranted:    false,
				seeOnly:           true,
				theOneAsked:       true, // for this permission the two states are the same set
				seeAndTheOneAsked: true,
				everything:        true,
			},
			onSpace: alwaysNo(),
		},

		// The five behind the gate. Each is refused without see-channel even
		// when it is granted, which is the presence decision's rule that a
		// channel you cannot view shows nothing including its existence.
		orchestration.JoinChannel: {
			onChannel: map[grantState]bool{
				nothingGranted:    false,
				seeOnly:           false,
				theOneAsked:       false,
				seeAndTheOneAsked: true,
				everything:        true,
			},
			onSpace: alwaysNo(),
		},
		orchestration.SpeakInChannel: {
			onChannel: map[grantState]bool{
				nothingGranted:    false,
				seeOnly:           false,
				theOneAsked:       false,
				seeAndTheOneAsked: true,
				everything:        true,
			},
			onSpace: alwaysNo(),
		},
		orchestration.MuteMember: {
			onChannel: map[grantState]bool{
				nothingGranted:    false,
				seeOnly:           false,
				theOneAsked:       false,
				seeAndTheOneAsked: true,
				everything:        true,
			},
			onSpace: alwaysNo(),
		},
		orchestration.MoveMember: {
			onChannel: map[grantState]bool{
				nothingGranted:    false,
				seeOnly:           false,
				theOneAsked:       false,
				seeAndTheOneAsked: true,
				everything:        true,
			},
			onSpace: alwaysNo(),
		},
		orchestration.ManageChannel: {
			onChannel: map[grantState]bool{
				nothingGranted:    false,
				seeOnly:           false,
				theOneAsked:       false,
				seeAndTheOneAsked: true,
				everything:        true,
			},
			onSpace: alwaysNo(),
		},

		// The one permission about a space. It is refused on a channel subject
		// however it is granted, and holding every channel permission does not
		// reach it.
		orchestration.ManageSpace: {
			onChannel: alwaysNo(),
			onSpace: map[grantState]bool{
				nothingGranted:    false,
				seeOnly:           false,
				theOneAsked:       true, // manage-space alone, which is the whole grant it needs
				seeAndTheOneAsked: true,
				everything:        true,
			},
		},
	}
}

// principalKinds is every kind a case is generated for, by constructor. Both
// are in the list rather than one being sampled, so every cell of the matrix is
// run twice and a bot never inherits a person's result.
func principalKinds() []struct {
	name  string
	build func(orchestration.ID) orchestration.Principal
} {
	return []struct {
		name  string
		build func(orchestration.ID) orchestration.Principal
	}{
		{"person", orchestration.Person},
		{"bot", orchestration.Bot},
	}
}

// heldFor turns a grant state into the actual permissions granted, for the
// permission the case is about.
func heldFor(state grantState, perm orchestration.Permission) []orchestration.Permission {
	switch state {
	case nothingGranted:
		return nil
	case seeOnly:
		return []orchestration.Permission{orchestration.SeeChannel}
	case theOneAsked:
		return []orchestration.Permission{perm}
	case seeAndTheOneAsked:
		return []orchestration.Permission{orchestration.SeeChannel, perm}
	case everything:
		return orchestration.Permissions()
	default:
		return nil
	}
}

// TestEveryDeclaredPermissionHasAnExpectationForEveryCase is the condition #37
// leads with. A permission with no declared expectation, or an expectation that
// covers only some of the states, fails here and names what is missing.
//
// It runs before the matrix rather than inside it, because a permission with no
// expectation generates no case, and a suite that silently generates fewer
// cases is exactly the partial coverage this issue is about.
func TestEveryDeclaredPermissionHasAnExpectationForEveryCase(t *testing.T) {
	declared := orchestration.Permissions()
	if len(declared) < 2 {
		t.Fatalf("the model declares %d permissions, which is not the model this suite means to cover", len(declared))
	}

	declaredNames := map[string]bool{}
	for _, perm := range declared {
		declaredNames[perm.String()] = true
	}

	table := expectations()
	for _, perm := range declared {
		want, has := table[perm]
		if !has {
			t.Errorf("the model declares %v and this suite declares no expectation for it, "+
				"so no case would be generated and it would go untested", perm)
			continue
		}
		for _, state := range grantStates() {
			if _, covered := want.onChannel[state]; !covered {
				t.Errorf("%v has no expectation on a channel with %v", perm, state)
			}
			if _, covered := want.onSpace[state]; !covered {
				t.Errorf("%v has no expectation on a space with %v", perm, state)
			}
		}
	}

	// The other direction. An expectation for a permission the model no longer
	// declares is a row that will never run, and it reads in a diff exactly
	// like a row that does.
	for perm := range table {
		if !declaredNames[perm.String()] {
			t.Errorf("this suite declares an expectation for %v and the model does not declare that permission", perm)
		}
	}
}

// TestThePermissionMatrix runs every generated case.
//
// Every permission, against every grant state, against both shapes of subject,
// against both principal kinds. The count is asserted rather than assumed,
// because a loop that silently ran fewer cases is the failure this test exists
// to prevent and it would otherwise look identical to a clean run.
func TestThePermissionMatrix(t *testing.T) {
	space := mustID(t, "space@example.test")
	channel := mustID(t, "general@example.test")
	who := mustID(t, "member@example.test")

	table := expectations()
	ran := 0
	perKind := map[string]int{}

	for _, perm := range orchestration.Permissions() {
		want, has := table[perm]
		if !has {
			// Already reported by name in the test above. Skipping here keeps
			// this test's failure output about wrong answers rather than about
			// missing rows.
			continue
		}
		for _, state := range grantStates() {
			set := orchestration.NewPermissionSet(heldFor(state, perm)...)
			g := grants{rows: map[string]orchestration.PermissionSet{
				who.String() + "|" + space.String() + "|" + channel.String(): set,
				who.String() + "|" + space.String() + "|":                    set,
			}}

			for _, shape := range subjectShapes() {
				subject := orchestration.ChannelSubject(space, channel)
				expected := want.onChannel[state]
				if shape == aboutASpace {
					subject = orchestration.SpaceSubject(space)
					expected = want.onSpace[state]
				}

				for _, kind := range principalKinds() {
					got := orchestration.Allow(g, kind.build(who), perm, subject)
					ran++
					perKind[kind.name]++
					if got != expected {
						t.Errorf("%v for a %s about %v with %v: Allow said %t and the declared expectation is %t",
							perm, kind.name, shape, state, got, expected)
					}
				}
			}
		}
	}

	wantCases := len(orchestration.Permissions()) * len(grantStates()) * len(subjectShapes()) * len(principalKinds())
	if ran != wantCases {
		t.Errorf("ran %d cases and the model, the states, the shapes and the kinds multiply out to %d, "+
			"so some part of the matrix was not generated", ran, wantCases)
	}

	// Every case twice, once per kind, and the two counts equal. This is the
	// difference between a bot in every case and a bot in a sample of them.
	var counts []string
	for name, n := range perKind {
		counts = append(counts, fmt.Sprintf("%s=%d", name, n))
	}
	sort.Strings(counts)
	if len(perKind) != len(principalKinds()) || perKind["person"] != perKind["bot"] || perKind["bot"] == 0 {
		t.Errorf("the kinds did not run the same cases: %v", counts)
	}
}

// TestTheMatrixCoversEveryPermissionTheModelDeclares is the count read from the
// model's own list rather than from a number written here. #37 asks for every
// permission, and "every" is a claim about a list that moves.
func TestTheMatrixCoversEveryPermissionTheModelDeclares(t *testing.T) {
	table := expectations()
	var missing []string
	for _, perm := range orchestration.Permissions() {
		if _, has := table[perm]; !has {
			missing = append(missing, perm.String())
		}
	}
	if len(missing) != 0 {
		sort.Strings(missing)
		t.Errorf("the model declares %d permissions and this suite covers %d; uncovered: %v",
			len(orchestration.Permissions()), len(table), missing)
	}
}
