package core

// NoteSpecFnDef marks name a SPECULATIVE fn family (CheckState.SpecFnNames)
// and hands the recorder its placed install — the seventieth increment. A
// fn def inside a rolled-back CONDITIONAL body binds at run time exactly
// when the arm runs: a fresh def leaves the name bound or unbound, an
// overlapping redefinition (installDef's same-scope filter, family L)
// leaves the outer overload or the shadow — and the check pass's model,
// which keeps the fn for typing, cannot say which. So the binding's
// dispatches route with a live lead (the routed op resolves the word in
// the running registry and raises the interpreter's undefined_word on a
// miss), the join notes no root twin for the arm's install
// (InstallJoinedDefs), and the install itself is placed at its site by the
// recorder (EmitRecorder.RecordSpeculativeFnDef, which refuses what it
// cannot place). outer is the overlap case's DROPPED entry — the outer
// overload the arm's run replaces: the placed install goes through the
// interpreter's own installer, which drops it as the arm's run does, and
// the recorder compiles its body to a unit of its own so the routed op can
// run whichever fn is live (CompiledFn.BodyPos). A fresh def passes a zero
// Value.
func NoteSpecFnDef(r *Registry, name string, outer Value, pos SrcPos) {
	if r == nil || r.Check == nil || name == "" {
		return
	}
	if r.Check.SpecFnNames == nil {
		r.Check.SpecFnNames = map[string]bool{}
	}
	r.Check.SpecFnNames[name] = true
	r.analysisRecorder().RecordSpeculativeFnDef(name, outer, pos)
}

// specFnJoin reports whether k is a speculative fn family: the branch join
// pushes the model's binding for it and notes NO transition — the arm's
// install is placed at its own site, and a root twin replayed before the
// branch bound the name whether or not the arm ran (measured as
// `def m {e: false}  if (m "e" get) [def f fn […]] []  f 1` answering the
// arm's 101 where the interpreter raises undefined_word).
func specFnJoin(r *Registry, k string) bool {
	return r != nil && r.Check != nil && r.Check.SpecFnNames[k]
}
