package compiler

import (
	check "github.com/boru-lang/boru/check/go"
	core "github.com/boru-lang/boru/core/go"
)

// Compiler-piece dispatch planning extracted from core_helpers.go (Stage 2c
// of the four-piece split): the user-poly lowering decision for hazardous
// multi-overload call sites (its bake lives in user_poly.go).

// planUserPolyDispatch decides whether a multi-overload user-fn call site
// must ride the user-poly RUNTIME re-match instead of a static arm commit,
// and returns the plan plus whether the static unit path is barred. Two
// hazard classes:
//
//   - Cluster C (broad miscompile hunt): a gradual-Any arg makes the
//     dispatch AMBIGUOUS — the checker's committed overload's CALL_USER
//     param guard raises at runtime when the value matches a SIBLING arm the
//     interpreter re-matches to (`def g fn [[a:Integer]['i'] [a:String]['s']]
//     (g (id 5))` returned 'i' interpreted but signature_error compiled).
//   - An fn-PREDICATE-typed param slot with more than one reachable arm
//     (fnPredicateOverloadHazard): check-mode sigTypeMatches accepts
//     predicate types LENIENTLY (RunPredicate short-circuits true —
//     registry.go), so the static commit may be an arm the interpreter's
//     runtime predicate run rejects before falling through to a sibling —
//     `classify -3` over [x:Pos]/[x:Any] arms baked the Pos arm and raised
//     where the interpreter returns the Any arm's value (the 2026-07-15
//     flip's dispatch twin of the fn-predicate BIND finding, f8a5bba).
//
// Either way the sound lowering is OpCallUserPoly — bake EVERY same-arity
// overload's body unit and let the VM re-run MatchSignature at entry (the
// real predicate runs there) — or, when the poly bake declines any arm, the
// hazard's compile failure, byte-identical to the pre-poly taxonomy. A
// single-overload predicate fn is NOT barred: its CALL_USER param guard
// re-validates at entry and raises exactly the interpreter's no-match error.
func planUserPolyDispatch(r *core.Registry, es core.EmitRecorder, word string, args []core.Value, declaredReturns []*core.Type) (*userPolyPlan, bool) {
	// A gradual-Any arg OR a strict Any carrier (an Any param's generalised
	// arg — a wrapper forwarding a value through an `Any` param) both make the
	// dispatch runtime-dynamic: the committed Any-slot arm is not a proof, the
	// interpreter re-matches on the true value, and dynamicReachableOverloadCount
	// counts the strict-Any arg as reaching every arm.
	clusterC := es.Active() && (check.AnyDynamicCarrier(args) || check.AnyAnyCarrier(args)) &&
		check.DynamicReachableOverloadCount(r, word, args) >= 2
	predHazard := !clusterC && es.Active() && check.FnPredicateOverloadHazard(r, word, args)
	if !clusterC && !predHazard {
		return nil, false
	}
	plan := tryCompileUserPolyArms(r, es, word, args, declaredReturns)
	if plan == nil {
		if clusterC {
			es.MarkUncompilable("gradual-Any arg to multi-overload user fn `" + word + "`: ambiguous dispatch, no poly re-match")
		} else {
			es.MarkUncompilable("fn-predicate-typed overload dispatch at `" + word + "` is runtime-evaluated (no poly re-match)")
		}
	}
	return plan, true
}
