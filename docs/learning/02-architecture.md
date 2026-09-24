# 02 — Architecture: how the packages fit together

PromptHub's server is a small number of packages with a strict dependency
direction. Understanding that direction is most of understanding the codebase.

## Package map

```
cmd/server            main(): config → db → migrate → api.Server → ListenAndServe
   │
   ▼
internal/api          HTTP layer: routing, middleware, handlers, JSON encoding
   │        │
   │        └────────────────────────┐
   ▼                                  ▼
internal/experiment            internal/template     internal/diff
   │  (assignment,               (render prompts)     (version diffs)
   │   analyze, auto-promote)          │
   │        │                          │
   ▼        ▼                          │
internal/stats   internal/store  ◄─────┘ (via api's include resolver)
   (chi², MWU)     (all SQL)
                       │
                       ▼
internal/db  ──►  PostgreSQL          internal/model (pure types, imported everywhere)
```

**The rule:** dependencies point *downward*. `api` knows about everything;
`store` knows only about `db` and `model`; `model` knows about nothing. There are
no cycles. This is what keeps each layer testable in isolation — `stats` and
`template` have zero dependencies, so their tests are pure; `store` needs only a
database; `api` is the only layer that needs all of it wired together.

## The layers

| Layer | Responsibility | Depends on |
|-------|----------------|------------|
| `model` | Pure domain types (`Prompt`, `Version`, `Experiment`, …) | nothing |
| `db` | Connection pool + migration runner | `pgx`, `migrations` |
| `store` | Every SQL query, behind typed methods | `db`, `model` |
| `template` | The prompt template engine | `model` (for `Variable`) |
| `stats` | Significance tests | nothing |
| `diff` | LCS version diffs | nothing |
| `experiment` | A/B workflow: assign, analyze, promote | `store`, `stats`, `model` |
| `api` | HTTP handlers, routing, middleware | all of the above |
| `config` | Env-var configuration | nothing |

## The request lifecycle: a render, end to end

Follow `POST /api/v1/prompts/order-assistant/render` through the stack — it
touches almost everything:

1. **Middleware** (`api/middleware.go`): `recover → log → cors` wrap the request.
2. **Router** (`api/server.go`): the Go 1.22 mux matches the method+path and calls
   `Server.render`.
3. **Handler** (`api/render.go`):
   a. `store.GetPromptByName` resolves the prompt (`store` → `db` → Postgres).
   b. **Version selection** (`selectVersion`): explicit version pins; else a
      running experiment assigns an arm via `experiment.Assign` (which hashes the
      unit id) and records the assignment; else `store.ResolveVersion` picks by
      tag or latest.
   c. **Validation**: `template.BuildContext` checks the caller's variables against
      the version's declared schema, filling defaults, collecting all errors.
   d. **Rendering**: `engine.Render` runs the system and user templates. If they
      contain `{% include %}`, the engine calls back into the api package's
      **DB-backed include resolver** (`api/include.go`), which fetches another
      prompt's content from the `store`.
   e. The handler writes a JSON response with the rendered messages, model config,
      a `request_id`, and (if assigned) the experiment id and arm.
4. **Later**, the SDK calls `POST /api/v1/results` with that `request_id` and the
   outcome (latency, rating). `store.LogResult` merges it into the assignment row.
5. **Analysis** happens on demand: `GET /experiments/{id}/report` loads results via
   `store`, runs `experiment.Analyze` (which calls `stats`), and — if auto-promote
   is on and the gates pass — moves the `production` tag via `store.SetTag`.

## Why this shape

- **Handlers are thin.** They parse input, call one or two domain methods, and
  encode output. All the interesting logic lives in `store`, `template`,
  `experiment`, and `stats`, where it's unit-testable without HTTP.
- **`store` is the only place with SQL.** If you want to know how data is
  persisted, there's exactly one directory to read.
- **The template engine is pure.** It takes strings and a context and returns a
  string; the *only* impurity (fetching includes from a database) is injected as
  an interface, so the engine's tests never touch a DB.
- **Statistics are isolated and pure.** `Analyze` is a plain function of an
  experiment plus a slice of results — no clock, no DB, no randomness — so its
  tests are deterministic.

## Where to read next

- The data model those `store` queries run against → `03-database-design.md`
- The template engine `engine.Render` drives → `04-template-engine.md`
- The versioning/tag logic behind `selectVersion` → `05-versioning-and-diff.md`
- The statistics behind `Analyze` → `06-ab-testing-and-stats.md`
- The full HTTP surface → `docs/api-reference.md`
