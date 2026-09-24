package api

import (
	"net/http"
	"strconv"

	"github.com/1morshed1/prompt-hub/server/internal/diff"
)

// diffVersions compares two versions of a prompt at the line or token level.
// GET /prompts/{name}/diff?from=1&to=2&mode=line|token
func (s *Server) diffVersions(w http.ResponseWriter, r *http.Request) {
	p, err := s.store.GetPromptByName(r.Context(), projectOf(r), r.PathValue("name"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	from, err1 := strconv.Atoi(r.URL.Query().Get("from"))
	to, err2 := strconv.Atoi(r.URL.Query().Get("to"))
	if err1 != nil || err2 != nil {
		writeError(w, http.StatusBadRequest, "from and to must be integer version numbers")
		return
	}
	vFrom, err := s.store.GetVersionByNumber(r.Context(), p.ID, from)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	vTo, err := s.store.GetVersionByNumber(r.Context(), p.ID, to)
	if err != nil {
		writeStoreError(w, err)
		return
	}

	diffFn := diff.Lines
	if r.URL.Query().Get("mode") == "token" {
		diffFn = diff.Tokens
	}

	sysSegs := diffFn(vFrom.SystemTemplate, vTo.SystemTemplate)
	userSegs := diffFn(vFrom.UserTemplate, vTo.UserTemplate)

	writeJSON(w, http.StatusOK, map[string]any{
		"from":            from,
		"to":              to,
		"mode":            orDefault(r.URL.Query().Get("mode"), "line"),
		"system_template": map[string]any{"segments": sysSegs, "stats": diff.Summarize(sysSegs)},
		"user_template":   map[string]any{"segments": userSegs, "stats": diff.Summarize(userSegs)},
	})
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
