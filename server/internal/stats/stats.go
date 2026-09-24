// Package stats implements the two significance tests the experiment analyzer
// needs, from scratch and with no dependencies:
//
//   - ChiSquared2x2  — for categorical outcomes (thumbs up/down ratings).
//   - MannWhitneyU   — for continuous, non-normal metrics (latency).
//
// Both rest on one shared primitive, the error function (Erf/Erfc), from which
// the normal CDF and the relevant p-values are derived. The math and its
// assumptions are documented inline because, for this product, the statistics
// ARE the value proposition — see docs/decisions/0004-statistics-from-scratch.md
// and docs/learning/06-ab-testing-and-stats.md.
package stats

import (
	"math"
	"sort"
)

// Erf is the Gauss error function, via the Abramowitz & Stegun 7.1.26 rational
// approximation. Max absolute error ~1.5e-7, which is far tighter than anything
// that matters at a 0.05 significance threshold.
func Erf(x float64) float64 {
	sign := 1.0
	if x < 0 {
		sign = -1.0
		x = -x
	}
	const (
		a1 = 0.254829592
		a2 = -0.284496736
		a3 = 1.421413741
		a4 = -1.453152027
		a5 = 1.061405429
		p  = 0.3275911
	)
	t := 1.0 / (1.0 + p*x)
	y := 1.0 - (((((a5*t+a4)*t)+a3)*t+a2)*t+a1)*t*math.Exp(-x*x)
	return sign * y
}

// Erfc is the complementary error function, 1 - Erf(x).
func Erfc(x float64) float64 { return 1.0 - Erf(x) }

// NormalCDF is Φ(x), the standard normal cumulative distribution function.
func NormalCDF(x float64) float64 { return 0.5 * Erfc(-x/math.Sqrt2) }

// twoSidedNormalP returns the two-sided p-value for a z statistic:
// P(|Z| > |z|) = erfc(|z|/√2).
func twoSidedNormalP(z float64) float64 { return Erfc(math.Abs(z) / math.Sqrt2) }

// chiSqSurvival1 returns P(χ²₁ > x), the upper-tail probability of a chi-squared
// distribution with ONE degree of freedom. For 1 dof, χ² = Z², so
// P(χ² > x) = P(|Z| > √x) = erfc(√(x/2)).
func chiSqSurvival1(x float64) float64 {
	if x <= 0 {
		return 1.0
	}
	return Erfc(math.Sqrt(x / 2.0))
}

// ChiSquaredResult is the outcome of a 2x2 chi-squared test of independence.
type ChiSquaredResult struct {
	ChiSquared float64 `json:"chi_squared"`
	PValue     float64 `json:"p_value"`
	// N is the total sample size; the test is unreliable for very small N.
	N int `json:"n"`
}

// ChiSquared2x2 tests whether the success rate differs between two groups, given
// the 2x2 contingency table:
//
//	           success   failure
//	control      a          b
//	variant      c          d
//
// It applies Yates' continuity correction (subtracting 0.5 from |ad-bc| scaled),
// which reduces the test's tendency to overstate significance on modest samples.
// The p-value comes from the χ²(1) upper tail.
//
// Formula (with Yates):
//
//	χ² = N·(|ad − bc| − N/2)² / ((a+b)(c+d)(a+c)(b+d))
func ChiSquared2x2(a, b, c, d int) ChiSquaredResult {
	af, bf, cf, df := float64(a), float64(b), float64(c), float64(d)
	n := af + bf + cf + df

	row1 := af + bf // control total
	row2 := cf + df // variant total
	col1 := af + cf // success total
	col2 := bf + df // failure total

	res := ChiSquaredResult{N: int(n)}
	// If any margin is zero the table is degenerate (a group or an outcome never
	// occurred); there's no association to measure, so p = 1.
	if row1 == 0 || row2 == 0 || col1 == 0 || col2 == 0 {
		res.PValue = 1.0
		return res
	}

	num := math.Abs(af*df-bf*cf) - n/2.0
	if num < 0 {
		num = 0 // Yates correction can't push the statistic below zero
	}
	chi2 := n * num * num / (row1 * row2 * col1 * col2)
	res.ChiSquared = chi2
	res.PValue = chiSqSurvival1(chi2)
	return res
}

// MannWhitneyUResult is the outcome of a Mann-Whitney U test.
type MannWhitneyUResult struct {
	U      float64 `json:"u"`       // the U statistic for sample 1
	Z      float64 `json:"z"`       // normal-approximation z score
	PValue float64 `json:"p_value"` // two-sided
	N1     int     `json:"n1"`
	N2     int     `json:"n2"`
}

// MannWhitneyU tests whether two continuous samples come from the same
// distribution, without assuming normality — the right tool for latency, which is
// right-skewed with fat tails. It ranks the pooled data (averaging ranks for
// ties), sums the ranks of sample 1, forms U, and uses the tie-corrected normal
// approximation to get a two-sided p-value.
//
// The normal approximation is accurate for the sample sizes A/B tests reach
// (tens+ per arm). For tiny samples an exact test would be needed; we note that
// via the sample sizes in the result so callers can gate on min_samples.
func MannWhitneyU(sample1, sample2 []float64) MannWhitneyUResult {
	n1, n2 := len(sample1), len(sample2)
	res := MannWhitneyUResult{N1: n1, N2: n2, PValue: 1.0}
	if n1 == 0 || n2 == 0 {
		return res
	}

	ranks, tieCorrection := rankPooled(sample1, sample2)

	// Sum ranks belonging to sample 1 (the first n1 pooled entries).
	var r1 float64
	for i := 0; i < n1; i++ {
		r1 += ranks[i]
	}

	n1f, n2f := float64(n1), float64(n2)
	u1 := r1 - n1f*(n1f+1)/2.0
	res.U = u1

	meanU := n1f * n2f / 2.0
	nTot := n1f + n2f
	// Tie-corrected variance of U.
	varU := (n1f * n2f / 12.0) * ((nTot + 1) - tieCorrection/(nTot*(nTot-1)))
	if varU <= 0 {
		// All values identical across both samples: no difference detectable.
		res.PValue = 1.0
		return res
	}
	// Continuity correction: shift the numerator 0.5 toward the mean before
	// standardizing. U is discrete but we approximate it with the continuous
	// normal; the 0.5 nudge accounts for that and matches the standard (and
	// SciPy's default) treatment, making the p-value slightly less anti-conservative.
	diff := u1 - meanU
	switch {
	case diff > 0:
		diff -= 0.5
	case diff < 0:
		diff += 0.5
	}
	z := diff / math.Sqrt(varU)
	res.Z = z
	res.PValue = twoSidedNormalP(z)
	return res
}

// rankPooled pools the two samples (sample1 first), assigns ranks with ties
// averaged, and returns the ranks in pooled order plus the tie-correction term
// Σ(tᵢ³ − tᵢ) used in the variance formula.
func rankPooled(sample1, sample2 []float64) ([]float64, float64) {
	type item struct {
		val float64
		idx int // position in the pooled slice
	}
	pooled := make([]item, 0, len(sample1)+len(sample2))
	for i, v := range sample1 {
		pooled = append(pooled, item{v, i})
	}
	for i, v := range sample2 {
		pooled = append(pooled, item{v, len(sample1) + i})
	}
	sort.Slice(pooled, func(i, j int) bool { return pooled[i].val < pooled[j].val })

	ranks := make([]float64, len(pooled))
	var tieCorrection float64
	i := 0
	for i < len(pooled) {
		j := i
		for j < len(pooled) && pooled[j].val == pooled[i].val {
			j++
		}
		// Positions i..j-1 are tied; they share the average of ranks (i+1 .. j).
		count := j - i
		avgRank := (float64(i+1) + float64(j)) / 2.0
		for k := i; k < j; k++ {
			ranks[pooled[k].idx] = avgRank
		}
		if count > 1 {
			t := float64(count)
			tieCorrection += t*t*t - t
		}
		i = j
	}
	return ranks, tieCorrection
}

// Mean is a small convenience used by the analyzer.
func Mean(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	var sum float64
	for _, x := range xs {
		sum += x
	}
	return sum / float64(len(xs))
}
