package lang

// Check-prop property-body comparator-dispatch pin (the third voxgig-surfaced
// main-era regression, sibling of the check-prop GENERATOR miscompile).
//
// `Test.check-prop`'s gen/property bodies compile as STORED-PARAM units
// (compileStoredParamBody, since 853dcaa4). Their inputs — the per-invoke
// generated value — are dispatched over their RUNTIME type, the gradual
// contract. Compiling them with a STRICT Any carrier fell to `v.Is(t)` in
// sigTypeMatches (Any does not conform to List), so a comparator-taking
// dispatch over the untyped generated list — the sort suite's `(lst Sort.quick
// Sort.by-number)` — failed to commit `Sort.quick` (its `lst:List` slot
// rejected the strict Any) and the trailing comparator `Sort.by-number`
// dispatched instead, inverting the call to `cmp(lst, quick-fn)`
// (`cmp: cannot order List and Function`) under `--compile` while the
// interpreter passed — a silent-fallback contract violation (sort_prop_test,
// 3 of 7 props).
//
// The fix makes stored-param inputs GRADUAL (ParamInputCarrier): the not-
// disjoint rule matches List optimistically, `Sort.quick` commits, and the body
// compiles the SAME dispatch the interpreter runs — FULL COMPILATION, not a
// fallback.

import (
	"fmt"
	"strings"
	"testing"
)

// The miniature of the sort_prop_test shape: a check-prop property body that
// dispatches a comparator-taking sort (comparator-FIRST param, applying the
// comparator via `(a b comp)` inside a nested each — the quick-go shape) over
// the generated list, passing a module comparator fn as the argument. It must
// FULLY compile (no interpreter island) AND agree with the interpreter.
func TestCheckPropStoredBodyComparatorDispatchCompiles(t *testing.T) {
	src := `import "boru:test" end
import module [
  def by-number fn [[b:Any a:Any] [Integer] [(a cmp b)]]
  def srt fn [[comp:Function lst:List] [List] [
    def arr (flex lst)
    def _ (iota (lst size) each [ var [[i]
      def c ((arr get i) (arr get 0) comp)
      0
    ]])
    (arr slice 0 (lst size))
  ]]
  export "S" {by-number: by-number/v, srt: srt/v}
] end
Test.check-prop "sort-dispatch"
  [ iota (r.int 0 5) each [ var [[i] r.int 0 12 ] ] ]
  [ var [[lst] (lst S.srt S.by-number) drop true ] ]
  25 1 0
end`
	// Full compilation: no compile failure, no fallback island.
	a, _ := New()
	prog, reason, _, cerr := a.CompileCheck(src)
	if cerr != nil || reason != "" || prog == nil {
		t.Fatalf("expected full compilation, got compile failure: reason=%q err=%v", reason, cerr)
	}
	if strings.Contains(prog.Disassemble(), "FALLBACK") {
		t.Errorf("expected a full lowering, got an interpreter island:\n%s", prog.Disassemble())
	}
	// compile == interpret, and the property actually HOLDS on both surfaces
	// (a shared ok:false would match yet still be the miscompile).
	b, _ := New()
	got, _, gerr := b.RunCompiled(src)
	if noteCompileDefect(t, src, got, gerr) {
		return
	}
	c, _ := New()
	want, werr := c.RunInterp(src)
	if (werr == nil) != (gerr == nil) {
		t.Fatalf("error disagreement:\n  interp:   %v\n  compiled: %v", werr, gerr)
	}
	if werr == nil && fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("SILENT MISCOMPILE (compile != interpret):\n  interp:   %v\n  compiled: %v", want, got)
	}
	if !strings.Contains(fmt.Sprint(want), "true") {
		t.Fatalf("expected the property to hold under the interpreter, got %v", want)
	}
}
