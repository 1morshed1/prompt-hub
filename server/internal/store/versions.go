package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/1morshed1/prompt-hub/server/internal/model"
	"github.com/jackc/pgx/v5"
)

// CreateVersion appends a new immutable version to a prompt and returns it with
// its assigned version_number.
//
// The version_number must be strictly monotonic per prompt with no gaps or
// duplicates, even under concurrent edits. We guarantee that by doing the whole
// thing in one transaction and taking a row lock on the parent prompt first:
//
//	SELECT ... FROM prompts WHERE id = $1 FOR UPDATE
//
// That lock serializes concurrent CreateVersion calls for the *same* prompt:
// the second caller blocks until the first commits, then reads the updated MAX.
// Without it, two callers could both read MAX=4 and both try to insert v5 — one
// would fail the UNIQUE(prompt_id, version_number) constraint. Locking makes the
// read-modify-write atomic. (Callers editing *different* prompts don't contend,
// since the lock is per-row.)
func (s *Store) CreateVersion(ctx context.Context, v model.Version) (model.Version, error) {
	modelCfg, err := json.Marshal(orEmptyObj(v.ModelConfig))
	if err != nil {
		return model.Version{}, fmt.Errorf("marshal model_config: %w", err)
	}
	vars, err := json.Marshal(orEmptyArr(v.Variables))
	if err != nil {
		return model.Version{}, fmt.Errorf("marshal variables: %w", err)
	}

	var out model.Version
	err = s.withTx(ctx, func(tx pgx.Tx) error {
		// 1. Lock the parent prompt row so concurrent version creation for this
		//    prompt is serialized. Also confirms the prompt exists.
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT true FROM prompts WHERE id = $1 FOR UPDATE`, v.PromptID).
			Scan(&exists); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}

		// 2. Next version number = current max + 1 (1 for the first version).
		var next int
		if err := tx.QueryRow(ctx,
			`SELECT COALESCE(MAX(version_number), 0) + 1 FROM prompt_versions WHERE prompt_id = $1`,
			v.PromptID).Scan(&next); err != nil {
			return err
		}

		// 3. Insert the immutable row.
		const ins = `
			INSERT INTO prompt_versions
				(prompt_id, version_number, system_template, user_template,
				 model_config, variables, commit_message, created_by)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
			RETURNING id, version_number, created_at`
		if err := tx.QueryRow(ctx, ins,
			v.PromptID, next, v.SystemTemplate, v.UserTemplate,
			modelCfg, vars, v.CommitMessage, v.CreatedBy,
		).Scan(&out.ID, &out.VersionNumber, &out.CreatedAt); err != nil {
			return err
		}

		// 4. Reflect the edit as prompt activity.
		return touchPrompt(ctx, tx, v.PromptID)
	})
	if err != nil {
		return model.Version{}, err
	}

	out.PromptID = v.PromptID
	out.SystemTemplate = v.SystemTemplate
	out.UserTemplate = v.UserTemplate
	out.ModelConfig = orEmptyObj(v.ModelConfig)
	out.Variables = orEmptyArr(v.Variables)
	out.CommitMessage = v.CommitMessage
	out.CreatedBy = v.CreatedBy
	return out, nil
}

// GetVersionByNumber fetches a specific version of a prompt.
func (s *Store) GetVersionByNumber(ctx context.Context, promptID string, number int) (model.Version, error) {
	const q = baseVersionSelect + ` WHERE prompt_id = $1 AND version_number = $2`
	return s.scanVersionRow(s.pool.QueryRow(ctx, q, promptID, number))
}

// GetVersionByID fetches a version by its UUID.
func (s *Store) GetVersionByID(ctx context.Context, id string) (model.Version, error) {
	const q = baseVersionSelect + ` WHERE id = $1`
	return s.scanVersionRow(s.pool.QueryRow(ctx, q, id))
}

// GetLatestVersion fetches the highest-numbered version of a prompt.
func (s *Store) GetLatestVersion(ctx context.Context, promptID string) (model.Version, error) {
	const q = baseVersionSelect + ` WHERE prompt_id = $1 ORDER BY version_number DESC LIMIT 1`
	return s.scanVersionRow(s.pool.QueryRow(ctx, q, promptID))
}

// ListVersions returns all versions of a prompt, newest first — the data behind
// the dashboard's version-history timeline.
func (s *Store) ListVersions(ctx context.Context, promptID string) ([]model.Version, error) {
	const q = baseVersionSelect + ` WHERE prompt_id = $1 ORDER BY version_number DESC`
	rows, err := s.pool.Query(ctx, q, promptID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Version
	for rows.Next() {
		v, err := scanVersion(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

const baseVersionSelect = `
	SELECT id, prompt_id, version_number, system_template, user_template,
	       model_config, variables, commit_message, created_by, created_at
	FROM prompt_versions`

// rowScanner is satisfied by both pgx.Row and pgx.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

func (s *Store) scanVersionRow(row pgx.Row) (model.Version, error) {
	v, err := scanVersion(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Version{}, ErrNotFound
	}
	return v, err
}

func scanVersion(row rowScanner) (model.Version, error) {
	var v model.Version
	var modelCfg, vars []byte
	if err := row.Scan(&v.ID, &v.PromptID, &v.VersionNumber, &v.SystemTemplate, &v.UserTemplate,
		&modelCfg, &vars, &v.CommitMessage, &v.CreatedBy, &v.CreatedAt); err != nil {
		return model.Version{}, err
	}
	if len(modelCfg) > 0 {
		if err := json.Unmarshal(modelCfg, &v.ModelConfig); err != nil {
			return model.Version{}, fmt.Errorf("unmarshal model_config: %w", err)
		}
	}
	if len(vars) > 0 {
		if err := json.Unmarshal(vars, &v.Variables); err != nil {
			return model.Version{}, fmt.Errorf("unmarshal variables: %w", err)
		}
	}
	return v, nil
}

func orEmptyObj(m model.ModelConfig) model.ModelConfig {
	if m == nil {
		return model.ModelConfig{}
	}
	return m
}

func orEmptyArr(v []model.Variable) []model.Variable {
	if v == nil {
		return []model.Variable{}
	}
	return v
}
