package core

import (
	"testing"
)

// --- polyNoMatchRaise: runtime drift guards + the raise itself ------------

func pnmSig(barrier int, args ...*Type) Signature {
	return Signature{Args: args, BarrierPos: barrier,
		Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			return nil, nil
		})}
}

// --- the record-time spec derivation ---------------------------------------

func pnmVal(id string, v Value) Value {
	SetID(&v, id)
	return v
}

func TestMapTupleToWindow(t *testing.T) {
	w0 := pnmVal("ida", NewInteger(1))
	w1 := pnmVal("idb", NewString("s"))
	window := []Value{w0, w1}
	if idx, ok := mapTupleToWindow([]Value{w1, w0}, window); !ok || idx[0] != 1 || idx[1] != 0 {
		t.Errorf("mapping = %v/%v, want [1 0]/true", idx, ok)
	}
	if _, ok := mapTupleToWindow([]Value{pnmVal("", NewInteger(1))}, window); ok {
		t.Error("an empty ID must decline")
	}
	if _, ok := mapTupleToWindow([]Value{pnmVal("idz", NewInteger(9))}, window); ok {
		t.Error("an unlocatable value must decline")
	}
	// The same ID twice maps to DISTINCT window slots (used[] bookkeeping)
	// and declines when the window has fewer copies than the tuple.
	dup := pnmVal("ida", NewInteger(1))
	if idx, ok := mapTupleToWindow([]Value{dup, dup}, []Value{w0, pnmVal("ida", NewInteger(1))}); !ok || idx[0] != 0 || idx[1] != 1 {
		t.Errorf("duplicate-ID mapping = %v/%v, want [0 1]/true", idx, ok)
	}
	if _, ok := mapTupleToWindow([]Value{dup, dup}, window); ok {
		t.Error("more tuple copies than window copies must decline")
	}
}

// --- the tape probe + reach bound ------------------------------------------

// pnmEngine builds a top engine over a hand tape with the pointer at the
// failing word.
func pnmEngine(t *testing.T, r *Registry, tape []Value, pointer int) *Engine {
	t.Helper()
	e := NewTop(r)
	e.Tape = NewTape(tape, StackHeadroom)
	e.Pointer = pointer
	return e
}

func TestPolyReachBound(t *testing.T) {
	r := covRegistry(t, nil)
	r.Defs.Push("pnmval", NewInteger(7))
	r.Defs.Push("pnmfn", Value{d: &Datum{Parent: TFunction, Data: &FnDefInfo{Name: "pnmfn"}}})
	r.RegisterNativeFunc(NativeFunc{Name: "pnmnat", Signatures: []Signature{pnmSig(-1, TInteger)}})
	if err := r.Err(); err != nil {
		t.Fatalf("register: %v", err)
	}
	word := NewWord("pnmw")
	cases := []struct {
		name    string
		tape    []Value
		pointer int
		reach   int
		ok      bool
	}{
		{"end bounds the forward walk", []Value{word, NewEnd(), NewInteger(1)}, 0, 0, true},
		{"concrete+carrier count", []Value{word, NewInteger(1), NewCarrier(TInteger)}, 0, 2, true},
		{"def value word counts one", []Value{word, NewWord("pnmval")}, 0, 1, true},
		{"def fn word stops", []Value{word, NewWord("pnmfn"), NewInteger(1)}, 0, 0, true},
		// A `/v`-marked fn word is NO stop — it denotes its REFERENCE value
		// (one claimable datum, NUR050/G12), so the walk counts it and the
		// following token.
		{"def fn word /v counts one", []Value{word, NewWordRef("pnmfn"), NewInteger(1)}, 0, 2, true},
		{"reserved literal counts", []Value{word, NewWord("true")}, 0, 1, true},
		// A REGISTERED word takes the same FnDefInfo-binding stop as a def'd
		// fn — registration pushes the binding Defs.Top catches.
		{"registered word stops", []Value{word, NewWord("pnmnat"), NewInteger(1)}, 0, 0, true},
		{"unbound word unboundable", []Value{word, NewWord("no-such")}, 0, 0, false},
		{"marker unboundable", []Value{word, NewForward(ForwardInfo{})}, 0, 0, false},
		{"deferred expression unboundable", []Value{word, NewParenExpr([]Value{NewInteger(1)})}, 0, 0, false},
		{"prefix values count to the paren", []Value{NewInteger(3), NewOpenParen(), NewInteger(2), word}, 3, 1, true},
		{"prefix end bounds", []Value{NewInteger(3), NewEnd(), NewInteger(2), word}, 3, 1, true},
		{"prefix marker unboundable", []Value{NewForward(ForwardInfo{}), NewInteger(2), word}, 2, 0, false},
		{"prefix word unboundable", []Value{NewWord("pnmval"), NewInteger(2), word}, 2, 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := pnmEngine(t, r, c.tape, c.pointer)
			reach, ok := e.polyReachBound()
			if reach != c.reach || ok != c.ok {
				t.Errorf("reach = %d/%v, want %d/%v", reach, ok, c.reach, c.ok)
			}
		})
	}
}
