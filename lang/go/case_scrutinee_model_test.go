package lang

import "testing"

// case_scrutinee_model_test.go pins how the compile pass models a `case`
// CODE-BODY scrutinee (basic/go conditional.go, CaseReturnsFn): the
// interpreter runs it exactly once, unconditionally, before the dispatch, so
// what it binds stands after the `case` and what it nets is what the clauses
// match. Each row is a shape where the model disagreed with the run — most
// compiled to a different answer, silently — and each now declines loudly,
// with the interpreter's answer pinned beside it.

// A scrutinee that BINDS a name and nets NOTHING: the scrutinee is a block
// (design §2.1, phase 2), so its def ends with it, and the interpreter
// raises case_error over nothing; the compiled lane traps the same error.
func TestCaseBindingScrutineeNettingNothingTraps(t *testing.T) {
	blockRuleOrParity(t, `def x 1 end case [def x 5] [5 "five" "other"] end x`, "ERROR:case: value expression produced no value")
}

// The same scrutinees inside a body the compiled program ISLANDS (a `do`
// whose body declines to a closure) or runs per element: the scrutinee's
// def is its own block local on both lanes, so the read after the body is
// the root's x — the answer the stale model once baked by accident is the
// rule's answer now; the compiled lane agrees where it compiles and declines
// the block shadow where it does not.
func TestCaseScrutineeBindingsInIslandedBodyAgree(t *testing.T) {
	// The scrutinee is a block since phase 2 (design §2.1): its def ends
	// with it, and the read after the body sees the root's x.
	const noValue = "error(case: value expression produced no value to dispatch on)"
	for _, c := range []struct{ src, want string }{
		{`def x 1 end do [case [def x 5] [5 "five" "other"]] end x`, "[" + noValue + " 1]"},
		{`def x 1 end do [case [def x 5 1] [1 "one" 2 "two" "other"]] end x`, "[one 1]"},
		{`def x 1 end do [case [def x 5 5] [5 "five" "other"] drop case [def y 5] [5 "a" "b"]] end x`, "[" + noValue + " 1]"},
		{`def x 1 end do [case [def x 5] [5 "five" "other"]] error [drop "caught"] end x`, "[caught 1]"},
		{`def x 1 end [1 2] each [case [def x 9 x] [9 "nine" "other"]] end x`, "[['nine' 'nine'] 1]"},
	} {
		blockRuleOrParity(t, c.src, c.want)
	}
}

// A scrutinee the pass holds as a CARRIER — a list the gradual-contagion
// rule flagged, here the literal a `nip` over a gradual value returns — has a
// residual only the run knows. condResidual used to report it with the nil a
// body that nets nothing reports, and the case recorded its terminal trap:
// the program compiled to `case_error` where the interpreter answers "two".
// Now the dispatch stands as the generic record leaves it, which declines.
func TestCaseCarrierScrutineeNeverTraps(t *testing.T) {
	const mk = `def mk fn [[][List][quote [9]]] end `
	for _, src := range []string{
		mk + `def v (do (mk)) end case (v [1 2] nip) [2 "two" "other"]`,
		mk + `case ((do (mk)) [1 2] nip) [2 "two" "other"]`,
	} {
		requireLoudDecline(t, src, "code-body word case", "[two]")
	}
}
