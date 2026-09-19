package lang

import (
	"fmt"
	"strings"
	"testing"
)

// compiledEqualsInterp is the local shape these pins share: the program must
// COMPILE (no refusal) and the compiled answer must equal the interpreter's.
func compiledEqualsInterp(t *testing.T, label, src string) {
	t.Helper()
	a, _ := New()
	prog, reason, _, _ := a.CompileCheck(src)
	if prog == nil {
		t.Errorf("%s: must compile, refused %q\n  %s", label, reason, src)
		return
	}
	ar, _ := New()
	gotC, compiled, errC := ar.RunCompiled(src)
	if noteCompileDefect(t, src, gotC, errC) {
		return
	}
	b, _ := New()
	gotI, errI := b.RunInterp(src)
	if !compiled {
		t.Errorf("%s: must take the compiled lane\n  %s", label, src)
	}
	if fmt.Sprint(errC) != fmt.Sprint(errI) || fmt.Sprint(gotC) != fmt.Sprint(gotI) {
		t.Errorf("%s: compiled/interp disagree\n  compiled %v / %v\n  interp   %v / %v\n  %s",
			label, gotC, errC, gotI, errI, src)
	}
}

// TestQuotedKeyCarrierCompiles pins the property CompileQuoteKey exists for:
// `set`/`del`'s quoted key may arrive through a CARRIER, not only as an inert
// literal, and the call still compiles.
//
// This is how an open-words override delegates to the base overload — the
// `Atom/q` param passed back through a paren — and it is the shape of
// lang/spec/as.tsv:52-54. Declaring CompileQuoteInert instead, whose
// admission requires IsInertConst, refused all three and moved the
// compile-refusal ceiling 113 -> 116. The corpus catches that, but only on a
// full unfiltered run: under BORU_SPEC_FILES the absolute counts are reported
// rather than asserted, so a filtered run goes green. These pins fail in
// seconds instead.
func TestQuotedKeyCarrierCompiles(t *testing.T) {
	for _, tc := range []struct{ label, src string }{
		{
			"as.tsv:52 — an anchored set override delegating via as",
			`def SortedFlexMap (refine FlexMap)  def set fn [[k:Atom/q v:Any m:SortedFlexMap] [SortedFlexMap] [(set (k) v (m as FlexMap)) drop  m]]  def w:SortedFlexMap (flex { })  (set b/q 2 w) drop  [(keys w) (typeof w)]`,
		},
		{
			"as.tsv:54 — the override routing through a helper fn",
			`def SortedFlexMap (refine FlexMap)  def wr fn [[k2:Atom v2:Any m2:SortedFlexMap] [SortedFlexMap] [(set (k2) v2 (m2 as FlexMap)) drop  m2]]  def set fn [[k:Atom/q v:Any m:SortedFlexMap] [SortedFlexMap] [(wr (k) v m) drop  m]]  def w:SortedFlexMap (flex { })  (set b/q 2 w) drop  [(keys w) (typeof w)]`,
		},
		{
			"del's carrier key",
			`def f fn [[k:Atom/q m:Map] [Map] [(m del (k))]]  f a {a:1 b:2}`,
		},
		{
			"the inert literal key still compiles",
			`def m {a:1}  m set b 2`,
		},
	} {
		compiledEqualsInterp(t, tc.label, tc.src)
	}
}

// TestQuotedKeyGateKeepsRaiseOutOfPoly pins why the two gates read the narrow
// CompileQuoteKey declaration rather than quoteOperandInertOK.
//
// The wider predicate admits every CompileQuoteInert declarer, and `raise` is
// one — it carries CompileQuoteInert|CompileDiverges. A poly-recorded call
// carries no signature on its event and therefore no divergence flag, so a
// poly `raise` stops being a divergent terminal: its `if` arm counts as a
// 0-value contributor, the enclosing fn turns variadic, and every fixed-arity
// consumer refuses. Measured: this program answered 42 interpreted and
// refused compiled with "consumes loop results".
//
// The existing divergence pin does not catch it because its raise operand is
// STATIC, so the word never goes poly there. A raise whose message is a
// dynamic read does.
func TestQuotedKeyGateKeepsRaiseOutOfPoly(t *testing.T) {
	compiledEqualsInterp(t,
		"a trapped raise with a dynamic message keeps its divergence",
		`def f fn [[m:Map] [Integer] [if m.ok [1] [raise bad_input m.k]]]  do [((f {ok:false k:"boom"}) add 1)] error [(42)]`)
	// The static twin, which the older pin covers — kept adjacent so the pair
	// reads as one property.
	compiledEqualsInterp(t,
		"a trapped raise with a static message",
		`def f fn [[m:Map] [Integer] [if m.ok [1] [raise bad_input "z"]]]  do [((f {ok:false}) add 1)] error [(42)]`)
}

// TestQuotedKeyRebindCarriesItsHandler records a widening the declaration
// makes and the by-name test did not: a value rebind (`def myset set/v`)
// copies the signature wholesale — QuoteArgs, CompileEffect and Locked — with
// no FnFrame and no RunInCheck, so it is the one construction that reaches
// the gate carrying the flag. It is sound because the flag can only travel
// ATTACHED to the handler it was registered on: the rebind carries the kernel
// mutator too, so nothing re-steps and the VM runs what the interpreter runs.
// Pinned so that if some future construction separates a declaration from its
// handler, this fails rather than silently compiling a re-stepping word.
func TestQuotedKeyRebindCarriesItsHandler(t *testing.T) {
	const src = `def myset set/v  (myset b/q 2 {a:1})`
	compiledEqualsInterp(t, "a value rebind of set keeps the kernel handler", src)
	a, _ := New()
	prog, _, _, _ := a.CompileCheck(src)
	if prog != nil && strings.Contains(prog.Disassemble(), "FALLBACK") {
		t.Errorf("the rebind should bake a native call, got an island:\n%s", prog.Disassemble())
	}
}
