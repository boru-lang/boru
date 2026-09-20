package lang

import (
	"fmt"
	"strings"
	"testing"
)

// TestBranchValueDefPromote pins the fix for a value-def bound to a 2-arm `if`
// result that is read MORE THAN ONCE — `def bi (if (raw gte n) [(n sub 1)] [raw])`
// then `bcount set bi ((bcount get bi) add 1)` (bucket-sort's bucket-index clamp).
// planValueDefLocals USED to skip every evBranch result (only ever dead-dropping a
// zero-ref merge, never promoting), so bi's single simulated-stack copy was consumed
// by the first use (`bcount get bi`) and the SECOND use (the `set` key) could not be
// seated — "stack discipline: operands of set not adjacent on top". A multiply-read
// single-value 2-arm if-result value-def is now PROMOTED to a frame local (stored
// after the branch, references re-push from the slot), exactly like a multiply-read
// call result. This is the bucket-sort leaf; compile == interpret MUST hold: compiles
// natively (no island), RunCompiledStrict == Run.
func TestBranchValueDefPromote(t *testing.T) {
	strict := []struct{ name, src, want string }{
		// the bucket clamp shape: an if-result value-def read by a get AND a set key.
		{"if-result value-def read twice (bucket clamp)",
			`def bcount (flex [0 0 0])
def n 3
def raw 5
def bi (if (raw gte n) [(n sub 1)] [raw])
def _ (bcount set bi ((bcount get bi) add 1)) end
(node bcount)`, "[[0 0 1]]"},
		// an if-result read across a get and an arithmetic use (two reads, no set).
		{"if-result value-def in two arithmetic reads",
			`def k (if (3 gt 1) [2] [0])
((k mul 10) add k)`, "[22]"},
		// inside an each-closure body (the real bucket nesting): if-result read twice.
		{"if-result value-def read twice inside an each body",
			`import module [
  def srt fn [[xs:List] [List] [
    def bcount (flex (iota 3 each [drop 0]))
    def n 3
    def _bc (iota 4 each [ var [[i]
      def raw (i add 1)
      def bi (if (raw gte n) [(n sub 1)] [raw])
      def _ (bcount set bi ((bcount get bi) add 1)) end
      0
    ] ])
    (node bcount)
  ]]
  export "M" {srt: srt/v}
] end ([0 0 0 0] M.srt)`, "[[0 1 3]]"},
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

// TestBranchArmResultSelfConsumed pins the fix for a value-def that is BOTH the
// arm RESULT and consumed as an operand WITHIN the same arm — the todo-api PUT
// handler shape `def t2 {…}; (todos set (id) t2) drop; t2`, where t2 is the
// `set` argument and the arm's returned value. planValueDefLocals USED to skip
// promotion for every fragment-internal arm result (assuming it stays on the
// arm's simulated stack); but the intra-arm `set` consumed t2's single sim slot,
// so nothing was left to seat as the result and the arm declined "branch leaves
// extra values" (out=opEvent, vm=0). Such a self-consumed arm-result value-def is
// now promoted to a frame local (stored once inside the arm, re-pushed for the
// operand use AND re-resolved as the arm out). compile == interpret MUST hold.
func TestBranchArmResultSelfConsumed(t *testing.T) {
	strict := []struct{ name, src, want string }{
		// the PUT shape: build t2, mutate a flex with it, return t2.
		{"arm result also a set argument",
			`def m (flex {b: 0})
def out (if (3 gt 1) [
  def t2 {a: 1}
  (m set "k" t2) drop
  t2
] [ {a: 0} ])
out`, "[{a:1}]"},
		// the value feeds a plain call operand AND is the arm result.
		{"arm result also a call operand",
			`def out (if (2 gt 1) [
  def xs [10 20 30]
  (size xs) drop
  xs
] [ [] ])
out`, "[[10 20 30]]"},
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

// TestBranchArmReadsEnclosingValueDef pins the Stage-3 fix for a NESTED `if`
// whose inner arm reads a value-def defined in an ENCLOSING scope — the
// mini-redis LRANGE shape `def start …; def cur …; if … [ … if (start gte n)
// [ … ] [ slice start n cur ] ]`. The enclosing value-defs are promoted to frame
// locals; their CONSUMING references rewrite to local pushes, but an arm-OUT
// designation referencing a promoted producer is left for lowerFragment to
// re-resolve — and the scopeFloor guard USED to treat that residual opEvent as a
// forbidden enclosing-scope read ("branch reads enclosing computation (Stage 3)").
// The guard now skips promoted operands (delivered via the frame, not the sim).
// compile == interpret MUST hold.
func TestBranchArmReadsEnclosingValueDef(t *testing.T) {
	strict := []struct{ name, src, want string }{
		{"nested if inner arm reads enclosing start/n/cur",
			`def cur [10 20 30]
def start 1
def out (if (2 gt 1) [
  def n (size cur)
  if (start gte n) [ "empty" ] [ slice start n cur ]
] [ "x" ])
out`, "[[20 30]]"},
		// the inner arm-OUT is itself an enclosing computed value-def (bx),
		// the shape rewritePromotedRefs leaves for lowerFragment to re-resolve.
		{"inner arm-out is an enclosing computed value-def",
			`def bx (10 add 5)
def out (if (3 gt 1) [
  def k (bx add 1)
  if (k gt 100) [ 0 ] [ bx ]
] [ 0 ])
out`, "[15]"},
	}
	for _, c := range strict {
		t.Run(c.name, func(t *testing.T) {
			a, _ := New()
			prog, reason, _, _ := a.CompileCheck(c.src)
			if prog == nil {
				t.Fatalf("must compile natively, declined: %q", reason)
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

// TestBranchArmEnclosingLoopStillFailsToCompile is the NEGATIVE guard for the Stage-3
// scopeFloor fix: skipping PROMOTED enclosing operands must NOT silence a genuine
// enclosing read that CANNOT be promoted. A nested `if` arm reading an enclosing
// LOOP result (variadic — never promoted) still declines "branch reads enclosing
// computation (Stage 3)", so the program does not compile. Proves precision, not blanket suppression.
func TestBranchArmEnclosingLoopStillFailsToCompile(t *testing.T) {
	src := `def f fn [[n:Integer] [Any] [ def xs (for 1 [ 7 ]) if (n gt 0) [ if (n gt 1) [ xs ] [ 0 ] ] [ 0 ] ]] 2 f`
	a, _ := New()
	prog, reason, _, _ := a.CompileCheck(src)
	declined := prog == nil || strings.Contains(prog.Disassemble(), "FALLBACK")
	if !declined {
		t.Errorf("expected the enclosing loop-variadic read to decline native compile; reason=%q", reason)
	}
	got, err := a.RunInterp(src)
	if err != nil {
		t.Fatalf("run (fallback): %v", err)
	}
	b, _ := New()
	want, _ := b.RunInterp(src)
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("fallback %v != interpreter %v", got, want)
	}
}
