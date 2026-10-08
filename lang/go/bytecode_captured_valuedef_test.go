package lang

import (
	"fmt"
	"strings"
	"testing"
)

// TestCapturedUserCallValueDefPromote pins the fix for a USER-call value-def whose
// result is captured by a CLOSURE — `def mx (arr hmax)` (hmax a user fn, one
// result) read inside an `each` body (`if (i lt mx) …`). The interpreter binds the
// call result to `mx` OFF the residual and the closure captures the binding, so the
// arm nets only its single trailing result. The compiler, however, left the
// user-call result loose on the simulated stack BELOW the arm result (the
// "leave single-use user call on the stack" Stage-3 case shadowed the value-def
// promotion that already fires for a NATIVE captured value-def), so the arm declined
// "branch leaves extra values". planValueDefLocals now PROMOTES a closure-captured
// user-call value-def to a frame local (the capture re-pushes from the slot at
// OpPushClosure). This is the radix-lsd-sort `def mx (lst list-max)` leaf —
// reproduced inline because list-max is module-private. compile == interpret MUST
// hold: compiles natively (no fallback island), RunCompiledStrict == Run.
func TestCapturedUserCallValueDefPromote(t *testing.T) {
	strict := []struct{ name, src, want string }{
		// the radix shape: a user-call value-def in an `if` else-arm, captured by an
		// each-closure, with a trailing `convert List` arm result.
		{"captured user-call value-def in an each-closure (radix list-max shape)",
			`import module [
  def hmax fn [[a:FlexList] [Integer] [ a get 0 ]]
  def srt fn [[xs:List] [List] [
    def n (xs size)
    if (n lte 1) [ xs ] [
      def arr (flex xs)
      def mx (arr hmax)
      def _ (iota 3 each [ var [[i] if (i lt mx) [ arr set i mx end 0 ] [0] ] ])
      (node arr)
    ]
  ]]
  export "M" {srt: srt/v}
] end ([3 1 2] M.srt)`, "[[3 3 3]]"},
		// the captured user-call value read TWICE inside the closure body (still one
		// frame local, re-pushed per capture-read at run time).
		{"captured user-call value-def read twice in the closure",
			`import module [
  def hmax fn [[a:FlexList] [Integer] [ a get 0 ]]
  def srt fn [[xs:List] [List] [
    def arr (flex xs)
    def mx (arr hmax)
    def _ (iota 3 each [ var [[i] if (i lt mx) [ arr set i (mx add mx) end 0 ] [0] ] ])
    (node arr)
  ]]
  export "M" {srt: srt/v}
] end ([4 1 2] M.srt)`, "[[8 8 8]]"},
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
