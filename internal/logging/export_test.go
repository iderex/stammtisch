// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026  iderex

package logging

// DeclaredFieldNames returns every field name a record may carry, in the order
// Record writes them.
//
// It is here rather than on the surface because nothing a caller does needs it.
// What needs it is the suite next door, which drives a session and asserts that
// the log it produced carries no name outside this set, and that suite is an
// external test package so that a package logging one day can still be one this
// package's tests drive.
func DeclaredFieldNames() []string {
	names := make([]string, 0, len(declaredKeys))
	for _, k := range declaredKeys {
		names = append(names, k.String())
	}
	return names
}
