package api

import (
	"net/http"

	"github.com/1morshed1/prompt-hub/server/internal/model"
)

type createExperimentReq struct {
	// Prompt is referenced by name (+ optional project via query) for ergonomics;
	// versions are referenced by number so callers don't juggle UUIDs.
	Prompt         string  `json:"prompt"`
	Name           string  `json:"name"`
	ControlVersion int     `json:"control_version"`
	VariantVersion int     `json:"variant_version"`
	TrafficSplit   float64 `json:"traffic_split"`
	AutoPromote    bool    `json:"auto_promote"`
	MinSamples     int     `json:"min_samples"`
}

func (s *Server) createExperiment(w http.ResponseWriter, r *http.Request) {
	var req createExperimentReq
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Prompt == "" || req.ControlVersion <= 0 || req.VariantVersion <= 0 {
		writeError(w, http.StatusBadRequest, "prompt, control_version and variant_version are required")
		return
	}
	p, err := s.store.GetPromptByName(r.Context(), projectOf(r), req.Prompt)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	control, err := s.store.GetVersionByNumber(r.Context(), p.ID, req.ControlVersion)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	variant, err := s.store.GetVersionByNumber(r.Context(), p.ID, req.VariantVersion)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	exp, err := s.store.CreateExperiment(r.Context(), model.Experiment{
		PromptID:         p.ID,
		Name:             req.Name,
		ControlVersionID: control.ID,
		VariantVersionID: variant.ID,
		TrafficSplit:     req.TrafficSplit,
		AutoPromote:      req.AutoPromote,
		MinSamples:       req.MinSamples,
	})
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, exp)
}

func (s *Server) listExperiments(w http.ResponseWriter, r *http.Request) {
	// Optional ?prompt= filter (by name).
	promptID := ""
	if name := r.URL.Query().Get("prompt"); name != "" {
		p, err := s.store.GetPromptByName(r.Context(), projectOf(r), name)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		promptID = p.ID
	}
	exps, err := s.store.ListExperiments(r.Context(), promptID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"experiments": exps})
}

func (s *Server) getExperiment(w http.ResponseWriter, r *http.Request) {
	exp, err := s.store.GetExperiment(r.Context(), r.PathValue("id"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, exp)
}

// experimentReport returns the full statistical analysis. If the experiment has
// auto_promote enabled, it also evaluates promotion so a poll of the report keeps
// the experiment moving toward a decision.
func (s *Server) experimentReport(w http.ResponseWriter, r *http.Request) {
	report, exp, err := s.exp.ReportFor(r.Context(), r.PathValue("id"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	decision, err := s.exp.MaybeAutoPromote(r.Context(), exp)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"report":    report,
		"promotion": decision,
	})
}

func (s *Server) stopExperiment(w http.ResponseWriter, r *http.Request) {
	if err := s.store.SetExperimentStatus(r.Context(), r.PathValue("id"), "stopped"); err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
}

// promoteExperiment manually promotes the variant to production and closes the
// experiment — the human-override counterpart to auto-promote.
func (s *Server) promoteExperiment(w http.ResponseWriter, r *http.Request) {
	exp, err := s.store.GetExperiment(r.Context(), r.PathValue("id"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if _, err := s.store.SetTag(r.Context(), exp.PromptID, "production", exp.VariantVersionID, "manual-promote"); err != nil {
		writeStoreError(w, err)
		return
	}
	if err := s.store.SetExperimentStatus(r.Context(), exp.ID, "promoted"); err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "promoted"})
}
