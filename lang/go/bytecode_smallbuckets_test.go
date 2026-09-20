package lang

import (
	"fmt"
	"strings"
	"testing"
)

// TestReachComputedSegmentLowers — a `reach` key list whose segments include a
// deferred COMPUTED paren (`reach 5 [a (add 1 2) c]`, `reach 0 [x (k)]`) now
// compiles. The paren is stored unevaluated and re-run at APPLY time over the
// shared registry, never stepped at the VM pointer, so the key list bakes as a
// const (isInertConstMember admits a ParenExpr riding inside a never-evaluated
// compound) + CALL_NATIVE. The NEGATIVE half: a body whose computed segment is
// NOT inert (drags in a carrier) keeps the conservative compile failure — covered by the
// const-bake gate; here we pin the positive shapes and parity.
func TestReachComputedSegmentLowers(t *testing.T) {
	cases := []struct {
		src  string
		want string
	}{
		{`reach 5 [a (add 1 2) c]`, "5.a.(add 1 2).c"},
		{`def p {x:{y:9}}  def k "y"  p (reach 0 [x (k)]) apply`, "9"},
	}
	for _, c := range cases {
		a, _ := New()
		prog, reason, _, cerr := a.CompileCheck(c.src)
		if cerr != nil || prog == nil {
			t.Errorf("%q did not compile: reason=%q err=%v", c.src, reason, cerr)
			continue
		}
		if strings.Contains(prog.Disassemble(), "FALLBACK") {
			t.Errorf("%q islanded; expected a native CALL_NATIVE bake", c.src)
		}
		b, _ := New()
		gotC, compiled, errC := b.RunCompiled(c.src)
		if noteCompileDefect(t, c.src, gotC, errC) {
			continue
		}
		d, _ := New()
		gotI, _ := d.RunInterp(c.src)
		if !compiled || errC != nil || fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotC) != "["+c.want+"]" {
			t.Errorf("%q: parity broke: compiled=%v gotC=%v errC=%v gotI=%v want=[%s]", c.src, compiled, gotC, errC, gotI, c.want)
		}
	}
}

// TestTimeUtilBodyNotEager pins the latent runtime bug the NoEvalArgs fix closed:
// `TimeUtil.timeout 1000 [body]` (and interval) must STORE the body as code, not
// sub-Run it at construction. The module wrapper's trivial delegation runs
// execMatch on the inner sig, whose auto-eval gate keys only off NoEvalArgs — so
// before the fix the body was evaluated eagerly. A body naming an undefined word
// would raise if evaluated; correct behaviour returns a handle with no error.
func TestTimeUtilBodyNotEager(t *testing.T) {
	for _, s := range []string{
		`import "boru:time-util"  typeof (TimeUtil.timeout 100000 [totally-undefined-xyz])`,
		`import "boru:time-util"  typeof (TimeUtil.interval 100000 [totally-undefined-xyz])`,
	} {
		// Interpreter: the undefined word must NOT raise (body is inert code).
		b, _ := New()
		if _, err := b.RunInterp(s); err != nil {
			t.Errorf("%q: body was eagerly evaluated (interp): %v", s, err)
		}
		// Compiled path: identical — no error, same handle.
		c, _ := New()
		gotC, _, errC := c.RunCompiled(s)
		if noteCompileDefect(t, s, gotC, errC) {
			continue
		}
		if errC != nil {
			t.Errorf("%q: body was eagerly evaluated (compiled): %v", s, errC)
		}
		d, _ := New()
		gotI, _ := d.RunInterp(s)
		if fmt.Sprint(gotC) != fmt.Sprint(gotI) {
			t.Errorf("%q: parity broke: compiled=%v interp=%v", s, gotC, gotI)
		}
	}
}

// TestTimeUtilQuotedBodyLowers — the TimeUtil timeout/interval/cancel rows with a
// computed code body bake natively (the inner LIST sig's NoEvalArgs keeps the
// body raw; noEvalBodiesInert bakes it; hasUncoveredQuoteArg stops the QuoteArgs
// compile failure from double-declining a position already covered by NoEvalArgs).
func TestTimeUtilQuotedBodyLowers(t *testing.T) {
	cases := []struct {
		src  string
		want string
	}{
		{`import "boru:time-util"  typeof (TimeUtil.timeout 1000 [1 add 2])`, "Timeout"},
		{`import "boru:time-util"  typeof (TimeUtil.interval 1000 [1 add 2])`, "Interval"},
		{`import "boru:time-util"  100 (TimeUtil.cancel (TimeUtil.timeout 1000 [1 add 2])) add 23`, "123"},
	}
	for _, c := range cases {
		a, _ := New()
		prog, reason, _, cerr := a.CompileCheck(c.src)
		if cerr != nil || prog == nil {
			t.Errorf("%q did not compile: reason=%q err=%v", c.src, reason, cerr)
			continue
		}
		if strings.Contains(prog.Disassemble(), "FALLBACK") {
			t.Errorf("%q islanded; expected a native bake", c.src)
		}
		b, _ := New()
		gotC, compiled, errC := b.RunCompiled(c.src)
		if noteCompileDefect(t, c.src, gotC, errC) {
			continue
		}
		d, _ := New()
		gotI, _ := d.RunInterp(c.src)
		if !compiled || errC != nil || fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotC) != "["+c.want+"]" {
			t.Errorf("%q: parity broke: compiled=%v gotC=%v errC=%v gotI=%v want=[%s]", c.src, compiled, gotC, errC, gotI, c.want)
		}
	}
}
