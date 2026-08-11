// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026  iderex

// Package bench holds the mouth-to-ear measurement rig.
//
// It measures one-way delay for a system it treats as a black box: a signal
// enters at a sending endpoint, the same signal leaves at a receiving endpoint,
// and the interval between the two is the figure. Nothing in the rig knows what
// the system is made of, which is the property that lets one instrument measure
// this project's software and somebody else's on the same terms.
//
// The rig reports a distribution rather than an average, because the figure a
// person in a conversation notices is the tail.
package bench

import (
	"fmt"
	"math"
	"time"
)

// Config describes one measurement run.
type Config struct {
	// Rate is the sample rate of both ports, in hertz.
	Rate int `json:"rate_hz"`
	// FrameSamples is the number of samples exchanged per tick.
	FrameSamples int `json:"frame_samples"`
	// ChirpSamples is the length of the injected sweep.
	ChirpSamples int `json:"chirp_samples"`
	// ChirpLowHz and ChirpHighHz bound the sweep.
	ChirpLowHz  float64 `json:"chirp_low_hz"`
	ChirpHighHz float64 `json:"chirp_high_hz"`
	// Trials is how many chirps a pass injects. Each detected chirp is one
	// sample of the distribution.
	Trials int `json:"trials"`
	// TrialPeriodSamples is the spacing between chirps, and with it the
	// largest delay the rig can tell apart from the next chirp arriving early.
	TrialPeriodSamples int `json:"trial_period_samples"`
	// DetectionThreshold is the normalised correlation below which a trial is
	// counted as a miss rather than as a measurement. A miss is reported; it is
	// never quietly dropped, because a pass that detected nothing and a pass
	// that measured zero delay have to look different.
	DetectionThreshold float64 `json:"detection_threshold"`
}

// DefaultConfig is the configuration a run uses when nothing overrides it.
//
// The sweep runs from 300 Hz to 8 kHz: low enough to survive a narrowband path,
// high enough that the correlation peak sits on one sample. 40 ms of sweep at a
// 500 ms spacing leaves 460 ms of measurable delay, which is well past anything
// this project would accept and past the point where a conversation has already
// stopped working.
func DefaultConfig() Config {
	return Config{
		Rate:               48000,
		FrameSamples:       960,
		ChirpSamples:       1920,
		ChirpLowHz:         300,
		ChirpHighHz:        8000,
		Trials:             300,
		TrialPeriodSamples: 24000,
		DetectionThreshold: 0.30,
	}
}

// Validate reports why cfg cannot be run, or nil.
func (c Config) Validate() error {
	switch {
	case c.Rate <= 0:
		return fmt.Errorf("rate must be positive, got %d", c.Rate)
	case c.FrameSamples <= 0:
		return fmt.Errorf("frame must be positive, got %d", c.FrameSamples)
	case c.ChirpSamples <= 0:
		return fmt.Errorf("chirp must be positive, got %d", c.ChirpSamples)
	case c.Trials <= 0:
		return fmt.Errorf("trials must be positive, got %d", c.Trials)
	case c.TrialPeriodSamples%c.FrameSamples != 0:
		return fmt.Errorf("trial period %d is not a whole number of %d-sample frames",
			c.TrialPeriodSamples, c.FrameSamples)
	case c.TrialPeriodSamples <= c.ChirpSamples:
		return fmt.Errorf("trial period %d leaves no room for a %d-sample chirp",
			c.TrialPeriodSamples, c.ChirpSamples)
	}
	return nil
}

// TrialPeriod is the spacing between chirps as a duration.
func (c Config) TrialPeriod() time.Duration {
	return time.Duration(float64(c.TrialPeriodSamples) / float64(c.Rate) * float64(time.Second))
}

// MaxMeasurableDelay is the largest delay a pass can report. A system slower
// than this is not measured as slow; it is measured as something else, so the
// figure is carried in every report rather than left to a reader to derive.
func (c Config) MaxMeasurableDelay() time.Duration {
	n := c.TrialPeriodSamples - c.ChirpSamples
	return time.Duration(float64(n) / float64(c.Rate) * float64(time.Second))
}

// Pass is the result of driving a system once.
type Pass struct {
	TrialsAttempted int `json:"trials_attempted"`
	// SampleCount is how many trials produced a detection. Every percentile
	// below is over exactly these.
	SampleCount int `json:"sample_count"`
	// Misses is how many trials correlated below the threshold.
	Misses int `json:"misses"`
	// WeakestAccepted is the lowest correlation among the detections, so a run
	// that only just cleared the threshold is visible as such.
	WeakestAccepted float64   `json:"weakest_accepted_correlation"`
	P50Ms           float64   `json:"p50_ms"`
	P95Ms           float64   `json:"p95_ms"`
	P99Ms           float64   `json:"p99_ms"`
	MinMs           float64   `json:"min_ms"`
	MaxMs           float64   `json:"max_ms"`
	DelaysMs        []float64 `json:"delays_ms,omitempty"`
}

// Measure drives sys through cfg.Trials chirps and returns the distribution of
// the delay between each chirp entering the sending port and leaving the
// receiving port.
func Measure(sys System, cfg Config) (Pass, error) {
	if err := cfg.Validate(); err != nil {
		return Pass{}, err
	}
	chirp := Chirp(cfg.ChirpSamples, cfg.Rate, cfg.ChirpLowHz, cfg.ChirpHighHz)
	corr := NewCorrelator(chirp, cfg.TrialPeriodSamples)

	in := make([]float64, cfg.FrameSamples)
	out := make([]float64, cfg.FrameSamples)
	window := make([]float64, cfg.TrialPeriodSamples)
	framesPerTrial := cfg.TrialPeriodSamples / cfg.FrameSamples

	pass := Pass{TrialsAttempted: cfg.Trials, WeakestAccepted: math.Inf(1)}
	delays := make([]float64, 0, cfg.Trials)
	for t := 0; t < cfg.Trials; t++ {
		for f := 0; f < framesPerTrial; f++ {
			base := f * cfg.FrameSamples
			for i := range in {
				k := base + i
				if k < len(chirp) {
					in[i] = chirp[k]
				} else {
					in[i] = 0
				}
			}
			sys.Exchange(in, out)
			copy(window[base:], out)
		}
		offset, score := corr.Find(window)
		if score < cfg.DetectionThreshold {
			pass.Misses++
			continue
		}
		if score < pass.WeakestAccepted {
			pass.WeakestAccepted = score
		}
		delays = append(delays, float64(offset)/float64(cfg.Rate)*1000)
	}
	pass.SampleCount = len(delays)
	pass.DelaysMs = append([]float64(nil), delays...)
	if pass.SampleCount == 0 {
		pass.WeakestAccepted = 0
		pass.P50Ms, pass.P95Ms, pass.P99Ms = math.NaN(), math.NaN(), math.NaN()
		pass.MinMs, pass.MaxMs = math.NaN(), math.NaN()
		return pass, nil
	}
	pass.P50Ms = Percentile(delays, 50)
	pass.P95Ms = Percentile(delays, 95)
	pass.P99Ms = Percentile(delays, 99)
	// Percentile sorts in place, so the ends are now the extremes.
	pass.MinMs = delays[0]
	pass.MaxMs = delays[len(delays)-1]
	return pass, nil
}

// Shaping records the network conditions a pass was taken under.
//
// The rig does not apply shaping and does not check it. Shaping is the
// operating system's own facility applied from outside to the container the
// system under test runs in, which is a thing the rig has no handle on, so the
// most it can honestly do is carry what it was given and say that it carried it
// rather than verified it.
//
// What it is given is two different things. A profile name is a label somebody
// chose, and two runs under one label are comparable only if whoever ran them
// meant the same thing by it. Command is the shaping that was actually applied,
// written out, and it is the only part of this record a later reader can act
// on: they can run it, read it against the label, or see that the label was
// wrong. Neither is verified here, and carrying the command does not make the
// shaping any more checked than the name does.
type Shaping struct {
	Profile string `json:"profile"`
	Command string `json:"command"`
	Origin  string `json:"origin"`
}

// DeclaredShaping labels a profile and the command behind it as what they are:
// assertions by whoever ran the rig.
//
// A named profile with no command is the case worth handling rather than
// permitting quietly. An empty command field is what an unshaped run also
// carries, so the two would read alike in a report, and the difference between
// them is the difference between nothing to record and something nobody wrote
// down. The origin sentence says which one it is.
func DeclaredShaping(profile, command string) Shaping {
	if profile == "" {
		profile = "none"
	}
	origin := "declared by the caller; not applied and not verified by the rig"
	if command == "" && profile != "none" {
		origin += "; no command was declared for this profile, so the name is the whole of the record"
	}
	return Shaping{
		Profile: profile,
		Command: command,
		Origin:  origin,
	}
}

// Repeatability compares two passes over an unchanged system.
type Repeatability struct {
	FirstP95Ms  float64 `json:"first_p95_ms"`
	SecondP95Ms float64 `json:"second_p95_ms"`
	DeltaMs     float64 `json:"delta_ms"`
	BoundMs     float64 `json:"bound_ms"`
	WithinBound bool    `json:"within_bound"`
	Statement   string  `json:"statement"`
}

// Report is what a run writes out.
type Report struct {
	Schema           string         `json:"schema"`
	System           string         `json:"system"`
	Config           Config         `json:"config"`
	TrialPeriodMs    float64        `json:"trial_period_ms"`
	MaxMeasurableMs  float64        `json:"max_measurable_delay_ms"`
	PercentileMethod string         `json:"percentile_method"`
	Shaping          Shaping        `json:"shaping"`
	First            Pass           `json:"first_pass"`
	Second           *Pass          `json:"second_pass,omitempty"`
	Repeatability    *Repeatability `json:"repeatability,omitempty"`
	Notes            []string       `json:"notes"`
}

// RepeatabilityBoundMs is the bound the rig holds itself to between two passes
// over an unchanged system.
const RepeatabilityBoundMs = 10

// CompareP95 builds the repeatability record for two passes.
func CompareP95(first, second Pass) Repeatability {
	delta := math.Abs(first.P95Ms - second.P95Ms)
	r := Repeatability{
		FirstP95Ms:  first.P95Ms,
		SecondP95Ms: second.P95Ms,
		DeltaMs:     delta,
		BoundMs:     RepeatabilityBoundMs,
		WithinBound: delta <= RepeatabilityBoundMs,
	}
	if r.WithinBound {
		r.Statement = fmt.Sprintf(
			"two passes over the same system gave p95 of %.3f ms and %.3f ms, a difference of %.3f ms, which is inside the %g ms bound",
			first.P95Ms, second.P95Ms, delta, float64(RepeatabilityBoundMs))
	} else {
		r.Statement = fmt.Sprintf(
			"two passes over the same system gave p95 of %.3f ms and %.3f ms, a difference of %.3f ms, which is outside the %g ms bound",
			first.P95Ms, second.P95Ms, delta, float64(RepeatabilityBoundMs))
	}
	return r
}
