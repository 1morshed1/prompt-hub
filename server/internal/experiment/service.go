package experiment

import (
	"context"

	"github.com/1morshed1/prompt-hub/server/internal/model"
	"github.com/1morshed1/prompt-hub/server/internal/store"
)

// ProductionTag is the tag auto-promotion moves. Making it a constant keeps the
// "what does winning mean" decision in one place.
const ProductionTag = "production"

// Service composes the store and the analyzer into the experiment workflow used
// by the HTTP layer.
type Service struct {
	store *store.Store
}

// NewService builds a Service.
func NewService(s *store.Store) *Service { return &Service{store: s} }

// ReportFor loads an experiment and its results and returns the analysis.
func (svc *Service) ReportFor(ctx context.Context, experimentID string) (Report, model.Experiment, error) {
	exp, err := svc.store.GetExperiment(ctx, experimentID)
	if err != nil {
		return Report{}, model.Experiment{}, err
	}
	results, err := svc.store.ResultsFor(ctx, experimentID)
	if err != nil {
		return Report{}, model.Experiment{}, err
	}
	return Analyze(exp, results), exp, nil
}

// PromotionDecision explains an auto-promote evaluation, so the outcome is
// transparent (the dashboard/logs can show *why* nothing was promoted).
type PromotionDecision struct {
	Promoted bool   `json:"promoted"`
	Reason   string `json:"reason"`
}

// MaybeAutoPromote evaluates an experiment and promotes the variant to production
// if it has genuinely, significantly won — but only when the experiment opted in
// (auto_promote) and both arms have reached min_samples. The sample-size gate
// matters: a "significant" result on 8 requests is noise dressed as signal, so we
// refuse to act until there's enough data for the normal approximations to hold.
//
// Promotion moves the production tag to the variant version and marks the
// experiment 'promoted'. Rollback, if you disagree later, is just moving the tag
// back — see the versioning doc.
func (svc *Service) MaybeAutoPromote(ctx context.Context, exp model.Experiment) (PromotionDecision, error) {
	if !exp.AutoPromote {
		return PromotionDecision{false, "auto-promote is disabled for this experiment"}, nil
	}
	if exp.Status != "running" {
		return PromotionDecision{false, "experiment is not running"}, nil
	}

	results, err := svc.store.ResultsFor(ctx, exp.ID)
	if err != nil {
		return PromotionDecision{}, err
	}
	rep := Analyze(exp, results)

	// Gate 1: enough rated samples in BOTH arms.
	if rep.Control.RatedCount < exp.MinSamples || rep.Variant.RatedCount < exp.MinSamples {
		return PromotionDecision{false, "not enough rated samples yet in both arms"}, nil
	}
	// Gate 2: the variant is the significant winner on the primary metric.
	if rep.Winner != ArmVariant {
		return PromotionDecision{false, "variant is not a significant winner"}, nil
	}

	// Promote: move production to the variant version, close the experiment.
	if _, err := svc.store.SetTag(ctx, exp.PromptID, ProductionTag, exp.VariantVersionID, "auto-promote"); err != nil {
		return PromotionDecision{}, err
	}
	if err := svc.store.SetExperimentStatus(ctx, exp.ID, "promoted"); err != nil {
		return PromotionDecision{}, err
	}
	return PromotionDecision{true, "variant won significantly; promoted to production"}, nil
}
