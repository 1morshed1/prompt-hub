package api

import (
	"errors"
	"net/http"

	"github.com/1morshed1/prompt-hub/server/internal/experiment"
	"github.com/1morshed1/prompt-hub/server/internal/model"
	"github.com/1morshed1/prompt-hub/server/internal/store"
	"github.com/1morshed1/prompt-hub/server/internal/template"
)

type renderReq struct {
	// Version selection. Precedence: explicit Version pins exactly (no experiment);
	// otherwise a running experiment assigns the arm; otherwise Tag (or latest).
	Tag     string `json:"tag"`
	Version int    `json:"version"`
	// UnitID makes experiment assignment sticky (e.g. a user or session id). If
	// empty, a fresh id is generated and assignment is effectively random.
	UnitID    string         `json:"unit_id"`
	Variables map[string]any `json:"variables"`
}

// renderResp is the LLM-ready payload plus the bookkeeping the SDK needs to log
// results back.
type renderResp struct {
	PromptName     string            `json:"prompt_name"`
	VersionNumber  int               `json:"version_number"`
	System         string            `json:"system"`
	User           string            `json:"user"`
	ModelConfig    model.ModelConfig `json:"model_config"`
	RequestID      string            `json:"request_id"`
	ExperimentID   string            `json:"experiment_id,omitempty"`
	Arm            string            `json:"arm,omitempty"`
}

// render resolves a version, renders its templates against the supplied
// variables, and (when an experiment is running) records the A/B assignment.
func (s *Server) render(w http.ResponseWriter, r *http.Request) {
	p, err := s.store.GetPromptByName(r.Context(), projectOf(r), r.PathValue("name"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	var req renderReq
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	requestID := newUUID()
	resp := renderResp{PromptName: p.Name, RequestID: requestID}

	version, err := s.selectVersion(r, p, &req, requestID, &resp)
	if err != nil {
		writeStoreError(w, err)
		return
	}

	// Validate caller input against the version's declared variable schema, then
	// render both templates in the resulting context.
	ctx, err := template.BuildContext(version.Variables, req.Variables)
	if err != nil {
		writeRenderError(w, err)
		return
	}
	sysOut, err := s.engine.Render(version.SystemTemplate, ctx)
	if err != nil {
		writeRenderError(w, err)
		return
	}
	userOut, err := s.engine.Render(version.UserTemplate, ctx)
	if err != nil {
		writeRenderError(w, err)
		return
	}

	resp.VersionNumber = version.VersionNumber
	resp.System = sysOut
	resp.User = userOut
	resp.ModelConfig = version.ModelConfig
	writeJSON(w, http.StatusOK, resp)
}

// selectVersion applies the version-selection precedence and, if it assigns via a
// running experiment, records the assignment and fills the experiment fields on
// resp. It returns the concrete version to render.
func (s *Server) selectVersion(r *http.Request, p model.Prompt, req *renderReq, requestID string, resp *renderResp) (model.Version, error) {
	// 1. Explicit version number pins exactly — reproducible, bypasses experiments.
	if req.Version > 0 {
		return s.store.GetVersionByNumber(r.Context(), p.ID, req.Version)
	}

	// 2. A running experiment takes over serving: assign an arm and record it.
	exp, err := s.store.GetRunningExperiment(r.Context(), p.ID)
	if err == nil {
		unit := req.UnitID
		if unit == "" {
			unit = requestID
		}
		arm, versionID := experiment.Assign(exp, unit)
		v, err := s.store.GetVersionByID(r.Context(), versionID)
		if err != nil {
			return model.Version{}, err
		}
		if err := s.store.RecordAssignment(r.Context(), model.Result{
			ExperimentID: exp.ID, VersionID: versionID, RequestID: requestID, Arm: arm,
		}); err != nil {
			return model.Version{}, err
		}
		resp.ExperimentID = exp.ID
		resp.Arm = arm
		return v, nil
	}
	if !errors.Is(err, store.ErrNotFound) {
		return model.Version{}, err
	}

	// 3. No experiment: resolve by tag, or latest if no tag given.
	return s.store.ResolveVersion(r.Context(), p.ID, req.Tag, 0)
}
