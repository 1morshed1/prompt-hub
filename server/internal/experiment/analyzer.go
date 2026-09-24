package experiment

import (
	"fmt"

	"github.com/1morshed1/prompt-hub/server/internal/model"
	"github.com/1morshed1/prompt-hub/server/internal/stats"
)

// ArmSummary is the aggregate view of one arm.
type ArmSummary struct {
	Arm        string  `json:"arm"`
	SampleSize int     `json:"sample_size"`
	RatedCount int     `json:"rated_count"` // results that carry a user_rating
	ThumbsUp   int     `json:"thumbs_up"`
	ThumbsDown int     `json:"thumbs_down"`
	AvgRating  float64 `json:"avg_rating"` // up-rate in [0,1]
	AvgLatency float64 `json:"avg_latency_ms"`
	AvgCost    float64 `json:"avg_cost_usd"`
}

// Report is the full analysis of an experiment — the payload behind the
// dashboard's side-by-side comparison and the /report API.
type Report struct {
	ExperimentID string     `json:"experiment_id"`
	Status       string     `json:"status"`
	Control      ArmSummary `json:"control"`
	Variant      ArmSummary `json:"variant"`

	// Rating comparison (chi-squared on thumbs up/down).
	RatingChiSquared  float64 `json:"rating_chi_squared"`
	RatingPValue      float64 `json:"rating_p_value"`
	RatingSignificant bool    `json:"rating_significant"`

	// Latency comparison (Mann-Whitney U).
	LatencyPValue      float64 `json:"latency_p_value"`
	LatencySignificant bool    `json:"latency_significant"`

	Winner         string `json:"winner"` // control | variant | no_winner
	Recommendation string `json:"recommendation"`
}

// SignificanceLevel is the alpha threshold below which a p-value is "significant".
const SignificanceLevel = 0.05

// Analyze aggregates results per arm and runs the appropriate statistical test on
// each metric. It's a pure function of its inputs (no DB), which makes it trivial
// to unit-test with synthetic result sets.
func Analyze(exp model.Experiment, results []model.Result) Report {
	control := summarize(ArmControl, results)
	variant := summarize(ArmVariant, results)

	rep := Report{
		ExperimentID: exp.ID,
		Status:       exp.Status,
		Control:      control,
		Variant:      variant,
	}

	// --- Ratings: chi-squared on the 2x2 up/down table ---
	chi := stats.ChiSquared2x2(
		control.ThumbsUp, control.ThumbsDown,
		variant.ThumbsUp, variant.ThumbsDown,
	)
	rep.RatingChiSquared = chi.ChiSquared
	rep.RatingPValue = chi.PValue
	rep.RatingSignificant = chi.PValue < SignificanceLevel

	// --- Latency: Mann-Whitney U on the raw latency samples ---
	mwu := stats.MannWhitneyU(latencies(control.Arm, results), latencies(variant.Arm, results))
	rep.LatencyPValue = mwu.PValue
	rep.LatencySignificant = mwu.PValue < SignificanceLevel

	// --- Winner: driven by the primary metric (user rating) ---
	switch {
	case rep.RatingSignificant && variant.AvgRating > control.AvgRating:
		rep.Winner = ArmVariant
	case rep.RatingSignificant && control.AvgRating > variant.AvgRating:
		rep.Winner = ArmControl
	default:
		rep.Winner = "no_winner"
	}
	rep.Recommendation = recommend(rep)
	return rep
}

func summarize(arm string, results []model.Result) ArmSummary {
	s := ArmSummary{Arm: arm}
	var latSum, costSum float64
	var latN, costN int
	for _, r := range results {
		if r.Arm != arm {
			continue
		}
		s.SampleSize++
		if r.UserRating != nil {
			s.RatedCount++
			if *r.UserRating == 1 {
				s.ThumbsUp++
			} else {
				s.ThumbsDown++
			}
		}
		if r.LatencyMS != nil {
			latSum += float64(*r.LatencyMS)
			latN++
		}
		if r.CostUSD != nil {
			costSum += *r.CostUSD
			costN++
		}
	}
	if s.RatedCount > 0 {
		s.AvgRating = float64(s.ThumbsUp) / float64(s.RatedCount)
	}
	if latN > 0 {
		s.AvgLatency = latSum / float64(latN)
	}
	if costN > 0 {
		s.AvgCost = costSum / float64(costN)
	}
	return s
}

func latencies(arm string, results []model.Result) []float64 {
	var out []float64
	for _, r := range results {
		if r.Arm == arm && r.LatencyMS != nil {
			out = append(out, float64(*r.LatencyMS))
		}
	}
	return out
}

// recommend turns the numbers into a sentence a human can act on. It leads with
// the rating verdict, then notes latency and cost as supporting context.
func recommend(r Report) string {
	switch r.Winner {
	case ArmVariant:
		msg := fmt.Sprintf("Promote the variant: its satisfaction rate (%.1f%%) beats control (%.1f%%) with p=%.4f.",
			r.Variant.AvgRating*100, r.Control.AvgRating*100, r.RatingPValue)
		return msg + latencyCostNote(r)
	case ArmControl:
		return fmt.Sprintf("Keep control: it outperforms the variant on satisfaction (%.1f%% vs %.1f%%, p=%.4f).",
			r.Control.AvgRating*100, r.Variant.AvgRating*100, r.RatingPValue) + latencyCostNote(r)
	default:
		return fmt.Sprintf("No significant difference in satisfaction yet (p=%.4f). Keep the experiment running to gather more samples.",
			r.RatingPValue)
	}
}

func latencyCostNote(r Report) string {
	note := ""
	if r.LatencySignificant {
		faster, slower := "variant", "control"
		if r.Variant.AvgLatency > r.Control.AvgLatency {
			faster, slower = "control", "variant"
		}
		note += fmt.Sprintf(" Latency differs significantly (p=%.4f); %s is faster than %s.",
			r.LatencyPValue, faster, slower)
	}
	if r.Variant.AvgCost != r.Control.AvgCost {
		delta := r.Variant.AvgCost - r.Control.AvgCost
		dir := "more"
		if delta < 0 {
			dir = "less"
			delta = -delta
		}
		note += fmt.Sprintf(" Variant costs $%.6f %s per request on average.", delta, dir)
	}
	return note
}
