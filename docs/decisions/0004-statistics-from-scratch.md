# ADR-0004 — Statistics implemented from scratch

**Status:** Accepted
**Date:** 2026-09-24

## Context

The experiment analyzer needs to answer "is variant *really* better than control, or is this
noise?" for two kinds of metric:

- **User ratings** — categorical (thumbs up / thumbs down). The right tool is a **chi-squared
  test of independence** on the 2×2 table (variant/control × up/down).
- **Latency** — continuous, and definitely *not* normally distributed (latency is right-skewed
  with fat tails). A t-test would be wrong. The right tool is the **Mann-Whitney U test**, a
  non-parametric test that compares distributions via ranks and assumes no normality.

## Decision

Implement both tests in a dependency-free `stats` package:

- `ChiSquared2x2` — computes the χ² statistic with Yates' continuity correction and converts
  it to a p-value via the χ²(1) survival function (which, for 1 degree of freedom, reduces to
  `erfc`).
- `MannWhitneyU` — computes U with a tie-corrected rank sum and uses the normal approximation
  (valid for the sample sizes an experiment accumulates) to get a two-sided p-value.

Both rely on one shared primitive, the error function `erf`/`erfc`, implemented via the
Abramowitz & Stegun 7.1.26 approximation.

## Why not pull in a stats library

Go has no batteries-included scientific stack, and adding a heavyweight numeric dependency to a
serving binary is a poor trade. More importantly: **the statistics are the intellectual core of
the product's value proposition.** A reader should be able to open one file
and see exactly how significance is computed, with the assumptions written down next to the
code. That's worth more than a one-line import.

## Consequences

- The p-values are approximations (normal approximation for U; A&S for `erf`). For the sample
  sizes A/B tests reach (hundreds to thousands), the approximations are accurate to well within
  what matters for a 0.05 threshold. The limitations are documented in the code and in
  `docs/learning/06-ab-testing-and-stats.md`.
- Everything is unit-tested against values computed independently (e.g. SciPy) so the
  approximations are pinned, not hand-waved.
- No external math dependency; the whole thing is pure standard-library Go.
