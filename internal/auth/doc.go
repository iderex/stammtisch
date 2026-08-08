// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026  iderex

// Package auth holds the credential a server stores, the comparison it makes
// against one presented to it, and the session that comparison opens.
//
// The server never holds a credential in a form it could return. What it holds
// is the output of a memory-hard function over the credential and a random
// salt, written out with the parameters that produced it, so a stored value
// carries its own cost and an operator who raises the cost does not invalidate
// what is already stored.
//
// It holds a token the same way. What the session store is keyed on is a digest
// of the token rather than the token, so a copy of the table is not a set of
// working tokens and nothing here can hand a token back to anybody.
//
// Where accounts and sessions are kept is not decided here. Accounts arrive
// through an interface with one method, and the session store is in memory and
// says so; issue #27 is where durability is chosen.
package auth
