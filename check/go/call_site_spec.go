package check

import (
	"strconv"

	core "github.com/boru-lang/boru/core/go"
)

// Call-site specialisation of user-fn units.
//
// A fn body compiles ONCE per generalised call shape: every arg becomes a
// carrier of its type (BuildFnBodyReturnsFn's genArgs), so one unit serves
// every caller. For a Function-typed param that generalisation discards the
// most useful fact the call site has — WHICH fn it passes. `Function` carries
// no signature (ADR-011, one function type), so a body read of the param
// (`g 2`, `fold [drop g] xs`) cannot know the callee's arity, its result
// count or its result types: it compiles to a run-time apply that re-matches
// the fn's signatures on every call, and everything downstream of the result
// re-matches too (CALL_NATIVE_POLY).
//
// When the call site passes a CONSTANT fn — a `/v` reference, a lambda
// literal — to a named Function param of a single-signature fn, the call
// records a SPECIALISED unit instead: the same body compiled with that param
// bound to the fn itself, so the body's reads dispatch the fn exactly as
// `inc 2` does when written directly. The specialised unit is valid only
// while the runtime arg IS that fn, so it carries a guard per specialised
// param (CompiledFn.SpecGuards): the VM checks fn identity (core.ExactEqual,
// what `eq` compares) at CALL_USER entry, before any of the body runs, and
// when a guard fails applies the fn itself to the args through the fn-value
// callback seam (CompiledFn.SpecFallback) — the dispatch the interpreter
// makes. Nothing has run when the guard decides, so there is nothing to
// deoptimise. No generic unit is compiled for a specialised call site: an
// unexecuted generic body is not free (a dynamic code body in it arms the
// program-wide dynamic environment, which every unit then pays for).
//
// A specialisation can only ever add a faster path to a program that already
// compiles, never cost it its compilation: a compile pass that tried one and
// failed is re-run once with specialisation off (CheckState.SpecOff, set by
// lang's CompileCheck), which records exactly what it recorded before. The
// retry is the containment — a failed specialised body cannot be unwound in
// place, because a failing unit leaves the recorder's unit stack mid-body.

// FnSpecQuota caps the call-site specialisations compiled per fn definition
// site: a recursion that passes a freshly constructed fn at every level would
// otherwise mint one specialised unit per level.
const FnSpecQuota = 4

// specialiseCallSite compiles the specialised unit for the constant fn args a
// call site passes and returns it, or -1 when the call site does not
// specialise (the caller compiles the generic unit). compileUnit is
// BuildFnBodyReturnsFn's own unit compile (StartFnCompile, the unit's
// contracts, the armed body analysis), so a specialised unit carries exactly
// the generic unit's param and return contracts. fnDef is the callee: its one
// signature is the fallback's. foreign reports that the callee's home is not
// the dispatching registry (core.FnHomeForeign).
func specialiseCallSite(r *core.Registry, es core.EmitRecorder, compileUnit func([]core.Value, string) int, fnDef core.FnDefInfo, foreign bool, name string, body []core.Value, params []core.FnParam, args, genArgs []core.Value) int {
	// A generic fn instantiates per call already (its type bindings key the
	// unit), a stored-handler compile generalises gradually on purpose, a
	// multi-signature fn's runtime pick is not one fallback signature, a
	// capturing fn's captures are per construction (they ride CALL_USER as
	// trailing slots, and the fallback's fn value would carry the check
	// pass's), and a fn whose home is not the dispatching registry — a
	// module's, resolved or inline — runs in a registry whose fn values are
	// minted per instance: the check pass's are not the run's, and a fallback
	// applied from the caller does not run where the module's body resolves.
	if r.Check.SpecOff || fnDef.Gen != nil || es.StoredGradualActive() || len(fnDef.OwnSigs()) != 1 || len(fnDef.Captured) > 0 || foreign {
		return -1
	}
	specArgs, specParams, specFns, suffix := specialisedArgs(r, params, args, genArgs)
	if specArgs == nil {
		return -1
	}
	if r.Check.FnSpecCounts == nil {
		r.Check.FnSpecCounts = map[string]int{}
	}
	site := fnQuotaKey(r.AnalysisScopeID(), name, body)
	seen := "seen:" + site + suffix
	if r.Check.FnSpecCounts[seen] == 0 {
		if r.Check.FnSpecCounts[site] >= FnSpecQuota {
			return -1
		}
		r.Check.FnSpecCounts[site]++
		// Marked before the compile: a recursive call inside the body being
		// specialised reaches here again with the same fn and must reuse the
		// in-flight unit, not count a second specialisation.
		r.Check.FnSpecCounts[seen] = 1
	}
	r.Check.SpecTried = true
	unit := compileUnit(specArgs, suffix)
	es.SetUnitSpecialisation(unit, specParams, specFns, core.NewFunction(fnDef))
	return unit
}

// specialisedArgs builds the specialised unit's arg vector: genArgs with each
// specialisable constant fn arg kept as that fn (a fresh value ID, so the
// unit's param slot does not alias the caller's operand), the guarded param
// indexes and fns, and the key suffix naming them. nil when no arg qualifies.
func specialisedArgs(r *core.Registry, params []core.FnParam, args, genArgs []core.Value) ([]core.Value, []int, []core.Value, string) {
	var specArgs []core.Value
	var specParams []int
	var specFns []core.Value
	suffix := ""
	for i := range min(len(params), len(args), len(genArgs)) {
		a := args[i]
		id, ok := specialisableFnArg(r, params[i], a)
		if !ok {
			continue
		}
		if specArgs == nil {
			specArgs = append([]core.Value(nil), genArgs...)
		}
		c := a
		c.ID = core.GenerateID(core.IDPrefixForType(c.Parent))
		specArgs[i] = c
		specParams = append(specParams, i)
		specFns = append(specFns, a)
		suffix += "!spec" + strconv.Itoa(i) + "=" + id
	}
	return specArgs, specParams, specFns, suffix
}

// specialisableFnArg reports whether arg a to param p warrants a
// specialisation, and a's identity key. The param must be a NAMED
// Function-typed one — the body reads it by name, and the declared type is
// what the generic unit loses. The arg must be a constant fn with an
// identity (the guard compares it) and nothing that makes it more than its
// signatures and body: no captures (their values are per construction), no
// generic spec, no macro splice, no modifier wrapper (both dispatch through
// tokens), and a home in the registry the specialised body runs in, r — a
// module export is re-minted per import instance.
func specialisableFnArg(r *core.Registry, p core.FnParam, a core.Value) (string, bool) {
	if p.Name == "" || p.Type == nil || !p.Type.Equal(core.TFunction) || p.Pattern != nil || a.Carrier {
		return "", false
	}
	fd, isFn := a.Data.(core.FnDefInfo)
	if !isFn || len(fd.Captured) > 0 || fd.Gen != nil || fd.Macro || fd.Wrap != core.WrapNone || core.FnHomeForeign(r, &fd) {
		return "", false
	}
	// A fn LITERAL a fn or closure unit writes is a new function each time
	// the unit runs it (the compiled lane's fresh push, NUR288), so a guard
	// on its identity never holds and the specialised unit would only ever
	// take its fallback: such an arg does not specialise. A `/v` reference
	// and a root literal keep one identity.
	if fd.Name == "" && freshLiteralContext(r) {
		return "", false
	}
	return core.FnIdentityKey(a)
}

// freshLiteralContext reports whether the pass stands inside a unit the
// compiled lane runs per call — a fn body's analysis or a closure unit —
// where a fn literal is pushed with a fresh identity (NUR288).
func freshLiteralContext(r *core.Registry) bool {
	return r.Check.FnBodyDepth > 0 || r.Check.Recorder().InClosureUnit()
}

// specResidualMeetsReturns reports whether a specialised body's residual can
// meet the unit's RET contract: one value per declared return, and no STRICT
// value outside its declared type. A specialised body knows what the generic
// one does not — the passed fn's result count and types — so a body it shows
// returning the wrong count (`x f/v apply f/v apply` over a two-result fn) is
// a return error the specialised unit would raise at its own RET, where the
// generic unit's run-time apply raises it as the interpreter does, at the
// call. Such a specialisation declines; the retry compiles the call site as
// before. A gradual (dynamic) value is only a bound, never a mismatch.
func specResidualMeetsReturns(residual []core.Value, returns []*core.Type) bool {
	if len(returns) == 0 {
		return true
	}
	if len(residual) != len(returns) {
		return false
	}
	for i, v := range residual {
		t := returns[i]
		if v.Dynamic || v.Parent == nil || t == nil || v.Parent.ConformsTo(t) {
			continue
		}
		return false
	}
	return true
}

// specParamCallMayRefuse reports whether a call dispatched THROUGH a
// specialised constant-fn param could fail the callee's param contract at run
// time: an argument with no type, one whose type — or, for a gradual value,
// whose bound — lies outside the param's declared type (an `Any` among
// them), or a patterned param. The specialised unit would raise that refusal
// as CALL_USER's contract check — the compiled unit's no-match notes (the
// values) — where the generic unit's run-time apply raises the interpreter's
// own (the written tuple, NUR172's class), so such a call declines the
// specialisation and the retry compiles the call site as before. A gradual
// value inside its param's type is refused only where its bound was wrong,
// the exposure every compiled named call's contract check already carries.
func specParamCallMayRefuse(params []core.FnParam, args []core.Value) bool {
	for i := range min(len(params), len(args)) {
		a, p := args[i], params[i]
		if p.Pattern != nil || a.Parent == nil || (p.Type != nil && !a.Parent.ConformsTo(p.Type)) {
			return true
		}
	}
	return false
}
