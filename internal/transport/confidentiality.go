// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026  iderex

package transport

import "net/http"

// A Transit says what the deployment guarantees about the leg between a
// participant and this process. It is an argument to Handler rather than a
// setting read from somewhere, because the arrangement it describes is a
// property of how the operator wired the service up and there is nowhere yet
// for such a thing to be read from. docs/decisions/transport-confidentiality.md
// is where the requirement it enforces is argued.
type Transit int

const (
	// TLSHere is the zero value and refuses a request that did not arrive over
	// TLS. A caller that says nothing about its deployment gets the refusal,
	// which is the direction that matters: the arrangement this whole project
	// is a reaction to is the one nobody chose.
	TLSHere Transit = iota

	// TLSTerminatedAhead declares that something in front of this process
	// terminates TLS and forwards over a link the operator controls, which is
	// how a self-hosted service usually sits behind a reverse proxy.
	//
	// It is a declaration and not a measurement. Nothing here can tell a
	// request forwarded from a proxy on the same host from one that crossed a
	// network in the clear, so this value is the operator asserting the first
	// and being taken at their word. The record above says so in the same
	// words, and the value is named for what it claims rather than for what it
	// switches off so that a reader of a call site sees the claim.
	TLSTerminatedAhead
)

// confidential reports whether r arrived over a connection this deployment
// guarantees.
//
// A Transit that is neither constant behaves as TLSHere, so a value arriving
// from a conversion or from a future constant nobody handled here refuses
// rather than admits.
func (t Transit) confidential(r *http.Request) bool {
	if t == TLSTerminatedAhead {
		return true
	}
	return r.TLS != nil
}
