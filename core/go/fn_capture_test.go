package core

import "testing"

// The leaf rule sees a binding word through its VALUE: a name bound to the
// word's own dispatch table (`def/v` keeps the word's name), to a trivial-
// delegation wrapper of one, or to a word-extension clone of one, makes a
// body need its frame state exactly as the literal word would (Codex P1
// on #532 — the CallBoru path's unconditional snapshots used to cover it).
func TestBodyNeedsFrameStateSeesBindingWordAliases(t *testing.T) {
	r := newTestRegistry(t)
	goImpl := Go(func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) { return nil, nil })
	cases := []struct {
		name  string
		value Value
		want  bool
	}{
		{"the word's own table as a value", NewFunction(FnDefInfo{Name: "def", Signatures: []Signature{{Impl: goImpl, BarrierPos: 0}}}), true},
		{"a trivial-delegation wrapper", NewFunction(FnDefInfo{Name: "w", Signatures: []Signature{{Impl: Boru([]Value{NewWord("var")}), BarrierPos: 0}}}), true},
		{"a word-extension clone", NewFunction(FnDefInfo{Name: "c", Extends: "undef", Signatures: []Signature{{Impl: goImpl, BarrierPos: 0}}}), true},
		{"an ordinary fn value", NewFunction(FnDefInfo{Name: "g", Signatures: []Signature{{Impl: Boru([]Value{NewWord("cadd")}), BarrierPos: 0}}}), false},
		{"a wrapper of an ordinary word", NewFunction(FnDefInfo{Name: "w2", Signatures: []Signature{{Params: []FnParam{{Name: "x"}}, Impl: Boru([]Value{NewWord("var")}), BarrierPos: 0}}}), false},
		{"a native table that is no binding word", NewFunction(FnDefInfo{Name: "plain", Signatures: []Signature{{Impl: goImpl, BarrierPos: 0}}}), false},
		{"a value with no signatures", NewFunction(FnDefInfo{Name: "empty"}), false},
	}
	// The real alias shape: `def mydef def/v` resolves the word's table
	// through ResolveRef and installs it under the new name, which keeps the
	// word's identity token and takes the alias's name. Here with `do`.
	r.RegisterNativeFunc(NativeFunc{
		Name:       "do",
		Signatures: []Signature{{Impl: goImpl, Returns: []*Type{}, BarrierPos: 0}},
	})
	ref, ok := ResolveRef(r, "do")
	if !ok {
		t.Fatal("ResolveRef(do) failed")
	}
	renamed, _ := ref.Data.(FnDefInfo)
	renamed.Name = "mydo"
	cases = append(cases,
		struct {
			name  string
			value Value
			want  bool
		}{"the word's table under another name (def mydo do/v)", NewFunction(renamed), true},
	)
	for _, tc := range cases {
		r.Defs.Push("alias$"+tc.name, tc.value)
		got := bodyNeedsFrameState(r, []Value{NewWord("alias$" + tc.name), NewInteger(1)})
		r.Defs.Pop("alias$" + tc.name)
		if got != tc.want {
			t.Errorf("%s: bodyNeedsFrameState = %v, want %v", tc.name, got, tc.want)
		}
	}
}
