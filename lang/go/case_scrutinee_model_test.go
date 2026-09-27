package lang

import "testing"

// case_scrutinee_model_test.go pins how the compile pass models a `case`
// CODE-BODY scrutinee (basic/go conditional.go, CaseReturnsFn): the
// interpreter runs it exactly once, unconditionally, before the dispatch, so
// what it binds stands after the `case` and what it nets is what the clauses
// match. Each row is a shape where the model disagreed with the run — most
// compiled to a different answer, silently — and each now declines loudly,
// with the interpreter's answer pinned beside it.

// A scrutinee that BINDS a name and nets NOTHING: the interpreter keeps the
// binding and raises case_error. No lowering keeps the binding (only the
// single-clause desugar runs the scrutinee as a kept condition), so the
// recording pass declines rather than trap over a model that dropped it.
func TestCaseBindingScrutineeNettingNothingDeclines(t *testing.T) {
	requireLoudDeclineErr(t, `def x 1 end case [def x 5] [5 "five" "other"] end x`,
		"the scrutinee binds a name the interpreter keeps past it", "case_error")
}

// The same bindings inside a body the compiled program ISLANDS (a `do` whose
// body declines to a closure) or runs per element. That body's model is a
// suspended run that recorded nothing for the case and, until
// keepScrutineeBindings, never ran the scrutinee: the model after the body
// still held x = 1 and the read after it was baked from it. The first four
// rows compiled to `[error(…) 1]`, `[one 1]`, `[error(…) 1]` (a desugarable
// binding case beside a declining one) and `[caught 1]`; the last compiled
// to the right answer only because its closure re-read x dynamically, over a
// model just as stale. Kept in the model, the install reaches the bind
// ledger, whose twin regime has no placement for it and declines — as it
// already did for the same binding in an `if` condition
// (`do [if [def x 5 true] [1] [2]]`, `[1 2] each [if [def x 9 true] …]`).
func TestCaseScrutineeBindingsInIslandedBodyDecline(t *testing.T) {
	const twin = "twin regime: a bind transition has no stream placement"
	const noValue = "error(case: value expression produced no value to dispatch on)"
	for _, c := range []struct{ src, want string }{
		{`def x 1 end do [case [def x 5] [5 "five" "other"]] end x`, "[" + noValue + " 5]"},
		{`def x 1 end do [case [def x 5 1] [1 "one" 2 "two" "other"]] end x`, "[one 5]"},
		{`def x 1 end do [case [def x 5 5] [5 "five" "other"] drop case [def y 5] [5 "a" "b"]] end x`, "[" + noValue + " 5]"},
		{`def x 1 end do [case [def x 5] [5 "five" "other"]] error [drop "caught"] end x`, "[caught 5]"},
		{`def x 1 end [1 2] each [case [def x 9 x] [9 "nine" "other"]] end x`, "[['nine' 'nine'] 9]"},
	} {
		requireLoudDecline(t, c.src, twin, c.want)
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
