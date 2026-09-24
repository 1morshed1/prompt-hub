package store

import "context"

// VersionAnalytics is the aggregate performance of one version across ALL renders
// (experiment or not). This powers the dashboard's per-prompt performance view:
// "how is each version doing on latency, cost, and satisfaction?"
type VersionAnalytics struct {
	VersionID     string   `json:"version_id"`
	VersionNumber int      `json:"version_number"`
	Renders       int      `json:"renders"`
	RatedCount    int      `json:"rated_count"`
	ThumbsUp      int      `json:"thumbs_up"`
	AvgRating     *float64 `json:"avg_rating"`      // null until at least one rating exists
	AvgLatencyMS  *float64 `json:"avg_latency_ms"`
	AvgCostUSD    *float64 `json:"avg_cost_usd"`
	TotalTokens   int      `json:"total_tokens"`
}

// AnalyticsForPrompt aggregates render results per version for a prompt. It uses
// SQL aggregate functions (COUNT/AVG/SUM with FILTER) so the database does the
// number-crunching in one query rather than pulling every row into Go.
func (s *Store) AnalyticsForPrompt(ctx context.Context, promptID string) ([]VersionAnalytics, error) {
	const q = `
		SELECT
			pv.id,
			pv.version_number,
			COUNT(er.id)                                           AS renders,
			COUNT(er.user_rating)                                  AS rated_count,
			COUNT(er.user_rating) FILTER (WHERE er.user_rating = 1) AS thumbs_up,
			AVG(er.user_rating::float)                             AS avg_rating,
			AVG(er.latency_ms::float)                              AS avg_latency,
			AVG(er.cost_usd)                                       AS avg_cost,
			COALESCE(SUM(er.tokens_used), 0)                       AS total_tokens
		FROM prompt_versions pv
		LEFT JOIN experiment_results er ON er.version_id = pv.id
		WHERE pv.prompt_id = $1
		GROUP BY pv.id, pv.version_number
		ORDER BY pv.version_number DESC`
	rows, err := s.pool.Query(ctx, q, promptID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []VersionAnalytics
	for rows.Next() {
		var a VersionAnalytics
		if err := rows.Scan(&a.VersionID, &a.VersionNumber, &a.Renders, &a.RatedCount,
			&a.ThumbsUp, &a.AvgRating, &a.AvgLatencyMS, &a.AvgCostUSD, &a.TotalTokens); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
