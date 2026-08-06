// Package orchestration holds signalling, sessions, permissions, presence and
// the state machines that move a member between channels.
//
// It does not import the media plane. Everything it needs from the media plane
// arrives through the port in the sibling package, passed in by whoever wires
// the two together, and the seam is enforced by
// TestOrchestrationDoesNotReachTheMediaPlane rather than asked for in prose.
// That is what keeps the unit suite runnable on a machine with no audio device.
//
// The domain this package models is issue #26. Nothing is in it yet.
package orchestration
