package lang

import (
	"strings"
	"testing"
)

// region_collect_test.go is the whole-program half of the region COLLECT
// (NUR067's consuming half): a list literal whose one element is a paren over
// a runtime-variadic region — `[(for 3 [i])]`, `size [(await {mode:'any'}
// [[7 8]])]`.
//
// OpMakeList cannot build it: its Arg is a static element count, and the run's
// length is a runtime value. The lowering opens a mark before the region's
// producing event and closes with MAKE_LIST_TO_MARK, whose element count IS
// the mark.
//
// `size [(await {mode:'any'} [[7 8]])]` is the row NUR067 was opened on: its
// one-seat layout compiled a stranded 7 and a 1-element list where the
// interpreter answers 2. It is pinned here at the answer AND at the emitted
// stream, so a lowering that happened to agree by another route would still
// have to say so.

func TestRegionCollectCompilesWithParity(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		// The run collected into a List, and its size.
		{`[(for 3 [i])]`, "[[0 1 2]]"},
		{`size [(for 3 [i])]`, "[3]"},
		// ZERO iterations: the empty List, not an underflow.
		{`[(for 0 [i])]`, "[[]]"},
		// A computed body — the elements are the body's per-iteration values.
		{`[(for 2 [(1 add 2)])]`, "[[3 3]]"},
		// The collected List is an ordinary single value: it promotes to a
		// frame slot and re-pushes per reference like any other.
		{`def xs [(for 3 [i])] xs`, "[[0 1 2]]"},
		{`def xs [(for 3 [i])] (size xs) add (size xs)`, "[6]"},
		{`[(for 3 [i])] get 1`, "[1]"},
		// NUR067's own miscompile row.
		{`import "boru:time-util" size [(TimeUtil.await {mode:"any"} [[7 8]])]`, "[2]"},
		{`import "boru:time-util" [(TimeUtil.await {mode:"any"} [[7 8]])]`, "[[7 8]]"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			got, compiled, err := rpRun(t, tc.src)
			if err != nil {
				t.Fatalf("RunCompiled: %v", err)
			}
			if !compiled {
				t.Fatal("a list literal collecting a region must compile")
			}
			if got != tc.want {
				t.Errorf("compiled %s, want %s", got, tc.want)
			}
			if in := rpInterp(t, tc.src); in != tc.want {
				t.Errorf("interpreter %s, want %s — the oracle moved", in, tc.want)
			}
		})
	}
}

func TestRegionCollectEmitsTheMarkAndCollect(t *testing.T) {
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	prog, reason, _, err := a.CompileCheck(`size [(for 3 [i])]`)
	if err != nil {
		t.Fatalf("CompileCheck: %v", err)
	}
	if prog == nil {
		t.Fatalf("refused: %s", reason)
	}
	dis := prog.Disassemble()
	if !strings.HasPrefix(dis, "0000 STACK_MARK") {
		t.Errorf("the mark must open before the loop's own operands:\n%s", dis)
	}
	if !strings.Contains(dis, "MAKE_LIST_TO_MARK") {
		t.Errorf("the collect must close the region:\n%s", dis)
	}
	// The static MAKE_LIST would be the wrong instrument here: its Arg is an
	// element count, and there is none.
	if strings.Contains(dis, "MAKE_LIST ") {
		t.Errorf("a static MAKE_LIST cannot count a runtime run:\n%s", dis)
	}
}

// TestRegionCollectDeclinesAMixedListLiteral — the scope line. A list literal
// with any element BESIDE the region would need its other elements seated
// either side of a run whose length is a runtime value; the collect closes
// only the whole-run case, so these keep the pre-existing refusal with the
// interpreter's answer intact.
func TestRegionCollectDeclinesAMixedListLiteral(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`[9 (for 3 [i])]`, "[[9 0 1 2]]"},
		{`size [(for 3 [i]) 9]`, "[4]"},
		// A DEAD binding drops its result, which wants a static count to pop.
		{`def _ [(for 3 [i])] 5`, "[5]"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			_, compiled, err := rpRun(t, tc.src)
			if compiled {
				t.Fatal("a list literal that is not the whole run must decline the collect")
			}
			if err == nil || !strings.Contains(err.Error(), "consumes loop results") {
				t.Fatalf("refusal reason drifted: %v", err)
			}
			if got := rpInterp(t, tc.src); got != tc.want {
				t.Errorf("interpreter %s, want %s", got, tc.want)
			}
		})
	}
}
