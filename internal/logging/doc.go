// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026  iderex

// Package logging is the one place this server writes a log line.
//
// An operator's logs are the most likely route for a personal communication to
// leave the boundary docs/privacy.md describes, because logs are shipped to
// somewhere else by design and nobody reads them on the way past. So the rule
// is structural rather than advisory: identifiers are logged, and a value a
// person typed cannot reach a log line at all.
//
// What makes that true is the shape of this package's surface rather than a
// convention its callers are asked to keep. Record takes a closed set of
// Events and a closed set of Fields, every Field constructor takes an
// Identifier, a number or a duration, and none of them takes a string. A
// caller holding a channel name, a display name or a decoded payload has
// nothing on this surface to pass it to, so the compiler refuses the call
// rather than the reviewer refusing the change.
//
// The second half of that is Identifier, which is what a value has to become
// before it is a field. NewIdentifier admits local@host and nothing else: no
// space, no control character, no second separator, and a bounded length. That
// is the same grammar docs/decisions/federation.md fixes for an identifier in
// this tree, so a caller who tries to launder free text through it is refused
// by the grammar rather than trusted not to try.
//
// This package depends on nothing else in this module, and
// TestTheLogSurfaceDependsOnNoOtherPackageInThisModule refuses a change to
// that. Every package under internal/ has to be able to log, so a surface that
// imported the domain would be one the domain could not import back. It is
// also why Clock is declared here rather than taken from orchestration or
// auth, which is the same trade those two packages already make with each
// other.
//
// What this package does not do is decide that a log is written at all. It
// writes to an io.Writer it is handed and holds no file, no rotation policy
// and no severity. Where that writer points is the operator's configuration,
// which is issue #66.
package logging
