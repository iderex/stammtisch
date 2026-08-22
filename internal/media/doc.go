// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026  Nils Lehnen

// Package media holds the media plane port: one interface, specified in
// docs/decisions/media-plane-port.md, through which the engine underneath is
// reachable and no other way.
//
// Its implementations sit in directories beside this one rather than in it. The
// in-memory fake is issue #36 and the binding to the chosen unit is issue #40;
// neither exists yet.
//
// Nothing here decodes, mixes, transcodes or reads a payload. That is not an
// omission waiting to be filled in: the property that the server never looks
// inside the payload is what the per-person volume decision rests on.
//
// The interface itself is not here, and what is missing is narrower than that
// sounds. Its specification is finished: docs/decisions/media-plane-port.md
// fixes every operation with its preconditions, its postconditions and its
// error set, and issue #3 is closed. What has no issue behind it is the Go
// declaration those operations become. Issue #36 is where that absence is
// recorded, because the fake it asks for has nothing to implement and no method
// set for a check to compare against until the declaration exists, and issue
// #40 waits behind the same line.
//
// This paragraph used to send a reader to issue #26 for that declaration. That
// issue is closed, it is `Model the domain in code`, and the one line in it
// naming the media plane asks for the opposite of a declaration here: that
// nothing in the domain types imports this package.
package media
