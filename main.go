// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026  Nils Lehnen

// Command stammtisch is the server. It does nothing yet.
//
// This file exists so that the language decision in
// docs/decisions/server-language.md is something a toolchain can be run
// against rather than a sentence. It is replaced by the real entry point, and
// where it goes when the tree gains directories is issue #14.
package main

import (
	"fmt"
	"os"
)

func main() {
	if _, err := fmt.Fprintln(os.Stdout, "stammtisch: nothing is implemented yet"); err != nil {
		os.Exit(1)
	}
}
