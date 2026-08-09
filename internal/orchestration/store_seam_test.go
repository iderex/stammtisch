// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026  iderex

package orchestration_test

import (
	"context"
	"testing"

	"github.com/iderex/stammtisch/internal/orchestration"
	"github.com/iderex/stammtisch/internal/store"
)

const (
	storePrefix  = modulePath + "/internal/store"
	durablePkg   = storePrefix + "/sqlite"
	driverModule = "modernc.org/sqlite"
)

// TestTheOrchestrationSuiteRunsAgainstAStoreWithNoDriver is the fourth
// condition on issue #27, and it is two claims rather than one.
//
// The first is that this layer holds a real store rather than a table written
// for the occasion: store.Memory is an implementation of the port the durable
// store implements, and the contract suite beside that one runs the same
// assertions against both. The second is the graph, below.
func TestTheOrchestrationSuiteRunsAgainstAStoreWithNoDriver(t *testing.T) {
	ctx := context.Background()
	held := store.NewMemory()

	space, err := orchestration.NewID("space", "example.test")
	if err != nil {
		t.Fatalf("NewID: %v", err)
	}
	general, err := orchestration.NewID("general", "example.test")
	if err != nil {
		t.Fatalf("NewID: %v", err)
	}
	ada, err := orchestration.NewID("ada", "example.test")
	if err != nil {
		t.Fatalf("NewID: %v", err)
	}

	subject := orchestration.ChannelSubject(space, general)
	person := orchestration.Person(ada)

	if orchestration.Allow(held, person, orchestration.JoinChannel, subject) {
		t.Fatal("an empty store allowed join-channel")
	}
	if err := held.Grant(ctx, ada, subject, orchestration.SeeChannel, orchestration.JoinChannel); err != nil {
		t.Fatalf("Grant: %v", err)
	}
	if !orchestration.Allow(held, person, orchestration.JoinChannel, subject) {
		t.Error("join-channel was refused after being granted with see-channel")
	}
}

// TestOrchestrationDoesNotReachADatabaseDriver is the graph half. The port is
// importable here; the durable implementation and the driver underneath it are
// not, in the package or in its test binary.
//
// This is what "runs without the store" has to mean to be worth anything. A
// suite that linked a driver would still pass on a bare runner today and would
// start needing a file, a lock and a schema the first time somebody reached for
// the convenient implementation, and the cost would land on whoever ran the
// suite next rather than on whoever wrote that line.
func TestOrchestrationDoesNotReachADatabaseDriver(t *testing.T) {
	deps := dependencyGraph(t, true, selfPackagePath+"/...")
	requirePresent(t, deps, selfPackagePath)
	// The port is in the graph because the test above holds one. Asserting
	// that first is what makes the two absences below absences rather than a
	// graph that happens to carry no store packages at all.
	requirePresent(t, deps, storePrefix)
	requireAbsent(t, deps, durablePkg,
		"the orchestration suite must run against the port and not against the durable store")
	requireAbsent(t, deps, driverModule,
		"the orchestration suite must not link a database driver")
}
