package core

// The run-time type install — NUR231's type half.
//
// A type whose content holds a refinement over a bound the analysis pass
// does not know (`def T (Integer gte (size s))`, `def T ((Integer gt n) tor
// String)`) cannot be replayed: the pass minted T over its PLACEHOLDER for
// the bound — the carrier — and a bind twin re-installs that placeholder. So
// the compiled lane installs it as the interpreter does: at the def's
// position the run hands the body it computed to InstallType, the
// interpreter's own front door, which mints — or aliases, for an empty
// interval the run computed — exactly as the interpreter's `def` does.
//
// Every compiled reference, though, names the node the PASS minted: a type
// operand by ID, a signature or a typed-bind spec by *Type. That node
// FORWARDS to the run's: its membership and unification are the run's
// node's (forwardingBehavior), and a type operand pushes the run's node
// itself (ForwardedType), so identity — `T eq Never` over an empty interval
// the run computed — is the interpreter's too.

// HasUnknownRefinement reports whether a type's content holds a refinement
// whose bound the analysis pass does not know (NUR231) — a refinement, a
// union or negation holding one, reached through a named node's recorded
// body, or a typed container's child. Such a type's membership is the run's
// to decide: its install and every typed bind against it happen at run time.
func HasUnknownRefinement(v Value) bool {
	return hasUnknownRefinement(v, 0)
}

func hasUnknownRefinement(v Value, depth int) bool {
	if depth > 32 {
		return false
	}
	if IsBareTypeNode(v) {
		if body, ok := v.TypeBody(); ok {
			return hasUnknownRefinement(body, depth+1)
		}
		return false
	}
	switch d := v.Data.(type) {
	case DepScalarInfo:
		return !depBoundConst(d.Lo) || !depBoundConst(d.Hi)
	case DisjunctInfo:
		for _, alt := range d.Alternatives {
			if hasUnknownRefinement(alt, depth+1) {
				return true
			}
		}
	case NegationInfo:
		return hasUnknownRefinement(d.Inner, depth+1)
	case ChildTypeInfo:
		return hasUnknownRefinement(d.Child, depth+1)
	}
	return false
}

// RunTypeInstall performs one OpBindTypeRun: the interpreter's own type
// install of name over the body the run computed, then the analysis pass's
// node forwarded to the node that install bound (a fresh mint, or an
// adopted alias — Never for an empty interval). The bind twin of the same
// def is written back (it replays nothing), so this is the one install.
func RunTypeInstall(r *Registry, spec *TypeRunInstallSpec, body Value) error {
	if err := InstallType(r, spec.Name, body); err != nil {
		return err
	}
	// InstallType's success pushed the binding it installed; a spec without
	// the pass's node (the lowering builds none) has nothing to forward.
	if entry, ok := r.Defs.TopEntry(spec.Name); ok && entry.TypeDef != nil && spec.Node != nil {
		forwardType(CanonicalType(r, spec.Node), entry.TypeDef)
	}
	return nil
}

// forwardType forwards the pass's node to the run's. A node forwarded to
// itself (the run re-installed the very node) is left alone.
func forwardType(node, target *Type) {
	if node == target || node.ID == target.ID {
		return
	}
	m := node.ensureTMeta()
	m.RunForward = target
	m.Behavior = forwardingBehavior{target: target}
}

// ForwardedType follows a pass-minted node's forward to the node the run
// installed (RunTypeInstall), or returns t unchanged.
func ForwardedType(t *Type) *Type {
	for i := 0; t != nil && i < 8; i++ {
		if t.tmeta == nil || t.tmeta.RunForward == nil {
			return t
		}
		t = t.tmeta.RunForward
	}
	return t
}

// forwardingBehavior is the behaviour a pass-minted node takes once the run
// has installed its type: membership, unification, rendering and equality
// are the run's node's. It decides membership by content — its target's.
type forwardingBehavior struct{ target *Type }

func (forwardingBehavior) ContentMembership() {}

func (f forwardingBehavior) Match(v Value, _ *Type) bool { return v.Is(f.target) }

func (f forwardingBehavior) Format(v Value) string {
	return baseBehavior(f.target.Behavior()).Format(v)
}

func (f forwardingBehavior) Equal(a, b Value) bool {
	return baseBehavior(f.target.Behavior()).Equal(a, b)
}

// Unify re-asks the question with the forwarded operand replaced by the
// run's node.
func (f forwardingBehavior) Unify(a, b Value, r *Registry) (Value, *UnifyError) {
	return unifyWithin(forwardedOperand(a), forwardedOperand(b), r)
}

// forwardedOperand is v with a forwarded node replaced by the run's node.
func forwardedOperand(v Value) Value {
	if !IsBareTypeNode(v) {
		return v
	}
	if t := ForwardedType(&v); t != &v {
		return NewTypeLiteral(t)
	}
	return v
}
