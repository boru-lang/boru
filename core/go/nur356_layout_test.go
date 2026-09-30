package core

import "testing"

// nur356_layout_test.go pins NUR356's additions to the dispatch layout: a
// written operand the pass read from a data word is published with the word
// the tape still holds (DispatchLayout.Words), and a word past every forward
// position the word's overloads reach closes the After run.

func nur356LayoutEngine(t *testing.T, tape []Value) *Engine {
	t.Helper()
	e := layoutEngine(t, tape, 0)
	e.Registry.Register("ea", Signature{Args: []*Type{TFunction, TList}, BarrierPos: -1})
	e.Registry.Register("eb", Signature{Args: []*Type{TFunction, TList}, BarrierPos: -1}, Signature{Args: []*Type{TFunction, TList, TInteger}, BarrierPos: -1})
	return e
}

func TestDispatchLayoutWordReads(t *testing.T) {
	three, five := withID(NewInteger(3)), withID(NewInteger(5))
	l := NewWord("l")
	e := nur356LayoutEngine(t, []Value{NewWord("ea"), three, l})
	e.Registry.Defs.Push("l", five)
	args := []Value{three, five}
	restore := e.PublishLayout(args, []int{1, 2}, SrcPos{})
	if got := e.Registry.Check.LayoutFor(args); got == nil || got.NFwd != 2 || !IsWord(got.Words[1]) || len(got.Words) != 1 {
		t.Errorf("ea 3 l: the word read rides as written; got %+v", got)
	}
	restore()

	for name, prep := range map[string]func(e *Engine) []Value{
		"a word bound to a fn": func(e *Engine) []Value {
			fn := withID(NewFunction(FnDefInfo{Name: "k", Signatures: []Signature{{}}}))
			e.Registry.Defs.Push("l", fn)
			return []Value{three, fn}
		},
		"an unbound word": func(e *Engine) []Value { return []Value{three, five} },
		"a gradual binding": func(e *Engine) []Value {
			g := withID(NewDynamicCarrier(TInteger))
			e.Registry.Defs.Push("l", g)
			return []Value{three, g}
		},
	} {
		e := nur356LayoutEngine(t, []Value{NewWord("ea"), three, l})
		args := prep(e)
		restore := e.PublishLayout(args, []int{1, 2}, SrcPos{})
		if e.Registry.Check.CurLayout != nil {
			t.Errorf("%s: no layout", name)
		}
		restore()
	}
	// A `/v` read, and a word beneath the word (a stack operand), publish none.
	e = nur356LayoutEngine(t, []Value{NewWord("ea"), three, NewValueRaw(TWord, WordInfo{Name: "l", ArgCount: -1, ForceVal: true})})
	e.Registry.Defs.Push("l", five)
	restore = e.PublishLayout([]Value{three, five}, []int{1, 2}, SrcPos{})
	if e.Registry.Check.CurLayout != nil {
		t.Error("a /v read")
	}
	restore()
	e = nur356LayoutEngine(t, []Value{l, NewWord("ea"), three})
	e.Pointer = 1
	e.Registry.Defs.Push("l", five)
	restore = e.PublishLayout([]Value{three, five}, []int{2, 0}, SrcPos{})
	if e.Registry.Check.CurLayout != nil {
		t.Error("a word beneath the word")
	}
	restore()
}

func TestDispatchLayoutWordPastReach(t *testing.T) {
	three, list := withID(NewInteger(3)), withID(NewList([]Value{NewInteger(1)}))
	m := NewWord("m")
	e := nur356LayoutEngine(t, []Value{NewWord("ea"), three, list, m})
	args := []Value{three, list}
	restore := e.PublishLayout(args, []int{1, 2}, SrcPos{})
	if got := e.Registry.Check.LayoutFor(args); got == nil || len(got.After) != 1 || !IsWord(got.After[0]) {
		t.Errorf("ea 3 [1] m: m stands past every reach; got %+v", got)
	}
	restore()
	// eb's wider overload reaches the word: no layout.
	e = nur356LayoutEngine(t, []Value{NewWord("eb"), three, list, m})
	restore = e.PublishLayout(args, []int{1, 2}, SrcPos{})
	if e.Registry.Check.CurLayout != nil {
		t.Error("a word a wider overload reaches")
	}
	restore()
	if _, ok := nur356LayoutEngine(t, []Value{NewInteger(1)}).forwardReachMax(); ok {
		t.Error("no word at the pointer")
	}
	if _, ok := nur356LayoutEngine(t, []Value{NewWord("nope")}).forwardReachMax(); ok {
		t.Error("a word no registry holds")
	}
	if n, ok := nur356LayoutEngine(t, []Value{NewWordModified("eb", -1, true, false)}).forwardReachMax(); !ok || n != 0 {
		t.Errorf("a forced stack read reaches nothing: %d %v", n, ok)
	}
	// The all-forward sentinel reaches the signature's arity.
	e = nur356LayoutEngine(t, []Value{NewWord("ea")})
	e.Registry.Lookup("ea").Signatures[0].BarrierPos = BarrierAllForward
	if n, ok := e.forwardReachMax(); !ok || n != 2 {
		t.Errorf("an all-forward barrier: %d %v", n, ok)
	}
}

func TestIsPendingLiteral(t *testing.T) {
	lit := NewList([]Value{NewWord("x")})
	lit.Eval = true
	quoted := lit
	quoted.Quoted = true
	m := NewMap(NewOrderedMap())
	m.Eval = true
	if !IsPendingLiteral(lit) || !IsPendingLiteral(m) || IsPendingLiteral(quoted) || IsPendingLiteral(NewDynamicCarrier(TList)) || IsPendingLiteral(NewInteger(1)) {
		t.Error("IsPendingLiteral")
	}
}
