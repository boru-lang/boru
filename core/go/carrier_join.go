package core

// The carrier JOIN lattice (ADR-013, 2026-08-08 amendment). A branch —
// `if`, `case`, a loop body — produces one carrier per arm, and the
// analysis has to fold them into the single carrier the code after the
// branch sees. That fold is this file: type-level widening
// (CommonAncestorType), the value-level merge (JoinCarriers /
// JoinCarriersInner), the per-position residual fold
// (JoinCarrierStacks), and the per-name binding merge
// (InstallJoinedDefs).
//
// It lives in core, not check, because every operand and result is a
// core Value or *Type: nothing here consults the checker beyond
// r.Defs, which core owns. Keeping it here is what lets `basic` carry
// the analysis half of its control words without importing the
// checker — the ADR-013 rule that basic depends on the pieces it
// actually uses.
//
// Identity is load-bearing in two places and must not drift: the fresh
// ID JoinCarriersInner mints for a genuine if-RESULT (the compiler's
// per-value provenance reasons about it), and the ID joinBranchDef
// PRESERVES when the arms merely narrowed an enclosing binding.

// CommonAncestorType returns the longest common prefix of two type
// paths, as a new Type. For example, given Number/Integer/42 and
// Number/Integer/99, returns Number/Integer. Returns TAny if there is
// no shared prefix.
func CommonAncestorType(a, b *Type) *Type {
	if a == nil || b == nil {
		return TAny
	}
	seen := make(map[*Type]bool)
	for d := a; d != nil; d = d.Parent {
		seen[d] = true
	}
	for d := b; d != nil; d = d.Parent {
		if seen[d] {
			return d
		}
	}
	return TAny
}

// CarrierDisjunctCap is the maximum number of alternatives a carrier
// disjunction may hold before it is widened to the common ancestor
// of all alternatives. Matches the report's recommended cap of 8.
const CarrierDisjunctCap = 8

// FlattenAlternatives walks a carrier value and returns the unique
// type literals it represents. For a disjunct carrier, flattens its
// alternatives recursively; for any other carrier, returns a single
// type literal of its Parent.
func FlattenAlternatives(v Value) []Value {
	if IsDisjunct(v) {
		di, _ := AsDisjunct(v)
		var out []Value
		for _, alt := range di.Alternatives {
			out = append(out, FlattenAlternatives(alt)...)
		}
		return out
	}
	// A bare type literal IS its node — return it as-is. Taking
	// v.Parent of a literal would shift one lattice level up (the
	// node's parent), silently widening every stored alternative on
	// re-join. Carriers and concrete values stand for their Parent.
	if IsBareTypeNode(v) {
		return []Value{v}
	}
	return []Value{NewTypeLiteral(v.Parent)}
}

// JoinCarriers folds two carriers into a single carrier that
// represents the disjunction of both. Applies a few simple
// normalisations:
//
//   - Identical VTypes collapse to one carrier.
//   - If one side is a strict subtype of the other, the parent wins.
//   - Sibling literal types (e.g. Number/Integer/42 vs Number/Integer/99)
//     collapse to their nearest common ancestor (Number/Integer).
//   - Disjunctions wider than CarrierDisjunctCap widen to the common
//     ancestor of all alternatives.
//   - Otherwise a TDisjunct carrier is returned whose Data is a
//     DisjunctInfo listing the unique alternative type literals.
//
// This is the primary join used when the checker needs to combine
// two branch outcomes (e.g. `if` then/else).
// JoinCarriers merges two arm carriers (an `if`/loop/case branch result). If
// EITHER arm is gradual (dynamic), the merge is too — the same gradual
// contagion a dynamic operand already spreads through a dispatch result: a
// branch that may yield an unknown-typed value is itself optimistically typed,
// so the merge poly-matches a concrete slot instead of a strict disjunct
// rejecting it. Notably the "default-or-self" rebind `if (nd eq none) [Map]
// [nd]` over a `nd:Any` (gradual) param: the merge stays dynamic(Map|…) and a
// later `nd:Map` consumer matches, instead of a strict Disjunct(None|Map|…)
// failing no_signature (the tst/radix node-rebuild walkers). Looser, never
// tighter — a guard discharges the modality back to strict downstream.
func JoinCarriers(a, b Value) Value {
	out := JoinCarriersInner(a, b)
	if a.Dynamic || b.Dynamic {
		out.Carrier = true
		out.Dynamic = true
	}
	return out
}

// isNoneArm reports whether a branch carrier is the None sentinel — the bare
// None type node, a `none` value, or a carrier bound to None.
func isNoneArm(v Value) bool {
	if v.Parent != nil {
		return v.Parent.Equal(TNone)
	}
	return IsNoneShape(v)
}

func JoinCarriersInner(a, b Value) Value {
	// A None arm is a lattice-root literal — NewTypeLiteral(TNone) has BOTH
	// Data==nil AND Parent==nil (None has no lattice parent). The parent-math
	// collapse blocks below are unsafe against such a nil Parent: ConformsTo(nil)
	// and Equal(nil) are vacuously permissive, so `Integer.ConformsTo(None)` would
	// collapse the merge to NewCarrier(None.Parent==nil) — a Parent-less carrier
	// the engine HALTS on when it steps the `if` result (undefined stack entry).
	// Handle None arms explicitly: two None arms join to a None carrier; a mixed
	// None/value arm falls through to the alternatives-union path (None|T), the
	// tested general join — JoinCarrierStacks then flips that result gradual
	// (the optional / sentinel merge). Only the parent-math shortcuts are
	// bypassed; the union below already treats None as a first-class alternative.
	aNone := a.Parent == nil
	bNone := b.Parent == nil
	if aNone && bNone {
		return NewCarrier(TNone)
	}
	collapse := !aNone && !bNone
	if collapse && a.Parent.Equal(b.Parent) && !IsDisjunct(a) && !IsDisjunct(b) {
		out := a
		out.Carrier = true
		out.Data = nil
		// The merged carrier is a NEW value (an `if`/loop result), not arm `a`.
		// Keeping a's ID lets the result COLLIDE with a's own binding when an arm
		// returns a live local — `def v0 3 def v1 (if c [v0] [4])` makes the if-
		// result reuse v0's id, so a later `v0` reference resolves to the if-event
		// (the compiler bakes the wrong value). Mint a distinct identity.
		out.ID = GenerateID(IDPrefixForType(out.Parent))
		return out
	}
	if collapse && !IsDisjunct(a) && !IsDisjunct(b) {
		if a.Parent.ConformsTo(b.Parent) {
			// a is subtype of b → widen to b
			return NewCarrier(b.Parent)
		}
		if b.Parent.ConformsTo(a.Parent) {
			return NewCarrier(a.Parent)
		}
		// Collapse DIRECT siblings to their shared parent — the case
		// this was built for is value-tagged literals (Number/Integer/42
		// vs Number/Integer/99 → Number/Integer), and it also folds
		// Integer|Float → Number, where every dispatch the parent
		// reaches is one the alternatives reach identically. Distant
		// cousins (Integer vs String → Scalar) must NOT collapse: the
		// widened type changes first-match dispatch (Integer|String
		// reaching `add` picks the Scalar catch-all although the
		// Integer path takes [Number Number]) — keep them as a
		// disjunct so per-alternative dispatch
		// (disjunctPartitionReturns) sees the real alternatives.
		anc := CommonAncestorType(a.Parent, b.Parent)
		if anc != nil && !anc.Equal(TAny) &&
			a.Parent.Parent != nil && anc.Equal(a.Parent.Parent) &&
			b.Parent.Parent != nil && anc.Equal(b.Parent.Parent) {
			return NewCarrier(anc)
		}
	}
	// Gather unique alternatives across a and b, subsume subtypes,
	// then apply the width cap. SimplifyDisjunctAlts is the runtime
	// path's helper but produces identical output for the
	// type-literal-only inputs the carrier path supplies.
	combined := append([]Value(nil), FlattenAlternatives(a)...)
	combined = append(combined, FlattenAlternatives(b)...)
	alts := SimplifyDisjunctAlts(combined)
	if len(alts) == 1 {
		return CarrierOfLiteral(alts[0])
	}
	if len(alts) > CarrierDisjunctCap {
		t := TypeNodeOf(alts[0])
		for i := 1; i < len(alts); i++ {
			t = CommonAncestorType(t, TypeNodeOf(alts[i]))
		}
		return NewCarrier(t)
	}
	v := NewDisjunct(alts)
	v.Carrier = true
	return v
}

// joinBranchDef merges a name's per-arm carriers for InstallJoinedDefs.
// When both inputs carry the SAME identity — the arms only NARROWED the
// enclosing binding (narrowDynamicUses preserves the value's ID; a read
// like `mkm (tab)` narrows-through-use without reassigning), never
// rebound it — the merge is an identity no-op, so KEEP that ID. The
// binding's value is genuinely unchanged across the branch, and
// preserving its ID keeps its compile seat (producing event / frame
// local) reachable for references AFTER the join. JoinCarriers otherwise
// mints a fresh ID — correct for a genuine if-RESULT (it must not
// collide with an arm's live local, JoinCarriers' own §), but for a
// merely-narrowed enclosing def that fresh ID strands every later
// reference as an unseated dynamic carrier ("fn call operand of unknown
// provenance"; the aless viewer's render fn hit exactly this). A GENUINE
// reassignment gives the arms DIFFERING IDs and keeps the fresh-ID join.
func joinBranchDef(a, b Value) Value {
	out := JoinCarriers(a, b)
	if a.ID != "" && a.ID == b.ID {
		out.ID = a.ID
	}
	return out
}

// narrowedSameBinding reports whether an arm's `add` for a name is a
// NARROWING of the enclosing binding rather than a rebind: narrowDynamicUses
// preserves the value's ID when it tightens a dynamic binding at a typed use,
// so an add whose ID equals the pre-branch binding's ID is the SAME runtime
// value under a tighter static bound — an analysis artifact, not a binding
// transition. A genuine arm `def` mints a fresh ID and never matches. Codex
// (P2, PR #418) caught the consequence of pushing these: an arm that merely
// CONSUMED a dynamic name through a typed slot grew the def stack, the join
// re-pushed it, the ledger recorded a phantom `def` for a name the source
// never defines, and the joined carrier leaked past the pass. Skipping the
// push only loosens downstream matching to the pre-branch bound — sound, per
// narrowing's own doctrine — and every reference keeps its seat, because the
// pre binding carries the same ID.
func narrowedSameBinding(r *Registry, k string, v Value) bool {
	if !v.Dynamic || v.ID == "" {
		return false
	}
	pre, ok := r.Defs.Top(k)
	return ok && pre.ID == v.ID
}

// InstallJoinedDefs merges the `adds` maps from two branches back
// into r.DefStacks. If both branches defined the same name, their
// carriers are joined via joinBranchDef and the joined carrier is
// pushed. If only one branch defined it, that def is pushed back —
// but joined with the pre-branch carrier (if any) since the other
// branch's path kept the original binding.
// Every arm push here is a runtime-visible transition and is noted for the bind
// ledger (§6.5): the joined binding is what the check model LEAVES BEHIND, and a
// twin has to reproduce it. Recording only the speculative arm's own installDef
// would undercount exactly the branch-arm population NUR110 is about. An arm
// add that is only a NARROWING of the enclosing binding (narrowedSameBinding)
// is neither pushed nor noted — it is the pass's own refinement, not a binding
// the runtime leaves.
func InstallJoinedDefs(r *Registry, then, else_ map[string]Value) {
	seen := make(map[string]bool)
	for k, tv := range then {
		seen[k] = true
		if ev, ok := else_[k]; ok {
			if narrowedSameBinding(r, k, tv) && narrowedSameBinding(r, k, ev) {
				continue
			}
			r.Defs.Push(k, joinBranchDef(tv, ev))
			// The both-arms join is a transition exactly like the one-arm
			// joins below — this note was MISSING (the doc above claimed
			// every push noted; a both-arms `def op` left a live binding
			// with no ledger entry, which the ledgered-names-only oracle
			// could not see).
			r.NoteBindTransition(BindDef, k, tv.Pos())
			continue
		}
		// then-only: join with the pre-branch top-of-stack if any.
		if pre, ok := r.Defs.Top(k); ok {
			if narrowedSameBinding(r, k, tv) {
				continue
			}
			r.Defs.Push(k, joinBranchDef(tv, pre))
		} else {
			r.Defs.Push(k, tv)
			if specFnJoin(r, k) {
				continue
			}
		}
		r.NoteBindTransition(BindDef, k, tv.Pos())
	}
	for k, ev := range else_ {
		if seen[k] {
			continue
		}
		// else-only: join with pre-branch top-of-stack.
		if pre, ok := r.Defs.Top(k); ok {
			if narrowedSameBinding(r, k, ev) {
				continue
			}
			r.Defs.Push(k, joinBranchDef(ev, pre))
		} else {
			r.Defs.Push(k, ev)
			if specFnJoin(r, k) {
				continue
			}
		}
		r.NoteBindTransition(BindDef, k, ev.Pos())
	}
}

// JoinCarrierStacks folds two carrier result stacks (e.g. produced by
// two branches of an `if`) into a single stack. The shorter stack is
// padded out with TNone carriers; per-position join uses JoinCarriers.
func JoinCarrierStacks(a, b []Value) []Value {
	n := len(a)
	if len(b) > n {
		n = len(b)
	}
	out := make([]Value, n)
	for i := 0; i < n; i++ {
		var ai, bi Value
		aReal := i < len(a)
		bReal := i < len(b)
		if aReal {
			ai = a[i]
		} else {
			ai = NewCarrier(TNone)
		}
		if bReal {
			bi = b[i]
		} else {
			bi = NewCarrier(TNone)
		}
		joined := JoinCarriers(ai, bi)
		// Both arms produce a REAL value at this position and exactly one is
		// None: a None-vs-value merge is the optional / builder sentinel
		// pattern (`if (nd eq none) [build-node] [nd]`, where the else arm
		// hands back the still-`none` receiver) — None is "absent / not built
		// yet" and the downstream code targets the REAL type, so the merge is
		// gradual. Without this a DIRECT call passing a concrete `none`
		// (tst/radix's `TstMap.set` on an empty map → `none key val
		// tst-insert`) merged to a STRICT Disjunct(None|Map) whose None
		// alternative made every node-rebuild `get` fail no_signature — whereas
		// the same code reached through a gradual `:Any` param already merged
		// dynamically (JoinCarriers' arm-dynamic rule). Gated to both-real so a
		// PADDED None (a variadic 0-or-1 arm — `if cond [98]`) keeps its
		// precise strict shape; only a genuine value-vs-none branch widens.
		// Looser, never tighter; a guard discharges the modality back to strict.
		if aReal && bReal && (isNoneArm(ai) != isNoneArm(bi)) {
			joined.Carrier = true
			joined.Dynamic = true
		}
		out[i] = joined
	}
	return out
}
