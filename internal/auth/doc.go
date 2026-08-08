// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026  iderex

// Package auth holds the credential a server stores and the comparison it makes
// against one presented to it.
//
// What is here is storage and verification only. Sessions, tokens and their
// revocation are the rest of issue #28 and are not in this package yet, so
// nothing here decides whether a connection may proceed.
//
// The server never holds a credential in a form it could return. What it holds
// is the output of a memory-hard function over the credential and a random
// salt, written out with the parameters that produced it, so a stored value
// carries its own cost and an operator who raises the cost does not invalidate
// what is already stored.
package auth
