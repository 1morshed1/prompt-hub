package api

import (
	"context"
	"fmt"

	"github.com/1morshed1/prompt-hub/server/internal/store"
	"github.com/1morshed1/prompt-hub/server/internal/template"
)

// dbIncludeResolver resolves {% include "name" %} to the *content* of another
// prompt in the default project: it looks up the prompt by name, prefers its
// "production"-tagged version, and returns that version's user_template. This is
// what lets teams factor out a shared "safety-preamble" prompt and include it
// everywhere — edit it once, every includer picks up the change.
//
// It implements template.IncludeResolver. Keeping resolution here (in the api
// package, which already has the store) keeps the template package pure and
// database-free.
type dbIncludeResolver struct {
	store   *store.Store
	project string
}

func newDBIncludeResolver(st *store.Store) template.IncludeResolver {
	return &dbIncludeResolver{store: st, project: "default"}
}

func (d *dbIncludeResolver) Resolve(name string) (string, error) {
	ctx := context.Background()
	p, err := d.store.GetPromptByName(ctx, d.project, name)
	if err != nil {
		return "", fmt.Errorf("include target prompt %q not found", name)
	}
	// Prefer the production-tagged version; fall back to latest.
	v, err := d.store.ResolveVersion(ctx, p.ID, "production", 0)
	if err != nil {
		v, err = d.store.GetLatestVersion(ctx, p.ID)
		if err != nil {
			return "", fmt.Errorf("include target %q has no versions", name)
		}
	}
	return v.UserTemplate, nil
}
