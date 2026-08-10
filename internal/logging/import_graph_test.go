// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026  iderex

package logging_test

import (
	"os/exec"
	"strings"
	"testing"
)

const (
	modulePath      = "github.com/iderex/stammtisch"
	selfPackagePath = modulePath + "/internal/logging"
)

// TestTheLogSurfaceDependsOnNoOtherPackageInThisModule holds the property that
// makes one surface possible.
//
// Every package under internal/ has to be able to log, and the greppable
// invariants check refuses a package that writes its own line instead. A
// surface that imported the domain, the store or the transport could not be
// imported back by any of them, so the rule would be one nobody could keep and
// the first package that needed to log would write its own.
//
// It asks the toolchain for the dependency graph rather than reading imports out
// of the source, so a dependency arriving through a third package is caught the
// same as a direct one. It asks for the graph without the test binary on
// purpose: this file's neighbours drive a session through orchestration, auth
// and signalling, which is exactly the direction that would be a cycle if the
// surface itself took it, and an external test package is where Go allows it.
func TestTheLogSurfaceDependsOnNoOtherPackageInThisModule(t *testing.T) {
	args := []string{"list", "-deps", selfPackagePath}
	out, err := exec.Command("go", args...).Output()
	if err != nil {
		var stderr string
		if ee, ok := err.(*exec.ExitError); ok {
			stderr = string(ee.Stderr)
		}
		t.Fatalf("go %s: %v\n%s", strings.Join(args, " "), err, stderr)
	}

	var deps []string
	for _, line := range strings.Split(string(out), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			deps = append(deps, line)
		}
	}
	if len(deps) == 0 {
		t.Fatalf("go %s returned nothing, so this is not the graph the test asked for", strings.Join(args, " "))
	}

	// A guard that passes on an answer about the wrong package is not a guard.
	var found bool
	for _, dep := range deps {
		if dep == selfPackagePath {
			found = true
		}
	}
	if !found {
		t.Fatalf("the graph does not contain %s, so it is not the graph this test asked for", selfPackagePath)
	}

	for _, dep := range deps {
		if dep == selfPackagePath {
			continue
		}
		if strings.HasPrefix(dep, modulePath+"/") {
			t.Errorf("the log surface reaches %s: a package every other one imports cannot import one of them back", dep)
		}
	}
}
