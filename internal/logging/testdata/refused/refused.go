// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026  iderex

// Command refused is the deliberate attempt. It holds a channel name, which is
// free text a person typed, and tries to log it.
//
// It is expected not to compile, and TestTheCompilerRefusesFreeText next door
// builds it and asserts that. It lives under testdata so the go tool leaves it
// out of every pattern that would otherwise try to build it.
package main

import "github.com/iderex/stammtisch/internal/logging"

func main() {
	channelName := "Geburtstag von Nils"
	_ = logging.Channel(channelName)
}
