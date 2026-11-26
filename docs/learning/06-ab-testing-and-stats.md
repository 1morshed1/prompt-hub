# 06 — A/B testing and the statistics behind it

This is the subsystem that turns PromptHub from "a nice notepad for prompts" into a decision
tool. "Variant looks better" is an opinion. "Variant's rating is higher with p = 0.009 over 200
requests" is a decision you can defend. Code: `server/internal/stats/` (the math) and
`server/internal/experiment/` (the workflow).

## The workflow, end to end

1. You have version v5 in `production`. You think v6 is better.
2. You start an **experiment**: control = v5, variant = v6, `traffic_split = 0.5`.
3. Every render request for that prompt gets **assigned** to an arm (control or variant),
   deterministically by request id, and the served version is recorded.
4. Your app reports **results** — latency, cost, and the end user's thumbs up/down — against the
   request id.
5. The **analyzer** aggregates results per arm and asks the statistics: is the difference real?
6. If the variant wins convincingly and `auto_promote` is on, PromptHub **moves the
   `production` tag** to v6. Rollback, if you change your mind, is just moving it back.

Assignment and results live in doc 06's sibling code (`experiment` package); this doc focuses
on the **statistics**, because that's the part worth truly understanding.

## Two metrics, two different tests

You cannot use one test for everything. The *type* of data dictates the test.

### Ratings → chi-squared test

A user rating is **categorical**: thumbs up (1) or thumbs down (0). To compare the up-rate
between control and variant you build a 2×2 contingency table:

```
            👍 up     👎 down
control       a          b
variant       c          d
```

and ask: "is the outcome (up/down) *independent* of the arm (control/variant)?" If they're
independent, the arm doesn't affect the rating — no effect. The **chi-squared test of
independence** measures how far the observed table is from what you'd expect under
independence.

We use **Yates' continuity correction** because χ² is a continuous approximation to discrete
counts, and without the correction it overstates significance on modest samples:

```
χ² = N·(|ad − bc| − N/2)² / ((a+b)(c+d)(a+c)(b+d))
```

For a 2×2 table there is **1 degree of freedom**, and a beautiful simplification falls out:
χ²(1) is just Z², so the p-value is

```
P(χ² > x) = P(|Z| > √x) = erfc(√(x/2))
```

That's why the whole thing reduces to the error function. See `ChiSquared2x2` in `stats.go`.

> Pinned test: `ChiSquared2x2(10,90,25,75)` gives χ²=6.787879, p=0.009178 — identical to
> SciPy's `chi2_contingency([[10,90],[25,75]], correction=True)`.

### Latency → Mann-Whitney U test

Latency is **continuous**, but the naïve choice (a t-test) is *wrong* here, because a t-test
assumes roughly normal data and latency is nothing of the sort: it's right-skewed with a long
tail (most requests fast, a few very slow). Using a t-test on it gives misleading p-values.

The **Mann-Whitney U test** is the right tool. It's **non-parametric**: it throws away the
actual values and works only with their **ranks**, so it makes no normality assumption. The
idea: pool both samples, rank everything, and if one group's values systematically hold the
higher ranks, the distributions differ.

Mechanically:

1. Pool both samples and sort.
2. Assign ranks, **averaging ranks within ties** (three values tied for ranks 4,5,6 all get 5).
3. Sum the ranks of sample 1 → `R₁`.
4. `U₁ = R₁ − n₁(n₁+1)/2`.
5. Under the null, `U` is approximately normal with mean `n₁n₂/2` and a **tie-corrected**
   variance. Standardize to `z` (with a continuity correction) and read off a two-sided
   p-value via `erfc(|z|/√2)`.

See `MannWhitneyU` and `rankPooled` in `stats.go`.

> Pinned tests match SciPy's `mannwhitneyu(..., alternative='two-sided', method='asymptotic')`,
> including the ties case (U=10.5, p=0.075656).

## The one primitive under it all: `erf`

Both p-values come from the normal distribution, and the normal CDF is expressible via the
**error function** `erf`. We implement `erf` with the Abramowitz & Stegun 7.1.26 rational
approximation (max error ~1.5×10⁻⁷ — vastly tighter than a 0.05 threshold needs), then:

```
NormalCDF(x)      = ½·erfc(−x/√2)
two-sided p (z)   = erfc(|z|/√2)
χ²(1) upper tail  = erfc(√(x/2))
```

One approximation, three p-values. That's the whole numerical core.

## Reproduce the reference values yourself

```python
import scipy.stats as st
st.chi2_contingency([[10,90],[25,75]], correction=True)      # chi2, p
st.mannwhitneyu(x, y, alternative='two-sided', method='asymptotic')
```

The Go tests in `stats_test.go` assert against exactly these numbers, so the from-scratch
implementation is *pinned* to a trusted reference, not merely "looks about right."

## Why implement it by hand?

Because the statistics are the product's intellectual core, and a reader
should be able to open one file and see precisely how "significant" is computed, with the
assumptions written next to the code. The full argument is in
[ADR-0004](../decisions/0004-statistics-from-scratch.md).

Next: how assignment, result logging, and auto-promotion are wired — read the `experiment`
package and its tests.
