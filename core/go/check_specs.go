package core

// Check-piece compile-spec records (Stage 3c of the four-piece split,
// seam S2): the typed-bind, fallback-span and poly-no-match specs the
// CHECK piece proves, the compiler stores, and the VM replays. Homing
// them check-side lets both neighbours import them legally at the
// package cut.

// PolyNoMatchSpec carries the tape-derived pieces of the interpreter's
// unmatched-dispatch diagnostic (sigError) that the VM cannot see at run
// time, resolved at record time into operand-window indices. sigError reads
// the tape twice — the WRITTEN tuple its notes render (unclaimed concrete
// forward tokens in source order, else the stack prefix top-first) and the
// SECONDARY reorder-probe tuple (always the stack prefix) — and both were
// proven at record time to be exactly window values, position for position
// (Value.ID equality, the RecordDispatchRematchValues gate). Everything else
// in the diagnostic (the candidate verdicts, the value-based reorder probe,
// the suggestions) is a pure function of the word, its live signature table,
// and these tuples (noMatchDiag / reorderHintFor).
type PolyNoMatchSpec struct {
	// Written / StackTuple index the runtime operand window (window[i] = sig
	// position i, the callPoly layout) to rebuild the two tape tuples.
	Written    []int
	StackTuple []int
	// NSigs pins the record-time signature-table length: a table that grew or
	// shrank between the record and the run invalidates the record-time arity
	// screen (an other-arity overload could now match where this raise claims
	// nothing can), so the VM falls back to the defer.
	NSigs int
	// Pos is the word's source position — the interpreter's raise anchor.
	Pos SrcPos
	// Uncalled marks the dispatch as a named fn VALUE's (execFnDefLiteral's
	// no-signature recovery for a module native, Engine.fnValueRecovery):
	// the interpreter's no-match there raises uncalled_function ("call to
	// 'w' matched no signature") at Pos, not sigError's signature_error, so
	// the VM raises that instead. Written / StackTuple are unused for it.
	Uncalled bool
}

// FallbackSpan is one interpreter island: a recorded token sequence
// the VM re-runs through a sub-engine for a construct the compiler
// could not lower (Stage 5 FALLBACK_INTERP). NIn operand-stack values
// are popped and pre-loaded onto the island (deepest first), the
// island runs Tokens, and its residual is pushed back. Desc is a
// human label for the disassembler.
type FallbackSpan struct {
	Tokens []Value
	NIn    int
	Desc   string
}

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
	// know (NUR231): a named node the run installed (RunTypeInstall forwards
	// it to the run's type) or, with ConsOperand, the inline constraint the
	// run computed. Runtime unifies the value against it; the value keeps its
	// tag (a refinement, union or negation constraint never reparents).
	TypedBindRunMembership
)

// TypedBindSpec describes one OpBindTyped: the typed-def name (for the error
// text), the rendered constraint (Describe — the annotation NAME when the
// constraint was named, else the constraint's rendering, mirroring
// defTypedHandler's describeType), the reparent target type, and the
// constraint VALUE for the kinds that validate against a self-contained value
// (the predicate fn / the DepScalar). Def rides as a *Type pointer — the same
// convention CompiledFn.Returns/Params use — resolved through CanonicalType at
// run time, so a type minted in a module sub-registry (whose ID the main
// TypeTable does not know) still reaches its live canonical node.
type TypedBindSpec struct {
	Kind     TypedBindKind
	Name     string
	Describe string
	Def      *Type  // reparent target: TypedBindRefine always; TypedBindPredicate when the interpreter reparents; nil otherwise
	Cons     *Value // constraint value: TypedBindPredicate (the fn) and TypedBindDepScalar (the DepScalar); nil for TypedBindRefine
	// ConsOperand: the constraint is not baked — the run computed it, and it
	// sits on the stack beneath the value (TypedBindRunMembership over an
	// inline constraint, NUR231). An empty Describe renders the run's
	// constraint, as defTypedHandler's describeType renders an inline one.
	ConsOperand bool
}

// TypeRunInstallSpec describes one OpBindTypeRun (NUR231): the type name the
// run installs from the body it computed, and the node the analysis pass
// minted under that name — the node every compiled reference names, by ID or
// by *Type, which forwards to the run's (RunTypeInstall).
type TypeRunInstallSpec struct {
	Name string
	Node *Type
}
