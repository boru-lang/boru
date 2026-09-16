package core

// NoteSpecFnDef offers the recorder a fn def made inside a branch arm the
// model cannot decide (CheckState.SpecArmDepth) — fresh (a zero outer), or
// replacing the overlapping overload outer in place (family L) — and, if
// the recorder places it (EmitRecorder.RecordSpeculativeFnDef), marks the
// family SPECULATIVE (SpecFnNames): the seventieth increment. Such a def
// binds at run time exactly when the arm runs: a fresh def leaves the name
// bound or unbound, a replace leaves the outer overload or the shadow —
// and the check pass's model, which keeps the fn for typing, cannot say
// which. So the binding's dispatches route with a live lead (the routed
// op resolves the word in the running registry and raises the
// interpreter's undefined_word on a miss), the join notes no root twin for
// the arm's install (InstallJoinedDefs), and the install is placed at its
// site. Declined — a recorder that cannot place it, or none — the family
// keeps the model it had: the join's, and installDef's own refusal for a
// replace. Nothing for a nil registry or an empty name.
func NoteSpecFnDef(r *Registry, name string, outer, fn Value, pos SrcPos) bool {
	if r == nil || r.Check == nil || name == "" {
		return false
	}
	if !r.analysisRecorder().RecordSpeculativeFnDef(name, outer, fn, pos) {
		return false
	}
	if r.Check.SpecFnNames == nil {
		r.Check.SpecFnNames = map[string]bool{}
	}
	r.Check.SpecFnNames[name] = true
	return true
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
