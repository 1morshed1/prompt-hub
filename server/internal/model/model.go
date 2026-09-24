// Package model holds PromptHub's pure domain types — plain data structs with no
// database or HTTP dependencies. Keeping them here lets the store, api, template,
// and experiment packages all share one vocabulary without import cycles.
package model

import "time"

// Prompt is the named entity (like a git repository). Its content lives in Versions.
type Prompt struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Project     string    `json:"project"`
	Description string    `json:"description"`
	CreatedBy   string    `json:"created_by"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Variable declares one template variable and its expected type. Stored in the
// version's `variables` JSONB and used by the template engine's validation pass.
type Variable struct {
	Name     string `json:"name"`
	Type     string `json:"type"`              // string | number | boolean | list | object
	Required bool   `json:"required"`
	Default  any    `json:"default,omitempty"` // used when the caller omits an optional var
}

// ModelConfig is the LLM configuration attached to a version. It's intentionally
// open-ended (stored as JSONB) so new knobs don't require a schema change.
type ModelConfig map[string]any

// Version is an immutable snapshot of a prompt's content. Never mutated after
// creation — an edit creates a new Version with the next VersionNumber.
type Version struct {
	ID             string      `json:"id"`
	PromptID       string      `json:"prompt_id"`
	VersionNumber  int         `json:"version_number"`
	SystemTemplate string      `json:"system_template"`
	UserTemplate   string      `json:"user_template"`
	ModelConfig    ModelConfig `json:"model_config"`
	Variables      []Variable  `json:"variables"`
	CommitMessage  string      `json:"commit_message"`
	CreatedBy      string      `json:"created_by"`
	CreatedAt      time.Time   `json:"created_at"`
}

// Tag is a movable pointer from (prompt, tag name) to a specific version — the
// deploy layer. Like a git branch.
type Tag struct {
	PromptID  string    `json:"prompt_id"`
	Tag       string    `json:"tag"`
	VersionID string    `json:"version_id"`
	UpdatedAt time.Time `json:"updated_at"`
	UpdatedBy string    `json:"updated_by"`
}

// Experiment is an A/B test between two versions of one prompt.
type Experiment struct {
	ID               string     `json:"id"`
	PromptID         string     `json:"prompt_id"`
	Name             string     `json:"name"`
	ControlVersionID string     `json:"control_version_id"`
	VariantVersionID string     `json:"variant_version_id"`
	TrafficSplit     float64    `json:"traffic_split"` // fraction routed to variant
	Status           string     `json:"status"`        // running | stopped | promoted
	AutoPromote      bool       `json:"auto_promote"`
	MinSamples       int        `json:"min_samples"`
	StartedAt        time.Time  `json:"started_at"`
	EndedAt          *time.Time `json:"ended_at,omitempty"`
}

// Result is one served-request record — the raw analytics data the stats package
// aggregates. Metric fields are pointers because they arrive at different times
// (latency immediately; user_rating possibly minutes later) and may be absent.
type Result struct {
	ID            string         `json:"id"`
	ExperimentID  string         `json:"experiment_id"`
	VersionID     string         `json:"version_id"`
	RequestID     string         `json:"request_id"`
	Arm           string         `json:"arm"` // control | variant
	LatencyMS     *int           `json:"latency_ms,omitempty"`
	TokensUsed    *int           `json:"tokens_used,omitempty"`
	CostUSD       *float64       `json:"cost_usd,omitempty"`
	UserRating    *int           `json:"user_rating,omitempty"` // 0 or 1
	CustomMetrics map[string]any `json:"custom_metrics,omitempty"`
	CreatedAt     time.Time      `json:"created_at"`
}
