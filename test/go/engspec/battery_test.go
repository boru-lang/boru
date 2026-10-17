package engspec

// The kernel's control-flow battery: crafted branch/loop programs over the
// specfix vocabulary plus the fixture control words (`if`/`for`/`doq`/
// `dofbq`/`bakefnq`/…, specfix.RegisterControlWords), each run on the
// interpreter over a core-only registry and its result pinned against the
// expected render; TestCheckModeBattery pins the check pass's renders of
// the same shapes. It drives the kernel's branch/loop machinery with no
// language layer above it. The battery lived in eng/go, where every row
// also ran through the bytecode VM; that lane went with the compiler
// (design/COMPILER-LANE-REMOVAL.0.md) and the interpreter's half moved
// here. Rows stay out of the shared eng/spec corpus until the TS fixture
// twin of the control words exists.

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
	parser "github.com/boru-lang/boru/parser/go"
	"github.com/boru-lang/boru/test/specfix"
)

func batteryRegistry(t *testing.T) *core.Registry {
	t.Helper()
	r, err := standaloneRegistry()
	if err != nil {
		t.Fatal(err)
	}
	specfix.RegisterControlWords(r)
	return r
}

// runBatteryInterp runs one row on a fresh interpreter.
func runBatteryInterp(t *testing.T, input string) ([]core.Value, error) {
	t.Helper()
	values, err := parser.Parse(input)
	if err != nil {
		t.Fatalf("parse %q: %v", input, err)
	}
	return core.NewTop(batteryRegistry(t)).Run(values)
}

type batteryRow struct {
	input   string
	want    string // Canon of the result stack
	wantErr string // substring of the run error
}

// runBattery runs every row on the interpreter and checks it against the
// row's expected render — or, for an error row, that the run fails with
// an error containing wantErr.
func runBattery(t *testing.T, rows []batteryRow) {
	t.Helper()
	for _, row := range rows {
		t.Run(row.input, func(t *testing.T) {
			out, err := runBatteryInterp(t, row.input)
			if row.wantErr != "" {
				if err == nil {
					t.Errorf("want error containing %q, got %s", row.wantErr, core.Canon(out))
				} else if !strings.Contains(err.Error(), row.wantErr) {
					t.Errorf("error %q does not contain %q", err.Error(), row.wantErr)
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if got := core.Canon(out); got != row.want {
				t.Errorf("got %s, want %s", got, row.want)
			}
			checkBatteryRowClean(t, row.input)
		})
	}
}

// knownBatteryCheckFindings pins the battery rows the check pass cannot
// model cleanly over the core-only fixture vocabulary, with what it reports:
// each is a limit of the specfix fixtures' check halves, not of the
// language's words, which type these programs precisely. A value is the
// comma-joined error-severity codes, or "fail:" and a substring of the
// error the pass returns. The ledger only shrinks: a pinned row that starts
// checking clean fails until its entry is deleted.
var knownBatteryCheckFindings = map[string]string{
	// The fixture if's check model does not splice a List-param arm.
	"def f fn [[x:List] [Integer] [if [true] x [9]]] f [7]": "type_error",
	// The fixture enum's check-mode result is a carrier the fixture def
	// cannot install as a type.
	"def E (enum [a b]) for 2 [E]":   "fail:body must be a type value or literal",
	"def E (enum [a b]) for [2] [E]": "fail:body must be a type value or literal",
	// addq declares Number, so a count or an Integer param fed by it is not
	// provably an Integer.
	"def x (1 addq 1) for x [i]": "no_signature",
	"def g fn [[x:Integer] [Integer] [x mulq 2]] def f fn [[x:Integer] [Integer] [g (x addq 1)]] f 3": "no_signature",
	// The fixture def's map-pattern bind is not modelled by the check pass.
	"def {x:Integer} 5 x addq 1": "undefined_word,no_signature",
}

// checkBatteryRowClean runs a row the interpreter answers cleanly through
// the check pass, which must model it without an error-severity finding — a
// diagnostic there is a false positive on a program that runs — unless the
// row is pinned in knownBatteryCheckFindings, when it must report exactly
// the pinned finding.
func checkBatteryRowClean(t *testing.T, input string) {
	t.Helper()
	values, err := parser.Parse(input)
	if err != nil {
		t.Fatalf("parse %q: %v", input, err)
	}
	r := batteryRegistry(t)
	specfix.RegisterCheckExtras(r)
	r.Source = input
	done := r.Check.Begin()
	_, runErr := core.NewTop(r).Run(values)
	r.RescueForwardRefDiagnostics()
	diags := r.Check.Diagnostics
	done()
	var codes []string
	for _, d := range diags {
		if d.Severity == core.SeverityError {
			codes = append(codes, d.Code)
		}
	}
	want, pinned := knownBatteryCheckFindings[input]
	switch {
	case pinned && strings.HasPrefix(want, "fail:"):
		if runErr == nil || !strings.Contains(runErr.Error(), strings.TrimPrefix(want, "fail:")) {
			t.Errorf("pinned check failure %q: got error %v, codes %v", want, runErr, codes)
		}
	case runErr != nil:
		t.Errorf("check pass failed: %v", runErr)
	case pinned:
		if got := strings.Join(codes, ","); got != want {
			t.Errorf("pinned check finding %q: got %q (delete the entry if the row now checks clean)", want, got)
		}
	case len(codes) > 0:
		t.Errorf("check pass reports %v on a program that runs: %v", codes, diags)
	}
}

func TestControlBattery(t *testing.T) {
	runBattery(t, []batteryRow{
		// Constant conditions: the literal-cond reduction arm.
		{input: "if true [1] [2]", want: "1"},
		{input: "if false [1] [2]", want: "2"},

		// List conditions: mark/move at run time, cond fragment when
		// emitting, JMP_IF_FALSE in the lowering.
		{input: "if [1 is Integer] [10] [20]", want: "10"},
		{input: "if [1 is String] [10] [20]", want: "20"},
		{input: "def x 5 if [x is Integer] [x addq 1] [0]", want: "6"},
		{input: "def x 'a' if [x is Integer] [1] [x concatq 'b']", want: "'ab'"},

		// Value arms (no body capture).
		{input: "if [1 is Integer] 7 8", want: "7"},
		{input: "if [1 is String] 7 8", want: "8"},

		// Branches over an enclosing binding.
		{input: "def n 3 if [true] [n addq 1] [n addq 2]", want: "4"},
		{input: "def n 3 if [false] [n addq 1] [n addq 2]", want: "5"},

		// Nested branches.
		{input: "if [true] [if [false] [1] [2]] [3]", want: "2"},

		// Loops: iterator spread, constant body, zero-trip prune.
		{input: "for 3 [i]", want: "0 1 2"},
		{input: "for 3 ['x']", want: "'x' 'x' 'x'"},
		{input: "for 0 [i]", want: ""},
		{input: "for 2 [i addq 10]", want: "10 11"},

		// Nested loops (the inner iterator shadows).
		{input: "for 2 [for 2 [i]]", want: "0 1 0 1"},

		// Branch inside a loop.
		{input: "for 3 [if [i is Integer] [i addq 1] [0]]", want: "1 2 3"},

		// Loop flow control.
		{input: "for 3 [break]", want: ""},
		{input: "for 3 [continue]", want: ""},

		// Errors: a non-list body, count-form taxonomy.
		{input: "for 2 5", wantErr: "no signature matches"},
	})
}

// TestControlEdgeBattery drives the fixture control words' edge arms:
// branches that produce no value (both the literal-cond fragment and
// the dynamic join), the spliced computed-list arm, doq bodies
// reached through a def'd word and a fn param, loop captures whose
// element type is a disjunct, the range-parse error taxonomy, and the
// empty negative-step loop.
func TestControlEdgeBattery(t *testing.T) {
	runBattery(t, []batteryRow{
		// A literal-cond branch that nets no value.
		{input: "1 if [true] [1 drop]", want: ""},
		// A dynamic cond whose branches both net no value: the empty join.
		{input: "5 if (0 addq 1) [1 drop] [1 drop]", want: "5"},
		// A List param as an arm is the interpreter's spliced code body.
		{input: "def f fn [[x:List] [Integer] [if [true] x [9]]] f [7]", want: "7"},
		// doq bodies: a def'd word resolves under the recorder; a fn
		// param stays dynamic.
		{input: "def b [1 2] doq b", want: "1 2"},
		{input: "def f fn [[x:List] [Any] [doq x]] f [9]", want: "9"},
		// Loop captures whose top-of-stack element is a Disjunct — both
		// a body-constructed enum and a def'd one the model resolves.
		{input: "for 2 [enum [a b]]", want: "a tor b a tor b"},
		{input: "for [2] [enum [a b]]", want: "a tor b a tor b"},
		// Stage 2 flip (design/legacy/TYPE-REPRESENTATION.1.ignore §6): the NAME
		// denotes its minted node and renders as the name (the inline
		// enum rows above keep the body rendering).
		{input: "def E (enum [a b]) for 2 [E]", want: "E E"},
		{input: "def E (enum [a b]) for [2] [E]", want: "E E"},
		// A List param as an arm under a DYNAMIC cond: the computed-list
		// arm (the literal-cond twin of the row above).
		{input: "def f fn [[x:List] [Integer] [if (0 addq 1) x [9]]] f [7]", want: "7"},
		// Range-parse taxonomy: non-integer elements in every position,
		// and the arity gate on both sides.
		{input: "for ['a' 2] [i]", wantErr: "expected a concrete integer"},
		{input: "for ['a' 1 2] [i]", wantErr: "expected a concrete integer"},
		{input: "for [1 'b' 2] [i]", wantErr: "expected a concrete integer"},
		{input: "for [1 5 'c'] [i]", wantErr: "expected a concrete integer"},
		{input: "for [] [i]", wantErr: "range must have 1-3 elements"},
		{input: "for [1 2 3 4] [i]", wantErr: "range must have 1-3 elements"},
		// A negative-step range that never enters the body.
		{input: "for [1 5 -1] [i]", want: ""},
	})
}

// TestHigherOrderBattery drives eachq — the lambda-hook Callable: the
// quotation form, the Function-value form (tryRecordLambdaClosure /
// lambdaHookCompatible / lambdaCallbackInputs under the recorder),
// empty data, and the no-result taxonomy.
func TestHigherOrderBattery(t *testing.T) {
	runBattery(t, []batteryRow{
		{input: "eachq [1 addq] [1 2 3]", want: "[2 3 4]"},
		// A body netting no value has no result to collect.
		{input: "eachq [drop] [1 2]", wantErr: "body produced no result"},
		{input: "eachq [2 mulq] []", want: "[]"},
		{input: "eachq [dup addq] [1 2]", want: "[2 4]"},
		// An empty body passes each element through (the element IS the
		// residual top of the invoked body).
		{input: "eachq [] [1 2]", want: "[1 2]"},
		{input: "def f (fn [[x:Integer] [Integer] [x mulq 2]]) eachq f/v [1 2 3]", want: "[2 4 6]"},
		{input: "def f (fn [[s:String] [String] [s concatq '!']]) eachq f/v ['a' 'b']", want: "['a!' 'b!']"},
		{input: "eachq [drop] [1]", wantErr: "body produced no result"},
		// Nested: the outer body is itself a higher-order call.
		{input: "eachq [[1 addq] swap eachq] [[1 2] [3]]", wantErr: "no signature matches"},
	})
}

// TestFnBodyControlBattery drives branches and loops INSIDE fn bodies:
// carrier-typed params flowing through the branch/loop analysis, and the
// bodies run per call.
func TestFnBodyControlBattery(t *testing.T) {
	runBattery(t, []batteryRow{
		// Branch on a Boolean param (carrier cond at check time).
		{input: "def f fn [[b:Boolean] [Any] [if b [99] [0]]] f true", want: "99"},
		{input: "def f fn [[b:Boolean] [Any] [if b [99] [0]]] f false", want: "0"},
		// Branch joining different arm types.
		{input: "def f fn [[b:Boolean] [Any] [if b [1] ['s']]] f false", want: "'s'"},
		// Branch on a list condition over the param.
		{input: "def f fn [[n:Integer] [Any] [if [n is Integer] [n addq 1] [0]]] f 4", want: "5"},
		// A spreading loop violates the fn's declared single return —
		// the return-conformance check fires on both engines.
		{input: "def f fn [[n:Integer] [Any] [for n [i]]] f 3",
			wantErr: "expected 1 return value(s), got 3"},
		// Side-effect loop (body nets 0) with a trailing value.
		{input: "def f fn [[n:Integer] [Any] [for n [5 drop] 0]] f 3", want: "0"},
		// Loop whose body reads the param, netting one value.
		{input: "def f fn [[n:Integer] [Any] [for 1 [n addq i]]] f 10", want: "10"},
		// Nested fn calls with control inside.
		{input: "def g fn [[x:Integer] [Integer] [x addq 1]] def f fn [[n:Integer] [Any] [if [n is Integer] [g n] [0]]] f 7", want: "8"},
		// A fn called from a loop body, netting one value.
		{input: "def g fn [[x:Integer] [Integer] [x mulq 2]] def f fn [[n:Integer] [Any] [for 1 [g n]]] f 3", want: "6"},
	})
}

// TestTruthinessAndBindBattery drives the condition-coercion table and
// computed def bindings (the dyn-bind record/lower path).
func TestTruthinessAndBindBattery(t *testing.T) {
	runBattery(t, []batteryRow{
		// Truthiness coercions through the fixture if.
		{input: "if 1 [1] [2]", want: "1"},
		{input: "if 0 [1] [2]", want: "2"},
		{input: "if 'x' [1] [2]", want: "1"},
		{input: "if '' [1] [2]", want: "2"},
		// An empty-LIST condition is an empty cond region — an error;
		// an empty MAP is an ordinary falsy value.
		{input: "if [] [1] [2]", wantErr: "condition produced no value"},
		{input: "if {} [1] [2]", want: "2"},

		// Computed defs: the bound value is computed at run time.
		{input: "def x (5 addq 1) x", want: "6"},
		{input: "def x (5 addq 1) x addq 2", want: "8"},
		{input: "def x (5 addq 1) def y (x addq 1) y", want: "7"},
		// A computed def read inside a branch and a loop.
		{input: "def x (2 addq 3) if [x is Integer] [x] [0]", want: "5"},
		{input: "def x (1 addq 1) for x [i]", want: "0 1"},
		// Rebinding via undef.
		{input: "def x 1 undef x def x 2 x", want: "2"},
		// Quote / splice interactions with control.
		{input: "def s word 42 if [true] [s] [0]", want: "42"},
	})
}

// TestClosureBattery drives closures through doq, the battery's
// body-runner, so closure construction, capture and dispatch run
// standalone.
func TestClosureBattery(t *testing.T) {
	runBattery(t, []batteryRow{
		// Single- and multi-value residual closures.
		{input: "doq [1 addq 2]", want: "3"},
		{input: "doq [10 20 30]", want: "10 20 30"},
		{input: "doq []", want: ""},
		// An enclosing binding read inside the body.
		{input: "def n 4 doq [n addq 1]", want: "5"},
		// A def-leaking body (do keeps body defs bound).
		{input: "doq [def z 5 z] z", want: "5 5"},
		// Nested closures.
		{input: "doq [doq [1] addq 1]", want: "2"},
		// A word-bound body (the deref arm; the computed body declines
		// the closure and rides the fallback).
		{input: "def b [1 addq 2] doq b", want: "3"},
		// Inside a fn body.
		{input: "def f fn [[x:Integer] [Any] [doq [x addq 1]]] f 3", want: "4"},
		// Control flow inside the closure body.
		{input: "doq [if [true] [7] [8]]", want: "7"},
		{input: "doq [for 2 [i]]", want: "0 1"},
		// A raising body surfaces as an Error VALUE.
		{input: "doq [refine 5] typeof", want: "Error"},
	})
}

// TestProgramShapeBattery drives the program shapes the earlier batteries
// miss: interpolation, quote/splice flows, global rebinds through undef,
// args frames inside fns, and stack shuffles.
func TestProgramShapeBattery(t *testing.T) {
	runBattery(t, []batteryRow{
		// Interpolation.
		{input: "`a${1 addq 2}b`", want: "'a3b'"},
		{input: "`${'x'}${'y'}`", want: "'xy'"},
		{input: "def n 7 `v=${n}`", want: "'v=7'"},
		// Rebinds and undef inside programs.
		{input: "def x 1 def x (x addq 1) x", want: "2"},
		{input: "def x 1 undef x 5", want: "5"},
		// args frames inside fn bodies.
		{input: "def f fn [[x:Integer] [Any] [args lengthq]] f 9", want: "1"},
		// Stack shuffles.
		{input: "1 2 3 swap drop", want: "1 3"},
		{input: "1 2 over", want: "1 2 1"},
		// Splice through control.
		{input: "def s word 42 for 2 [s]", want: "42 42"},
		// Deep closure nesting.
		{input: "doq [doq [doq [5]]]", want: "5"},
		// Typed defs read back.
		{input: "def {x:Integer} 5 x addq 1", want: "6"},
	})
}

// TestStoredFnBattery drives stored fns: repeated calls, fn-calling-fn
// chains, fn values rebound and reused, and overloaded fns dispatching
// by type.
func TestStoredFnBattery(t *testing.T) {
	runBattery(t, []batteryRow{
		// Repeated calls to one stored fn.
		{input: "def f fn [[x:Integer] [Integer] [x addq 1]] f 1 f 2 f 3", want: "2 3 4"},
		// A chain of stored fns.
		{input: "def g fn [[x:Integer] [Integer] [x mulq 2]] def f fn [[x:Integer] [Integer] [g (x addq 1)]] f 3", want: "8"},
		// The same fn used in a loop body and at top level.
		{input: "def f fn [[x:Integer] [Integer] [x addq 10]] for 2 [f i] f 5", want: "10 11 15"},
		// An fn VALUE bound, rebound through a paren, and applied.
		{input: "def f (fn [[x:Integer] [Integer] [x addq 1]]) def g (f) (g 4)",
			wantErr: "no signature matches"},
		// Overloads: two sigs on one name dispatch by type.
		{input: "def f fn [[x:Integer] [Integer] [x addq 1] [s:String] [String] [s concatq '!']] f 1 f 'a'", want: "2 'a!'"},
		// A zero-arg fn called twice.
		{input: "def z fn [[] [Integer] [7]] (z) addq (z)", want: "14"},
		// Fn results feeding fn args.
		{input: "def f fn [[x:Integer] [Integer] [x addq 1]] f (f (f 0))", want: "3"},
	})
}

// TestFallbackBodyBattery drives dofbq, the fixture body-runner that
// runs its body through a sub-engine.
func TestFallbackBodyBattery(t *testing.T) {
	runBattery(t, []batteryRow{
		{input: "dofbq [1 addq 2]", want: "3"},
		{input: "dofbq [def z 5 z]", want: "5"},
		{input: "dofbq [10 20]", want: "10 20"},
		{input: "def n 3 dofbq [n addq 1]", want: "4"},
		{input: "dofbq [dofbq [2]]", want: "2"},
		{input: "1 dofbq [2] addq", want: "3"},
	})
}

// TestBakingBattery drives the stored-fn and stored-body fixture words:
// fn values through bakefnq (with and without captures) and NoEval bodies
// through bakebodyq.
func TestBakingBattery(t *testing.T) {
	runBattery(t, []batteryRow{
		{input: "bakefnq (fn [[x:Integer] [Integer] [x addq 1]])", want: "8"},
		{input: "def f (fn [[x:Integer] [Integer] [x mulq 2]]) bakefnq f/v", want: "14"},
		{input: "bakebodyq [1 addq 2]", want: "3"},
		{input: "bakebodyq [10 20]", want: "10 20"},
		{input: "def n 4 bakebodyq [n addq 1]", want: "5"},
		{input: "bakefnq 5", wantErr: "no signature matches"},
	})
}

// TestDispatchFormBattery drives the forward-collection splits and
// dispatch-modifier forms over the fixture vocabulary.
func TestDispatchFormBattery(t *testing.T) {
	runBattery(t, []batteryRow{
		// The one split rule: all-forward, all-stack, and mixed splits.
		{input: "addq 1 2", want: "3"},
		{input: "1 2 addq", want: "3"},
		{input: "1 addq 2", want: "3"},
		{input: "subq 10 3", want: "-7"},
		{input: "10 3 subq", want: "7"},
		{input: "10 subq 3", want: "7"},
		// Explicit dispatch modifiers.
		{input: "1 2 addq/s", want: "3"},
		{input: "addq/f 1 2", want: "3"},
		{input: "addq/2 1 2", want: "3"},
		// A quoted word via /q is data.
		{input: "addq/q typeof", want: "Atom"},
		// Modifier on a fixture fn.
		{input: "def f fn [[x:Integer] [Integer] [x addq 1]] f/1 5", want: "6"},
	})
}

// TestParenFnCheckBattery pins the paren-fn collapse shapes in check
// mode: zero-arg fn values applied through parens, fn values named
// bare, and paren groups over dynamic names.
func TestParenFnCheckBattery(t *testing.T) {
	rows := []struct {
		input string
		want  string
	}{
		{input: "def f (fn [[] [Integer] [7]]) (f)", want: "Integer"},
		{input: "def f (fn [[] [Integer] [7]]) (f) addq (f)", want: "Number"},
		{input: "def f (fn [[x:Integer] [Integer] [x]]) (f 1)", want: "Integer"},
		{input: "(1 addq 2)", want: "Number"},
		{input: "((1 addq 2))", want: "Number"},
		{input: "(noretq 1)", want: "dynamic(Any) :: ~missing_returns"},
	}
	for _, row := range rows {
		t.Run(row.input, func(t *testing.T) {
			got, err := runBatteryCheck(t, row.input)
			if err != nil {
				t.Fatalf("check run: %v", err)
			}
			if got != row.want {
				t.Errorf("check render = %q, want %q", got, row.want)
			}
		})
	}
}

// TestRangeLoopBattery drives the range-form for: static ranges with
// every arity, negative steps, and computed bounds (const start/step,
// runtime end) that still lower to FOR_SETUP.
func TestRangeLoopBattery(t *testing.T) {
	runBattery(t, []batteryRow{
		{input: "for [3] [i]", want: "0 1 2"},
		// A count the check pass cannot fold (lengthq declares an Integer it
		// does not compute) still runs its trips.
		{input: "for (lengthq [1 2]) [i]", want: "0 1"},
		{input: "for [1 4] [i]", want: "1 2 3"},
		{input: "for [1 7 2] [i]", want: "1 3 5"},
		{input: "for [3 0 -1] [i]", want: "3 2 1"},
		{input: "for [2 2] [i]", want: ""},
		{input: "for [1 4 0] [i]", wantErr: "step cannot be zero"},
		// Computed bounds inside a fn body (carrier end).
		{input: "def f fn [[n:Integer] [Any] [for [n] [5 drop] 0]] f 3", want: "0"},
		{input: "def f fn [[n:Integer] [Any] [for [1 n] [5 drop] 0]] f 3", want: "0"},
		{input: "def f fn [[n:Integer] [Any] [for [0 n 1] [5 drop] 0]] f 2", want: "0"},
		// A computed range at top level via a computed def.
		{input: "def n (1 addq 2) for [n] [i]", want: "0 1 2"},
	})
}

// TestFnValueApplyBattery drives fn VALUES applied through parens —
// the dynamic-apply record/lower path.
func TestFnValueApplyBattery(t *testing.T) {
	runBattery(t, []batteryRow{
		// A def-bound fn applied in a paren group.
		{input: "def f (fn [[x:Integer] [Integer] [x addq 1]]) (f 5)", want: "6"},
		// Applied twice; nested applications.
		{input: "def f (fn [[x:Integer] [Integer] [x addq 1]]) (f (f 5))", want: "7"},
		// A fn value passed through a def and applied in a branch body.
		{input: "def f (fn [[x:Integer] [Integer] [x mulq 3]]) if [true] [(f 2)] [0]", want: "6"},
		// Zero-arg fn value.
		{input: "def f (fn [[] [Integer] [42]]) (f)", want: "42"},
		// A list param through a fn-value apply stays quoted (inert) on
		// return — the binding-inertness rule, canon-visible.
		{input: "def g (fn [[l:List] [Any] [l]]) (g [1 2])", want: "(quote [1 2])"},
	})
}

// runBatteryCheck runs one row in plain check mode and renders the
// carrier stack + diagnostics, exactly like the corpus check lane.
func runBatteryCheck(t *testing.T, input string) (string, error) {
	t.Helper()
	values, err := parser.Parse(input)
	if err != nil {
		t.Fatalf("parse %q: %v", input, err)
	}
	r := batteryRegistry(t)
	specfix.RegisterCheckExtras(r)
	r.Source = input
	done := r.Check.Begin()
	out, runErr := core.NewTop(r).Run(values)
	r.RescueForwardRefDiagnostics()
	r.Check.EmitUnusedDefDiagnostics()
	diags := r.Check.Diagnostics
	done()
	if runErr != nil {
		return "", runErr
	}
	return specfix.RenderCheck(out, diags), nil
}

// TestCheckModeBattery pins the check renders of control/closure rows:
// the branch join shapes, the loop spread model, the unreachable-branch
// diagnostic, and dynamic dispatch over carriers.
func TestCheckModeBattery(t *testing.T) {
	rows := []struct {
		input string
		want  string
	}{
		// Both branches netting no value: the empty join under plain
		// check (recorder inactive) and its doq/loop edge kin.
		{input: "5 if (0 addq 1) [1 drop] [1 drop]", want: "Integer"},
		{input: "def b [1 2] doq b", want: "Integer Integer"},
		{input: "def f fn [[x:List] [Any] [doq x]] f [9]", want: "dynamic(Any)"},
		{input: "for 2 [enum [a b]]", want: "Enum Enum"},
		// Constant LIST conditions reduce with the unreachable diagnostic;
		// a bare scalar condition keeps the live join.
		{input: "if [true] [1] [2]", want: "Integer :: ~unreachable_branch"},
		{input: "if [false] [1] ['s']", want: "ProperString :: ~unreachable_branch"},
		{input: "if false [1] ['s']", want: "Disjunct"},
		// A live condition joins the arms.
		{input: "if [1 is Integer] [1] [2]", want: "Integer"},
		{input: "if [1 is Integer] [1] ['s']", want: "Disjunct"},
		// Loop spread: three per-iteration values.
		{input: "for 3 [i]", want: "Integer Integer Integer"},
		{input: "for [1 3] ['x']", want: "ProperString ProperString"},
		// Closure body residuals.
		{input: "doq [1 addq 2]", want: "Number"},
		{input: "doq [10 20 30]", want: "Integer Integer Integer"},
		// A word body derefs through the def table under check too.
		{input: "def b [1 addq 2] doq b", want: "Number"},
		// A fn value applied in parens under check.
		{input: "def f (fn [[x:Integer] [Integer] [x addq 1]]) (f 5)", want: "Integer"},
		// Dynamic carriers flowing into dispatch: the assume-sig path.
		{input: "noretq 1 addq 2", want: "dynamic(Number) :: ~missing_returns"},
		{input: "noretq 1 dup", want: "dynamic(Any) dynamic(Any) :: ~missing_returns"},
		{input: "noretq 1 typeof", want: "dynamic(Type) :: ~missing_returns"},
		{input: "noretq 1 lengthq", want: "dynamic(Integer) :: ~missing_returns"},
		// A dynamic value through a branch join.
		{input: "if [true] [noretq 1] [2]", want: "dynamic(Any) :: ~unreachable_branch ~missing_returns"},
		// A dynamic condition keeps both arms live.
		{input: "if [noretq true] [1] [2]", want: "Integer"},
		// A dynamic count on the loop.
		{input: "for [noretq 2 drop 2] [i]", want: "Integer Integer :: ~missing_returns"},
		// An Integer count the pass does not fold: one typed-list carrier,
		// not a spread of per-trip residuals.
		{input: "for (lengthq [1 2]) [i]", want: "List"},
		// An each body netting no value: the residual is a plain List.
		{input: "eachq [drop] [1 2]", want: "List"},
	}
	for _, row := range rows {
		t.Run(row.input, func(t *testing.T) {
			got, err := runBatteryCheck(t, row.input)
			if err != nil {
				t.Fatalf("check run: %v", err)
			}
			if got != row.want {
				t.Errorf("check render = %q, want %q", got, row.want)
			}
		})
	}
}
