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

// TestStepWordValCarrierKeepsUndefinedDiag — the `/v` read path must NOT
// consult the side table, even on a compile pass with the name noted:
// substituting there green-lights units whose lowering drops the operand
// (the pmany/pseq miscompile — see stepWordVal). The diagnostic keeps
// those units refused.
func TestStepWordValCarrierKeepsUndefinedDiag(t *testing.T) {
	r := compileCheckRegistry(t)
	NoteCheckFnCarrierBind(r, "hv", NewCarrier(TFunction))

	e := NewTop(r)
	e.Tape = NewTape([]Value{NewWord("hv")}, StackHeadroom)
	if err := e.stepWordVal(e.Tape.At(0), WordInfo{Name: "hv", ArgCount: -1, ForceVal: true}); err != nil {
		t.Fatalf("/v step errored: %v", err)
	}
	if got := e.Tape.At(0); !got.Undefined {
		t.Errorf("a /v read of a carrier-bound name must keep the Undefined placeholder: %v", got)
	}
	if len(r.Check.Diagnostics) != 1 {
		t.Errorf("expected the one undefined_word diagnostic, got %v", r.Check.Diagnostics)
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

// TestInstallDefRefusesCapturingRedefinitionInFnBody pins installDef's
// fn-body arm (the thirty-first increment): a CAPTURING fn value — a
// factory's returned closure — redefining an outer overloading def from
// inside a fn body outlives the call on the interpreter (the drop-then-push
// leaves the frame's def depth unchanged, so DefCleanup pops nothing) where
// the compiled program keeps the outer bake, so the install refuses at the
// conditional-redefinition site. A capture-free literal in a fn body and a
// capturing value at the top level are not refused.
func TestInstallDefRefusesCapturingRedefinitionInFnBody(t *testing.T) {
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
		t.Errorf("a capturing redefinition inside a fn body refuses: %v", es.uncompilable)
	}
	es.uncompilable = nil
	installDef(r, "p", NewFunction(FnDefInfo{Anonymous: true, Signatures: []Signature{sig()}}), false)
	r.Check.FnBodyDepth = 0
	installDef(r, "p", capturing, false)
	if len(es.uncompilable) != 0 {
		t.Errorf("a capture-free literal in a fn body and a top-level capturing value are not refused: %v", es.uncompilable)
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
