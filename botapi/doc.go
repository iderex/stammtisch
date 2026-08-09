// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026  iderex

// Package botapi holds the bot API surface, which is a public contract third
// parties write against.
//
// It sits outside internal/ so that it can be imported, and it is its own
// package so that a change to the contract is a diff a reader can judge on its
// own without the server's internals in the way.
//
// It holds the contract and nothing else: no server state, no transport, no
// orchestration. The contract is issue #50. Nothing is in it yet.
//
// This package is under Apache-2.0 and the rest of the repository is under
// AGPL-3.0-or-later. The terms are in botapi/LICENSE, and which paths sit under
// which arm is the list in .github/check-licence-headers.sh, which refuses the
// wrong identifier per path rather than accepting either one anywhere.
package botapi
