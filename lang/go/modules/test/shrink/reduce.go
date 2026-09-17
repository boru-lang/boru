package shrink

import (
	"github.com/boru-lang/boru/lang/go/stackform"
)

// Outcome reports how an evaluation of a candidate StackForm
// resolved against the user's property predicate.
//
//   - Fail:    candidate is failure-preserving. Reducer accepts it
//     if it lowers cost.
//   - Pass:    property holds — candidate does NOT preserve the
//     failure. Reducer rejects.
//   - Invalid: candidate failed to evaluate at all (engine error,
//     missing word, type mismatch). Reducer rejects.
type Outcome int

const (
	Pass Outcome = iota
	Fail
	Invalid
)

// EvalFn is the caller-supplied function that evaluates a candidate
// StackForm and reports whether it preserves the original failure.
// For the PBT integration in boru:test, this closes over the property
// body and the seeded rand instance so each candidate reproduces the
// same generator/property pipeline.
type EvalFn func(*stackform.StackForm) Outcome

// Strategy selects the reducer's search algorithm.
type Strategy int

const (
	// Greedy: at each step pick the lowest-cost failure-preserving
	// candidate and commit to it. Stops the moment no candidate
	// lowers cost. Fast, deterministic, sufficient for typical
	// counterexamples — the existing PBT integration's default.
	Greedy Strategy = iota

	// BestFirst: maintain a priority queue of failure-preserving
	// candidates. Pop the lowest-cost open state, expand its
	// children, push improving ones back. Continues exploring
	// until the queue empties or MaxSteps is reached, returning
	// the lowest-cost failing state ever observed. Wider search
	// than Greedy — finds better reductions when the local lowest-
	// cost candidate dead-ends but a slightly-higher-cost sibling
	// shrinks further. More work per call; opt-in.
	BestFirst
)

// Profile bundles the reducer's tuning knobs — how many iterations
// to attempt, which Policy to consult for cost weighting + word
// classification, and which search Strategy to use.
type Profile struct {
	// MaxSteps caps the outer loop. For Greedy, this is the max
	// number of accepted reductions. For BestFirst, this is the
	// max total candidate evaluations (across all explored states).
	// 200 is a sensible default for most cases.
	MaxSteps int

	// Policy controls per-word transparency + cost weights. See
	// shrink/policy.go.
	Policy *Policy

	// Strategy selects greedy descent (default) or best-first
	// search. BestFirst widens the search at the cost of more
	// candidate evaluations per Reduce call.
	Strategy Strategy

	// BeamWidth caps the BestFirst priority queue. Zero means
	// "use the implementation default" (16). Ignored under Greedy.
	BeamWidth int
}

// DefaultProfile returns sensible defaults: 200 max steps, the
// canonical DefaultPolicy, Greedy strategy.
func DefaultProfile() *Profile {
	return &Profile{
		MaxSteps: 200,
		Policy:   DefaultPolicy(),
		Strategy: Greedy,
	}
}

// Reduce minimises `initial` to the smallest StackForm (by
// ShrinkCost under the profile's Policy) for which `eval` still
// returns Fail. Dispatches to greedyReduce or bestFirstReduce
// based on profile.Strategy.
//
// Returns the smallest failing form found. If `initial` itself does
// NOT fail under `eval`, returns it unchanged — Reduce only shrinks
// known counterexamples.
//
// Algorithms mirror design/legacy/boru_property_based_reduction_report.10.ignore:
// §11 (greedy failure-preserving reduction) and §15 (best-first
// search). Exact small-program enumeration (§16) remains future
// work.
func Reduce(initial *stackform.StackForm, eval EvalFn, profile *Profile) *stackform.StackForm {
	if profile == nil {
		profile = DefaultProfile()
	}
	if initial == nil {
		return nil
	}
	// Reducer contract: only shrink genuine counterexamples. If the
	// caller hands us a form that doesn't fail, return it as-is.
	if eval(initial) != Fail {
		return initial
	}
	if profile.Strategy == BestFirst {
		return bestFirstReduce(initial, eval, profile)
	}
	return greedyReduce(initial, eval, profile)
}

// greedyReduce is the canonical PBT shrinking algorithm: at each
// step, take the lowest-cost failure-preserving candidate and
// commit. Stops when no candidate lowers cost OR MaxSteps reached.
//
// Fast and deterministic. Fits most real PBT counterexamples
// because typical shrinking paths are monotone (each step's best
// candidate is also the best long-term path).
func greedyReduce(initial *stackform.StackForm, eval EvalFn, profile *Profile) *stackform.StackForm {
	current := initial
	cost := ShrinkCost(current, profile.Policy)

	// Fingerprint set so a candidate already tried isn't
	// re-evaluated. Pretty serialisation is the fingerprint —
	// distinct forms have distinct pretty strings.
	seen := map[string]bool{stackform.Pretty(current): true}

	for step := 0; step < profile.MaxSteps; step++ {
		cands := generateCandidates(current, profile.Policy)
		sortByCost(cands, profile.Policy)

		accepted := false
		for _, cand := range cands {
			fp := stackform.Pretty(cand)
			if seen[fp] {
				continue
			}
			seen[fp] = true

			candCost := ShrinkCost(cand, profile.Policy)
			if candCost >= cost {
				// Sorted ascending — nothing remaining helps either.
				break
			}

			if eval(cand) == Fail {
				current, cost = cand, candCost
				accepted = true
				break
			}
		}
		if !accepted {
			break
		}
	}
	return current
}

// bestFirstReduce maintains a priority queue of failure-preserving
// states. At each iteration it pops the lowest-cost open state,
// expands its children, and pushes improving children back into
// the queue. Continues until the queue is empty or MaxSteps total
// candidate evaluations is exhausted.
//
// Returns the lowest-cost confirmed-failing state observed across
// the search. Wider than greedyReduce — explores siblings that
// greedy would abandon, useful when the locally-best candidate
// dead-ends.
//
// BeamWidth caps the queue (defaults to 16). When queue length
// exceeds the beam, worst entries are dropped — keeps memory and
// search cost bounded for pathological cases.
func bestFirstReduce(initial *stackform.StackForm, eval EvalFn, profile *Profile) *stackform.StackForm {
	policy := profile.Policy
	beam := profile.BeamWidth
	if beam <= 0 {
		beam = 16
	}

	initialCost := ShrinkCost(initial, policy)
	best := initial
	bestCost := initialCost

	seen := map[string]bool{stackform.Pretty(initial): true}
	queue := []*stackform.StackForm{initial}

	steps := 0
	for len(queue) > 0 && steps < profile.MaxSteps {
		// Pop lowest-cost (queue maintained sorted ascending).
		current := queue[0]
		queue = queue[1:]

		cands := generateCandidates(current, policy)
		sortByCost(cands, policy)

		for _, cand := range cands {
			if steps >= profile.MaxSteps {
				break
			}
			fp := stackform.Pretty(cand)
			if seen[fp] {
				continue
			}
			seen[fp] = true

			candCost := ShrinkCost(cand, policy)
			if candCost >= bestCost {
				// No way to improve on the current best from here
				// (sorted ascending — rest are no better).
				break
			}

			steps++
			if eval(cand) != Fail {
				continue
			}

			// Failure-preserving + improving. Update best, enqueue
			// for further expansion.
			best = cand
			bestCost = candCost
			queue = insertSorted(queue, cand, policy)
			if len(queue) > beam { //covergate:allow module provably-invariant / grammar-defensive guard (§modules)
				queue = queue[:beam]
			}
		}
	}
	return best
}

// insertSorted inserts `form` into `queue` (sorted ascending by
// ShrinkCost) and returns the result. Linear scan — queue length
// is bounded by BeamWidth so this is fine.
func insertSorted(queue []*stackform.StackForm, form *stackform.StackForm, policy *Policy) []*stackform.StackForm {
	cost := ShrinkCost(form, policy)
	for i, q := range queue {
		if ShrinkCost(q, policy) > cost {
			out := make([]*stackform.StackForm, 0, len(queue)+1)
			out = append(out, queue[:i]...)
			out = append(out, form)
			out = append(out, queue[i:]...)
			return out
		}
	}
	return append(queue, form)
}

// sortByCost orders cands in-place ascending by ShrinkCost.
// Insertion sort is fine — candidate counts are small (O(N) where
// N = number of ops in the form).
func sortByCost(cands []*stackform.StackForm, policy *Policy) {
	for i := 1; i < len(cands); i++ {
		ci := ShrinkCost(cands[i], policy)
		j := i
		for j > 0 && ShrinkCost(cands[j-1], policy) > ci {
			cands[j], cands[j-1] = cands[j-1], cands[j]
			j--
		}
	}
}
