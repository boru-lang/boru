package lang

import (
	"fmt"
	"strings"
	"testing"
)

// Stage M2 landing tests — the fn-value frontier
// (design/STAGE3-INLINING-DESIGN-ROUND.0.md §6 Stage M2, sub-stages a–d).
//
//   - M2a `apply` over a param/captured fn (recursion.tsv:90-92): the apply
//     dispatch over a Function-typed CARRIER is elided with a PENDING apply on
//     the enclosing unit; the unit's finish lowers the whole-residual window
//     as OpCallDynApplyTop — applyHandler's unquote-then-apply — or REFUSES
//     (a pending apply can never silently compile the fn+args as data).
//   - M2b path-modifier map-stored fns (path-modifier.tsv:17-25,28,52-55):
//     the modifier words' GRADUAL dispatch (usurp / stack-args / forward-args
//     / force-arity over a dynamic fn read) records an OpCallNativePoly, so
//     the wrapper is built at RUN time over the real fn; the residual applies
//     it via the leading OpCallDynamic or the new TRAILING-window
//     OpCallDynamicMixed (`10 3 m.s/s` — the whole window islands verbatim so
//     a BarrierPos-0 stack-args wrapper collects its args exactly as the
//     interpreter).
//   - M2c (partial) Log.register (module-log.tsv:62,83): the sink fn is
//     STORED (CompileStoresFn) and invoked by the Go-side sink machinery,
//     never on the tape — a pure fn literal bakes as a const.
//   - M2d fn-value-as-operand (module-minilang.tsv:306-315, corpus-core:134):
//     `is`'s VALUE slot treats a fn operand as DATA (FnInertArgs — positional,
//     because a Function in its TYPE slot is a predicate the handler INVOKES
//     and must keep refusing), and a `/v` dispatch-mod marker survives the
//     check pass's carrier strip so an inline `(lambda)/v` parks exactly as
//     the runtime does. The two-lambda walk positives live in
//     TestWalkHookClosureCompiles.

// fnValueM2Native pins a fully-native compile (no island — the design round's
// no-new-islands rule) plus compiled/interpreted parity on value and error.
func fnValueM2Native(t *testing.T, name, src, want string) {
	t.Helper()
	prog, reason, _, cerr := mustNew(t).CompileCheck(src)
	if cerr != nil {
		t.Fatalf("%s: check error %v", name, cerr)
	}
	if prog == nil {
		t.Fatalf("%s: refused: %s", name, reason)
	}
	if strings.Contains(prog.Disassemble(), "FALLBACK") {
		t.Errorf("%s: expected native, got island:\n%s", name, prog.Disassemble())
	}
	gotC, compiled, errC := mustNew(t).RunCompiled(src)
	gotI, errI := mustNew(t).RunInterp(src)
	if !compiled {
		t.Fatalf("%s: did not run compiled", name)
	}
	if errC != nil || errI != nil {
		t.Fatalf("%s: run errs compiled=%v interp=%v", name, errC, errI)
	}
	if fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotI) != want {
		t.Errorf("%s: compiled=%v interp=%v want %s", name, gotC, gotI, want)
	}
}

// fnValueM2Refusal pins a sound refusal: the program does NOT compile (reason
// carries the expected substring), and the interpreter fallback agrees with a
// plain interpreted run on value and error taxonomy.
func fnValueM2Refusal(t *testing.T, name, src, wantReason string) {
	t.Helper()
	prog, reason, _, _ := mustNew(t).CompileCheck(src)
	if prog != nil {
		t.Fatalf("%s: compiled; want refusal", name)
	}
	if wantReason != "" && !strings.Contains(reason, wantReason) {
		t.Errorf("%s: refusal reason %q; want substring %q", name, reason, wantReason)
	}
	gotC, compiled, errC := mustNew(t).RunCompiled(src)
	gotI, errI := mustNew(t).RunInterp(src)
	if compiled {
		t.Errorf("%s: ran compiled; want interpreter fallback", name)
	}
	if (errC == nil) != (errI == nil) || codeOf(errC) != codeOf(errI) {
		t.Fatalf("%s: fallback err=[%s] interp err=[%s] (should agree)", name, codeOf(errC), codeOf(errI))
	}
	if fmt.Sprint(gotC) != fmt.Sprint(gotI) {
		t.Errorf("%s: fallback=%v interp=%v", name, gotC, gotI)
	}
}

// --- M2a — `apply` over a param fn ---------------------------------------

func TestApplyOverParamFnCompiles(t *testing.T) {
	// Legacy refusal+fallback-parity contract: pins the one-release
	// BORU_COMPILE_FALLBACK=1 hatch behavior (Stage J flipped the default
	// to compile_refused; migrate this contract or retire it with the hatch).
	t.Setenv("BORU_COMPILE_FALLBACK", "1")
	for _, c := range []struct{ name, src, want string }{
		{"recursion.tsv:91 — apply over a Function param",
			`def myfn ([x:Integer] => [x add 1000]) def runner fn [[myfn:Function v:Integer] [Integer] [v myfn/v apply]] def doubler ([x:Integer] => [x mul 2]) runner (doubler/v) 5`,
			"[10]"},
		{"recursion.tsv:90 — nested: param fn threaded through two units",
			`def g ([x:Integer] => [x add 1]) def h fn [[comp:Function v:Integer] [Integer] [v comp/v apply]] def t fn [[comp:Function] [Integer] [def a (5 comp/v h) def b (7 comp/v h) a add b]] (g/v t)`,
			"[14]"},
		{"recursion.tsv:92 — outer binding untouched by the param shadow",
			`def myfn ([x:Integer] => [x add 1000]) def runner fn [[myfn:Function v:Integer] [Integer] [v myfn/v apply]] def doubler ([x:Integer] => [x mul 2]) def _ (runner (doubler/v) 5) myfn 5`,
			"[1005]"},
		// Two args below the fn + a QUOTED (inline-lambda /v) callee: the
		// operand-ORDER pin (top arg → first param: x=3, y=10 → 3*2+10=16) and
		// the OpCallDynApplyTop unquote pin in one — OpCallDynTrailTop would
		// leave the parked value as data and raise a return-count error.
		{"2-arg apply, quoted callee: order + unquote pin",
			`def h fn [[comp:Function a:Integer b:Integer] [Integer] [a b comp/v apply]] h (([x:Integer y:Integer] => [x mul 2 add y])/v) 10 3`,
			"[16]"},
	} {
		fnValueM2Native(t, c.name, c.src, c.want)
	}

	// NEGATIVES — a pending apply that is NOT the whole body-tail window must
	// REFUSE (never compile the fn+args as unapplied data, never drop an apply
	// the interpreter performed), with faithful fallback.
	fnValueM2Refusal(t, "mid-body apply (result dropped, tail is a literal)",
		`def h fn [[comp:Function v:Integer] [Integer] [v comp/v apply drop 42]] h (([x:Integer] => [x add 1])/v) 5`,
		"apply of a dynamic fn value not at the body tail")
	fnValueM2Refusal(t, "double apply (two pendings, one window)",
		`def h fn [[c1:Function c2:Function v:Integer] [Integer] [v c1/v apply c2/v apply]] h (([x:Integer] => [x add 1])/v) (([x:Integer] => [x mul 3])/v) 5`,
		"apply of a dynamic fn value not at the body tail")
}

// --- M2b — path-modifier map-stored fns -----------------------------------

func TestPathModifierMapFnCompiles(t *testing.T) {
	// Legacy refusal+fallback-parity contract: pins the one-release
	// BORU_COMPILE_FALLBACK=1 hatch behavior (Stage J flipped the default
	// to compile_refused; migrate this contract or retire it with the hatch).
	t.Setenv("BORU_COMPILE_FALLBACK", "1")
	for _, c := range []struct{ name, src, want string }{
		{"path-modifier.tsv:17 — /u leading apply",
			`def m {a:add/v} end m.a/u 1 2`, "[3]"},
		{"path-modifier.tsv:18 — /u non-commutative (usurp order pin)",
			`def m {s:sub/v} end m.s/u 10 3`, "[7]"},
		{"path-modifier.tsv:19 — nested map path",
			`def o {m:{a:add/v}} end o.m.a/u 1 2`, "[3]"},
		{"path-modifier.tsv:23 — /f forward-args (order pin: -7, not 7)",
			`def m {s:sub/v} end m.s/f 10 3`, "[-7]"},
		{"path-modifier.tsv:28 — /2 force-arity",
			`def m {a:add/v} end m.a/2 1 2`, "[3]"},
		{"path-modifier.tsv:24 — /s TRAILING window (stack-args wrapper is BarrierPos 0)",
			`def m {s:sub/v} end 10 3 m.s/s`, "[7]"},
		{"path-modifier.tsv:54 — /s over a composed wrapper chain",
			`def m {s:sub/v} end 10 3 stack-args (force-arity 2 (m.s))`, "[7]"},
		{"path-modifier.tsv:55 — word-form chain of three wrappers",
			`def m {a:add/v} end force-arity 2 (usurp (forward-args (m.a))) 1 2`, "[3]"},
		// An inert trailing concrete fn value is DATA in both engines — the
		// trailing-window shape must not fire for a non-event fn (it requires
		// an event-produced fn value on top).
		{"inert parked fn stays data (no phantom apply)",
			`def f ([x:Integer] => [x add 1]) 10 3 f/v`, "[10 3 fn f(Integer)]"},
		// The reverted-shape pin (design §6 M2 revert criteria): the
		// dispatch-recovery operand-order shape must keep the interpreter's
		// operand order — 'x1', never the poly-lowered '1x' of the reverted
		// attempt. (It compiles natively on this branch; the pin is the ORDER.)
		{"reverted shape: (3 and \"x\") add 1 keeps interpreter order",
			`(3 and "x") add 1`, "[x1]"},
	} {
		fnValueM2Native(t, c.name, c.src, c.want)
	}

	// NEGATIVE — a non-fn member under a modifier errors identically in both
	// engines (usurp finds an Integer): refusal + byte-identical taxonomy.
	fnValueM2Refusal(t, "non-fn member under /u (signature_error parity)",
		`def m {a:5} end m.a/u 1 2`, "")
}

// --- M2c (partial) — Log.register stores its sink fn ----------------------

func TestLogRegisterSinkCompiles(t *testing.T) {
	// Legacy refusal+fallback-parity contract: pins the one-release
	// BORU_COMPILE_FALLBACK=1 hatch behavior (Stage J flipped the default
	// to compile_refused; migrate this contract or retire it with the hatch).
	t.Setenv("BORU_COMPILE_FALLBACK", "1")
	// module-log.tsv:62 — a pure fn literal bakes as a const operand
	// (CompileStoresFn); the sink registry mutates at RUN time only.
	fnValueM2Native(t, "module-log.tsv:62 — register a pure fn sink",
		`import "boru:log" ; Log.register (fn [[rec:Any] [] []]) tap/q info/q ; Log.sinks`,
		"[[console tap]]")

	// module-log.tsv:83 — the duplicate-name error surfaces byte-identically
	// from the compiled CALL_NATIVE.
	{
		src := `import "boru:log" ; Log.register (fn [[rec:Any] [] []]) console/q info/q`
		_, compiled, errC := mustNew(t).RunCompiled(src)
		_, errI := mustNew(t).RunInterp(src)
		if !compiled {
			t.Errorf("register duplicate: did not run compiled")
		}
		if codeOf(errC) != "sink-exists" || codeOf(errC) != codeOf(errI) {
			t.Errorf("register duplicate: compiled err=[%s] interp err=[%s]; want sink-exists on both", codeOf(errC), codeOf(errI))
		}
	}

	// A LEXICALLY CAPTURING sink fn (an enclosing fn's param in the body) is
	// not a bakeable const, and REFUSED until 2026-09-07 (the twenty-sixth
	// increment): a lambda VALUE unit now takes the fn path's residual
	// replay, so the capturing fn compiles as a closure unit and rides as an
	// OpPushClosure operand — which a NON-strict store word invokes through
	// the compiled runtime, as Patrun's stored closures already did (a strict
	// handler slot, service/add, still refuses a capturing fn). Measured: the
	// sink fires with the captured `p` on both lanes, in this run and in a
	// later run of the same lane.
	fnValueM2Native(t, "capturing sink fn compiles as a closure unit",
		`import "boru:log" ; def f fn [[p:String] [List] [Log.register (fn [[rec:Any] [] [p print]]) tap/q info/q Log.sinks]] f "x"`,
		"[[console tap]]")
}

// --- M2d — fn value as an INERT operand of `is` ---------------------------

func TestIsFnValueOperandCompiles(t *testing.T) {
	// Legacy refusal+fallback-parity contract: pins the one-release
	// BORU_COMPILE_FALLBACK=1 hatch behavior (Stage J flipped the default
	// to compile_refused; migrate this contract or retire it with the hatch).
	t.Setenv("BORU_COMPILE_FALLBACK", "1")
	for _, c := range []struct{ name, src, want string }{
		{"module-minilang.tsv:306 — matcher fn is its minted kind",
			`import "boru:minilang"  (+re/[a-z]+/) is (MiniLang.Re)`, "[true]"},
		{"module-minilang.tsv:307 — sibling kind rejected",
			`import "boru:minilang"  (+re/[a-z]+/) is (MiniLang.Gex)`, "[false]"},
		{"module-minilang.tsv:309 — fn value against the Function root",
			`import "boru:minilang"  (+re/[a-z]+/) is Function`, "[true]"},
		{"module-minilang.tsv:315 — def-aliased member type",
			`import "boru:minilang"  def Rex (MiniLang.Re)  (+re/[a-z]+/) is Rex`, "[true]"},
		// Inline `(lambda)/v` — the dispatch-mod marker must survive the check
		// pass's carrier strip (toCarrier) so it parks the lambda exactly as
		// the runtime does, instead of leaking into the residual as a phantom.
		{"module-minilang.tsv:314 — inline parked lambda as the is-value",
			`import "boru:minilang"  ([x:Any] => [x])/v is (MiniLang.Re)`, "[false]"},
		{"marker-drop parity: /v on a non-fn paren result is a no-op",
			`(1 add 2)/v`, "[3]"},
	} {
		fnValueM2Native(t, c.name, c.src, c.want)
	}

	// The former NEGATIVE, flipped 2026-08-28 and kept for what it got wrong.
	//
	// It read: a Function in `is`'s TYPE slot is a PREDICATE the handler
	// INVOKES (RunPredicate), so it must keep refusing — "whole-sig
	// CompileReadsFn would run the predicate against a baked shape". The
	// premise is right and the inference is not. Invoking a predicate is not
	// re-stepping it: the node rides as data and RunPredicate reaches the body
	// through the CALLBACK seam. What was actually wrong was one level down —
	// that seam declined every detached unit mid-run, so the body interpreted
	// inside the handler. Fixed in eng/go/vm_foreign_unit.go; the row now
	// compiles and answers what the interpreter answers, with no interpreter
	// entry (design/FULL-COMPILATION.0.md §6.3).
	fnValueM2Native(t, "predicate fn in the TYPE slot compiles",
		`def Positive fn [n:Integer Integer [if (n gt 0) [n] [None]]] 5 is Positive`,
		"[true]")
}
