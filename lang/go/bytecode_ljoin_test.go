package lang

import (
	"fmt"
	"strings"
	"testing"
)

// The L-JOIN graduation (plan Phase 4): a strict-disjunct if-join feeding a
// RECURSIVE call used to decline "fn call operand of unknown provenance" —
// the check-mode disjunct distribution (disjunctPartitionReturns) ran the
// fn's ReturnsFn once per alternative COMBO with fresh-ID carrier copies,
// and the armed recording tried to RecordUserCall those copies. The combos
// now run as SUSPENDED type probes and the dispatch records ONCE with the
// original (provenance-carrying) args; the partition-joined carriers are
// re-IDed onto the recorded results (carrierResults' partitioned user-fn
// arm, gated by disjunctCombosTakeSig).

// ljoinParityCompiled asserts the row force-compiles, runs COMPILED with no
// runtime bails, and matches the interpreter's values/errors exactly.
func ljoinParityCompiled(t *testing.T, src string) {
	t.Helper()
	a := mustNew(t)
	if _, reason, _, err := a.CompileCheck(src); err != nil || reason != "" {
		t.Fatalf("the shape must compile, got compile failure %q / err %v\n  src: %s", reason, err, src)
	}
	b := mustNew(t)
	var bails []BailEvent
	defer b.ArmRuntimeBailHook(func(e BailEvent) { bails = append(bails, e) })()
	outC, ran, errC := b.RunCompiled(src)
	if noteCompileDefect(t, src, outC, errC) {
		return
	}
	if !ran {
		t.Fatalf("the shape must run COMPILED, fell back (err %v)", errC)
	}
	if len(bails) > 0 {
		t.Errorf("zero runtime bails required, got %d (%s …)", len(bails), bails[0].Site)
	}
	c := mustNew(t)
	outI, errI := c.RunInterp(src)
	if (errC == nil) != (errI == nil) || fmt.Sprint(outC) != fmt.Sprint(outI) {
		t.Errorf("parity: compiled %v/%v != interp %v/%v", outC, errC, outI, errI)
	}
}

func TestLJoinRecursiveUnionJoinCompiles(t *testing.T) {
	rows := []string{
		// The distant-cousins join (Integer|String), base case taken.
		`def f fn [[n:Any b:Any] [Any] [
  if (n eq none) [b] [
    def b2 (if (n "end" get) [7] ["s"])
    b2 (n "mid" get) f
  ]
]]
(f none 2)`,
		// The recursive arm actually taken: one hop, else-alternative ("s").
		`def f fn [[n:Any b:Any] [Any] [
  if (n eq none) [b] [
    def b2 (if (n "end" get) [7] ["s"])
    b2 (n "mid" get) f
  ]
]]
(f {mid: none} 2)`,
		// One hop, then-alternative (7).
		`def f fn [[n:Any b:Any] [Any] [
  if (n eq none) [b] [
    def b2 (if (n "end" get) [7] ["s"])
    b2 (n "mid" get) f
  ]
]]
(f {end: true, mid: none} 2)`,
		// The None-arm union join (7|none-param).
		`def f fn [[n:Any b:Any] [Any] [
  if (n eq none) [b] [
    def b2 (if (n "end" get) [7] [b])
    b2 (n "mid" get) f
  ]
]]
(f none none)`,
		// The frontier lJoinRepro (4 params, pc indirection, print consumer).
		lJoinRepro,
	}
	for i, src := range rows {
		t.Run(fmt.Sprintf("row-%d", i), func(t *testing.T) {
			ljoinParityCompiled(t, src)
		})
	}
}

// TestLJoinSiblingOverloadStaysSound — the miscompile the
// disjunctCombosTakeSig gate exists to prevent: a disjunct that matches the
// WIDE second arm as a whole, while its Integer alternative first-matches
// the NARROW first arm at run time. The multi-overload ambiguity machinery
// (OpCallUserPoly) owns the shape — both runtime directions must match the
// interpreter exactly.
func TestLJoinSiblingOverloadStaysSound(t *testing.T) {
	srcFor := func(rec string) string {
		return `def g fn [[a:Integer] [String] ["i"] [a:Any] [String] ["any"]]
def f fn [[n:Any] [Any] [
  def b2 (if (n "end" get) [7] ["s"])
  (g b2)
]]
(f ` + rec + `)`
	}
	for _, rec := range []string{`{}`, `{end: true}`} {
		src := srcFor(rec)
		a := mustNew(t)
		outC, _, errC := a.RunCompiled(src)
		if noteCompileDefect(t, src, outC, errC) {
			continue
		}
		b := mustNew(t)
		outI, errI := b.RunInterp(src)
		if (errC == nil) != (errI == nil) || fmt.Sprint(outC) != fmt.Sprint(outI) {
			t.Errorf("sibling-overload parity:\n  src: %s\n  compiled %v/%v != interp %v/%v", src, outC, errC, outI, errI)
		}
	}
}

// TestLJoinPartialDispatchDiagnosticSurvives — the combo probes now run
// SUSPENDED under a compile pass, but suspension gates RECORDING, not
// diagnostics: a declared-union domain hole must still report
// partial_dispatch on the compile pass exactly as on the plain check.
func TestLJoinPartialDispatchDiagnosticSurvives(t *testing.T) {
	const src = `def U (Integer tor String)
def h fn [[a:Integer] [Integer] [a]]
def f fn [[x:U] [Any] [(h x)]]
(f 5)`
	a := mustNew(t)
	_, _, cres, err := a.CompileCheck(src)
	if err != nil {
		t.Fatalf("CompileCheck: %v", err)
	}
	found := false
	for _, d := range cres.Diagnostics {
		if d.Code == "partial_dispatch" || (d.Code != "" && strings.Contains(d.Detail, "no overload for alternative")) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("the declared-union domain hole must still report under a compile pass; diagnostics: %v", cres.Diagnostics)
	}
}

// TestLJoinTopLevelDisjunctCallCompiles — the "separate gap" the probes
// surfaced (a top-level disjunct join feeding a call) turned out to be the
// SAME partition-recording leak, reached from the top-level dispatch: the
// fix graduates it with the recursive family. Both a runtime-dynamic
// condition and a constant-foldable one are pinned.
func TestLJoinTopLevelDisjunctCallCompiles(t *testing.T) {
	rows := []string{
		`def g fn [[x:Any] [Any] [x]]
def m {e: true}
def b2 (if (m "e" get) [7] ["s"])
(b2 g)`,
		`def g fn [[x:Any] [Any] [x]]
def m {}
def b2 (if (m "e" get) [7] ["s"])
(b2 g)`,
	}
	for i, src := range rows {
		t.Run(fmt.Sprintf("row-%d", i), func(t *testing.T) {
			ljoinParityCompiled(t, src)
		})
	}
}
