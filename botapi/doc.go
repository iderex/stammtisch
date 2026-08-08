// SPDX-License-Identifier: AGPL-3.0-or-later
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
package botapi
