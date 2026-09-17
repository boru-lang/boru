// Package shrink layers PBT-specific policy on top of the kernel
// eng/go/stackform package. The shrinker (Stage 5 of
// design/legacy/PBT-PLAN.10.ignore) uses the Policy here to decide which Ops in
// a failing StackForm are safe to delete, replace, or simplify, and
// the cost adjustments to drive failure-preserving minimisation.
//
// Two pieces:
//
//   - Transparency annotations (per-word) — Transparent | Generator |
//     Frozen | Opaque, per design/legacy/boru_property_based_reduction_report.10.ignore §8.
//   - ShrinkCost — wraps stackform.Cost with policy weights, per
//     report §9. Frozen words price high to discourage rewriting;
//     Transparent words price low so the reducer eagerly drops them.
package shrink

// Transparency classifies a word for shrinker purposes.
//
//   - Transparent: pure, side-effect-free, freely removable/replaceable.
//     Arithmetic, comparisons, stack ops, literal construction.
//   - Generator: produces randomness. rand.* family. The reducer
//     prefers smaller-yielding alternatives (e.g.
//     `Rand.int 0 100` → `Rand.int 0 1`).
//   - Frozen: side effects on external state (clock, IO, network).
//     Don't rewrite — semantics depend on environment. Priced high.
//   - Opaque: unknown user words. Conservative default; the reducer
//     leaves them alone but doesn't penalise them as hard as Frozen.
type Transparency int

const (
	Opaque Transparency = iota // default for unknown words
	Transparent
	Generator
	Frozen
)

// String for debug printing.
func (t Transparency) String() string {
	switch t {
	case Transparent:
		return "Transparent"
	case Generator:
		return "Generator"
	case Frozen:
		return "Frozen"
	case Opaque:
		return "Opaque"
	}
	return "Unknown"
}

// Policy associates each known word name with a Transparency class
// and carries the cost-adjustment weights ShrinkCost applies on top
// of the kernel-level stackform.Cost.
//
// `Words` is keyed by the same name the stackform.Call op carries.
// For module FnDef wrappers, that's the INNER NATIVE name (e.g.
// Rand.int's wrapper dispatches with Name="rand-int"), because the
// engine's trivial-delegation path passes fnDef.Name through to
// matchSignature → execMatch → OnCall.
//
// `Prefix` lets the policy match families wholesale (e.g. every
// "rand-*" word as Generator). Prefix lookup is tried only if Words
// has no exact match.
type Policy struct {
	Words   map[string]Transparency
	Prefix  map[string]Transparency // prefix -> class (e.g. "rand-" -> Generator)
	Weights map[Transparency]int    // per-class cost adjustment
}

// Classify returns the transparency class of `name`. Lookup order:
//  1. Words exact match
//  2. Prefix match (longest prefix wins)
//  3. Opaque (default)
func (p *Policy) Classify(name string) Transparency {
	if p == nil {
		return Opaque
	}
	if t, ok := p.Words[name]; ok {
		return t
	}
	// Longest-prefix match. The Prefix map is small enough that a
	// linear scan is fine; no need for a trie.
	bestLen := -1
	best := Opaque
	for prefix, t := range p.Prefix {
		if len(prefix) <= bestLen {
			continue
		}
		if startsWith(name, prefix) {
			bestLen = len(prefix)
			best = t
		}
	}
	if bestLen >= 0 {
		return best
	}
	return Opaque
}

// Weight returns the per-Op cost adjustment for a Transparency class.
// Higher = more expensive = reducer prefers to leave alone. Lower =
// cheap = reducer eagerly tries removing.
func (p *Policy) Weight(t Transparency) int {
	if p == nil || p.Weights == nil {
		return 0
	}
	return p.Weights[t]
}

// DefaultPolicy returns the canonical PBT classification + weights
// (per design/legacy/boru_property_based_reduction_report.10.ignore §8-9 and the
// PBT plan's Stage-4 reference table).
//
// Calibration:
//
//   - Transparent words add +0 (the reducer leaves cost equal to
//     stackform.Cost so transparent-Op removal is the obvious win).
//   - Generator words add +2 (slight bias against generator calls so
//     constant-shrinks like `Rand.int 0 1` are preferred over the
//     original range).
//   - Frozen words add +10 (heavy bias against rewriting — the
//     reducer essentially treats Frozen Ops as immovable).
//   - Opaque words add +5 (penalise unknown words enough that the
//     reducer prefers a recognised alternative when one exists).
func DefaultPolicy() *Policy {
	return &Policy{
		Words:  defaultWordTable(),
		Prefix: defaultPrefixTable(),
		Weights: map[Transparency]int{
			Transparent: 0,
			Generator:   2,
			Frozen:      10,
			Opaque:      5,
		},
	}
}

// defaultWordTable lists every kernel word the PBT plan classifies
// up front. Anything not here falls through to prefix lookup or to
// Opaque.
func defaultWordTable() map[string]Transparency {
	t := map[string]Transparency{}

	// Transparent: arithmetic + comparison + stack ops + boolean +
	// list/map construction. These are the safe targets for the
	// reducer's structural-deletion and literal-shrinking passes.
	for _, w := range []string{
		// Arithmetic
		"add", "sub", "mul", "div", "mod", "pow", "neg", "abs", "sign",
		"min", "max", "ceil", "floor", "round", "trunc", "sqrt", "cbrt",
		"exp", "log", "log2", "log10",
		// Comparison
		"eq", "neq", "lt", "gt", "lte", "gte", "cmp", "deq",
		// Boolean
		"and", "or", "xor", "nand", "implies", "not",
		// Stack ops
		"dup", "drop", "swap", "over", "rot", "nip", "tuck",
		"dup2", "drop2", "swap2", "over2", "depth", "pick", "roll",
		// List/Map construction & access
		"size", "append", "prepend", "head", "tail", "first", "last",
		"reverse", "concat", "flatten", "get", "getr", "set",
		// String
		"upper", "lower", "trim", "split", "join", "starts-with", "ends-with",
		// Conversion
		"convert",
		// Type introspection (pure)
		"typeof", "is", "inspect",
		// Identity
		"identity",
	} {
		t[w] = Transparent
	}

	// Frozen: clock, IO, network. The reducer must not rewrite these
	// because their behaviour depends on external state.
	for _, w := range []string{
		// Time / clock
		"now", "today", "today-utc", "now-local",
		// IO / fileops (when installed)
		"read", "write", "exists", "stat", "list-dir",
		// Network / process / env (when installed)
		"http-get", "http-post", "env-get",
	} {
		t[w] = Frozen
	}

	return t
}

// defaultPrefixTable groups whole families by name prefix. Module
// wrappers dispatch with their INNER NATIVE name (e.g. Rand.int's
// wrapper produces Call{Name: "rand-int"} in the stackform), so a
// "rand-" prefix catches every rand.* method.
func defaultPrefixTable() map[string]Transparency {
	return map[string]Transparency{
		// All rand.* are Generator. The reducer biases against them
		// slightly so it prefers tightening bounds (Rand.int 0 100
		// → Rand.int 0 1) over leaving the original range.
		"rand-": Generator,
		// time-tz, time-unix, time-format etc. — clock-dependent.
		"time-": Frozen,
		// fetch.* family — network-dependent.
		"fetch-": Frozen,
	}
}

// startsWith is the standard prefix check, inlined to avoid pulling
// in strings just for this.
func startsWith(s, prefix string) bool {
	if len(s) < len(prefix) {
		return false
	}
	return s[:len(prefix)] == prefix
}
