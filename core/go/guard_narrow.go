package core

// GUARD NARROWING (ADR-013, 2026-08-08 amendment): the flow-sensitive
// half of branch analysis. `if (x is T) [then] [else]` tells the
// analysis something different about x in each arm, and this file is
// what says it — extracting the `x is T` facts a condition carries
// (extractGuardClauses), installing the then-arm refinement
// (ApplyGuardNarrowing) and the else-arm complement
// (ApplyComplementNarrowing), each returning the restore closure its
// caller defers.
//
// Two invariants are load-bearing and must not drift:
//
//   - The narrowed binding KEEPS the source's value ID. is-narrowing is
//     static-only — at run time the binding is unchanged — so a fresh ID
//     would strand every later reference as an unseated carrier.
//   - Else-narrowing applies only to a SINGLE-clause condition. For a
//     conjunction the else path is `notA or notB`; neither clause can be
//     subtracted, and one exhaustive clause does not make the else
//     unreachable.
//
// Core, not check: every operand is a core Value / *Type, the bindings
// live in r.Defs, and the diagnostics go to r.Check — all core-owned.
// The predicate-implied-type derivation these clauses consult is
// guard_predicate.go, moved down alongside.

// GuardClause describes one `x is T` clause detected in a condition.
type GuardClause struct {
	Name string
	Type *Type
	// ThenOnly marks a clause whose fact is sound only in the THEN branch —
	// a predicate-derived fact (`if (param is T) …` in the predicate's own
	// body, surfaced via a 2-token `pred x` guard). The complement (else)
	// carries no information for these, so ApplyComplementNarrowing skips
	// them; a direct `x is T` clause narrows both arms and stays ThenOnly
	// false.
	ThenOnly bool
}

// extractGuardClauses walks a condition list looking for triplets
// `Word(x) Word(is) TypeLiteral(T)` and returns the corresponding
// GuardClause entries. Skips anything that doesn't resolve to a
// bare type literal or an ObjectType. Accepts type-word references
// by looking them up on DefStacks.
func extractGuardClauses(r *Registry, condList Value) []GuardClause {
	if r == nil || condList.Data == nil {
		return nil
	}
	// A pre-evaluated paren condition arrives as a Boolean carrier
	// whose GuardFactInfo payload preserves the group's original
	// tokens (A3) — extract from those exactly as from a list body.
	var elems []Value
	if gf, ok := condList.Data.(GuardFactInfo); ok {
		elems = gf.Toks
	} else {
		list, err := AsList(condList)
		if err != nil || list.IsNil() || list.Len() < 3 {
			return nil
		}
		elems = list.Elems()
	}
	if len(elems) < 2 {
		return nil
	}
	// A compound boolean condition (`(a is Map) or (b is Map)`, `not (x is T)`)
	// does not license per-clause narrowing: under a disjunction either clause
	// could be the true one, and a negation inverts the arm each fact holds in.
	// Bail entirely — no component clause is sound — rather than narrow one.
	for _, e := range elems {
		if e.Parent.Equal(TWord) {
			if w, werr := AsWord(&e); werr == nil {
				switch w.Name {
				case "or", "nor", "xor", "xnor", "not":
					return nil
				}
			}
		}
	}
	var out []GuardClause
	for i := 0; i+2 < len(elems); i++ {
		if !elems[i].Parent.Equal(TWord) || !elems[i+1].Parent.Equal(TWord) {
			continue
		}
		wx, err := AsWord(&elems[i])
		if err != nil {
			continue
		}
		wis, err := AsWord(&elems[i+1])
		if err != nil || wis.Name != "is" {
			continue
		}
		tv := elems[i+2]
		var minted *Type
		if tv.Data != nil && tv.Parent.Equal(TWord) {
			inner, _ := AsWord(&tv)
			if e, ok := r.Defs.TopEntry(inner.Name); ok {
				tv = e.Body
				minted = e.TypeDef
			} else if t, ok := ResolveBuiltinTypeName(inner.Name); ok {
				// Builtin arm of the canonical cascade — post-opacity
				// (ADR-012 rule 4) `x is Integer` carries a Word here.
				tv = NewTypeLiteral(t)
			}
		}
		if tv.Data != nil && !IsClassType(&tv) && !(tv.IsDepScalar() && minted != nil) {
			continue
		}
		// A bare type-literal clause IS its type; an ObjectType keeps
		// its type at Parent (the minted object-type node); a PREDICATE
		// refine (DepScalar body) narrows to its MINTED lattice node
		// (DefEntry.TypeDef — the body value's Parent is only the base
		// family), whose depScalarUnifier admits an abstract carrier
		// tagged with it nominally. The one membership rule makes the
		// guard exactly the test every downstream boundary re-asks, so
		// the then-branch may treat the name as the refined type. This
		// legalizes validate-then-call (`if (x is Big) [g x] [0]` with
		// x:Integer), previously a gating no_signature false positive
		// while the program ran correctly — the named blocker for
		// check-by-default (completion plan 2.2).
		guardType := tv.Parent
		switch {
		case tv.IsDepScalar() && minted != nil:
			guardType = NodeOf(minted)
		case tv.Data == nil:
			guardType = literalNode(&tv)
		}
		out = append(out, GuardClause{Name: wx.Name, Type: guardType})
	}
	// 2-token predicate guard: `if (is-map x) [then] [else]`. A user predicate
	// whose body is itself an `x is T` test (predicateImpliedType) implies its
	// argument is T in the THEN branch — but ONLY there (ThenOnly): a false
	// return says nothing about which non-T shape x has. The variable must be a
	// live binding; the predicate must reduce to a single `param is T` fact.
	if len(out) == 0 && len(elems) == 2 &&
		elems[0].Parent.Equal(TWord) && elems[1].Parent.Equal(TWord) {
		wp, perr := AsWord(&elems[0])
		wx, xerr := AsWord(&elems[1])
		if perr == nil && xerr == nil {
			if _, bound := r.Defs.Top(wx.Name); bound {
				if t, tok := predicateImpliedType(r, wp.Name); tok {
					out = append(out, GuardClause{Name: wx.Name, Type: t, ThenOnly: true})
				}
			}
		}
	}
	return out
}

// BoolWord returns "true" / "false" for use in human-readable
// diagnostic text.
func BoolWord(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// LiteralCondValue inspects a condition list for a single boolean
// literal (true/false word or Boolean carrier). Returns (value,
// true) when the condition is statically determinable, or (false,
// false) otherwise. Used by `if` analysis to warn about
// unreachable branches.
func LiteralCondValue(condList Value) (bool, bool) {
	if condList.Data == nil {
		return false, false
	}
	list, err := AsList(condList)
	if err != nil || list.IsNil() || list.Len() != 1 {
		return false, false
	}
	only := list.Get(0)
	// A pre-evaluated paren cond arrives wrapped in a GuardFactInfo carrier
	// (the A3 guard-fact attach); its Prev payload holds the value the group
	// actually reduced to. Read a decided Boolean straight through the wrapper
	// so the unreachable-branch analysis still fires.
	if gf, gok := only.Data.(GuardFactInfo); gok {
		if bp, bok := gf.Prev.(BoolPayload); bok {
			return bp.B, true
		}
	}
	// Bare true/false word (parser emits these as Word values that
	// resolve to booleans in engine.stepWord; in check mode the
	// words stay as Words until the branch runs).
	if only.Parent.Equal(TWord) {
		w, err := AsWord(&only)
		if err == nil {
			if w.Name == "true" {
				return true, true
			}
			if w.Name == "false" {
				return false, true
			}
		}
	}
	// Concrete Boolean value with Data set (post-runtime path).
	if only.Parent.ConformsTo(TBoolean) && only.Data != nil {
		b, err := AsBoolean(only)
		if err == nil {
			return b, true
		}
	}
	return false, false
}

// ApplyGuardNarrowing installs then-branch narrowings for each
// `x is T` clause in the condition. Returns a restore func to pop
// the narrowings after the then-branch runs.
func ApplyGuardNarrowing(r *Registry, condList Value) func() {
	noop := func() {}
	if !r.Check.IsActive() {
		return noop
	}
	clauses := extractGuardClauses(r, condList)
	if len(clauses) == 0 {
		return noop
	}
	deadArm := false
	for _, c := range clauses {
		narrowed := NewCarrier(c.Type)
		// is-narrowing is a static-only refinement: at runtime the binding is
		// UNCHANGED, so the narrowed carrier must keep the source's identity and
		// recorder key (ShareIdentity) — its provenance (param slot / producing
		// event). NewCarrier mints a FRESH ID with no recorder key and no
		// localByID entry, so resolveOperand fails ("fn call operand of unknown
		// provenance") when the narrowed value feeds a user call — the stats as-summary `if (x is List) [build x] [x]` shape. The
		// slot already holds the right runtime value because it IS the same
		// binding, so no value-passing half is needed (unlike a closure capture).
		if cur, ok := r.Defs.Top(c.Name); ok {
			ShareIdentity(&narrowed, &cur)
			// MEET the guard type with the binding's current (abstract) carrier
			// rather than overwriting it with a bare NewCarrier(c.Type): an
			// incompatible pair collapses to Never — a DEAD then-arm the runtime
			// never runs — and a compound meet keeps its payload. A concrete
			// binding is a per-shape analysis artifact (see the redundant_guard
			// note) whose lattice tag under-approximates predicate membership, so
			// it keeps the plain NewCarrier form. A dynamic binding stays dynamic
			// through the meet so the narrowed value still discharges downstream
			// modality checks.
			if cur.Carrier && !IsConcrete(&cur) && c.Type != nil {
				meet := TandValues(cur, NewTypeLiteral(c.Type))
				if IsNeverShape(meet) {
					deadArm = true
				} else if IsBareTypeNode(&meet) {
					narrowed = NewCarrier(ValueType(meet))
					ShareIdentity(&narrowed, &cur)
				} else {
					narrowed = MintCarrierKeeping(&meet, meet.Data)
					ShareIdentity(&narrowed, &cur)
				}
				if IsDynamic(&cur) && !deadArm {
					SetDynamic(&narrowed, true)
				}
			}
			// Advisory (non-gating): the binding's STATIC type already
			// entails the guard, so the check cannot fail — the residue the
			// local-reasoning report calls the misleading defensive check
			// (`if (n is Big) …` where n:Big is already in the signature).
			// Non-concrete STRICT carriers only: a dynamic binding genuinely
			// needs the guard (it DISCHARGES the modality); a CONCRETE
			// binding is a per-shape analysis artifact (an `[x:Any]` param
			// analysed for the call `f 5` binds the literal 5, whose Integer
			// tag would flag a guard the fn's OTHER callers rely on) and its
			// lattice tag under-approximates predicate membership anyway
			// (value-level entailment — interval reasoning — is future
			// work). A non-concrete strict carrier IS the declared-type
			// record, so tag conformance is shape-independent. Dedup: fn
			// bodies re-analyse per shape and fixpoint round.
			if cur.Carrier && !IsConcrete(&cur) && !IsDynamic(&cur) &&
				cur.Parent != nil && c.Type != nil &&
				cur.Parent.ConformsTo(c.Type) {
				detail := "guard is always true: " + c.Name + " is already " +
					c.Type.String() + " — drop the check or make it an assertion"
				dup := false
				for _, d := range r.Check.Diagnostics {
					if d.Code == "redundant_guard" && d.Detail == detail {
						dup = true
						break
					}
				}
				if !dup {
					r.Check.AddDiagnostic(CheckDiagnostic{
						Code:   "redundant_guard",
						Detail: detail,
						Word:   "is",
						Row:    condList.Pos().Row,
						Col:    condList.Pos().Col,
					})
				}
			}
		}
		r.Defs.Push(c.Name, narrowed)
	}
	if deadArm {
		// The guard can never hold for this arg shape — the then-arm is dead.
		// Suppress its (unreachable) dispatch errors; the runtime never runs it.
		r.Check.SuppressBodyErrors++
	}
	return func() {
		if deadArm {
			r.Check.SuppressBodyErrors--
		}
		for _, c := range clauses {
			r.Defs.Pop(c.Name)
		}
	}
}

// ApplyComplementNarrowing installs else-branch narrowings — for
// each `x is T` clause it tries to compute the complement of T in
// x's current carrier type and, if non-trivial, pushes the
// complement carrier onto x's DefStack. Currently only refines
// when x's existing binding is a disjunction: the matching
// alternative is subtracted. Returns a restore func.
func ApplyComplementNarrowing(r *Registry, condList Value) func() {
	noop := func() {}
	if !r.Check.IsActive() {
		return noop
	}
	clauses := extractGuardClauses(r, condList)
	// Else-branch narrowing (and its dead-arm suppression) is sound only when
	// the extracted clauses COMPLETELY characterise the condition — i.e. it is a
	// single `x is T` guard. For a conjunction `A and B`, the else path is
	// `notA or notB`: neither clause can be subtracted (either could be the
	// false one), and one exhaustive clause does NOT make the else unreachable
	// (it stays reachable whenever the other clause fails). So restrict to the
	// single-clause case and leave a conjunction's else untouched. (In practice
	// a compound condition's inner guards are captured as opaque ParenExpr
	// values, so extractGuardClauses already returns 0 clauses for them; this
	// guard makes the single-guard assumption explicit and sound-by-construction
	// rather than reliant on that representation.)
	if len(clauses) != 1 {
		return noop
	}
	type applied struct{ name string }
	var pushed []applied
	deadArm := false
	for _, c := range clauses {
		// A ThenOnly clause (a predicate-derived fact) carries no else-branch
		// information: a false predicate says nothing about which non-T shape x
		// has, so there is nothing to subtract.
		if c.ThenOnly {
			continue
		}
		cur, ok := r.Defs.Top(c.Name)
		if !ok {
			continue
		}
		// A DYNAMIC binding's else path stays gradual: the guard failed at
		// runtime, but the binding's static type is Any (or a dynamic carrier),
		// so subtracting T would wrongly forbid every later use. Leave it as-is.
		if IsDynamic(&cur) {
			continue
		}
		// Else-branch narrowing: x had type `cur`; the guard `x is T`
		// failed, so on the else path x is `cur tand (tnot T)`. The
		// negation + intersection algebra computes this uniformly and is
		// strictly more capable than the old exact-alternative subtraction:
		//   - a disjunct loses every alternative contained in T, including
		//     when T is a *supertype* of an alternative ((Integer tor
		//     String) tand tnot Number → String);
		//   - a plain type disjoint from T is unchanged (no-op);
		//   - a type wholly inside T collapses to Never (unreachable else).
		// Negate the minted NODE itself (not its recorded content): the
		// guard's else-fact is nominal — "x is not a T" — and resolving
		// a refinement node to its DepScalar body here would substitute
		// the interval complement, changing what this arm has always
		// claimed (see negateTypeResolved).
		complement := negateTypeResolved(NewTypeLiteral(c.Type))
		narrowed := TandValues(cur, complement)
		if IsNeverShape(narrowed) {
			// Else branch is unreachable for x — the guard held for every value
			// of x's type. Mark the arm dead (so its unreachable dispatch errors
			// are suppressed) and leave the binding as-is rather than push a
			// Never carrier that fails every later use. Sound because this is the
			// sole clause of the condition (single-guard gate above).
			deadArm = true
			continue
		}
		// Normalise to carrier form: a single surviving type becomes a
		// carrier of that type (Parent = the type, like NewCarrier); a
		// disjunct or other compound keeps its payload and is marked
		// abstract.
		if IsBareTypeNode(&narrowed) {
			narrowed = NewCarrier(ValueType(narrowed))
		} else {
			narrowed = MintCarrierKeeping(&narrowed, narrowed.Data)
		}
		if ValuesEqual(narrowed, cur) {
			// Complement did not refine cur (T disjoint from cur, or boru
			// has no positive representation for the exact difference).
			continue
		}
		// Preserve the source binding's value ID (see ApplyGuardNarrowing): the
		// else-branch value is the SAME runtime binding, statically refined to the
		// complement type, so it must resolve to cur's provenance. Set AFTER the
		// ValuesEqual(narrowed, cur) check above so the "did not refine" early-out
		// (which can compare by ID) is unaffected.
		ShareIdentity(&narrowed, &cur)
		r.Defs.Push(c.Name, narrowed)
		pushed = append(pushed, applied{name: c.Name})
	}
	if deadArm {
		// The guard held for every value of x's type — the else-arm is dead.
		// Suppress its (unreachable) dispatch errors; the runtime never runs it.
		r.Check.SuppressBodyErrors++
	}
	if len(pushed) == 0 && !deadArm {
		return noop
	}
	return func() {
		if deadArm {
			r.Check.SuppressBodyErrors--
		}
		for _, p := range pushed {
			r.Defs.Pop(p.name)
		}
	}
}
