package native

import (
	"strings"
	"testing"

	parser "github.com/boru-lang/boru/parser/go"
)

// Wave-3 coverage for native_control.go (case / for / do / error / clause-
// list if), conditional.go, forloop.go, listops.go, native_accessor.go
// (has / getr / dotr), native_valof.go (valof / apply / rebind / usurp /
// stack-args / forward-args / force-arity), getpath.go, and istype.

func w3Misc(t *testing.T, src string) (string, error) {
	t.Helper()
	return w3Run(t, w3TypeReg(), src)
}

func w3MiscWant(t *testing.T, src, want string) {
	t.Helper()
	got, err := w3Misc(t, src)
	if err != nil {
		t.Fatalf("%q: unexpected error: %v", src, err)
	}
	if got != want {
		t.Errorf("%q = %q, want %q", src, got, want)
	}
}

func w3MiscErr(t *testing.T, src, substr string) {
	t.Helper()
	_, err := w3Misc(t, src)
	if err == nil {
		t.Fatalf("%q: expected error containing %q, got none", src, substr)
	}
	if !strings.Contains(err.Error(), substr) {
		t.Errorf("%q: error %q does not contain %q", src, err.Error(), substr)
	}
}

// w3MiscCheck runs src in check mode and returns the residual stack.
func w3MiscCheck(t *testing.T, src string) ([]Value, *Registry) {
	t.Helper()
	r := w3TypeReg()
	r.Check.Mode = true
	toks, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	out, runErr := NewTop(r).Run(toks)
	if runErr != nil {
		t.Fatalf("check run %q: %v", src, runErr)
	}
	return out, r
}

// ---- case ----

func TestW3Case(t *testing.T) {
	w3MiscWant(t, `case 2 [1 "one" 2 "two" "many"]`, `'two'`)
	w3MiscWant(t, `case 9 [1 "one" 2 "two" "many"]`, `'many'`)
	w3MiscWant(t, `case 9 [1 "one" 2 "two"]`, ``) // no match, no default
	w3MiscWant(t, `case red/q [red/q "warm" blue/q "cool"]`, `'warm'`)
	// Type literals, def'd newtypes, predicate refinements, code bodies.
	w3MiscWant(t, `case "x" [Integer "int" String "str" "other"]`, `'str'`)
	w3MiscWant(t, `def Pos refine Integer def x:Pos 5 case x [Pos "pos" Integer "plain" "other"]`, `'pos'`)
	w3MiscWant(t, `def Big (Integer gt 10) case 50 [Big "big" Integer "small"]`, `'big'`)
	w3MiscWant(t, `case 5 [[gt 3] "big" "small"]`, `'big'`)
	w3MiscWant(t, `case 2 [[gt 3] "big" "small"]`, `'small'`)
	w3MiscWant(t, `case 5 [[gt 3] [mul 10] "small"]`, `50`)
	// A paren-group clause match evaluates inline.
	w3MiscWant(t, `case 2 [(1 add 1) "two" "other"]`, `'two'`)
	// Code-body scrutinee: the LAST result dispatches.
	w3MiscWant(t, `case [1 add 1] [2 "two" "other"]`, `'two'`)
	// Stack-value form.
	w3MiscWant(t, `5 case [5 "five" "other"]`, `'five'`)
	// Errors: a 0-net value expression, a non-list clause arg.
	w3MiscErr(t, `def f fn [[x:Integer] [] []] case [f 1] [2 "two" "other"]`, "case_error")
	w3MiscErr(t, `case 1 Integer`, "case_error")
}

func TestW3CaseCheckMode(t *testing.T) {
	// Plain check (no emit): caseReturnsFn joins the clause blocks.
	out, _ := w3MiscCheck(t, `case 2 [1 "one" 2 "two" "many"]`)
	if len(out) == 0 {
		t.Error("check case: expected a residual carrier")
	}
	// Code-body scrutinee keeps the dynamic Any in plain check.
	out, _ = w3MiscCheck(t, `case [1 add 1] [2 "two" "other"]`)
	if len(out) == 0 {
		t.Error("check case code-body: expected a residual carrier")
	}
	// Block-less shapes fall to the dynamic Any.
	out, _ = w3MiscCheck(t, `case 2 [1]`)
	if len(out) == 0 {
		t.Error("check case short clause list: expected a carrier")
	}
}

func TestW3CaseMatchHandlerDirect(t *testing.T) {
	// __casematch is the compiled desugar's guard: UnifyR(match, value).
	r := w3TypeReg()
	out, err := caseMatchHandler([]Value{NewInteger(2), NewInteger(2)}, nil, nil, r)
	if err != nil || Canon(out) != `true` {
		t.Errorf("__casematch 2 2 = %v (%v), want true", Canon(out), err)
	}
	out, err = caseMatchHandler([]Value{NewInteger(3), NewInteger(2)}, nil, nil, r)
	if err != nil || Canon(out) != `false` {
		t.Errorf("__casematch 3 2 = %v (%v), want false", Canon(out), err)
	}
}

// ---- for / forloop ----

func TestW3ForLoop(t *testing.T) {
	w3MiscWant(t, `for 3 [i]`, `0 1 2`)
	w3MiscWant(t, `for [1 4] [i]`, `1 2 3`)
	w3MiscWant(t, `for [0 10 3] [i]`, `0 3 6 9`)
	w3MiscWant(t, `for [5 0 -2] [i]`, `5 3 1`)
	w3MiscWant(t, `for [3 3] [i]`, ``)    // empty ascending span
	w3MiscWant(t, `for [0 3 -1] [i]`, ``) // wrong-direction span
	w3MiscErr(t, `for [0 3 0] [i]`, "step cannot be zero")
	w3MiscErr(t, `for [1 2 3 4] [i]`, "expected 1-3 elements")
	w3MiscErr(t, `for [1.5] [i]`, "expected a concrete integer")
	w3MiscErr(t, `for [1 2.5] [i]`, "expected a concrete integer")
	w3MiscErr(t, `for [1 5 1.5] [i]`, "expected a concrete integer")
	// A type-literal body is loud (direct call — dispatch gates it away).
	r := w3TypeReg()
	if _, err := runForLoop(r, 0, 3, 1, "i", NewTypeLiteral(TList)); err == nil ||
		!strings.Contains(err.Error(), "body must be a concrete list") {
		t.Errorf("runForLoop List body: got %v", err)
	}
}

func TestW3ForLoopCheckMode(t *testing.T) {
	// Check mode drives forCarrierAnalyse + loopIterations.
	for _, src := range []string{
		`for 3 [i]`,
		`for [1 4] [i]`,
		`for [0 10 3] [i mul 2]`,
		`for [5 0 -2] [i]`,
		`for 0 [i]`, // statically-zero iteration count
	} {
		if _, r := w3MiscCheck(t, src); r == nil {
			t.Fatalf("check %q failed", src)
		}
	}
}

func TestW3LoopIterations(t *testing.T) {
	cases := []struct{ start, end, step, want int64 }{
		{0, 3, 1, 3},
		{0, 3, 0, -1}, // zero step is non-static
		{3, 3, 1, 0},
		{0, 10, 3, 4},
		{5, 0, -2, 3},
		{0, 5, -1, 0},
	}
	for _, c := range cases {
		if got := loopIterations(c.start, c.end, c.step); got != c.want {
			t.Errorf("loopIterations(%d,%d,%d) = %d, want %d",
				c.start, c.end, c.step, got, c.want)
		}
	}
}

// ---- clause-list if ----

func TestW3IfClauseList(t *testing.T) {
	w3MiscWant(t, `if [[1 gt 0] 'yes' 'no']`, `'yes'`)
	w3MiscWant(t, `if [[1 lt 0] 'yes' 'no']`, `'no'`)
	w3MiscWant(t, `if [false 'a' true 'b' 'c']`, `'b'`)
	w3MiscWant(t, `if ['only']`, `'only'`)
	w3MiscWant(t, `if [[1 lt 0] 'a']`, ``) // unmatched, no else
	// Body splice: a code-body branch evaluates.
	w3MiscWant(t, `if [true [1 add 2] 'no']`, `3`)
	// Type-literal clause list (direct call).
	r := w3TypeReg()
	if _, err := ifListHandler([]Value{NewTypeLiteral(TList)}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "concrete list") {
		t.Errorf("if over List literal: got %v", err)
	}
}

func TestW3IfCheckMode(t *testing.T) {
	// ifListReturnsFn joins the clause bodies (with and without else);
	// if2ReturnsFn covers the 2-arg form.
	for _, src := range []string{
		`if [[1 gt 0] 'yes' 'no']`,
		`if [[1 gt 0] 'yes']`,
		`def c (1 gt 0) if c ['yes']`,
		`def c (1 gt 0) if c ['yes'] ['no']`,
	} {
		if _, r := w3MiscCheck(t, src); r == nil {
			t.Fatalf("check %q failed", src)
		}
	}
	// A non-concrete clause list falls back to the Any carrier.
	r := w3TypeReg()
	r.Check.Mode = true
	out := ifListReturnsFn([]Value{NewCarrier(TList)}, r)
	if len(out) != 1 || !out[0].Parent.Equal(TAny) {
		t.Errorf("ifListReturnsFn carrier = %v, want Any carrier", out)
	}
}

// ---- do / error ----

func TestW3DoAndError(t *testing.T) {
	w3MiscWant(t, `do [1 add 2]`, `3`)
	w3MiscWant(t, `do {a:[1 add 2] b:{c:[2 mul 3]}}`, `{a:3 b:{c:6}}`)
	// A raising body surfaces as an Error VALUE, caught by `error`.
	w3MiscWant(t, `do [raise bad_x 'boom'] error [drop 'fallback']`, `'fallback'`)
	// Success pass-through skips the handler.
	w3MiscWant(t, `do [42] error [drop 'fallback']`, `42`)
	// An empty handler keeps the error as the result (pass-through).
	got, err := w3Misc(t, `do [raise bad_x 'boom'] error []`)
	if err != nil {
		t.Fatalf("error []: %v", err)
	}
	if !strings.Contains(got, "boom") {
		t.Errorf("error [] should pass the error through, got %q", got)
	}
	// A handler that errors propagates.
	w3MiscErr(t, `do [raise bad_x 'boom'] error [quux]`, "quux")
	// Type-literal guards (direct calls).
	r := w3TypeReg()
	if _, herr := doListHandler([]Value{NewTypeLiteral(TList)}, nil, nil, r); herr == nil ||
		!strings.Contains(herr.Error(), "concrete list") {
		t.Errorf("do over List literal: got %v", herr)
	}
	if _, herr := errorHandler([]Value{NewTypeLiteral(TList), NewInteger(1)}, nil, nil, r); herr == nil ||
		!strings.Contains(herr.Error(), "concrete list") {
		t.Errorf("error over List literal: got %v", herr)
	}
	// doEvalList catches a body error as an Error value.
	out, derr := doEvalList(r, []Value{NewWord("no-such-word-w3")})
	if derr != nil || len(out) != 1 || !IsError(out[0]) {
		t.Errorf("doEvalList error body = %v (%v), want one Error value", out, derr)
	}
	out, derr = doEvalList(r, []Value{NewInteger(7)})
	if derr != nil || Canon(out) != `7` {
		t.Errorf("doEvalList [7] = %v (%v), want 7", Canon(out), derr)
	}
}

func TestW3DoCheckMode(t *testing.T) {
	// doListReturnsFn: full residual, empty body, raising body, def'd word.
	out, _ := w3MiscCheck(t, `do [10 20 30]`)
	if len(out) != 3 {
		t.Errorf("check do [10 20 30]: residual %v, want 3 values", out)
	}
	out, _ = w3MiscCheck(t, `do []`)
	if len(out) != 0 {
		t.Errorf("check do []: residual %v, want empty", out)
	}
	// A raising body models the residual as an Error carrier.
	out, _ = w3MiscCheck(t, `do [raise bad_x 'y']`)
	if len(out) != 1 || !out[0].Parent.ConformsTo(TError) {
		t.Errorf("check do [raise]: residual %v, want Error carrier", out)
	}
}

// ---- listops: push / pop / unshift / shift ----

func TestW3ListOps(t *testing.T) {
	w3MiscWant(t, `push 99 [1 2 3]`, `[1 2 3 99]`)
	w3MiscWant(t, `pop [1 2 3]`, `[1 2] 3`)
	w3MiscWant(t, `unshift 99 [1 2 3]`, `[99 1 2 3]`)
	w3MiscWant(t, `shift [1 2 3]`, `[2 3] 1`)
	w3MiscErr(t, `pop []`, "empty list")
	w3MiscErr(t, `shift []`, "empty list")
}

func TestW3ListOpsFlex(t *testing.T) {
	// push/unshift return the mutated flex value; `node f` then renders
	// the frozen copy — both stay on the stack.
	w3MiscWant(t, `def f (flex [1 2])  push 3 f  end  node f`, `(flex [1 2 3]) [1 2 3]`)
	w3MiscWant(t, `def f (flex [1 2])  unshift 0 f  end  node f`, `(flex [0 1 2]) [0 1 2]`)
	w3MiscWant(t, `def f (flex [1 2])  pop f  end`, `(flex [1]) 2`)
	w3MiscWant(t, `def f (flex [1 2])  shift f  end`, `(flex [2]) 1`)
	w3MiscErr(t, `def f (flex [])  pop f`, "empty list")
	w3MiscErr(t, `def f (flex [])  shift f`, "empty list")
}

// ---- has / getr / dotr ----

func TestW3Has(t *testing.T) {
	w3MiscWant(t, `{a:None} has 'a'`, `true`)
	w3MiscWant(t, `{a:1} has 'b'`, `false`)
	w3MiscWant(t, `{a:1} has 'a'`, `true`)
	w3MiscWant(t, `[10 20] has 1`, `true`)
	w3MiscWant(t, `[10 20] has 5`, `false`)
	w3MiscWant(t, `none has a/q`, `false`)
	// Class instances.
	w3MiscWant(t, `def P class {x:1} def p (make P {x:5}) p has x/q`, `true`)
	w3MiscWant(t, `def P class {x:1} def p (make P {x:5}) p has y/q`, `false`)
	// Store (the context store).
	w3MiscWant(t, `context set 'k' 1 end context has 'k'`, `true`)
	w3MiscWant(t, `context has 'nope-w3'`, `false`)
	// A type-literal container answers false, it never raises.
	out, err := hasNodeHandler([]Value{NewAtom("a"), NewTypeLiteral(TMap)}, nil, nil, nil)
	if err != nil || Canon(out) != `false` {
		t.Errorf("has over Map literal = %v (%v), want false", Canon(out), err)
	}
	out, err = hasObjectHandler([]Value{NewAtom("a"), NewTypeLiteral(TClass)}, nil, nil, nil)
	if err != nil || Canon(out) != `false` {
		t.Errorf("has over Class literal = %v (%v), want false", Canon(out), err)
	}
	out, err = hasStoreHandler([]Value{NewAtom("a"), NewTypeLiteral(TStore)}, nil, nil, nil)
	if err != nil || Canon(out) != `false` {
		t.Errorf("has over Store literal = %v (%v), want false", Canon(out), err)
	}
}

func TestW3Getr(t *testing.T) {
	w3MiscWant(t, `{a:1} getr 'a'`, `1`)
	w3MiscWant(t, `[10 20] getr 1`, `20`)
	w3MiscErr(t, `{a:1} getr 'b'`, "not found")
	w3MiscErr(t, `[10 20] getr 5`, "out of bounds")
	w3MiscErr(t, `none getr 'a'`, "parent is None")
	// dotr quotes a bare-word key; a miss raises.
	w3MiscWant(t, `{a:1} dotr a`, `1`)
	w3MiscErr(t, `{a:1} dotr b`, "not found")
	// Strict class-instance reads.
	w3MiscWant(t, `def P class {x:1} (make P {x:5}) dotr x`, `5`)
	w3MiscErr(t, `def P class {x:1} (make P {x:5}) dotr y`, "not found")
	// Type-literal containers are loud for the strict readers.
	r := w3TypeReg()
	if _, err := getrMapHandler([]Value{NewAtom("a"), NewTypeLiteral(TMap)}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "type literal") {
		t.Errorf("getr over Map literal: got %v", err)
	}
	if _, err := getrObjectHandler([]Value{NewAtom("a"), NewTypeLiteral(TClass)}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "type literal") {
		t.Errorf("getr over Class literal: got %v", err)
	}
}

func TestW3GetrCheckMode(t *testing.T) {
	// returnsGetrIndexChecked flags a provably out-of-range literal index.
	_, r := w3MiscCheck(t, `[10 20] getr 1`)
	if len(r.Check.Diagnostics) != 0 {
		t.Errorf("in-range getr should not diagnose: %+v", r.Check.Diagnostics)
	}
	out := returnsGetrIndexChecked(nil, r)
	if len(out) != 1 || !out[0].Parent.Equal(TAny) {
		t.Errorf("returnsGetrIndexChecked() = %v, want Any carrier", out)
	}
}

// ---- ref / apply / rebind / usurp / stack-args / forward-args / force-arity ----

func TestW3RefApply(t *testing.T) {
	// /v keeps the paren-produced Function as data — without it the
	// named Function auto-dispatches against the stack before apply.
	w3MiscWant(t, `def inc fn [[n:Integer][Integer][n add 1]]  5 ((valof inc)/v) apply`, `6`)
	w3MiscWant(t, `def inc2 fn [[n:Integer][Integer][n add 1]]  5 inc2/v apply`, `6`)
	w3MiscWant(t, `def z fn [[][Integer][42]]  z/v apply`, `42`)
	w3MiscErr(t, `valof no-such-w3`, "not bound")
	// `valof` is TOTAL over binding kinds: a non-fn binding is the identity,
	// not a rejection. Only an unbound name declines (the row above).
	w3MiscWant(t, `def x 5  valof x`, `5`)
	// apply rejections (direct calls).
	if _, err := applyHandler([]Value{NewInteger(5)}, nil, nil, nil); err == nil ||
		!strings.Contains(err.Error(), "expected Function") {
		t.Errorf("apply non-fn: got %v", err)
	}
	// A Function-typed value whose payload is not an FnDefInfo (the
	// sealed-payload rule forbids arbitrary Data, so rewrap an integer).
	bogus := NewInteger(5)
	bogus.Parent = TFunction
	if _, err := applyHandler([]Value{bogus}, nil, nil, nil); err == nil ||
		!strings.Contains(err.Error(), "no FnDefInfo") {
		t.Errorf("apply bogus payload: got %v", err)
	}
}

func TestW3ApplyRebindReach(t *testing.T) {
	// Stack form — apply is stack-only in BOTH overloads (NUR098's fix);
	// the forward spellings are pinned as compile failures in lang/spec/apply.tsv §5.
	w3MiscWant(t, `def p {name:'ada'}  p $.name apply`, `'ada'`)
	w3MiscWant(t, `def p {name:'ada'}  typeof (rebind $.name p)`, `Reach`)
	w3MiscWant(t, `getpath $.a.b {a:{b:7}}`, `7`)
	w3MiscWant(t, `getpath 'a.b' {a:{b:7}}`, `7`)
	// Rejections: a non-Reach where a lens is expected (direct calls).
	r := w3TypeReg()
	if _, err := applyReachHandler([]Value{NewInteger(1), NewInteger(2)}, nil, nil, r); err == nil {
		t.Error("applyReach with non-Reach should error")
	}
	if _, err := rebindHandler([]Value{NewInteger(1), NewInteger(2)}, nil, nil, r); err == nil {
		t.Error("rebind with non-Reach should error")
	}
	if _, err := getpathReachHandler([]Value{NewInteger(1), NewInteger(2)}, nil, nil, r); err == nil {
		t.Error("getpath with non-Reach should error")
	}
}

func TestW3Usurp(t *testing.T) {
	// Value form and by-name form.
	w3MiscWant(t, `def f fn [[x:Integer] [Integer] [mul x 2]]  3 (usurp f) apply`, `6`)
	w3MiscWant(t, `def sub2 fn [[a:Integer b:Integer][Integer][a sub b]]  10 3 sub2/uv apply`, `7`)
	w3MiscErr(t, `usurp no-such-w3`, "not bound")
	w3MiscErr(t, `def x 5 usurp x`, "requires a function word")
	// A non-fn VALUE is rejected by the value form (direct call).
	r := w3TypeReg()
	if _, err := usurpHandler([]Value{NewInteger(5)}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "requires a function value") {
		t.Errorf("usurp non-fn value: got %v", err)
	}
}

func TestW3StackForwardArgs(t *testing.T) {
	w3MiscWant(t, `def inc fn [[n:Integer][Integer][n add 1]]  5 (stack-args inc)/v apply`, `6`)
	w3MiscWant(t, `def inc fn [[n:Integer][Integer][n add 1]]  def fwi (forward-args inc)  fwi 5`, `6`)
	w3MiscErr(t, `stack-args no-such-w3`, "not bound")
	w3MiscErr(t, `def x 5 stack-args x`, "requires a function word")
	w3MiscErr(t, `forward-args no-such-w3`, "not bound")
	w3MiscErr(t, `def x 5 forward-args x`, "requires a function word")
	// Value-form rejection (direct).
	r := w3TypeReg()
	if _, err := stackArgsHandler([]Value{NewInteger(5)}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "requires a function value") {
		t.Errorf("stack-args non-fn value: got %v", err)
	}
	if _, err := rebarrierAtom(nil, nil, "stack-args", r); err == nil ||
		!strings.Contains(err.Error(), "missing name") {
		t.Errorf("rebarrierAtom no args: got %v", err)
	}
}

func TestW3ForceArity(t *testing.T) {
	w3MiscWant(t, `def add2 fn [[a:Integer b:Integer][Integer][a add b]]  1 2 (force-arity 2 add2)/v apply`, `3`)
	w3MiscErr(t, `force-arity 2 no-such-w3`, "not bound")
	w3MiscErr(t, `def x 5 force-arity 2 x`, "requires a function word")
	// A negative arity / non-fn value is rejected (direct).
	r := w3TypeReg()
	if _, err := forceArityHandler([]Value{NewInteger(-1), NewInteger(5)}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "non-negative arity") {
		t.Errorf("force-arity bad args: got %v", err)
	}
}

func TestW3RefCheckModeGradual(t *testing.T) {
	// A dispatch modifier over a statically-dynamic fn (a stored fn-ref read
	// via dot-access) rides as a gradual Function carrier in check mode.
	out, _ := w3MiscCheck(t, `def m {a:add/v}  usurp (m dot a)`)
	if len(out) == 0 {
		t.Error("check usurp over dynamic field: expected a carrier")
	}
	// checkModeGradualFn direct arms.
	r := w3TypeReg()
	if _, ok := checkModeGradualFn(r, NewCarrier(TInteger)); ok {
		t.Error("a concrete-typed carrier is provably not a function")
	}
	r.Check.Mode = true
	if out, ok := checkModeGradualFn(r, NewDynamicCarrier(TAny)); !ok || len(out) != 1 {
		t.Errorf("dynamic Any should ride gradual, got %v %v", out, ok)
	}
}

// ---- istype ----

func TestW3Istype(t *testing.T) {
	w3MiscWant(t, `istype Integer`, `true`)
	w3MiscWant(t, `istype 5`, `false`)
}
