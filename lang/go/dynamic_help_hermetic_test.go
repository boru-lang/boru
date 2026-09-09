package lang

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"testing"
)

// dynamic_help_hermetic_test.go pins the seventh leak channel of the synthetic
// help-example evaluation (native_help.go's makeDynamicEval, whose header
// enumerates all seven): the fn-body ANALYSIS MEMO family, isolated by
// core's IsolateFnAnalysis.
//
// The hook fires from installFnDef on EVERY fn installation — including the
// user program's own `def f fn […]`, DURING their check — and evaluates a
// synthesized example for real, in the registry that is mid-pass. That run
// analyses the user's own body and memoises the residual under a key that
// renders arg TYPE NAMES only, so the example's stand-in argument and the
// program's own argument of the same type collide: the real call site took the
// memo HIT and was handed the EXAMPLE's residual instead of analysing its own
// concrete arguments. The dispatch that should have failed was never
// attempted, and channel 2 (TruncateDiagnostics) ate the evidence, so the
// contamination was SILENT — visible only as a diagnostic that did not appear.
//
// The example is evaluated ONCE PER PROCESS (help's own result memo is
// package-level), so the FIRST check in a process was the polluted one and
// every later check was right. That is what these tests state: a check's
// verdict may not depend on what the process did before it.

// dhRow is the measured shape. `(m get "inc")` is 42 at run time, so `apply`
// over it cannot match, and the fn's declared single return cannot hold the
// two values the un-applied body leaves.
//
// The fn's name is UNIQUE to this file for the same reason `dhleak` below is,
// and the rule is easy to apply to one test and forget on the other: the
// rendered example string is built from the NAME and the PARAM TYPES, and the
// evaluation happens once per process per STRING. This row was first written
// as `def app fn [[nd:Any m:Map] …]` — a name+signature that
// `bytecode_edge_findings_test.go`, `gradual_apply_test.go` and
// `produced_closure_apply_test.go` all also define, rendering the identical
// `app 2 {a:1,b:2}`. `bytecode_edge_findings_test.go` sorts BEFORE this file,
// so in a full-package run it spent the one evaluation first and every check
// here ran on the already-warm path: the test passed with the isolation
// deleted, and only failed when run alone. A vacuous pin in the suite that CI
// actually runs is worse than no pin. `dhapp` is defined nowhere else.
const dhRow = `def dhapp fn [[nd:Any m:Map] [Any] [nd (m get "inc") apply]] def rules {inc: 42} dhapp 5 rules`

// dhDiags renders one check's findings, in order, from a FRESH instance.
func dhDiags(t *testing.T, src string) string {
	t.Helper()
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	res, cerr := a.Check(src)
	if cerr != nil {
		t.Fatalf("check: %v", cerr)
	}
	var out []string
	for _, d := range res.Diagnostics {
		out = append(out, fmt.Sprintf("%s/%s/%s@%d:%d", d.Code, d.Word, d.Severity, d.Row, d.Col))
	}
	return fmt.Sprint(out)
}

// TestCheckVerdictIsHermeticAcrossProcessHistory is the invariant: three
// checks of one program, each on its own instance, must agree. Stated as a
// property rather than an expected string, so it survives a change in the
// diagnostics' wording — before the isolation, the first was empty and the
// rest carried two errors.
func TestCheckVerdictIsHermeticAcrossProcessHistory(t *testing.T) {
	first, second, third := dhDiags(t, dhRow), dhDiags(t, dhRow), dhDiags(t, dhRow)
	if first != second || second != third {
		t.Errorf("a check's verdict must not depend on process history:\n  1st %s\n  2nd %s\n  3rd %s",
			first, second, third)
	}
	// And the shared verdict is the UNCONTAMINATED one — the apply cannot
	// match a non-fn, and the two values it leaves overflow the declared
	// single return. (Both were absent on a cold process before the fix.)
	for _, want := range []string{"no_signature/apply", "type_error/dhapp"} {
		if !strings.Contains(first, want) {
			t.Errorf("first check lost %q: %s", want, first)
		}
	}
}

// TestHelpExampleLeavesNoFnSummary pins the LEAK ITSELF, and needs no warm
// process: the program's only call site is (ProperString, Map), so an
// (Integer, Map) summary for the same fn can only have been written by the
// help example's own evaluation — the example samples an `Any` param as the
// integer 2.
//
// The fn's name is UNIQUE to this test on purpose, for the reason spelled out
// at dhRow above: a name another test could also define would let that test
// spend the one evaluation first and leave this one measuring the already-warm
// path — which is the very order-dependence under test. `dhleak` is defined
// nowhere else in the corpus or the suites.
func TestHelpExampleLeavesNoFnSummary(t *testing.T) {
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	const src = `def dhleak fn [[nd:Any m:Map] [Any] [nd (m get "inc") apply]] def rules {inc: 42} dhleak 'x' rules`
	if _, cerr := a.Check(src); cerr != nil {
		t.Fatalf("check: %v", cerr)
	}
	var leaked, own []string
	for key := range a.NativeRegistry().Check.FnSummaries {
		switch {
		case strings.Contains(key, "#dhleak#Integer,Map"):
			leaked = append(leaked, key)
		case strings.Contains(key, "#dhleak#"):
			own = append(own, key)
		}
	}
	if len(leaked) > 0 {
		t.Errorf("the help example's own body analysis leaked into the pass's memo: %v", leaked)
	}
	// The negative: the program's OWN analyses are still memoised, so the
	// isolation restores the pass's table rather than clearing it.
	if len(own) == 0 {
		t.Error("the program's own fn summaries went missing — the isolation must restore, not clear")
	}
}

// dhExample matches one rendered example LINE — the expression and the result
// the synthetic evaluation computed for it, `kk9 2 {a:1,b:2}   ;# 2`. The
// padding between them is layout, so it is matched loosely; the two halves are
// matched TOGETHER so a `;# 2` from elsewhere in the describe output cannot
// stand in for this line's own result.
var dhExample = regexp.MustCompile(`kk9 2 \{a:1,b:2\}\s+;# 2`)

// TestDynamicHelpStillGeneratesExamples is the regression guard for the
// FEATURE: the fix isolates the synthetic evaluation, it does not skip it, so
// `describe` still prints the generated example AND its computed result.
//
// The result half is the whole point of the assertion. The EXPRESSION is
// rendered from the signature alone and prints whether or not anything was
// ever evaluated; only the `;# 2` comes from the synthetic run (evalExample
// falls back to the placeholder `;# ...` for this shape). An earlier version
// of this test asserted the expression only, and passed with
// `EnableDynamicHelp` neutered to a no-op — i.e. it did not forbid the one
// wrong fix it exists to forbid, "skip the evaluation instead of isolating
// it". Assert the line, not the expression.
func TestDynamicHelpStillGeneratesExamples(t *testing.T) {
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	buf := &bytes.Buffer{}
	a.NativeRegistry().Output = buf
	if _, rerr := a.RunInterp(`def kk9 fn [[nd:Any m:Map] [Any] [nd]] describe kk9`); rerr != nil {
		t.Fatalf("describe: %v", rerr)
	}
	got := buf.String()
	if !dhExample.MatchString(got) {
		t.Errorf("dynamic help lost its generated example or its computed result:\n%s", got)
	}
	// The negative that names the wrong fix: a skipped evaluation renders the
	// same expression with the placeholder result.
	if strings.Contains(got, ";# ...") {
		t.Errorf("the example's result is the placeholder — the evaluation was skipped, not isolated:\n%s", got)
	}
}
