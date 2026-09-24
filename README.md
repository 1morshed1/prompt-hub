# PromptHub

> **Git + LaunchDarkly for LLM prompts.** A platform where prompts are first-class,
> version-controlled entities with performance analytics, A/B testing, and instant rollback.

PromptHub solves a problem every AI team hits: prompts live as scattered Python strings
with no version history, no performance tracking, and no safe way to change them. When a
system prompt edit drops accuracy 15%, nobody knows *when* it happened or *who* changed *what*.

This repo is built to be **read as a learning project**. Every non-trivial decision is
written down in [`docs/decisions/`](docs/decisions/) (Architecture Decision Records) and
every subsystem has a walkthrough in [`docs/learning/`](docs/learning/). Start there.

---

## What it does

| Capability | Where |
|---|---|
| Create prompts; every edit is a new **immutable version** (v1, v2, …) | `server/internal/store` |
| **Tag** versions (`production`, `staging`) — one tag → one version | `server/internal/store` |
| **Diff** any two versions (line + token level) | `server/internal/diff` |
| **Rollback** instantly (rollback = tag `production` at an old version) | `server/internal/store` |
| **Template engine**: variables, `{% if %}`, `{% for %}`, `{% include %}`, type validation | `server/internal/template` |
| **Render** endpoint: resolve version → fill variables → return LLM-ready messages | `server/internal/api` |
| **A/B experiments**: deterministic traffic split between two versions | `server/internal/experiment` |
| **Statistical significance**: chi-squared (ratings), Mann-Whitney U (latency) | `server/internal/stats` |
| **Auto-promote** a winning variant to `production` | `server/internal/experiment` |
| **Python SDK**: `hub.get(...).render(...)` one-liner + `hub.log_result(...)` | `sdk/python` |
| **Web dashboard**: version timeline, diffs, experiment comparison | `web` |

## Repository layout

```
prompt-hub/
├── README.md                 ← you are here
├── docs/
│   ├── learning/             ← subsystem walkthroughs (read these to learn)
│   └── decisions/            ← ADRs: why each choice was made
├── server/                   ← Go API server (the core)
│   ├── cmd/server/           ← main entrypoint
│   ├── internal/             ← config, db, store, template, stats, experiment, api
│   └── migrations/           ← SQL schema, applied in order
├── sdk/python/               ← pip-installable client
├── web/                      ← static dashboard (vanilla JS, no build step)
└── scripts/                  ← dev helpers (local postgres, seed data)
```

## Quick start

**With Docker (one command):**

```bash
docker compose up --build      # Postgres + server, dashboard at http://localhost:8080
python3 scripts/seed.py        # (optional) seed demo data + drive an A/B test
```

**Locally (no Docker):**

```bash
./scripts/dev_db.sh start      # throwaway local Postgres on :5433 (no root, no Docker)
make run                       # runs the server; applies migrations; serves API + dashboard
# in another shell:
python3 scripts/seed.py        # seed demo data
open http://localhost:8080/    # the dashboard
```

`make help` lists every developer command (run, build, test, seed, docker-up, …).

See [`docs/learning/01-getting-started.md`](docs/learning/01-getting-started.md) for the
full walkthrough, including driving an A/B test end to end.

## Tests

```bash
make test        # Go tests (unit + DB integration) and the Python SDK tests
```

Go integration tests need a database — set `TEST_DATABASE_URL` (the Makefile
defaults it to the local `scripts/dev_db.sh` instance's `prompthub_test`). Pure
unit tests (template engine, statistics, diff) run with no database.

## Learn the codebase (start here)

This repo is meant to be *read*. The [`docs/learning/`](docs/learning/) folder is a
short course, one file per subsystem:

| # | Doc | Teaches |
|---|-----|---------|
| 00 | [overview](docs/learning/00-overview.md) | the mental model for the whole system |
| 01 | [getting started](docs/learning/01-getting-started.md) | run it, seed it, drive an A/B test |
| 02 | [architecture](docs/learning/02-architecture.md) | how the packages fit; a request's lifecycle |
| 03 | [database design](docs/learning/03-database-design.md) | the schema, immutable versions, tag constraints |
| 04 | [template engine](docs/learning/04-template-engine.md) | lexer → parser → evaluator, from scratch |
| 05 | [versioning & diff](docs/learning/05-versioning-and-diff.md) | immutable versions, tags, rollback, LCS diff |
| 06 | [A/B testing & stats](docs/learning/06-ab-testing-and-stats.md) | chi-squared + Mann-Whitney U, explained |
| 07 | [Python SDK](docs/learning/07-python-sdk.md) | designing an ergonomic client library |
| 08 | [dashboard](docs/learning/08-dashboard.md) | a buildless SPA over the JSON API |

The **why** behind each major choice is argued in
[`docs/decisions/`](docs/decisions/) (Architecture Decision Records). The full HTTP
surface is in [`docs/api-reference.md`](docs/api-reference.md). The git history is
also part of the material: each commit ships one subsystem with its learning doc.

## Tech choices at a glance

- **Go** for the server — one static binary, great concurrency, the target audience
  (AI *engineers*) reads Go comfortably. ([ADR-0001](docs/decisions/0001-language-and-stack.md))
- **PostgreSQL** — immutable version rows + JSONB for flexible model config / metrics.
  ([ADR-0002](docs/decisions/0002-immutable-versioning.md))
- **Hand-written template engine** — the plan needs custom `{% %}` syntax and this is the
  most instructive part of the codebase. ([ADR-0003](docs/decisions/0003-template-engine.md))
- **Hand-written statistics** — no scientific-computing dependency in a Go service; the
  math is the teaching point. ([ADR-0004](docs/decisions/0004-statistics-from-scratch.md))
- **Standard-library HTTP router** (Go 1.22+ pattern routing) — no framework needed.
  ([ADR-0005](docs/decisions/0005-stdlib-http.md))

---

*Built as a portfolio + learning project. Follow the commit history — each commit ships one
subsystem together with its learning doc.*
