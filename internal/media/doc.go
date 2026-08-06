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
// The interface itself is issue #26. Nothing is in it yet.
package media
