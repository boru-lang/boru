package core

// Core value classifiers (Stage 3a of the four-piece split): pure
// predicates over core Values and registry scope state that the compiler,
// checker, AND interpreter all consult. Moved down from the compiler-piece
// files — they carry no recording or emission behavior.

// materialiseMember materialises one container member and reports whether it
// CHANGED — a stripped carrier recovered to a concrete original (different
// Carrier flag or ID). ok=false when the member's original is unknown, so the
// whole container cannot be recovered. The shared per-member step of
// materialise's list and map arms.
// bearsActiveTokens reports whether a value (recursively through lists and
// maps) contains a token the interpreter evaluates or re-steps — a word, a
// paren expression, an interpolation, a splice or reach marker.
func BearsActiveTokens(v Value) bool {
	if IsWord(v) || IsParenExpr(v) || IsInterpString(v) || IsSplice(v) || IsReach(v) {
		return true
	}
	switch d := v.Data.(type) {
	case ListPayload:
		for _, e := range d.Elems {
			if BearsActiveTokens(e) {
				return true
			}
		}
	case MapPayload:
		if d.M == nil {
			return false
		}
		for _, k := range d.M.Keys() {
			mv, _ := d.M.Get(k)
			if BearsActiveTokens(mv) {
				return true
			}
		}
	}
	return false
}

// ModuleScopeBinding reports whether name's active binding sits at module /
// global scope — NOT an enclosing fn's param or body-local (the
// ComputeCaptures depth rule: Depth > baseline means enclosing-fn-local). A
// nil baseline (no enclosing fn) makes every binding module scope.
func ModuleScopeBinding(r *Registry, name string) bool {
	baseline := r.TopFnBaseline()
	if baseline == nil {
		return true
	}
	return r.Defs.Depth(name) <= baseline[name]
}

// fnValueZeroArg reports whether v is a function VALUE whose LANDING the
// read-guard must refuse: it carries a genuine 0-arg overload the
// interpreter auto-fires the moment the value lands with no operands
// (containerFnAutoDispatchRisk), in a shape the compiled landing does
// not yet model. Built from the kernel's canonical Fallback-flag
// predicates (fnValueHasZeroArgSig / fnValueOnlyZeroArgSigs, engine.go)
// — this replaced a count-based phantom heuristic that encoded the same
// verdicts opaquely.
//
// The refusal set is REPRESENTATION-dependent, and deliberately so —
// probe-verified (2026-08-01) on `m.x` with a 0-arg+1-arg overload set:
//   - the DIRECT-literal spelling `{x: (fn …)}` compiles and agrees
//     (7/7): the check pass runs the interpreter loop over the concrete
//     member, and the recorded events model the fire;
//   - the PARKED spelling `{x: mx/v}` (an aggregate view, recognisable
//     by its synthetic Fallback sig) compiled to the raw fn value —
//     a silent divergence — so it must refuse until the landing model
//     covers parked mixed-overload members (the tracked graduation;
//     frontier-nur038-seal.tsv's mixed-overload row pins the refusal).
//
// A pure property fn (every real overload 0-arg) refuses in BOTH
// representations. Merging the two arms = extending the compiled
// landing model, not editing this predicate.
func FnValueZeroArg(v Value) bool {
	d, ok := v.Data.(FnDefInfo)
	if !ok {
		return false
	}
	if !fnValueHasZeroArgSig(v) {
		return false // no genuine 0-arg overload: needs args, lands as data
	}
	if FnValueOnlyZeroArgSigs(d) {
		return true // pure property fn: auto-fires in both representations
	}
	for i := range d.Signatures {
		if d.Signatures[i].Fallback {
			return true // parked aggregate view: mixed-set landing unmodelled
		}
	}
	return false // direct literal: the recorded events model the fire
}

// IsSteplessValue reports whether the step loop would leave v exactly as it is
// — a value it PLACES rather than one it evaluates, dispatches or applies.
//
// It admits the five concrete scalar leaves and nothing else, and that is a
// deliberate ALLOWLIST rather than a denylist of active token kinds. A shape it
// does not recognise is treated as active, so a token kind added later stays
// correct here by default; a denylist would silently begin treating it as data,
// and the failure mode of that is a body that stops running — a wrong answer,
// not an error. Eval marks a value the loop evaluates rather than places, so it
// is excluded even on an admitted payload.
//
// The wider scalar leaves (BigInt, Decimal, None) would be equally sound and
// are left out because nothing measured needs them yet: admitting a payload
// costs a proof each time.
func IsSteplessValue(v Value) bool {
	switch v.Data.(type) {
	case IntPayload, FloatPayload, StrPayload, BoolPayload, AtomPayload:
		return IsConcrete(v) && !v.Eval
	}
	return false
}

// IsSteplessWindow reports whether running vs through the interpreter would be
// the IDENTITY on them: every value is placed, nothing dispatches, so the
// residual is the window itself.
//
// Two callers, both skipping an engine that could only give back what it was
// handed:
//
//   - `do {key:[body]}` (basic): the compiled lane reaches the handler with the
//     body already computed — `do {n:[a add 1]}` lowers to CALL_NATIVE add /
//     MAKE_LIST, so the list to "run" is [6];
//   - CALL_DYNAMIC_MIXED (eng): the window islands because the compiler could
//     not rule out a callable value INTERIOR to it, and when the runtime values
//     turn out to be plain data the island returns the window verbatim.
func IsSteplessWindow(vs []Value) bool {
	for _, v := range vs {
		if !IsSteplessValue(v) {
			return false
		}
	}
	return true
}

func IsInertConst(v Value) bool {
	if v.Carrier || v.Dynamic || IsBareTypeNode(v) {
		return false
	}
	switch d := v.Data.(type) {
	case IntPayload, FloatPayload, StrPayload, BoolPayload, AtomPayload,
		PathonPayload, NonePayload, BigIntPayload, DecimalPayload,
		TimePayload, DurationPayload, TimezonePayload:
		return true
	case MicronPayload:
		// A Micron instance (Emailon / Urlon / user kind) is inert ONLY
		// because the family is immutable — `set` on a Micron is an
		// explicit error (the erroring set signatures in the language
		// layer). The payload's OrderedMap is pointer-backed, so if
		// mutation were ever allowed, pooled consts would corrupt
		// across loop iterations and this arm must be removed.
		return true
	case ExtensionPayload:
		// The kernel cannot see into an extension payload, so mutability
		// is the OWNING TYPE's call: bake only when its Behavior declares
		// the values immutable (the ConstBakeable capability — Bytes),
		// reached by the same parent-chain walk every capability dispatch
		// uses. Extension types without the capability — Socket, Listener,
		// Timeout/Interval, Module instances, every plugin type that never
		// opted in — keep the historical refusal. This is what lets a
		// module-scope `def crlf (convert Bytes "\r\n")` bake into a
		// stored-fn unit (mini-s3's recv-until delimiter) with freshness
		// still owned by the existing frozen-read / dep-snapshot gates.
		for t := v.Parent; t != nil; t = t.Parent {
			if cb, ok := t.Behavior().(ConstBakeable); ok {
				return cb.BakeableConst(v)
			}
		}
		return false
	case MicronTypeInfo:
		// A Micron type body (`refine Micron {fields}`): structural
		// descriptor like the RecordTypeInfo arm below — sound when its
		// field map is carrier-free.
		return typeBodyConstOK(v)
	case DepScalarInfo:
		// A predicate / refinement type (`Integer gt 10`): self-contained
		// (base family + bound, no registry, no canonical-pointer hazard per
		// eng CLAUDE.md), so the value bakes by value into the const pool.
		// The bound is recovered for a stripped operand via origByID
		// (RememberOriginal at the constructor); type-algebra words
		// (tcmp/teq/tand/…) then run over the baked predicate at run time.
		return true
	case RecordTypeInfo, OptionsTypeInfo, ChildTypeInfo, DisjunctInfo, ClassTypeInfo, TableTypeInfo:
		// STRUCTURAL type bodies (what a bound type name pushes at a
		// use site — make's operand). Sound as consts when their
		// interior is carrier-free (typeBodyConstOK): the payload is
		// pointer-backed (shared, not copied) and the minted lattice
		// node rides the body's Parent POINTER, which stays
		// canonical. Never deduped. A class/object body qualifies only
		// when every field default is data (no method fn-values). A
		// Table type (`Test.TestSet`) is a thin wrapper over its row
		// RecordType, so it folds whenever that record does.
		return typeBodyConstOK(v)
	case FnUndefInfo:
		// A function SIGNATURE value (`fnsig [[Integer] [String]]`,
		// `typeof (Mapper of [Integer String])`): pure descriptor data —
		// param/return *Type pointers, no invocable body and no mutable state —
		// so it bakes by value (the *Type pointers are shared, already
		// canonical from construction). typeof/teq/is then read the baked
		// signature at run time. A pattern-bearing param (rare in signatures)
		// could embed non-const data, so it refuses conservatively.
		return fnSigConstOK(d)
	case FnDefInfo:
		// A function VALUE used as DATA — a residual (`f/v`), a map/list member
		// (`{b:f/v}`), or an introspection operand (`arityof (fn …)`) — NOT a
		// call site. It bakes as a const only with no closure state: no captured
		// bindings (which would snapshot check-pass values, divergent from the VM
		// pass) and no module sub-registry. The body tokens ride inside the
		// payload and are never re-stepped while the value is data; a CALL of the
		// value is a separate dispatch path (a bare `(fn …) args` auto-dispatch
		// records the fn-body splice and refuses; a `/v`-referenced fn does not
		// auto-dispatch, so `f/v` / `{b:f/v}` are pure data).
		if len(d.Captured) > 0 {
			return false
		}
		if d.Registry == nil {
			return true
		}
		// A module-export fn value bakes as DATA — a bare residual (`MathUtil.sqrt`),
		// a branch-arm operand, a container member, OR a comparator passed to another
		// fn (`xs M.sort M.by-num`). The sub-registry pointer it carries is the SAME
		// object the compiled run shares (RunProgram runs on the check-pass
		// registry), so the baked const and the live fn are one object — no
		// check/VM value divergence. The VM applies it faithfully at run time:
		//   - a TRIVIAL-DELEGATION wrapper (`MathUtil.sqrt`, every own sig a
		//     `[Word(inner)]` pass-through) via callDynamic → tryNativeFnApply, which
		//     re-resolves the inner native in that registry;
		//   - a REAL boru body via the island sub-engine (callDynTrailTop/…'s
		//     `vc.island().Run([fn, args…])`), which INTERPRETS the fn in
		//     fnDef.Registry — CallBoru, module-private scope and all. So a real body
		//     applies soundly too (compile == interpret, verified). A macro stays
		//     refused (applied only by name / compile-time expansion, never as data).
		return !d.Macro
	case *SurfaceInfo:
		// A surface type (`def Shape surface {area: (fnsig …)}`): an immutable
		// contract descriptor riding its canonical minted node via the Type
		// pointer (shared, not copied). Its conformance set (Conform, filled in
		// by `exposes`) is consulted through the SAME shared payload the
		// canonical node's installed unifier holds — and the compiled path runs
		// the VM over the check-pass registry without re-minting, so the baked
		// const and the live surface are one object, never divergent. Admitting
		// it lets `Shape` as a residual / operand to is/typeof/teq/tand/tor/tnot/
		// unify all compile. The Required method shapes are fnsig values, const
		// by fnSigConstOK above.
		return surfaceConstOK(d)
	case ListPayload:
		for _, e := range d.Elems {
			if !IsInertConstMember(e) {
				return false
			}
		}
		return true
	case MapPayload:
		if d.M == nil {
			return false
		}
		for _, k := range d.M.Keys() {
			mv, _ := d.M.Get(k)
			if !IsInertConstMember(mv) {
				return false
			}
		}
		return true
	case XmlElementPayload:
		// An immutable Node/Xml literal (`<a x="1"><b/>text</a>`). It is a
		// constant value at parse time (parser/xml_literal.go emits
		// NewXmlElement; only a ${}-interpolated literal becomes the deferred
		// Word/__XI builder, which is genuine runtime construction and stays
		// refused). Value-semantics with structural sharing — never mutated in
		// place: the MUTABLE FlexXml is *FlexXmlData, which falls to default and
		// never bakes (the bytecode_constbake_test mutation-safety guard) — so it
		// is sound to pool, exactly like the List / Map cases. Bakes when its
		// attribute values and child nodes are themselves inert members (text
		// scalars + nested immutable elements, recursed via isInertConstMember →
		// isInertConst).
		for _, c := range d.Cren {
			if !IsInertConstMember(c) {
				return false
			}
		}
		if d.Attr != nil {
			for _, k := range d.Attr.Keys() {
				av, _ := d.Attr.Get(k)
				if !IsInertConstMember(av) {
					return false
				}
			}
		}
		return true
	case ReachInfo:
		// A receiverless inert lens (`$.name`, `$.a.b`, `$!.x`, `$.1`). See
		// isInertReach: only the non-eval, no-receiver, all-literal-key shape
		// qualifies — the dot-access Eval reach (which the engine expands in
		// place) is excluded.
		return isInertReach(v)
	case ParenExprPayload:
		// A codequote'd (Quoted) ParenExpr — `codequote (1 add 2)` →
		// `paren([1 word(add) 2])`. It is immutable CODE-AS-DATA: stepLiteral
		// leaves a Quoted ParenExpr unevaluated (engine.go step 4), and the VM
		// never re-steps a const, so it bakes by value exactly like the
		// macroexpand token list. An UNQUOTED ParenExpr is expanded and
		// re-stepped in place, so it is NEVER a const — gate strictly on Quoted.
		// Its tokens must themselves be inert members (Words / atoms / scalars /
		// nested inert parens), screened by isInertConstMember.
		return isInertQuotedParen(v)
	case *TypeSchemaInfo:
		// An installed generic schema (`def Box gen [T] class {value:T}`). The
		// schema is immutable data — its instantiation memo lives in the
		// registry, not this struct — and rides the canonical minted node via
		// its Type pointer (shared, not copied), so it bakes as a const exactly
		// like a structural type body. Admitting it lets `make Box {…}` (T
		// inferred at run time), `is`/`typeof`/`teq` over the schema, and the
		// schema as a residual all compile; the instantiated `Box of [Integer]`
		// type already baked via typeBodyConstOK.
		return schemaConstOK(d)
	default:
		return false
	}
}

// isFnTypedCarrier reports whether v is a Function-typed CARRIER — a
// [Function]-returning call result on the simulated stack (e.g. `(mk2 5)`), as
// distinct from a CONCRETE baked fn value (Carrier false, the introspection /
// inert-`/v` case). The carrier bit is what resolves the apply-vs-inert
// ambiguity in Finalize: a carrier lead auto-applies, a concrete fn does not.
//
// A carrier typed by a fn-SHAPE type counts too: a value the checker types as
// `T` (`def T fnsig Integer Integer`, then a T-typed class field read) IS a
// function at run time — the shape's membership admits nothing else — so the
// recorder's maybe-callable questions must answer for it exactly as for a
// Function-typed carrier, or the compiled program leaves the fn inert where
// the interpreter applies it (NUR095, retired by this rule; pinned in
// class.tsv's fn-members rows). The VM's dynamic-call
// ops stay faithful either way: a value that turns out not to be callable is
// left as data (callDynamic's IsAppliableFn arm).
func IsFnTypedCarrier(v Value) bool {
	return v.Carrier && v.Parent != nil &&
		(v.Parent.ConformsTo(TFunction) || TypeIsFnShape(v.Parent))
}

// TypeIsFnShape reports whether t is a function-SHAPE type — a type whose
// concrete inhabitants are function values: the anonymous fn-shape type
// itself (`fnsig …`, FunctionSignature) or a named fn-shape node minted
// under it (InstallType parents named shapes at FunctionSignature and
// attaches FnUndefUnifier membership).
func TypeIsFnShape(t *Type) bool {
	return t != nil && t.ConformsTo(TFnUndef)
}

// isFnValueResidual reports whether v is ANY fn value — a concrete FnDefInfo (a
// baked /v reference) or a Function-typed value (carrier or not). Used to
// keep a fn value out of the trailing-arg positions of a leading-fn apply.
func IsFnValueResidual(v Value) bool {
	// One body with the VM's runtime test (isAppliableFn, vm.go): the
	// recorder's residual question and the VM's apply question are the
	// SAME question, and post-ADR-011 both reduce to Function
	// conformance (the payload arm covers values mid-construction).
	return IsAppliableFn(v)
}

// isModuleFamilyValue reports whether v is a concrete module value — an
// Ideal/Module descriptor, or a module NAMESPACE (a plain Map carrying the
// module-namespace facet — the value `import` binds; NUR038 retired the
// Ideal/ModuleExport wrapper type). The descriptor is identified by the
// kernel-declared TModule (moduletype.go — the former string-path probe
// is gone), the namespace by its kernel facet. These values are
// immutable and produced deterministically by `import`, so a pure read
// of one is a compile-time constant (runtime export growth is
// ledger-modelled — module_export_growth.go).
func IsModuleFamilyValue(v Value) bool {
	if !IsConcrete(v) || v.Parent == nil {
		return false
	}
	return ModuleNSOf(v) != nil || v.Parent.Equal(TModule)
}

// constFoldAgrees reports whether two const-fold probe evaluations produced
// the SAME bakeable value, compared by CanonValue — the exact structural key
// the const interner dedups by (emit.go's constIdx). It is the shared
// determinism gate behind every twice-and-compare const-bake
// (tryFoldModuleConst, the macroexpand splice in carrierResults, and the
// engine's constFoldContainerVal): a clock / rand / mutation-bearing read
// whose two probes drift renders a different canon and is refused, so no
// nondeterministic value is ever frozen into the program.
//
// CanonValue, NOT String(): String() is a DISPLAY rendering that conflates
// values which bake DIFFERENTLY — a bare type node vs the string of its name
// (`Integer` vs 'Integer'), an atom vs a same-spelled string (name/q vs
// 'name'), an Integer vs an equal-magnitude Float, a fn vs a same-shaped fn
// with a different body — so two genuinely divergent probes could
// String()-match and freeze an UNSOUND const. CanonValue is the bake
// identity itself ("same canon" ⟺ "interns to one const"), and it is no
// coarser than String() on any bakeable shape, so a legitimately
// deterministic fold still agrees (no coverage change) while the
// conflations String() hid can no longer slip a frozen value through.
func ConstFoldAgrees(a, b Value) bool { return CanonValue(a) == CanonValue(b) }

// isAppliableFn reports whether a runtime value is a callable the interpreter
// would auto-apply: a Function-typed value or an FnDefInfo payload.
func IsAppliableFn(v Value) bool {
	if _, ok := v.Data.(FnDefInfo); ok {
		return true
	}
	return v.Parent != nil && v.Parent.ConformsTo(TFunction)
}

// dynStackShuffleWords is the closed set of Forth-style stack words whose
// dispatch the compiler may trust over dynamically-shaped stacks: each is
// stack-only (BarrierPos 0 — it never forward-collects) and, when its
// registered signature is the single all-Any one, one extra or differently-
// typed stack value cannot flip which overload it takes. Shared by
// dynamicStackShuffleOK (the refusal bypass for a MATCHED dispatch over a
// dynamic operand) and dynShuffleConsumerAt (the gradual-arity modeling
// gate for a mixed-arity mutator's statement-position result).
var DynStackShuffleWords = map[string]bool{
	"dup": true, "swap": true, "drop": true, "over": true, "rot": true,
	"nip": true, "tuck": true, "dup2": true, "swap2": true, "drop2": true,
	"over2": true,
}

// typeBodyConstOK walks a structural type body's interior: every
// reachable constraint/default must be a bare type node, an inert
// constant, or another clean type body. A check-mode CARRIER inside
// (a generic instantiation built over a class body whose default was
// stripped) would bake the analysis artefact into the const — the
// caught differential mismatch rendered `r:Float` where the
// interpreter rebuilds `r:1.0` — so any carrier, or any payload this
// walk doesn't know, refuses.
func typeBodyConstOK(v Value) bool {
	if v.Carrier || v.Dynamic {
		return false
	}
	if IsBareTypeNode(v) {
		return true
	}
	memberOK := func(m Value) bool {
		// A concrete mutable instance default (`class {x:(make Foo 1)}`,
		// `{items:(flex [])}`, `{bits:(make Array [0 0 0])}`) is a const-safe SCHEMA
		// member: `make` runs FreshenDefault over every field default
		// (core_make.go), handing each instance its OWN fresh copy, so the baked
		// body is a READ-ONLY TEMPLATE — never the mutable value a later `set`
		// writes. The mutation-safety invariant therefore holds for a default
		// INSIDE a type body, even though isInertConst still (correctly) rejects
		// the same instance standing alone or as a data-list member, where nothing
		// freshens it. Const-folded by constFoldContainerVal.
		if !m.Carrier && !m.Dynamic && isFreshenedInstance(m) {
			return true
		}
		return typeBodyConstOK(m) || IsInertConst(m)
	}
	switch d := v.Data.(type) {
	case RecordTypeInfo:
		return allFieldsInert(d.Fields, memberOK)
	case OptionsTypeInfo:
		return allFieldsInert(d.Fields, memberOK)
	case ChildTypeInfo:
		if !memberOK(d.Child) {
			return false
		}
		if !allInert(d.Elements, memberOK) {
			return false
		}
		for _, en := range d.Entries {
			if !memberOK(en.Value) {
				return false
			}
		}
		return true
	case DisjunctInfo:
		return allInert(d.Alternatives, memberOK)
	case ClassTypeInfo:
		// A class / object type body is const-bakeable iff every field
		// default is plain data — a method (fn-value) field is not, so a
		// class with methods (the surface-body case) still refuses. The
		// canonical *Type rides the body's payload pointer (shared, not
		// copied), so it stays canonical at run time; `make` recovers the
		// field schema from the baked body. The parent chain's fields must
		// be data too (AllFields merges them).
		return allFieldsInert(d.AllFields(), memberOK)
	case TableTypeInfo:
		// A Table type body (`make Test.TestSet …`, a module-exported
		// Table type as a get-fold result or residual) is a thin wrapper
		// over the row RecordType — const-safe exactly when that record's
		// field types are. The canonical *Type rides the body's payload
		// pointer (shared, not copied), like every structural body.
		return allFieldsInert(d.Record.Fields, memberOK)
	case MicronTypeInfo:
		// A Micron type body (`refine Micron {fields}`): field schema
		// like a record's — const-safe when every constraint/default is
		// inert. The canonical *Type rides the payload pointer.
		return allFieldsInert(d.Fields, memberOK)
	}
	return false
}

// fnSigConstOK reports whether a function-signature value bakes as a const.
// The signature is pure type/descriptor data; the only embeddable Value is an
// optional structural pattern on a param, which must itself be const-safe (a
// pattern that carried a carrier or data map would not be inert).
func fnSigConstOK(info FnUndefInfo) bool {
	for _, sig := range info.Sigs {
		for _, p := range sig.Params {
			if p.Pattern != nil && !(IsInertConst(*p.Pattern) || typeBodyConstOK(*p.Pattern)) {
				return false
			}
		}
	}
	return true
}

// surfaceConstOK reports whether a surface type bakes as a const: it must ride
// a canonical node (Type != nil) and every required-operation shape must be a
// const-safe value (a fnsig descriptor, or any other inert member).
func surfaceConstOK(s *SurfaceInfo) bool {
	if s == nil || s.Type == nil {
		return false
	}
	if s.Required == nil {
		return true
	}
	return allFieldsInert(s.Required, func(v Value) bool {
		return IsInertConst(v) || typeBodyConstOK(v)
	})
}

// schemaConstOK reports whether a generic schema bakes as a const: its body
// must be a const-safe structural type body (or itself inert / a bare node)
// and every parameter's extends-bound / default must be inert or a bare type
// node. A computed constraint or a non-data (method-bearing fn) body refuses,
// falling back faithfully.
func schemaConstOK(s *TypeSchemaInfo) bool {
	if s == nil || s.Type == nil {
		return false
	}
	// Post-opacity (ADR-012 rule 4) a schema body's type variables are
	// RESOLVED gen-placeholder literals — a `[:T]` child resolves at
	// consumption via the canonical cascade while the gen bindings are
	// live — so bare type nodes cover them and no Word-member escape
	// hatch is needed (the pre-opacity isParam plumbing is gone).
	memberOK := func(m Value) bool {
		return IsBareTypeNode(m) || typeBodyConstOK(m) || IsInertConst(m)
	}
	for _, p := range s.Params {
		if p.HasBound && !memberOK(p.Bound) {
			return false
		}
		if p.HasDefault && !memberOK(p.Default) {
			return false
		}
	}
	return memberOK(s.Body)
}

// isInertQuotedParen reports whether v is a codequote'd (Quoted) ParenExpr that
// bakes as a const. A Quoted ParenExpr is CODE-AS-DATA: the interpreter's
// stepLiteral leaves it unevaluated (engine.go step 4 gates on `!v.Quoted`),
// and the VM never re-steps a const, so it pushes verbatim like the macroexpand
// token list — the compiled residual renders byte-identically to the
// interpreter's data value. An UNQUOTED ParenExpr is expanded and re-stepped in
// place, so it is NEVER baked here. Every token must itself be an inert const
// member (Words / atoms / scalars / nested inert parens), via isInertConstMember.
func isInertQuotedParen(v Value) bool {
	if !v.Quoted || v.Carrier || v.Dynamic {
		return false
	}
	toks, err := AsParenExpr(v)
	if err != nil {
		return false
	}
	for _, tk := range toks {
		if !IsInertConstMember(tk) {
			return false
		}
	}
	return true
}

// isInertReach reports whether v is an INERT receiverless lens — a first-class
// Reach value that evaluates to ITSELF (`$.name`, `$.a.b`, `$!.x`, `$.1`):
// Eval=false, no Receiver tokens, and every segment a LITERAL key (no computed
// paren to evaluate at run time). Such a lens is immutable data, not code: it
// renders as itself, `typeof` reads Reach, and `apply`/`each`/`filter`/`sortby`
// /`getpath` walk its segments against a FRESH receiver — none of which the
// engine expands or re-steps. That is the opposite of a dot-access Eval reach
// (`m.a.b`), which `isEvalReach`/`expandReach` lower to a get-chain IN PLACE;
// isInertConst rightly keeps THAT one out (it is a structural token, not data).
// The lens keys carry no canonical-*Type staleness hazard: they are Words /
// Atoms / scalars whose Parents are the canonical kernel types, copied by value
// safely (isInertConstMember screens out a bare type node). So the inert lens
// pools into the const table like an atom or a path.
func isInertReach(v Value) bool {
	if !IsReach(v) || v.Carrier || v.Dynamic {
		return false
	}
	info, err := AsReach(v)
	if err != nil || info.Eval || len(info.Receiver) > 0 {
		return false
	}
	for _, seg := range info.Segments {
		if seg.Computed || !IsInertConstMember(seg.KeyLit) {
			return false
		}
	}
	return true
}

// isInertConstMember reports whether v may ride as a MEMBER of a const
// compound (a list element or map field): an inert const, OR a fn VALUE. A fn
// value is immutable code, so it is safe inside a READ-ONLY const container — a
// method field of a data map (`{f: fn}`), the receiver of `m.f`. It is admitted
// only as a member, never as a standalone const: the top-level isInertConst
// switch still rejects a bare FnDefInfo (a top-level fn value is the apply /
// closure case, not bakeable data). At run time a poly `get` of the field
// returns the fn, which the fn-value-call boundary (OpCallDynamic) applies.
func IsInertConstMember(v Value) bool {
	if !v.Carrier && !v.Dynamic {
		// A Word token riding inside a quoted (non-eval) compound — what
		// `macroexpand` returns as data (`[5 word(add) 5]`). Safe as a const
		// MEMBER: the compound is pushed as inert data and never auto-evaluated
		// (a source eval-list is reduced before baking), and a word's Parent is
		// the canonical kernel TWord, so the by-value copy carries no stale
		// behaviour (unlike a bare type node — the canonical-*Type hazard — which
		// is deliberately NOT admitted here). The standalone isInertConst switch
		// still rejects a bare Word, so a top-level word never bakes as code.
		if IsWord(v) {
			return true
		}
		// A bare type node as a structural-pattern MEMBER — the type leaves of
		// `{a:Integer}`, `[Integer String]`, `[Resource Entity]`: the inert
		// operand of a static `is` / `typeof` / `size`. Admitted as a const
		// member (it was previously excluded outright). Soundness: the member's
		// Parent is the canonical lattice pointer the parser resolved, copied by
		// value inside a READ-ONLY container, and the whole pattern bakes as one
		// const the VM pushes verbatim — the `is`/`typeof` handler then runs
		// byte-identically to the interpreter over the same value. The standalone
		// isInertConst switch still rejects a bare type node, so a top-level type
		// operand keeps reaching the runtime via the by-ID type table
		// (OpPushType), the canonical path that survives a later `behave`.
		if IsBareTypeNode(v) && v.ID != "" {
			return true
		}
		if fd, ok := v.Data.(FnDefInfo); ok {
			return len(fd.Captured) == 0 && fd.Registry == nil
		}
		// A dot-access reach (`r.int`, `m.a.b`) riding inside a NEVER-evaluated
		// compound — a NoEvalArgs code body the driving word stores or drops
		// (Test.prop builds a PropertySpec map; Test.skip discards it;
		// Test.check-prop CallBorus it via its native handler), or a quoted code
		// list. Unlike isInertReach (the STANDALONE detached lens, which must be
		// receiverless + Eval=false so the engine never expands it at the
		// pointer), a reach as a MEMBER is pure DATA: the VM pushes the baked
		// compound verbatim and never expands a reach (in-place expansion is an
		// interpreter stepLiteral behaviour), and the interpreter equally keeps it
		// as data inside the inert compound — so the reach bakes by value,
		// differential-identical. Its receiver / literal-key tokens must
		// themselves be inert members (Words / atoms / scalars, canonical
		// Parents); a COMPUTED segment (a paren to evaluate) is code, so refuse.
		if IsReach(v) {
			return inertReachMember(v)
		}
		// A deferred paren expression (`(add 1 2)`, `(k)`) riding inside a
		// NEVER-evaluated compound — a `reach` key list's computed segment
		// (`reach 5 [a (add 1 2) c]`), where the paren is stored unevaluated and
		// re-run at APPLY time over the shared registry, not stepped at the VM
		// pointer. Like the Word / Reach member cases this is pure DATA: the VM
		// pushes the baked compound verbatim and the reach handler builds the
		// same Computed segment the interpreter does, so it bakes by value
		// differential-identically. Its tokens must themselves be inert members
		// (Words / atoms / scalars / nested inert parens) — a token that would
		// drag in a carrier or mutable instance refuses.
		if IsParenExpr(v) {
			toks, err := AsParenExpr(v)
			if err != nil {
				return false
			}
			for _, tk := range toks {
				if !IsInertConstMember(tk) {
					return false
				}
			}
			return true
		}
		// A sugar marker (`Box<Integer>`, `=>`) riding inside a
		// never-evaluated compound — what a macro capturing an angle
		// operand re-splices as data (`macroexpand (m Box<Integer>)`).
		// Pure descriptor payload (SugarInfo) over the canonical kernel
		// TSugar Parent, so it bakes by value exactly like a Word member:
		// the VM pushes the compound verbatim, and the engine lowers the
		// marker only if the data is later evaluated — identically to
		// the interpreter. The value streams it carries must themselves
		// be inert members; the standalone isInertConst switch still
		// rejects a bare marker, so a top-level marker never bakes as
		// code.
		if sinfo, ok := AsSugar(v); ok && IsSugar(v) {
			if sinfo.Head.Parent != nil && !IsInertConstMember(sinfo.Head) {
				return false
			}
			for _, it := range sinfo.Items {
				if !IsInertConstMember(it) {
					return false
				}
			}
			return true
		}
	}
	return IsInertConst(v)
}

// isDelegationFnDef reports whether a Function VALUE is a trivial-delegation
// wrapper — EVERY own sig is a `[Word(inner)]` pass-through to an inner native
// (a module method like rand-int / MathUtil.sqrt), safely dispatched VM-native
// via tryNativeFnApply. A user fn carries a REAL body, so it is NOT a delegation
// and must island instead. An anonymous lambda or a sig-less value is not one.
func IsDelegationFnDef(fd FnDefInfo) bool {
	sigs := fd.OwnSigs()
	if len(sigs) == 0 {
		return false
	}
	for i := range sigs {
		if _, ok := trivialDelegationTarget(&sigs[i]); !ok {
			return false
		}
	}
	return true
}

// IsNativeWordFnDef reports whether a Function VALUE is a parked reference to a
// REGISTERED NATIVE word — `add/v`, `sub/v`, the value a `def m {a:add/v}` map
// hands back through `m.a`. No own sig carries a *BoruImpl body, which is
// exactly the shape tryNativeFnApply can run without a sub-engine: a Go
// handler reads host state off the dispatching registry and resolves no body
// words. Whether a matched sig actually HAS a handler is checked where it is
// called, not here — duplicating it would add an arm nothing can reach.
//
// The boru-body exclusion is the whole content of the predicate, and it is the
// same line IsDelegationFnDef draws for a different reason. A user fn's handler
// is InstallFnDef's body-splicing wrapper, which expects the dispatch frame the
// interpreter builds around it; calling it directly diverges. So a value with
// ANY boru-bodied own sig stays on the island, which runs the body faithfully
// as a nested Run.
//
// Measured shape (the census's largest single cluster, path-modifier.tsv):
//
//	add/v                      -> 9 own sigs, every one a Go handler  -> true
//	def d fn [[n:Integer]…]  d/v -> 1 own sig, *BoruImpl              -> false
func IsNativeWordFnDef(fd FnDefInfo) bool {
	sigs := fd.OwnSigs()
	if len(sigs) == 0 {
		return false
	}
	for i := range sigs {
		if _, isBoru := sigs[i].Impl.(*BoruImpl); isBoru {
			return false
		}
	}
	return true
}

// IsSelfContainedGoFnDef reports whether a Function VALUE is a SELF-CONTAINED
// Go-implemented fn: anonymous, wrapping nothing, with at least one own
// signature and every one of them a Go handler — the wrapper a fn-util word
// PRODUCES (`(FnUtil.const 7)`, `(FnUtil.on gt2/v sq/v)`: goFnValue's
// shape). Such a value dispatches on ITS OWN signatures, which is the
// interpreter's rule for an anonymous value (execFnDefLiteral: a
// self-contained Function value is a stable handle, compiled and dispatched
// from its own sigs, never a fresh registry lookup).
//
// It is NOT IsNativeWordFnDef's parked-native class, though it passes that
// predicate: a parked native resolves by NAME through the live registry, and
// a self-contained value's Name is a LABEL that may coincide with a
// registered word — fn-util's `const` wrapper shares its name with the
// singleton-type maker — so resolving it there dispatches the wrong word.
// Measured: `((FnUtil.const 7) 99)` compiled to 99 for the interpreter's 7
// through exactly that lookup (eng's vmNativeApplicable / tryNativeFnApply).
//
// A modifier wrapper (usurp, `/u`, `/s`, `/f` — Wraps set, and usurp's
// ArgsReversed mark) is excluded: its handler returns TOKENS for a tape,
// and the VM re-dispatches what it wraps instead (UnwrapModifierChain). A
// macro is data, applied only by name.
func IsSelfContainedGoFnDef(fd FnDefInfo) bool {
	if !fd.Anonymous || fd.Macro || fd.Wraps != nil || fd.ArgsReversed {
		return false
	}
	return IsNativeWordFnDef(fd)
}

// RegisteredWordIsNative reports whether name's LIVE binding in r is native
// through and through — it exists, and no overload carries a *BoruImpl body.
//
// It pairs with IsNativeWordFnDef, which asks the same question of a parked
// VALUE. Both have to hold before that value may be dispatched straight to a Go
// handler, because the two can disagree: a value parked while `size` was purely
// native keeps six native sigs, and a later `def size fn [[n:Pos] …]` extension
// (the only way the language lets a core word be rebound — see the
// `extend_owner` guard) adds a boru overload to the NAME. A consumer that
// resolves by name would then reach a boru body without the dispatch frame its
// handler expects. Requiring both keeps such a word on the interpreter, which
// runs it the way it was built to run.
func RegisteredWordIsNative(r *Registry, name string) bool {
	if r == nil {
		return false
	}
	inner := r.Lookup(name)
	if inner == nil {
		return false
	}
	for i := range inner.Signatures {
		if _, isBoru := inner.Signatures[i].Impl.(*BoruImpl); isBoru {
			return false
		}
	}
	return true
}

// isFreshenedInstance reports whether v is a concrete MUTABLE instance that
// make's FreshenDefault (core_make.go) copies per instance when v is a
// class-schema field default — an Object/Store/flex value. Admitting one
// as a SCHEMA member (ONLY through typeBodyConstOK's memberOK, never standalone)
// is mutation-safe precisely because every `make` freshens it into its own copy;
// outside a type body nothing freshens it, so isInertConst keeps it out of the
// const pool. Pairs with the const-bake regression gate.
func isFreshenedInstance(v Value) bool {
	return IsClassInstance(v) || IsStore(v) || IsFlexList(v)
}

// allInert reports whether pred holds for every value in ms — the shared
// "every member must be const-safe" loop of the type-body const-bake checks.
func allInert(ms []Value, pred func(Value) bool) bool {
	for _, m := range ms {
		if !pred(m) {
			return false
		}
	}
	return true
}

// allFieldsInert reports whether pred holds for every value in an ordered map
// (a nil map is not const-bakeable). Shared by the structural-type-body and
// surface-type const-bake checks.
func allFieldsInert(m *OrderedMap, pred func(Value) bool) bool {
	if m == nil {
		return false
	}
	for _, k := range m.Keys() {
		fv, _ := m.Get(k)
		if !pred(fv) {
			return false
		}
	}
	return true
}

// inertReachMember reports whether a Reach may ride as a MEMBER of an inert
// const compound (see isInertConstMember's reach clause). It is deliberately
// more permissive than isInertReach: a member reach is never expanded at the
// engine pointer (the containing compound is inert), so a receiver and Eval=true
// are fine — only a computed segment (a ParenExpr to run) or a non-inert
// receiver/key token disqualifies it.
func inertReachMember(v Value) bool {
	if !IsReach(v) || v.Carrier || v.Dynamic {
		return false
	}
	info, err := AsReach(v)
	if err != nil {
		return false
	}
	for _, rt := range info.Receiver {
		if !IsInertConstMember(rt) {
			return false
		}
	}
	for _, seg := range info.Segments {
		if seg.Computed || !IsInertConstMember(seg.KeyLit) {
			return false
		}
	}
	return true
}
