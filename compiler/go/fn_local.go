package compiler

import core "github.com/boru-lang/boru/core/go"

// NUR037's fn-local fn (the seventy-second increment): a code body — a `do`
// body, an `each` body, a `for-each` step — naming a fn the ENCLOSING fn's
// body defined (`def g fn [[][Integer][def f fn [[x:Integer][Integer][x add
// 1]] end  do [f 5]]]`) refused the whole program, because every path the
// body could take baked the NAME: the closure unit's dispatch, the island's
// re-run and the const-baked list all resolve `f` in the VM's registry,
// which never held the enclosing unit's local (its `def f fn …` compiled
// away — the unit reaches f by index). The lookup half binds it instead:
// the def's own event is stamped as a placed install (the seventieth
// increment's specFn lowering — `PUSH_CONST fn; BIND_DYN_SCOPE f` inside a
// unit, torn down at the frame's RET as the interpreter's def-cleanup pops
// it), so the body — compiled, islanded or interpreted — finds `f` where
// the interpreter does. A capturing local fn keeps the refusal: its value is
// a closure the placement cannot bake (the seventieth's own limit), and a
// def the current unit's frames do not hold keeps it too.

// placeFnLocalDef stamps the innermost open unit's recorded def of name —
// a capture-free fn with declared signatures — as a placed install, so the
// binding is registry-visible for the frame. Reports whether it did.
func (es *EmitState) placeFnLocalDef(name string, v core.Value) bool {
	if es == nil || !es.Active() || name == "" || len(es.openUnitRecs) == 0 || !fnSigsDeclared(v) {
		return false
	}
	if fd, ok := v.Data.(core.FnDefInfo); !ok || len(fd.Captured) > 0 {
		return false
	}
	for fi := len(es.frames) - 1; fi >= 0; fi-- {
		frame := es.frames[fi]
		for ei := len(frame) - 1; ei >= 0; ei-- {
			ev := &frame[ei]
			if ev.kind != evDynBind || ev.dyn == nil || ev.dyn.name != name || ev.dyn.root {
				continue
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
