package bench

import (
	"math"
	"testing"
)

func TestPercentileByNearestRank(t *testing.T) {
	// Ten values, so the p-th percentile is the ceil(p/10)-th of them.
	base := []float64{10, 20, 30, 40, 50, 60, 70, 80, 90, 100}
	cases := []struct{ p, want float64 }{
		{0, 10}, {1, 10}, {10, 10}, {11, 20}, {50, 50}, {95, 100}, {99, 100}, {100, 100},
	}
	for _, c := range cases {
		xs := append([]float64(nil), base...)
		if got := Percentile(xs, c.p); got != c.want {
			t.Errorf("p%v = %v, want %v", c.p, got, c.want)
		}
	}
}

func TestPercentileSortsWhatItIsGiven(t *testing.T) {
	xs := []float64{5, 1, 4, 2, 3}
	if got := Percentile(xs, 50); got != 3 {
		t.Fatalf("p50 of an unsorted sample = %v, want 3", got)
	}
}

func TestPercentileOfNothingIsNotZero(t *testing.T) {
	// Zero is a delay somebody could plausibly measure. A missing measurement
	// has to be distinguishable from a fast one.
	if got := Percentile(nil, 95); !math.IsNaN(got) {
		t.Fatalf("p95 of an empty sample = %v, want NaN", got)
	}
}
