// Command mouth-to-ear runs the mouth-to-ear measurement rig and writes a
// report.
//
// The only system it can be pointed at today is the rig's own delay fixture.
// There is no server to measure yet and no implementation of bench.System that
// speaks to a real audio path, so every figure this command produces is a
// figure about the instrument. It is not a measurement of this project and it
// is not a measurement of anybody else's.
//
//	go run ./bench/cmd/mouth-to-ear --delay 50ms --out report.json
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/iderex/stammtisch/bench"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "mouth-to-ear:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr *os.File) error {
	fs := flag.NewFlagSet("mouth-to-ear", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var (
		delay      = fs.Duration("delay", 50*time.Millisecond, "fixture: the delay every sample takes")
		jitter     = fs.Duration("jitter", 0, "fixture: width of the extra delay drawn per trial")
		noise      = fs.Float64("noise", 0, "fixture: amplitude of white noise added at the receiving port")
		seed       = fs.Int64("seed", 1, "fixture: seed for the first pass; the second pass uses seed+1")
		trials     = fs.Int("trials", 0, "chirps per pass (default 300)")
		minSamples = fs.Int("min-samples", 300, "fail unless a pass detects at least this many chirps")
		shaping    = fs.String("shaping", "none", "name of the network shaping profile in force")
		out        = fs.String("out", "", "write the report to this file instead of standard output")
		raw        = fs.Bool("raw", false, "include every individual delay in the report")
		repeat     = fs.Bool("repeat", true, "run a second pass and compare p95")
	)
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg := bench.DefaultConfig()
	if *trials > 0 {
		cfg.Trials = *trials
	}
	if err := cfg.Validate(); err != nil {
		return err
	}

	fixture := func(s int64) *bench.DelayLine {
		return bench.NewDelayLine(bench.DelayLineConfig{
			Rate:           cfg.Rate,
			Frame:          cfg.FrameSamples,
			Base:           *delay,
			Jitter:         *jitter,
			ResamplePeriod: cfg.TrialPeriod(),
			Noise:          *noise,
			Seed:           s,
		})
	}

	name := fmt.Sprintf("rig fixture: delay %v, jitter %v, noise %g", *delay, *jitter, *noise)
	rep := bench.Report{
		Schema:           "stammtisch.mouth-to-ear.v1",
		System:           name,
		Config:           cfg,
		TrialPeriodMs:    float64(cfg.TrialPeriod()) / float64(time.Millisecond),
		MaxMeasurableMs:  float64(cfg.MaxMeasurableDelay()) / float64(time.Millisecond),
		PercentileMethod: bench.PercentileMethod,
		Shaping:          bench.DeclaredShaping(*shaping),
		Notes: []string{
			"The system under test is the rig's own delay fixture. No implementation of bench.System reaches a real audio path in this tree, so nothing here measures this project's software or anybody else's.",
			"The shaping profile is carried as declared. The rig neither applied it nor checked it.",
			"Delays at or above max_measurable_delay_ms are not reported as large; they are not reported correctly at all.",
		},
	}

	first, err := bench.Measure(fixture(*seed), cfg)
	if err != nil {
		return err
	}
	rep.First = first
	if *repeat {
		second, err := bench.Measure(fixture(*seed+1), cfg)
		if err != nil {
			return err
		}
		rep.Second = &second
		r := bench.CompareP95(first, second)
		rep.Repeatability = &r
	}
	if !*raw {
		rep.First.DelaysMs = nil
		if rep.Second != nil {
			rep.Second.DelaysMs = nil
		}
	}

	body, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	if *out == "" {
		if _, err := stdout.Write(body); err != nil {
			return err
		}
	} else {
		if err := os.WriteFile(*out, body, 0o644); err != nil {
			return err
		}
	}

	fmt.Fprintf(stderr, "pass 1: %d/%d chirps detected, %d missed, p50 %.3f ms, p95 %.3f ms, p99 %.3f ms\n",
		first.SampleCount, first.TrialsAttempted, first.Misses, first.P50Ms, first.P95Ms, first.P99Ms)
	if rep.Second != nil {
		fmt.Fprintf(stderr, "pass 2: %d/%d chirps detected, %d missed, p50 %.3f ms, p95 %.3f ms, p99 %.3f ms\n",
			rep.Second.SampleCount, rep.Second.TrialsAttempted, rep.Second.Misses,
			rep.Second.P50Ms, rep.Second.P95Ms, rep.Second.P99Ms)
		fmt.Fprintln(stderr, rep.Repeatability.Statement)
	}

	// A pass that detected nothing has to fail rather than report a clean
	// empty distribution, and a repeatability figure outside the bound is a
	// result the command refuses to exit zero on.
	if first.SampleCount < *minSamples {
		return fmt.Errorf("pass 1 detected %d chirps, below the %d required", first.SampleCount, *minSamples)
	}
	if rep.Second != nil {
		if rep.Second.SampleCount < *minSamples {
			return fmt.Errorf("pass 2 detected %d chirps, below the %d required", rep.Second.SampleCount, *minSamples)
		}
		if !rep.Repeatability.WithinBound {
			return fmt.Errorf("%s", rep.Repeatability.Statement)
		}
	}
	return nil
}
