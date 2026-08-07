// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026  iderex

// Command stammtisch is the server. It does nothing yet.
//
// This file exists so that the language decision in
// docs/decisions/server-language.md is something a toolchain can be run
// against rather than a sentence. It is replaced by the real entry point, and
// where it goes when the tree gains directories is issue #14.
//
// On this branch it also carries a deliberate dependency it has no use for.
// See the pull request: this branch is a proof and is not for merging.
package main

import (
	"fmt"
	"os"

	"github.com/stretchr/testify/assert"
)

func main() {
	if _, err := fmt.Fprintln(os.Stdout, "stammtisch: nothing is implemented yet"); err != nil {
		os.Exit(1)
	}
	if !assert.ObjectsAreEqual(1, 1) {
		os.Exit(1)
	}
}
