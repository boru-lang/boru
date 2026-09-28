package core

// Function-signature comparison helpers — the single home for FnSig
// shape-matching across the engine. Two distinct rules live side by
// side:
//
//   - **Exact match** (FnSigMatchesSpec) — same arity AND pairwise
//     Type.Equal on params and returns. Used by `undef name fn [spec]`
//     to identify the precise previously-installed signature to remove.
//   - **Structural subtyping** (FnSigSatisfiesSpec) — same arity,
//     contravariant inputs (`spec ⊆ sig`) and covariant returns (`sig
//     ⊆ spec`). Used by `type Foo fn [...]` constraint matching via
//     `FnDefHasSig` / `FnUndefMatchesFnDef`.
//
// A FunctionSignature value (type Type/FunctionSignature) carries a
// list of FnSigSpec entries — each one a (Params, Returns) pair
// without a body. It's produced by `fnsig [[input] [output] …]` and
// acts as a structural function-shape constraint. Pattern
// (FnParam.Pattern) and Optional/BarrierPos
// differences are not yet considered.

// FnSigMatchesSpec returns true if a FnSig matches a FnSigSpec
// exactly: same arity, same param types pairwise, same return types
// pairwise. Variance is intentionally NOT applied — `undef name fn
// [spec]` names a specific shape to discard.
func FnSigMatchesSpec(sig FnSig, spec FnSigSpec) bool {
	if len(sig.Params) != len(spec.Params) {
		return false
	}
	for i := range sig.Params {
		if !sig.Params[i].Type.Equal(spec.Params[i].Type) {
			return false
		}
	}
	if len(sig.Returns) != len(spec.Returns) {
		return false
	}
	for i := range sig.Returns {
		if !sig.Returns[i].Equal(spec.Returns[i]) {
			return false
		}
	}
	return true
}

// FnSigSatisfiesSpec returns true if a candidate FnSig satisfies a
// FnSigSpec under structural function subtyping:
//
//   - **Inputs are contravariant on Type.** Each spec param type
//     must be a subtype of the candidate's param type at the same
//     position. Example: spec=[Integer], sig=[Number] — sig accepts
//     Integer (because Integer ⊂ Number), so it satisfies the spec.
//   - **Returns are covariant.** Each candidate return type must be
//     a subtype of the spec's return type at the same position.
//     Example: spec=[Number], sig=[Integer] — sig produces Integer
//     which is also a Number, so it satisfies the spec.
//   - **Optional alignment.** If spec.Params[i].Optional is true the
//     spec may omit arg i; the candidate must therefore also accept
//     omission (sig.Params[i].Optional must be true). The reverse
//     (sig optional, spec required) is fine — the candidate accepts
//     a superset of call shapes.
//   - **Pattern compatibility.** When the spec declares a Pattern
//     for arg i, the candidate's Pattern (if any) must accept every
//     value the spec admits — i.e., the spec's pattern must unify
//     with the candidate's. A spec without a pattern is satisfied
//     by any candidate (pattern absence = no extra constraint).
//
// BarrierPos is intentionally NOT compared — FnSigSpec doesn't
// carry one (it's a body-level collection setting, not part of the
// structural shape), so the type system can't declare a barrier
// requirement. Candidates may have any BarrierPos.
func FnSigSatisfiesSpec(sig FnSig, spec FnSigSpec) bool {
	return fnSigSatisfiesSpecR(sig, spec, nil)
}

// fnSigSatisfiesSpecR is FnSigSatisfiesSpec with the registry of an
// enclosing unify chain threaded into the Pattern-compatibility unify.
// The fn-shape unifiers (unifyFnUndefShape, FnUndefUnifier) run this
// check from INSIDE unifyInner, so a pattern pair such as `[:Pos]`
// against `[:Pos]` is decided with the chain's registry — a
// predicate-typed child resolves exactly as it would at the chain's
// top level. The exported entry, called from outside any unify, passes
// nil and stays unarmed.
func fnSigSatisfiesSpecR(sig FnSig, spec FnSigSpec, r *Registry) bool {
	if len(sig.Params) != len(spec.Params) {
		return false
	}
	for i := range sig.Params {
		sp := spec.Params[i]
		sg := sig.Params[i]
		// Contravariant: spec_input must be a subtype of sig_input.
		// `t.ConformsTo(pattern)` is true iff t ⊆ pattern in the type
		// lattice, so spec.Type.ConformsTo(sig.Type) checks spec ⊆ sig.
		if !sp.Type.ConformsTo(sg.Type) {
			return false
		}
		// Optional alignment: spec-optional → candidate must also be
		// optional. spec-required → candidate may be either.
		if sp.Optional && !sg.Optional {
			return false
		}
		// Pattern compatibility: when the spec demands a pattern,
		// the candidate's pattern must accept everything the spec
		// admits. Spec.Pattern == nil means no extra constraint.
		if sp.Pattern != nil {
			if sg.Pattern == nil {
				// Candidate doesn't constrain the arg at all, but
				// the spec does — the candidate's broader contract
				// still satisfies the spec's narrower demand.
				continue
			}
			if _, uerr := unifyWithin(*sp.Pattern, *sg.Pattern, r); uerr != nil {
				return false
			}
		}
	}
	if len(sig.Returns) != len(spec.Returns) {
		return false
	}
	for i := range sig.Returns {
		// Covariant: sig_return must be a subtype of spec_return.
		if !sig.Returns[i].ConformsTo(spec.Returns[i]) {
			return false
		}
	}
	return true
}

// FnUndefMatchesFnDef reports whether the candidate function value
// (TFunction wrapping FnDefInfo) satisfies every FnSigSpec
// declared by the FnUndef constraint.
func FnUndefMatchesFnDef(undef Value, fnVal Value) bool {
	return fnUndefMatchesFnDefR(undef, fnVal, nil)
}

// fnUndefMatchesFnDefR is FnUndefMatchesFnDef with the enclosing unify
// chain's registry threaded through to the Pattern-compatibility unify
// (see fnSigSatisfiesSpecR).
func fnUndefMatchesFnDefR(undef Value, fnVal Value, r *Registry) bool {
	uInfo, ok := undef.Data.(FnUndefInfo)
	if !ok {
		return false
	}
	fnDef, ok := fnVal.Data.(FnDefInfo)
	if !ok {
		return false
	}
	if len(uInfo.Sigs) == 0 {
		// An empty constraint trivially matches any function. Treat
		// this as an authoring error in practice — but it's well
		// defined.
		return true
	}
	for _, want := range uInfo.Sigs {
		if !fnDefHasSigR(fnDef, want, r) {
			return false
		}
	}
	return true
}

// FnDefHasSig reports whether the candidate has at least one
// signature that satisfies `want` under structural subtyping. It reads
// the function's own overloads (OwnSigs — full-fidelity Params/Returns,
// fallback excluded) so both boru fns and Go-implemented words are
// considered. The variance rule is delegated to FnSigSatisfiesSpec.
func FnDefHasSig(fnDef FnDefInfo, want FnSigSpec) bool {
	return fnDefHasSigR(fnDef, want, nil)
}

// fnDefHasSigR is FnDefHasSig with the enclosing unify chain's registry
// threaded through (see fnSigSatisfiesSpecR).
func fnDefHasSigR(fnDef FnDefInfo, want FnSigSpec, r *Registry) bool {
	for _, s := range fnDef.OwnSigs() {
		if fnSigSatisfiesSpecR(s, want, r) {
			return true
		}
	}
	return false
}

// MatchFnSig finds the first OWN signature of a fn VALUE whose params admit
// args, or nil when none does. Params are matched pairwise in sig order, which
// is the order a forward-bound call presents them.
//
// It lives in core because every operand is a core type, and because BOTH
// engines have to ask the same question: the interpreter's word dispatch
// raises on no-match, so the VM's dynamic apply must be able to tell "not a
// function" (leave the window as data — right) from "a function no overload of
// which admits these arguments" (raise — NUR107). `basic` keeps the historical
// spelling as a thin re-export; this is the one implementation.
//
// A value with no FnDefInfo payload — a fn-typed CARRIER, a closure — has no
// own signatures to consult and answers nil, so a caller must treat nil as "no
// opinion" unless it has already established that the value carries sigs.
func MatchFnSig(fn Value, args []Value) *FnSig {
	fnDef, ok := fn.Data.(FnDefInfo)
	if !ok {
		return nil
	}
	ownSigs := fnDef.OwnSigs()
	for i := range ownSigs {
		sig := &ownSigs[i]
		if len(sig.Params) != len(args) {
			continue
		}
		match := true
		for j, p := range sig.Params {
			// The matcher's own per-slot rule (stackSlotAdmits): a bare
			// type node's Parent is its SUPERTYPE, so the ConformsTo this
			// used to ask refused `Integer` at a `t:Type` slot the
			// interpreter's dispatch fills, and a type literal at a
			// concrete slot is refused by the rule, not by accident (NUR248).
			if !stackSlotAdmits(sig, j, args[j]) {
				match = false
				break
			}
			if p.Pattern != nil && !p.Pattern.Carrier {
				pat := *p.Pattern
				if pat.Parent.Equal(TMap) && args[j].Parent.Equal(TMap) &&
					pat.Data != nil && args[j].Data != nil {
					if !OpenUnifyMap(pat, args[j]) {
						match = false
						break
					}
				} else {
					if _, uOk := Unify(args[j], pat); !uOk {
						match = false
						break
					}
				}
			}
		}
		if match {
			return sig
		}
	}
	return nil
}
