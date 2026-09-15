package core

import (
	"fmt"
	"strings"
	"testing"
)

// NoMatchOverWindow is sigError's derivation over a routed dispatch's
// window (the VM's no-match raise, eng/go/vm_generic.go): the failing tuple
// is the unclaimed forward tokens after the word in source order when there
// are any, else the stack prefix top-first, and the reorder probe runs over
// the forward view first and the prefix view second. Pinned here, by core's
// own suite, because only the VM calls it — the engine's sigError takes the
// same steps over its live tape — and the standalone core gate (ADR-008,
// `make cover-gate-core`) counts core's tests alone.
func TestNoMatchOverWindow(t *testing.T) {
	fn := &FnDefInfo{Name: "w", Signatures: []Signature{{
		Args: []*Type{TInteger, TString}, BarrierPos: 2, Impl: Boru([]Value{NewWord("a")}),
	}}}
	pos := SrcPos{Row: 3, Col: 5}
	x, one := NewString("x"), NewInteger(1)
	cases := []struct {
		name    string
		tape    []Value
		pointer int
		written []Value // the failing tuple the diagnostic names
		hinted  bool    // a permutation of some view matches: the reorder hint is set
	}{
		// `w "x" 1`: the forward view, in source order; `w 1 "x"` would match.
		{"forward tokens, hint from the forward view", []Value{NewWord("w"), x, one}, 0, []Value{x, one}, true},
		// `1 "x" w`: no forward token, so the stack prefix top-first; the
		// hint comes from the same view.
		{"no forward token: the prefix, top-first", []Value{one, x, NewWord("w")}, 2, []Value{x, one}, true},
		// `1 "x" w true`: one forward token (too few for any reorder), the
		// hint comes from the prefix view.
		{"forward tokens, hint from the prefix view", []Value{one, x, NewWord("w"), NewBoolean(true)}, 2, []Value{NewBoolean(true)}, true},
		// `w "x" "y"`: no permutation matches either view — no hint.
		{"no reorder on either view", []Value{NewWord("w"), x, NewString("y")}, 0, []Value{x, NewString("y")}, false},
	}
	for _, c := range cases {
		win := NewTape(c.tape, StackHeadroom)
		got := NoMatchOverWindow("src", win, c.pointer, "w", fn, pos)
		if got == nil || got.Code != "signature_error" || got.Row != pos.Row || got.Col != pos.Col {
			t.Fatalf("%s: want signature_error at %d:%d, got %v", c.name, pos.Row, pos.Col, got)
		}
		// The hint the window owes: the written view's, else the prefix's.
		hint := ReorderHintFor("w", fn, c.written)
		if hint == "" {
			hint = ReorderHintFor("w", fn, ReorderCandidates(win.Prefix(c.pointer)))
		}
		if (hint != "") != c.hinted {
			t.Fatalf("%s: the case expects hinted=%v, the views give %q", c.name, c.hinted, hint)
		}
		want := NoMatchDiag("src", "w", fn, c.written, pos, hint)
		if got.Error() != want.Error() || fmt.Sprint(got.Notes) != fmt.Sprint(want.Notes) || fmt.Sprint(got.Suggestions) != fmt.Sprint(want.Suggestions) {
			t.Errorf("%s: the diagnostic is sigError's over the window:\n  got  %v | %v | %v\n  want %v | %v | %v", c.name, got, got.Notes, got.Suggestions, want, want.Notes, want.Suggestions)
		}
		suggested := false
		for _, sg := range got.Suggestions {
			if sg.Message == hint && hint != "" {
				suggested = true
			}
		}
		if c.hinted && !suggested {
			t.Errorf("%s: a matching permutation exists, the reorder suggestion is owed: %v", c.name, got.Suggestions)
		}
		for _, v := range c.written {
			if !strings.Contains(got.Error()+strings.Join(got.Notes, " "), v.String()) {
				t.Errorf("%s: the failing tuple names %s: %v %v", c.name, v, got, got.Notes)
			}
		}
	}
}
