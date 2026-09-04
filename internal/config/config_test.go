// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026  Nils Lehnen

package config_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/iderex/stammtisch/internal/config"
	"github.com/iderex/stammtisch/internal/orchestration"
)

// valid is one accepted value per key. It is a map rather than a field on the
// table because the table's job is to carry what has to be REFUSED; a valid
// sample beside it would be a second thing to keep in step for no refusal.
//
// TestEveryDeclaredKeyHasAValidSample refuses a key missing from here, so the
// helper below cannot quietly stop covering a key that was added.
var valid = map[string]string{
	"grace-period":    "45s",
	"resume-window":   "10s",
	"resume-breadth":  "8",
	"store-path":      "/var/lib/stammtisch/stammtisch.db",
	"log-destination": "stderr",
}

// everythingValid writes a configuration setting every key to its accepted
// sample, with the named key overridden. An empty override name leaves it
// whole.
func everythingValid(t *testing.T, override, raw string) string {
	t.Helper()
	var b strings.Builder
	for _, s := range config.DeclaredSettings() {
		v, ok := valid[s.Name]
		if !ok {
			t.Fatalf("%s has no accepted sample, so this fixture is not the configuration it claims to be", s.Name)
		}
		if s.Name == override {
			v = raw
		}
		b.WriteString(s.Name + " = " + v + "\n")
	}
	return b.String()
}

func TestEveryDeclaredKeyHasAValidSample(t *testing.T) {
	for _, s := range config.DeclaredSettings() {
		if _, ok := valid[s.Name]; !ok {
			t.Errorf("%s is a declared key with no accepted sample in this suite", s.Name)
		}
	}
	if len(valid) != len(config.DeclaredSettings()) {
		t.Errorf("this suite carries %d samples for %d declared keys, so one of them is for a key that is gone",
			len(valid), len(config.DeclaredSettings()))
	}
}

// TestAConfigurationSettingEveryKeyIsAccepted is the near miss every refusal
// below is measured against. Without it a suite of refusals proves that the
// parser says no and nothing about it saying yes.
func TestAConfigurationSettingEveryKeyIsAccepted(t *testing.T) {
	c, err := config.Parse(everythingValid(t, "", ""))
	if err != nil {
		t.Fatalf("a configuration setting every key was refused: %v", err)
	}
	if c.GracePeriod != 45*time.Second {
		t.Errorf("grace period is %v, want 45s", c.GracePeriod)
	}
	if c.ResumeWindow != 10*time.Second {
		t.Errorf("resume window is %v, want 10s", c.ResumeWindow)
	}
	if c.ResumeBreadth != 8 {
		t.Errorf("resume breadth is %d, want 8", c.ResumeBreadth)
	}
	if c.StorePath != valid["store-path"] {
		t.Errorf("store path is %q, want %q", c.StorePath, valid["store-path"])
	}
	if c.LogDestination != "stderr" {
		t.Errorf("log destination is %q, want stderr", c.LogDestination)
	}
	if len(c.Defaulted) != 0 {
		t.Errorf("a configuration setting every key reported %v as defaulted", c.Defaulted)
	}
}

// TestEverySettingIsReachedByValidation is the first Done-when line of issue
// #66, and it is the reason the faulty value lives in the table rather than in
// this file.
//
// It drives the whole parser rather than the setting's own validator, so what
// it proves is that the value reaches validation at all. A key added to the
// table without a faulty value reds here, which is what makes "validation is
// total" a property of the build and not of somebody's memory.
func TestEverySettingIsReachedByValidation(t *testing.T) {
	for _, s := range config.DeclaredSettings() {
		t.Run(s.Name, func(t *testing.T) {
			if s.Faulty == "" {
				t.Fatalf("%s declares no value it has to refuse, so nothing here shows validation reaches it", s.Name)
			}

			_, err := config.Parse(everythingValid(t, s.Name, s.Faulty))
			if err == nil {
				t.Fatalf("%s = %s was accepted", s.Name, s.Faulty)
			}
			if !errors.Is(err, config.ErrInvalid) {
				t.Errorf("the refusal is %v, and it does not carry ErrInvalid", err)
			}
			for what, want := range map[string]string{
				"the key":           s.Name,
				"the value":         s.Faulty,
				"what it has to be": s.Expected,
			} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("the refusal does not name %s (%q): %v", what, want, err)
				}
			}
		})
	}
}

// TestAKeyWithNoDefaultStopsStartupByName covers the other half of totality: a
// key absent from the file rather than wrong in it.
func TestAKeyWithNoDefaultStopsStartupByName(t *testing.T) {
	required := 0
	for _, s := range config.DeclaredSettings() {
		if s.Fallback != "" {
			continue
		}
		required++

		var b strings.Builder
		for _, other := range config.DeclaredSettings() {
			if other.Name == s.Name {
				continue
			}
			b.WriteString(other.Name + " = " + valid[other.Name] + "\n")
		}

		_, err := config.Parse(b.String())
		if err == nil {
			t.Errorf("a configuration leaving out %s was accepted", s.Name)
			continue
		}
		if !strings.Contains(err.Error(), s.Name) {
			t.Errorf("the refusal for a missing %s does not name it: %v", s.Name, err)
		}
	}
	if required == 0 {
		t.Fatal("no key is required, so this test proved nothing about a key that has to be set")
	}
}

// TestEveryKeyThatTookItsDefaultIsReported is the fourth Done-when line, read at
// the place the report is produced. What the entry point does with the lines is
// its own test.
func TestEveryKeyThatTookItsDefaultIsReported(t *testing.T) {
	var required []string
	for _, s := range config.DeclaredSettings() {
		if s.Fallback == "" {
			required = append(required, fmt.Sprintf("%s = %s", s.Name, valid[s.Name]))
		}
	}

	c, err := config.Parse(strings.Join(required, "\n") + "\n")
	if err != nil {
		t.Fatalf("a configuration setting only the required keys was refused: %v", err)
	}

	report := strings.Join(c.Report(), "\n")
	for _, s := range config.DeclaredSettings() {
		if s.Fallback == "" {
			if strings.Contains(report, s.Name) {
				t.Errorf("%s has no default and the report names it: %s", s.Name, report)
			}
			continue
		}
		if !strings.Contains(report, s.Name) {
			t.Errorf("%s took its default and the report does not name it: %s", s.Name, report)
		}
		if !strings.Contains(report, s.Fallback) {
			t.Errorf("the report names %s without the value it defaulted to: %s", s.Name, report)
		}
	}
	if len(c.Report()) != len(c.Defaulted) {
		t.Errorf("the report has %d lines for %d defaulted keys", len(c.Report()), len(c.Defaulted))
	}
}

// TestAValueThatWasSetIsNotReportedAsDefaulted is the near miss for the test
// above: a report that named every key would pass it.
func TestAValueThatWasSetIsNotReportedAsDefaulted(t *testing.T) {
	c, err := config.Parse(everythingValid(t, "", ""))
	if err != nil {
		t.Fatalf("parsing: %v", err)
	}
	if lines := c.Report(); len(lines) != 0 {
		t.Errorf("a configuration setting every key reported %d defaulted values: %v", len(lines), lines)
	}
}

func TestTheFormatRefusesWhatItCannotRead(t *testing.T) {
	base := everythingValid(t, "", "")
	for name, tc := range map[string]struct {
		text string
		want string
	}{
		"a line that is not a key and a value": {base + "grace-period 30s\n", "grace-period 30s"},
		"a line with nothing before the =":     {base + "= 30s\n", "names no key"},
		"a key with nothing after the =":       {base + "grace-period =\n", "has no value"},
		"a key this build does not have":       {base + "telemetry = on\n", "telemetry"},
		"a key set twice":                      {base + "grace-period = 10s\n", "set again"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := config.Parse(tc.text)
			if err == nil {
				t.Fatalf("%s was accepted", name)
			}
			if !errors.Is(err, config.ErrInvalid) {
				t.Errorf("the refusal is %v, and it does not carry ErrInvalid", err)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("the refusal does not say %q: %v", tc.want, err)
			}
		})
	}
}

// TestTheRefusalForAnUnknownKeyNamesTheKeysThatExist. An operator who wrote a
// key that is not here has a typo or an old document, and both are answered by
// the list rather than by being told the key is wrong.
func TestTheRefusalForAnUnknownKeyNamesTheKeysThatExist(t *testing.T) {
	_, err := config.Parse(everythingValid(t, "", "") + "grace_period = 30s\n")
	if err == nil {
		t.Fatal("an unknown key was accepted")
	}
	for _, s := range config.DeclaredSettings() {
		if !strings.Contains(err.Error(), s.Name) {
			t.Errorf("the refusal does not name the key %s: %v", s.Name, err)
		}
	}
}

func TestCommentsBlankLinesAndCarriageReturnsAreNotConfiguration(t *testing.T) {
	var b strings.Builder
	b.WriteString("# the store is the only key without a default\r\n")
	b.WriteString("\r\n")
	b.WriteString("   \r\n")
	b.WriteString("  store-path = " + valid["store-path"] + "  \r\n")
	b.WriteString("# grace-period = 1s\r\n")

	c, err := config.Parse(b.String())
	if err != nil {
		t.Fatalf("a file of comments, blank lines and one setting was refused: %v", err)
	}
	if c.StorePath != valid["store-path"] {
		t.Errorf("store path is %q, want %q", c.StorePath, valid["store-path"])
	}
	if c.GracePeriod != 30*time.Second {
		t.Errorf("a commented-out grace period was read: %v", c.GracePeriod)
	}
}

// TestEachValidatorSaysWhichMistakeWasMade. The table proves each key is
// reached; this proves the arms inside a validator are told apart, because "it
// is not a duration" and "zero is the feature turned off" send an operator to
// different places.
func TestEachValidatorSaysWhichMistakeWasMade(t *testing.T) {
	for name, tc := range map[string]struct {
		key  string
		raw  string
		want string
	}{
		"a duration that does not parse": {"grace-period", "soon", "not a duration"},
		"a duration of zero":             {"resume-window", "0s", "turned off rather than configured"},
		"a breadth that is not a number": {"resume-breadth", "many", "not a whole number"},
		"a breadth of zero":              {"resume-breadth", "0", "never answer with a difference"},
		"the in-memory database":         {"store-path", ":memory:", "gone when the process is"},
		"a sqlite URI":                   {"store-path", "file:/var/lib/s.db?mode=memory", "mode=memory"},
		"a log file":                     {"log-destination", "/var/log/s.log", "nobody rotates"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := config.Parse(everythingValid(t, tc.key, tc.raw))
			if err == nil {
				t.Fatalf("%s = %s was accepted", tc.key, tc.raw)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("the refusal does not say %q: %v", tc.want, err)
			}
		})
	}
}

// TestTheDefaultsAreTheConstantsTheServerUses. This package writes its defaults
// as the strings an operator would type, which is a second copy of a number
// orchestration already declares. This is what stops the two drifting: a
// default changed here without the constant, or the other way round, reds.
func TestTheDefaultsAreTheConstantsTheServerUses(t *testing.T) {
	window, err := time.ParseDuration(config.DefaultResumeWindow)
	if err != nil {
		t.Fatalf("the default resume window does not parse as one: %v", err)
	}
	if window != orchestration.ResumeWindow {
		t.Errorf("the default resume window is %v and orchestration.ResumeWindow is %v", window, orchestration.ResumeWindow)
	}

	breadth, err := strconv.Atoi(config.DefaultResumeBreadth)
	if err != nil {
		t.Fatalf("the default resume breadth does not parse as a number: %v", err)
	}
	if breadth != orchestration.ResumeMissedChannels {
		t.Errorf("the default resume breadth is %d and orchestration.ResumeMissedChannels is %d", breadth, orchestration.ResumeMissedChannels)
	}

	grace, err := time.ParseDuration(config.DefaultGracePeriod)
	if err != nil {
		t.Fatalf("the default grace period does not parse as one: %v", err)
	}
	if grace != 30*time.Second {
		t.Errorf("the default grace period is %v, and docs/decisions/channel-and-room-model.md derives 30s from the transport", grace)
	}
}

func TestLoadReadsAFileAndRefusesOneItCannotRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stammtisch.conf")
	if err := os.WriteFile(path, []byte(everythingValid(t, "", "")), 0o600); err != nil {
		t.Fatalf("writing the fixture: %v", err)
	}

	c, err := config.Load(path)
	if err != nil {
		t.Fatalf("loading a valid configuration: %v", err)
	}
	if c.LogDestination != "stderr" {
		t.Errorf("log destination is %q, want stderr", c.LogDestination)
	}

	missing := filepath.Join(t.TempDir(), "there-is-no-such-file.conf")
	_, err = config.Load(missing)
	if err == nil {
		t.Fatal("a configuration file that is not there was loaded")
	}
	if !errors.Is(err, config.ErrInvalid) {
		t.Errorf("the refusal is %v, and it does not carry ErrInvalid", err)
	}
	if !strings.Contains(err.Error(), missing) {
		t.Errorf("the refusal does not name the file it could not read: %v", err)
	}
}

// TestTheExampleConfigurationIsOneThisBuildAccepts. The example is what an
// operator copies, so an example that has drifted from the key set is a first
// run that fails for a reason the operator did not cause.
func TestTheExampleConfigurationIsOneThisBuildAccepts(t *testing.T) {
	const example = "../../docs/stammtisch.example.txt"

	b, err := os.ReadFile(example)
	if err != nil {
		t.Fatalf("reading %s: %v", example, err)
	}
	c, err := config.Parse(string(b))
	if err != nil {
		t.Fatalf("the example configuration is refused by this build: %v", err)
	}
	if len(c.Defaulted) != 0 {
		t.Errorf("the example leaves %v to their defaults, and an example an operator copies should show every key", c.Defaulted)
	}
	for _, s := range config.DeclaredSettings() {
		if !strings.Contains(string(b), s.Name) {
			t.Errorf("the example does not carry the key %s", s.Name)
		}
	}
}
