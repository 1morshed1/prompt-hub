// Package experiment wires the A/B testing workflow together: assigning traffic
// to arms, analyzing accumulated results with the stats package, and (optionally)
// auto-promoting a winning variant. The statistics themselves live in
// internal/stats; this package is the workflow around them.
package experiment

import (
	"hash/fnv"

	"github.com/1morshed1/prompt-hub/server/internal/model"
)

// Arm names.
const (
	ArmControl = "control"
	ArmVariant = "variant"
)

// Assign decides which arm a request belongs to, deterministically from a unit
// key. The key can be a stable identifier (a user id, a session id) so the SAME
// unit always sees the SAME arm — "sticky" assignment, which avoids a user
// flip-flopping between prompt versions mid-conversation. If the caller has no
// stable key, a fresh per-request id works too; assignment is then effectively
// random but still respects the split.
//
// We hash the key to a uniform fraction in [0,1) and compare against the variant
// split. Hashing (rather than a coin flip) is what makes it deterministic and
// reproducible: given the same key and split, you always get the same arm.
func Assign(exp model.Experiment, unitKey string) (arm string, versionID string) {
	if fraction(unitKey) < exp.TrafficSplit {
		return ArmVariant, exp.VariantVersionID
	}
	return ArmControl, exp.ControlVersionID
}

// fraction maps a string to a uniform value in [0,1) using FNV-1a. FNV is fast,
// has good avalanche for this purpose, and is in the standard library.
func fraction(key string) float64 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	// Divide by 2^32 to land in [0,1).
	return float64(h.Sum32()) / float64(1<<32)
}
