package experiment

import (
	"fmt"
	"math"
	"testing"

	"github.com/1morshed1/prompt-hub/server/internal/model"
)

// ptr helpers for the pointer metric fields.
func iptr(i int) *int         { return &i }
func fptr(f float64) *float64 { return &f }

func TestAssignRespectsSplit(t *testing.T) {
	exp := model.Experiment{TrafficSplit: 0.3, ControlVersionID: "c", VariantVersionID: "v"}
	variant := 0
	const n = 20000
	for i := 0; i < n; i++ {
		arm, vid := Assign(exp, fmt.Sprintf("user-%d", i))
		if arm == ArmVariant {
			variant++
			if vid != "v" {
				t.Fatalf("variant arm returned wrong version id %q", vid)
			}
		}
	}
	frac := float64(variant) / n
	if math.Abs(frac-0.3) > 0.02 {
		t.Errorf("variant fraction %.3f not close to 0.3", frac)
	}
}

func TestAssignIsDeterministic(t *testing.T) {
	exp := model.Experiment{TrafficSplit: 0.5, ControlVersionID: "c", VariantVersionID: "v"}
	arm1, _ := Assign(exp, "stable-user-42")
	arm2, _ := Assign(exp, "stable-user-42")
	if arm1 != arm2 {
		t.Errorf("same key gave different arms: %s vs %s", arm1, arm2)
	}
}

// buildResults makes n control + n variant results with the given up-rates and
// latencies, for analyzer tests.
func buildResults(nControl, upControl, nVariant, upVariant, latControl, latVariant int) []model.Result {
	var rs []model.Result
	for i := 0; i < nControl; i++ {
		rating := 0
		if i < upControl {
			rating = 1
		}
		rs = append(rs, model.Result{Arm: ArmControl, UserRating: iptr(rating), LatencyMS: iptr(latControl), CostUSD: fptr(0.002)})
	}
	for i := 0; i < nVariant; i++ {
		rating := 0
		if i < upVariant {
			rating = 1
		}
		rs = append(rs, model.Result{Arm: ArmVariant, UserRating: iptr(rating), LatencyMS: iptr(latVariant), CostUSD: fptr(0.001)})
	}
	return rs
}

func TestAnalyzeVariantWins(t *testing.T) {
	// Control 50/200 up (25%), Variant 100/200 up (50%): a big, clearly
	// significant difference favoring the variant.
	results := buildResults(200, 50, 200, 100, 1200, 900)
	rep := Analyze(model.Experiment{ID: "e"}, results)

	if rep.Control.AvgRating >= rep.Variant.AvgRating {
		t.Fatalf("expected variant avg rating higher: c=%.3f v=%.3f", rep.Control.AvgRating, rep.Variant.AvgRating)
	}
	if !rep.RatingSignificant {
		t.Errorf("expected significant rating difference, p=%.5f", rep.RatingPValue)
	}
	if rep.Winner != ArmVariant {
		t.Errorf("expected variant winner, got %q", rep.Winner)
	}
}

func TestAnalyzeNoWinner(t *testing.T) {
	// Identical up-rates -> no significant difference.
	results := buildResults(200, 100, 200, 100, 1000, 1000)
	rep := Analyze(model.Experiment{ID: "e"}, results)
	if rep.RatingSignificant {
		t.Errorf("identical rates should not be significant, p=%.5f", rep.RatingPValue)
	}
	if rep.Winner != "no_winner" {
		t.Errorf("expected no_winner, got %q", rep.Winner)
	}
}

func TestAnalyzeSummaryMath(t *testing.T) {
	results := buildResults(10, 7, 10, 3, 500, 800)
	rep := Analyze(model.Experiment{ID: "e"}, results)
	if rep.Control.ThumbsUp != 7 || rep.Control.ThumbsDown != 3 {
		t.Errorf("control up/down wrong: %+v", rep.Control)
	}
	if math.Abs(rep.Control.AvgRating-0.7) > 1e-9 {
		t.Errorf("control avg rating = %v, want 0.7", rep.Control.AvgRating)
	}
	if math.Abs(rep.Variant.AvgLatency-800) > 1e-9 {
		t.Errorf("variant avg latency = %v, want 800", rep.Variant.AvgLatency)
	}
}
