package api

import (
	"net/http"

	"github.com/1morshed1/prompt-hub/server/internal/model"
)

type logResultReq struct {
	RequestID     string         `json:"request_id"`
	LatencyMS     *int           `json:"latency_ms"`
	TokensUsed    *int           `json:"tokens_used"`
	CostUSD       *float64       `json:"cost_usd"`
	Rating        *int           `json:"rating"` // 0 or 1
	CustomMetrics map[string]any `json:"custom_metrics"`
}

// logResult records outcome metrics against a request id from a prior render.
// Fields are optional and can be sent across multiple calls (latency now, the
// user's thumbs-up later) — the store merges them. Unknown request ids 404.
func (s *Server) logResult(w http.ResponseWriter, r *http.Request) {
	var req logResultReq
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.RequestID == "" {
		writeError(w, http.StatusBadRequest, "request_id is required")
		return
	}
	if req.Rating != nil && *req.Rating != 0 && *req.Rating != 1 {
		writeError(w, http.StatusBadRequest, "rating must be 0 or 1")
		return
	}
	err := s.store.LogResult(r.Context(), model.Result{
		RequestID:     req.RequestID,
		LatencyMS:     req.LatencyMS,
		TokensUsed:    req.TokensUsed,
		CostUSD:       req.CostUSD,
		UserRating:    req.Rating,
		CustomMetrics: req.CustomMetrics,
	})
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "logged"})
}
