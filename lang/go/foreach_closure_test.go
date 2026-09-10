package lang

import (
	"fmt"
	"strings"
	"testing"
)

// foreach_closure_test.go is the whole-program half of `for-each`'s closure
// body (the forty-fourth increment).
//
// The word declared no CallableSpec at all, so its body never compiled: the
// dispatch baked the body as a plain List const and the handler interpreted
// it once per element, and its Function form never even reached the callback
// — a Function-typed sig slot meets the Stage-3 function-valued-operand gate
// unless the word's body compiles to a closure. It now carries each's spec
// minus the three flags its own handler does not earn, and the rows below
// pin both halves: the token body compiles to a unit, and the Function /
// lambda forms reach the callback with the convention the interpreter uses.

func feRun(t *testing.T, src string) (string, []string) {
	t.Helper()
	b, err := New()
	if err != nil {
		t.Fatal(err)
	}
	var islands []string
	disarm := b.ArmInterpEntryHook(func(ev InterpEntry) {
		if ev.CheckMode || ev.Attribution != "" || !strings.HasPrefix(ev.Seam, "vm:island") {
			return
		}
		islands = append(islands, ev.Seam)
	})
	out, compiled, cerr := b.RunCompiled(src)
	disarm()
	if cerr != nil {
		t.Fatalf("RunCompiled(%q): %v", src, cerr)
	}
	if !compiled {
		t.Fatalf("RunCompiled(%q): fell back to the interpreter", src)
	}
	return fmt.Sprintf("%v", out), islands
}

func TestForEachCompilesItsBodyWithParity(t *testing.T) {
	const dbl = `def dbl x:Integer => [mul 2 x] end `
	const acc = `def acc (flex []) end `
	for _, tc := range []struct{ src, want string }{
		// The frontier row: the Function form, which the Stage-3 gate used
		// to refuse outright.
		{dbl + `for-each dbl/v [1 2 3]`, "[]"},
		{acc + dbl + `for-each dbl/v [1 2 3] end acc`, "[[]]"},
		// A side-effecting fn value driven once per element — the result is
		// discarded, the mutation is not.
		{`def acc (flex {}) end def stp fn [[e:String] [Any] [acc set (e) true]] end for-each stp/v ['x' 'y'] end acc`, "[{x:true y:true}]"},
		// …and the quotation wrapper it replaces still answers identically.
		{`def acc (flex {}) end def stp fn [[e:String] [Any] [acc set (e) true]] end for-each [stp] ['x' 'y'] end acc`, "[{x:true y:true}]"},
		// A LAMBDA over a list receives the bare element.
		{`def acc (flex {}) end for-each ([e:Integer] => [acc set 'k' e]) [1 2] end acc`, "[{k:2}]"},
		// The EMPTY body — for-each's own case, and the one each raises on.
		{`[1 2 3] for-each [] end 'z'`, "[z]"},
		// A body that nets a value: discarded, not returned.
		{`[1 2 3] for-each [1] end 'z'`, "[z]"},
		// An empty collection never invokes the body at all.
		{`[] for-each [1] end 'z'`, "[z]"},
		// The MAP overload — the token body sees the value, and the
		// dispatch picks the (List, Map) signature.
		{`def acc (flex {}) end {a:1 b:2} for-each [drop] end acc`, "[{}]"},
		{`{a:1 b:2} for-each [print] end 'z'`, "[z]"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			got, islands := feRun(t, tc.src)
			if got != tc.want {
				t.Errorf("compiled %s, want %s", got, tc.want)
			}
			if len(islands) > 0 {
				t.Errorf("re-enters the interpreter: %v", islands)
			}
			c, err := New()
			if err != nil {
				t.Fatal(err)
			}
			out, ierr := c.RunInterp(tc.src)
			if ierr != nil {
				t.Fatalf("RunInterp: %v", ierr)
			}
			if in := fmt.Sprintf("%v", out); in != tc.want {
				t.Errorf("interpreter %s, want %s — the oracle moved", in, tc.want)
			}
		})
	}
}

// TestForEachBodyIsAClosureUnit pins the STREAM: the body must lower to its
// own unit pushed as a closure, not ride as a List const the handler
// interprets. A row that happens to agree either way would otherwise not
// notice the spec being dropped again.
func TestForEachBodyIsAClosureUnit(t *testing.T) {
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	prog, reason, _, cerr := a.CompileCheck(`[1 2 3] for-each [print]`)
	if cerr != nil || prog == nil {
		t.Fatalf("refused %q err=%v", reason, cerr)
	}
	dis := prog.Disassemble()
	if !strings.Contains(dis, "PUSH_CLOSURE") || !strings.Contains(dis, "for-each$body") {
		t.Errorf("the body must lower to its own closure unit:\n%s", dis)
	}
}

// TestForEachLambdaConventionMatchesTheInterpreter is the measurement the
// spec rests on, kept as a test rather than a comment: the convention was
// taken off the interpreter, not inherited from each because the two share a
// handler family. A LIST hands the bare element; a MAP hands the KeyVal.
func TestForEachLambdaConventionMatchesTheInterpreter(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`def acc (flex []) end for-each ([e:Any] => [acc (typeof e) append]) [1 2] end acc`, "[[Integer Integer]]"},
		{`def acc (flex []) end for-each ([e:Any] => [acc (typeof e) append]) {a:1 b:2} end acc`, "[[KeyVal KeyVal]]"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			c, err := New()
			if err != nil {
				t.Fatal(err)
			}
			out, ierr := c.RunInterp(tc.src)
			if ierr != nil {
				t.Fatalf("RunInterp: %v", ierr)
			}
			if got := fmt.Sprintf("%v", out); got != tc.want {
				t.Fatalf("interpreter %s, want %s — the convention moved", got, tc.want)
			}
			if got, _ := feRun(t, tc.src); got != tc.want {
				t.Errorf("compiled %s, want %s", got, tc.want)
			}
		})
	}
}

// TestForEachKeepsTheAmbiguousOverloadRefusal is the flag NOT set, and why.
// CrossCollectionTokenShape licenses committing to the List overload for a
// statically-ambiguous (gradual-Any) collection, because each's handler
// delegates to the map iteration when the runtime value turns out to be a
// map. forEachHandler does not — it reads args[1] as a list — so committing
// would raise where the interpreter iterates. The refusal is the sound
// fallback, and `each` compiling the same shape is what makes the
// difference visible.
func TestForEachKeepsTheAmbiguousOverloadRefusal(t *testing.T) {
	const src = `def mk fn [[f:Boolean] [Any] [if f [[1 2]] [{a:1}]]] end def d (mk true) end d for-each [drop]`
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	prog, reason, _, cerr := a.CompileCheck(src)
	if cerr != nil {
		t.Fatalf("check: %v", cerr)
	}
	if prog != nil {
		t.Fatalf("a gradual collection must refuse, not commit to the List overload:\n%s", prog.Disassemble())
	}
	if !strings.Contains(reason, "gradual-Any collection") {
		t.Errorf("refused %q, want the ambiguous-overload refusal", reason)
	}
	b, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if _, ierr := b.RunInterp(src); ierr != nil {
		t.Errorf("the interpreter must still answer: %v", ierr)
	}
}
