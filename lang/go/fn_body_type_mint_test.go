package lang

import (
	"fmt"
	"testing"
)

// TestFnBodyTypeMintIsTheCalls pins NUR167's close. A `def T` inside a fn
// body mints a node and reserves its name part for the registry's lifetime
// (the frame's teardown pops the binding and keeps the part), so the SECOND
// call of the body conflicts on the interpreter. The check pass's analysis
// of the body is not a call: its reservation now comes off with the body's
// bindings (core.ForgetTypePartsSince), so the run's first call reserves the
// part as the interpreter's does and the second conflicts as the
// interpreter's does — element 1, not element 0, and a single call answers.
// A fn UNIT whose body installs a type re-installs it per call (OpBindFnType:
// the name checked against the run's reservations and reserved, the
// check-time node bound for the frame) instead of dropping the mint, and a
// root type twin frees its reservation with the rollback and re-checks the
// name at its own position (applyTwinPush), so a run-time mint ahead of it
// conflicts where the interpreter's def does.
func TestFnBodyTypeMintIsTheCalls(t *testing.T) {
	const f = "def f fn [[n:Integer] [Integer] [def T (class {}) n]] end "
	for _, src := range []string{
		f + "each f/v [1 2]",
		f + "each f/v [1]",
		f + "do [f 1]",
		f + "each f/v [1] def T (class {}) 5",
		f + "each f/v [1] def T Integer 5",
		f + "def T (class {}) each f/v [1 2]",
		"def T (class {}) " + f + "each f/v [1 2]",
		"def f fn [[n:Integer] [Integer] [def T (refine Integer) n]] end each f/v [1 2]",
		"def g fn [[] [Integer] [def T (class {}) undef T 1]] end each g/v [1 2]",
		"def g ([n:Integer] => [def T (class {}) n]) end each g/v [1 2]",
		"def f fn [[n:Integer] [Boolean] [def T (class {}) (make T {}) is T]] end each f/v [1]",
		"def T (class {}) 5",
		"def T (class {}) undef T 1",
		"def T (class {}) def T (class {}) 5",
		"def A Integer 5 is A",
		"def P (refine Integer) def x:P 3 x",
		f + "f 1",
		f + "f 1 f 2",
		"def f fn [[n:Integer] [Boolean] [def T (class {}) (make T {}) is T]] end f 1",
		"def g fn [[] [Integer] [def T (class {}) undef T 1]] end g g",
		"def rpt fn [[] [Any] [do [def Big Integer 15 is Big]]] (rpt) (rpt)",
		"def rpt fn [[] [Any] [do [def Big Integer 15 is Big]]] (rpt) def Big Integer 2",
		"import \"boru:test\" end def res (Test.check-prop \"hazard\" [def Big Integer 9] [ 0 gte ] 2 1 0) res",
	} {
		requireSameVerdict(t, src)
	}
	// The interpreter's own verdicts, so the parity above is parity with
	// the rule: the first call answers, the second conflicts.
	for _, c := range []struct{ src, want string }{
		{f + "each f/v [1]", "[[1]]"},
		{f + "f 1", "[1]"},
		{"def rpt fn [[] [Any] [do [def Big Integer 15 is Big]]] (rpt)", "[true]"},
		{f + "def T (class {}) each f/v [1 2]", "[[1 2]]"},
	} {
		got, err := mustNew(t).RunInterp(c.src)
		if err != nil || fmt.Sprint(got) != c.want {
			t.Errorf("%s: interpreter = %v / %v, want %s", c.src, got, err, c.want)
		}
	}
	for _, src := range []string{
		f + "each f/v [1 2]",
		f + "f 1 f 2",
		f + "each f/v [1] def T (class {}) 5",
	} {
		if _, err := mustNew(t).RunInterp(src); err == nil {
			t.Errorf("%s: interpreter must conflict on the second mint", src)
		}
	}
}
