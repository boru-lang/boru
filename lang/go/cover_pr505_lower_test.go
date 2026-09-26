package lang

import (
	"fmt"
	"strings"
	"testing"
)

// cover_pr505_lower_test.go pins, on real programs, the lowering arms the
// reverse-order NUR run added to compiler/go/lower.go that no other test
// reached: the decline reasons of NUR204's index store (storeBindInto over
// the loop's index slot), the bound-checked operand arm of NUR109's parser
// dispatch refusal, and the layouts NUR190's landing lowering leaves
// without a skip or an island (seatLandingSkip, landingDeoptIsland).

// soundDecline asserts src declines to compile with a reason containing
// want, and that the interpreter answers interp ("ERROR:<code>" for a
// raise). A decline is asserted through CompileCheck — the house style of
// TestNamedFnCandidatesSoundCompileFailures — so the unit-suite
// compile-defect ledger is not moved by these witnesses.
func soundDecline(t *testing.T, src, want, interp string) {
	t.Helper()
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	prog, reason, _, cerr := a.CompileCheck(src)
	if cerr != nil || prog != nil || !strings.Contains(reason, want) {
		t.Errorf("%q: want a decline containing %q, got prog=%v reason=%q err=%v", src, want, prog != nil, reason, cerr)
	}
	d, err := New()
	if err != nil {
		t.Fatal(err)
	}
	got, errI := d.RunInterp(src)
	if code, isErr := strings.CutPrefix(interp, "ERROR:"); isErr {
		if codeOf(errI) != code {
			t.Errorf("%q: interpreter %v err=%v, want %s", src, got, errI, code)
		}
	} else if errI != nil || fmt.Sprint(got) != interp {
		t.Errorf("%q: interpreter %v err=%v, want %s", src, got, errI, interp)
	}
}

// TestForIndexDefUnstorableSourceDeclines pins the decline arms of NUR204's
// index store: a body def of the loop's OWN index stores into the index
// slot from a promoted local, the stack top or an inert literal
// (TestForIndexDefInBodyIsTheIterations), and any other source declines
// under the index's own reason instead of storing a value the slot cannot
// hold — a loop's results (the Stage-2 boundary every loop-result consumer
// hits), a multi-result call whose bound value is not the stack top, and a
// non-inert literal.
func TestForIndexDefUnstorableSourceDeclines(t *testing.T) {
	for _, c := range []struct{ src, reason, interp string }{
		{`def i 0 end for 3 [def i (for 2 [5])] end i`,
			"the loop's own index `i` binds loop results (Stage 2)", "[5 5 5 0]"},
		{`def two fn [[][Integer Integer][1 2]] end def i 0 end for 3 [def i (two)] end i`,
			"the loop's own index `i` source is not on top of the stack", "[2 2 2 0]"},
		{`def two fn [[][Integer Integer][1 2]] end for 3 [def i (two) i]`,
			"the loop's own index `i` source is not on top of the stack", "[2 1 2 1 2 1]"},
		{`def i 0 end for 3 [def i <a/> i] end i`,
			"the loop's own index `i` of unknown provenance", "[<a/> <a/> <a/> 0]"},
	} {
		soundDecline(t, c.src, c.reason, c.interp)
	}
}

// TestFnDispatchBranchBoundOperandDeclines pins the bound-checked slot arm
// of NUR109's refusal: parselang-fn-dispatch declines over an operand that
// a branch-carried def some path leaves unbound fills (lowerer.boundSlots),
// since no op re-resolves a name at run time. The fn-valued parser a branch
// binds takes the other arm (TestParseFnDispatchMissParity) — the join does
// not carry a fn value, so no bound-checked slot ever holds a parser; what
// reaches this arm is the SOURCE operand, a String the join carries. The
// arm scans every operand of the dispatch, so the source declines under the
// parser's wording; the interpreter answers the bound twin and raises
// undefined_word for the unbound one. Measured 2026-09-26: with the scan
// restricted to the parser operand, both twins compile and agree (the
// source's PUSH_LOCAL_BOUND raises the interpreter's undefined_word) — so
// this is a sound over-decline, and whoever narrows the scan revises this
// test, and finds the arm left with no program to reach it.
func TestFnDispatchBranchBoundOperandDeclines(t *testing.T) {
	const pre = `import "boru:parselang" def p (fn [[source:String opts:Map] [Any] [7]]) `
	const reason = "`s` is bound only on a branch — an unbound name resolves as a kind at run time (NUR109)"
	soundDecline(t, pre+`def c true if c [def s 'inc'] [] end parse p s`, reason, "[7]")
	soundDecline(t, pre+`def c false if c [def s 'inc'] [] end parse p s`, reason, "ERROR:undefined_word")
	soundDecline(t, `import "boru:parselang" def c true if c [def s 'inc'] [] end parse (fn [[source:String opts:Map] [Any] [7]]) s`, reason, "[7]")
}

// TestLandingLayoutsWithoutSkipOrIsland pins the NUR190 landing layouts
// the lowering answers WITHOUT a skip or an island, each compiled and
// agreeing with the interpreter:
//
//   - a skip still pending from a three-operand apply (`(m.f three 5)`,
//     which the skip does not model) when a later paren apply over another
//     lead is lowered: the later apply finds no landing of its own pending
//     and leaves the table as it is;
//   - a word call behind a pushed operand (`(m.f dbl 5)`), or two word
//     calls (`(m.f three dbl)`, `(m.f dbl (three))`): only the word's
//     call, alone, may follow a skipped landing, so no skip is seated;
//   - a walked landing a fn unit did not plan (its word sits in a list or
//     map literal, no token of the body the island is rebuilt from) while
//     another landing in the unit did: no island for it;
//   - a unit whose unnamed param is still to be seated beneath the landing
//     (the island hands the interpreter the stack alone): no island.
//
// The first two kinds raise uncalled_function on both lanes (the landing raises
// the interpreter's no-match for a named fn over a word candidate); the
// residual and the code agree, and — as TestNamedFnCandidatesRaiseAlike
// notes — the rendered position may differ (the compiled lane stamps the
// landing op's token).
func TestLandingLayoutsWithoutSkipOrIsland(t *testing.T) {
	const nfU = `def inc fn [[n:Integer] [Integer] [n add 1]] end def mk fn [[] [Map] [{f: inc/v}]] end def m (mk) end ` +
		`def dbl fn [[n:Integer][Integer][n mul 2]] end def three fn [[][Integer][3]] end `
	for _, src := range []string{
		nfU + `if true [(m.f three 5) (m.f 5)] [0]`,
		nfU + `if true [(m.f dbl 5)] [0]`,
		nfU + `[(m.f dbl 5)]`,
		nfU + `if true [(m.f three dbl)] [0]`,
		nfU + `if true [(m.f dbl (three))] [0]`,
	} {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
		if !compiled {
			t.Errorf("%q: not compiled: %v", src, errC)
			continue
		}
		if codeOf(errI) != "uncalled_function" || codeOf(errC) != codeOf(errI) || detailOf(errC) != detailOf(errI) ||
			fmt.Sprint(gotC) != fmt.Sprint(gotI) {
			t.Errorf("%q: compiled %v [%s] %s, interp %v [%s] %s", src, gotC, codeOf(errC), detailOf(errC), gotI, codeOf(errI), detailOf(errI))
		}
	}
	const nfQ = `def z fn [[] [Atom] [(quote z)]] end def y fn [[] [Integer] [42]] end ` +
		`def h fn [[] [Integer] [42]] end def h fn [[x:Atom/q] [Atom] [x]] end ` +
		`def mk fn [[] [Map] [{f: h/v}]] end def m (mk) end `
	for _, c := range []struct{ src, want string }{
		{nfQ + `def g fn [[] [Any] [(m.f y) drop [(m.f y)]]] end g`, "[[y]]"},
		{nfQ + `def g fn [[] [Any] [(m.f y) drop {a: (m.f y)}]] end g`, "[{a:y}]"},
		{nfQ + `def g fn [[Integer] [Any] [(m.f y)]] end g 5`, "[y]"},
	} {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, c.src)
		if !compiled || errC != nil || errI != nil || fmt.Sprint(gotC) != c.want || fmt.Sprint(gotI) != c.want {
			t.Errorf("%q: compiled=%v %v err=%v, interp %v err=%v, want %s on both lanes", c.src, compiled, gotC, errC, gotI, errI, c.want)
		}
	}
}
