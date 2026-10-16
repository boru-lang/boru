package lang

// NUR120 (measured 2026-09-05). A `=>` / `afn` lambda carries a
// `Returns=[Any]` PLACEHOLDER: the analyser infers past it for the result
// TYPE, but the interpreter enforces its COUNT at every dispatch boundary —
// the named call, the paren apply, the `apply` word, a member read, a
// Function param, and the callback seam (`each f/v`). The compiled lane
// dropped the placeholder outright (BuildFnBodyReturnsFn nilled it for the
// unit's RET, fnValueRetSpec declined the value) and, for callbacks, trimmed
// the closure body to its top value at compile time, so `def f ([x:Integer]
// => [x 1])  f 5` answered `5 1`, `each (…) [1 2]` answered `[1 1]`, and a
// NAMED fn's declared count went unenforced at the same seam (`def f fn
// [[n:Integer][Integer][n 1]]  each f/v [1 2]` → `[1 1]`) — all on the
// DEFAULT lane, exit 0. check.LambdaCountContract and the VM's
// checkClosureReturn count are the fix; these rows pin every path on both
// lanes, and the 1-value bodies alongside them so the contract cannot
// over-reach.

import (
	"strings"
	"testing"
)

// firstErrLine is the error's code + message line: the NUR118 blame rule
// puts the named-call error at the definition on the compiled lane and at
// the call on the interpreter, so the pin is code and message, not position.
func firstErrLine(err error) string {
	if err == nil {
		return ""
	}
	return strings.SplitN(err.Error(), "\n", 2)[0]
}

func TestLambdaReturnCountParity(t *testing.T) {
	rows := []struct{ src, note string }{
		// the named call, every lambda spelling
		{`def f ([x:Integer] => [x 1])  f 5`, "expected 1, got 2 — was 5 1"},
		{`def f ([x:Integer] => [x 1])  (f 5)`, "paren call"},
		{`def f ([x:Integer] => [print x])  f 5`, "expected 1, got 0 — was silent"},
		{`def f ([x:Integer] => [def y x])  f 5  9`, "expected 1, got 0 — was 9"},
		{`def f ([] => [1 2])  f`, "0-arg lambda, expected 1, got 2"},
		{`def f ([x:Integer] afn [x 1])  f 5`, "afn word"},
		// the apply word and a value applied in a paren
		{`5 ([x:Integer] => [x 1]) apply`, "apply — was 5 1"},
		{`def f ([x:Integer] => [x 1])  5 f/v apply`, "apply of a def-bound lambda"},
		{`(5 ([x:Integer] => [x 1]))`, "paren-applied value"},
		// the callback seam: the lambda's count, then a NAMED fn's declared count
		{`each ([x:Integer] => [x 1]) [1 2]`, "each: expected 1, got 2 — was [1 1]"},
		{`each ([x:Integer] => [print x]) [1 2]`, "each: expected 1, got 0 — was each_error"},
		{`def f ([x:Integer] => [x 1])  each f/v [1 2]`, "def-bound lambda callback"},
		{`def f ([x:Integer] => [x 1])  [1 2] each f/v`, "stack-form callback"},
		{`def f fn [[n:Integer][Integer][n 1]]  each f/v [1 2]`, "named fn, declared 1 — was [1 1]"},
		{`def f fn [[n:Integer][Integer][add n 1 0]]  each f/v [1 2]`, "named fn, a computed extra — was [0 0]"},
		// the contract's allowances, unchanged: two declared returns satisfied,
		// an unnamed input left at the frame bottom, no declaration at all, a
		// raw token body
		{`def f fn [[n:Integer][Integer Integer][n 1]]  each f/v [1 2]`, "[1 1] — was a compiled count error"},
		{`def f fn [[Integer][Integer][add 1 0]]  each f/v [1 2]`, "[1 1] — the unnamed-input allowance"},
		{`def f fn [[n:Integer][][n 1]]  each f/v [1 2]`, "[1 1] — no contract"},
		{`each [add 1 0] [1 2]`, "[1 1] — a raw body reads its top"},
		{`each [dup add] [1 2]`, "[2 4]"},
		// a tail call to a callee whose declared count differs from the
		// caller's is not a tail call: the caller's RET check runs on both
		// lanes (a lambda callee carries its 1-count placeholder and stays one)
		{`def g fn [[] [Integer Integer] [1 2]]  def f fn [[] [Integer] [g]]  f`, "f: expected 1, got 2 — on both lanes"},
		{`def g fn [[] [] [7]]  def f fn [[] [Integer] [g]]  f`, "7"},
		{`def g ([] => [7])  def f fn [[] [Integer] [g]]  f`, "7 — a lambda callee tail-calls under its placeholder count"},
		// the same callbacks INSIDE a fn unit: the contract is keyed in the
		// unit's own table (CompiledFn.ClosureRet) — keyed program-wide at
		// the main code's length it was lost, and `[1 1]` came back with
		// exit 0 (measured 2026-09-05, the in-unit face of NUR120)
		{`def f fn [[] [Any] [each (x:Integer => [x 1]) [1 2]]]  f`, "in-unit lambda callback — was [1 1]"},
		{`def f fn [[n:Integer] [Any] [each (x:Integer => [x n]) [1 2]]]  f 9`, "in-unit capturing lambda — was [9 9]"},
		{`def f fn [[] [Any] [def cb fn [[x:Integer][Integer][x 1]]  each cb/v [1 2]]]  f`, "in-unit named callback — was [1 1]"},
		{`def f fn [[] [Any] [[1 2] each (x:Integer => [x 1]) size]]  f`, "in-unit, consumed — was 2"},
		{`def f fn [[] [Any] [[1 2] each (x:Integer => [x add 1])]]  f`, "[2 3] — the in-unit 1-value body"},
		// 1-value bodies: nothing changes
		{`def f ([x:Integer] => [mul 2 x])  f 5`, "10"},
		{`def f ([x:Integer] => [x])  f 5`, "5"},
		{`def f ([] => [42])  f`, "42"},
		{`each ([x:Integer] => [mul 2 x]) [1 2]`, "[2 4]"},
		{`def f fn [[Integer][Integer][1]]  f 5`, "1 — the unnamed allowance by name"},
	}
	for _, c := range rows {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, c.src)
		if !compiled {
			t.Errorf("%q (%s): must compile natively; err=%v", c.src, c.note, errC)
			continue
		}
		if (errC == nil) != (errI == nil) {
			t.Errorf("%q (%s): one lane raised: compiled=%v/%v interp=%v/%v", c.src, c.note, gotC, errC, gotI, errI)
			continue
		}
		if errC != nil {
			if firstErrLine(errC) != firstErrLine(errI) {
				t.Errorf("%q (%s): error text differs:\n  compiled: %s\n  interp:   %s", c.src, c.note, firstErrLine(errC), firstErrLine(errI))
			}
			continue
		}
		requireParity(t, c.src, gotC, errC, gotI, errI)
	}
}

// The fn-VALUE seam. A handler that hands a FnDefInfo to InvokeCallbackFn
// runs it through CallBoru, whose return discipline is enforceCallBoruReturns:
// the declared TYPES are checked over the aligned residual and the COUNT is
// never raised — `walk`'s hooks, `each`/`fold` over a MAP, `filter`'s
// Function form. A compiled closure crosses the same seam through
// InvokeCallbackBody (core.IsRetTrim), and a compiled FnDefInfo's
// unit through enterCallbackUnit, so both lanes trim: the lambda's placeholder
// count never raises here (the token seam above is where it does), and a
// NAMED fn's declared count never raises here either — the compiled lane
// used to raise `expected 1 return value(s), got 0` for `walk … cb/v` over a
// 0-value body where the interpreter runs clean.
func TestLambdaReturnCountOnTheFnValueSeam(t *testing.T) {
	rows := []struct{ src, note string }{
		{`walk {mode:"breadth"} {a:1 b:{c:2}} (m:Any => [m.path print])`, "a 0-value hook lambda runs clean"},
		{`def cb fn [[m:Any][Integer][m.path print]]  walk {mode:"breadth"} {a:1} cb/v`, "a 0-value named hook runs clean — was a compiled count error"},
		{`def cb fn [[m:Any][Integer Integer][1]]  walk {mode:"breadth"} {a:1} cb/v`, "an under-count named hook runs clean — was a compiled count error"},
		{`def cb fn [[m:Any][Integer][m.path 1]]  walk {mode:"breadth"} {a:1} cb/v`, "{a:1}"},
		{`def m {a:1}  m each ([e:Any] => [1 2])`, "{a:2} — the map seam trims to the top"},
		{`def m {a:1}  m each ([e:Any] => [print 1])`, "each_error: body produced no result — the handler's own"},
		{`def m {a:1}  def cb fn [[e:Any][Integer][1 2]]  m each cb/v`, "{a:2}"},
		{`def m {a:1}  def cb fn [[e:Any][Integer]["s" 2]]  m each cb/v`, "{a:2} — types are checked head-to-head, the surplus sits at the bottom"},
		// a declared UNION return carries its domain as a pattern; a member
		// of the union passes on both lanes
		{`def cb fn [[m:Any][(Integer tor String)][m.path "s"]]  walk {mode:"breadth"} {a:1} cb/v`, "{a:1} — the union's pattern admits a String"},
		{`def m {a:1}  def cb fn [[e:Any][(Integer tor String)][7]]  m each cb/v`, "{a:7}"},
	}
	for _, c := range rows {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, c.src)
		if !compiled {
			t.Errorf("%q (%s): must compile natively; err=%v", c.src, c.note, errC)
			continue
		}
		requireParity(t, c.src, gotC, errC, gotI, errI)
	}
	// The TYPE half of the discipline still raises, under the NUR119 name
	// difference (`: return value 1` interpreted, `cb: return value 1`
	// compiled) — pinned on code and clause.
	for _, c := range []struct{ src, clause string }{
		{`def cb fn [[m:Any][Integer][m.path "s"]]  walk {mode:"breadth"} {a:1} cb/v`, "return value 1: expected Integer, got ProperString"},
		{`def m {a:1}  def cb fn [[e:Any][Integer][2 "s"]]  m each cb/v`, "return value 1: expected Integer, got ProperString"},
		// the union's PATTERN rejects a non-member (the pattern half of the
		// discipline, `expected Integer tor String` rather than `Any`)
		{`def cb fn [[m:Any][(Integer tor String)][true]]  walk {mode:"breadth"} {a:1} cb/v`, "return value 1: expected Integer tor String, got Boolean"},
		{`def m {a:1}  def cb fn [[e:Any][(Integer tor String)][true]]  m each cb/v`, "return value 1: expected Integer tor String, got Boolean"},
	} {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, c.src)
		if !compiled {
			t.Errorf("%q: must compile natively; err=%v", c.src, errC)
			continue
		}
		if errC == nil || errI == nil {
			t.Errorf("%q: both lanes must raise the type error; compiled=%v/%v interp=%v/%v", c.src, gotC, errC, gotI, errI)
			continue
		}
		for _, e := range []error{errC, errI} {
			if !strings.Contains(e.Error(), "[boru/type_error]") || !strings.Contains(e.Error(), c.clause) {
				t.Errorf("%q: want the type error on both lanes, got %v", c.src, e)
			}
		}
	}
}

// Two paths raise the same error under a DIFFERENT callee name: a member
// read (`m.f 5`) and a Function param called inside a user fn (`(g 5)`)
// re-label the value under the name it is read through on the interpreter
// (`g: expected …`, NUR119) where the compiled lane reports the value's own
// (empty) name. The count is enforced on both lanes — that is NUR120's
// claim — so the pin is the code and the count clause.
func TestLambdaReturnCountUnderARelabelledName(t *testing.T) {
	for _, src := range []string{
		`def m {f: ([x:Integer] => [x 1])}  m.f 5`,
		`def ap fn [[g:Function][Any][(g 5)]]  ap ([x:Integer] => [x 1])`,
	} {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
		if !compiled {
			t.Errorf("%q: must compile natively; err=%v", src, errC)
			continue
		}
		if errC == nil || errI == nil {
			t.Errorf("%q: both lanes must raise; compiled=%v/%v interp=%v/%v", src, gotC, errC, gotI, errI)
			continue
		}
		for _, e := range []error{errC, errI} {
			if !strings.Contains(e.Error(), "[boru/type_error]") || !strings.Contains(e.Error(), "expected 1 return value(s), got 2 — [5 1]") {
				t.Errorf("%q: want the count error on both lanes, got %v", src, e)
			}
		}
	}
}
