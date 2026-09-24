package api

import (
	"net/http"
	"strconv"

	"github.com/1morshed1/prompt-hub/server/internal/model"
)

type createVersionReq struct {
	SystemTemplate string            `json:"system_template"`
	UserTemplate   string            `json:"user_template"`
	ModelConfig    model.ModelConfig `json:"model_config"`
	Variables      []model.Variable  `json:"variables"`
	CommitMessage  string            `json:"commit_message"`
	CreatedBy      string            `json:"created_by"`
}

// createVersion appends a new immutable version. The store assigns the number.
func (s *Server) createVersion(w http.ResponseWriter, r *http.Request) {
	p, err := s.store.GetPromptByName(r.Context(), projectOf(r), r.PathValue("name"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	var req createVersionReq
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	v, err := s.store.CreateVersion(r.Context(), model.Version{
		PromptID:       p.ID,
		SystemTemplate: req.SystemTemplate,
		UserTemplate:   req.UserTemplate,
		ModelConfig:    req.ModelConfig,
		Variables:      req.Variables,
		CommitMessage:  req.CommitMessage,
		CreatedBy:      req.CreatedBy,
	})
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, v)
}

func (s *Server) listVersions(w http.ResponseWriter, r *http.Request) {
	p, err := s.store.GetPromptByName(r.Context(), projectOf(r), r.PathValue("name"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	versions, err := s.store.ListVersions(r.Context(), p.ID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"versions": versions})
}

func (s *Server) getVersion(w http.ResponseWriter, r *http.Request) {
	p, err := s.store.GetPromptByName(r.Context(), projectOf(r), r.PathValue("name"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	number, err := strconv.Atoi(r.PathValue("number"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "version number must be an integer")
		return
	}
	v, err := s.store.GetVersionByNumber(r.Context(), p.ID, number)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}
