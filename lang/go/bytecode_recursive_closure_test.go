package lang

import (
	"fmt"
	"strings"
	"testing"
)

// TestRecursionThroughClosure pins the fix for SELF-RECURSION through a closure
// body — a fn that calls itself inside an each/fold/scan body (the radix-msd
// `msd-go` recurse-into-each-bucket shape). The closure body is compiled in a
// throwaway PROBE state (recordClosureDispatch) so a compile failure can't pollute the
// real program; that throwaway used a fresh NewEmitState, which LOST the
// enclosing in-progress fn's unit — so the recursive call MISSED the fn-unit memo
// and RE-COMPILED the enclosing fn in the throwaway, where it re-hit the same
// closure and never registered its own residual ("fn go: body result of unknown
// provenance" → "code-body word each"). The probe is now SEEDED with the
// enclosing fn-unit tables (forkForProbe), so a recursive call is a memo HIT
// against the reserved unit — the same recursion guard the in-state non-closure
// path already relied on. The REAL compile resolves recursion naturally (the
// enclosing key is live in the real state). compile == interpret MUST hold:
// compiles natively (no island), RunCompiledStrict == Run.
func TestRecursionThroughClosure(t *testing.T) {
	strict := []struct{ name, src, want string }{
		// recursion inside an EACH body if (msd-go shape): the recursive call rides
		// in `def _x (if (blo lt hi) [ a hi blo go ] [ a ])` inside `iota _ each […]`.
		{"each-body recursion",
			`import module [
  def go fn [[hi:Integer lo:Integer a:FlexList] [FlexList] [
    def cnt ((hi sub lo) add 1)
    if (cnt lte 1) [ a ] [
      def _r (iota 2 each [ var [[v] def blo (lo add v) def _x (if (blo lt hi) [ a hi blo go ] [ a ]) 0 ] ])
      a
    ]
  ]]
  def srt fn [[xs:List] [List] [ def arr (flex xs) def _s (arr 0 ((xs size) sub 1) go) (node arr) ]]
  export "M" {srt: srt/v}
] end ([5 3 1 2] M.srt)`, "[[5 3 1 2]]"},
		// recursion inside a FOLD body (different higher-order word, same closure path).
		{"fold-body recursion",
			`import module [
  def go fn [[n:Integer a:FlexList] [FlexList] [
    if (n lte 0) [ a ] [
      def _r (0 fold [ var [[acc v] def _x (if (v lt n) [ a (n sub 1) go ] [ a ]) acc ] ] [0 1])
      a
    ]
  ]]
  def srt fn [[xs:List] [List] [ def arr (flex xs) def _s (arr 2 go) (node arr) ]]
  export "M" {srt: srt/v}
] end ([7 8 9] M.srt)`, "[[7 8 9]]"},
	}
	for _, c := range strict {
		t.Run(c.name, func(t *testing.T) {
			a, _ := New()
			prog, reason, _, _ := a.CompileCheck(c.src)
			if prog == nil {
				t.Fatalf("must compile natively, declined: %q", reason)
			}
			if strings.Contains(prog.Disassemble(), "FALLBACK") {
				t.Errorf("%s must compile native (no island)", c.name)
			}
			got, err := a.RunCompiledStrict(c.src)
			if err != nil {
				t.Fatalf("RunCompiledStrict: %v", err)
			}
			b, _ := New()
			want, _ := b.RunInterp(c.src)
			if fmt.Sprint(got) != fmt.Sprint(want) {
				t.Errorf("compiled %v != interpreter %v (MISCOMPILE)", got, want)
			}
			if fmt.Sprint(got) != c.want {
				t.Errorf("got %v, want %s", got, c.want)
			}
		})
	}
}
