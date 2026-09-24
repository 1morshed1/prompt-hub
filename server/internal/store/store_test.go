package store_test

import (
	"context"
	"testing"

	"github.com/1morshed1/prompt-hub/server/internal/model"
	"github.com/1morshed1/prompt-hub/server/internal/store"
	"github.com/1morshed1/prompt-hub/server/internal/testutil"
)

// These are integration tests: they run against a real Postgres so the SQL,
// constraints, and transaction logic are exercised for real (not mocked). Point
// TEST_DATABASE_URL at a throwaway database; if it's unset the tests skip, so
// `go test ./...` still works in environments without a DB. testutil.Setup
// serializes DB access across packages so parallel test runs don't collide.
func newTestStore(t *testing.T) *store.Store {
	return testutil.Setup(t)
}

func TestPromptCRUD(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	p, err := s.CreatePrompt(ctx, model.Prompt{Name: "order-assistant", Description: "helps with orders"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if p.Project != "default" {
		t.Errorf("expected default project, got %q", p.Project)
	}

	// Duplicate (project, name) must conflict.
	if _, err := s.CreatePrompt(ctx, model.Prompt{Name: "order-assistant"}); err != store.ErrConflict {
		t.Errorf("expected ErrConflict on duplicate, got %v", err)
	}

	got, err := s.GetPromptByName(ctx, "default", "order-assistant")
	if err != nil || got.ID != p.ID {
		t.Fatalf("get: %v id=%s", err, got.ID)
	}

	if _, err := s.GetPromptByName(ctx, "default", "nope"); err != store.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestVersioningIsMonotonic(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	p, _ := s.CreatePrompt(ctx, model.Prompt{Name: "p"})

	for i := 1; i <= 3; i++ {
		v, err := s.CreateVersion(ctx, model.Version{
			PromptID:       p.ID,
			SystemTemplate: "You are helpful",
			UserTemplate:   "{{query}}",
			ModelConfig:    model.ModelConfig{"model": "gpt-4o", "temperature": 0.2},
			Variables:      []model.Variable{{Name: "query", Type: "string", Required: true}},
			CommitMessage:  "edit",
		})
		if err != nil {
			t.Fatalf("create version: %v", err)
		}
		if v.VersionNumber != i {
			t.Fatalf("expected version %d, got %d", i, v.VersionNumber)
		}
	}

	latest, err := s.GetLatestVersion(ctx, p.ID)
	if err != nil || latest.VersionNumber != 3 {
		t.Fatalf("latest: %v n=%d", err, latest.VersionNumber)
	}
	// JSONB round-trips.
	if latest.ModelConfig["model"] != "gpt-4o" {
		t.Errorf("model_config didn't round-trip: %+v", latest.ModelConfig)
	}
	if len(latest.Variables) != 1 || latest.Variables[0].Name != "query" {
		t.Errorf("variables didn't round-trip: %+v", latest.Variables)
	}

	all, _ := s.ListVersions(ctx, p.ID)
	if len(all) != 3 {
		t.Errorf("expected 3 versions, got %d", len(all))
	}
}

func TestTagsAndRollback(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	p, _ := s.CreatePrompt(ctx, model.Prompt{Name: "p"})
	v1, _ := s.CreateVersion(ctx, model.Version{PromptID: p.ID, UserTemplate: "v1"})
	v2, _ := s.CreateVersion(ctx, model.Version{PromptID: p.ID, UserTemplate: "v2"})

	// Deploy v2 to production.
	if _, err := s.SetTag(ctx, p.ID, "production", v2.ID, "alice"); err != nil {
		t.Fatalf("set tag: %v", err)
	}
	resolved, err := s.ResolveVersion(ctx, p.ID, "production", 0)
	if err != nil || resolved.ID != v2.ID {
		t.Fatalf("resolve production: %v id=%s want=%s", err, resolved.ID, v2.ID)
	}

	// Roll back to v1 — same SetTag, moving the pointer.
	if _, err := s.SetTag(ctx, p.ID, "production", v1.ID, "bob"); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	resolved, _ = s.ResolveVersion(ctx, p.ID, "production", 0)
	if resolved.ID != v1.ID {
		t.Fatalf("after rollback expected v1, got %s", resolved.ID)
	}

	// Still exactly one production tag.
	tags, _ := s.ListTags(ctx, p.ID)
	prodCount := 0
	for _, tg := range tags {
		if tg.Tag == "production" {
			prodCount++
		}
	}
	if prodCount != 1 {
		t.Errorf("expected exactly one production tag, got %d", prodCount)
	}

	// Explicit version number beats tag.
	byNum, _ := s.ResolveVersion(ctx, p.ID, "production", 2)
	if byNum.ID != v2.ID {
		t.Errorf("explicit version should win, got %s", byNum.ID)
	}
}
