package lang

import (
	"strings"
	"testing"
)

// TestNUR291CaseScrutineeThatMayBeAList pins NUR291's close. `case` RUNS a
// list scrutinee as a code body and dispatches on its last result, whatever
// produced the list: over mk's `[1 2]`, `case (mk) [Integer "i" …]` is "i".
// The compiled desugar matched the value itself when the pass held it
// abstractly (`o`, and `l` against a List clause). The forward form records
// case's own scrutinee rule ahead of the chain (__casesubject), so the
// chain matches what the interpreter dispatches on — a list's last result,
// its refusal when it leaves none, any other value as itself. In the stack
// form a list makes both operands lists, which CaseHandler reads the forward
// way round (the clause list is the scrutinee): no chain over the written
// clauses is that, so the stack form's guard (__casestack) defers on a list,
// loudly, and passes every other value.
func TestNUR291CaseScrutineeThatMayBeAList(t *testing.T) {
	mk := func(v string) string { return `def mk fn [[][Any][` + v + `]] end ` }
	const clauses = ` [Integer "i" String "s" "o"]`
	for _, c := range []struct{ src, want string }{
		{mk(`[1 2]`) + `case (mk)` + clauses, "[i]"},
		{mk(`["a" 2]`) + `case (mk)` + clauses, "[i]"},
		{mk(`[1 "b"]`) + `case (mk)` + clauses, "[s]"},
		{mk(`[1 2]`) + `case (mk) [List "l" "o"]`, "[o]"},
		{mk(`[]`) + `case (mk)` + clauses, "ERROR:case: value expression produced no value to dispatch on"},
		{mk(`quote [1 nope]`) + `case (mk)` + clauses, "ERROR:nope"},
		{mk(`42`) + `case (mk)` + clauses, "[i]"},
		{mk(`"s"`) + `case (mk)` + clauses, "[s]"},
		{mk(`{a:1}`) + `case (mk)` + clauses, "[o]"},
		{`def f fn [[v:Any][Any][case v` + clauses + `]] end f [1 2]`, "[i]"},
		{`def f fn [[v:Any][Any][case v` + clauses + `]] end f 2.5`, "[o]"},
		{mk(`42`) + `(mk) case` + clauses, "[i]"},
		{mk(`"s"`) + `(mk) case` + clauses, "[s]"},
		{`do [raise aa "A"] error [dot code case [aa/q 1 bb/q 2 3]]`, "[1]"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
	src := mk(`[1 2]`) + `(mk) case` + clauses
	gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
	if !compiled || codeOf(errC) != "internal_error" || !strings.Contains(errC.Error(), "NUR291") ||
		errI != nil || len(gotI) != 0 || len(gotC) != 0 {
		t.Errorf("%s: the stack form over a list defers compiled; got %v %v / %v %v", src, gotC, errC, gotI, errI)
	}
}
