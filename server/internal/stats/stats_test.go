package stats

import (
	"math"
	"testing"
)

// Reference values are computed independently with SciPy (see the commands in
// docs/learning/06-ab-testing-and-stats.md) so the from-scratch approximations
// are pinned, not hand-waved.

func approx(t *testing.T, name string, got, want, tol float64) {
	t.Helper()
	if math.Abs(got-want) > tol {
		t.Errorf("%s: got %.6f, want %.6f (tol %.1e)", name, got, want, tol)
	}
}

func TestErf(t *testing.T) {
	approx(t, "erf(0)", Erf(0), 0.0, 1e-7)
	approx(t, "erf(1)", Erf(1), 0.8427007929, 1e-6)
	approx(t, "erf(0.5)", Erf(0.5), 0.5204998778, 1e-6)
	approx(t, "erf(-1)", Erf(-1), -0.8427007929, 1e-6)
	approx(t, "erfc(1)", Erfc(1), 1-0.8427007929, 1e-6)
	approx(t, "NormalCDF(0)", NormalCDF(0), 0.5, 1e-7)
	approx(t, "NormalCDF(1.96)", NormalCDF(1.96), 0.9750021049, 1e-6)
}

func TestChiSquared2x2(t *testing.T) {
	// SciPy: chi2_contingency([[10,90],[25,75]], correction=True)
	//   chi2 = 6.787879, p = 0.009178
	r := ChiSquared2x2(10, 90, 25, 75)
	approx(t, "chi2", r.ChiSquared, 6.787879, 1e-4)
	approx(t, "chi2 p", r.PValue, 0.009178, 1e-3)
	if r.N != 200 {
		t.Errorf("N = %d, want 200", r.N)
	}
}

func TestChiSquaredDegenerate(t *testing.T) {
	// A whole column is zero (nobody gave a thumbs-up): no association measurable.
	r := ChiSquared2x2(0, 50, 0, 50)
	if r.PValue != 1.0 {
		t.Errorf("degenerate table should give p=1, got %v", r.PValue)
	}
}

func TestChiSquaredNoDifference(t *testing.T) {
	// Identical rates -> statistic ~0 -> p near 1 (not significant).
	r := ChiSquared2x2(50, 50, 50, 50)
	if r.PValue < 0.9 {
		t.Errorf("identical rates should be clearly non-significant, got p=%v", r.PValue)
	}
}

func TestMannWhitneyUSeparated(t *testing.T) {
	// SciPy asymptotic two-sided with continuity: U=0, p=0.000183
	x := []float64{12, 15, 14, 20, 18, 22, 19, 17, 16, 21}
	y := []float64{25, 30, 28, 35, 26, 40, 33, 29, 31, 27}
	r := MannWhitneyU(x, y)
	approx(t, "U", r.U, 0.0, 1e-9)
	approx(t, "MWU p", r.PValue, 0.000183, 5e-5)
}

func TestMannWhitneyUWithTies(t *testing.T) {
	// SciPy asymptotic two-sided with continuity: U=10.5, p=0.075656
	x := []float64{1, 2, 2, 3, 3, 3, 4}
	y := []float64{2, 3, 3, 4, 5, 5, 6}
	r := MannWhitneyU(x, y)
	approx(t, "U ties", r.U, 10.5, 1e-9)
	approx(t, "MWU p ties", r.PValue, 0.075656, 5e-3)
}

func TestMannWhitneyUIdentical(t *testing.T) {
	x := []float64{5, 5, 5}
	y := []float64{5, 5, 5}
	r := MannWhitneyU(x, y)
	if r.PValue != 1.0 {
		t.Errorf("identical samples should give p=1, got %v", r.PValue)
	}
}

func TestMannWhitneyUEmpty(t *testing.T) {
	r := MannWhitneyU(nil, []float64{1, 2, 3})
	if r.PValue != 1.0 {
		t.Errorf("empty sample should give p=1, got %v", r.PValue)
	}
}

func TestMean(t *testing.T) {
	approx(t, "mean", Mean([]float64{1, 2, 3, 4}), 2.5, 1e-9)
	approx(t, "mean empty", Mean(nil), 0, 1e-9)
}
