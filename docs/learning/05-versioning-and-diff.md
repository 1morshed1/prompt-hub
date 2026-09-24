# 05 — Versioning, tags, rollback, and diff

This is the feature that justifies the whole product. Everything here lives in
`server/internal/store/` (`versions.go`, `tags.go`) and `server/internal/diff/`.

## Immutable versions: the golden rule

**Content rows are never updated.** `store.CreateVersion` only ever `INSERT`s. There is no
`UpdateVersion` method — by design. If you want to change a prompt, you create the next version.

### The race condition, and how we kill it

`version_number` must be a per-prompt sequence: 1, 2, 3, … with no gaps, no duplicates. The
number is computed as `MAX(version_number) + 1`. That's a classic **read-modify-write**, and
under concurrency it's a bug waiting to happen:

```
Time  Editor A                 Editor B
 t0   read MAX = 4
 t1                            read MAX = 4
 t2   insert v5  ✔
 t3                            insert v5  �’ UNIQUE violation (or a lost update)
```

We prevent it with a **row lock** taken at the start of the transaction:

```sql
SELECT true FROM prompts WHERE id = $1 FOR UPDATE;   -- lock this prompt
SELECT COALESCE(MAX(version_number),0)+1 FROM prompt_versions WHERE prompt_id = $1;
INSERT INTO prompt_versions (...) VALUES (...);
```

`FOR UPDATE` locks the parent `prompts` row. Editor B's transaction blocks at the `SELECT ...
FOR UPDATE` until Editor A commits, then reads `MAX = 5` and correctly inserts v6. The lock is
**per prompt row**, so edits to *different* prompts never contend. All three statements run in
one transaction (`store.withTx`), so the read-modify-write is atomic.

> Test that proves it: `TestVersioningIsMonotonic` in `store_test.go` creates three versions
> and asserts they number 1, 2, 3 and that the JSONB `model_config`/`variables` round-trip.

## Tags: the deploy layer

A tag is a movable pointer: `(prompt, "production") → version`. `store.SetTag` is a single
upsert:

```sql
INSERT INTO prompt_tags (prompt_id, tag, version_id, ...)
VALUES (...)
ON CONFLICT (prompt_id, tag)
DO UPDATE SET version_id = EXCLUDED.version_id, updated_at = now();
```

The `UNIQUE(prompt_id, tag)` primary key means the upsert can only ever *replace* the pointer,
never create a duplicate. One `production` per prompt, guaranteed by the database.

## Rollback is not a feature — it's the absence of one

There is no `Rollback` method. "Roll back production to v3" is just:

```go
store.SetTag(ctx, promptID, "production", v3.ID, who)
```

You move the pointer back. Nothing is copied, nothing is destroyed, and because v3 is immutable
it's byte-for-byte what it always was. `updated_at`/`updated_by` on the tag row record who
rolled back and when. This is the payoff of the git-style model: the hard operation becomes
trivial because the data model was right.

> Test: `TestTagsAndRollback` deploys v2, rolls back to v1, and asserts there's still exactly
> one production tag and that resolution follows the pointer.

## Version resolution: how "give me the prompt" picks a version

`store.ResolveVersion(promptID, tag, versionNumber)` implements this precedence:

1. **explicit version number** (`> 0`) → that exact version (reproducibility: pin a version)
2. **tag** (non-empty) → the version the tag points at (deploys: `tag="production"`)
3. **neither** → the latest version (convenience for dev)

This is exactly what lets the SDK expose `hub.get(name, tag="production")`,
`hub.get(name, version=3)`, and `hub.get(name)`.

## Diff: comparing two versions

`server/internal/diff` computes a **line-level diff** between two versions' templates using the
classic **Longest Common Subsequence (LCS)** dynamic-programming algorithm — the same core idea
behind `git diff`. For each of the system and user templates we:

1. split both versions into lines,
2. compute the LCS table (DP over an `m×n` grid),
3. walk the table backwards to emit a sequence of `equal` / `insert` / `delete` ops.

We also expose a **token-level** diff (split on whitespace/punctuation) for the fine-grained
"what words changed" view the dashboard shows. LCS is `O(m·n)` time and space; for prompt-sized
text (tens to hundreds of lines) that's instant, and it keeps the algorithm simple and
readable. See `diff.go` and its tests for the walkable implementation.

## Try it

```bash
# Requires a running server + seeded data (scripts/seed.sh)
curl -s "localhost:8080/api/v1/prompts/order-assistant/diff?from=1&to=2" | jq
```
