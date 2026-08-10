// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026  iderex

// Command allowed is the near miss. It is the same call as its neighbour, made
// with an identifier instead of a name, and it has to build.
//
// Without it a refusal next door would prove only that the fixture was broken.
// With it, the difference between the two files is the one argument, so what
// the compiler refused is the type and nothing else.
package main

import "github.com/iderex/stammtisch/internal/logging"

func main() {
	channel, err := logging.NewIdentifier("allgemein@stammtisch.example")
	if err != nil {
		return
	}
	_ = logging.Channel(channel)
}
