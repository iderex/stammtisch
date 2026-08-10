// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026  iderex

package logging_test

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestTheCompilerRefusesFreeText is the deliberate attempt this package's
// second condition asks to be shown refused.
//
// The refusal is the compiler's rather than a reviewer's, so the proof has to
// run one. It builds two fixtures that differ in one argument: the near miss
// passes an identifier and has to build, and the attempt passes a channel name
// and has to fail with a type error naming Identifier. Without the near miss a
// failure here would prove the fixture was broken and nothing about the type.
func TestTheCompilerRefusesFreeText(t *testing.T) {
	out, err := build(t, "./testdata/allowed")
	if err != nil {
		t.Fatalf("the near miss did not build, so nothing below is about the type:\n%s", out)
	}

	out, err = build(t, "./testdata/refused")
	if err == nil {
		t.Fatal("the compiler accepted a channel name where an Identifier is wanted")
	}
	for _, want := range []string{"cannot use", "logging.Identifier"} {
		if !strings.Contains(out, want) {
			t.Errorf("the build failed without %q in its output, so it failed for some other reason:\n%s", want, out)
		}
	}
}

// build compiles one package and returns whatever the toolchain said.
//
// It fails the test rather than returning nothing when go itself cannot be run,
// so an absent toolchain is a red test and not a proof that passed without
// compiling anything.
func build(t *testing.T, pattern string) (string, error) {
	t.Helper()
	cmd := exec.Command("go", "build", "-o", filepath.Join(t.TempDir(), "fixture"), pattern)
	out, err := cmd.CombinedOutput()
	if err != nil && len(out) == 0 {
		t.Fatalf("go build %s could not be run at all: %v", pattern, err)
	}
	return string(out), err
}
