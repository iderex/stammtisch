// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026  Nils Lehnen

package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iderex/stammtisch/internal/config"
)

// write puts text in a temporary file and returns its path.
func write(t *testing.T, text string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stammtisch.conf")
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatalf("writing the fixture: %v", err)
	}
	return path
}

// good is a configuration this build accepts, with every key set.
const good = "grace-period = 45s\n" +
	"resume-window = 10s\n" +
	"resume-breadth = 8\n" +
	"store-path = /var/lib/stammtisch/stammtisch.db\n" +
	"log-destination = stderr\n"

// TestAnInvalidValueStopsStartupAndTheOperatorSeesWhy is the second Done-when
// line of issue #66 at the place startup happens.
//
// Which keys are validated and what each refusal names is proved over the whole
// key table in internal/config's suite. What is proved here is the half that
// suite cannot see: that a refusal stops the process rather than being written
// past, and that the message reaches an operator as it was written rather than
// being reworded on the way out.
func TestAnInvalidValueStopsStartupAndTheOperatorSeesWhy(t *testing.T) {
	path := write(t, strings.Replace(good, "grace-period = 45s", "grace-period = 0s", 1))

	_, err := config.Load(path)
	if err == nil {
		t.Fatal("the fixture is accepted, so this test is not about a refusal")
	}

	var stdout, stderr bytes.Buffer
	if code := run([]string{"-config", path}, &stdout, &stderr); code == 0 {
		t.Errorf("a configuration this build refuses started the process, exit %d", code)
	}
	if !strings.Contains(stderr.String(), err.Error()) {
		t.Errorf("stderr does not carry the refusal as it was written.\n got: %s\nwant: %s", stderr.String(), err.Error())
	}
	if stdout.Len() != 0 {
		t.Errorf("a refused configuration wrote to stdout: %s", stdout.String())
	}
}

// TestAConfigurationFileThatIsNotThereStopsStartup. A service that starts on
// built-in defaults because its configuration was not where it was told to look
// is one an operator finds out about later.
func TestAConfigurationFileThatIsNotThereStopsStartup(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "there-is-no-such-file.conf")

	var stdout, stderr bytes.Buffer
	if code := run([]string{"-config", missing}, &stdout, &stderr); code == 0 {
		t.Error("a missing configuration file started the process")
	}
	if !strings.Contains(stderr.String(), missing) {
		t.Errorf("stderr does not name the file it could not read: %s", stderr.String())
	}
}

// TestStartingWithNoConfigurationNamedIsRefused. There is no built-in
// configuration, so an operator who forgot the flag is told that rather than
// getting a server configured by nobody.
func TestStartingWithNoConfigurationNamedIsRefused(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run(nil, &stdout, &stderr); code == 0 {
		t.Error("the process started with no configuration named")
	}
	if !strings.Contains(stderr.String(), "-config") {
		t.Errorf("stderr does not name the flag that was missing: %s", stderr.String())
	}
}

func TestAFlagThisBuildDoesNotHaveIsRefused(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"-telemetry"}, &stdout, &stderr); code == 0 {
		t.Error("a flag this build does not have started the process")
	}
	if stdout.Len() != 0 {
		t.Errorf("a refused invocation wrote to stdout: %s", stdout.String())
	}
}

// TestEveryDefaultedValueIsReportedAtStartup is the fourth Done-when line at
// the place an operator reads it. internal/config's suite proves the lines are
// produced; this proves they reach stdout, which is what makes them part of the
// output somebody pastes when they ask for help.
func TestEveryDefaultedValueIsReportedAtStartup(t *testing.T) {
	path := write(t, "store-path = /var/lib/stammtisch/stammtisch.db\n")

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("the fixture is refused, so this test is not about a report: %v", err)
	}
	if len(cfg.Report()) == 0 {
		t.Fatal("the fixture leaves nothing to a default, so this test would pass on a process that reports nothing")
	}

	var stdout, stderr bytes.Buffer
	if code := run([]string{"-config", path}, &stdout, &stderr); code != 0 {
		t.Fatalf("a valid configuration did not start: exit %d, %s", code, stderr.String())
	}
	for _, line := range cfg.Report() {
		if !strings.Contains(stdout.String(), line) {
			t.Errorf("stdout does not carry %q:\n%s", line, stdout.String())
		}
	}
	if stderr.Len() != 0 {
		t.Errorf("a valid configuration wrote to stderr: %s", stderr.String())
	}
}

// TestAValueTheOperatorSetIsNotReportedAsDefaulted is the near miss for the
// test above. A process printing a line per key would pass it.
func TestAValueTheOperatorSetIsNotReportedAsDefaulted(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"-config", write(t, good)}, &stdout, &stderr); code != 0 {
		t.Fatalf("a valid configuration did not start: exit %d, %s", code, stderr.String())
	}
	if strings.Contains(stdout.String(), "uses the default") {
		t.Errorf("a configuration setting every key reported a defaulted value: %s", stdout.String())
	}
}

// TestNothingIsServedAndTheOutputSaysSo. The absence is disclosed rather than
// left to be inferred from a process that exits quietly.
func TestNothingIsServedAndTheOutputSaysSo(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"-config", write(t, good)}, &stdout, &stderr); code != 0 {
		t.Fatalf("a valid configuration did not start: exit %d, %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "nothing is served yet") {
		t.Errorf("the output does not say that nothing is served: %s", stdout.String())
	}
}

// brokenWriter fails every write, which is the stream an operator redirected
// into something that has gone away.
type brokenWriter struct{}

func (brokenWriter) Write([]byte) (int, error) { return 0, errors.New("the stream is gone") }

// TestAReportThatDidNotReachStdoutIsNotASuccessfulStart. A process that
// validated a configuration and then wrote its report into a broken pipe has
// told nobody anything, and exiting zero would report that as a good start.
func TestAReportThatDidNotReachStdoutIsNotASuccessfulStart(t *testing.T) {
	var stderr bytes.Buffer
	if code := run([]string{"-config", write(t, good)}, brokenWriter{}, &stderr); code == 0 {
		t.Error("a start whose whole output went nowhere reported success")
	}
	if !strings.Contains(stderr.String(), "did not reach stdout") {
		t.Errorf("stderr does not say what went wrong: %s", stderr.String())
	}
}
