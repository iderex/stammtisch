// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026  Nils Lehnen

// Package media holds the media plane port: one interface, specified in
// docs/decisions/media-plane-port.md, through which the engine underneath is
// reachable and no other way.
//
// Its implementations sit in directories beside this one rather than in it. The
// in-memory fake is internal/media/fake and arrived with issue #36; the binding
// to the chosen unit is issue #40 and does not exist.
//
// Nothing here decodes, mixes, transcodes or reads a payload. That is not an
// omission waiting to be filled in: the property that the server never looks
// inside the payload is what the per-person volume decision rests on.
//
// The interface is in port.go, and it is the twelve operations that record
// fixes with their preconditions, their postconditions and their error sets.
// Two paragraphs stood here until it landed. One said the declaration was
// missing and that issue #36 recorded the absence; that absence is closed. The
// other said an earlier version of this comment sent a reader to issue #26,
// which is `Model the domain in code`, closed, and whose one line naming the
// media plane asks for the opposite of a declaration here: that nothing in the
// domain types imports this package. That correction is kept, because the wrong
// pointer is the thing a reader might remember.
package media
