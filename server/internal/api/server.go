// Package api is the HTTP layer: routing, request/response encoding, and the
// handlers that translate JSON calls into store/experiment/template operations.
//
// Routing uses the standard library's Go 1.22+ pattern mux (method + path
// wildcards) — no framework. See docs/decisions/0005-stdlib-http.md.
package api

import (
	"log/slog"
	"net/http"

	"github.com/1morshed1/prompt-hub/server/internal/experiment"
	"github.com/1morshed1/prompt-hub/server/internal/store"
	"github.com/1morshed1/prompt-hub/server/internal/template"
)

// Server holds the dependencies every handler needs. Handlers are methods on it,
// so they share the store, experiment service, template engine, and logger
// without package-level globals.
type Server struct {
	store  *store.Store
	exp    *experiment.Service
	engine *template.Engine
	log    *slog.Logger
	webDir string // directory of static dashboard files ("" disables serving)
}

// New builds a Server. The template engine is configured with a database-backed
// include resolver so {% include "name" %} pulls another prompt's content.
func New(st *store.Store, log *slog.Logger, webDir string) *Server {
	engine := template.New(template.WithIncludeResolver(newDBIncludeResolver(st)))
	return &Server{
		store:  st,
		exp:    experiment.NewService(st),
		engine: engine,
		log:    log,
		webDir: webDir,
	}
}

// Handler builds the fully-wired http.Handler: routes plus the middleware chain.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// Health.
	mux.HandleFunc("GET /healthz", s.health)

	// Prompts.
	mux.HandleFunc("POST /api/v1/prompts", s.createPrompt)
	mux.HandleFunc("GET /api/v1/prompts", s.listPrompts)
	mux.HandleFunc("GET /api/v1/prompts/{name}", s.getPrompt)
	mux.HandleFunc("DELETE /api/v1/prompts/{name}", s.deletePrompt)

	// Versions.
	mux.HandleFunc("POST /api/v1/prompts/{name}/versions", s.createVersion)
	mux.HandleFunc("GET /api/v1/prompts/{name}/versions", s.listVersions)
	mux.HandleFunc("GET /api/v1/prompts/{name}/versions/{number}", s.getVersion)

	// Tags (deploy / rollback).
	mux.HandleFunc("GET /api/v1/prompts/{name}/tags", s.listTags)
	mux.HandleFunc("PUT /api/v1/prompts/{name}/tags/{tag}", s.setTag)

	// Diff.
	mux.HandleFunc("GET /api/v1/prompts/{name}/diff", s.diffVersions)

	// Per-version analytics.
	mux.HandleFunc("GET /api/v1/prompts/{name}/analytics", s.analytics)

	// Render (the serving path).
	mux.HandleFunc("POST /api/v1/prompts/{name}/render", s.render)

	// Experiments.
	mux.HandleFunc("POST /api/v1/experiments", s.createExperiment)
	mux.HandleFunc("GET /api/v1/experiments", s.listExperiments)
	mux.HandleFunc("GET /api/v1/experiments/{id}", s.getExperiment)
	mux.HandleFunc("GET /api/v1/experiments/{id}/report", s.experimentReport)
	mux.HandleFunc("POST /api/v1/experiments/{id}/stop", s.stopExperiment)
	mux.HandleFunc("POST /api/v1/experiments/{id}/promote", s.promoteExperiment)

	// Result logging (SDK feedback loop).
	mux.HandleFunc("POST /api/v1/results", s.logResult)

	// Static dashboard at / (optional).
	if s.webDir != "" {
		mux.Handle("/", http.FileServer(http.Dir(s.webDir)))
	}

	// Middleware chain, outermost first: recover -> log -> cors -> mux.
	return recoverMW(s.log)(logMW(s.log)(corsMW(mux)))
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
