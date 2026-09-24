package store

import (
	"context"
	"errors"

	"github.com/1morshed1/prompt-hub/server/internal/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// CreatePrompt inserts a new named prompt. Returns ErrConflict if (project, name)
// already exists.
func (s *Store) CreatePrompt(ctx context.Context, p model.Prompt) (model.Prompt, error) {
	const q = `
		INSERT INTO prompts (name, project, description, created_by)
		VALUES ($1, $2, $3, $4)
		RETURNING id, name, project, description, created_by, created_at, updated_at`
	var out model.Prompt
	err := s.pool.QueryRow(ctx, q, p.Name, defaultStr(p.Project, "default"), p.Description, p.CreatedBy).
		Scan(&out.ID, &out.Name, &out.Project, &out.Description, &out.CreatedBy, &out.CreatedAt, &out.UpdatedAt)
	if isUniqueViolation(err) {
		return model.Prompt{}, ErrConflict
	}
	return out, err
}

// GetPromptByName resolves a prompt by (project, name). This is the lookup the
// API uses since callers reference prompts by human name, not UUID.
func (s *Store) GetPromptByName(ctx context.Context, project, name string) (model.Prompt, error) {
	const q = `
		SELECT id, name, project, description, created_by, created_at, updated_at
		FROM prompts WHERE project = $1 AND name = $2`
	var p model.Prompt
	err := s.pool.QueryRow(ctx, q, defaultStr(project, "default"), name).
		Scan(&p.ID, &p.Name, &p.Project, &p.Description, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Prompt{}, ErrNotFound
	}
	return p, err
}

// ListPrompts returns all prompts in a project, newest first.
func (s *Store) ListPrompts(ctx context.Context, project string) ([]model.Prompt, error) {
	const q = `
		SELECT id, name, project, description, created_by, created_at, updated_at
		FROM prompts WHERE project = $1 ORDER BY created_at DESC`
	rows, err := s.pool.Query(ctx, q, defaultStr(project, "default"))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Prompt
	for rows.Next() {
		var p model.Prompt
		if err := rows.Scan(&p.ID, &p.Name, &p.Project, &p.Description, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// DeletePrompt removes a prompt and (via ON DELETE CASCADE) its versions, tags,
// experiments and results. This is the only destructive operation in the system.
func (s *Store) DeletePrompt(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM prompts WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// touchPrompt bumps updated_at, called within the same tx whenever a version is
// added so a prompt's "last activity" reflects edits, not just metadata changes.
func touchPrompt(ctx context.Context, tx pgx.Tx, promptID string) error {
	_, err := tx.Exec(ctx, `UPDATE prompts SET updated_at = now() WHERE id = $1`, promptID)
	return err
}

// --- small helpers shared across the store ---

func defaultStr(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

// isUniqueViolation reports whether err is a Postgres unique-constraint error
// (SQLSTATE 23505), which we surface as ErrConflict.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
