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
// keeps the model it had: the join's, and installDef's own compile failure for a
// replace. Nothing for a nil registry or an empty name.
func NoteSpecFnDef(r *Registry, name string, outer, fn Value, pos SrcPos) bool {
	if r == nil || r.Check == nil || name == "" {
		return false
	}
	if !r.analysisRecorder().RecordSpeculativeFnDef(r, name, outer, fn, pos) {
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

// specFamilyAtFnBaseline reports whether name's standing binding — the one an
// in-fn-body redefinition would drop — existed at the enclosing fn's baseline,
// i.e. a MODULE-scope (or outer-fn) family whose in-place replacement leaks
// past the call (family L, NUR149). An IN-FUNCTION family, created inside this
// fn above the baseline, is torn down by the frame's RET and compiles soundly,
// so it is NOT one (Codex P2 on #469). Uses the ComputeCaptures depth rule:
// Depth(name) <= baseline[name] means the binding predates the fn. Off any
// enclosing-fn baseline (a nil map) nothing is baseline-scoped.
func specFamilyAtFnBaseline(r *Registry, name string) bool {
	if r == nil || r.Defs == nil {
		return false
	}
	base := r.TopFnBaseline()
	return base != nil && r.Defs.Depth(name) <= base[name]
}

// specFamilyJoinModel is the binding a join pushes for a speculative fn
// family BOTH arms define (NUR245). The family's dispatches route with a
// live lead — the running registry's binding picks the body, and the op
// enters the unit compiled for that binding's own signature — so the
// model is only what the pass types the call against and compiles the
// call site's unit from.
//
// Under a DECIDED condition it is the running arm's fn, whatever the other
// arm declares: the skipped arm never binds. Undecided, it must stand for
// either arm. Arms that agree on every shape the call's record fixes
// (SameSigShapes) share the then arm's fn. Arms that agree only on what the
// routed op's CLAIM fixes — arity, barrier, quoting, patterns and the
// return count (claimCompatibleSigs) — share a WIDENED model
// (widenedFamilyModel): each parameter and return type is the two arms'
// join, so the pass types the call against a slot either arm's value
// fits, and the routed op's live plan decides the match and raises the
// interpreter's no-match. A pair that differs on the claim keeps the
// payload-less join, and its call's standing failure.
func specFamilyJoinModel(then, else_ Value, runs armRuns) (Value, bool) {
	a, aFn := then.Data.(FnDefInfo)
	b, bFn := else_.Data.(FnDefInfo)
	switch {
	case !aFn || !bFn:
		return Value{}, false
	case runs == elseArmRuns:
		return else_, true
	case runs == thenArmRuns || SameSigShapes(a.OwnSigs(), b.OwnSigs()):
		return then, true
	}
	return widenedFamilyModel(then, &a, &b)
}

// widenedFamilyModel is the then arm's fn with each own signature's
// parameter and return types joined with the else arm's (CommonAncestorType)
// and NO declaration site: the routed op locates a unit by the LIVE
// signature's site (eng: specFnUnit), so the unit a call site compiles from
// this model — under types neither arm declares, with a return contract
// neither enforces — is never entered; each arm's own is compiled where it
// is placed. false when the arms differ on the claim (claimCompatibleSigs).
func widenedFamilyModel(then Value, a, b *FnDefInfo) (Value, bool) {
	as, bs := a.OwnSigs(), b.OwnSigs()
	if !claimCompatibleSigs(as, bs) {
		return Value{}, false
	}
	fd := *a
	fd.Signatures = make([]Signature, 0, len(a.Signatures))
	own := 0
	for i := range a.Signatures {
		s := a.Signatures[i]
		if !s.Fallback {
			s = widenedSig(s, &bs[own])
			own++
		}
		fd.Signatures = append(fd.Signatures, s)
	}
	model := then
	model.Data = fd
	return model, true
}

// widenedSig is a with every parameter and return type joined with b's at
// the same position, and no declaration site. The parameter types are read
// through SigArgType, whichever of Params and the legacy Args each side
// stores them in (claimCompatible fixed the arity), and written back to
// whichever a stores.
func widenedSig(a Signature, b *Signature) Signature {
	joined := make([]*Type, a.TotalArgs())
	for i := range joined {
		joined[i] = CommonAncestorType(SigArgType(&a, i), SigArgType(b, i))
	}
	if len(a.Params) > 0 {
		ps := append([]FnParam(nil), a.Params...)
		for i := range ps {
			ps[i].Type = joined[i]
		}
		a.Params = ps
	}
	if len(a.Args) == len(joined) {
		a.Args = joined
	}
	a.Returns = joinedTypes(a.Returns, b.Returns)
	a.Decl = DeclSite{}
	return a
}

// joinedTypes joins two equally long type lists position by position.
func joinedTypes(a, b []*Type) []*Type {
	out := make([]*Type, len(a))
	for i := range a {
		out[i] = CommonAncestorType(a[i], b[i])
	}
	return out
}

// SameSigShapes reports whether two own-signature lists agree, position by
// position, on every shape sameSigShape compares: the pair a joined family
// shares one fn for (specFamilyJoinModel). The recorder asks it too — a
// family placed a second time with a differing shape compiles its first
// placement's units, which no call site will (NUR245).
func SameSigShapes(a, b []Signature) bool {
	same := len(a) == len(b)
	for i := 0; same && i < len(a); i++ {
		same = sameSigShape(&a[i], &b[i])
	}
	return same
}

// sameSigShape reports whether two signatures claim the same call: the
// same arity and barrier, per position the same declared type, pattern and
// quoting, and the same declared returns with no return pattern on either.
func sameSigShape(a, b *Signature) bool {
	same := claimCompatible(a, b) && sameTypes(a.Returns, b.Returns)
	for i := 0; same && i < a.TotalArgs(); i++ {
		same = SigArgType(a, i).Equal(SigArgType(b, i))
	}
	return same
}

// claimCompatibleSigs reports whether two own-signature lists agree,
// position by position, on what the routed op's claim fixes
// (claimCompatible).
func claimCompatibleSigs(a, b []Signature) bool {
	same := len(a) == len(b)
	for i := 0; same && i < len(a); i++ {
		same = claimCompatible(&a[i], &b[i])
	}
	return same
}

// claimCompatible reports whether two signatures make the same CLAIM on a
// call — the same arity and barrier, per position the same pattern,
// quoting and type-slot marking, and the same return count with no return
// pattern on either — whatever types they declare: the routed op's live
// plan must claim the forward and total counts the record did, and its
// unit must leave the count the call site seats.
func claimCompatible(a, b *Signature) bool {
	same := a.TotalArgs() == b.TotalArgs() && a.BarrierPos == b.BarrierPos &&
		len(a.Returns) == len(b.Returns) && len(a.ReturnPatterns) == 0 && len(b.ReturnPatterns) == 0
	for i := 0; same && i < a.TotalArgs(); i++ {
		pa, hasA := SigPattern(a, i)
		pb, hasB := SigPattern(b, i)
		same = hasA == hasB && (!hasA || ExactEqual(pa, pb)) && a.QuoteArgs[i] == b.QuoteArgs[i] && a.TypeArgs[i] == b.TypeArgs[i]
	}
	return same
}

// sameTypes reports whether two type lists are equal position by position.
func sameTypes(a, b []*Type) bool {
	same := len(a) == len(b)
	for i := 0; same && i < len(a); i++ {
		same = a[i].Equal(b[i])
	}
	return same
}
