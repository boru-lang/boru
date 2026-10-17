package lang

import (
	"fmt"
	"strings"
	"testing"

	"github.com/boru-lang/boru/lang/go/native"
)

// Interpreter-only twins of the coverage the compiler lane's parity suites
// carried (design/COMPILER-LANE-REMOVAL.0.md §4). Each program is one the
// deleted tests drove through both engines; each row now pins what the run
// answers and, where the arm under test is the check pass's, what the check
// reports — every negative beside its positive twin.

// lrRun runs src on a fresh instance and renders the result or the error.
func lrRun(t *testing.T, src string) string {
	t.Helper()
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	got, err := a.Run(src)
	if err != nil {
		return "ERR " + strings.SplitN(err.Error(), "\n", 2)[0]
	}
	return fmt.Sprint(got)
}

// lrCheck checks src on a fresh instance: the residual stack and the
// error-severity codes.
func lrCheck(t *testing.T, src string) (string, []string) {
	t.Helper()
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	res, err := a.Check(src)
	if err != nil {
		t.Fatalf("check %q: %v", src, err)
	}
	var codes []string
	for _, d := range res.Diagnostics {
		if d.Severity == SeverityError {
			codes = append(codes, d.Code)
		}
	}
	return fmt.Sprint(res.Stack), codes
}

type lrRow struct{ src, run, check string }

// lrRows asserts each row's run, and its check residual when one is given;
// every row must check free of errors.
func lrRows(t *testing.T, rows []lrRow) {
	t.Helper()
	for _, row := range rows {
		if got := lrRun(t, row.src); got != row.run {
			t.Errorf("run %q\n  got  %s\n  want %s", row.src, got, row.run)
		}
		stack, codes := lrCheck(t, row.src)
		if len(codes) != 0 {
			t.Errorf("check %q: error diagnostics %v", row.src, codes)
		}
		if row.check != "" && stack != row.check {
			t.Errorf("check %q: residual %s, want %s", row.src, stack, row.check)
		}
	}
}

// A fn value def-bound from a param is a CARRIER under analysis, recorded
// in the fn-carrier side table rather than Defs; read inside a `do` body it
// stands in the residual where the run dispatches the word over the empty
// frame `do` opens (NUR193). `do`'s model takes the computed-body hatch —
// one dynamic(Any) — for both the caught no-match and the written operand.
func TestDoBodyReadOfADefBoundFnCarrier(t *testing.T) {
	lrRows(t, []lrRow{
		{`def h fn [[g:Function] [Any] [def a5 g/v do [a5]]] h ([n:Integer] => [n])`,
			"[error(cannot call `a5` — no signature matches the arguments)]", "[dynamic(Any)]"},
		{`def h fn [[g:Function] [Any] [def a5 g/v do [a5 7]]] h ([n:Integer] => [n])`,
			"[7]", "[dynamic(Any)]"},
	})
}

// The parse / mini / emit value forms resolve a NAME def-bound to a fn
// carrier through the same side table, and a container member the pass
// knows only as dynamic(Any) degrades to an advisory rather than the false
// "the kind must be a literal name" error; the run dispatches on the value.
func TestMacroValueFormsOverCarriers(t *testing.T) {
	lrRows(t, []lrRow{
		{`def h fn [[g:Function] [Any] [def p g/v mini p 'ab']] h ([s:String o:Map] => [s])`, "[ab]", "[dynamic(Any)]"},
		{`def h fn [[g:Function] [Any] [def p g/v emit p {a:1}]] h (fn [[value:Any opts:Map] [String] ['e']])`, "[e]", "[dynamic(Any)]"},
		{`def f fn [[m:Map] [Any] [mini m.d 'ab']] f {d: ([s:String o:Map] => [s])}`, "[ab]", "[dynamic(Any)]"},
		{`def f fn [[m:Map] [Any] [parse m.p 'x']] f {p: ([s:String o:Map] => [s])}`, "[x]", "[dynamic(Any)]"},
		{`def f fn [[m:Map] [Any] [emit m.e {a:1}]] f {e: (fn [[value:Any opts:Map] [String] ['E']])}`, "[E]", "[dynamic(Any)]"},
	})
	// A member that is no fn at run time raises there; the pass, which
	// cannot see the member, stays clean.
	src := `def f fn [[m:Map] [Any] [mini m.d 'ab']] f {d: 'x'}`
	if got := lrRun(t, src); !strings.Contains(got, "signature_error") {
		t.Errorf("run %q = %s, want the no-match raise", src, got)
	}
	if _, codes := lrCheck(t, src); len(codes) != 0 {
		t.Errorf("check %q: %v, want the degraded advisory only", src, codes)
	}
}

// A container member that holds a fn CARRIER (a param read into a map) is
// read as dynamic(Any) and noted for the shaped-member model; the run
// dispatches the stored fn over what follows (NUR112). behave over such a
// member sees through the read to the fn it resolved (NUR257).
func TestFnCarrierMemberReads(t *testing.T) {
	lrRows(t, []lrRow{
		{`def h fn [[g:Function] [Any] [def m {f: g/v} m.f [1 2 3]]] h size/v`, "[3]", "[dynamic(Any)]"},
		{`def Temp refine Integer end def h fn [[g:Function] [] [def m {f: g/v} behave canon/q m.f]] h (fn [[t:Temp][String]['T']]) canon (make Temp 5)`, "[T]", ""},
	})
}

// A typed def over a fn-signature type whose body the pass holds only as a
// carrier binds a carrier retagged to the declared FnUndef (NUR290): the
// value's membership is the run's to decide, and here the run refuses it.
func TestTypedFnUndefDefOverACarrier(t *testing.T) {
	src := `def F fnsig [[Integer] [Integer]] def mk fn [[] [Any] [([x:Integer] => [x])]] def g:F (mk) 1`
	if got := lrRun(t, src); !strings.Contains(got, "does not unify with declared type F") {
		t.Errorf("run %q = %s, want the unify refusal", src, got)
	}
	if stack, codes := lrCheck(t, src); len(codes) != 0 || stack != "[Integer]" {
		t.Errorf("check %q = %s %v, want a clean [Integer]", src, stack, codes)
	}
}

// A case finding is reported once per site: a call shape narrowed to a
// carrier (`f n` with n:Integer) re-analyses the body and meets the same
// clause-vs-clause finding at the same position.
func TestCaseFindingOncePerSiteAcrossCallShapes(t *testing.T) {
	const src = `def f fn [[x:Any][Integer][case x [Integer 1 1 2 3]]] def g fn [[n:Integer] [Integer] [f n]] g 5`
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	res, err := a.Check(src)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, d := range res.Diagnostics {
		if d.Code == "case_unreachable_clause" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("want exactly one case_unreachable_clause, got %d: %v", n, res.Diagnostics)
	}
	if got := lrRun(t, src); got != "[1]" {
		t.Errorf("run = %s, want [1]", got)
	}
}

// A break in a keyword def's evaluated operand (`def C class {a: (i
// break)}`) abandons the definition and resolves at the loop (NUR358): the
// loop ends, and the name was never bound.
func TestKeywordDefOperandBreakAbandonsTheDef(t *testing.T) {
	for _, src := range []string{
		`for 3 [def C class {a: (i break)}] 7`,
		`for 3 [def S surface {a: (i break)}] 7`,
		`for 3 [def P refine Integer [(i break)]] 7`,
		`for 3 [def C gen [T] class {a: (i break)}] 7`,
	} {
		if got := lrRun(t, src); got != "[7]" {
			t.Errorf("run %q = %s, want [7]", src, got)
		}
	}
	if got := lrRun(t, `for 3 [def C class {a: (i break)}] make C {a:1}`); !strings.Contains(got, "undefined word: C") {
		t.Errorf("the abandoned def must leave C unbound, got %s", got)
	}
}

// foldaxis reduces each lane through fold's step: a lane of one element is
// its own result, a longer one threads the body.
func TestFoldaxisLanes(t *testing.T) {
	lrRows(t, []lrRow{
		{`import "boru:array-util" ArrayUtil.foldaxis 1 [add] [[1 2 3] [4 5 6]]`, "[[6 15]]", ""},
		{`import "boru:array-util" ArrayUtil.foldaxis 1 [add] [[1] [2]]`, "[[1 2]]", ""},
		{`import "boru:array-util" ArrayUtil.foldaxis 0 [add] [[1 2 3] [4 5 6]]`, "[[5 7 9]]", ""},
	})
	if got := lrRun(t, `import "boru:array-util" ArrayUtil.foldaxis 1 [add] [[1] []]`); !strings.Contains(got, "rectangular") {
		t.Errorf("a ragged list = %s, want the rectangular refusal", got)
	}
}

// each over a map with a lambda that leaves nothing raises at the entry.
func TestEachMapLambdaWithNoResult(t *testing.T) {
	if got := lrRun(t, `each ([kv:Any] => [kv drop]) {a:1}`); !strings.Contains(got, `each: key "a": body produced no result`) {
		t.Errorf("a resultless lambda = %s, want each_error", got)
	}
	if got := lrRun(t, `each ([kv:Any] => [7]) {a:1}`); got != "[{a:7}]" {
		t.Errorf("a lambda with a result = %s, want [{a:7}]", got)
	}
}

// A service handler's typed binding slots decide whether it takes a request
// routing reached: a declining one falls back to a slot-free catch-all, or
// refuses; a request `prior` hands on is held to the slots of the handler
// it reaches; and the reply is the handler's last result.
func TestServiceBindingSlots(t *testing.T) {
	const typed = `def svc (service {}) add {cmd: "get" n: Integer} ([r:Map s:Any] => ['typed']) svc `
	lrRows(t, []lrRow{
		{typed + `add {} ([r:Map s:Any] => ['any']) svc call {cmd: "get" n: "x"} svc`, "[any]", ""},
		{typed + `add {} ([r:Map s:Any] => ['any']) svc call {cmd: "get" n: 5} svc`, "[typed]", ""},
		{`def svc (service {}) add {} ([r:Map s:Any] => [1 2]) svc call {} svc`, "[2]", ""},
		{`def svc (service {}) add {cmd: "go" n: Integer} ([r:Map s:Any] => [r.n]) svc add {cmd: "go"} ([r:Map s:Any p:Any] => [p r]) svc call {cmd: "go" n: 1} svc`, "[1]", ""},
	})
	if got := lrRun(t, typed+`call {cmd: "get" n: "x"} svc`); !strings.Contains(got, "failed its typed binding slots") {
		t.Errorf("a declining slot with no catch-all = %s, want no_match", got)
	}
	src := `def svc (service {}) add {cmd: "go" n: Integer} ([r:Map s:Any] => [r.n]) svc add {cmd: "go"} ([r:Map s:Any p:Any] => [p {cmd: "go"}]) svc call {cmd: "go" n: 1} svc`
	if got := lrRun(t, src); !strings.Contains(got, "request passed on by prior fails the handler's typed binding slots") {
		t.Errorf("prior handing on a request without the slot = %s, want no_match", got)
	}
}

// A Go compile hook on a built-in kind expands `mini <kind> <literal>` at
// the call site: tokens it returns are spliced, and a hook that returns none
// leaves the standard transducer call.
func TestMiniGoHookExpansion(t *testing.T) {
	const src = `import "boru:minilang" end mini bf '++++++++[>++++++++<-]>+.'`
	for _, c := range []struct {
		name string
		hook native.MiniCompileHook
		want string
	}{
		{"splices the hook's tokens", func(s string, _ native.Value, _ *native.Registry) ([]native.Value, error) {
			return []native.Value{native.NewString(fmt.Sprint(len(s)))}, nil
		}, "[24]"},
		{"declines to the transducer", func(string, native.Value, *native.Registry) ([]native.Value, error) {
			return nil, nil
		}, "[A]"},
	} {
		a, err := New()
		if err != nil {
			t.Fatal(err)
		}
		native.RegisterMiniCompileGoHook(a.registry, "bf", c.hook)
		got, err := a.Run(src)
		if err != nil || fmt.Sprint(got) != c.want {
			t.Errorf("%s: %v / %v, want %s", c.name, got, err, c.want)
		}
	}
	if got := lrRun(t, src); got != "[A]" {
		t.Errorf("no hook: %s, want the transducer's [A]", got)
	}
}

// NewFromRegistry refuses a nil registry and wraps a wired one.
func TestNewFromRegistry(t *testing.T) {
	if _, err := NewFromRegistry(nil); err == nil {
		t.Fatal("nil registry must be rejected")
	}
	base, err := New()
	if err != nil {
		t.Fatal(err)
	}
	inst, err := NewFromRegistry(base.NativeRegistry())
	if err != nil {
		t.Fatal(err)
	}
	if got, err := inst.Run("1 add 2"); err != nil || fmt.Sprint(got) != "[3]" {
		t.Errorf("Run over a wrapped registry: %v / %v, want [3]", got, err)
	}
}

// NewFormatParserFn wraps a read Format as a parser fn VALUE: bound with
// DefineValue it parses import-free.
func TestNewFormatParserFnLangSurface(t *testing.T) {
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	v, err := NewFormatParserFn("bracket", &bracketFormat{})
	if err != nil {
		t.Fatalf("NewFormatParserFn: %v", err)
	}
	if err := a.DefineValue("bracket", v); err != nil {
		t.Fatalf("DefineValue: %v", err)
	}
	got, err := a.Run("parse bracket 'hi'")
	if err != nil {
		t.Fatalf("parse bracket: %v", err)
	}
	if len(got) != 1 || got[0] != "hi" {
		t.Fatalf("parse bracket = %v, want [hi]", got)
	}
}

// lrFails asserts the run of src raises an error containing want.
func lrFails(t *testing.T, src, want string) {
	t.Helper()
	if got := lrRun(t, src); !strings.HasPrefix(got, "ERR ") || !strings.Contains(got, want) {
		t.Errorf("run %q = %s, want an error containing %q", src, got, want)
	}
}

// `do`'s model of a body that raises unconditionally keeps none of the defs
// the body makes after the raise, as the run makes none; a body that only
// shuffles what it pushed nets nothing.
func TestDoBodyRaiseAndShuffleModels(t *testing.T) {
	lrRows(t, []lrRow{
		{`do [raise bad_input "boom" def x 1]`, "[error(boom)]", "[Error]"},
		{`do [1 drop]`, "[]", "[]"},
		{`1 do [1 drop] drop`, "[]", "[]"},
	})
	const after = `do [raise bad_input "boom" def x 1] drop x`
	lrFails(t, after, "undefined word: x")
	if _, codes := lrCheck(t, after); fmt.Sprint(codes) != "[undefined_word]" {
		t.Errorf("check %q: %v, want the undefined_word the run raises", after, codes)
	}
}

// The taken arm of a literal condition: a 0-value arm makes the if a
// statement, a fn-value residual stays placed, and the else-less if over a
// literal false nets nothing.
func TestIfTakenArmModels(t *testing.T) {
	lrRows(t, []lrRow{
		{`if [true] [def x 1] [2]`, "[]", "[]"},
		{`def g fn [[x:Integer] [Integer] [x add 1]] 10 if [true] [g/v] [1]`, "[10 fn g(Integer)]", ""},
		{`if [false] [1]`, "[]", ""},
		{`def f fn [[b:Boolean] [Any] [if b [5]]] f true`, "[5]", "[dynamic(Any)]"},
	})
}

// A range that runs zero iterations, and `error`'s handler models: a
// handler that consumes the error nets nothing more, one that leaves two
// values is variadic, and a non-body operand matches no signature.
func TestForZeroRangeAndErrorHandlerModels(t *testing.T) {
	lrRows(t, []lrRow{
		{`for [2 2] [i]`, "[]", "[]"},
		{`for [3 3 1] [i]`, "[]", "[]"},
		{`5 error [drop]`, "[5]", "[dynamic(Any)]"},
		{`5 error [dup 'k']`, "[5]", "[dynamic(Any)]"},
	})
	lrFails(t, `error 5 7`, "signature_error")
	if _, codes := lrCheck(t, `error 5 7`); fmt.Sprint(codes) != "[no_signature]" {
		t.Errorf("check `error 5 7`: %v, want no_signature", codes)
	}
}

// A keyword def whose constructor operand the pass holds only as a carrier
// binds nothing under analysis when the name is fresh: the run's install
// meets the same registry.
func TestKeywordDefOverAComputedOperand(t *testing.T) {
	lrRows(t, []lrRow{
		{`def mk fn [[] [List] [[[Integer] [Integer]]]] def F fnsig (mk)`, "[]", "[]"},
	})
}

// A typed def under a refinement whose bound only the run computes (`Integer
// lt (f)`): a concrete member binds, a value outside the base is refused by
// the check as by the run, and the bound itself is the run's to apply.
func TestRunDependentRefinementTypedDef(t *testing.T) {
	const small = `def f fn [[] [Integer] [10]] def Small (Integer lt (f)) `
	lrRows(t, []lrRow{
		{small + `def x:Small 5 x`, "[5]", "[Integer]"},
	})
	for _, src := range []string{small + `def x:Small 50 x`, small + `def x:Small (f) x`} {
		lrFails(t, src, "does not unify with declared type Small")
		if _, codes := lrCheck(t, src); len(codes) != 0 {
			t.Errorf("check %q: %v — the bound is the run's to apply", src, codes)
		}
	}
	src := small + `def x:Small 'hi' x`
	lrFails(t, src, "does not unify with declared type Small")
	if _, codes := lrCheck(t, src); fmt.Sprint(codes) != "[type_error]" {
		t.Errorf("check %q: %v, want type_error", src, codes)
	}
}

// An annotation only the run computes binds a gradual value under analysis
// (NUR327); the call shape that fixes it is checked like the run.
func TestRunDependentAnnotation(t *testing.T) {
	const f = `def f fn [[y:Any] [List] [def xs:[:(typeof y)] [1 2] xs]] `
	lrRows(t, []lrRow{{f + `f 1`, "[[1 2]]", "[List]"}})
	lrFails(t, f+`f 'a'`, "does not unify")
}

// A typed def over a call's carrier: an Any annotation binds the carrier, a
// carrier proven a member binds as the untyped def does, and an unproven
// one binds the annotation's builtin node — the value's membership is the
// run's (NUR290).
func TestTypedDefOverACallCarrier(t *testing.T) {
	lrRows(t, []lrRow{
		{`def f fn [[] [Any] [5]] def x:Any (f) x`, "[5]", "[dynamic(Any)]"},
		{`def f fn [[] [Integer] [5]] def x:Integer (f) x`, "[5]", "[Integer]"},
		{`def f fn [[] [Any] [5]] def x:Integer (f) x`, "[5]", "[Integer]"},
	})
	const src = `def f fn [[] [Any] ['s']] def x:Integer (f) x`
	lrFails(t, src, "does not unify with declared type Integer")
	if stack, codes := lrCheck(t, src); len(codes) != 0 || stack != "[Integer]" {
		t.Errorf("check %q = %s %v, want a clean [Integer]", src, stack, codes)
	}
}

// An undef inside a region that may not run keeps the binding in the model
// with its value generalised, so no later read folds the popped value.
func TestSpeculativeUndefGeneralises(t *testing.T) {
	lrRows(t, []lrRow{
		{`def x 5 for 2 [undef x def x 6] x`, "[6]", "[Integer]"},
	})
	// The run pops it; the model keeps a generalised binding — lenient in
	// the one direction a check may be.
	lrFails(t, `def x 5 if (1 lt 2) [undef x] [0] x`, "undefined word: x")
}

// Driven loops and callbacks: a break escaping a Rand.map-from generator,
// a filter callback that raises, a fold seeded with a type, a walk over a
// quotation hook, the map fold / scan error arms, the walk hook refusals.
func TestDrivenLoopAndCallbackArms(t *testing.T) {
	lrRows(t, []lrRow{
		{`import "boru:rand" def f fn [[] [Any] [break]] end for 3 [Rand.map-from {a:[f]}] 7`, "[7]", ""},
		{`import "boru:rand" for 3 [Rand.map-from {a:[i break]}]`, "[]", ""},
		{`filter ([p:Any] => [true]) [1 2]`, "[[1 2]]", "[List]"},
		{`Integer fold [drop] [1 2]`, "[Integer]", "[Type]"},
		{`walk {mode: "depth"} {a:1} [drop]`, "[{a:1}]", "[Map]"},
	})
	lrFails(t, `filter ([p:Any] => [p.value no_such_word]) [1 2]`, "filter: callback error")
	lrFails(t, `0 fold ([a:Any b:Any] => [a no_such_word]) {a:1}`, `fold: key "a"`)
	lrFails(t, `scan (fn [[a:Any b:Any] [] [a drop b drop]]) {a:1 b:2}`, `scan: key "b": body produced no result`)
	lrFails(t, `walk {mode: "depth"} {a:1} [drop] 5`, "hook must be a quotation list or a lambda")
	lrFails(t, `def mk fn [[tag:String][Function][([m:Integer] => [tag print])]] end def h (mk "x") end walk {mode: "depth"} {a:1} h/v`,
		"no matching hook signature")
}

// Reads and refinements: a typed map param's field, a flex member, a fn
// member under behave (map and list reads seen through, NUR257) and the
// behave refusal the run raises, an overflowing pow, a child type that does
// not resolve.
func TestReadAndRefinementArms(t *testing.T) {
	const temp = `def Temp refine Integer end `
	lrRows(t, []lrRow{
		{`def f fn [[m:{:Integer}] [Integer] [m get 'a']] f {a:1}`, "[1]", "[Integer]"},
		{`def m {xs: (flex [1])} m get 'xs'`, "[[1]]", "[FlexList]"},
		{temp + `def m {f: (fn [[t:Temp][String]['T']])} end behave canon/q m.f end canon (make Temp 5)`, "[T]", "[String]"},
		{temp + `def fs [(fn [[t:Temp][String]['T']])] end behave canon/q (fs get 0) end canon (make Temp 5)`, "[T]", "[String]"},
		{`2 pow 10`, "[1024]", "[Integer]"},
	})
	const refused = `def T (refine Integer) def m {f: ([x:Integer] => [x])} behave T m.f`
	lrFails(t, refused, "unknown behavior name")
	if _, codes := lrCheck(t, refused); len(codes) != 0 {
		t.Errorf("check %q: %v — the refusal is the run's to raise", refused, codes)
	}
	lrFails(t, `3037000500 pow 3`, "integer overflow in pow")
	if _, codes := lrCheck(t, `3037000500 pow 3`); fmt.Sprint(codes) != "[integer_overflow]" {
		t.Errorf("check `3037000500 pow 3`: %v, want the integer_overflow mirror", codes)
	}
	lrFails(t, `[5 1] is [:(no_such_word)]`, "child type expression")
}
