package core

import (
	"strings"
	"testing"
)

// compileCheckRegistry arms a registry as a COMPILE pass: check mode on
// (Begin) plus the Compiling flag the compile entry points set.
func compileCheckRegistry(t *testing.T) *Registry {
	t.Helper()
	r := covRegistry(t, nil)
	end := r.Check.Begin()
	r.Check.Compiling = true
	t.Cleanup(func() {
		r.Check.Compiling = false
		end()
	})
	return r
}

// TestCheckFnCarrierBindTable drives the side table's arms directly: the
// nil-registry reset, the absent-table reset, the create and reuse write
// arms, a miss, and the pass-scoped clearing. (The lang-level twin
// exercises the shim aliases; this one keeps core's own coverage whole
// after the move down from basic.)
func TestCheckFnCarrierBindTable(t *testing.T) {
	ResetCheckFnCarrierBinds(nil) // nil-registry arm
	r := covRegistry(t, nil)
	ResetCheckFnCarrierBinds(r) // absent table: no-op
	if _, hit := CheckFnCarrierBind(r, "x"); hit {
		t.Error("empty table must miss")
	}
	NoteCheckFnCarrierBind(r, "a", NewCarrier(TFunction)) // create arm
	NoteCheckFnCarrierBind(r, "b", NewCarrier(TFunction)) // reuse arm
	if _, hit := CheckFnCarrierBind(r, "a"); !hit {
		t.Error("noted name must resolve")
	}
	if _, hit := CheckFnCarrierBind(r, "zz"); hit {
		t.Error("unnoted name must miss")
	}
	ResetCheckFnCarrierBinds(r)
	if _, hit := CheckFnCarrierBind(r, "a"); hit {
		t.Error("reset must clear the pass-scoped table")
	}
}

// TestCheckFnCarrierBindDepth: the table keeps the OUTERMOST fn-body depth
// a name was bound at (NUR192's shadow test) — a deeper rebind does not
// hide the enclosing bind, a shallower one lowers it — and the depth goes
// with the bind on undef and on the pass-scoped reset.
func TestCheckFnCarrierBindDepth(t *testing.T) {
	r := covRegistry(t, nil)
	if _, bound := CheckFnCarrierBindDepth(r, "a"); bound {
		t.Error("an unbound name has no depth")
	}
	r.Check.FnBodyDepth = 0
	NoteCheckFnCarrierBind(r, "a", NewCarrier(TFunction))
	r.Check.FnBodyDepth = 1
	NoteCheckFnCarrierBind(r, "a", NewCarrier(TFunction))
	NoteCheckFnCarrierBind(r, "b", NewCarrier(TFunction))
	if d, bound := CheckFnCarrierBindDepth(r, "a"); !bound || d != 0 {
		t.Errorf("the outermost depth stays: got %d %v, want 0", d, bound)
	}
	if d, bound := CheckFnCarrierBindDepth(r, "b"); !bound || d != 1 {
		t.Errorf("a name first bound in a fn body records that depth: got %d %v, want 1", d, bound)
	}
	r.Check.FnBodyDepth = 0
	NoteCheckFnCarrierBind(r, "b", NewCarrier(TFunction))
	if d, _ := CheckFnCarrierBindDepth(r, "b"); d != 0 {
		t.Errorf("a shallower bind lowers the depth: got %d, want 0", d)
	}
	DropCheckFnCarrierBind(r, "a")
	if _, bound := CheckFnCarrierBindDepth(r, "a"); bound {
		t.Error("undef drops the depth with the bind")
	}
	ResetCheckFnCarrierBinds(r)
	if _, bound := CheckFnCarrierBindDepth(r, "b"); bound {
		t.Error("the reset clears the depths with the table")
	}
	// A registry without a check state records depth 0.
	r2 := covRegistry(t, nil)
	r2.Check = nil
	NoteCheckFnCarrierBind(r2, "c", NewCarrier(TFunction))
	if d, bound := CheckFnCarrierBindDepth(r2, "c"); !bound || d != 0 {
		t.Errorf("no check state: depth 0, got %d %v", d, bound)
	}
}

// TestStepWordCompileCarrierSubstitute — a COMPILE pass resolves a plain
// read of a name def-bound to a Function carrier through the side table:
// stepWord substitutes the carrier with no undefined_word diagnostic.
func TestStepWordCompileCarrierSubstitute(t *testing.T) {
	r := compileCheckRegistry(t)
	NoteCheckFnCarrierBind(r, "h", NewCarrier(TFunction))

	e := NewTop(r)
	e.Tape = NewTape([]Value{NewWord("h")}, StackHeadroom)
	if err := e.stepWord(e.Tape.At(0)); err != nil {
		t.Fatalf("substituting stepWord errored: %v", err)
	}
	got := e.Tape.At(0)
	if !got.Carrier || got.Parent == nil || !got.Parent.ConformsTo(TFunction) {
		t.Errorf("stepWord did not substitute the carrier: %v", got)
	}
	if len(r.Check.Diagnostics) != 0 {
		t.Errorf("no diagnostics expected, got %v", r.Check.Diagnostics)
	}
	if !r.Check.FnCarrierReadSubstituted {
		t.Error("the substitution must mark the pass (the silent-fallback flag)")
	}
}

// TestStepWordValCarrierSubstitutes — the `/v` read path resolves a
// carrier-bound name through the side table exactly as the bare read does
// (S1b-2): the carrier replaces the token, the pass is marked, no
// undefined_word is reported. This path used to decline the table on
// purpose (a substituted `/v` read once had no producing event and
// `(pmany digit/v)` compiled to a 0-arg call); the bare read's provenance
// notes — the def read and the local read — are what the lowering needs,
// and the `/v` read now carries the same ones.
func TestStepWordValCarrierSubstitutes(t *testing.T) {
	r := compileCheckRegistry(t)
	NoteCheckFnCarrierBind(r, "hv", NewCarrier(TFunction))

	e := NewTop(r)
	e.Tape = NewTape([]Value{NewWord("hv")}, StackHeadroom)
	if err := e.stepWordVal(e.Tape.At(0), WordInfo{Name: "hv", ArgCount: -1, ForceVal: true}); err != nil {
		t.Fatalf("/v step errored: %v", err)
	}
	got := e.Tape.At(0)
	if got.Undefined || !got.Carrier || got.Parent == nil || !got.Parent.ConformsTo(TFunction) {
		t.Errorf("a /v read of a carrier-bound name must substitute the carrier: %v", got)
	}
	if len(r.Check.Diagnostics) != 0 {
		t.Errorf("no diagnostics expected, got %v", r.Check.Diagnostics)
	}
	if !r.Check.FnCarrierReadSubstituted {
		t.Error("the substitution must mark the pass (the silent-fallback flag)")
	}
	// An UNBOUND name keeps the undefined_word diagnostic and the placeholder.
	e = NewTop(r)
	e.Tape = NewTape([]Value{NewWord("nope")}, StackHeadroom)
	if err := e.stepWordVal(e.Tape.At(0), WordInfo{Name: "nope", ArgCount: -1, ForceVal: true}); err != nil {
		t.Fatalf("/v step of an unbound name errored: %v", err)
	}
	if got := e.Tape.At(0); !got.Undefined {
		t.Errorf("an unbound /v read must keep the Undefined placeholder: %v", got)
	}
	if len(r.Check.Diagnostics) != 1 {
		t.Errorf("expected the one undefined_word diagnostic, got %v", r.Check.Diagnostics)
	}
}

// TestDefTopResolvesCarrierUnderAnalysis — the collection seat's binding
// lookup (Engine.DefTop, the plan walk's and the candidate scan's word
// resolution) reads the side table under an analysis pass, so a forward
// slot claims a computed fn's carrier where it used to see a bare word
// (`each f/v [1 2 3]` over `def f (mk 10)` declined "unmatched dispatch
// recovered at each"). A Defs binding wins; outside analysis the table is
// never consulted.
func TestDefTopResolvesCarrierUnderAnalysis(t *testing.T) {
	r := compileCheckRegistry(t)
	NoteCheckFnCarrierBind(r, "f", NewCarrier(TFunction))
	r.Defs.Push("d", NewInteger(7))
	e := NewTop(r)
	if got, ok := e.DefTop("f"); !ok || !got.Carrier || !got.Parent.ConformsTo(TFunction) {
		t.Errorf("under analysis the seat must resolve the table-bound carrier: %v %v", got, ok)
	}
	if got, ok := e.DefTop("d"); !ok || !IsConcrete(got) {
		t.Errorf("a Defs binding must resolve as itself: %v %v", got, ok)
	}
	if _, ok := e.DefTop("zz"); ok {
		t.Error("an unbound name must miss")
	}

	plain := covRegistry(t, nil)
	NoteCheckFnCarrierBind(plain, "f", NewCarrier(TFunction))
	if _, ok := NewTop(plain).DefTop("f"); ok {
		t.Error("outside analysis the table must not be consulted")
	}
}

// TestStepWordPlainCheckSubstitutesCarrier — the substitution fires on a
// PLAIN check too (the thirty-fifth increment): a name in the side table
// reads as its fn carrier with no undefined_word, exactly as on a compile
// pass — `boru check` used to flag `def k (FnUtil.const 7)  (k 99)` — while
// the compile-only FnCarrierReadSubstituted mark stays clear.
func TestStepWordPlainCheckSubstitutesCarrier(t *testing.T) {
	r := covRegistry(t, nil)
	defer r.Check.Begin()()
	NoteCheckFnCarrierBind(r, "h", NewCarrier(TFunction))

	e := NewTop(r)
	e.Tape = NewTape([]Value{NewWord("h")}, StackHeadroom)
	if err := e.stepWord(e.Tape.At(0)); err != nil {
		t.Fatalf("plain-check stepWord errored: %v", err)
	}
	if got := e.Tape.At(0); got.Undefined || !IsFnTypedCarrier(got) {
		t.Errorf("plain check must read the bound fn carrier, got %v", got)
	}
	if len(r.Check.Diagnostics) != 0 {
		t.Errorf("a resolved read reports nothing, got %v", r.Check.Diagnostics)
	}
	if r.Check.FnCarrierReadSubstituted {
		t.Error("the silent-fallback mark is a compile pass's alone")
	}
}

// TestTrapDeclinesFnCarrierBoundWord — a fallback window naming a
// fn-carrier-bound word declines the unmatched-dispatch trap: the name IS
// bound at run time (stepWordVal delivers the real Function value there),
// so the static no-match is a modeling artifact, not a definite runtime
// failure — a trap here raised signature_error where the interpreter
// succeeds (the pmany/pseq shape).
func TestTrapDeclinesFnCarrierBoundWord(t *testing.T) {
	e, _ := trapEngine(t, []Value{NewWord("trapw"), NewWord("cb")}, 0, []int{1})
	NoteCheckFnCarrierBind(e.Registry, "cb", NewCarrier(TFunction))
	if e.TryRecordUnmatchedDispatchTrap(WordInfo{Name: "trapw"}, trapFn(), SrcPos{}) {
		t.Error("a table-bound word in the window must decline the trap")
	}
}

// TestStepWordCompileCarrierMissStaysUndefined — the Compiling gate open
// but the name absent from the table: the diagnostic path is unchanged.
func TestStepWordCompileCarrierMissStaysUndefined(t *testing.T) {
	r := compileCheckRegistry(t)

	e := NewTop(r)
	e.Tape = NewTape([]Value{NewWord("nope")}, StackHeadroom)
	if err := e.stepWord(e.Tape.At(0)); err != nil {
		t.Fatalf("miss-arm stepWord errored: %v", err)
	}
	if got := e.Tape.At(0); !got.Undefined {
		t.Errorf("a table miss must keep the Undefined placeholder: %v", got)
	}
	if len(r.Check.Diagnostics) != 1 {
		t.Errorf("expected the one undefined_word diagnostic, got %v", r.Check.Diagnostics)
	}
}

// TestCheckFnCarrierBoundName pins the reverse lookup the def site uses to
// catch a DROPPED APPLY: a bind whose value is already table-bound under
// another name means the body's apply was not modeled (the analysis
// returned the callee unchanged), which compiled both names onto one slot
// and leaked the unconsumed argument into the residual.
func TestCheckFnCarrierBoundName(t *testing.T) {
	r := compileCheckRegistry(t)
	if _, hit := CheckFnCarrierBoundName(r, ""); hit {
		t.Error("an empty id must miss")
	}
	if _, hit := CheckFnCarrierBoundName(r, "v1"); hit {
		t.Error("an empty table must miss")
	}
	carrier := NewCarrier(TFunction)
	carrier.ID = "v1"
	NoteCheckFnCarrierBind(r, "f1", carrier)
	name, hit := CheckFnCarrierBoundName(r, "v1")
	if !hit || name != "f1" {
		t.Errorf("want the bound name f1, got %q hit=%v", name, hit)
	}
	if _, hit := CheckFnCarrierBoundName(r, "v2"); hit {
		t.Error("an unbound id must miss")
	}
}

// TestStepWordCarrierIsData — a word-TYPED CARRIER at the pointer is DATA,
// not a token. `IsWord` classifies on the parent type alone, so a carrier of
// type Word used to be dispatched as a word; it has no WordInfo, so the name
// was "" and every name arm fell through to the undefined-word tail, emitting
// a diagnostic that named no word and carried no position (NUR103). stepWord
// now collects it like any other value: there is nothing to look up and no
// arguments to collect.
func TestStepWordCarrierIsData(t *testing.T) {
	r := covRegistry(t, nil)
	end := r.Check.Begin()
	defer end()

	carrier := NewCarrier(TWord)
	e := NewTop(r)
	e.Tape = NewTape([]Value{carrier}, StackHeadroom)
	if err := e.stepWord(e.Tape.At(0)); err != nil {
		t.Fatalf("stepWord over a word carrier errored: %v", err)
	}
	if len(r.Check.Diagnostics) != 0 {
		t.Errorf("a word carrier raised diagnostics: %v", r.Check.Diagnostics)
	}
	if got := e.Tape.At(0); got.Undefined || !got.Carrier || !got.Parent.Equal(TWord) {
		t.Errorf("word carrier not collected as data: %v", got)
	}
}

// TestInstallDefDoesNotLowerCapturingRedefinitionInFnBody pins installDef's
// fn-body arm (the thirty-first increment): a CAPTURING fn value — a
// factory's returned closure — redefining an outer overloading def from
// inside a fn body outlives the call on the interpreter (the drop-then-push
// leaves the frame's def depth unchanged, so DefCleanup pops nothing) where
// the compiled program keeps the outer bake, so the install declines at the
// conditional-redefinition site. A capture-free literal in a fn body and a
// capturing value at the top level are not declined.
func TestInstallDefDoesNotLowerCapturingRedefinitionInFnBody(t *testing.T) {
	r := compileCheckRegistry(t)
	es := newS5BEmit()
	r.Check.Emit = es
	sig := func() Signature { return Signature{Params: []FnParam{{Name: "z", Type: TInteger}}} }
	installDef(r, "p", NewFunction(FnDefInfo{Anonymous: true, Signatures: []Signature{sig()}}), false)
	capturing := NewFunction(FnDefInfo{Anonymous: true, Signatures: []Signature{sig()},
		Captured: []CapturedBinding{{Name: "k", Value: NewCarrier(TInteger)}}})
	r.Check.FnBodyDepth = 1
	installDef(r, "p", capturing, false)
	if len(es.uncompilable) != 1 || !strings.Contains(es.uncompilable[0], "redefined inside a fn body by a capturing fn value") {
		t.Errorf("a capturing redefinition inside a fn body declines: %v", es.uncompilable)
	}
	es.uncompilable = nil
	installDef(r, "p", NewFunction(FnDefInfo{Anonymous: true, Signatures: []Signature{sig()}}), false)
	r.Check.FnBodyDepth = 0
	installDef(r, "p", capturing, false)
	if len(es.uncompilable) != 0 {
		t.Errorf("a capture-free literal in a fn body and a top-level capturing value are not declined: %v", es.uncompilable)
	}
}

// TestInstallDefDoesNotLowerSpecFamilyRedefinitionInFnBody pins installDef's
// fn-body arm for NUR149 (the seventy-third increment): a CAPTURE-FREE
// redefinition inside a fn body of a SPECULATIVE-FAMILY name (a fn a branch
// arm the model could not decide defined — SpecFnNames, the seventieth
// increment) is the family-L leak. The drop-then-push leaves the frame's def
// depth unchanged, so the interpreter keeps the shadow past the call while the
// compiled def lowers to nothing and the family's live-lead dispatch resolves
// the wrong binding; no compiled twin reproduces it, so it declines. The same
// capture-free redefinition of a NON-family name takes the compiled replace
// twin and is not declined.
func TestInstallDefDoesNotLowerSpecFamilyRedefinitionInFnBody(t *testing.T) {
	sig := func() Signature { return Signature{Params: []FnParam{{Name: "z", Type: TInteger}}} }
	lit := func() Value { return NewFunction(FnDefInfo{Anonymous: true, Signatures: []Signature{sig()}}) }

	// A MODULE-scope speculative family (p installed, then the fn baseline
	// snapshotted so p sits AT the baseline) redefined capture-free inside a
	// fn body is the family-L leak — declined.
	r := compileCheckRegistry(t)
	es := newS5BEmit()
	r.Check.Emit = es
	installDef(r, "p", lit(), false)
	r.Check.SpecFnNames = map[string]bool{"p": true}
	r.PushFnBaseline(r.Defs.Snapshot())
	r.Check.FnBodyDepth = 1
	installDef(r, "p", lit(), false)
	if len(es.uncompilable) != 1 || !strings.Contains(es.uncompilable[0], "redefined inside a fn body replaces a module-scope speculative-family overload") {
		t.Errorf("a module-scope spec-family redefinition inside a fn body declines: %v", es.uncompilable)
	}

	// An IN-FUNCTION family (p created INSIDE the fn, above the baseline) is
	// torn down by RET — NOT the leak, so NOT declined (the baseline gate,
	// Codex P2 on #469).
	r2 := compileCheckRegistry(t)
	es2 := newS5BEmit()
	r2.Check.Emit = es2
	r2.PushFnBaseline(r2.Defs.Snapshot()) // p absent at the baseline
	r2.Check.SpecFnNames = map[string]bool{"p": true}
	r2.Check.FnBodyDepth = 1
	installDef(r2, "p", lit(), false) // created in-fn
	installDef(r2, "p", lit(), false) // redefined in-fn
	if len(es2.uncompilable) != 0 {
		t.Errorf("an in-function spec family is not declined (the baseline gate): %v", es2.uncompilable)
	}

	// Not a speculative family: the compiled replace twin agrees, so no compile failure.
	r3 := compileCheckRegistry(t)
	es3 := newS5BEmit()
	r3.Check.Emit = es3
	installDef(r3, "p", lit(), false)
	r3.PushFnBaseline(r3.Defs.Snapshot())
	r3.Check.FnBodyDepth = 1
	installDef(r3, "p", lit(), false)
	if len(es3.uncompilable) != 0 {
		t.Errorf("a non-family capture-free redefinition in a fn body is not declined: %v", es3.uncompilable)
	}

	// specFamilyAtFnBaseline: false with no enclosing baseline, true when the
	// binding sits at/below it, false above it.
	r4 := compileCheckRegistry(t)
	installDef(r4, "p", lit(), false)
	if specFamilyAtFnBaseline(r4, "p") {
		t.Error("no enclosing fn baseline: not baseline-scoped")
	}
	r4.PushFnBaseline(r4.Defs.Snapshot())
	if !specFamilyAtFnBaseline(r4, "p") {
		t.Error("a binding at the baseline is baseline-scoped")
	}
	installDef(r4, "q", lit(), false) // q created above the baseline
	if specFamilyAtFnBaseline(r4, "q") {
		t.Error("a binding above the baseline is not baseline-scoped")
	}
	var nilR *Registry
	if specFamilyAtFnBaseline(nilR, "p") {
		t.Error("a nil registry is not baseline-scoped")
	}
}

// TestStepWordNestedBodySubstitutesCarrier — the substitution fires inside
// a NESTED body too (the thirty-eighth increment): a branch arm / loop body
// / `do` body's read of a name in the side table reads as its fn carrier.
// It used to decline at NestedBodyDepth > 0, which left `if c [(f 2)] [0]`
// reporting a FALSE undefined_word on the plain check and `do [(f 2)]`
// behind the check-diagnostics sentinel.
func TestStepWordNestedBodySubstitutesCarrier(t *testing.T) {
	r := covRegistry(t, nil)
	defer r.Check.Begin()()
	NoteCheckFnCarrierBind(r, "h", NewCarrier(TFunction))
	r.Check.NestedBodyDepth = 1
	defer func() { r.Check.NestedBodyDepth = 0 }()

	e := NewTop(r)
	e.Tape = NewTape([]Value{NewWord("h")}, StackHeadroom)
	if err := e.stepWord(e.Tape.At(0)); err != nil {
		t.Fatalf("nested-body stepWord errored: %v", err)
	}
	if got := e.Tape.At(0); got.Undefined || !IsFnTypedCarrier(got) {
		t.Errorf("a nested body must read the bound fn carrier, got %v", got)
	}
	if len(r.Check.Diagnostics) != 0 {
		t.Errorf("no diagnostics expected, got %v", r.Check.Diagnostics)
	}
	if r.Check.FnCarrierReadSubstituted {
		t.Error("a plain check must not set the compile-only mark")
	}
}
