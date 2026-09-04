// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026  Nils Lehnen

package config

// DeclaredSetting is one row of the key table, as the external suite reads it.
//
// The table itself stays unexported. What the suite needs is the name, the
// value that has to be refused and whether the key has a default, and handing
// it those three rather than the row keeps `apply` out of reach: a suite that
// could call the validator directly would prove the function refuses and never
// that the parser reaches it, which is the whole of the totality claim.
type DeclaredSetting struct {
	Name     string
	Expected string
	Fallback string
	Faulty   string
}

// DeclaredSettings is every key this build has, in declared order.
func DeclaredSettings() []DeclaredSetting {
	out := make([]DeclaredSetting, 0, len(settings))
	for _, s := range settings {
		out = append(out, DeclaredSetting{
			Name:     s.name,
			Expected: s.expected,
			Fallback: s.fallback,
			Faulty:   s.faulty,
		})
	}
	return out
}
