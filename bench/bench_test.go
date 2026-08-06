package bench

import (
	"math"
	"testing"
	"time"
)

func fixture(cfg Config, base, jitter time.Duration, noise float64, seed int64) *DelayLine {
	return NewDelayLine(DelayLineConfig{
		Rate:           cfg.Rate,
		Frame:          cfg.FrameSamples,
		Base:           base,
		Jitter:         jitter,
		ResamplePeriod: cfg.TrialPeriod(),
		Noise:          noise,
		Seed:           seed,
	})
}

// TestRigTracksAKnownSyntheticDelay is the accuracy claim in issue #4: a fixed
// delay of 50 ms and of 200 ms, reported to within 5 ms.
func TestRigTracksAKnownSyntheticDelay(t *testing.T) {
	cfg := DefaultConfig()
	for _, want := range []time.Duration{50 * time.Millisecond, 200 * time.Millisecond} {
		pass, err := Measure(fixture(cfg, want, 0, 0, 1), cfg)
		if err != nil {
			t.Fatalf("%v fixture: %v", want, err)
		}
		if pass.SampleCount != cfg.Trials {
			t.Fatalf("%v fixture: %d of %d chirps detected", want, pass.SampleCount, cfg.Trials)
		}
		wantMs := float64(want) / float64(time.Millisecond)
		for _, f := range []struct {
			name string
			got  float64
		}{{"p50", pass.P50Ms}, {"p95", pass.P95Ms}, {"p99", pass.P99Ms}} {
			if diff := math.Abs(f.got - wantMs); diff > 5 {
				t.Errorf("%v fixture: %s reported %.3f ms, %.3f ms away from the %.0f ms inserted",
					want, f.name, f.got, diff, wantMs)
			}
		}
		t.Logf("%v fixture: p50 %.3f ms, p95 %.3f ms, p99 %.3f ms over %d samples",
			want, pass.P50Ms, pass.P95Ms, pass.P99Ms, pass.SampleCount)
	}
}

// TestRigReportsATailAndNotAnAverage is the other half of the instrument's
// purpose. A fixture with a 60 ms spread has to come back as a spread.
func TestRigReportsATailAndNotAnAverage(t *testing.T) {
	cfg := DefaultConfig()
	pass, err := Measure(fixture(cfg, 40*time.Millisecond, 60*time.Millisecond, 0, 9), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if pass.MinMs < 39.9 || pass.MaxMs > 100.1 {
		t.Fatalf("delays ran from %.3f ms to %.3f ms, outside the fixture's 40 ms to 100 ms",
			pass.MinMs, pass.MaxMs)
	}
	if pass.P99Ms-pass.P50Ms < 20 {
		t.Fatalf("p99 %.3f ms is only %.3f ms above p50 %.3f ms; a 60 ms spread has been flattened",
			pass.P99Ms, pass.P99Ms-pass.P50Ms, pass.P50Ms)
	}
	t.Logf("p50 %.3f ms, p95 %.3f ms, p99 %.3f ms, min %.3f ms, max %.3f ms",
		pass.P50Ms, pass.P95Ms, pass.P99Ms, pass.MinMs, pass.MaxMs)
}

// TestTwoPassesOverOneSystemAgreeOnP95 is the repeatability claim. The two
// passes draw their variation from different seeds, so this is a bound the rig
// could miss rather than an identity it cannot.
func TestTwoPassesOverOneSystemAgreeOnP95(t *testing.T) {
	cfg := DefaultConfig()
	const base, jitter = 40 * time.Millisecond, 30 * time.Millisecond
	first, err := Measure(fixture(cfg, base, jitter, 0, 100), cfg)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Measure(fixture(cfg, base, jitter, 0, 101), cfg)
	if err != nil {
		t.Fatal(err)
	}
	r := CompareP95(first, second)
	if !r.WithinBound {
		t.Fatalf("%s", r.Statement)
	}
	// The bound is only worth something if the two passes are actually
	// different runs. Two p95 figures can land on the same sample by chance, so
	// the check is against the sequences behind them.
	same := len(first.DelaysMs) == len(second.DelaysMs)
	if same {
		for i := range first.DelaysMs {
			if first.DelaysMs[i] != second.DelaysMs[i] {
				same = false
				break
			}
		}
	}
	if same {
		t.Fatal("both passes measured the same sequence of delays; the seeds are not reaching the fixture")
	}
	t.Log(r.Statement)
}

// TestRigFindsTheChirpUnderEqualNoise is the near-miss for the correlator. At
// an amplitude of 1 the noise carries roughly the power of the sweep itself.
func TestRigFindsTheChirpUnderEqualNoise(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Trials = 100
	pass, err := Measure(fixture(cfg, 50*time.Millisecond, 0, 1.0, 42), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if pass.SampleCount != cfg.Trials {
		t.Fatalf("%d of %d chirps detected under equal noise", pass.SampleCount, cfg.Trials)
	}
	if diff := math.Abs(pass.P95Ms - 50); diff > 5 {
		t.Fatalf("p95 under equal noise is %.3f ms, %.3f ms from the 50 ms inserted", pass.P95Ms, diff)
	}
	t.Logf("under equal noise: p95 %.3f ms, weakest accepted correlation %.4f",
		pass.P95Ms, pass.WeakestAccepted)
}

// TestAPassThatDetectsNothingSaysSo guards the difference between a system that
// is fast and a rig that is broken.
func TestAPassThatDetectsNothingSaysSo(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Trials = 10
	cfg.DetectionThreshold = 1.5 // unreachable, so every trial is a miss
	pass, err := Measure(fixture(cfg, 50*time.Millisecond, 0, 0, 1), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if pass.SampleCount != 0 || pass.Misses != cfg.Trials {
		t.Fatalf("detected %d and missed %d of %d", pass.SampleCount, pass.Misses, cfg.Trials)
	}
	for _, f := range []struct {
		name string
		got  float64
	}{{"p50", pass.P50Ms}, {"p95", pass.P95Ms}, {"p99", pass.P99Ms}} {
		if !math.IsNaN(f.got) {
			t.Errorf("%s of nothing is %v, want NaN", f.name, f.got)
		}
	}
}

// TestADelayPastTheCeilingIsNotReportedAsALargeOne holds the rig to the bound
// it prints. Past the ceiling the figure is wrong, and the report says the
// ceiling rather than leaving a reader to trust a number taken beyond it.
func TestADelayPastTheCeilingIsNotReportedAsALargeOne(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Trials = 10
	ceiling := cfg.MaxMeasurableDelay()
	pass, err := Measure(fixture(cfg, ceiling+50*time.Millisecond, 0, 0, 1), cfg)
	if err != nil {
		t.Fatal(err)
	}
	ceilingMs := float64(ceiling) / float64(time.Millisecond)
	if pass.SampleCount > 0 && pass.P50Ms > ceilingMs {
		t.Fatalf("a delay past the %.1f ms ceiling was reported as %.3f ms", ceilingMs, pass.P50Ms)
	}
	t.Logf("ceiling %.1f ms: %d detected, %d missed, p50 %.3f ms",
		ceilingMs, pass.SampleCount, pass.Misses, pass.P50Ms)
}

func TestValidateRefusesAPeriodThatIsNotWholeFrames(t *testing.T) {
	cfg := DefaultConfig()
	cfg.TrialPeriodSamples = cfg.FrameSamples*25 + 1
	if err := cfg.Validate(); err == nil {
		t.Fatal("a trial period of 24001 samples over 960-sample frames was accepted")
	}
	if _, err := Measure(fixture(cfg, 0, 0, 0, 1), cfg); err == nil {
		t.Fatal("Measure ran a configuration Validate refuses")
	}
}

func TestMaxMeasurableDelayMatchesTheConfiguration(t *testing.T) {
	cfg := DefaultConfig()
	want := time.Duration(float64(cfg.TrialPeriodSamples-cfg.ChirpSamples) / float64(cfg.Rate) * float64(time.Second))
	if got := cfg.MaxMeasurableDelay(); got != want {
		t.Fatalf("MaxMeasurableDelay = %v, want %v", got, want)
	}
	if cfg.MaxMeasurableDelay() < 300*time.Millisecond {
		t.Fatalf("the default ceiling is %v, too low to carry the budget lines this rig exists to check",
			cfg.MaxMeasurableDelay())
	}
}

func TestCompareP95RefusesADifferenceOutsideTheBound(t *testing.T) {
	r := CompareP95(Pass{P95Ms: 40}, Pass{P95Ms: 61})
	if r.WithinBound {
		t.Fatalf("a 21 ms difference was called within the %d ms bound", RepeatabilityBoundMs)
	}
	if r.DeltaMs != 21 {
		t.Fatalf("delta reported as %v, want 21", r.DeltaMs)
	}
}
