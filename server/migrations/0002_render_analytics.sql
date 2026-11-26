-- 0002_render_analytics.sql
--
-- Motivation: the spec calls for per-prompt analytics (latency / cost / token
-- usage and satisfaction trend PER VERSION), not only for versions under an A/B
-- test. To support that, EVERY render records a usage row — even when no
-- experiment is running — so we can aggregate metrics per version regardless.
--
-- That means an experiment_results row is no longer necessarily tied to an
-- experiment. We relax the schema accordingly:
--   * experiment_id becomes nullable (a plain render has no experiment),
--   * arm becomes nullable (only experiment renders are assigned an arm),
--   * a prompt_id column is added so per-prompt analytics is a simple, indexed
--     query without joining through prompt_versions.
--
-- The table keeps its name for continuity; think of it as "render results",
-- where experiment_id is set only for the subset that belong to an experiment.

ALTER TABLE experiment_results ALTER COLUMN experiment_id DROP NOT NULL;
ALTER TABLE experiment_results ALTER COLUMN arm DROP NOT NULL;

ALTER TABLE experiment_results
    ADD COLUMN prompt_id UUID REFERENCES prompts(id) ON DELETE CASCADE;

-- Backfill prompt_id for any pre-existing rows via the version they reference.
UPDATE experiment_results er
SET prompt_id = pv.prompt_id
FROM prompt_versions pv
WHERE er.version_id = pv.id AND er.prompt_id IS NULL;

CREATE INDEX idx_results_prompt_version ON experiment_results (prompt_id, version_id);
