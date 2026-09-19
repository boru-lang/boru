package lang

import (
	"fmt"
	"strings"
	"testing"
)

// suspendedRecordProg reproduces the voxgig-template liquid `split-args` shape
// that miscompiled under --compile. `join-segs`'s fold body calls `mid`, which
// folds AND calls `split-seg`, which ITSELF folds over a two-field accumulator
// building a LIST-valued map `do {cur:[…] q:[q]}` from a fold-body-local `q`.
//
// split-seg's inner fold is type-analysed with recording SUSPENDED
// (analyseHigherOrderBodyVals runs the body in a sub-engine for the accumulator
// fixed point). Before the fix, autoEvalList / autoEvalMap gated their
// consumed-list recording on the recorder being ARMED rather than ACTIVE, so the
// suspended analysis recorded the consumed `[q]` list into the OUTER unit's
// frame — an OpLookupDynScope `q` with no coherent OpBindDynScope. At runtime it
// missed (vm:dyn-scope-miss) and, under Test.test's per-case error trap, surfaced
// as a spurious FAIL instead of the clean whole-program fallback.
const suspendedRecordProg = `import "boru:test"
module [
  import "boru:string-util"
  def split-seg fn [ [s:String] [List] [
    def out (flex [])
    def res (do {cur:[""] q:[""]} (iota (s size)) [ var [[i acc]
      def ch  (slice i (i add 1) s)
      def cur (acc get "cur")
      def q   (acc get "q")
      if (q eq "")
        [ if (ch eq ",")
            [ def _ (out push cur)  do {cur:[""] q:[""]} ]
            [ do {cur:[(StringUtil.concat [cur ch])] q:[""]} ] ]
        [ do {cur:[(StringUtil.concat [cur ch])] q:[q]} ]
    ] ] fold)
    def _2 (out push (res get "cur"))
    slice 0 (out size) out
  ] ]
  def mid fn [ [seg:String acc:String] [String] [
    def parts ((split-seg seg) each [ var [[a] (StringUtil.trim a) ] ])
    StringUtil.concat [acc "/" (StringUtil.concat parts {sep:","})]
  ] ]
  def join-segs fn [ [xs:List] [String] [
    ("" xs [ var [[seg acc] (mid seg acc) ] ] fold)
  ] ]
  export "M" { run: join-segs/v }
] import
def go fn [ [s:String] [String] [ (M.run [s "c,d"]) ] ]
[ "/a,b/c,d" (go "a,b") Assert.equal end ] "t" Test.test
Test.fail-count`

// TestSuspendedFoldAnalysisNoSpuriousDynScope pins the fix for the
// suspended-analysis spurious-record bug two ways: (1) the compiled run must NOT
// runtime-bail on a dyn-scope miss (the compiler must not emit an incoherent
// OpLookupDynScope), observed via the runtime-bail hook; and (2) the compiled
// fail-count must match the interpreter's (0 — the assertion holds).
func TestSuspendedFoldAnalysisNoSpuriousDynScope(t *testing.T) {
	a := mustNew(t)
	var bails []string
	disarm := a.ArmRuntimeBailHook(func(e BailEvent) { bails = append(bails, e.Site) })
	compiled, _, cerr := a.RunCompiled(suspendedRecordProg)
	if noteCompileDefect(t, suspendedRecordProg, compiled, cerr) {
		return
	}
	disarm()
	if cerr != nil {
		t.Fatalf("RunCompiled: %v", cerr)
	}
	for _, s := range bails {
		if strings.Contains(s, "dyn-scope") {
			t.Fatalf("compiled program bailed on %q — a SUSPENDED higher-order body "+
				"analysis recorded a spurious LOOKUP_DYN_SCOPE into the outer unit", s)
		}
	}
	interp, ierr := a.RunInterp(suspendedRecordProg)
	if ierr != nil {
		t.Fatalf("RunInterp: %v", ierr)
	}
	if len(compiled) == 0 || len(interp) == 0 {
		t.Fatalf("empty residual: compiled=%v interp=%v", compiled, interp)
	}
	cN := fmt.Sprint(compiled[len(compiled)-1])
	iN := fmt.Sprint(interp[len(interp)-1])
	if iN != "0" {
		t.Fatalf("interpreter fail-count = %s, want 0 (the assertion holds)", iN)
	}
	if cN != iN {
		t.Fatalf("compiled/interpreter fail-count divergence: compiled=%s interp=%s "+
			"(a spurious compiled FAIL from the suspended-analysis dyn-scope miss)", cN, iN)
	}
}
