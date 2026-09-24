package api

import (
	"net/http"
)

func (s *Server) listTags(w http.ResponseWriter, r *http.Request) {
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
	writeJSON(w, http.StatusOK, map[string]any{"tags": tags})
}

type setTagReq struct {
	// Point the tag at this version number. This one field expresses both
	// deploying a new version and rolling back to an old one.
	VersionNumber int    `json:"version_number"`
	UpdatedBy     string `json:"updated_by"`
}

// setTag moves a tag (e.g. PUT /prompts/x/tags/production {version_number: 3}).
// Deploy and rollback are the same operation — see the versioning learning doc.
func (s *Server) setTag(w http.ResponseWriter, r *http.Request) {
	p, err := s.store.GetPromptByName(r.Context(), projectOf(r), r.PathValue("name"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	var req setTagReq
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.VersionNumber <= 0 {
		writeError(w, http.StatusBadRequest, "version_number is required and must be >= 1")
		return
	}
	// Resolve the version number to its id (also validates it exists).
	v, err := s.store.GetVersionByNumber(r.Context(), p.ID, req.VersionNumber)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	tag, err := s.store.SetTag(r.Context(), p.ID, r.PathValue("tag"), v.ID, req.UpdatedBy)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, tag)
}
