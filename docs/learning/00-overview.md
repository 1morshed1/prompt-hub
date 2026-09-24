# 00 — Overview: what we're building and why

This folder is a **course in disguise**. Each file walks through one subsystem of PromptHub,
explains the problem it solves, the design we chose, and the actual code — so you can rebuild
it yourself or explain it in an interview.

Read them in order:

| # | Doc | What you'll learn |
|---|-----|-------------------|
| 00 | `00-overview.md` | The mental model for the whole system (this file) |
| 01 | `01-getting-started.md` | Run it locally, seed data, drive an A/B test |
| 02 | `02-architecture.md` | How the packages fit together; request lifecycle |
| 03 | `03-database-design.md` | The schema, why versions are immutable, tag constraints |
| 04 | `04-template-engine.md` | Lexer → parser → evaluator for `{{ }}` and `{% %}` |
| 05 | `05-versioning-and-diff.md` | Immutable versions, tagging, rollback, diffing |
| 06 | `06-ab-testing-and-stats.md` | Traffic splitting + chi-squared + Mann-Whitney U |
| 07 | `07-python-sdk.md` | Designing an ergonomic client library |
| 08 | `08-dashboard.md` | A zero-build-step dashboard over the JSON API |
| 09 | `09-gotchas-and-decisions-journal.md` | Real problems hit while building, and how they were fixed |

Decisions (the *why*, argued out) live one folder up in `docs/decisions/` as ADRs.

---

## The one-paragraph mental model

A **prompt** is a named thing (`order-assistant`). Editing it never overwrites anything —
each edit appends an immutable **version** (v1, v2, v3…). A **tag** (`production`) is a movable
pointer to one version, exactly like a git branch pointing at a commit. **Rollback** is just
re-pointing the `production` tag at an older version — no data is destroyed. To actually *use*
a prompt you **render** it: pick a version (by tag or number), run its templates through the
**template engine** with your variables, and get back LLM-ready `system` + `user` messages. An
**experiment** splits live traffic between two versions and records **results** (latency, cost,
rating). The **stats** package decides whether the difference is real, and if a variant wins
convincingly we **auto-promote** it by moving the `production` tag.

```
prompt ──has many──> versions (immutable)
   │                     ▲
   │                     │ points to one
   └──has many──> tags ──┘        (production, staging, ...)

experiment ── control_version + variant_version
     │
     └──has many──> results (latency, cost, rating)  ──> stats ──> winner ──> auto-promote (moves a tag)
```

## Why these five subsystems earn their place

- **Immutable versioning** is the whole reason the product exists. If edits overwrote, there'd
  be nothing to track, diff, or roll back to.
- **Tags** turn versions into something you can *deploy*. Code references `tag="production"`,
  not `version=7`, so ops can move production without a code change.
- **Template engine** exists because real prompts aren't strings — they have conditional
  sections ("if the user speaks Bengali, add this") and loops (over retrieved documents).
- **Experiments + statistics** are what separate this from a fancy notepad. "Variant looks
  better" is an opinion; "p < 0.05 over 4,000 requests" is a decision.
- **SDK** is the adoption surface. If integrating PromptHub is harder than a hardcoded string,
  nobody switches. The SDK has to be a genuine one-liner.

Now go to `01-getting-started.md` and get it running.
