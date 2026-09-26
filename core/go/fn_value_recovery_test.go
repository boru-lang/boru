package core

import (
	"errors"
	"testing"
)

// Standalone coverage for the fn-VALUE no-signature recovery
// (Engine.fnValueRecovery): execFnDefLiteral's recovery arm, its
// fnValueNoMatchRecovers gate, the probe's uncalled snapshot and the
// window-arity first-match proof behind PolyNoMatchSpec.Uncalled.

// --- the uncalled probe + spec --------------------------------------------

func TestPolyNoMatchProbeFnValueRecovery(t *testing.T) {
	r := covRegistry(t, nil)
	e := pnmEngine(t, r, []Value{NewCarrier(TAny), NewWord("fvr")}, 1)
	e.fnValueRecovery = true
	p := e.PolyNoMatchProbe("fvr", SrcPos{Row: 3, Col: 7})
	if !p.ok || !p.uncalled || !p.Uncalled() || p.pos.Row != 3 || p.written != nil || p.stackVals != nil {
		t.Fatalf("a fn value's recovery must snapshot only ok+uncalled+pos, got %+v", p)
	}
	e.fnValueRecovery = false
	if p := e.PolyNoMatchProbe("fvr", SrcPos{}); p.Uncalled() {
		t.Error("a word's probe must not be uncalled")
	}
}

func TestPolyNoMatchSpecUncalled(t *testing.T) {
	window := []Value{NewInteger(1), NewString("s")}
	p := polyNoMatchProbe{ok: true, uncalled: true, pos: SrcPos{Row: 4, Col: 1}}
	fn := &FnDefInfo{Signatures: []Signature{pnmSig(-1, TInteger, TString), {Fallback: true}}}
	s := p.Spec(fn, window)
	if s == nil || !s.Uncalled || s.NSigs != 2 || s.Pos.Row != 4 || s.Written != nil || s.StackTuple != nil {
		t.Fatalf("the uncalled spec = %+v, want Uncalled NSigs 2 at row 4 with no tuples", s)
	}
	// A wider overload leaves the window's first match unproven: decline.
	wide := &FnDefInfo{Signatures: []Signature{pnmSig(-1, TInteger, TString), pnmSig(-1, TInteger, TString, TBoolean)}}
	if s := p.Spec(wide, window); s != nil {
		t.Errorf("an unproven first match must decline, got %+v", s)
	}
}

// --- windowArityFirstMatch / narrowOverloadShadowed ------------------------

func TestWindowArityFirstMatch(t *testing.T) {
	m := NewMap(NewOrderedMap())
	window := []Value{NewInteger(1), NewString("s"), m}
	full := pnmSig(-1, TInteger, TString, TMap)
	narrow := pnmSig(-1, TInteger, TString)
	cases := []struct {
		name   string
		sigs   []Signature
		window []Value
		want   bool
	}{
		{"window-arity and fallback only", []Signature{full, {Fallback: true}}, window, true},
		{"a wider overload", []Signature{full, pnmSig(-1, TInteger, TString, TMap, TAny)}, window, false},
		{"a narrower overload with no predecessor", []Signature{narrow, full}, window, false},
		// The mini-s3 shape: the 3-arity overload precedes the 2-arity one,
		// its first two slots conform, and the Map literal fills its third.
		{"a shadowed narrower overload", []Signature{{Fallback: true}, full, narrow}, window, true},
		{"slot types that do not conform", []Signature{pnmSig(-1, TString, TString, TMap), narrow}, window, false},
		{"a nil slot type", []Signature{pnmSig(-1, nil, TString, TMap), narrow}, window, false},
		{"an extra operand that is not concrete", []Signature{full, narrow}, []Value{NewInteger(1), NewString("s"), NewCarrier(TMap)}, false},
		{"an extra operand the wide slot rejects", []Signature{full, narrow}, []Value{NewInteger(1), NewString("s"), NewInteger(2)}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fn := &FnDefInfo{Signatures: c.sigs}
			if got := windowArityFirstMatch(fn, c.window); got != c.want {
				t.Errorf("windowArityFirstMatch = %v, want %v", got, c.want)
			}
		})
	}
}

// --- fnValueNoMatchRecovers + execFnDefLiteral's recovery arm --------------

// fvrModule builds a foreign module registry exporting the native `fvr-send`
// (Integer String, all-stack).
func fvrModule(t *testing.T) *Registry {
	t.Helper()
	mod := covRegistry(t, func(r *Registry) {
		r.RegisterNativeFunc(NativeFunc{Name: "fvr-send", Signatures: []Signature{pnmSig(0, TInteger, TString)}})
	})
	return mod
}

// fvrFnValue is the module native read as a fn value (`Mod.fvr-send`).
func fvrFnValue(mod *Registry, name string) Value {
	return Value{Parent: TFunction, Data: FnDefInfo{Name: name, Registry: mod}}
}

// fvrCompiling arms a compile pass on r and pins the fallback window.
func fvrCompiling(t *testing.T, r *Registry, window []int) {
	t.Helper()
	s5aCheckOn(t, r)
	r.Check.Compiling = true
	t.Cleanup(func() { r.Check.Compiling = false })
	prev := CheckBraid.CheckModeFallbackPositions
	CheckBraid.CheckModeFallbackPositions = func(*Engine, int) []int { return window }
	t.Cleanup(func() { CheckBraid.CheckModeFallbackPositions = prev })
}

func TestFnValueNoMatchRecoversGate(t *testing.T) {
	mod := fvrModule(t)
	r := covRegistry(t, func(r *Registry) { r.Defs.Push("fvr-any", NewCarrier(TAny)) })
	fvrCompiling(t, r, []int{0})
	fnDef := FnDefInfo{Name: "fvr-send", Registry: mod}
	fn := &FnDefInfo{Name: "fvr-send", Signatures: []Signature{pnmSig(0, TInteger, TString), pnmSig(0, TInteger)}}
	eng := func(operand Value) *Engine {
		return pnmEngine(t, r, []Value{operand, fvrFnValue(mod, "fvr-send")}, 1)
	}

	// Each unknown-type operand recovers: a strict Any, a gradual (dynamic)
	// carrier, a union carrier, and a word whose binding is an Any carrier.
	dyn := NewCarrier(TInteger)
	dyn.Dynamic = true
	for name, operand := range map[string]Value{
		"strict Any":      NewCarrier(TAny),
		"gradual carrier": dyn,
		"union carrier":   NewCarrier(TDisjunct),
		"bound word":      NewWord("fvr-any"),
	} {
		if !eng(operand).fnValueNoMatchRecovers(1, fnDef, fn) {
			t.Errorf("%s: an unknown-type operand must recover", name)
		}
	}
	// Definite operands (a concrete value, a definite carrier, an unbound
	// word) are a definite mismatch: the value parks, no recovery.
	for name, operand := range map[string]Value{
		"concrete":         NewInteger(1),
		"definite carrier": NewCarrier(TString),
		"unbound word":     NewWord("fvr-unbound"),
	} {
		if eng(operand).fnValueNoMatchRecovers(1, fnDef, fn) {
			t.Errorf("%s: a definite operand must not recover", name)
		}
	}

	// A foreign name that is not a registered native there declines.
	local := FnDefInfo{Name: "fvr-local", Registry: mod}
	if eng(NewCarrier(TAny)).fnValueNoMatchRecovers(1, local, fn) {
		t.Error("a non-native foreign fn must not recover")
	}
	// An at-home fn value declines at the first gate.
	if eng(NewCarrier(TAny)).fnValueNoMatchRecovers(1, FnDefInfo{Name: "fvr-send", Registry: r}, fn) {
		t.Error("an at-home fn value must not recover")
	}
	// An overload no poly re-match can drive declines: a callable spec, a
	// no-eval slot, a quoted slot, a code body.
	callable := pnmSig(0, TInteger)
	callable.Callable = &CallableSpec{}
	noEval := pnmSig(0, TInteger)
	noEval.NoEvalArgs = map[int]bool{0: true}
	quoted := pnmSig(0, TInteger)
	quoted.QuoteArgs = map[int]bool{0: true}
	bodied := pnmSig(0, TInteger)
	bodied.Impl = &BoruImpl{Body: []Value{NewInteger(1)}}
	for name, s := range map[string]Signature{"callable": callable, "no-eval": noEval, "quoted": quoted, "body": bodied} {
		bad := &FnDefInfo{Name: "fvr-send", Signatures: []Signature{pnmSig(0, TInteger, TString), s}}
		if eng(NewCarrier(TAny)).fnValueNoMatchRecovers(1, fnDef, bad) {
			t.Errorf("%s: an undrivable overload must not recover", name)
		}
	}
}

func TestExecFnDefLiteralFnValueRecovery(t *testing.T) {
	mod := fvrModule(t)
	r := covRegistry(t, nil)
	fvrCompiling(t, r, []int{0})
	sentinel := errors.New("fvr assume-sig")
	var (
		sawRecovery bool
		sawReg      *Registry
		sawPos      SrcPos
	)
	prev := CheckBraid.CheckModeAssumeSig
	CheckBraid.CheckModeAssumeSig = func(e *Engine, _ WordInfo, fn *FnDefInfo, _ *Signature, pos SrcPos) error {
		sawRecovery, sawReg, sawPos = e.fnValueRecovery, fn.Registry, pos
		return sentinel
	}
	t.Cleanup(func() { CheckBraid.CheckModeAssumeSig = prev })

	operand := NewCarrier(TAny)
	operand.SetPos(SrcPos{Row: 6, Col: 2})
	e := pnmEngine(t, r, []Value{operand, fvrFnValue(mod, "fvr-send")}, 1)
	if err := e.execFnDefLiteral(1); !errors.Is(err, sentinel) {
		t.Fatalf("the recovery must return the assume-sig result, got %v", err)
	}
	if !sawRecovery || e.fnValueRecovery {
		t.Errorf("fnValueRecovery must be set for the assume-sig call only (during %v, after %v)", sawRecovery, e.fnValueRecovery)
	}
	if sawReg != mod {
		t.Errorf("the recovered fn must run on its home module registry, got %p want %p", sawReg, mod)
	}
	if sawPos.Row != 6 {
		t.Errorf("the recovery pos must borrow the operand's span, got %+v", sawPos)
	}
}
