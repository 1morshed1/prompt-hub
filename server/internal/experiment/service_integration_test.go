package experiment_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/1morshed1/prompt-hub/server/internal/experiment"
	"github.com/1morshed1/prompt-hub/server/internal/model"
	"github.com/1morshed1/prompt-hub/server/internal/store"
	"github.com/1morshed1/prompt-hub/server/internal/testutil"
)

func newStore(t *testing.T) *store.Store {
	return testutil.Setup(t)
}

// TestFullExperimentFlow drives assignment -> result logging -> analysis ->
// auto-promotion against a real database, exercising the whole workflow.
func TestFullExperimentFlow(t *testing.T) {
	s := newStore(t)
	svc := experiment.NewService(s)
	ctx := context.Background()

	p, _ := s.CreatePrompt(ctx, model.Prompt{Name: "assistant"})
	v1, _ := s.CreateVersion(ctx, model.Version{PromptID: p.ID, UserTemplate: "control"})
	v2, _ := s.CreateVersion(ctx, model.Version{PromptID: p.ID, UserTemplate: "variant"})
	// Start with production on control.
	_, _ = s.SetTag(ctx, p.ID, "production", v1.ID, "seed")

	exp, err := s.CreateExperiment(ctx, model.Experiment{
		PromptID:         p.ID,
		Name:             "v2 test",
		ControlVersionID: v1.ID,
		VariantVersionID: v2.ID,
		TrafficSplit:     0.5,
		AutoPromote:      true,
		MinSamples:       50,
	})
	if err != nil {
		t.Fatalf("create experiment: %v", err)
	}

	// A second running experiment for the same prompt must conflict.
	if _, err := s.CreateExperiment(ctx, model.Experiment{
		PromptID: p.ID, ControlVersionID: v1.ID, VariantVersionID: v2.ID,
	}); err != store.ErrConflict {
		t.Errorf("expected ErrConflict for second running experiment, got %v", err)
	}

	// Simulate 120 requests per arm. Control up-rate 30%, variant up-rate 70%.
	logArm := func(arm, versionID string, n, up, latency int) {
		for i := 0; i < n; i++ {
			reqID := uuid()
			if err := s.RecordUsage(ctx, model.Result{PromptID: p.ID, 
				ExperimentID: exp.ID, VersionID: versionID, RequestID: reqID, Arm: arm,
			}); err != nil {
				t.Fatalf("record assignment: %v", err)
			}
			rating := 0
			if i < up {
				rating = 1
			}
			lat := latency
			if err := s.LogResult(ctx, model.Result{RequestID: reqID, UserRating: &rating, LatencyMS: &lat}); err != nil {
				t.Fatalf("log result: %v", err)
			}
		}
	}
	logArm(experiment.ArmControl, v1.ID, 120, 36, 1200) // 30% up
	logArm(experiment.ArmVariant, v2.ID, 120, 84, 950)  // 70% up

	// Idempotency: re-recording an assignment for an existing request id is a no-op.
	results, _ := s.ResultsFor(ctx, exp.ID)
	if len(results) != 240 {
		t.Fatalf("expected 240 results, got %d", len(results))
	}

	// Report shows the variant winning significantly.
	rep, _, err := svc.ReportFor(ctx, exp.ID)
	if err != nil {
		t.Fatalf("report: %v", err)
	}
	if rep.Winner != experiment.ArmVariant || !rep.RatingSignificant {
		t.Fatalf("expected significant variant win, got winner=%s sig=%v p=%.5f",
			rep.Winner, rep.RatingSignificant, rep.RatingPValue)
	}

	// Auto-promote should move production to v2 and close the experiment.
	dec, err := svc.MaybeAutoPromote(ctx, mustGet(t, s, exp.ID))
	if err != nil {
		t.Fatalf("auto-promote: %v", err)
	}
	if !dec.Promoted {
		t.Fatalf("expected promotion, reason: %s", dec.Reason)
	}
	prod, _ := s.GetTag(ctx, p.ID, "production")
	if prod.VersionID != v2.ID {
		t.Errorf("production should point at v2 after promotion, got %s", prod.VersionID)
	}
	promoted := mustGet(t, s, exp.ID)
	if promoted.Status != "promoted" {
		t.Errorf("experiment status should be 'promoted', got %s", promoted.Status)
	}
}

func TestAutoPromoteRespectsMinSamples(t *testing.T) {
	s := newStore(t)
	svc := experiment.NewService(s)
	ctx := context.Background()
	p, _ := s.CreatePrompt(ctx, model.Prompt{Name: "p"})
	v1, _ := s.CreateVersion(ctx, model.Version{PromptID: p.ID})
	v2, _ := s.CreateVersion(ctx, model.Version{PromptID: p.ID})
	exp, _ := s.CreateExperiment(ctx, model.Experiment{
		PromptID: p.ID, ControlVersionID: v1.ID, VariantVersionID: v2.ID,
		AutoPromote: true, MinSamples: 100,
	})
	// Only a handful of lopsided samples: significant-looking but below min_samples.
	for i := 0; i < 5; i++ {
		r := uuid()
		_ = s.RecordUsage(ctx, model.Result{PromptID: p.ID, ExperimentID: exp.ID, VersionID: v1.ID, RequestID: r, Arm: experiment.ArmControl})
		down := 0
		_ = s.LogResult(ctx, model.Result{RequestID: r, UserRating: &down})
		r2 := uuid()
		_ = s.RecordUsage(ctx, model.Result{PromptID: p.ID, ExperimentID: exp.ID, VersionID: v2.ID, RequestID: r2, Arm: experiment.ArmVariant})
		up := 1
		_ = s.LogResult(ctx, model.Result{RequestID: r2, UserRating: &up})
	}
	dec, err := svc.MaybeAutoPromote(ctx, exp)
	if err != nil {
		t.Fatalf("auto-promote: %v", err)
	}
	if dec.Promoted {
		t.Errorf("should not promote below min_samples, but did: %s", dec.Reason)
	}
}

func mustGet(t *testing.T, s *store.Store, id string) model.Experiment {
	t.Helper()
	e, err := s.GetExperiment(context.Background(), id)
	if err != nil {
		t.Fatalf("get experiment: %v", err)
	}
	return e
}

// uuid returns a fresh v4-ish UUID string for request ids in tests, using the
// database's gen_random_uuid via a helper would need a round-trip, so we build a
// simple random one here.
var uuidCounter int

func uuid() string {
	uuidCounter++
	return fmt.Sprintf("00000000-0000-4000-8000-%012d", uuidCounter)
}
