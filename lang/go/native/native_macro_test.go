package native

import (
	"testing"

	parser "github.com/boru-lang/boru/parser/go"
)

// Phase 1b (design/legacy/MACROS-PHASE1.10.ignore §3): FormArgs raw-form capture, tested in
// isolation with a temp native that records its operands. The macro definer
// (1c) reuses this; here we prove the capture mechanism independently.
//
// A FormArgs (+ NoEvalArgs + NoEvalMapArgs) param captures the next operand as
// a raw FORM: a bare word stays a Word (no resolution / dispatch / undefined
// error / Atom coercion); a paren stays a ParenExpr (not pre-evaluated); a list
// stays an un-evaluated List; a map keeps its values un-evaluated.
func TestFormArgsRawCapture(t *testing.T) {
	r := defaultRegistry(t)

	var captured []Value
	r.RegisterNativeFunc(NativeFunc{
		Name: "__formecho",
		Signatures: []Signature{{
			Args:          []*Type{TAny, TAny, TAny, TAny},
			FormArgs:      map[int]bool{0: true, 1: true, 2: true, 3: true},
			NoEvalArgs:    map[int]bool{0: true, 1: true, 2: true, 3: true},
			NoEvalMapArgs: map[int]bool{0: true, 1: true, 2: true, 3: true},
			Impl: Go(func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				captured = append([]Value(nil), args...)
				return []Value{NewBoolean(true)}, nil
			}),
			BarrierPos: -1,
		}},
	})

	// `foo` and `[x y]` and `{k:(add 1 2)}` reference undefined words — if
	// anything were evaluated, this would raise undefined_word.
	toks, err := parser.Parse(`__formecho foo (add 1 2) [x y] {k: (add 3 4)}`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if _, err := NewTop(r).Run(toks); err != nil {
		t.Fatalf("run (nothing should evaluate): %v", err)
	}
	if len(captured) != 4 {
		t.Fatalf("expected 4 captured operands, got %d: %v", len(captured), captured)
	}

	// 0: a bare word stays a Word (not dispatched, not an Atom).
	if !IsWord(captured[0]) {
		t.Errorf("operand 0: want a raw Word, got %s (%v)", captured[0].Parent, captured[0])
	} else if w, _ := AsWord(captured[0]); w.Name != "foo" {
		t.Errorf("operand 0: want Word(foo), got Word(%s)", w.Name)
	}
	// 1: a paren stays an un-evaluated ParenExpr (not 5).
	if !IsParenExpr(captured[1]) {
		t.Errorf("operand 1: want a raw ParenExpr, got %s (%v)", captured[1].Parent, captured[1])
	}
	// 2: a list stays an un-evaluated List with raw word elements.
	if !captured[2].Parent.Equal(TList) || !IsConcrete(captured[2]) {
		t.Fatalf("operand 2: want a concrete List, got %s (%v)", captured[2].Parent, captured[2])
	}
	l, _ := AsList(captured[2])
	if l.Len() != 2 || !IsWord(l.Get(0)) {
		t.Errorf("operand 2: want [Word(x) Word(y)] un-evaluated, got %v", captured[2])
	}
	// 3: a map keeps its value un-evaluated (a ParenExpr, not 7).
	if !captured[3].Parent.Equal(TMap) || !IsConcrete(captured[3]) {
		t.Fatalf("operand 3: want a concrete Map, got %s (%v)", captured[3].Parent, captured[3])
	}
	m, _ := AsMap(captured[3])
	kv, _ := m.Get("k")
	if !IsParenExpr(kv) {
		t.Errorf("operand 3: map value k should be a raw ParenExpr, got %s (%v)", kv.Parent, kv)
	}
}
