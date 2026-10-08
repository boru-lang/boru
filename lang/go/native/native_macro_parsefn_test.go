package native

import (
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// Coverage for the `parse` macro's ParseLang-value form (parseFnExpand) —
// the arms a surface program cannot reach through sig dispatch: the matcher
// declines a bare Function type literal for a TFunction slot, and a carrier
// operand only arrives under analysis. Driven directly, mirroring the W9
// macro seams. See design/TEST-SEAMS.10.md.

// A fn-family value whose payload is not an FnDefInfo is declined: the sig
// matcher never delivers one from surface syntax (a bare `Function` type
// literal is parented at Type and declines every parse sig — pinned by the
// module-parselang.tsv §10 signature_error row), so the defensive guard is
// driven directly with a crafted payload, like the eng fn-value seams.
func TestParseFnExpandNonFnPayload(t *testing.T) {
	r := seam5Reg(t)
	_, err := parseHandler([]Value{NewValueRaw(TFunction, IntPayload{N: 1}), NewString("x")}, nil, nil, r)
	if err == nil {
		t.Fatal("parse: a non-FnDefInfo function payload must error")
	}
}

// A TFunction CARRIER operand (a computed parser under analysis) degrades to
// a dynamic value with the macro advisory instead of erroring.
func TestParseFnExpandCarrier(t *testing.T) {
	r := seam5Reg(t)
	cleanup := r.Check.Begin()
	defer cleanup()
	out, err := parseHandler([]Value{NewCarrier(TFunction), NewString("x")}, nil, nil, r)
	if err != nil {
		t.Fatalf("parse: a carrier parser under analysis must degrade, got %v", err)
	}
	if len(out) != 1 || !out[0].Dynamic {
		t.Fatalf("parse: carrier parser should yield a dynamic carrier, got %v", Canon(out))
	}
}

// A bare word BOUND to a TFunction carrier (a fn param used as the parser)
// takes the name-path carrier arm: degrade + advisory, and the binding is
// recorded as used so unused_def stays quiet.
func TestParseFnNameBoundCarrier(t *testing.T) {
	r := seam5Reg(t)
	cleanup := r.Check.Begin()
	defer cleanup()
	r.Defs.Push("p", NewCarrier(TFunction))
	out, err := parseHandler([]Value{NewAtom("p"), NewString("x")}, nil, nil, r)
	if err != nil {
		t.Fatalf("parse: a carrier fn binding under analysis must degrade, got %v", err)
	}
	if len(out) != 1 || !out[0].Dynamic {
		t.Fatalf("parse: carrier fn binding should yield a dynamic carrier, got %v", Canon(out))
	}
	if !r.Check.DefsUsed["p"] {
		t.Fatal("parse: the parser binding must be recorded as used")
	}
}

// TestCheckFnCarrierBindTable drives the per-pass fn-carrier side table's
// arms directly: nil/absent guards, the create and reuse write arms, hit and
// miss lookups, and the per-pass reset. (The corpus covers the production
// flow — `def op (Parse.parser g)  parse op '…'` — end to end.)
func TestCheckFnCarrierBindTable(t *testing.T) {
	ResetCheckFnCarrierBinds(nil)
	r := seam5Reg(t)
	ResetCheckFnCarrierBinds(r) // absent table: no-op
	if _, hit := checkFnCarrierBind(r, "x"); hit {
		t.Fatal("empty table must miss")
	}
	noteCheckFnCarrierBind(r, "a", NewCarrier(TFunction)) // create arm
	noteCheckFnCarrierBind(r, "b", NewCarrier(TFunction)) // reuse arm
	if _, hit := checkFnCarrierBind(r, "a"); !hit {
		t.Fatal("noted name must resolve")
	}
	if _, hit := checkFnCarrierBind(r, "zz"); hit {
		t.Fatal("an un-noted name must miss")
	}
	ResetCheckFnCarrierBinds(r)
	if _, hit := checkFnCarrierBind(r, "a"); hit {
		t.Fatal("reset must clear the table")
	}
}

// --- recordParseLangFnDispatch (compile-recorder arms; mirrors the W9
// recordDeferredParseDispatch tests) ---

func TestParseFnDispatchInstallGuard(t *testing.T) {
	// Nil registry / nil sig are no-ops (no panic) — mirror of the
	// InstallParseDeferredDispatch guard test.
	InstallParseLangFnDispatch(nil, nil)
	r := seam5Reg(t)
	InstallParseLangFnDispatch(r, nil)
}

func TestParseFnDispatchRecordNoDispatcher(t *testing.T) {
	r := seam5Reg(t)
	cleanup := r.Check.Begin()
	defer cleanup()
	r.Check.Emit = NewEmitState()
	r.Check.Compiling = true
	fn := NewCarrier(TFunction)
	// No dispatcher installed → the sig lookup fails and recording declines.
	if _, ok := recordParseLangFnDispatch(r, fn, []Value{fn, NewString("x")}); ok {
		t.Fatal("recordParseLangFnDispatch must decline without an installed dispatcher")
	}
}

func TestParseFnDispatchRecordThreeArgs(t *testing.T) {
	r := seam5Reg(t)
	cleanup := r.Check.Begin()
	defer cleanup()
	r.Check.Emit = NewEmitState()
	r.Check.Compiling = true
	InstallParseLangFnDispatch(r, &Signature{
		Args:       []*Type{TAny, TAny, TMap},
		Returns:    []*Type{TAny},
		BarrierPos: -1,
	})
	fn := NewCarrier(TFunction)
	// Three args → the opts-middle / source-last split branch.
	out, ok := recordParseLangFnDispatch(r, fn,
		[]Value{fn, NewMap(NewOrderedMap()), NewString("x + y")})
	if !ok {
		t.Fatal("recordParseLangFnDispatch should record with a dispatcher installed")
	}
	if !out.Dynamic {
		t.Fatalf("fn-dispatch result should be a dynamic carrier, got %s", out.String())
	}
}

// --- the emit / mini fn-dispatch twins (recordMacroFnDispatch) and the
// lead / fn-dispatch install guards ---

// TestMacroDispatchInstallGuards: a nil registry or a nil dispatcher installs
// nothing and never panics — for the parse LEAD dispatch as for the emit /
// mini fn dispatch — so the recorder then finds no dispatcher to record.
func TestMacroDispatchInstallGuards(t *testing.T) {
	InstallParseLangLeadDispatch(nil, nil)
	InstallEmitLangFnDispatch(nil, nil)
	InstallMiniLangFnDispatch(nil, nil)
	r := seam5Reg(t)
	InstallParseLangLeadDispatch(r, nil)
	InstallEmitLangFnDispatch(r, nil)
	InstallMiniLangFnDispatch(r, nil)
	if _, ok, _ := core.Cap[*Signature](r, capParseLangLeadDispatch); ok {
		t.Error("a nil lead dispatcher must not be installed")
	}
	for _, key := range []string{capEmitLangFnDispatch, capMiniLangFnDispatch} {
		if _, ok, _ := core.Cap[*FnDefInfo](r, key); ok {
			t.Errorf("%s: a nil fn dispatcher must not be installed", key)
		}
	}
}

// macroDispatchFn is a stand-in for boru:minilang's minilang-fn-dispatch: one
// Any-typed signature per surface arity (2 and 3), the lead read as data.
func macroDispatchFn() *FnDefInfo {
	sig := func(n int) Signature {
		args := make([]*Type, n)
		for i := range args {
			args[i] = TAny
		}
		return Signature{
			Args: args, Returns: []*Type{TAny}, BarrierPos: -1,
			FnDataArgs: map[int]bool{0: true}, FnInertArgs: map[int]bool{0: true},
			Impl: Go(func(a []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return a[:1], nil
			}),
		}
	}
	return &FnDefInfo{Name: "minilang-fn-dispatch", Signatures: []Signature{sig(2), sig(3)}}
}

// TestMacroFnDispatchRecordDeclines: the record declines, leaving the macro
// on its degrade, when no dispatcher is installed — the module that owns it
// was not imported, so a compiled program has no runtime resolver to call
// (`emit m.up {a:1}` without boru:emitlang interprets through the value
// form, and the compile declines rather than guessing) — and for an arity
// no dispatcher signature takes.
func TestMacroFnDispatchRecordDeclines(t *testing.T) {
	r := seam5Reg(t)
	defer r.Check.BeginCompilePass()()
	lead := NewCarrier(TFunction)
	if _, ok := recordMacroFnDispatch(r, capEmitLangFnDispatch, "emitlang-fn-dispatch",
		[]Value{lead, NewMap(NewOrderedMap())}); ok {
		t.Error("emit: with no dispatcher installed the record must decline")
	}
	InstallMiniLangFnDispatch(r, macroDispatchFn())
	if _, ok := recordMacroFnDispatch(r, capMiniLangFnDispatch, "minilang-fn-dispatch",
		[]Value{lead, NewString("src"), NewMap(NewOrderedMap()), NewString("extra")}); ok {
		t.Error("mini: an arity no dispatcher signature takes must decline")
	}
}

// TestMacroFnDispatchRecordAnchorsWithoutAWordPosition: the dispatch raises
// what the macro WORD raises, so its event is stamped at the word
// (CheckState.CurWordPos); with no word position to read, the event anchors
// at the last surface operand — the data / source written at the call — so
// a runtime raise still carries a real location, never 0:0.
func TestMacroFnDispatchRecordAnchorsWithoutAWordPosition(t *testing.T) {
	r := seam5Reg(t)
	defer r.Check.BeginCompilePass()()
	InstallMiniLangFnDispatch(r, macroDispatchFn())
	r.Check.CurWordPos = SrcPos{}
	at := SrcPos{Row: 3, Col: 7}
	src := core.WithPosAt(NewString("ab"), at)
	out, ok := recordMacroFnDispatch(r, capMiniLangFnDispatch, "minilang-fn-dispatch",
		[]Value{NewString("lead"), src})
	if !ok {
		t.Fatal("mini: an installed dispatcher over a surface arity must record")
	}
	es, isES := r.Check.Recorder().(*compiler.EmitState)
	if !isES {
		t.Fatal("a compile pass records into the compiler's EmitState")
	}
	prog, reason, fin := es.Finalize([]Value{out})
	if !fin {
		t.Fatalf("the recorded dispatch must finalize, got %q", reason)
	}
	found := false
	for pc, in := range prog.Code {
		if in.Op != compiler.OpCallNative || prog.Sigs[in.Arg].Word != "minilang-fn-dispatch" {
			continue
		}
		found = true
		if got := prog.Debug[pc]; got.Row != at.Row || got.Col != at.Col {
			t.Errorf("the dispatch is stamped at %d:%d, want the operand's %d:%d", got.Row, got.Col, at.Row, at.Col)
		}
	}
	if !found {
		t.Fatalf("no minilang-fn-dispatch call in the program:\n%s", prog.Disassemble())
	}
}

// Outside check mode a non-concrete fn-family binding is NOT a parser: the
// name path falls through to the ordinary unknown-kind error.
func TestParseFnNameBoundTypeLiteralFallsThrough(t *testing.T) {
	r := seam5Reg(t)
	r.Defs.Push("p", NewTypeLiteral(TFunction))
	if _, err := parseHandler([]Value{NewAtom("p"), NewString("x")}, nil, nil, r); err == nil {
		t.Fatal("parse: a Function type-literal binding must fall through to unknown-kind")
	}
}
