// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026  Nils Lehnen

package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// ErrInvalid is returned for every configuration this package will not accept.
// One sentinel rather than one per fault: a caller does nothing different for a
// bad duration than for an unknown key, and what it shows an operator is the
// message.
var ErrInvalid = errors.New("config: the configuration is not valid")

// Defaults, written as an operator would write them in the file rather than as
// Go values, so the report at startup and the example configuration quote the
// same string the parser reads.
//
// The two durations repeat numbers orchestration declares as constants.
// TestTheDefaultsAreTheConstantsTheServerUses refuses a drift between them, so
// the repetition is checked rather than trusted.
const (
	DefaultGracePeriod    = "30s"
	DefaultResumeWindow   = "30s"
	DefaultResumeBreadth  = "64"
	DefaultLogDestination = "stdout"
)

// A Config is one validated configuration. Every field is set, from the file or
// from the default, before this package returns one.
type Config struct {
	// GracePeriod is how long a room outlives its last leave.
	GracePeriod time.Duration
	// ResumeWindow is how long the server will accept a client claiming
	// continuity with a suspended session.
	ResumeWindow time.Duration
	// ResumeBreadth is how many changed channels a resume answer names before
	// it gives up naming them and sends the whole state instead.
	ResumeBreadth int
	// StorePath is the file the durable store lives in.
	StorePath string
	// LogDestination is where this process writes its log: stdout or stderr.
	LogDestination string

	// Defaulted names the keys that took their default rather than a value from
	// the file, in the order the keys are declared. Report turns it into the
	// lines the entry point writes at startup.
	Defaulted []string
}

// A setting is one key. The table below is the whole vocabulary: a key not in
// it is refused, and a key in it is validated.
type setting struct {
	// name is the key as it is written in the file.
	name string
	// why is what the value is for, one line. It is written into the example
	// configuration and into the refusal for a required key nobody set.
	why string
	// expected is what a valid value looks like. Every refusal names it, so a
	// message tells an operator what to write and not only that they were
	// wrong.
	expected string
	// fallback is the default, written as it would be in the file. The empty
	// string means the key is required and its absence stops startup.
	fallback string
	// faulty is a value this setting has to refuse. It is what makes the
	// totality claim provable rather than asserted, and the suite walks the
	// table rather than a list of its own.
	faulty string
	// apply validates raw and stores it. It returns the detail that goes after
	// the expectation in a refusal, or nil.
	apply func(c *Config, raw string) error
}

// settings is every key, in the order they are reported and written.
var settings = []setting{
	{
		name:     "grace-period",
		why:      "how long a room is held open after its last member leaves",
		expected: "a positive duration such as 30s",
		fallback: DefaultGracePeriod,
		faulty:   "0s",
		apply: func(c *Config, raw string) error {
			d, err := positiveDuration(raw)
			c.GracePeriod = d
			return err
		},
	},
	{
		name:     "resume-window",
		why:      "how long a dropped client may claim its session back",
		expected: "a positive duration such as 30s",
		fallback: DefaultResumeWindow,
		faulty:   "-5s",
		apply: func(c *Config, raw string) error {
			d, err := positiveDuration(raw)
			c.ResumeWindow = d
			return err
		},
	},
	{
		name:     "resume-breadth",
		why:      "how many changed channels a resume answer names before it sends the whole state instead",
		expected: "a positive whole number",
		fallback: DefaultResumeBreadth,
		faulty:   "0",
		apply: func(c *Config, raw string) error {
			n, err := strconv.Atoi(raw)
			if err != nil {
				return errors.New("it is not a whole number")
			}
			if n <= 0 {
				return errors.New("a register that may name no channel can never answer with a difference")
			}
			c.ResumeBreadth = n
			return nil
		},
	},
	{
		name:     "store-path",
		why:      "the file the durable store lives in",
		expected: "a filesystem path",
		fallback: "",
		faulty:   ":memory:",
		apply: func(c *Config, raw string) error {
			// Both of these are accepted by the driver and neither is the file
			// an operator thinks they named. The first is SQLite's in-memory
			// database, which is gone when the process is. The second carries
			// parameters, mode=memory among them, so a value that reads as a
			// path can be the first one spelled differently. Refusing them here
			// is refusing a store that is silently not durable.
			if raw == ":memory:" {
				return errors.New("that is SQLite's in-memory database, and it is gone when the process is")
			}
			if strings.HasPrefix(raw, "file:") {
				return errors.New("a file: URI carries parameters, mode=memory among them, so it is not a path this program can hold anybody to")
			}
			c.StorePath = raw
			return nil
		},
	},
	{
		name:     "log-destination",
		why:      "where this process writes its log",
		expected: "stdout or stderr",
		fallback: DefaultLogDestination,
		faulty:   "/var/log/stammtisch.log",
		apply: func(c *Config, raw string) error {
			if raw != "stdout" && raw != "stderr" {
				// A path is refused rather than opened. The log surface holds
				// no file, no rotation policy and no severity, so a path here
				// would be a file this program writes forever and never
				// rotates. Under the supervisor an operator already runs, the
				// stream is captured and rotated for them.
				return errors.New("this program holds no file and rotates nothing, so a path would be a log nobody rotates")
			}
			c.LogDestination = raw
			return nil
		},
	},
}

// positiveDuration is the shared arm of the two duration settings. A value that
// parses and is not positive is a different mistake from one that does not
// parse, and an operator is told which.
func positiveDuration(raw string) (time.Duration, error) {
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, errors.New("it is not a duration")
	}
	if d <= 0 {
		return 0, errors.New("a period of zero or less is the feature turned off rather than configured")
	}
	return d, nil
}

// Load reads the configuration at path and validates it.
//
// A file that cannot be read is refused rather than replaced by a default
// configuration. A service that starts on defaults because its configuration
// was not where it was told to look is one an operator finds out about later.
func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("%w: reading %s: %w", ErrInvalid, path, err)
	}
	return Parse(string(b))
}

// Parse validates the text of one configuration file.
//
// It returns on the first fault rather than collecting them all, and the reason
// is how each is read: a list of faults is worked through, and one sentence
// naming a key, a value and an expectation is acted on. Startup wants the
// second.
func Parse(text string) (*Config, error) {
	seen := map[string]int{}
	values := map[string]string{}

	for i, line := range strings.Split(text, "\n") {
		number := i + 1
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, raw, found := strings.Cut(line, "=")
		if !found {
			return nil, fmt.Errorf("%w: line %d is %q, and every line is a key, an = and a value, a # comment, or empty", ErrInvalid, number, line)
		}
		key = strings.TrimSpace(key)
		raw = strings.TrimSpace(raw)

		if key == "" {
			return nil, fmt.Errorf("%w: line %d names no key before its =", ErrInvalid, number)
		}
		if raw == "" {
			return nil, fmt.Errorf("%w: line %d: %q has no value, and a key written with nothing after the = is not the same as a key left out", ErrInvalid, number, key)
		}
		if declaredSetting(key) == nil {
			return nil, fmt.Errorf("%w: line %d: %q is not a key this build has, and the keys are %s", ErrInvalid, number, key, strings.Join(declaredNames(), ", "))
		}
		if first, repeated := seen[key]; repeated {
			return nil, fmt.Errorf("%w: line %d: %q is set again, and it was already set on line %d", ErrInvalid, number, key, first)
		}
		seen[key] = number
		values[key] = raw
	}

	c := &Config{}
	for _, s := range settings {
		raw, set := values[s.name]
		if !set {
			if s.fallback == "" {
				return nil, fmt.Errorf("%w: %q is not set and has no default, and it is %s", ErrInvalid, s.name, s.why)
			}
			raw = s.fallback
			c.Defaulted = append(c.Defaulted, s.name)
		}
		if err := s.apply(c, raw); err != nil {
			return nil, fmt.Errorf("%w: %q is %q, and it has to be %s: %s", ErrInvalid, s.name, raw, s.expected, err)
		}
	}
	return c, nil
}

// Report is what the entry point writes at startup: one line per key that took
// its default rather than a value from the file.
//
// It reports the defaulted keys and not every key, which is what this package's
// issue asks for. A value an operator wrote is one they can read back out of
// their own file; a value they did not write is the one that surprises them.
func (c *Config) Report() []string {
	lines := make([]string, 0, len(c.Defaulted))
	for _, name := range c.Defaulted {
		s := declaredSetting(name)
		lines = append(lines, fmt.Sprintf("configuration: %s is not set, and this run uses the default %s", name, s.fallback))
	}
	return lines
}

// declaredSetting returns the setting called name, or nil.
func declaredSetting(name string) *setting {
	for i := range settings {
		if settings[i].name == name {
			return &settings[i]
		}
	}
	return nil
}

// declaredNames is every key, for the refusal that has to say what was
// expected.
func declaredNames() []string {
	names := make([]string, 0, len(settings))
	for _, s := range settings {
		names = append(names, s.name)
	}
	return names
}
