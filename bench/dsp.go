// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026  iderex

package bench

import (
	"math"
	"math/cmplx"
)

// Chirp returns a linear frequency sweep of n samples running from low to high
// hertz at the given sample rate, windowed at both ends.
//
// A sweep is used rather than a tone because the correlation peak has to be
// sharp. A tone of one frequency correlates almost as well one period late as
// it does at the right offset, which puts the peak anywhere within a few
// milliseconds; a sweep correlates with itself only where it lines up, so the
// peak sits on one sample. The window keeps the ends from clicking, which would
// otherwise be a broadband transient the correlator could lock onto instead of
// the sweep.
func Chirp(n, rate int, low, high float64) []float64 {
	c := make([]float64, n)
	if n <= 0 {
		return c
	}
	dur := float64(n) / float64(rate)
	k := (high - low) / dur
	for i := range c {
		t := float64(i) / float64(rate)
		phase := 2 * math.Pi * (low*t + k*t*t/2)
		// Hann window over the whole sweep.
		w := 0.5 * (1 - math.Cos(2*math.Pi*float64(i)/float64(n-1)))
		c[i] = w * math.Sin(phase)
	}
	return c
}

// fft replaces x with its discrete Fourier transform in place. len(x) must be a
// power of two. Passing inverse computes the unnormalised inverse transform.
//
// This is an iterative radix-2 Cooley-Tukey transform. It is here rather than
// behind a dependency because it is forty lines, because the module declares no
// requirements today, and because a numerical routine the rig's own accuracy
// claim rests on is worth being able to read in the tree that claims it.
func fft(x []complex128, inverse bool) {
	n := len(x)
	if n <= 1 {
		return
	}
	// Bit-reversal permutation.
	for i, j := 1, 0; i < n; i++ {
		bit := n >> 1
		for ; j&bit != 0; bit >>= 1 {
			j ^= bit
		}
		j |= bit
		if i < j {
			x[i], x[j] = x[j], x[i]
		}
	}
	sign := -1.0
	if inverse {
		sign = 1.0
	}
	for length := 2; length <= n; length <<= 1 {
		ang := sign * 2 * math.Pi / float64(length)
		wl := cmplx.Rect(1, ang)
		for i := 0; i < n; i += length {
			w := complex(1, 0)
			for j := 0; j < length/2; j++ {
				u := x[i+j]
				v := x[i+j+length/2] * w
				x[i+j] = u + v
				x[i+j+length/2] = u - v
				w *= wl
			}
		}
	}
}

// nextPow2 returns the smallest power of two greater than or equal to n.
func nextPow2(n int) int {
	p := 1
	for p < n {
		p <<= 1
	}
	return p
}

// Correlator holds the transform of a reference signal so a run does not
// recompute it once per trial.
type Correlator struct {
	ref     []float64
	refFFT  []complex128
	refNorm float64
	window  int
	size    int
}

// NewCorrelator prepares to find ref inside windows of the given length.
func NewCorrelator(ref []float64, window int) *Correlator {
	size := nextPow2(window + len(ref))
	h := make([]complex128, size)
	var norm float64
	for i, v := range ref {
		h[i] = complex(v, 0)
		norm += v * v
	}
	fft(h, false)
	return &Correlator{ref: ref, refFFT: h, refNorm: math.Sqrt(norm), window: window, size: size}
}

// Find returns the offset into signal at which the reference best lines up and
// the normalised correlation there, between 0 and 1. Offsets whose reference
// would run past the end of signal are not considered, so the largest offset it
// can return is len(signal)-len(ref).
//
// The normalisation divides by the energy of the stretch of signal under the
// reference, which is what makes the returned figure comparable between trials
// and usable as a detection threshold. Without it a loud stretch of unrelated
// signal outscores a quiet copy of the reference.
func (c *Correlator) Find(signal []float64) (offset int, score float64) {
	if len(signal) < len(c.ref) {
		return 0, 0
	}
	x := make([]complex128, c.size)
	for i, v := range signal {
		if i >= c.size {
			break
		}
		x[i] = complex(v, 0)
	}
	fft(x, false)
	for i := range x {
		x[i] *= cmplx.Conj(c.refFFT[i])
	}
	fft(x, true)
	inv := 1 / float64(c.size)

	// Sliding energy of the signal under the reference, so the divisor at each
	// offset costs one add and one subtract rather than a second pass.
	m := len(c.ref)
	var energy float64
	for i := 0; i < m; i++ {
		energy += signal[i] * signal[i]
	}
	last := len(signal) - m
	best, bestScore := 0, math.Inf(-1)
	for p := 0; p <= last; p++ {
		if p > 0 {
			energy += signal[p+m-1]*signal[p+m-1] - signal[p-1]*signal[p-1]
		}
		den := math.Sqrt(energy) * c.refNorm
		if den == 0 {
			continue
		}
		s := real(x[p]) * inv / den
		if s > bestScore {
			best, bestScore = p, s
		}
	}
	if math.IsInf(bestScore, -1) {
		return 0, 0
	}
	return best, bestScore
}
