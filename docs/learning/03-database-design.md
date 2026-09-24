# 03 — Database design

The schema is the backbone of PromptHub. If you understand these five tables, you understand
the product. The full DDL is `server/migrations/0001_init.sql`; this doc explains the *why*.

## The five tables

```
prompts ──1:N──> prompt_versions ──referenced by──> prompt_tags
   │                    │                                 (movable pointers)
   │                    │
   └──1:N──> experiments ──1:N──> experiment_results
```

### 1. `prompts` — the named entity

Think of a row here as a git repository: it has a name, a project, a description, but no
content of its own. Content lives in versions.

```sql
UNIQUE (project, name)
```

A prompt name is unique *within a project*, not globally, so two teams can each have an
`assistant` prompt without colliding.

### 2. `prompt_versions` — immutable snapshots

This is the table that makes the product possible. **Rows here are never `UPDATE`d.** Every
edit inserts a new row with the next `version_number`.

```sql
version_number  INT   NOT NULL,       -- 1, 2, 3 ... per prompt
system_template TEXT,                 -- "You are a {{role}} assistant"
user_template   TEXT,                 -- "Answer about {{topic}}: {{query}}"
model_config    JSONB,                -- {"model":"gpt-4o","temperature":0.2}
variables       JSONB,                -- [{"name":"role","type":"string","required":true}]
commit_message  TEXT,                 -- "Improved refund tone" (like a git commit msg)
UNIQUE (prompt_id, version_number)
```

**Why `JSONB` for `model_config` and `variables`?** These are structured but open-ended.
Model config today is `{model, temperature, max_tokens}` but tomorrow someone wants `top_p` or
`stop` sequences. A rigid column-per-field schema would need a migration for every new knob.
`JSONB` stores the shape flexibly *and* stays queryable (Postgres can index into JSONB). This
is the classic "relational skeleton, JSON for the soft tissue" pattern.

**Why `variables` is stored, not inferred?** We could parse the templates to discover `{{role}}`
etc., but storing an explicit schema lets us (a) declare types for validation, (b) mark
required vs optional, and (c) show a nice form in the dashboard. The template engine's
validation pass (doc 04) checks caller input against exactly this schema.

### 3. `prompt_tags` — movable pointers (the deploy layer)

```sql
PRIMARY KEY (prompt_id, tag)   -- one "production" per prompt, enforced by the DB
```

A tag maps `(prompt, "production") → some version`. This is a git branch. The primary key
guarantees a prompt can't have two `production` tags — the database, not application code,
enforces the invariant.

**Deploying** a new version = upsert this row to point at it.
**Rolling back** = upsert this row to point at an older version. No content is copied or
destroyed; you're just moving a pointer. `updated_at` records *when* the pointer last moved,
so the rollback itself is auditable.

### 4. `experiments` — an A/B test

```sql
control_version_id UUID,   -- the incumbent
variant_version_id UUID,   -- the challenger
traffic_split      DOUBLE,  -- fraction (0..1) routed to the VARIANT
status             TEXT CHECK (status IN ('running','stopped','promoted')),
auto_promote       BOOLEAN,
min_samples        INT      -- gate before auto-promote is considered
```

Two constraints do real work here:

- `CHECK (traffic_split >= 0 AND traffic_split <= 1)` — a split can't be nonsense.
- A **partial unique index** `WHERE status = 'running'` allows only one *running* experiment
  per prompt. You can have many stopped/promoted experiments in history, but assignment is
  never ambiguous because at most one is live.

```sql
CREATE UNIQUE INDEX idx_one_running_experiment_per_prompt
    ON experiments (prompt_id) WHERE status = 'running';
```

Partial indexes are a Postgres superpower: the uniqueness rule applies only to the rows you
care about.

### 5. `experiment_results` — the raw analytics rows

One row per served request. This is the data the `stats` package aggregates.

```sql
request_id  UUID UNIQUE,     -- the join key the SDK gets from /render, uses in /log_result
arm         TEXT CHECK (arm IN ('control','variant')),
latency_ms  INT,
cost_usd    DOUBLE,
user_rating INT CHECK (user_rating IN (0,1)),  -- thumbs down / up
custom_metrics JSONB
```

`request_id` is `UNIQUE`, which makes result logging idempotent: the SDK renders (creating the
row's identity), then later logs the outcome against the same `request_id`. Metrics can be
nullable because they arrive at different times — latency is known immediately, the user's
thumbs-up may come minutes later.

## The migration runner

Migrations live in `server/migrations/*.sql` and are **embedded into the binary**
(`migrations/embed.go` uses `//go:embed *.sql`), so the compiled server carries its own schema
— no separate files to deploy. On boot, `db.Migrate`:

1. ensures a `schema_migrations(name)` bookkeeping table exists,
2. lists embedded `*.sql` files sorted by name (`0001_` before `0002_`),
3. runs any not yet recorded, **each in its own transaction**, recording the name on success.

Forward-only and about 60 lines. That's intentional — the mechanism is meant to be readable in
one sitting. See `server/internal/db/migrate.go`.

## Try it yourself

```bash
./scripts/dev_db.sh start
cd server && DATABASE_URL=$(../scripts/dev_db.sh url) go run ./cmd/server   # applies 0001_init
./scripts/dev_db.sh psql -c '\dt'    # see the six tables (five + schema_migrations)
```
