package lang_test

import (
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
)

// checkErrs counts error-severity diagnostics from `boru check` over src.
func checkErrs(t *testing.T, src string) int {
	t.Helper()
	a, _ := lang.New()
	cr, _ := a.Check(src)
	n := 0
	for _, d := range cr.Diagnostics {
		if d.Severity == "error" {
			n++
		}
	}
	return n
}

// A FLEX container's check-mode type must be its precise subtype (FlexMap /
// FlexList / FlexXml), not the bare Node supertype, so a downstream Flex* op
// matches its overload. `flex` / `node` carry a ReturnsFn modelling the input's
// node family (mirroring FlexDeepCopy). flex.tsv regressed all 40 of these.
func TestFlexCheckSubtype(t *testing.T) {
	clean := []string{
		`set b/q 2 (flex {a:1})`,                                 // FlexMap set
		`append 4 (flex [1 2])`,                                  // FlexList append
		`push 3 (flex [1 2])`,                                    // FlexList push
		`pop (flex [1 2])`,                                       // FlexList pop
		`sort (flex [3 1 2])`,                                    // FlexList sort
		`each [ add 1 ] (flex [1 2 3])`,                          // FlexList each
		`def f (flex {a:1}) def n (node f) set a/q 9 f drop n.a`, // node → plain Map
	}
	for _, src := range clean {
		if n := checkErrs(t, src); n != 0 {
			t.Errorf("flex op should check clean: got %d errors for %q", n, src)
		}
	}
	// NEGATIVE: a scalar is not flexable — the [TNode] arg slot rejects it
	// (this is a static signature error, not a runtime one).
	if n := checkErrs(t, `flex 1`); n == 0 {
		t.Errorf("flex of a scalar must still be flagged")
	}
}

// A user DISJUNCT / ENUM type must be usable as a `def` type body and a type-
// algebra operand in check mode. `tor` mirrors its handler's disjunct (not a
// branch-merge that mishandles the nil-Parent None literal), `enum` runs its
// pure handler so the result carries its members, and toCarrier preserves the
// DisjunctInfo. compare.tsv regressed 15 of these.
func TestDisjunctEnumTypeBodyCheck(t *testing.T) {
	clean := []string{
		`String tor None`, // union builds, no halt
		`def Maybe (String tor None) Maybe tcmp Maybe`,                           // disjunct def + tcmp
		`def Maybe (String tor None) def g fn [[m:Maybe] [Integer] [1]] ('x' g)`, // disjunct param
		`def Color enum [red green blue] Color tcmp Color`,                       // enum def + tcmp
		`def Color enum [red green blue] Color tcmp Enum`,                        // enum vs builtin Enum
		`def Cat class {x:Integer} def Maybe (String tor None) Cat tcmp Maybe`,
	}
	for _, src := range clean {
		if n := checkErrs(t, src); n != 0 {
			t.Errorf("type-value op should check clean: got %d errors for %q", n, src)
		}
	}
}

// `typeof` of a CONCRETE argument must return the precise type literal in check
// mode (a concrete-gated ReturnsFn), so `def T (typeof v)` gets a valid type
// body — `typeof (const 1)` is the singleton type, not the bare `Type` carrier
// the def-type validator rejected. class.tsv regressed these.
func TestTypeofConcreteSingletonCheck(t *testing.T) {
	clean := []string{
		`def One (typeof (const 1)) end 1 is One`,
		`def One (typeof (const 1)) end 2 is One`,
		`def One (typeof (const 1)) end 1.0 is One`,
		`def T (typeof 5) end 7 is T`, // typeof a plain literal is a valid body too
	}
	for _, src := range clean {
		if n := checkErrs(t, src); n != 0 {
			t.Errorf("typeof-of-concrete type body should check clean: got %d errors for %q", n, src)
		}
	}
}

// An abstract carrier already TAGGED as a predicate-refine (subset) type
// satisfies that type's param nominally — the tag is the contract guarantee
// (a `Big`-returning fn), and the value-level predicate cannot be re-verified
// on an abstract carrier. `def mk fn [[] [Big] [50]] use (mk)` failed because
// the Big-returning result was predicate-checked as if abstract. A CONCRETE
// value is still predicate-checked.
func TestPredicateRefineReturnCheck(t *testing.T) {
	clean := `def Big (Integer gt 10) def mk fn [[] [Big] [50]] def use fn [[n:Big] [Integer] [n]] use (mk)`
	if n := checkErrs(t, clean); n != 0 {
		t.Errorf("predicate-refine-returning fn into a same-type param: expected 0 errors, got %d", n)
	}
	// NEGATIVE: a concrete value that fails the predicate is still rejected.
	bad := `def Big (Integer gt 10) def use fn [[n:Big] [Integer] [n]] use 5`
	if n := checkErrs(t, bad); n == 0 {
		t.Errorf("a concrete 5 (not > 10) for a Big param must still be flagged")
	}
}

// A dispatch modifier (usurp / stack-args / forward-args / force-arity, the
// `/u` `/s` `/f` `/N` desugarings) over a stored fn-ref read via dot-access
// (`m.a` where `m = {a:add/v}`) sees a dynamic(Any) carrier — getNodeReturns
// cannot narrow a dispatch-bearing field — and must yield a gradual Function
// carrier in check mode rather than an illegal_ref. path-modifier.tsv regressed
// 12 of these.
func TestStoredFnRefModifierCheck(t *testing.T) {
	clean := []string{
		`def m {a:add/v} end m.a/u 1 2`,
		`def m {s:sub/v} end m.s/f 10 3`,
		`def m {s:sub/v} end 10 3 m.s/s`,
		`def m {a:add/v} end m.a/2 1 2`,
		`def o {m:{a:add/v}} end o.m.a/u 1 2`,
		`def m {s:sub/v} end force-arity 2 (usurp (m.s)) 10 3`,
	}
	for _, src := range clean {
		if n := checkErrs(t, src); n != 0 {
			t.Errorf("stored-fn-ref modifier should check clean: got %d errors for %q", n, src)
		}
	}
	// NEGATIVE: usurp of a CONCRETE non-fn value is still an illegal_ref — the
	// check-mode gradual fallback fires only for a dynamic-Any / Function
	// carrier, never a concrete-typed one.
	if n := checkErrs(t, `usurp 5`); n == 0 {
		t.Errorf("usurp of a concrete non-fn value must still be flagged")
	}
}

// A module's exports are statically DECLARED, so the checker resolves them in
// check mode — for `import` (qualified) it already did, and now `unpack` binds
// them UNQUALIFIED too. `unpack 'boru:mod'` / `unpack Export 'boru:mod'` gained
// RunInCheckMode + a kept-concrete module-name string, so a later bare word
// (`sqrt`) resolves instead of flagging undefined_word. NOT a runtime-only
// effect — the same declared exports `import` reads. (module-struct / unpack.tsv)
func TestUnpackModuleCheck(t *testing.T) {
	clean := []string{
		`unpack 'boru:math-util' sqrt 16.0`,
		`unpack MathUtil 'boru:math-util' sqrt 16.0`,
	}
	for _, src := range clean {
		if n := checkErrs(t, src); n != 0 {
			t.Errorf("unpack module then bare-word use: expected 0 errors, got %d for %q", n, src)
		}
	}
}

// A def-bound mini-language value (`def poly (fn …)`) resolves statically:
// a later `mini poly …` expands through the binding instead of flagging
// "no mini-language is registered". The register tombstone, by contrast,
// is flagged AT CHECK TIME (its DryPassWrap ReturnsFn mirrors the
// unconditional mini_registry_frozen raise).
func TestMiniLangRegisterCheck(t *testing.T) {
	clean := `import "boru:minilang"  def poly (fn [[src:String opts:Map] [Integer] [((opts.x pow 2) add (3 mul opts.y))]])  mini poly 'x^2 + 3*y' {x:10, y:2}`
	if n := checkErrs(t, clean); n != 0 {
		t.Errorf("minilang value form: expected 0 errors, got %d", n)
	}
	// The tombstone surfaces statically as a guaranteed-error mirror
	// diagnostic (its DryPassWrap ReturnsFn re-raises under the dry pass).
	a, _ := lang.New()
	cr, _ := a.Check(`import "boru:minilang"  MiniLang.register`)
	found := false
	for _, d := range cr.Diagnostics {
		if d.Code == "mini_registry_frozen" {
			found = true
		}
	}
	if !found {
		t.Error("the register tombstone must be flagged statically (mini_registry_frozen)")
	}
}

// A name def-bound to a COMPUTED fn value has no Defs binding under
// analysis — it resolves through the per-pass fn-carrier side table. The
// mini and emit bound-name paths must consult it like parse does, so a
// valid computed-value program checks clean (degrading to a dynamic
// carrier) instead of flagging <surface>_unknown_lang, and still runs.
func TestMiniEmitComputedBindingCheck(t *testing.T) {
	// NB the binding must not collide with a built-in kind name (a name
	// like `m` is the micron short form, and a registered kind wins).
	miniSrc := `import "boru:minilang"  def mk fn [[] [Function] [fn [[src:String opts:Map] [Any] [src add src]]]]  def myml (mk)  mini myml 'x'`
	if n := checkErrs(t, miniSrc); n != 0 {
		t.Errorf("computed mini binding: expected 0 check errors, got %d", n)
	}
	emitSrc := `import "boru:emitlang"  def mke fn [[] [Function] [fn [[value:Any opts:Map] [String] ['E']]]]  def mye (mke)  emit mye {a:1}`
	if n := checkErrs(t, emitSrc); n != 0 {
		t.Errorf("computed emit binding: expected 0 check errors, got %d", n)
	}
	// The interpreter runs both for real — the doubled source proves the
	// BOUND fn ran (a fallthrough parse of 'x' could echo it unchanged).
	// This is what the program MEANS; it is asserted on the reference
	// engine because neither shape compiles yet, and each compile failure
	// is booked as the defect it is.
	a, _ := lang.New()
	if got, err := a.RunInterp(miniSrc); err != nil || len(got) != 1 || got[0] != "xx" {
		t.Errorf("computed mini binding run = %v (err %v), want [xx]", got, err)
	}
	b, _ := lang.New()
	if got, err := b.RunInterp(emitSrc); err != nil || len(got) != 1 || got[0] != "E" {
		t.Errorf("computed emit binding run = %v (err %v), want [E]", got, err)
	}
	for _, src := range []string{miniSrc, emitSrc} {
		c, _ := lang.New()
		gotC, cErr := c.Run(src)
		if !lang.NoteCompileDefect(t, src, gotC, cErr) && cErr != nil {
			t.Errorf("compiled: %v", cErr)
		}
	}
}

// A fn whose analysed body residual is PROVABLY DISJOINT from its declared
// return type is flagged at check time with the byte-identical runtime
// type_error (checkBodyReturnConformance, eng/go/core_helpers.go) — the
// check-mode mirror of the RET check. Only impossibility flags; every
// value-dependent or under-modeled shape stays with the runtime check.
func TestDeclaredReturnBodyConformance(t *testing.T) {
	flagged := []string{
		`def f fn [[a:Integer] [String] [42]] f 1`,      // constant body, wrong family
		`def f fn [[a:Integer] [String] [a add 1]] f 1`, // computed body, wrong family
	}
	for _, src := range flagged {
		if n := checkErrs(t, src); n == 0 {
			t.Errorf("impossible declared return must be flagged: %q", src)
		}
	}
	clean := []string{
		`def f fn [[a:Integer] [String] ["ok"]] f 1`, // conforming body
		// value-dependent subset return: Integer residual against a
		// DepScalar refine of Integer — runtime may accept, stays clean.
		`def Big (Integer gt 10) def f fn [[n:Big] [Big] [n add 1]] f 50`,
		// disjunct residual with a conforming alternative (branch join).
		`def f fn [[a:Integer] [Integer] [if (a gt 0) [a] ["neg"]]] f 1`,
		// mutual recursion: the sibling is undefined at install-time
		// analysis; the memoized residual is not evidence.
		`def isod fn [[n:Integer] [Boolean] [if (n eq 0) [false] [isev (n sub 1)]]] def isev fn [[n:Integer] [Boolean] [if (n eq 0) [true] [isod (n sub 1)]]] isev 10`,
		// fn-value frontier: apply leaves an unapplied Function residual.
		`def myfn ([x:Integer] => [x add 1000]) def runner fn [[myfn:Function v:Integer] [Integer] [v myfn/v apply]] def doubler ([x:Integer] => [x mul 2]) runner (doubler/v) 5`,
	}
	for _, src := range clean {
		if n := checkErrs(t, src); n != 0 {
			t.Errorf("must stay clean (%d errors): %q", n, src)
		}
	}
}

// A typed-def whose DepScalar constraint is violated by a CONCRETE literal is
// flagged at check time (the lenient base-conformance install is gated to
// abstract bodies; a concrete body falls through to Unify, which runs the
// self-contained predicate) — native_definition.go's DepScalar branch.
func TestTypedDefDepScalarCheck(t *testing.T) {
	flagged := []string{
		`def Big (Integer gt 10) def x:Big 5 x`,
		`def x:(Integer gt 10) 5 x`,
	}
	for _, src := range flagged {
		if n := checkErrs(t, src); n == 0 {
			t.Errorf("failing DepScalar literal must be flagged: %q", src)
		}
	}
	clean := []string{
		`def Big (Integer gt 10) def x:Big 50 x`,
		`def x:(Integer gt 10) 50 x`,
		// abstract (computed) body: value unknown, stays with the runtime.
		`def Big (Integer gt 10) def g fn [[a:Integer] [Integer] [a]] def x:Big (g 50) x`,
	}
	for _, src := range clean {
		if n := checkErrs(t, src); n != 0 {
			t.Errorf("must stay clean (%d errors): %q", n, src)
		}
	}
}

// A `make` whose target resolves to a class / Resource schema and whose
// construction map is CONCRETE is validated at check time — unknown field,
// missing non-defaulted field, concrete field-value type — with the
// byte-identical runtime messages (check.CheckMakeConstruction via
// makeObjReturns). Value-dependent shapes stay with the runtime constructor.
func TestMakeConstructionCheck(t *testing.T) {
	flagged := []string{
		`def P class {x:1} end make P {z:9}`,                // unknown field
		`def P class {x:Integer} end make P {x:"s"}`,        // field type mismatch
		`def P class {x:Integer} end make P {}`,             // missing non-defaulted field
		`make Entity {nope:1 kind:"k" spec:"s" entity:"e"}`, // Resource unknown field
	}
	for _, src := range flagged {
		if n := checkErrs(t, src); n == 0 {
			t.Errorf("bad construction must be flagged: %q", src)
		}
	}
	clean := []string{
		`def P class {x:1} end make P {x:5}`, // valid override
		`def P class {x:1} end make P {}`,    // defaulted field omitted
		`make Entity {kind:"k" spec:"s" entity:"e"}`,
		// computed field value: value unknown, stays with the runtime.
		`def P class {x:Integer} end def g fn [[v:Integer] [Integer] [v]] make P {x:(g 5)}`,
	}
	for _, src := range clean {
		if n := checkErrs(t, src); n != 0 {
			t.Errorf("must stay clean (%d errors): %q", n, src)
		}
	}
}

// A HETEROGENEOUS concrete collection's element carrier is the lattice JOIN
// of its element types (ElementCarrierFromValue): distant cousins stay a
// strict Disjunct so the body dispatch distributes per alternative, exactly
// as the runtime dispatches per element — not the common ancestor, which
// failed no_signature on bodies every element actually satisfies.
func TestHeterogeneousElementBodyCheck(t *testing.T) {
	clean := []string{
		`[1 2 "s"] each [1 add]`,    // add has Integer AND String overloads
		`[1 "s"] fold [add] 0`,      // fold body over mixed elements
		`[1 2 "s"] each [true and]`, // connective accepts both
	}
	for _, src := range clean {
		if n := checkErrs(t, src); n != 0 {
			t.Errorf("heterogeneous body should check clean (%d errors): %q", n, src)
		}
	}
	// NEGATIVE: a genuinely-bad body still flags (undefined word).
	if n := checkErrs(t, `[1 2 "s"] each [nosuchword9]`); n == 0 {
		t.Errorf("undefined body word must still be flagged")
	}
	// NEGATIVE: a statically-empty no-init fold is fold's own guaranteed
	// runtime error, flagged with the byte-identical message.
	for _, src := range []string{`fold [add] []`, `fold [add] {}`} {
		if n := checkErrs(t, src); n == 0 {
			t.Errorf("statically-empty no-init fold must be flagged: %q", src)
		}
	}
}

// The DISTRIBUTE-OVER-DISPATCH invariant (checker-accuracy-review.10.md §3):
// any carrier that can denote two or more concrete types must dispatch as
// the JOIN of its per-alternative dispatches — never fail no_signature
// against a slot some alternative satisfies, never commit to one
// alternative. Each carrier shape that can denote multiple types is pinned:
// (1) a payload-bearing strict Disjunct (a branch join of distant cousins),
// (2) a NAMED-union declared return (`[T]` with `def T (Integer tor
// String)`) — the shape that historically carried only the T tag and failed
// to distribute, (3) a dynamic carrier (reachable-overload union), and
// (4) a union-typed param inside the body. A NEW multi-denotation carrier
// shape must be added here when introduced.
func TestDistributeOverDispatchInvariant(t *testing.T) {
	clean := []string{
		// (1) branch-join disjunct into an overloaded word (both alternatives
		// have add overloads; runtime takes one, checker joins both).
		`def f fn [[x:Integer] [Boolean] [x lt 2]] def v (if (f 1) [1] ["s"]) v add 1`,
		`def f fn [[x:Integer] [Boolean] [x lt 2]] def v (if (f 1) [1] ["s"]) v add v`,
		// (2) named-union declared return distributes downstream.
		`def T (Integer tor String) def id fn [[x:T] [T] [x]] (id 1) add 1`,
		`def T (Integer tor String) def id fn [[x:T] [T] [x]] (id 1) add (id 1)`,
		// (3) dynamic carrier (map get) into a typed word.
		`def m {a:1} (m get "a") add 1`,
		// (4) union param used with an overload every alternative satisfies.
		`def T (Integer tor String) def g fn [[x:T] [Any] [x add x]] g 1`,
	}
	for _, src := range clean {
		if n := checkErrs(t, src); n != 0 {
			t.Errorf("multi-denotation dispatch must distribute (%d errors): %q", n, src)
		}
	}
	// NEGATIVE: a word NO alternative satisfies still fails loudly.
	if n := checkErrs(t, `def T (Integer tor String) def id fn [[x:T] [T] [x]] (id 1) nosuchword8`); n == 0 {
		t.Errorf("an undefined word after a union result must still be flagged")
	}
}

// A DECLARED-union parameter (`x:T`, `def T (A tor B)`) claims every
// alternative is a valid input, so a body dispatch with NO overload for one
// alternative is an ERROR — the fn breaks its own contract for a valid
// argument (`g true` raises no_signature at runtime). An ANALYSIS-join
// disjunct (if/else branch join) keeps the partial_dispatch WARNING: the
// runtime materialises one alternative, so the failing path may be dead.
// disjunctPartitionReturns severity by DisjunctInfo.Declared provenance.
func TestDeclaredUnionParamPartialDispatchError(t *testing.T) {
	if n := checkErrs(t, `def T (Integer tor Boolean) def g fn [[x:T] [Any] [x add 1]] g 1`); n == 0 {
		t.Errorf("partial dispatch over a declared union param must be an error")
	}
	clean := []string{
		// branch-join disjunct: same partial shape, stays a warning.
		`def y (if (1 gt 0) [1] ["s"]) mod y 2`,
		// a guard discharges the failing alternative — no partial at all.
		`def T (Integer tor Boolean) def g fn [[x:T] [Any] [if (x is Integer) [x add 1] [0]]] g 2`,
		// every alternative dispatches — no partial.
		`def T (Integer tor String) def g fn [[x:T] [Any] [x add x]] g 1`,
	}
	for _, src := range clean {
		if n := checkErrs(t, src); n != 0 {
			t.Errorf("must stay clean (%d errors): %q", n, src)
		}
	}
}

// The undefined-forward-ref skip in checkBodyReturnConformance is scoped to
// the analysed body's SOURCE SPAN: an undefined word inside one overload /
// earlier definition of `f` must not shield a DIFFERENT `f` body whose
// declared return is provably wrong (the runtime raises the same type_error).
func TestBodyConformanceScopedToRedefinition(t *testing.T) {
	src := `def f fn [[a:Integer] [String] [g a]] def f fn [[a:Integer] [String] [42]] def g fn [[a:Integer] [String] ["ok"]] f 1`
	if n := checkErrs(t, src); n == 0 {
		t.Errorf("redefined f with impossible return must be flagged despite the first f's rescued forward ref")
	}
	// The forward-ref shape itself (mutual recursion) still skips cleanly —
	// pinned here alongside the positive so the span scoping never regresses it.
	clean := `def isod fn [[n:Integer] [Boolean] [if (n eq 0) [false] [isev (n sub 1)]]] def isev fn [[n:Integer] [Boolean] [if (n eq 0) [true] [isod (n sub 1)]]] isod 3`
	if n := checkErrs(t, clean); n != 0 {
		t.Errorf("mutual recursion must stay clean (%d errors)", n)
	}
}

// SetStrictCheck invalidates the fn-body analysis memo when the mode
// changes: a summary cached by a NON-strict Check on the same instance
// would skip re-analysis, so the body's dynamic_dispatch advisories would
// never surface in the later strict pass.
func TestStrictToggleInvalidatesFnSummaries(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	src := `def f fn [[x:Any] [Any] [x add 1]] f 1`
	if _, err := a.Check(src); err != nil {
		t.Fatalf("non-strict check: %v", err)
	}
	a.SetStrictCheck(true)
	res, err := a.Check(src)
	if err != nil {
		t.Fatalf("strict check: %v", err)
	}
	found := false
	for _, d := range res.Diagnostics {
		if d.Code == "dynamic_dispatch" {
			found = true
		}
	}
	if !found {
		t.Errorf("strict re-check on a reused instance must re-analyse the fn body and report dynamic_dispatch; diagnostics: %+v", res.Diagnostics)
	}
}
