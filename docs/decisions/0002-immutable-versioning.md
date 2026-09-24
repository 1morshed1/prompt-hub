# ADR-0002 — Immutable versioning with movable tags

**Status:** Accepted
**Date:** 2026-09-24

## Context

The core promise of PromptHub is "you can always see what changed, when, and by whom, and roll
back instantly." That promise is only as strong as the data model behind it.

A naive design stores the current prompt text in one row and updates it in place. That throws
away history the moment someone edits — exactly the failure mode we exist to prevent.

## Decision

**Prompt content is never updated in place. Every edit inserts a new immutable
`prompt_versions` row** with an incrementing `version_number` scoped to the prompt.

**Deployment state is expressed by tags**, not by mutating content. A row in `prompt_tags`
maps `(prompt_id, tag) → version_id`, with a `UNIQUE(prompt_id, tag)` constraint so a prompt
has exactly one `production`, one `staging`, etc. Moving a tag is an upsert on that row.

This is the git model:

- `prompt_versions` ≈ commits (immutable, ordered).
- `prompt_tags` ≈ branches/tags (movable pointers).
- **Rollback** = point the `production` tag at an older version. Nothing is deleted, and the
  rollback itself is visible because the tag row's `updated_at` changes.

## Why version numbers are computed server-side, transactionally

`version_number` is `MAX(version_number)+1` for the prompt, computed inside the same
transaction that inserts the row, under a row lock on the parent prompt. This prevents two
concurrent edits from both grabbing "v5". See `docs/learning/05-versioning-and-diff.md` for
the exact SQL and the locking reasoning.

## Consequences

- Storage grows with every edit. That's the point, and prompt text is tiny; this is a
  non-issue at any realistic scale.
- Diffing "v3 vs v7" is a pure read of two immutable rows — no history reconstruction.
- Experiments reference `version_id`s that can never change underneath them, so recorded
  results always map to the exact text that produced them.
- Deleting a prompt is the only destructive operation, and it cascades to its versions/tags —
  an explicit, deliberate action, never a side effect of editing.
