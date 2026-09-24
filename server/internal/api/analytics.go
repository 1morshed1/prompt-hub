package api

import "net/http"

// analytics returns per-version render metrics for a prompt.
// GET /api/v1/prompts/{name}/analytics
func (s *Server) analytics(w http.ResponseWriter, r *http.Request) {
	p, err := s.store.GetPromptByName(r.Context(), projectOf(r), r.PathValue("name"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	rows, err := s.store.AnalyticsForPrompt(r.Context(), p.ID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"prompt": p.Name, "versions": rows})
}
