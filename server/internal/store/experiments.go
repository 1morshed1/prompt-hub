package store

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/1morshed1/prompt-hub/server/internal/model"
	"github.com/jackc/pgx/v5"
)

// CreateExperiment starts an A/B test. The partial unique index in the schema
// (one running experiment per prompt) means a second concurrent "running"
// experiment for the same prompt fails with ErrConflict.
func (s *Store) CreateExperiment(ctx context.Context, e model.Experiment) (model.Experiment, error) {
	const q = `
		INSERT INTO experiments
			(prompt_id, name, control_version_id, variant_version_id,
			 traffic_split, status, auto_promote, min_samples)
		VALUES ($1,$2,$3,$4,$5,'running',$6,$7)
		RETURNING id, status, started_at`
	split := e.TrafficSplit
	if split == 0 {
		split = 0.5
	}
	minS := e.MinSamples
	if minS == 0 {
		minS = 100
	}
	out := e
	err := s.pool.QueryRow(ctx, q, e.PromptID, e.Name, e.ControlVersionID, e.VariantVersionID,
		split, e.AutoPromote, minS).Scan(&out.ID, &out.Status, &out.StartedAt)
	if isUniqueViolation(err) {
		return model.Experiment{}, ErrConflict
	}
	out.TrafficSplit = split
	out.MinSamples = minS
	return out, err
}

const baseExperimentSelect = `
	SELECT id, prompt_id, name, control_version_id, variant_version_id,
	       traffic_split, status, auto_promote, min_samples, started_at, ended_at
	FROM experiments`

func scanExperiment(row rowScanner) (model.Experiment, error) {
	var e model.Experiment
	err := row.Scan(&e.ID, &e.PromptID, &e.Name, &e.ControlVersionID, &e.VariantVersionID,
		&e.TrafficSplit, &e.Status, &e.AutoPromote, &e.MinSamples, &e.StartedAt, &e.EndedAt)
	return e, err
}

// GetExperiment fetches one experiment by id.
func (s *Store) GetExperiment(ctx context.Context, id string) (model.Experiment, error) {
	e, err := scanExperiment(s.pool.QueryRow(ctx, baseExperimentSelect+` WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Experiment{}, ErrNotFound
	}
	return e, err
}

// GetRunningExperiment returns the single running experiment for a prompt, or
// ErrNotFound if there isn't one. This is what the render path consults to decide
// whether to A/B assign.
func (s *Store) GetRunningExperiment(ctx context.Context, promptID string) (model.Experiment, error) {
	e, err := scanExperiment(s.pool.QueryRow(ctx,
		baseExperimentSelect+` WHERE prompt_id = $1 AND status = 'running'`, promptID))
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Experiment{}, ErrNotFound
	}
	return e, err
}

// ListExperiments returns experiments for a prompt (or all if promptID == "").
func (s *Store) ListExperiments(ctx context.Context, promptID string) ([]model.Experiment, error) {
	q := baseExperimentSelect + ` ORDER BY started_at DESC`
	args := []any{}
	if promptID != "" {
		q = baseExperimentSelect + ` WHERE prompt_id = $1 ORDER BY started_at DESC`
		args = append(args, promptID)
	}
	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Experiment
	for rows.Next() {
		e, err := scanExperiment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// SetExperimentStatus transitions an experiment to stopped or promoted, stamping
// ended_at. Once out of 'running' it frees the partial-unique slot so a new
// experiment can start for the prompt.
func (s *Store) SetExperimentStatus(ctx context.Context, id, status string) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE experiments SET status = $2, ended_at = now() WHERE id = $1`, id, status)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// RecordUsage inserts the result row created at render time (metrics still null).
// It records EVERY render for per-version analytics; experiment_id and arm are set
// only when the render was assigned by a running experiment (empty strings become
// SQL NULL via NULLIF). It's idempotent on request_id, so rendering the same
// request id twice never double-counts.
func (s *Store) RecordUsage(ctx context.Context, r model.Result) error {
	const q = `
		INSERT INTO experiment_results (prompt_id, experiment_id, version_id, request_id, arm)
		VALUES ($1, NULLIF($2,'')::uuid, $3, $4, NULLIF($5,''))
		ON CONFLICT (request_id) DO NOTHING`
	_, err := s.pool.Exec(ctx, q, r.PromptID, r.ExperimentID, r.VersionID, r.RequestID, r.Arm)
	return err
}

// LogResult attaches metrics to an already-assigned request. Only non-nil fields
// are written, so a caller can log latency immediately and the user's rating
// later, each in its own call. Returns ErrNotFound if the request_id is unknown.
func (s *Store) LogResult(ctx context.Context, r model.Result) error {
	custom, err := json.Marshal(orEmptyMap(r.CustomMetrics))
	if err != nil {
		return err
	}
	// COALESCE keeps existing values when a field is not supplied this call.
	const q = `
		UPDATE experiment_results SET
			latency_ms     = COALESCE($2, latency_ms),
			tokens_used    = COALESCE($3, tokens_used),
			cost_usd       = COALESCE($4, cost_usd),
			user_rating    = COALESCE($5, user_rating),
			custom_metrics = CASE WHEN $6::jsonb = '{}'::jsonb THEN custom_metrics ELSE $6::jsonb END
		WHERE request_id = $1`
	tag, err := s.pool.Exec(ctx, q, r.RequestID, r.LatencyMS, r.TokensUsed, r.CostUSD, r.UserRating, custom)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ResultsFor returns every result row for an experiment — the raw data the
// analyzer aggregates.
func (s *Store) ResultsFor(ctx context.Context, experimentID string) ([]model.Result, error) {
	const q = `
		SELECT id, experiment_id, version_id, request_id, arm,
		       latency_ms, tokens_used, cost_usd, user_rating, custom_metrics, created_at
		FROM experiment_results WHERE experiment_id = $1`
	rows, err := s.pool.Query(ctx, q, experimentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Result
	for rows.Next() {
		var r model.Result
		var custom []byte
		if err := rows.Scan(&r.ID, &r.ExperimentID, &r.VersionID, &r.RequestID, &r.Arm,
			&r.LatencyMS, &r.TokensUsed, &r.CostUSD, &r.UserRating, &custom, &r.CreatedAt); err != nil {
			return nil, err
		}
		if len(custom) > 0 {
			_ = json.Unmarshal(custom, &r.CustomMetrics)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func orEmptyMap(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}
