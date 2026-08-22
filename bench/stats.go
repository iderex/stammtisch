// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026  Nils Lehnen

package bench

import (
	"math"
	"sort"
)

// PercentileMethod names the definition used by Percentile, so a report carries
// it rather than leaving a reader to guess which of the several in circulation
// produced the figure.
const PercentileMethod = "nearest rank on the sorted sample, index = ceil(p/100 * n) - 1"

// Percentile returns the p-th percentile of xs by nearest rank. xs is sorted in
// place. p is in percent. An empty sample returns NaN rather than zero, because
// zero is a plausible delay and a missing measurement is not.
func Percentile(xs []float64, p float64) float64 {
	if len(xs) == 0 {
		return math.NaN()
	}
	sort.Float64s(xs)
	rank := int(math.Ceil(p / 100 * float64(len(xs))))
	if rank < 1 {
		rank = 1
	}
	if rank > len(xs) {
		rank = len(xs)
	}
	return xs[rank-1]
}
