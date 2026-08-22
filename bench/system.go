// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026  Nils Lehnen

package bench

import (
	"math"
	"math/rand"
	"time"
)

// System is the black box under measurement.
//
// The rig hands it one frame captured at the sending endpoint per tick and
// takes one frame from the receiving endpoint in the same tick, which is how a
// pair of virtual devices clocked by one host behaves: one clock drives both
// ports, so the two sample indices are directly comparable and no skew estimate
// is needed. Whatever happens between the two ports belongs to the system. The
// rig assumes nothing about it, which is what lets one rig measure this
// project's software and somebody else's.
//
// Nothing in the tree implements this against a real audio path today. The only
// implementation here is DelayLine, which is a fixture for proving the rig
// rather than a system anybody would ship.
type System interface {
	// Exchange consumes in and fills out. Both are FrameSamples long. Neither
	// slice is valid after the call returns, so an implementation that needs to
	// keep either one copies it.
	Exchange(in, out []float64)
}

// DelayLine is a rig fixture: a system whose whole behaviour is a delay, with
// optional variation and optional additive noise. It exists so the rig can be
// pointed at a delay somebody already knows and asked whether it reports that
// delay back.
//
// It is not a network model and no figure taken through it says anything about
// a network. Variation here is drawn from a uniform distribution because that
// is the least flattering assumption for a correlator, not because packet
// delay is uniform.
type DelayLine struct {
	ring     []float64
	mask     int
	pos      int // absolute index of the next sample to be written
	base     int // delay in samples
	jitter   int // extra delay in samples, drawn uniformly in [0, jitter]
	cur      int // delay in force for the current block
	resample int // block length in samples; the delay is redrawn at each boundary
	noise    float64
	rng      *rand.Rand
}

// DelayLineConfig describes a fixture.
type DelayLineConfig struct {
	Rate  int
	Frame int
	// Base is the delay every sample takes.
	Base time.Duration
	// Jitter is the width of the extra delay drawn per block. Zero makes the
	// line exact, which is what the fixed-delay proof needs.
	Jitter time.Duration
	// ResamplePeriod is how often the extra delay is redrawn. The rig sets it
	// to the trial period so that one chirp is carried at one delay and the
	// correlator is not asked to find a signal the fixture stretched.
	ResamplePeriod time.Duration
	// Noise is the amplitude of white noise added at the receiving port. The
	// chirp leaves the sender at amplitude 1, so 1.0 here is roughly equal
	// signal and noise power.
	Noise float64
	// Seed fixes the fixture's randomness. Two runs with different seeds are
	// what makes a repeatability check something that could have failed.
	Seed int64
}

// NewDelayLine builds a fixture from cfg.
func NewDelayLine(cfg DelayLineConfig) *DelayLine {
	base := int(math.Round(cfg.Base.Seconds() * float64(cfg.Rate)))
	jitter := int(math.Round(cfg.Jitter.Seconds() * float64(cfg.Rate)))
	resample := int(math.Round(cfg.ResamplePeriod.Seconds() * float64(cfg.Rate)))
	if resample <= 0 {
		resample = cfg.Frame
	}
	size := nextPow2(base + jitter + cfg.Frame + 1)
	d := &DelayLine{
		ring:     make([]float64, size),
		mask:     size - 1,
		base:     base,
		jitter:   jitter,
		resample: resample,
		noise:    cfg.Noise,
		rng:      rand.New(rand.NewSource(cfg.Seed)),
	}
	d.cur = base
	return d
}

// Delay reports the fixture's base delay, which is the figure a caller who set
// no jitter is entitled to see reported back by the rig.
func (d *DelayLine) Delay(rate int) time.Duration {
	return time.Duration(float64(d.base) / float64(rate) * float64(time.Second))
}

// Exchange implements System.
func (d *DelayLine) Exchange(in, out []float64) {
	for i := range in {
		t := d.pos
		if t%d.resample == 0 && d.jitter > 0 {
			d.cur = d.base + d.rng.Intn(d.jitter+1)
		}
		d.ring[t&d.mask] = in[i]
		src := t - d.cur
		var v float64
		if src >= 0 {
			v = d.ring[src&d.mask]
		}
		if d.noise > 0 {
			v += d.noise * (2*d.rng.Float64() - 1)
		}
		out[i] = v
		d.pos++
	}
}
