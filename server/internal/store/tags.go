package store

import (
	"context"
	"errors"

	"github.com/1morshed1/prompt-hub/server/internal/model"
	"github.com/jackc/pgx/v5"
)

// SetTag points a tag (e.g. "production") at a version. This single upsert is the
// mechanism behind both *deploying* a new version and *rolling back* to an old
// one — in both cases you're just moving the pointer. The UNIQUE(prompt_id, tag)
// primary key means the upsert either inserts the first pointer or overwrites the
// existing one; a prompt can never end up with two "production" tags.
//
// Rollback has no dedicated code path on purpose: "roll back to v3" is literally
// SetTag(prompt, "production", v3's id). Fewer concepts, less to get wrong.
func (s *Store) SetTag(ctx context.Context, promptID, tag, versionID, updatedBy string) (model.Tag, error) {
	const q = `
		INSERT INTO prompt_tags (prompt_id, tag, version_id, updated_by, updated_at)
		VALUES ($1, $2, $3, $4, now())
		ON CONFLICT (prompt_id, tag)
		DO UPDATE SET version_id = EXCLUDED.version_id,
		              updated_by = EXCLUDED.updated_by,
		              updated_at = now()
		RETURNING prompt_id, tag, version_id, updated_at, updated_by`
	var t model.Tag
	err := s.pool.QueryRow(ctx, q, promptID, tag, versionID, updatedBy).
		Scan(&t.PromptID, &t.Tag, &t.VersionID, &t.UpdatedAt, &t.UpdatedBy)
	return t, err
}

// GetTag resolves a tag to the version it currently points at.
func (s *Store) GetTag(ctx context.Context, promptID, tag string) (model.Tag, error) {
	const q = `
		SELECT prompt_id, tag, version_id, updated_at, updated_by
		FROM prompt_tags WHERE prompt_id = $1 AND tag = $2`
	var t model.Tag
	err := s.pool.QueryRow(ctx, q, promptID, tag).
		Scan(&t.PromptID, &t.Tag, &t.VersionID, &t.UpdatedAt, &t.UpdatedBy)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Tag{}, ErrNotFound
	}
	return t, err
}

// ListTags returns every tag on a prompt.
func (s *Store) ListTags(ctx context.Context, promptID string) ([]model.Tag, error) {
	const q = `
		SELECT prompt_id, tag, version_id, updated_at, updated_by
		FROM prompt_tags WHERE prompt_id = $1 ORDER BY tag`
	rows, err := s.pool.Query(ctx, q, promptID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Tag
	for rows.Next() {
		var t model.Tag
		if err := rows.Scan(&t.PromptID, &t.Tag, &t.VersionID, &t.UpdatedAt, &t.UpdatedBy); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ResolveVersion is the workhorse behind rendering: given a prompt and a
// selector, return the concrete version to use. The selector precedence is:
//
//	explicit version number (> 0)  →  that exact version
//	tag (non-empty)                →  the version the tag points at
//	neither                        →  the latest version
//
// This is what lets SDK callers say get("order-assistant", tag="production")
// or get("order-assistant", version=3) or just get("order-assistant").
func (s *Store) ResolveVersion(ctx context.Context, promptID, tag string, versionNumber int) (model.Version, error) {
	switch {
	case versionNumber > 0:
		return s.GetVersionByNumber(ctx, promptID, versionNumber)
	case tag != "":
		t, err := s.GetTag(ctx, promptID, tag)
		if err != nil {
			return model.Version{}, err
		}
		return s.GetVersionByID(ctx, t.VersionID)
	default:
		return s.GetLatestVersion(ctx, promptID)
	}
}
