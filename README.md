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

```bash
# 1. Start a local Postgres and create the database (see scripts/dev_db.sh)
./scripts/dev_db.sh start

# 2. Run the server (applies migrations on boot)
cd server
export DATABASE_URL="postgres://postgres@localhost:5433/prompthub?sslmode=disable"
go run ./cmd/server

# 3. Open the dashboard
#    http://localhost:8080/  (served by the same Go process)
```

See [`docs/learning/01-getting-started.md`](docs/learning/01-getting-started.md) for the
full walkthrough, including seeding demo data and driving an A/B test end to end.

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
