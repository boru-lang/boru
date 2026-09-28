package lang

import (
	"fmt"
	"strings"
	"testing"
)

// TestNUR242FoldNoMatchIsTheInterpreters pins NUR242's `fold` program: a
// fold over a class field declared Any whose run-time value no overload
// takes. The check pass cannot match the field statically and records the
// widest overload's window as a poly re-match (the dyn-body backstop), and
// the VM's no-match could not rebuild the interpreter's report for a word
// whose overloads differ in arity, so the run bailed (internal_error, "please
// report it"). The record now carries the window's exact layout (how many
// operands were written after the word), and the VM plans and reports over
// that tape as the interpreter does.
func TestNUR242FoldNoMatchIsTheInterpreters(t *testing.T) {
	const box = `def Box class {data: Any} end def b (make Box {data: %s}) end `
	for _, c := range []struct{ data, prog string }{
		{`"s"`, `0 fold [add] b.data`},
		{`"s"`, `b.data 0 fold [add]`},
		{`[1 2]`, `b.data 0 fold [add]`},
		{`5`, `0 fold [add] b.data`},
		{`"s"`, `fold [add] b.data`},
		{`"s"`, `(0 fold [add] b.data)`},
		{`"s"`, `[(0 fold [add] b.data)]`},
		{`"s"`, `0 fold [add] b.data end 7`},
		{`"s"`, `def f fn [[] [Any] [0 fold [add] b.data]] end f`},
		{`"s"`, `if true [0 fold [add] b.data] [0]`},
		{`"s"`, `do [0 fold [add] b.data] error [(99)]`},
		// A function word after the window (NUR283).
		{`"s"`, `0 fold [add] b.data drop`},
		// …and a window a plan fills keeps answering.
		{`[1 2]`, `0 fold [add] b.data`},
		{`{a:1}`, `0 fold [add] b.data`},
		{`"s"`, `[1 2] fold [add] b.data`},
		{`"s"`, `b.data [1 2] fold [add]`},
	} {
		requireEngineParity(t, fmt.Sprintf(box, c.data)+c.prog, true)
	}
	// The report is the interpreter's attempted window: the written pair,
	// and for one written operand the stack value beneath it too.
	for _, c := range []struct{ prog, note string }{
		{`0 fold [add] b.data`, "the arguments were [word(add)] (a List) and 's' (a ProperString)"},
		{`b.data 0 fold [add]`, "the arguments were [word(add)] (a List) and 0 (an Integer)"},
	} {
		if _, err := mustNew(t).Run(fmt.Sprintf(box, `"s"`) + c.prog); err == nil || !strings.Contains(err.Error(), c.note) {
			t.Errorf("%s: the compiled report names %q, got %v", c.prog, c.note, err)
		}
	}
	// A value the dispatch did not take beside its window — a 5 written
	// after the field, which the interpreter's report lists — rides with
	// the layout (After, NUR283), so the compiled run raises the same
	// report, the 5 included.
	src := fmt.Sprintf(box, `"s"`) + `0 fold [add] b.data 5`
	_, ei := mustNew(t).RunInterp(src)
	_, ec := mustNew(t).Run(src)
	if ei == nil || !strings.Contains(ei.Error(), "and 5 (an Integer)") {
		t.Fatalf("%s: the interpreter's report lists the 5, got %v", src, ei)
	}
	if ec == nil || ec.Error() != ei.Error() {
		t.Errorf("%s: the compiled run raises the interpreter's report; got %v", src, ec)
	}
}
