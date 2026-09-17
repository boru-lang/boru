package compiler

import (
	check "github.com/boru-lang/boru/check/go"
	core "github.com/boru-lang/boru/core/go"
)

// Typed code values — stage 1 of design/legacy/checker-precision-fronts.0.ignore §1
// (the `do` escape hatch). A QUOTED code list stored in data (a dispatch
// table, a rule list, a callback slot) is statically described by its
// STACK EFFECT — Code[in → out]. The effect is computed by the same
// check-mode body analysis the closure path runs (RunCarrierBody),
// while the concrete tokens are still in hand at the element-READ site,
// and attached to the list CARRIER the checker holds from then on — so
// a downstream `do` over the non-literal carrier can type its result
// instead of degrading to dynamic(Any).
//
// Gradual contract: the effect is a best static BOUND, not a proof.
// The body's free words re-resolve against the runtime environment at
// `do` time, which may have drifted since the read site (`def add …`
// between the read and the do). Consumers therefore surface the effect
// as DYNAMIC carriers bounded by Out — dynamic(Integer), never strict
// Integer — so matching stays optimistic, environment drift can never
// manufacture a false positive, and a code value the analysis cannot
// type stays dynamic(Any) exactly as before.
//
// Compile-pass discipline: checker precision must NOT imply compile
// coverage. Both the producer (AnalyseCodeEffectCarrier) and the
// consumer (`do`'s ReturnsFn) are gated to !Check.Compiling, so a real
// compile pass (CompileCheck / RunCompiled) sees the pre-existing
// dynamic(Any) hatch unchanged and a `do` over a computed body carrier
// keeps REFUSING to lower (whole-program interpreter fallback) —
// lang/go/code_effect_test.go pins the refusal.

// AnalyseCodeEffectCarrier converts a CONCRETE quoted code list — a
// stored body, recognised by containing at least one Word token — into
// a TList carrier carrying its analysed core.CodeEffectInfo. ok=false means
// the effect could not be computed soundly and the caller must keep
// its existing conversion (a plain type carrier), so precision only
// ever increases; a code value the analysis cannot type stays
// dynamic(Any) downstream, never a guess.
//
// Declines (conservative decay, in order):
//   - outside plain check mode, or during a COMPILE pass (Compiling)
//     — compiled lowering must be byte-identical to today's;
//   - nested inside another code-effect analysis (a stored body that
//     itself reads stored code — `quote [ops get 0 do]` — would recurse
//     through the element-read producer unboundedly; one level is the
//     stage-1 precision);
//   - a non-concrete / non-plain-list / word-free value (pure data
//     lists already type through the container paths);
//   - a body carrying a flow-control sentinel (break/continue/return
//     target an ENCLOSING frame, not the body's own effect);
//   - an analysis that emits diagnostics or leaves an empty residual
//     (a body needing inputs, an undefined word, a diverging raise —
//     the do hatch keeps its dynamic(Any)/caught-Error modelling);
//   - a residual the []*Type domain cannot represent faithfully (a
//     bare type literal's denotation, a structured disjunct).
//
// The analysis is a DRY pass: a read site describes the body, it does
// not run it, so any diagnostics it raises are truncated — at runtime
// `do` (the error-catching word) surfaces a failing body as a caught
// Error VALUE, not a program error.
func AnalyseCodeEffectCarrier(r *core.Registry, body core.Value) (core.Value, bool) {
	if r == nil || !r.Check.IsActive() || r.Check.Compiling || r.Check.CodeEffectDepth > 0 {
		return core.Value{}, false
	}
	if !core.IsConcrete(body) || !body.Parent.ConformsTo(core.TList) {
		return core.Value{}, false
	}
	lp, ok := body.Data.(core.ListPayload)
	if !ok || len(lp.Elems) == 0 {
		return core.Value{}, false
	}
	hasWord := false
	for _, e := range lp.Elems {
		if core.IsWord(e) {
			hasWord = true
			break
		}
	}
	if !hasWord || check.BodyHasSentinel(body) {
		return core.Value{}, false
	}

	r.Check.CodeEffectDepth++
	diagBase := len(r.Check.Diagnostics)
	stk := core.RunCarrierBody(r, body)
	// Error- or warning-severity findings mean the analysis did not
	// describe the body cleanly (a failing dispatch, a partial one) —
	// decline. INFO advisories (a forward-strand note, an analysis-
	// truncation) do not invalidate the residual. Either way the dry
	// pass truncates everything it raised: a read site describes the
	// body, it does not run it.
	clean := true
	for _, d := range r.Check.Diagnostics[diagBase:] {
		if d.Severity != core.SeverityInfo {
			clean = false
			break
		}
	}
	r.Check.TruncateDiagnostics(diagBase)
	r.Check.CodeEffectDepth--

	if !clean || len(stk) == 0 {
		return core.Value{}, false
	}
	out := make([]*core.Type, len(stk))
	for i, v := range stk {
		if v.Parent == nil || core.IsBareTypeNode(v) || core.IsDisjunct(v) {
			return core.Value{}, false
		}
		out[i] = v.Parent
	}
	c := core.NewCarrier(core.TList)
	c.Data = core.CodeEffectInfo{Out: out, Analysed: true}
	return c, true
}
