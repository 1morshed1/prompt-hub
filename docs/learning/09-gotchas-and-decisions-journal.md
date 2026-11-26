# 09 — Gotchas & decisions journal

The ADRs in `docs/decisions/` capture the *big* upfront choices. This doc is
different: it's the honest log of problems that surfaced **while building**, and
the reasoning used to fix each one. These are the moments you actually learn from,
because they're the ones that don't show up until code meets reality.

Read this to see how the design evolved under pressure — and to recognize the same
traps in your own projects.

---

## 1. Version numbers race under concurrency

**Symptom (anticipated):** `version_number = MAX(...)+1` is a read-modify-write. Two
editors saving at once could both read `MAX=4` and both try to insert v5 — one
crashes on the unique constraint, or worse, an edit is silently lost.

**Fix:** take a row lock on the parent prompt (`SELECT ... FROM prompts WHERE id=$1
FOR UPDATE`) at the start of the transaction, so concurrent version creation for the
*same* prompt serializes. Edits to *different* prompts don't contend (per-row lock).

**Lesson:** any "compute next number from the current max" needs a lock or a
sequence. The database has the tools (`FOR UPDATE`, unique constraints) — use them
instead of hoping the race won't happen. → `store/versions.go`, doc 05.

---

## 2. `log_result` 404'd when there was no experiment

**Symptom (caught by the SDK integration test):** rendering a prompt *outside* any
experiment returned a `request_id`, but calling `log_result(request_id, ...)`
returned 404. The render path only recorded a row when an experiment assigned the
request — so there was nothing to attach metrics to otherwise.

**Decision:** the spec wants **per-prompt analytics for every version**, not only
versions under test. So the right fix wasn't "make log_result tolerate a missing
row" — it was "record *every* render." Migration 0002 made `experiment_id`/`arm`
nullable and added `prompt_id` to the results table; `RecordUsage` now writes a row
for every render, with experiment fields set only when assigned.

**Lesson:** a bug in one layer (SDK) revealed a *modeling* gap (we weren't capturing
the data the product promised). Fixing the model, not just the symptom, unlocked the
`/analytics` endpoint too. → migration `0002`, `store/analytics.go`, doc 03.

---

## 3. Parallel test packages corrupted each other's database

**Symptom:** `go test ./...` intermittently failed with foreign-key violations and
"not found" errors — but only when the `store` and `experiment` integration tests
ran together. Alone, each passed.

**Root cause:** `go test` runs *different packages* in parallel, and both integration
suites pointed at the same `TEST_DATABASE_URL` and `TRUNCATE`d the same tables. One
package wiped the other's data mid-test.

**Fix:** a shared `testutil.Setup` that holds a **Postgres advisory lock** on a
dedicated connection for each test's duration. Only one test anywhere runs its
truncate-and-work sequence at a time; within a package tests were already serial, so
this only ever blocks across packages.

**Lesson:** shared mutable state (a test DB) plus parallelism equals flakes. Either
isolate the state (a schema/DB per worker) or serialize access (a lock). We chose the
lock for simplicity. Verified by running the suite repeatedly. → `testutil/db.go`.

---

## 4. A slash in a prompt name broke routing

**Symptom:** the seed used `shared/safety-preamble` as an include target. Creating a
version 404'd with Go's default "404 page not found" (not our JSON error).

**Root cause:** routes use a single path segment for the name
(`/prompts/{name}/versions`). A `/` in the name spilled into extra segments, so no
route matched.

**Decision:** rather than complicate routing (multi-segment wildcards conflict with
sub-paths like `/versions`), constrain names to a single segment and **validate it
server-side** — reject names containing `/` or spaces with a clear 400. The demo uses
a flat name (`safety-preamble`).

**Lesson:** identifiers that appear in URLs have URL constraints. Enforce them at the
boundary with a helpful message, instead of letting a router mismatch produce a
confusing 404 later. → `api/prompts.go`.

---

## 5. Which statistical test? (not a bug — a correctness decision)

It's tempting to use one test (a t-test) for everything. That would be **wrong**:

- Ratings are categorical (up/down) → chi-squared test of independence.
- Latency is continuous but heavily right-skewed → the t-test's normality assumption
  fails, so use the non-parametric **Mann-Whitney U**.

**Lesson:** "statistically significant" is only meaningful if the test matches the
data's shape. Picking the test *is* the analysis. We pinned both implementations to
SciPy reference values so the from-scratch math is trustworthy, not just plausible.
→ `stats/`, doc 06, ADR-0004.

---

## 6. JSON numbers are always floats

**Symptom (design-time):** `{{ count }}` where `count` came from JSON rendered as
`3.0`, not `3`. JSON has one number type, which decodes to Go `float64`.

**Fix:** `stringify` prints whole-valued floats without the trailing `.0`.

**Lesson:** the boundary between JSON and a typed language is full of small surprises
(numbers, nulls, missing keys). Handle them once, centrally (here, in `values.go`),
so every call site behaves consistently. → `template/values.go`, doc 04.

---

## Meta-lesson

Notice how many of these were found by **tests** (2, 3, 5) or by **actually running
the thing** (4) rather than by reading code. The workflow throughout this project was:
build a slice, write a test or drive it live, let reality find the gap, fix the model
not just the symptom, document the decision. That loop is the real deliverable.
