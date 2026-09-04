// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026  Nils Lehnen

// Command stammtisch is the server.
//
// What it does today is the first thing an operator meets: it reads one
// configuration file, validates it in full, and stops with a message naming the
// key, the value and what was expected if it cannot be used. A value that took
// its default is reported on stdout so it is in the output an operator pastes
// when they ask for help. Nothing is served yet, and the last line says so
// rather than leaving a reader to conclude from silence that something is
// running.
//
// The report is written here rather than through internal/logging, and that is
// a decision rather than a shortcut. Where the log writes is one of the
// configuration's own keys, so at the moment the report exists there is no log
// to write it to. internal/config's package comment carries the argument.
//
// Where this file goes when the tree gains a cmd/ directory is issue #14's
// leftover, which docs/layout.md records.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/iderex/stammtisch/internal/config"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// A reporter is one output stream and the first error writing to it.
//
// It is here because the alternative is checking four writes and doing the same
// thing after each, and because a process whose own startup output does not
// arrive has not started successfully however valid its configuration was. The
// first error is kept rather than the last: what went wrong first is what a
// reader needs, and every write after it goes into the same broken pipe.
type reporter struct {
	w   io.Writer
	err error
}

// say writes one line. The newline is added here rather than by every caller,
// so a caller cannot produce a line without one.
func (r *reporter) say(format string, a ...any) {
	if r.err != nil {
		return
	}
	_, r.err = fmt.Fprintf(r.w, format+"\n", a...)
}

// run is main with its edges passed in, so the suite can assert on what an
// operator sees rather than on what a process did.
//
// It returns the exit status. Every refusal goes to stderr and every report to
// stdout, so an operator redirecting one still has the other.
func run(args []string, stdout, stderr io.Writer) int {
	out := &reporter{w: stdout}
	problem := &reporter{w: stderr}

	flags := flag.NewFlagSet("stammtisch", flag.ContinueOnError)
	flags.SetOutput(stderr)
	path := flags.String("config", "", "path to the configuration file; there is no default and no built-in configuration")

	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *path == "" {
		problem.say("stammtisch: -config names the configuration file, and there is no built-in one to fall back to")
		return 2
	}

	cfg, err := config.Load(*path)
	if err != nil {
		// The message is printed as it was written rather than summarised here.
		// It already names the key, the value and what was expected, and a
		// second sentence about it would be this file's guess at which of the
		// three the operator needs.
		problem.say("stammtisch: %v", err)
		return 1
	}

	for _, line := range cfg.Report() {
		out.say("stammtisch: %s", line)
	}
	out.say("stammtisch: the configuration is valid, and nothing is served yet")

	if out.err != nil {
		// A valid configuration whose report reached nobody is not a successful
		// start, and saying so on the other stream is the only thing left that
		// can be done about it.
		problem.say("stammtisch: the configuration is valid and the report did not reach stdout: %v", out.err)
		return 1
	}
	return 0
}
