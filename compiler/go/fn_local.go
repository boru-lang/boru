package compiler

import core "github.com/boru-lang/boru/core/go"

// NUR037's fn-local fn (the seventy-second increment): a code body — a `do`
// body, an `each` body, a `for-each` step — naming a fn the ENCLOSING fn's
// body defined (`def g fn [[][Integer][def f fn [[x:Integer][Integer][x add
// 1]] end  do [f 5]]]`) declined the whole program, because every path the
// body could take baked the NAME: the closure unit's dispatch, the island's
// re-run and the const-baked list all resolve `f` in the VM's registry,
// which never held the enclosing unit's local (its `def f fn …` compiled
// away — the unit reaches f by index). The lookup half binds it instead:
// the def's own event is stamped as a placed install (the seventieth
// increment's specFn lowering — `PUSH_CONST fn; BIND_DYN_SCOPE f` inside a
// unit, torn down at the frame's RET as the interpreter's def-cleanup pops
// it), so the body — compiled, islanded or interpreted — finds `f` where
// the interpreter does. A capturing local fn keeps the compile failure: its value is
// a closure the placement cannot bake (the seventieth's own limit), and a
// def the current unit's frames do not hold keeps it too.

// placeFnLocalDef stamps the innermost open unit's recorded def of name —
// a capture-free fn with declared signatures, the CURRENT binding v — as a
// placed install, so the binding is registry-visible for the frame.
// Reports whether it did. The unit's latest def event of the name must be
// v's own (sameFnDecls): a current binding no event of the unit made — a
// closed body's redefinition, `do [def f …]` before an islanded read
// (review of #468) — has no install to place, and stamping the stale
// event would install the ORIGINAL where the interpreter runs the
// redefinition; it declines.
func (es *EmitState) placeFnLocalDef(name string, v core.Value) bool {
	if es == nil || !es.Active() || name == "" || len(es.openUnitRecs) == 0 {
		return false
	}
	if fd, ok := v.Data.(core.FnDefInfo); !ok || len(fd.Captured) > 0 {
		return false
	}
	if es.placeFnParam(name, v) {
		return true
	}
	if !fnSigsDeclared(v) {
		return false
	}
	for fi := len(es.frames) - 1; fi >= 0; fi-- {
		frame := es.frames[fi]
		for ei := len(frame) - 1; ei >= 0; ei-- {
			ev := &frame[ei]
			if ev.kind != evDynBind || ev.dyn == nil || ev.dyn.name != name || ev.dyn.root {
				continue
			}
			if !sameFnDecls(ev.dyn.val, v) {
				return false
			}
			ev.dyn.specFn = true
			return true
		}
	}
	// No def event of the name in the unit's frames: a name the seventieth
	// increment placed already (defined inside an arm the model cannot
	// decide) counts as placed, and the family's dispatches route. The
	// frames are searched FIRST: a body-local def of a name that is ALSO a
	// speculative family's at module scope must be placed itself, or the
	// body's routed dispatch resolves the module binding where the
	// interpreter resolves the local.
	return es.specFnNames[name]
}

// sameFnDecls reports whether two fn values are the same declaration: the
// same signature count and, position by position, the same declaration
// site (Signature.Decl — the output-sig token of the triple plus the
// declaring source and file, unique per signature; empty for a lambda or
// a Go-registered fn, which never matches).
func sameFnDecls(a, b core.Value) bool {
	fa, oka := a.Data.(core.FnDefInfo)
	fb, okb := b.Data.(core.FnDefInfo)
	if !oka || !okb || len(fa.Signatures) != len(fb.Signatures) {
		return false
	}
	for i := range fa.Signatures {
		if fa.Signatures[i].Decl == (core.DeclSite{}) || fa.Signatures[i].Decl != fb.Signatures[i].Decl {
			return false
		}
	}
	return true
}

// placeFnParam places a fn-valued PARAM an open unit binds as a constant fn —
// a call-site specialised unit's (check's specialiseCallSite): a code body
// naming it (`fold [drop g] xs` with `g` specialised to `inc`) declined
// exactly as a body-local fn def did, for the same reason — the body's paths
// resolve the NAME, and a unit's params live in slots. The placement is the
// param twin of the def's: the unit binds the param registry-visible at
// entry (emitDynParamBinds, the interpreter's own InstallFrameBinding of
// every named param), torn down at its RET, so the name resolves to the
// slot's runtime value — the guarded fn itself — wherever the body looks it
// up. v is the binding the body sees, and it must be that unit's param: the
// param named name whose compiled-against value is v's fn (core.ExactEqual —
// fn identity, which the interpreter-faithful re-install under the param's
// name keeps).
func (es *EmitState) placeFnParam(name string, v core.Value) bool {
	for k := len(es.openUnitRecs) - 1; k >= 0; k-- {
		rec := es.fnRecs[es.openUnitRecs[k]]
		for i := range rec.nParams {
			if rec.locals[i] != name || !core.ExactEqual(rec.paramVals[i], v) {
				continue
			}
			if rec.placedParams == nil {
				rec.placedParams = map[string]bool{}
			}
			rec.placedParams[name] = true
			return true
		}
	}
	return false
}
