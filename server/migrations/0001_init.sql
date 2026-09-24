-- 0001_init.sql — the core PromptHub schema.
--
-- Design notes live in docs/decisions/0002-immutable-versioning.md and
-- docs/learning/03-database-design.md. The short version:
--   * prompts          = the named entity (like a git repo)
--   * prompt_versions  = immutable snapshots (like git commits), never UPDATEd
--   * prompt_tags      = movable pointers to a version (like git branches/tags)
--   * experiments      = an A/B test between two versions of one prompt
--   * experiment_results = one row per served request, the raw analytics data

-- Enable gen_random_uuid() (pgcrypto ships with Postgres; no extra install).
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- ─────────────────────────────────────────────────────────────────────────────
-- prompts: the named entity. "order-assistant", "refund-classifier", etc.
-- ─────────────────────────────────────────────────────────────────────────────
CREATE TABLE prompts (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT        NOT NULL,
    project     TEXT        NOT NULL DEFAULT 'default',
    description TEXT        NOT NULL DEFAULT '',
    created_by  TEXT        NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- A name is unique within a project: two teams can both have "assistant".
    UNIQUE (project, name)
);

-- ─────────────────────────────────────────────────────────────────────────────
-- prompt_versions: immutable snapshots. One row per edit; content is NEVER updated.
-- version_number is 1-based and monotonic *per prompt* (see store.CreateVersion for
-- the transactional MAX+1 logic that keeps this race-free).
-- ─────────────────────────────────────────────────────────────────────────────
CREATE TABLE prompt_versions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    prompt_id       UUID        NOT NULL REFERENCES prompts(id) ON DELETE CASCADE,
    version_number  INT         NOT NULL,
    system_template TEXT        NOT NULL DEFAULT '',   -- "You are a {{role}} assistant"
    user_template   TEXT        NOT NULL DEFAULT '',   -- "Answer about {{topic}}: {{query}}"
    model_config    JSONB       NOT NULL DEFAULT '{}', -- {model, temperature, max_tokens}
    variables       JSONB       NOT NULL DEFAULT '[]', -- [{name:"role", type:"string", required:true}]
    commit_message  TEXT        NOT NULL DEFAULT '',   -- "Improved refund tone"
    created_by      TEXT        NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (prompt_id, version_number)
);

CREATE INDEX idx_prompt_versions_prompt ON prompt_versions (prompt_id, version_number DESC);

-- ─────────────────────────────────────────────────────────────────────────────
-- prompt_tags: movable named pointers. UNIQUE(prompt_id, tag) enforces "one
-- production tag per prompt". Moving/rolling back a tag is an UPSERT on this row.
-- ─────────────────────────────────────────────────────────────────────────────
CREATE TABLE prompt_tags (
    prompt_id  UUID        NOT NULL REFERENCES prompts(id) ON DELETE CASCADE,
    tag        TEXT        NOT NULL,
    version_id UUID        NOT NULL REFERENCES prompt_versions(id) ON DELETE CASCADE,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_by TEXT        NOT NULL DEFAULT '',
    PRIMARY KEY (prompt_id, tag)
);

CREATE INDEX idx_prompt_tags_version ON prompt_tags (version_id);

-- ─────────────────────────────────────────────────────────────────────────────
-- experiments: an A/B test between two versions of the same prompt.
-- traffic_split is the fraction (0..1) of traffic routed to the VARIANT.
-- ─────────────────────────────────────────────────────────────────────────────
CREATE TABLE experiments (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    prompt_id          UUID        NOT NULL REFERENCES prompts(id) ON DELETE CASCADE,
    name               TEXT        NOT NULL DEFAULT '',
    control_version_id UUID        NOT NULL REFERENCES prompt_versions(id),
    variant_version_id UUID        NOT NULL REFERENCES prompt_versions(id),
    traffic_split      DOUBLE PRECISION NOT NULL DEFAULT 0.5
                       CHECK (traffic_split >= 0 AND traffic_split <= 1),
    status             TEXT        NOT NULL DEFAULT 'running'
                       CHECK (status IN ('running', 'stopped', 'promoted')),
    auto_promote       BOOLEAN     NOT NULL DEFAULT false,
    -- Minimum sample size per arm before auto-promote is even considered.
    min_samples        INT         NOT NULL DEFAULT 100,
    started_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    ended_at           TIMESTAMPTZ
);

CREATE INDEX idx_experiments_prompt ON experiments (prompt_id);

-- Only one running experiment per prompt keeps assignment unambiguous.
CREATE UNIQUE INDEX idx_one_running_experiment_per_prompt
    ON experiments (prompt_id) WHERE status = 'running';

-- ─────────────────────────────────────────────────────────────────────────────
-- experiment_results: the raw analytics rows the stats package aggregates.
-- One row per served request. request_id is the idempotency/join key the SDK
-- gets back from /render and later references in /log_result.
-- ─────────────────────────────────────────────────────────────────────────────
CREATE TABLE experiment_results (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    experiment_id  UUID        NOT NULL REFERENCES experiments(id) ON DELETE CASCADE,
    version_id     UUID        NOT NULL REFERENCES prompt_versions(id),
    request_id     UUID        NOT NULL,
    arm            TEXT        NOT NULL CHECK (arm IN ('control', 'variant')),
    latency_ms     INT,
    tokens_used    INT,
    cost_usd       DOUBLE PRECISION,
    user_rating    INT         CHECK (user_rating IN (0, 1)),  -- thumbs down / up
    custom_metrics JSONB       NOT NULL DEFAULT '{}',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (request_id)
);

CREATE INDEX idx_experiment_results_exp_arm ON experiment_results (experiment_id, arm);
