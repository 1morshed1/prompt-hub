# ADR-0001 — Language and stack

**Status:** Accepted
**Date:** 2026-09-24

## Context

We need to build an API server, a template engine, statistical analysis, a client SDK, and a
dashboard. The audience is *AI engineers* — the people who would run this tool. It also has
to be operable like real infrastructure.

## Decision

- **Server: Go.** Compiles to a single static binary (trivial to deploy), has first-class
  concurrency for a serving path that will fan out to a database, and a standard library rich
  enough that the whole HTTP + DB + JSON stack needs almost no third-party framework. Go is
  also the lingua franca of infrastructure tooling (Kubernetes, Terraform, Docker), so it
  signals "I build tools" rather than "I build apps."
- **Database: PostgreSQL.** We need relational integrity (a version belongs to a prompt; a tag
  is unique per prompt) *and* schemaless flexibility for model config and custom metrics —
  Postgres gives both via foreign keys + `JSONB`. See ADR-0002.
- **SDK: Python.** That's where LLM application code overwhelmingly lives, so the SDK meets
  users where they are.
- **Dashboard: vanilla HTML/CSS/JS, no build step.** The dashboard is a thin read view over
  the JSON API. A React/Vite toolchain would add a build pipeline and node_modules for no
  real benefit at this scale. Keeping it buildless means the Go binary can serve it directly.

## Dependencies we allow

- `github.com/jackc/pgx/v5` — the de-facto Postgres driver/toolkit for Go.

Everything else (router, template engine, statistics, diffing) is standard library or written
by hand, deliberately, because those parts *are* the learning material.

## Consequences

- One `go build` produces a deployable server that also serves the dashboard. No node build.
- Writing the template engine and stats by hand is more code, but it's the code most worth
  reading. See ADR-0003 and ADR-0004.
- Go's static typing pushes us to model prompts/versions/experiments as explicit structs,
  which makes the domain model easy to follow.
