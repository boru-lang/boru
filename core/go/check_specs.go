package core

// Check-piece compile-spec records (Stage 3c of the four-piece split,
// seam S2): the typed-bind, fallback-span and poly-no-match specs the
// CHECK piece proves, the compiler stores, and the VM replays. Homing
// them check-side lets both neighbours import them legally at the
// package cut.

// TypedBindKind selects which of defTypedHandler's refinement branches one
// OpBindTyped mirrors at run time. Explicit non-zero values (iota+1) so the
// struct zero value is an invalid kind that RunTypedBind rejects loudly, never
// a silently-valid predicate bind (No-Zero-Value-Overload, eng/go/CLAUDE.md).
type TypedBindKind uint8

const (
	// TypedBindPredicate: the constraint is a predicate TYPE (an fn body —
	// `def Positive fn [[n:Integer] [Boolean] [n gt 0]]`). Runtime runs
	// RunPredicate over the value; Def (when non-nil) is the minted type the
	// passing value reparents to (the interpreter reparents only for a NAMED
	// non-builtin predicate type with a concrete declared input).
	TypedBindPredicate TypedBindKind = iota + 1
	// TypedBindRefine: the constraint is a bare-refine NEWTYPE (`def Pos
	// (refine Integer)`). Runtime unifies the value against Def's nearest
	// builtin ancestor, then reparents to Def.
	TypedBindRefine
	// TypedBindDepScalar: the constraint is a DepScalar subset (`(Integer gt
	// 10)`, inline or named). Runtime unifies the value against the
	// self-contained Constraint; the value keeps its base tag (no reparent).
	TypedBindDepScalar
	// TypedBindRunMembership: a constraint whose membership only the run
	// knows — a type over a refinement whose bound the analysis pass did not
	// know (NUR308): a named node the run installed (RunTypeInstall forwards
	// it to the run's type) or, with ConsOperand, the inline constraint the
	// run computed — or a value only the run knows, a dynamic body whose
	// carrier does not prove it a member of the annotation (NUR290). Runtime
	// unifies the value against it; the value keeps its tag (a refinement,
	// union or negation constraint never reparents; an FnUndef annotation,
	// which does, never records one — only a fn is its member, and a body
	// that may hold a fn declines).
	TypedBindRunMembership
)

// TypedBindSpec describes one OpBindTyped: the typed-def name (for the error
// text), the rendered constraint (Describe — the annotation NAME when the
// constraint was named, else the constraint's rendering, mirroring
// defTypedHandler's describeType), the reparent target type, and the
// constraint VALUE for the kinds that validate against a self-contained value
// (the predicate fn / the DepScalar). Def rides as a *Type pointer — the same
// convention CompiledFn.Returns/Params use — resolved through NodeOf at run
// time, which needs no table, so a type minted in a module sub-registry
// (whose ID the main TypeTable does not know) still reaches its node.
type TypedBindSpec struct {
	Kind     TypedBindKind
	Name     string
	Describe string
	Def      *Type  // reparent target: TypedBindRefine always; TypedBindPredicate when the interpreter reparents; nil otherwise
	Cons     *Value // constraint value: TypedBindPredicate (the fn) and TypedBindDepScalar (the DepScalar); nil for TypedBindRefine
	// ConsOperand: the constraint is not baked — the run computed it, and it
	// sits on the stack beneath the value (TypedBindRunMembership over an
	// inline constraint, NUR308). An empty Describe renders the run's
	// constraint, as defTypedHandler's describeType renders an inline one.
	ConsOperand bool
}

// TypeRunInstallSpec describes one OpBindTypeRun (NUR308): the type name the
// run installs from the body it computed, and the node the analysis pass
// minted under that name — the node every compiled reference names, by ID or
// by *Type, which forwards to the run's (RunTypeInstall).
type TypeRunInstallSpec struct {
	Name string
	Node *Type
}
