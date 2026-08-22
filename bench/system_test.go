// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026  Nils Lehnen

package bench

import (
	"testing"
	"time"
)

// impulseArrival drives the fixture with a single sample of 1 at absolute index
// zero and returns the absolute index at which it leaves the receiving port.
func impulseArrival(t *testing.T, d *DelayLine, frame, frames int) int {
	t.Helper()
	in := make([]float64, frame)
	out := make([]float64, frame)
	in[0] = 1
	for f := 0; f < frames; f++ {
		d.Exchange(in, out)
		for i, v := range out {
			if v != 0 {
				return f*frame + i
			}
		}
		in[0] = 0
	}
	return -1
}

func TestDelayLineHoldsASampleForExactlyItsDelay(t *testing.T) {
	const rate, frame = 48000, 960
	for _, want := range []time.Duration{0, 20 * time.Millisecond, 50 * time.Millisecond, 137 * time.Millisecond} {
		d := NewDelayLine(DelayLineConfig{Rate: rate, Frame: frame, Base: want, ResamplePeriod: time.Second})
		got := impulseArrival(t, d, frame, 40)
		wantSamples := int(want.Seconds() * rate)
		if got != wantSamples {
			t.Errorf("a %v fixture delivered the impulse at sample %d, want %d", want, got, wantSamples)
		}
	}
}

func TestDelayLineReportsTheDelayItWasBuiltWith(t *testing.T) {
	d := NewDelayLine(DelayLineConfig{Rate: 48000, Frame: 960, Base: 200 * time.Millisecond})
	if got := d.Delay(48000); got != 200*time.Millisecond {
		t.Fatalf("Delay reported %v, want 200ms", got)
	}
}

func TestDelayLineKeepsJitterInsideItsWidth(t *testing.T) {
	const rate, frame = 48000, 960
	const base, jitter = 40 * time.Millisecond, 30 * time.Millisecond
	seen := map[int]bool{}
	for seed := int64(0); seed < 24; seed++ {
		d := NewDelayLine(DelayLineConfig{
			Rate: rate, Frame: frame, Base: base, Jitter: jitter,
			ResamplePeriod: 500 * time.Millisecond, Seed: seed,
		})
		got := impulseArrival(t, d, frame, 40)
		lo := int(base.Seconds() * rate)
		hi := lo + int(jitter.Seconds()*rate)
		if got < lo || got > hi {
			t.Fatalf("seed %d put the impulse at sample %d, outside [%d, %d]", seed, got, lo, hi)
		}
		seen[got] = true
	}
	if len(seen) < 5 {
		t.Fatalf("24 seeds produced only %d distinct delays; the fixture is not varying", len(seen))
	}
}
