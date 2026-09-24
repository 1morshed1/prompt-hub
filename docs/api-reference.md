# PromptHub API reference

Base URL: `http://localhost:8080`. All request/response bodies are JSON. Every
prompt-scoped route accepts an optional `?project=<name>` query param (default
`default`). Errors use `{"error": "..."}` with an appropriate HTTP status.

## Health

| Method | Path | Description |
|---|---|---|
| GET | `/healthz` | Liveness check → `{"status":"ok"}` |

## Prompts

### `POST /api/v1/prompts`
Create a prompt.
```json
{ "name": "order-assistant", "description": "helps with orders", "created_by": "you" }
```
→ `201` the created prompt. `409` if the name already exists in the project.

### `GET /api/v1/prompts`
List prompts in the project → `{ "prompts": [...] }`.

### `GET /api/v1/prompts/{name}`
Prompt detail: the prompt plus its `tags` and full `versions` list.

### `DELETE /api/v1/prompts/{name}`
Delete a prompt and everything under it (versions, tags, experiments, results).
→ `204`.

## Versions (immutable)

### `POST /api/v1/prompts/{name}/versions`
Append a new version. The server assigns the version number.
```json
{
  "system_template": "You are a {{role}} assistant.{% if language == \"bn\" %} Reply in Bengali.{% endif %}",
  "user_template": "About {{topic}}: {{query}}",
  "model_config": { "model": "gpt-4o", "temperature": 0.2 },
  "variables": [
    { "name": "role", "type": "string", "required": true },
    { "name": "language", "type": "string", "required": false, "default": "en" }
  ],
  "commit_message": "initial version"
}
```
→ `201` the created version (with `version_number`).

### `GET /api/v1/prompts/{name}/versions`
List versions, newest first → `{ "versions": [...] }`.

### `GET /api/v1/prompts/{name}/versions/{number}`
Get one version by number.

## Tags (deploy / rollback)

### `GET /api/v1/prompts/{name}/tags`
List tags → `{ "tags": [...] }`.

### `PUT /api/v1/prompts/{name}/tags/{tag}`
Point a tag at a version. This is **both deploy and rollback** — move it forward
to release, backward to roll back.
```json
{ "version_number": 3, "updated_by": "you" }
```
→ `200` the updated tag.

## Diff

### `GET /api/v1/prompts/{name}/diff?from=1&to=2&mode=line`
Diff two versions. `mode` is `line` (default) or `token`. Returns per-template
`segments` (`equal`/`insert`/`delete`) and `stats` (`added`/`removed`/`unchanged`)
for both the system and user templates.

## Render (the serving path)

### `POST /api/v1/prompts/{name}/render`
Resolve a version, validate variables, render templates, and return LLM-ready
messages. Version selection precedence: explicit `version` > running experiment >
`tag` > latest.
```json
{
  "tag": "production",
  "version": 0,
  "unit_id": "user-123",
  "variables": { "role": "support", "topic": "refunds", "query": "Where is my order?", "language": "bn" }
}
```
→ `200`
```json
{
  "prompt_name": "order-assistant",
  "version_number": 2,
  "system": "You are a support assistant. ... Reply in Bengali.",
  "user": "About refunds: Where is my order?",
  "model_config": { "model": "gpt-4o", "temperature": 0.3 },
  "request_id": "…",
  "experiment_id": "…",   // present only when a running experiment assigned this request
  "arm": "variant"        // present only when assigned
}
```
`422` if variable validation fails (missing required / wrong type), listing all
problems.

## Experiments

### `POST /api/v1/experiments`
Start an A/B test. Versions are referenced by number.
```json
{
  "prompt": "order-assistant",
  "name": "concise rewrite",
  "control_version": 1,
  "variant_version": 2,
  "traffic_split": 0.5,
  "auto_promote": true,
  "min_samples": 100
}
```
→ `201` the experiment. `409` if the prompt already has a running experiment.

### `GET /api/v1/experiments?prompt=order-assistant`
List experiments (optionally filtered by prompt) → `{ "experiments": [...] }`.

### `GET /api/v1/experiments/{id}`
Get one experiment.

### `GET /api/v1/experiments/{id}/report`
The full statistical analysis. If `auto_promote` is on, evaluating the report also
performs promotion when the gates pass.
```json
{
  "report": {
    "control": { "sample_size": 76, "avg_rating": 0.33, "avg_latency_ms": 1182, ... },
    "variant": { "sample_size": 84, "avg_rating": 0.69, "avg_latency_ms": 879, ... },
    "rating_p_value": 0.00001, "rating_significant": true,
    "latency_p_value": 0.0, "latency_significant": true,
    "winner": "variant",
    "recommendation": "Promote the variant: ..."
  },
  "promotion": { "promoted": true, "reason": "variant won significantly; promoted to production" }
}
```

### `POST /api/v1/experiments/{id}/stop`
Stop an experiment without promoting → `{"status":"stopped"}`.

### `POST /api/v1/experiments/{id}/promote`
Manually promote the variant to production and close the experiment →
`{"status":"promoted"}`.

## Results (the feedback loop)

### `POST /api/v1/results`
Log outcome metrics against a `request_id` from a prior render. Fields are optional
and can be sent across multiple calls (latency now, the user's rating later).
```json
{ "request_id": "…", "latency_ms": 950, "rating": 1, "cost_usd": 0.0012, "tokens_used": 320 }
```
→ `200 {"status":"logged"}`. `404` if the request id is unknown. `rating` must be
`0` or `1`.
