package bench

import (
	"math"
	"math/cmplx"
	"math/rand"
	"testing"
)

func TestFFTRoundTripsToItself(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	const n = 1024
	orig := make([]complex128, n)
	for i := range orig {
		orig[i] = complex(rng.NormFloat64(), 0)
	}
	x := append([]complex128(nil), orig...)
	fft(x, false)
	fft(x, true)
	for i := range x {
		got := x[i] / complex(float64(n), 0)
		if cmplx.Abs(got-orig[i]) > 1e-9 {
			t.Fatalf("sample %d round-tripped to %v, want %v", i, got, orig[i])
		}
	}
}

func TestFFTMatchesTheDirectTransform(t *testing.T) {
	// The direct transform is the definition. Agreeing with it is what makes
	// the fast one usable as evidence rather than as a black box inside a rig
	// whose whole purpose is not to have black boxes in it.
	rng := rand.New(rand.NewSource(11))
	const n = 64
	x := make([]complex128, n)
	for i := range x {
		x[i] = complex(rng.NormFloat64(), rng.NormFloat64())
	}
	want := make([]complex128, n)
	for k := 0; k < n; k++ {
		var s complex128
		for j := 0; j < n; j++ {
			s += x[j] * cmplx.Rect(1, -2*math.Pi*float64(k*j)/float64(n))
		}
		want[k] = s
	}
	got := append([]complex128(nil), x...)
	fft(got, false)
	for k := range got {
		if cmplx.Abs(got[k]-want[k]) > 1e-9 {
			t.Fatalf("bin %d is %v, want %v", k, got[k], want[k])
		}
	}
}

func TestChirpEnergySitsInsideItsBand(t *testing.T) {
	const rate = 48000
	c := Chirp(1920, rate, 300, 8000)
	n := nextPow2(len(c))
	x := make([]complex128, n)
	for i, v := range c {
		x[i] = complex(v, 0)
	}
	fft(x, false)
	var inBand, total float64
	for k := 0; k < n/2; k++ {
		f := float64(k) * rate / float64(n)
		p := real(x[k])*real(x[k]) + imag(x[k])*imag(x[k])
		total += p
		if f >= 250 && f <= 8500 {
			inBand += p
		}
	}
	if frac := inBand / total; frac < 0.98 {
		t.Fatalf("only %.4f of the sweep's energy is between 250 Hz and 8.5 kHz", frac)
	}
}

func TestCorrelatorFindsAKnownOffset(t *testing.T) {
	ref := Chirp(1920, 48000, 300, 8000)
	const window = 24000
	for _, offset := range []int{0, 1, 137, 4801, window - len(ref)} {
		signal := make([]float64, window)
		copy(signal[offset:], ref)
		got, score := NewCorrelator(ref, window).Find(signal)
		if got != offset {
			t.Errorf("reference planted at %d was found at %d", offset, got)
		}
		if score < 0.99 {
			t.Errorf("an exact copy at %d scored %.4f, want at least 0.99", offset, score)
		}
	}
}

func TestCorrelatorScoresNoiseWellBelowTheDetectionThreshold(t *testing.T) {
	// The threshold in DefaultConfig is what separates a measurement from a
	// miss. This is the measurement of the margin it has over a window that
	// contains nothing but noise.
	ref := Chirp(1920, 48000, 300, 8000)
	const window = 24000
	rng := rand.New(rand.NewSource(3))
	corr := NewCorrelator(ref, window)
	worst := 0.0
	for run := 0; run < 20; run++ {
		signal := make([]float64, window)
		for i := range signal {
			signal[i] = rng.NormFloat64()
		}
		if _, score := corr.Find(signal); score > worst {
			worst = score
		}
	}
	if worst >= DefaultConfig().DetectionThreshold {
		t.Fatalf("noise reached %.4f, at or above the %.2f detection threshold",
			worst, DefaultConfig().DetectionThreshold)
	}
	t.Logf("best correlation reached by noise over 20 windows: %.4f", worst)
}

func TestCorrelatorIsNotFooledByALoudBurst(t *testing.T) {
	// This is the near-miss the normalisation exists for. A stretch of loud
	// unrelated signal produces a large raw correlation and a small normalised
	// one; drop the divisor and the burst wins over the real copy.
	ref := Chirp(1920, 48000, 300, 8000)
	const window = 24000
	const planted = 9000
	rng := rand.New(rand.NewSource(5))
	signal := make([]float64, window)
	copy(signal[planted:], ref)
	for i := 2000; i < 3000; i++ {
		signal[i] = 40 * rng.NormFloat64()
	}
	got, score := NewCorrelator(ref, window).Find(signal)
	if got != planted {
		t.Fatalf("the copy at %d was passed over for offset %d", planted, got)
	}
	if score < 0.9 {
		t.Fatalf("the copy scored %.4f beside the burst, want at least 0.9", score)
	}
}

func TestNextPow2(t *testing.T) {
	for _, c := range []struct{ in, want int }{{1, 1}, {2, 2}, {3, 4}, {1023, 1024}, {1024, 1024}, {1025, 2048}} {
		if got := nextPow2(c.in); got != c.want {
			t.Errorf("nextPow2(%d) = %d, want %d", c.in, got, c.want)
		}
	}
}
