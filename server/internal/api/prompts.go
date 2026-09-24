package api

import (
	"net/http"

	"github.com/1morshed1/prompt-hub/server/internal/model"
)

// projectOf reads the project from the ?project= query param, defaulting to
// "default". Every prompt-scoped route is implicitly within a project.
func projectOf(r *http.Request) string {
	if p := r.URL.Query().Get("project"); p != "" {
		return p
	}
	return "default"
}

type createPromptReq struct {
	Name        string `json:"name"`
	Project     string `json:"project"`
	Description string `json:"description"`
	CreatedBy   string `json:"created_by"`
}

func (s *Server) createPrompt(w http.ResponseWriter, r *http.Request) {
	var req createPromptReq
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	p, err := s.store.CreatePrompt(r.Context(), model.Prompt{
		Name: req.Name, Project: req.Project, Description: req.Description, CreatedBy: req.CreatedBy,
	})
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

func (s *Server) listPrompts(w http.ResponseWriter, r *http.Request) {
	prompts, err := s.store.ListPrompts(r.Context(), projectOf(r))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"prompts": prompts})
}

// promptDetail is the composite view for GET /prompts/{name}: the prompt plus its
// tags and version list, so the dashboard can render everything in one request.
type promptDetail struct {
	model.Prompt
	Tags     []model.Tag     `json:"tags"`
	Versions []model.Version `json:"versions"`
}

func (s *Server) getPrompt(w http.ResponseWriter, r *http.Request) {
	p, err := s.store.GetPromptByName(r.Context(), projectOf(r), r.PathValue("name"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	tags, err := s.store.ListTags(r.Context(), p.ID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	versions, err := s.store.ListVersions(r.Context(), p.ID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, promptDetail{Prompt: p, Tags: tags, Versions: versions})
}

func (s *Server) deletePrompt(w http.ResponseWriter, r *http.Request) {
	p, err := s.store.GetPromptByName(r.Context(), projectOf(r), r.PathValue("name"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if err := s.store.DeletePrompt(r.Context(), p.ID); err != nil {
		writeStoreError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
