package lang

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	core "github.com/boru-lang/boru/core/go"
	"github.com/boru-lang/boru/lang/go/capabilities"
)

// compiledRunError is the one classifier on a compiled RUN failure now that
// nothing re-runs the source. A genuine boru runtime error and a policy
// denial are the PROGRAM's own result and pass through untouched. So is an
// internal_error a HANDLER raised — the interpreter raises the identical one
// running the identical handler, and booking the compiler for a handler's
// choice of error code would put fifty-odd corpus rows in a ledger meant to
// name VM bails. So is a PLAIN Go error: a handler's fmt.Errorf surfaces
// untouched on the interpreter (stampErrPos leaves a non-BoruError alone),
// and the compiled lane surfaces the same one (2026-09-26 — it used to be
// wrapped as a defect, which put the forty-odd "<native>'s own error" rows on
// the runtime-defers ledger). Only the VM's OWN errors, marked VMDefer, are
// compiler DEFECTS.
func TestCompiledRunErrorClassifies(t *testing.T) {
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	reg := a.NativeRegistry()
	const note = "this is a compiler defect"
	cases := []struct {
		name   string
		err    error
		defect bool
	}{
		{"a VM defer is a defect", vmDeferErr("bytecode: internal: STORE_LOCAL stack underflow (pc=2)"), true},
		{"a recovered VM panic is a defect", vmDeferErr("internal bytecode VM error: index out of range"), true},
		{"a handler's internal_error is the program's", core.MakeBoruError("internal_error", "convert: cannot convert Float to BigInteger", "", "", ""), false},
		{"a user `raise internal_error` is the program's", core.MakeBoruError("internal_error", "boom", "", "", ""), false},
		{"a plain (non-Boru) error is the program's", errors.New("some go error"), false},
		{"the VM's own entry refusal is a defect", vmDeferErr("bytecode: nil program"), true},
		{"type_error is the program's result", core.MakeBoruError("type_error", "bad", "", "", ""), false},
		{"evaluation_limit is the program's result", core.MakeBoruError("evaluation_limit", "too long", "", "", ""), false},
		{"tape_exhausted is the program's result", core.MakeBoruError("tape_exhausted", "too big", "", "", ""), false},
		{"signature_error is the program's result", core.MakeBoruError("signature_error", "no sig", "", "", ""), false},
		{"policy denial is the program's verdict", core.PolicyDenied{Err: errors.New("word `IO.print` denied by policy")}, false},
	}
	for _, c := range cases {
		got, defect := compiledRunError(reg, c.err)
		if defect != c.defect {
			t.Errorf("%s: reported disposition %v, want %v", c.name, defect, c.defect)
		}
		var ae *core.BoruError
		marked := errors.As(got, &ae) && len(ae.Notes) > 0 &&
			strings.Contains(ae.Notes[len(ae.Notes)-1], note)
		if marked != c.defect {
			t.Errorf("%s: marked as a defect = %v, want %v (err=%v)", c.name, marked, c.defect, got)
		}
		if !c.defect && got != c.err {
			t.Errorf("%s: the program's own error was rewritten: %v", c.name, got)
		}
	}
}

// A designed defer that prepared the interpreter's own error for this moment
// raises THAT, not the defect note: it is the program's real failure at the
// point it happens, which is one of the three legal dispositions for a defer
// site — never a re-run looking for an answer.
func TestCompiledRunErrorPrefersDeferAlt(t *testing.T) {
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	ae := vmDeferErr("bytecode: internal: vm:poly-no-match (pc=5)")
	ae.DeferAlt = core.MakeBoruError("signature_error", "no matching signature for `f`", "f", "", "")
	got, defect := compiledRunError(a.NativeRegistry(), ae)
	if got != ae.DeferAlt {
		t.Fatalf("DeferAlt not surfaced: got %v", got)
	}
	// And it is NOT a defect disposition: the alt IS the program's own
	// error, so the caller must not roll the registry back over it.
	if defect {
		t.Error("a surfaced DeferAlt was reported as a defect — the caller would discard the program's work")
	}
}

// vmDeferErr builds an internal_error the way the VM does, with the VMDefer
// marker set. The marker is the discriminator: the CODE alone cannot tell a
// VM bail from a handler's choice or a user's `raise internal_error`.
func vmDeferErr(detail string) *core.BoruError {
	e := core.MakeBoruError("internal_error", detail, "", "", "")
	e.VMDefer = true
	return e
}

// Finding A — a genuine compiled-mode runtime error is surfaced WITHOUT a
// silent interpreter re-run (it matches the interpreter by taxonomy, so
// masking it would only hide the result and burn the budget twice).
func TestRunCompiledSurfacesGenuineError(t *testing.T) {
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	// An integer overflow is a genuine RUNTIME error the emitter still compiles
	// (concrete args) and the VM raises. It must surface FROM THE COMPILED PATH
	// (not be masked by a fallback) with the boru taxonomy intact, matching the
	// interpreter.
	const src = `1000000 pow 1000000`
	_, compiled, errC := a.RunCompiled(src)
	if noteCompileDefect(t, src, nil, errC) {
		return
	}
	if !compiled {
		t.Fatal("overflow program did not run compiled — a genuine runtime error must not trigger fallback")
	}
	b, _ := New()
	_, errI := b.RunInterp(src)
	if codeOf(errC) == "" || codeOf(errC) != codeOf(errI) {
		t.Fatalf("integer_overflow: compiled=[%s] interp=[%s] (want equal, non-empty)", codeOf(errC), codeOf(errI))
	}
}

func codeOf(err error) string {
	var ae *core.BoruError
	if errors.As(err, &ae) {
		return ae.Code
	}
	if err != nil {
		return "non-boru"
	}
	return ""
}

// Finding D — a loop body that rebinds a def needs multiple fixed-point
// analysis rounds, but only the stabilised round is recorded: the body's
// dispatches must be counted ONCE in SiteCounts (not per-round), and the
// compiled result must still match the interpreter.
func TestLoopFixedPointNoReRecord(t *testing.T) {
	// `def x (add i 1) x` adds exactly one `add` dispatch to the body. With the
	// loop event that is mono=2; a re-recorded second round would make it 3.
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	prog, reason, res, cerr := a.CompileCheck(`for 3 [def x (add i 1) x]`)
	if cerr != nil || prog == nil {
		t.Fatalf("rebinding loop did not compile: reason=%q err=%v", reason, cerr)
	}
	if got := res.SiteCounts["mono"]; got != 2 {
		t.Errorf("rebinding-loop mono dispatches = %d, want 2 (the discarded fixed-point round must not re-record)", got)
	}

	// Parity holds regardless of the round count.
	b, _ := New()
	gotC, compiled, errC := b.RunCompiled(`for 3 [def x (add i 1) x]`)
	if noteCompileDefect(t, `for 3 [def x (add i 1) x]`, gotC, errC) {
		return
	}
	if !compiled || errC != nil {
		t.Fatalf("rebinding loop run: compiled=%v err=%v", compiled, errC)
	}
	c, _ := New()
	gotI, _ := c.RunInterp(`for 3 [def x (add i 1) x]`)
	if fmt.Sprint(gotC) != fmt.Sprint(gotI) {
		t.Errorf("rebinding loop: compiled=%v interp=%v", gotC, gotI)
	}
}

// Roadmap item 1 — a 3-arg native call whose sig-0 operand is a COMPUTED
// result (a receiver above two const operands) lowers via a push+swap chain
// instead of declining "operand shape needs reordering". `setpath recv k v`
// with a computed receiver is the driving shape.
func TestThreeArgComputedReceiverLowers(t *testing.T) {
	const src = `import "boru:struct-util" (StructUtil.setpath (flex {a:1}) "b" 2) dot b`

	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	prog, reason, _, cerr := a.CompileCheck(src)
	if cerr != nil || prog == nil {
		t.Fatalf("3-arg computed-receiver row did not compile: reason=%q err=%v", reason, cerr)
	}
	dis := prog.Disassemble()
	if strings.Contains(dis, "FALLBACK") {
		t.Errorf("expected a native lowering, got an island:\n%s", dis)
	}
	if !strings.Contains(dis, "SWAP") || !strings.Contains(dis, "setpath") {
		t.Errorf("expected a push+swap chain into CALL_NATIVE setpath:\n%s", dis)
	}

	// Parity: the compiled result matches the interpreter.
	b, _ := New()
	gotC, compiled, errC := b.RunCompiled(src)
	if noteCompileDefect(t, src, gotC, errC) {
		return
	}
	if !compiled || errC != nil {
		t.Fatalf("compiled run: compiled=%v err=%v", compiled, errC)
	}
	c, _ := New()
	gotI, _ := c.RunInterp(src)
	if fmt.Sprint(gotC) != fmt.Sprint(gotI) {
		t.Errorf("setpath parity: compiled=%v interp=%v", gotC, gotI)
	}
}

// Roadmap item 4 — a STRICT-disjunct straddle (a type-algebra dispatch whose
// operand is a complement/predicate type reaching more than one overload, e.g.
// `is` over `tnot (Integer gt 0)`) lowers to OpCallNativePoly (runtime
// re-match) instead of declining "polymorphic dispatch". The runtime value is a
// single concrete alternative, so the VM dispatches it faithfully.
func TestStrictDisjunctTypeAlgebraPoly(t *testing.T) {
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	prog, reason, _, cerr := a.CompileCheck(`5 is (tnot (Integer gt 0))`)
	if cerr != nil || prog == nil {
		t.Fatalf("type-algebra poly row did not compile: reason=%q err=%v", reason, cerr)
	}
	dis := prog.Disassemble()
	if strings.Contains(dis, "FALLBACK") {
		t.Errorf("expected a native poly lowering, got an island:\n%s", dis)
	}
	if !strings.Contains(dis, "CALL_NATIVE_POLY") {
		t.Errorf("expected CALL_NATIVE_POLY for the straddling `is`:\n%s", dis)
	}

	// Value + taxonomy parity across both engines, including the truth flip
	// (5 is NOT in the complement of Integer>0; 0 IS) — the negative half.
	for _, c := range []struct {
		src  string
		want string
	}{
		{`5 is (tnot (Integer gt 0))`, "false"},
		{`0 is (tnot (Integer gt 0))`, "true"},
		{`Integer tand (tnot (Integer gt 0))`, "(Integer lte 0)"},
		{`"hi" is (tnot (Integer gt 0))`, "true"},
	} {
		b, _ := New()
		gotC, compiled, errC := b.RunCompiled(c.src)
		if noteCompileDefect(t, c.src, gotC, errC) {
			continue
		}
		if !compiled || errC != nil {
			t.Fatalf("%q: compiled=%v err=%v", c.src, compiled, errC)
		}
		d, _ := New()
		gotI, _ := d.RunInterp(c.src)
		if fmt.Sprint(gotC) != fmt.Sprint(gotI) {
			t.Errorf("%q: compiled=%v interp=%v", c.src, gotC, gotI)
		}
		if fmt.Sprint(gotC) != "["+c.want+"]" {
			t.Errorf("%q: got %v, want [%s]", c.src, gotC, c.want)
		}
	}
}

// Roadmap item 2 — atom-keyed `set` on an object/class instance (a field write
// like `p set x 7`, whose quoted key previously declined as "quoted-operand
// word set") lowers to CALL_NATIVE. The receiver is a non-const instance so
// the in-place mutation is safe, exactly as integer-keyed `set 1 v arr` already
// relied on.
func TestObjectClassSetLowers(t *testing.T) {
	// Positive: a declared-field write compiles natively and is visible after.
	const ok = `def Point class {x:1} def p (make Point {}) p set x 7 end p.x`
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	prog, reason, _, cerr := a.CompileCheck(ok)
	if cerr != nil || prog == nil {
		t.Fatalf("object set did not compile: reason=%q err=%v", reason, cerr)
	}
	dis := prog.Disassemble()
	if strings.Contains(dis, "FALLBACK") || !strings.Contains(dis, "set") {
		t.Errorf("expected a native CALL_NATIVE set, got:\n%s", dis)
	}
	gotC, compiled, errC := a.RunCompiled(ok)
	if noteCompileDefect(t, ok, gotC, errC) {
		return
	}
	if !compiled || errC != nil {
		t.Fatalf("object set run: compiled=%v err=%v", compiled, errC)
	}
	b, _ := New()
	gotI, _ := b.RunInterp(ok)
	if fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotC) != "[7]" {
		t.Errorf("object set: compiled=%v interp=%v (want [7])", gotC, gotI)
	}

	// Negative: an undeclared (sealed) field write raises the SAME sealed_field
	// error in both engines — the compiled mutator must not silently succeed.
	const bad = `def Point class {x:1} def p (make Point {}) p set z 9`
	c, _ := New()
	_, _, errCbad := c.RunCompiled(bad)
	if noteCompileDefect(t, bad, nil, errCbad) {
		return
	}
	d, _ := New()
	_, errIbad := d.RunInterp(bad)
	if codeOf(errCbad) != "sealed_field" || codeOf(errCbad) != codeOf(errIbad) {
		t.Errorf("sealed-field set: compiled=[%s] interp=[%s] (want sealed_field both)",
			codeOf(errCbad), codeOf(errIbad))
	}
}

// Roadmap item 6 — a computed-else `if cond [then] (expr)` (the else is an
// eagerly-evaluated paren result on the stack, not a literal/local) lowers via
// SWAP (cond to top) + JMP_IF_FALSE + OpDrop (discard the else value on the
// taken path), instead of declining "computed else value". Both branch
// directions must match the interpreter.
func TestComputedElseIfLowers(t *testing.T) {
	const src = `if (1 eq 1) [99] (add 1 2)`
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	prog, reason, _, cerr := a.CompileCheck(src)
	if cerr != nil || prog == nil {
		t.Fatalf("computed-else if did not compile: reason=%q err=%v", reason, cerr)
	}
	dis := prog.Disassemble()
	if strings.Contains(dis, "FALLBACK") || !strings.Contains(dis, "DROP") {
		t.Errorf("expected a native SWAP/JMP_IF_FALSE/DROP lowering:\n%s", dis)
	}

	// Both directions + downstream consumption, value parity with the interpreter.
	for _, c := range []struct {
		src  string
		want string
	}{
		{`if (1 eq 1) [99] (add 1 2)`, "99"},           // taken: then; else value dropped
		{`if (1 eq 2) [99] (add 1 2)`, "3"},            // not taken: computed else is the result
		{`add 10 (if (1 eq 1) [99] (add 1 2))`, "109"}, // consumed downstream
	} {
		b, _ := New()
		gotC, compiled, errC := b.RunCompiled(c.src)
		if noteCompileDefect(t, c.src, gotC, errC) {
			continue
		}
		if !compiled || errC != nil {
			t.Fatalf("%q: compiled=%v err=%v", c.src, compiled, errC)
		}
		d, _ := New()
		gotI, _ := d.RunInterp(c.src)
		if fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotC) != "["+c.want+"]" {
			t.Errorf("%q: compiled=%v interp=%v (want [%s])", c.src, gotC, gotI, c.want)
		}
	}
}

// Roadmap item 6 (computed-THEN) — the mirror of the computed-else case:
// `if cond (expr) e` where the THEN is an eagerly-evaluated paren result on the
// stack. It lowers via SWAP (cond to top) + JMP_IF_FALSE, keeping the eager then
// value on the TRUE (fall-through) path and DROPping it on the FALSE path before
// producing the else arm. Covers a value else, a body else, and both directions;
// the both-arms-computed shape stays declined (the negative half).
func TestComputedThenIfLowers(t *testing.T) {
	const src = `def x 0  if (x eq 0) (add 1 2) 88`
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	prog, reason, _, cerr := a.CompileCheck(src)
	if cerr != nil || prog == nil {
		t.Fatalf("computed-then if did not compile: reason=%q err=%v", reason, cerr)
	}
	dis := prog.Disassemble()
	if strings.Contains(dis, "FALLBACK") || !strings.Contains(dis, "DROP") || !strings.Contains(dis, "SWAP") {
		t.Errorf("expected a native SWAP/JMP_IF_FALSE/DROP lowering:\n%s", dis)
	}

	for _, c := range []struct {
		src  string
		want string
	}{
		{`def x 0  if (x eq 0) (add 1 2) 88`, "3"},          // taken: computed then is the result
		{`def x 1  if (x eq 0) (add 1 2) 88`, "88"},         // not taken: value else
		{`def x 0  if (x eq 0) (add 1 2) [sub 10 1]`, "3"},  // taken, body else
		{`def x 1  if (x eq 0) (add 1 2) [sub 10 1]`, "-9"}, // not taken, body else
		{`add 10 (if (1 eq 1) (add 1 2) 88)`, "13"},         // consumed downstream
	} {
		b, _ := New()
		gotC, compiled, errC := b.RunCompiled(c.src)
		if noteCompileDefect(t, c.src, gotC, errC) {
			continue
		}
		if !compiled || errC != nil {
			t.Fatalf("%q: compiled=%v err=%v", c.src, compiled, errC)
		}
		d, _ := New()
		gotI, _ := d.RunInterp(c.src)
		if fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotC) != "["+c.want+"]" {
			t.Errorf("%q: compiled=%v interp=%v (want [%s])", c.src, gotC, gotI, c.want)
		}
	}

	// Both arms computed (`if c (a) (b)`) now compiles too — see
	// TestBothComputedIfLowers for the dedicated coverage.
}

// Roadmap item 6 (computed-arm CONDITIONS) — a computed then/else arm compiles
// not only under a pre-evaluated event condition (`if (x eq 0) (expr) e`, which
// needs a SWAP) but also under a LIST-FORM condition body (`if [x gt 0] (expr) e`
// — lowered inline above the eager value, no SWAP) and a CONST/LOCAL condition
// (`if flag (expr) e` — pushed above the eager value). All must compile native
// and match the interpreter in both directions, for both computed-then and
// computed-else.
func TestComputedArmConditions(t *testing.T) {
	cases := []struct {
		src  string
		want string
	}{
		// list-form condition, computed then
		{`def x 0  if [x eq 0] (add 1 2) 88`, "3"},
		{`def x 1  if [x eq 0] (add 1 2) 88`, "88"},
		// list-form condition, computed else
		{`def x 0  if [x eq 0] 99 (add 1 2)`, "99"},
		{`def x 1  if [x eq 0] 99 (add 1 2)`, "3"},
		// local (fn-param) condition, computed then
		{`def f fn [[flag:Boolean] [Integer] [if flag (add 1 2) 88]]  f true`, "3"},
		{`def f fn [[flag:Boolean] [Integer] [if flag (add 1 2) 88]]  f false`, "88"},
		// local condition, computed else
		{`def g fn [[flag:Boolean] [Integer] [if flag 99 (add 1 2)]]  g true`, "99"},
		{`def g fn [[flag:Boolean] [Integer] [if flag 99 (add 1 2)]]  g false`, "3"},
	}
	for _, c := range cases {
		a, _ := New()
		prog, reason, _, cerr := a.CompileCheck(c.src)
		if cerr != nil || prog == nil {
			t.Errorf("%q: must compile (computed-arm condition); reason=%q err=%v", c.src, reason, cerr)
			continue
		}
		if strings.Contains(prog.Disassemble(), "FALLBACK") {
			t.Errorf("%q: must compile native, not island:\n%s", c.src, prog.Disassemble())
			continue
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

// Roadmap item 6 (BOTH arms computed) — `if (c) (a) (b)` where both arms are
// eagerly-computed parens. Both run (paren args are eager — faithful to the
// interpreter), so the lowering only SELECTS: it rotates the cond to the top
// (OpReverse 3) and drops the unselected value. Covers both directions, an
// event condition, and downstream consumption. The remaining bounded case — a
// non-event (list-form / const) condition with both arms computed — stays
// declined (the negative half).
func TestBothComputedIfLowers(t *testing.T) {
	// Legacy compile failure+fallback-parity contract: pins the one-release
	const src = `add 10 (if (1 eq 1) (add 1 2) (sub 9 4))`
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	prog, reason, _, cerr := a.CompileCheck(src)
	if cerr != nil || prog == nil {
		t.Fatalf("both-computed if did not compile: reason=%q err=%v", reason, cerr)
	}
	dis := prog.Disassemble()
	if strings.Contains(dis, "FALLBACK") || !strings.Contains(dis, "REVERSE") {
		t.Errorf("expected a native OpReverse select lowering:\n%s", dis)
	}

	for _, c := range []struct {
		src  string
		want string
	}{
		{`if (1 eq 1) (add 1 2) (sub 9 4)`, "3"},           // true: then
		{`if (1 eq 2) (add 1 2) (sub 9 4)`, "-5"},          // false: else (4-9)
		{`add 10 (if (1 eq 1) (add 1 2) (sub 9 4))`, "13"}, // consumed downstream, true
		{`add 10 (if (1 eq 2) (add 1 2) (sub 9 4))`, "5"},  // consumed downstream, false
		{`def n 7  if (n eq 0) (mul 2 3) (mul 4 5)`, "20"}, // false: else, dynamic cond
	} {
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

	// GRADUATED 2026-07-17 (the S9.4 probe sweep): both arms computed with a
	// NON-event cond now compiles — lowerBothComputedMatCond materialises the
	// cond above the two eager values exactly as the single-computed
	// lowerComputedCond does, then JMP_IF_FALSE selects.
	const listCond = `if [1 eq 1] (add 1 2) (sub 9 4)`
	nb, _ := New()
	gotC, compiled, errC := nb.RunCompiled(listCond)
	if noteCompileDefect(t, listCond, gotC, errC) {
		return
	}
	nbi, _ := New()
	gotI, _ := nbi.RunInterp(listCond)
	if !compiled || errC != nil || fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotC) != "[3]" {
		t.Errorf("%q: want native compile with parity [3], got compiled=%v gotC=%v errC=%v gotI=%v", listCond, compiled, gotC, errC, gotI)
	}
}

// Roadmap item 6b — a statement-`if` where exactly one arm nets a value and the
// other nets 0 WITHOUT diverging (`if c [99] []`, `if c [] [99]`,
// `if c [raise] [99]`) lowers as a VARIADIC (0-or-1) branch result instead of
// declining "branch produces no value". Both directions, the empty arm, and the
// raise-guard (which errors on its taken path) must match the interpreter.
func TestVariadicElseIfLowers(t *testing.T) {
	cases := []struct {
		src       string
		want      string // expected residual when no error
		wantError bool
	}{
		{`if (1 gt 0) [99] []`, "[99]", false},          // then value taken
		{`if (1 eq 2) [99] []`, "[]", false},            // empty else taken → nothing
		{`if (1 eq 2) [] [99]`, "[99]", false},          // else value taken
		{`if (1 eq 1) [] [99]`, "[]", false},            // empty then taken → nothing
		{`if (1 eq 2) [raise "x"] [99]`, "[99]", false}, // guard not taken
		{`if (1 eq 1) [raise "x"] [99]`, "", true},      // guard taken → raises
	}
	for _, c := range cases {
		a, _ := New()
		gotC, compiled, errC := a.RunCompiled(c.src)
		if noteCompileDefect(t, c.src, gotC, errC) {
			continue
		}
		b, _ := New()
		gotI, errI := b.RunInterp(c.src)
		if (errC != nil) != (errI != nil) {
			t.Errorf("%q: error parity compiled=%v interp=%v", c.src, errC, errI)
			continue
		}
		if c.wantError {
			if errC == nil {
				t.Errorf("%q: expected an error (raise on the taken path), got none", c.src)
			}
			continue
		}
		if !compiled {
			t.Errorf("%q: did not run compiled", c.src)
		}
		if fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotC) != c.want {
			t.Errorf("%q: compiled=%v interp=%v (want %s)", c.src, gotC, gotI, c.want)
		}
	}
}

// Roadmap item (fn-value introspection) — a type-reading word (typeof / tcmp /
// teq / tand / tor / tnot) over a fn VALUE bakes the immutable fn as a const
// the handler inspects, instead of declining "function value reaches word". A
// fn-INVOKING use must stay declined (the VM cannot re-step a fn body).
func TestFnValueIntrospectionLowers(t *testing.T) {
	const setup = `def Positive fnpred n:Integer [if (n gt 0) [n] [None]] `
	for _, c := range []struct {
		src  string
		want string
	}{
		{`typeof (fn [[a:Integer][Integer][a add 1]])`, "[Function]"},
		{setup + `Positive tcmp Positive`, "[0]"}, // equal
		// Stage 2 flip (design/legacy/TYPE-REPRESENTATION.1.ignore §6, tcmp row): the
		// name denotes its minted node, so a named predicate type orders as
		// a TYPE (lattice band: the subtype below its base), not as a
		// concrete fn value above the Function literal.
		{setup + `Positive tcmp Function`, "[-1]"},
		{setup + `Function tcmp Positive`, "[1]"},
	} {
		a, _ := New()
		prog, reason, _, cerr := a.CompileCheck(c.src)
		if cerr != nil || prog == nil {
			t.Errorf("%q: did not compile: reason=%q", c.src, reason)
			continue
		}
		if strings.Contains(prog.Disassemble(), "FALLBACK") {
			t.Errorf("%q: expected native, got island", c.src)
		}
		ar, _ := New()
		gotC, compiled, errC := ar.RunCompiled(c.src)
		if noteCompileDefect(t, c.src, gotC, errC) {
			continue
		}
		b, _ := New()
		gotI, _ := b.RunInterp(c.src)
		if !compiled || errC != nil || fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotC) != c.want {
			t.Errorf("%q: compiled=%v gotC=%v gotI=%v (want %s)", c.src, compiled, gotC, gotI, c.want)
		}
	}

	// The former negative, kept as a POSITIVE with its history.
	//
	// This asserted that `is` over a predicate fn must not compile, "because
	// it INVOKES the fn". The premise held and the conclusion did not follow:
	// invoking is not re-stepping. A predicate NODE rides as data and its body
	// runs through the callback seam, so the tape hazard introspection is
	// exempted from was never present here either. What genuinely blocked it
	// was that the callback seam interpreted the body — fixed in
	// eng/go/vm_foreign_unit.go, after which these rows compile with no
	// interpreter entry at all (design/FULL-COMPILATION.0.md §6.3).
	const inv = `def Positive fnpred n:Integer [if (n gt 0) [n] [None]] 5 is Positive`
	c, _ := New()
	gotInv, compiled, errInv := c.RunCompiled(inv)
	if noteCompileDefect(t, inv, gotInv, errInv) {
		return
	}
	if !compiled || errInv != nil {
		t.Errorf("`is` over a predicate fn: compiled=%v err=%v, want a compiled run", compiled, errInv)
	}
	d, _ := New()
	wantInv, _ := d.RunInterp(inv)
	if fmt.Sprint(gotInv) != fmt.Sprint(wantInv) {
		t.Errorf("`is` over a predicate fn: compiled=%v interpreted=%v", gotInv, wantInv)
	}
}

// Roadmap item 5 (part A) — each/fold/filter over a MAP compiles NATIVE
// instead of islanding: the value-body closure runs per map value through the
// InvokeBody seam (newMapBody routes a compiled closure like a quotation), and
// the closure's input type is the map's common VALUE type. The interpreter's
// token-body path is unchanged.
func TestMapIterationCompilesNative(t *testing.T) {
	for _, c := range []struct {
		src  string
		want string
	}{
		{`each [mul 10] {a:1 b:2}`, "[{a:10 b:20}]"},
		{`{a:1 b:2} each [mul 10]`, "[{a:10 b:20}]"},
		{`fold [add] {a:1 b:2 c:3} 0`, "[6]"},
		{`filter [gt 2] {a:1 b:5 c:3}`, "[{b:5 c:3}]"},
	} {
		a, _ := New()
		prog, reason, _, cerr := a.CompileCheck(c.src)
		if cerr != nil || prog == nil {
			t.Errorf("%q: did not compile: reason=%q", c.src, reason)
			continue
		}
		if strings.Contains(prog.Disassemble(), "FALLBACK") {
			t.Errorf("%q: expected a native closure lowering, got an island", c.src)
		}
		ar, _ := New()
		gotC, compiled, errC := ar.RunCompiled(c.src)
		if noteCompileDefect(t, c.src, gotC, errC) {
			continue
		}
		b, _ := New()
		gotI, _ := b.RunInterp(c.src)
		if !compiled || errC != nil || fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotC) != c.want {
			t.Errorf("%q: compiled=%v gotC=%v gotI=%v (want %s)", c.src, compiled, gotC, gotI, c.want)
		}
	}

	// Regression: list iteration still compiles native + correct.
	a, _ := New()
	gotC, compiled, _ := a.RunCompiled(`each [mul 2] [1 2 3]`)
	if !compiled || fmt.Sprint(gotC) != "[[2 4 6]]" {
		t.Errorf("list each regressed: compiled=%v got=%v", compiled, gotC)
	}
}

// Roadmap item 5 part B (filter lambda args) — a `filter ([p] => …) data`
// lambda compiles its afn body to a closure with the word's callback input
// shape ({key,value} pair over a list, KeyVal over a map), driven natively
// through InvokeBody rather than islanding or declining. Verifies the closure
// path is value- and taxonomy-identical to the interpreter.
func TestFilterLambdaCompilesNative(t *testing.T) {
	for _, c := range []struct {
		src  string
		want string
	}{
		// list filter: the lambda reads the element via the {key,value} pair.
		{`filter ([p:Any] => [(p.value mod 2) eq 0]) [1 2 3 4 5 6]`, "[[2 4 6]]"},
		{`filter ([p:Any] => [p.value gt 3]) [1 2 3 4 5]`, "[[4 5]]"},
		// map filter: the lambda reads the value via a KeyVal, shape preserved.
		{`filter ([kv:KeyVal] => [(kv.v mod 2) eq 0]) {a:1 b:2 c:3 d:4}`, "[{b:2 d:4}]"},
		{`{a:1 b:5 c:3} filter ([kv:KeyVal] => [kv.v gt 2])`, "[{b:5 c:3}]"},
	} {
		a, _ := New()
		prog, reason, _, cerr := a.CompileCheck(c.src)
		if cerr != nil || prog == nil {
			t.Errorf("%q: did not compile: reason=%q", c.src, reason)
			continue
		}
		if strings.Contains(prog.Disassemble(), "FALLBACK") {
			t.Errorf("%q: expected a native closure lowering, got an island", c.src)
		}
		ar, _ := New()
		gotC, compiled, errC := ar.RunCompiled(c.src)
		if noteCompileDefect(t, c.src, gotC, errC) {
			continue
		}
		b, _ := New()
		gotI, _ := b.RunInterp(c.src)
		if !compiled || errC != nil || fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotC) != c.want {
			t.Errorf("%q: compiled=%v gotC=%v gotI=%v (want %s)", c.src, compiled, gotC, gotI, c.want)
		}
	}

	// A non-Boolean map predicate is a loud filter_error in BOTH engines (the
	// compiled closure runs the same strict check as the interpreter path).
	bad := `{a:1 b:5} filter ([kv:KeyVal] => [kv.v])`
	a, _ := New()
	_, compiled, errC := a.RunCompiled(bad)
	if noteCompileDefect(t, bad, nil, errC) {
		return
	}
	b, _ := New()
	_, errI := b.RunInterp(bad)
	if !compiled || errC == nil || errI == nil {
		t.Errorf("%q: want both engines to error (compiled=%v errC=%v errI=%v)", bad, compiled, errC, errI)
	}
}

// args.N — `args.N` inside a fn body reads the N-th call argument. The params
// ARE the frame's leading locals, so AnalyseFnBody projects them as the args
// list and `get N args` folds to PUSH_LOCAL N (carrier.go tryFoldStaticIndex) —
// no runtime args stack. The compiled body is byte-for-byte the named-param
// form. (Concrete proof that the P7-gate "tier 2" words are reducible, not
// irreducible.) Bare `args` (the whole list) still declines, by design.
func TestArgsAccessorCompilesNative(t *testing.T) {
	for _, c := range []struct {
		src  string
		want string
	}{
		{`def f fn [[a:Integer b:Integer] [Integer] [args.0 add args.1]] f 3 4`, "[7]"},
		{`def f fn [[a:Integer b:Integer] [Integer] [args.1 sub args.0]] f 10 3`, "[-7]"},
		{`def f fn [[n:Integer] [Integer] [if (n lte 0) [args.0] [f (n sub 1)]]] f 3`, "[0]"},
	} {
		a, _ := New()
		prog, reason, _, cerr := a.CompileCheck(c.src)
		if cerr != nil || prog == nil {
			t.Errorf("%q: did not compile: reason=%q", c.src, reason)
			continue
		}
		if strings.Contains(prog.Disassemble(), "FALLBACK") {
			t.Errorf("%q: expected a native lowering, got an island", c.src)
		}
		// args.N must lower to a frame-local read, not a runtime args call.
		if strings.Contains(prog.Disassemble(), "CALL_NATIVE_POLY") && strings.Contains(c.src, "args.0 add") {
			t.Errorf("%q: args.N did not fold to a local (poly dot remains):\n%s", c.src, prog.Disassemble())
		}
		ar, _ := New()
		gotC, compiled, errC := ar.RunCompiled(c.src)
		if noteCompileDefect(t, c.src, gotC, errC) {
			continue
		}
		b, _ := New()
		gotI, _ := b.RunInterp(c.src)
		if !compiled || errC != nil || fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotC) != c.want {
			t.Errorf("%q: compiled=%v gotC=%v gotI=%v (want %s)", c.src, compiled, gotC, gotI, c.want)
		}
	}

	// The named-param form is byte-identical (args.N is just another spelling).
	a, _ := New()
	p1, _, _, _ := a.CompileCheck(`def f fn [[a:Integer b:Integer] [Integer] [args.0 add args.1]] f 3 4`)
	b, _ := New()
	p2, _, _, _ := b.CompileCheck(`def f fn [[a:Integer b:Integer] [Integer] [a add b]] f 3 4`)
	if p1 == nil || p2 == nil || p1.Disassemble() != p2.Disassemble() {
		t.Errorf("args.N body should compile identically to the named-param body")
	}
}

// word (Forth-style macro splice) — `def x word [body]` binds x to an __SP
// splice marker; at each use site stepLiteral inlines the body and re-steps it
// against the live stack. The bytecode does the SAME thing: carrierResults
// produces the marker as a non-emitting compile-time value (toCarrier preserves
// it), and the use-site splice expands inline — so the body's instructions land
// in place, late binding and all. Proof that the gate's "tier 2" is reducible:
// `word` was the 30-row macro-splice cluster I'd called irreducible meta.
func TestWordSpliceCompilesNative(t *testing.T) {
	for _, c := range []struct {
		src  string
		want string
	}{
		{`def x word [1,2,3] x`, "[1 2 3]"},
		{`word [1 add 2]`, "[3]"}, // splices 1 add 2 -> evaluates inline
		{`def dbl word [dup add] 5 dbl`, "[10]"},
		// Late binding: the splice re-resolves c1 at the USE site (= 20), not at
		// definition (= 10) — the exact case the bytecode "freezes things"
		// objection predicted would break, and does not.
		{`def c1 10 def x word [c1 2] def c1 20 x`, "[20 2]"},
		{`def a word [1,2] def b word [a 3] b`, "[1 2 3]"}, // recursive splice
	} {
		a, _ := New()
		prog, reason, _, cerr := a.CompileCheck(c.src)
		if cerr != nil || prog == nil {
			t.Errorf("%q: did not compile: reason=%q", c.src, reason)
			continue
		}
		if strings.Contains(prog.Disassemble(), "FALLBACK") {
			t.Errorf("%q: expected an inline native lowering, got an island", c.src)
		}
		ar, _ := New()
		gotC, compiled, errC := ar.RunCompiled(c.src)
		if noteCompileDefect(t, c.src, gotC, errC) {
			continue
		}
		b, _ := New()
		gotI, _ := b.RunInterp(c.src)
		if !compiled || errC != nil || fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotC) != c.want {
			t.Errorf("%q: compiled=%v gotC=%v gotI=%v (want %s)", c.src, compiled, gotC, gotI, c.want)
		}
	}

	// A splice of an undefined word errors in BOTH engines (the inline
	// expansion surfaces the undefined_word at check time / run time alike).
	a, _ := New()
	_, _, errC := a.RunCompiled(`def x word [nope 2 3] x`)
	if noteCompileDefect(t, `def x word [nope 2 3] x`, nil, errC) {
		return
	}
	b, _ := New()
	_, errI := b.RunInterp(`def x word [nope 2 3] x`)
	if (errC == nil) != (errI == nil) {
		t.Errorf("word-splice undefined: error divergence compiled=%v interp=%v", errC, errI)
	}
}

// macroexpand — Lisp-style: when the macro and its operands are static the
// expansion is a compile-time computation, so carrierResults runs it and bakes
// the resulting token list as a code-as-data const (a Word is admitted as a
// const MEMBER — isInertConstMember). The compiled result is the same data list
// the interpreter returns. A too-deep recursive expansion at top level compiles
// to a terminal OpTrap raising the byte-identical macroexpand_error; a nested
// occurrence declines the trap and falls back. Proof that macroexpand is
// reducible compiler work, not irreducible reflection.
func TestMacroexpandCompilesNative(t *testing.T) {
	// Legacy compile failure+fallback-parity contract: pins the one-release
	for _, c := range []struct {
		src  string
		want string
	}{
		{`def twice (macro [[e] [ quote [ unquote e add unquote e ] ]])  macroexpand (twice 5)`, "[[5 word(add) 5]]"},
		{`def innr (macro [[x] [quote [unquote x add 1]]])  def outr (macro [[y] [quote [innr unquote y]]])  macroexpand (outr 5)`, "[[5 word(add) 1]]"},
	} {
		a, _ := New()
		prog, reason, _, cerr := a.CompileCheck(c.src)
		if cerr != nil || prog == nil {
			t.Errorf("%q: did not compile: reason=%q", c.src, reason)
			continue
		}
		if strings.Contains(prog.Disassemble(), "FALLBACK") {
			t.Errorf("%q: expected a baked code-const, got an island", c.src)
		}
		ar, _ := New()
		gotC, compiled, errC := ar.RunCompiled(c.src)
		if noteCompileDefect(t, c.src, gotC, errC) {
			continue
		}
		b, _ := New()
		gotI, _ := b.RunInterp(c.src)
		if !compiled || errC != nil || fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotC) != c.want {
			t.Errorf("%q: compiled=%v gotC=%v gotI=%v (want %s)", c.src, compiled, gotC, gotI, c.want)
		}
	}

	// A recursive (too-deep) macro at TOP LEVEL now compiles to a TERMINAL
	// OpTrap: the macro head is captured raw as macroexpand's FormArg,
	// macroexpand dispatches in check mode, ExpandMacroForm runs the expansion
	// to the depth guard, and the resulting macroexpand_error is recorded as a
	// trap (mirroring the mini/parse/emit *_unknown_lang expansion-time traps).
	// The compiled program raises the byte-identical error the interpreter does.
	deep := `def loopy (macro [[a] [quote [loopy unquote a]]])  macroexpand (loopy 1)`
	a, _ := New()
	prog, reason, _, cerr := a.CompileCheck(deep)
	if cerr != nil || prog == nil {
		t.Fatalf("too-deep macroexpand: expected compile-to-trap, declined: reason=%q err=%v", reason, cerr)
	}
	dis := prog.Disassemble()
	if !strings.Contains(dis, "TRAP") || !strings.Contains(dis, "macroexpand_error") {
		t.Errorf("too-deep macroexpand: expected an OpTrap for macroexpand_error, got:\n%s", dis)
	}
	if strings.Contains(dis, "FALLBACK") {
		t.Errorf("too-deep macroexpand: expected a terminal trap, got an island:\n%s", dis)
	}
	// Both engines raise the IDENTICAL error (taxonomy + detail byte-match).
	ar, _ := New()
	_, compiled, errC := ar.RunCompiled(deep)
	if noteCompileDefect(t, deep, nil, errC) {
		return
	}
	b, _ := New()
	_, errI := b.RunInterp(deep)
	if !compiled {
		t.Errorf("too-deep macroexpand: must run on the compiled path (the trap), not fall back")
	}
	if errC == nil || errI == nil {
		t.Fatalf("too-deep macroexpand: both engines must error (compiled=%v interp=%v)", errC, errI)
	}
	if codeOf(errC) != "macroexpand_error" || codeOf(errC) != codeOf(errI) {
		t.Errorf("too-deep macroexpand: code mismatch compiled=%q interp=%q", codeOf(errC), codeOf(errI))
	}
	if errC.Error() != errI.Error() {
		t.Errorf("too-deep macroexpand: detail must byte-match\n  compiled=%q\n  interp  =%q", errC.Error(), errI.Error())
	}

	// Determinism: the compile-to-trap outcome must be identical every time —
	// the recursive expansion advances the time-seeded global value-ID RNG
	// (value.go) which feeds the provenance map; the trap must not be sensitive
	// to it. 20 in-process CompileChecks must all yield the same single OpTrap.
	for i := 0; i < 20; i++ {
		d, _ := New()
		p, _, _, _ := d.CompileCheck(deep)
		if p == nil || p.Disassemble() != dis {
			t.Fatalf("too-deep macroexpand: non-deterministic compile on iteration %d\n  first:\n%s\n  now:\n%s", i, dis, func() string {
				if p == nil {
					return "<declined>"
				}
				return p.Disassemble()
			}())
		}
	}

	// NEGATIVE — a NESTED occurrence (inside an if-branch fragment, not at the
	// top-level unit/frame) must DECLINE the trap (RecordTrap is top-level-only)
	// and keep the lenient fallback: the row fails to compile and the
	// interpreter surfaces the identical error. Confirms the trap is conditional
	// only where it is provably reached.
	nested := `def loopy (macro [[a] [quote [loopy unquote a]]])  if true [ macroexpand (loopy 1) ] [ 0 ]`
	c, _ := New()
	np, _, _, _ := c.CompileCheck(nested)
	if np != nil {
		t.Errorf("nested too-deep macroexpand: must decline (top-level-only trap), but compiled:\n%s", np.Disassemble())
	}
	cn, _ := New()
	_, _, nerrC := cn.RunCompiled(nested)
	if noteCompileDefect(t, nested, nil, nerrC) {
		return
	}
	dn, _ := New()
	_, nerrI := dn.RunInterp(nested)
	if nerrC == nil || nerrI == nil || nerrC.Error() != nerrI.Error() {
		t.Errorf("nested too-deep macroexpand: error parity broken compiled=%v interp=%v", nerrC, nerrI)
	}
}

// Dynamic-help budget isolation — the OnRegisterHook synthetic example eval
// (native_help.go) shares the registry's check-mode StepCount, and the engine's
// check loop short-circuits every sub-engine once BudgetTripped. A documentation
// example that loops to the step ceiling — a RECURSIVE macro registered via
// `def` — would otherwise burn the real program's budget and abort its later
// statements before they are checked. IsolateBudget (the fourth hermetic channel
// alongside IsolateEmit / TruncateDiagnostics / Defs restore) snapshots and
// restores the counters so the synthetic eval cannot abort the program's
// compile. This is exactly what unblocked macro.tsv:45's compile-to-trap: with
// the leak, `macroexpand (loopy 1)` was never reached.
func TestDynamicHelpBudgetIsolation(t *testing.T) {
	// A recursive macro DEFINITION followed by an ordinary, statically
	// materialisable statement: without budget isolation the def's synthetic
	// help eval exhausts the budget and the `add` never compiles. With it, the
	// trailing statement compiles to a plain native call.
	const src = `def loopy (macro [[a] [quote [loopy unquote a]]])  1 add 2`
	a, _ := New()
	prog, reason, _, cerr := a.CompileCheck(src)
	if cerr != nil || prog == nil {
		t.Fatalf("expected the trailing statement to compile despite the recursive-macro def: reason=%q err=%v", reason, cerr)
	}
	dis := prog.Disassemble()
	if strings.Contains(dis, "TRAP") || strings.Contains(dis, "step_budget") {
		t.Errorf("recursive-macro def must not pollute the program's compile via the help eval:\n%s", dis)
	}
	out, compiled, errC := a.RunCompiled(src)
	if noteCompileDefect(t, src, out, errC) {
		return
	}
	if !compiled || errC != nil || fmt.Sprint(out) != "[3]" {
		t.Errorf("trailing statement: compiled=%v out=%v err=%v (want [3])", compiled, out, errC)
	}
}

// Nested-variadic branch lowering — a no-default `case` desugars to a nested
// if-chain whose innermost `if guard [block]` has no else, so the chain is
// variadic (0-or-1: a value on a match, nothing on no-match). The lowerer now
// lets a branch arm CARRY a variadic (0-or-1) result and propagates the
// variadic-ness up to the residual, so a 2+-clause no-default case compiles.
func TestNestedVariadicCaseCompiles(t *testing.T) {
	for _, c := range []struct {
		src  string
		want string
	}{
		{`def Big (Integer gt 10) case 50 [Big "big" Integer "small-int"]`, "[big]"},
		{`def Big (Integer gt 10) case 5 [Big "big" Integer "small-int"]`, "[small-int]"},
		{`if true [if false [9]]`, "[]"}, // bare nested-variadic if -> 0 values
	} {
		a, _ := New()
		prog, reason, _, cerr := a.CompileCheck(c.src)
		if cerr != nil || prog == nil {
			t.Errorf("%q: did not compile: reason=%q", c.src, reason)
			continue
		}
		if strings.Contains(prog.Disassemble(), "FALLBACK") {
			t.Errorf("%q: expected a native if-chain, got an island", c.src)
		}
		ar, _ := New()
		gotC, compiled, errC := ar.RunCompiled(c.src)
		if noteCompileDefect(t, c.src, gotC, errC) {
			continue
		}
		b, _ := New()
		gotI, _ := b.RunInterp(c.src)
		if !compiled || errC != nil || fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotC) != c.want {
			t.Errorf("%q: compiled=%v gotC=%v gotI=%v (want %s)", c.src, compiled, gotC, gotI, c.want)
		}
	}

	// A variadic case result CONSUMED by another word can't compile (the
	// consumer needs a fixed count) — it must still fall back, with parity.
	const consumed = `def Big (Integer gt 10) [99 (case 5 [Big "big"]) 88]`
	a, _ := New()
	_, compiled, _ := a.RunCompiled(consumed)
	b, _ := New()
	gotI, _ := b.RunInterp(consumed)
	if compiled {
		t.Errorf("%q: a consumed variadic case must fall back, not compile", consumed)
	}
	if fmt.Sprint(gotI) != "[[99 88]]" {
		t.Errorf("%q: interp = %v, want [[99 88]]", consumed, gotI)
	}
}

// usurp (and the /u suffix) — a static arg permutation: `usurp f` wraps f so a
// call reverses the arg order (`usurped a b c` ≡ `f c b a`). The wrapper's
// re-dispatch handler now runs in check mode, so the carrier compiler steps the
// re-dispatch and compiles the reversed ORIGINAL call directly — there was never
// any "tape coupling" to model, just an opaque wrapper the compiler stepped past.
func TestUsurpCompilesNative(t *testing.T) {
	for _, c := range []struct {
		src  string
		want string
	}{
		{`def sub2 fn [[a:Integer b:Integer][Integer][a sub b]]  usurp (valof sub2) 10 3`, "[-7]"},
		{`def sub2 fn [[a:Integer b:Integer][Integer][a sub b]]  sub2/u 10 3`, "[-7]"},
		{`def inc fn [[n:Integer][Integer][n add 1]]  inc/u 5`, "[6]"}, // 1-arg usurp is a no-op on order
		{`def cat3 fn [[a:String b:String c:String][String][a add b add c]]  cat3/u 'x' 'y' 'z'`, "[zyx]"},
	} {
		a, _ := New()
		prog, reason, _, cerr := a.CompileCheck(c.src)
		if cerr != nil || prog == nil {
			t.Errorf("%q: did not compile: reason=%q", c.src, reason)
			continue
		}
		if strings.Contains(prog.Disassemble(), "FALLBACK") {
			t.Errorf("%q: expected a native reversed-call lowering, got an island", c.src)
		}
		ar, _ := New()
		gotC, compiled, errC := ar.RunCompiled(c.src)
		if noteCompileDefect(t, c.src, gotC, errC) {
			continue
		}
		b, _ := New()
		gotI, _ := b.RunInterp(c.src)
		if !compiled || errC != nil || fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotC) != c.want {
			t.Errorf("%q: compiled=%v gotC=%v gotI=%v (want %s)", c.src, compiled, gotC, gotI, c.want)
		}
	}
}

// make computed container defaults — a class field default (or data-map value)
// that is itself a COMPUTED paren-expr ((make Foo 1), (1 add 2)) used to decline:
// in check mode the inner computation recorded an event the container swallowed
// ("unconsumed call results"). A deterministic computed value is a compile-time
// constant, so it now const-folds (constFoldContainerVal: two non-recording
// concrete evals that must agree), and the container bakes. An INSTANCE default
// bakes only as a class-schema member (make copies it per instance — mutation-
// safe), never as a data-map member. Drives `make`-class-default compilation.
func TestMakeComputedDefaultsCompile(t *testing.T) {
	for _, c := range []struct {
		src  string
		want string
	}{
		{`def Foo refine Integer end def S class {x:(make Foo 1)} end (make S {}) dot x`, "[1]"},
		{`def Foo refine Integer end def S class {x:(make Foo 1)} end typeof ((make S {}) dot x)`, "[Foo]"},
		{`def Foo class {y:1} end def S class {x:(make Foo {})} end (make S {}) dot x`, "[Class/Foo{y:1}]"},
		{`def Foo refine Integer end def S class {x:(make Foo 7)} end (make S {x:(make Foo 9)}) dot x`, "[9]"},
		{`def S class {x:(1 add 2)} end (make S {}) dot x`, "[3]"},
		{`def m {a:(1 add 2)} m`, "[{a:3}]"}, // data map scalar default also const-folds
	} {
		a, _ := New()
		prog, reason, _, cerr := a.CompileCheck(c.src)
		if cerr != nil || prog == nil {
			t.Errorf("%q: did not compile: reason=%q", c.src, reason)
			continue
		}
		if strings.Contains(prog.Disassemble(), "FALLBACK") {
			t.Errorf("%q: expected a native const-bake, got an island", c.src)
		}
		ar, _ := New()
		gotC, compiled, errC := ar.RunCompiled(c.src)
		if noteCompileDefect(t, c.src, gotC, errC) {
			continue
		}
		b, _ := New()
		gotI, _ := b.RunInterp(c.src)
		if !compiled || errC != nil || fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotC) != c.want {
			t.Errorf("%q: compiled=%v gotC=%v gotI=%v (want %s)", c.src, compiled, gotC, gotI, c.want)
		}
	}

	// Per-instance copy isolation: make copies the baked schema default, so
	// mutating one instance's field must not affect another's.
	const iso = `def Foo class {n:1} end def S class {x:(make Foo {})} end def a (make S {}) def b (make S {}) (a.x set n 9) end b.x dot n`
	a, _ := New()
	gotC, compiled, _ := a.RunCompiled(iso)
	b, _ := New()
	gotI, _ := b.RunInterp(iso)
	if fmt.Sprint(gotC) != fmt.Sprint(gotI) {
		t.Errorf("per-instance isolation: compiled=%v interp=%v (must match)", gotC, gotI)
	}
	_ = compiled
}

// with-decimal block — a 0-input body run inside a scoped BigDecimal rounding
// context compiles to a closure (like `do`) driven through InvokeBody, so the
// VM-run body's decimal ops read the precision/rounding override exactly as the
// interpreter's doEvalList does. Verifies native lowering + parity, including a
// NESTED context (the inner override shadows the outer).
func TestWithDecimalCompilesNative(t *testing.T) {
	for _, c := range []struct {
		src  string
		want string
	}{
		{`with-decimal {precision: 5} [0d1.0 div 0d3.0]`, "[0d0.33333]"},
		{`with-decimal {precision: 4 rounding: "down"} [0d2.0 div 0d3.0]`, "[0d0.6666]"},
		{`with-decimal {precision: 6} [with-decimal {precision: 3} [0d1.0 div 0d3.0]]`, "[0d0.333]"},
	} {
		a, _ := New()
		prog, reason, _, cerr := a.CompileCheck(c.src)
		if cerr != nil || prog == nil {
			t.Errorf("%q: did not compile: reason=%q", c.src, reason)
			continue
		}
		if strings.Contains(prog.Disassemble(), "FALLBACK") {
			t.Errorf("%q: expected a native closure lowering, got an island", c.src)
		}
		ar, _ := New()
		gotC, compiled, errC := ar.RunCompiled(c.src)
		if noteCompileDefect(t, c.src, gotC, errC) {
			continue
		}
		b, _ := New()
		gotI, _ := b.RunInterp(c.src)
		if !compiled || errC != nil || fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotC) != c.want {
			t.Errorf("%q: compiled=%v gotC=%v gotI=%v (want %s)", c.src, compiled, gotC, gotI, c.want)
		}
	}
}

// Roadmap item 5 part B (each/fold/scan map lambda args) — the map-iteration
// words run a KeyVal-shaped lambda closure natively. These share the same
// handler as their token-quotation form, which sees the bare VALUE; the unit's
// recorded ClosureInShape (ClosureWantsKeyVal) keeps the two apart, so a
// KeyVal-destructuring lambda and a value-consuming quotation both compile and
// both stay correct.
func TestMapLambdaCompilesNative(t *testing.T) {
	for _, c := range []struct {
		src  string
		want string
	}{
		// KeyVal-shaped lambda closures (the lambda reads kv.v / kv.i / acc).
		{`{a:1 b:2} each ([kv:KeyVal] => [kv.v add kv.i])`, "[{a:1 b:3}]"},
		{`0 fold ([acc:Integer kv:KeyVal] => [acc add kv.v]) {a:1 b:2 c:3}`, "[6]"},
		{`{a:1 b:2 c:3} scan ([acc:Integer kv:KeyVal] => [acc add kv.v])`, "[{a:1 b:3 c:6}]"},
		// Token-quotation map closures share the handler but take the bare value
		// — the ClosureInShape flag must NOT wrap these in a KeyVal.
		{`each [mul 10] {a:1 b:2}`, "[{a:10 b:20}]"},
		{`fold [add] {a:1 b:2 c:3} 0`, "[6]"},
		{`{a:1 b:2 c:3} scan [add]`, "[{a:1 b:3 c:6}]"},
	} {
		a, _ := New()
		prog, reason, _, cerr := a.CompileCheck(c.src)
		if cerr != nil || prog == nil {
			t.Errorf("%q: did not compile: reason=%q", c.src, reason)
			continue
		}
		if strings.Contains(prog.Disassemble(), "FALLBACK") {
			t.Errorf("%q: expected a native closure lowering, got an island", c.src)
		}
		ar, _ := New()
		gotC, compiled, errC := ar.RunCompiled(c.src)
		if noteCompileDefect(t, c.src, gotC, errC) {
			continue
		}
		b, _ := New()
		gotI, _ := b.RunInterp(c.src)
		if !compiled || errC != nil || fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotC) != c.want {
			t.Errorf("%q: compiled=%v gotC=%v gotI=%v (want %s)", c.src, compiled, gotC, gotI, c.want)
		}
	}
}

// query DSL (boru:query) — the SQL-style query words are trivial-delegation
// module wrappers over inner natives. The clause words (select/where/order/
// group/having/limit/offset/distinct/on/using) carry NoEvalArgs whose clause is
// an inert word-list (`[name age]`, `[age gt 1]`) the compiler now bakes as a
// code-as-data const (noEvalBodiesInert). The source words (from/join/innerjoin/
// leftjoin/crossjoin) carry a QuoteArgs table-NAME atom the compiler now bakes
// via the module-inner quote exemption (quoteOperandInertOK). Either way the
// recorded dispatch is the inner native as a plain CALL_NATIVE, which the
// interpreter reaches identically through the wrapper's trivial delegation, so
// the lazy-query value is built the same on both paths. The spec rows do not
// seed a FROM table, so the query carries a deferred error and renders the same
// unforced value on both paths — the invariant under test is native compilation
// + exact compiled/interpreted parity (the compiler must not change behaviour),
// not the lazy-query rendering itself.
func TestQueryDSLCompilesNative(t *testing.T) {
	const imp = `import "boru:query"  `
	for _, src := range []string{
		imp + `Query.select [name age]`,
		imp + `Query.where [age gt 1] (Query.select [name])`,
		imp + `Query.order [age desc] (Query.select [name])`,
		imp + `Query.group [city] (Query.select [city])`,
		imp + `Query.having [cnt gt 1] (Query.group [city] (Query.select [city]))`,
		imp + `Query.limit 5 (Query.select [name])`,
		imp + `Query.offset 2 (Query.select [name])`,
		imp + `Query.distinct (Query.select [name])`,
		imp + `Query.join visits (Query.select [name])`,
		imp + `Query.innerjoin visits (Query.select [name])`,
		imp + `Query.leftjoin visits (Query.select [name])`,
		imp + `Query.crossjoin visits (Query.select [name])`,
		imp + `Query.on [name eq who] (Query.join visits (Query.select [name]))`,
		imp + `Query.using [name] (Query.join visits (Query.select [name]))`,
	} {
		a, _ := New()
		prog, reason, _, cerr := a.CompileCheck(src)
		if cerr != nil || prog == nil {
			t.Errorf("%q: did not compile: reason=%q", src, reason)
			continue
		}
		if strings.Contains(prog.Disassemble(), "FALLBACK") {
			t.Errorf("%q: expected a native query DSL lowering, got an island", src)
		}
		ar, _ := New()
		gotC, compiled, errC := ar.RunCompiled(src)
		if noteCompileDefect(t, src, gotC, errC) {
			continue
		}
		b, _ := New()
		gotI, _ := b.RunInterp(src)
		if !compiled || errC != nil || fmt.Sprint(gotC) != fmt.Sprint(gotI) {
			t.Errorf("%q: compiled=%v errC=%v gotC=%v gotI=%v (compiled/interp must match)",
				src, compiled, errC, gotC, gotI)
		}
	}

	// SCOPING: `inspect` (a core quoted-operand word that quotes a bare def
	// name) now DECLARES CompileQuoteInert, so it bakes a plain CALL_NATIVE over
	// the inert Atom const — the same opt-in quote/codequote/raise use. The
	// handler is a pure registry reader, so `def x 5  inspect x` compiles native
	// and runs compiled == interpreter (the binding lookup happens at VM time
	// against the same registry the interpreter sees). A core quoted-operand
	// word that does NOT opt in still declines — quoteOperandInertOK fires only
	// for a CompileQuoteInert declarer or a module-inner sig, so the exemption
	// does not silently leak.
	const core = `def x 5  inspect x`
	a, _ := New()
	prog, reason, _, _ := a.CompileCheck(core)
	if prog == nil {
		t.Errorf("%q: inspect should compile (CompileQuoteInert), declined %q", core, reason)
	} else if strings.Contains(prog.Disassemble(), "FALLBACK") {
		t.Errorf("%q: inspect should bake a CALL_NATIVE, got an island:\n%s", core, prog.Disassemble())
	}
	ar, _ := New()
	gotC, _, _ := ar.RunCompiled(core)
	b, _ := New()
	gotI, _ := b.RunInterp(core)
	if fmt.Sprint(gotC) != fmt.Sprint(gotI) {
		t.Errorf("%q: compiled/interp parity broke: compiled=%v interp=%v", core, gotC, gotI)
	}
}

// reach lenses — a RECEIVERLESS reach (`$.name`, `$.a.b`, `$!.x`, `$.1`) is an
// inert first-class lens value that evaluates to itself. It now bakes into the
// const pool (isInertReach: Eval=false, no Receiver, all-literal-key segments —
// the opposite of a dot-access Eval reach the engine expands in place), so the
// lens VALUE, `typeof`, `apply`, `rebind`, and the StructUtil getpath/setpath
// forms compile to a plain const-operand CALL_NATIVE. The lens keys carry no
// canonical-*Type hazard (Words/Atoms/scalars), so the by-value const copy is
// faithful — the differential confirms compiled == interpreted across the family.
func TestReachLensCompilesNative(t *testing.T) {
	for _, c := range []struct {
		src  string
		want string
	}{
		{`$.name`, "[$.name]"},
		{`$.a.b`, "[$.a.b]"},
		{`$!.x`, "[$!.x]"},
		{`typeof $.name`, "[Reach]"},
		{`def p {name:"ada" age:36}  p $.name apply`, "[ada]"},
		{`def p {a:{b:7}}  p $.a.b apply`, "[7]"},
		{`def xs [10 20 30]  xs $.1 apply`, "[20]"},
		{`def p {name:"ada"}  p $!.name apply`, "[ada]"},
		{`def p {name:"ada"}  typeof (rebind $.name p)`, "[Reach]"},
		{`import "boru:struct-util"  StructUtil.getpath $.a.b {a:{b:7}}`, "[7]"},
		{`import "boru:struct-util"  StructUtil.setpath $.a.b 99 {a:{b:1} c:2}`, "[{a:{b:99} c:2}]"},
	} {
		a, _ := New()
		prog, reason, _, cerr := a.CompileCheck(c.src)
		if cerr != nil || prog == nil {
			t.Errorf("%q: did not compile: reason=%q", c.src, reason)
			continue
		}
		if strings.Contains(prog.Disassemble(), "FALLBACK") {
			t.Errorf("%q: expected a native const-baked lens, got an island", c.src)
		}
		ar, _ := New()
		gotC, compiled, errC := ar.RunCompiled(c.src)
		if noteCompileDefect(t, c.src, gotC, errC) {
			continue
		}
		b, _ := New()
		gotI, _ := b.RunInterp(c.src)
		if !compiled || errC != nil || fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotC) != c.want {
			t.Errorf("%q: compiled=%v gotC=%v gotI=%v (want %s)", c.src, compiled, gotC, gotI, c.want)
		}
	}

	// The CONCRETE-output lens-over-DATA forms (`each $.name people`,
	// `ArrayUtil.sortby $.age people`) also compile native: the lens bakes as a
	// const (isInertReach) AND the string-bearing data list now bakes too (the
	// toCarrier scalar-keep stopped stripping its interior strings), so the
	// reach-form dispatch records a plain CALL_NATIVE.
	for _, c := range []struct {
		src  string
		want string
	}{
		{`def people [{name:"ada"}]  each $.name people`, "[['ada']]"},
		{`each $.name [{name:"a"} {name:"b"}]`, "[['a' 'b']]"},
	} {
		a, _ := New()
		prog, reason, _, cerr := a.CompileCheck(c.src)
		if cerr != nil || prog == nil || strings.Contains(prog.Disassemble(), "FALLBACK") {
			t.Errorf("%q: lens-over-data did not compile native: reason=%q", c.src, reason)
			continue
		}
		ar, _ := New()
		gotC, compiled, errC := ar.RunCompiled(c.src)
		if noteCompileDefect(t, c.src, gotC, errC) {
			continue
		}
		b, _ := New()
		gotI, _ := b.RunInterp(c.src)
		if !compiled || errC != nil || fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotC) != c.want {
			t.Errorf("%q: compiled=%v gotC=%v gotI=%v (want %s)", c.src, compiled, gotC, gotI, c.want)
		}
	}

	// `filter $.on {map}` lens form: filter now narrows its result to the
	// INPUT collection type (filterReturnsFn — a Map subset, not a dynamic
	// Any), so the reach-form dispatch records a faithful CALL_NATIVE instead
	// of declining on a dynamic output. Parity must hold.
	const filter = `filter $.on {a:{on:true} b:{on:false}}`
	a, _ := New()
	prog, reason, _, cerr := a.CompileCheck(filter)
	if cerr != nil || prog == nil || strings.Contains(prog.Disassemble(), "FALLBACK") {
		t.Errorf("%q: filter lens form did not compile native: reason=%q", filter, reason)
	}
	ar, _ := New()
	gotC, compiledFilter, _ := ar.RunCompiled(filter)
	if !compiledFilter {
		t.Errorf("%q: filter lens form should compile native now", filter)
	}
	b, _ := New()
	gotI, _ := b.RunInterp(filter)
	if fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotI) != "[{a:{on:true}}]" {
		t.Errorf("%q: filter parity broke: compiled=%v interp=%v", filter, gotC, gotI)
	}

	// The dot-access EVAL reach (`m.a.b`, a get-chain the engine expands) is
	// untouched by isInertReach (it is excluded: Eval=true, has a receiver) and
	// keeps compiling via its lowered get-chain.
	const dot = `def m {a:{b:7}}  m.a.b`
	c2, _ := New()
	p2, r2, _, _ := c2.CompileCheck(dot)
	if p2 == nil || strings.Contains(p2.Disassemble(), "FALLBACK") {
		t.Errorf("%q: dot-access reach must still compile native (reason %q)", dot, r2)
	}
}

// scalar carrier-keep + carrier-identity de-collision. Two coupled
// runtime-independence steps:
//
//   - toCarrier now keeps a concrete inert SCALAR (string/bool/float/atom/
//     temporal, not just integer) concrete through check mode, so a DATA list or
//     map whose interior is scalar bakes as an inert const instead of stripping
//     to a type-only carrier — `size people`, `each $.name people`, a
//     string-bearing table row all const-bake their operand.
//   - a call OUTPUT whose deterministic id collides with a PRIOR generic event
//     (a repeated identical computed call — `(context get 'n') add (context get
//     'n')`, both gets returning the same id once the key is concrete) mints a
//     fresh id, so the two stack values stay distinct and the residual layout no
//     longer declines "call results reordered". The skip that owns structured
//     hooks (if / user-fn / poly / closure) is gated on the producer being a
//     structured event (not a prior generic native), so it still fires for them.
func TestScalarKeepAndCarrierIdentity(t *testing.T) {
	for _, c := range []struct {
		src  string
		want string
	}{
		// scalar-keep: string-bearing data lists const-bake
		{`size ["a" "b" "c"]`, "[3]"},
		{`def people [{name:"ada"}]  size people`, "[1]"},
		{`def people [{name:"ada"} {name:"bob"}]  each $.name people`, "[['ada' 'bob']]"},
		// carrier-identity: repeated identical computed calls
		{`context set 'n' 5 end ( context get 'n' ) add ( context get 'n' )`, "[10]"},
		{`context set 'n' 5 end ( context get 'n' ) add ( context get 'n' ) add ( context get 'n' )`, "[15]"},
		// the de-collision must NOT disturb dup's intentional same-id outputs,
		// nor a computed receiver feeding dup
		{`5 dup add`, "[10]"},
		{`( 1 add 2 ) dup add`, "[6]"},
	} {
		a, _ := New()
		prog, reason, _, cerr := a.CompileCheck(c.src)
		if cerr != nil || prog == nil {
			t.Errorf("%q: did not compile: reason=%q", c.src, reason)
			continue
		}
		if strings.Contains(prog.Disassemble(), "FALLBACK") {
			t.Errorf("%q: expected a native lowering, got an island", c.src)
		}
		ar, _ := New()
		gotC, compiled, errC := ar.RunCompiled(c.src)
		if noteCompileDefect(t, c.src, gotC, errC) {
			continue
		}
		b, _ := New()
		gotI, _ := b.RunInterp(c.src)
		if !compiled || errC != nil || fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotC) != c.want {
			t.Errorf("%q: compiled=%v gotC=%v gotI=%v (want %s)", c.src, compiled, gotC, gotI, c.want)
		}
	}

	// Control: an `if` whose output id the structured RecordBranch hook
	// pre-registers must STILL be skipped by the generic path (the de-collision
	// gate only bypasses the skip for a prior GENERIC producer) — it compiles
	// natively and matches the interpreter.
	const cond = `if (5 gt 3) [10] [20]`
	a, _ := New()
	gotC, compiled, _ := a.RunCompiled(cond)
	b, _ := New()
	gotI, _ := b.RunInterp(cond)
	if !compiled || fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotC) != "[10]" {
		t.Errorf("%q: if regressed: compiled=%v gotC=%v gotI=%v", cond, compiled, gotC, gotI)
	}
}

// module-synthetic const-fold — a PURE read over an import-bound module value is
// a compile-time constant. `import` binds an immutable, deterministic namespace
// map + Module descriptor, so `MathUtil.$name`, `X.$module.name`, and
// `typeof`/`is`/`convert` over a Module always yield the same value. The
// checker's recorded RESULT is the declared TYPE (a Map/Boolean carrier), not
// the value, so tryFoldModuleConst RE-EVALUATES the dispatch concretely (check
// mode off, twice, must agree) and bakes the real value — `convert List
// Foo.$module` -> the export-name LIST, never the type literal (the bug this
// guards). Module values are kept concrete through toCarrier so the $module
// chain resolves. (A namespace itself is a plain Map — `convert` no longer
// applies to it; convert-ideal.tsv pins the compile failure.)
func TestModuleSyntheticConstFold(t *testing.T) {
	for _, c := range []struct {
		src  string
		want string
	}{
		{`import "boru:math-util"  MathUtil.$name`, "[MathUtil]"},
		{`import "boru:math-util"  typeof MathUtil.$module`, "[Module]"},
		{`import "boru:math-util"  MathUtil.$module is Module`, "[true]"},
		{`import "boru:math-util"  MathUtil.$module.name`, "[boru:math-util]"},
		{`import "boru:math-util"  MathUtil.$module.kind`, "[native]"},
		{`import "boru:math-util"  MathUtil.$module.exports`, "[['MathUtil']]"},
		// the VALUE, not the declared type List (the convert-folding bug guard)
		{`import module [export "Foo" {a:1}] convert List Foo.$module`, "[['Foo']]"},
	} {
		a, _ := New()
		prog, reason, _, cerr := a.CompileCheck(c.src)
		if cerr != nil || prog == nil {
			t.Errorf("%q: did not compile: reason=%q", c.src, reason)
			continue
		}
		if strings.Contains(prog.Disassemble(), "FALLBACK") {
			t.Errorf("%q: expected a native const-fold, got an island", c.src)
		}
		ar, _ := New()
		gotC, compiled, errC := ar.RunCompiled(c.src)
		if noteCompileDefect(t, c.src, gotC, errC) {
			continue
		}
		b, _ := New()
		gotI, _ := b.RunInterp(c.src)
		if !compiled || errC != nil || fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotC) != c.want {
			t.Errorf("%q: compiled=%v gotC=%v gotI=%v (want %s)", c.src, compiled, gotC, gotI, c.want)
		}
	}

	// CONTROL: `convert` over a NON-module ideal (a class instance) is not a
	// module-synthetic fold — it goes through the normal path and still produces
	// the value, proving the fold is scoped to module operands and does not
	// hijack every convert.
	const arr = `def C class {a:1 b:2} convert List (make C {})`
	a, _ := New()
	gotC, compiled, _ := a.RunCompiled(arr)
	b, _ := New()
	gotI, _ := b.RunInterp(arr)
	if !compiled || fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotC) != "[[1 2]]" {
		t.Errorf("%q: array convert control failed: compiled=%v gotC=%v gotI=%v", arr, compiled, gotC, gotI)
	}
}

// OpMakeList — a TOP-LEVEL computed list literal (`[1 add 2]`, `[(1 add 2)
// (3 add 4)]`) cannot bake as an inert const (its elements are event results),
// so autoEvalList records an OpMakeList assembly of the evaluated elements: the
// elements lower onto the stack, then the opcode pops N and pushes the list.
// Gated to the top frame (a fn-body / higher-order list is re-evaluated), to
// CORE-builtin element producers (a module/stateful word like `rand-int` must
// re-run, not freeze), and away from type-pattern (`[Integer]`) and make-
// instance lists (which the type machinery / schema-member const-bake own).
func TestOpMakeListCompiles(t *testing.T) {
	for _, c := range []struct {
		src  string
		want string
	}{
		{`[1 add 2]`, "[[3]]"},
		{`[1 add 2 mul 3]`, "[[9]]"},
		{`[10 sub 4 add 1]`, "[[7]]"},
		{`def x [1 add 2] x`, "[[3]]"},
		{`[(1 add 2) (3 add 4)]`, "[[3 7]]"},
		{`def a { b:5 } def c { d:6 } [ a.b c.d ]`, "[[5 6]]"},
	} {
		a, _ := New()
		prog, reason, _, cerr := a.CompileCheck(c.src)
		if cerr != nil || prog == nil {
			t.Errorf("%q: did not compile: reason=%q", c.src, reason)
			continue
		}
		dis := prog.Disassemble()
		if strings.Contains(dis, "FALLBACK") {
			t.Errorf("%q: expected a native MAKE_LIST, got an island", c.src)
		}
		if !strings.Contains(dis, "MAKE_LIST") {
			t.Errorf("%q: expected a MAKE_LIST opcode:\n%s", c.src, dis)
		}
		ar, _ := New()
		gotC, compiled, errC := ar.RunCompiled(c.src)
		if noteCompileDefect(t, c.src, gotC, errC) {
			continue
		}
		b, _ := New()
		gotI, _ := b.RunInterp(c.src)
		if !compiled || errC != nil || fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotC) != c.want {
			t.Errorf("%q: compiled=%v gotC=%v gotI=%v (want %s)", c.src, compiled, gotC, gotI, c.want)
		}
	}

	// A fully-LITERAL list stays a pooled const — no MAKE_LIST.
	c2, _ := New()
	p2, _, _, _ := c2.CompileCheck(`[1 2 3]`)
	if p2 == nil || strings.Contains(p2.Disassemble(), "MAKE_LIST") {
		t.Errorf("[1 2 3]: a literal list must bake as a const, not MAKE_LIST")
	}

	// A const-EVENT-const interleave (`[1 (2 add 3) 4]`) is an operand shape the
	// cheap stack paths can't seat (the event sits at a middle sig position). It
	// now compiles NATIVELY by spilling the event operand to a frame-local
	// destination (STORE_LOCAL) and re-pushing in sig order — no fallback — with
	// faithful parity.
	const inter = `[1 (2 add 3) 4]`
	a, _ := New()
	prog, _, _, _ := a.CompileCheck(inter)
	if prog == nil {
		t.Fatalf("%q: must compile via spill, but declined", inter)
	}
	dis := prog.Disassemble()
	if strings.Contains(dis, "FALLBACK") {
		t.Errorf("%q: must compile native (spill), not island:\n%s", inter, dis)
	}
	if !strings.Contains(dis, "STORE_LOCAL") {
		t.Errorf("%q: expected a spill (STORE_LOCAL):\n%s", inter, dis)
	}
	ar, _ := New()
	gotC, compiled, errC := ar.RunCompiled(inter)
	if noteCompileDefect(t, inter, gotC, errC) {
		return
	}
	b, _ := New()
	gotI, _ := b.RunInterp(inter)
	if !compiled || errC != nil || fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotI) != "[[1 5 4]]" {
		t.Errorf("%q: spill parity broke: compiled=%v gotC=%v interp=%v", inter, compiled, gotC, gotI)
	}
}

// Paren-bounded fn-value application — a method-field fn dispatch whose result
// is consumed across a paren boundary. `m.g` is a dynamic method-field get the
// checker cannot dispatch in place. GRADUATED (COMPILE FAILURE-CLOSURE §9.2e,
// 2026-07-17): the leading-dynamic paren apply records a guarded
// OpCallDynMethod that COLLAPSES the window to one carrier BEFORE any
// trailing op, so `(m.g 3) add 1` seats the apply RESULT (7) instead of the
// old paren-unaware OpCallDynamic reorder that lowered `m.g(3 add 1)=8`. The
// member-read provenance gate keeps a non-member leading dynamic declining.
func TestParenBoundedFnValueApplyFallsBack(t *testing.T) {
	const def = `def m {g: (fn [[x:Integer][Integer][x mul 2]])} `

	// GRADUATED: the paren-bounded member-read apply now compiles natively to
	// an OpCallDynMethod, with the correct result.
	for _, pos := range []struct{ src, want string }{
		{def + `((m.g 3) add 1)`, "[7]"},
		{def + `(m.g 3) add 1`, "[7]"}, // same shape without the outer paren
	} {
		a, _ := New()
		gotC, compiled, errC := a.RunCompiled(pos.src)
		if noteCompileDefect(t, pos.src, gotC, errC) {
			continue
		}
		if !compiled || errC != nil {
			t.Errorf("%q: §9.2e must compile native, got compiled=%v err=%v", pos.src, compiled, errC)
		}
		c, _ := New()
		gotI, _ := c.RunInterp(pos.src)
		if fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotI) != pos.want {
			t.Errorf("%q: parity: compiled=%v interp=%v want=%s", pos.src, gotC, gotI, pos.want)
		}
	}

	// POSITIVE: a simple method dispatch (arg directly follows, no paren
	// boundary) and a bare-word fn call still COMPILE correctly — the guard is
	// scoped to the dynamic-value-bounded-by-a-paren shape only.
	for _, pos := range []struct{ src, want string }{
		{def + `m.g 5`, "[10]"}, // simple stored dispatch
		{`def g (fn [[x:Integer][Integer][x mul 2]]) ((g 3) add 1)`, "[7]"}, // bare-word fn, concrete dispatch
	} {
		a, _ := New()
		gotC, compiled, errC := a.RunCompiled(pos.src)
		if noteCompileDefect(t, pos.src, gotC, errC) {
			continue
		}
		if !compiled || errC != nil {
			t.Errorf("%q: expected a native compile, got compiled=%v err=%v", pos.src, compiled, errC)
		}
		b, _ := New()
		gotI, _ := b.RunInterp(pos.src)
		if fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotI) != pos.want {
			t.Errorf("%q: parity: compiled=%v interp=%v want=%s", pos.src, gotC, gotI, pos.want)
		}
	}
}

// PR #141 automated-review findings — three latent bytecode-compiler bugs in
// the higher-order / const-fold paths. Each pairs the negative (the hazard
// declines / no longer diverges) with the positive (the legitimate shape still
// compiles), so the fix is pinned without over-declining.
func TestPRReviewFindings(t *testing.T) {
	// #4 / #1 — a lambda callback whose declared param TYPE cannot accept the
	// higher-order word's callback shape: the interpreter raises a callback
	// error, but the compiled closure used to bind the value and keep it. The
	// closure lowering now matches the param type (and declines overloaded fn
	// values) so these fall back faithfully; Any / KeyVal callbacks still compile.
	neg := []string{
		`filter ([p:String] => [true]) [1 2]`,     // String param vs {key,value} pair (list)
		`filter ([p:String] => [true]) {a:1 b:2}`, // String param vs KeyVal (map)
		`filter ([p:Integer] => [true]) {a:1 b:2}`,
		`filter ([kv:KeyVal] => [true]) [1 2]`, // KeyVal param vs a list's plain pair
	}
	for _, src := range neg {
		a, _ := New()
		gotC, _, errC := a.RunCompiled(src)
		if noteCompileDefect(t, src, gotC, errC) {
			continue
		}
		b, _ := New()
		gotI, errI := b.RunInterp(src)
		if fmt.Sprint(gotC) != fmt.Sprint(gotI) || (errC == nil) != (errI == nil) {
			t.Errorf("%q: callback type mismatch diverged: compiled=%v(e %v) interp=%v(e %v)", src, gotC, errC, gotI, errI)
		}
	}
	pos := []struct{ src, want string }{
		{`filter ([p:Any] => [(p.value mod 2) eq 0]) [1 2 3 4]`, "[[2 4]]"},
		{`filter ([kv:KeyVal] => [(kv.v mod 2) eq 0]) {a:1 b:2 c:3 d:4}`, "[{b:2 d:4}]"},
		{`0 fold ([acc:Integer kv:KeyVal] => [acc add kv.v]) {a:1 b:2 c:3}`, "[6]"},
	}
	for _, p := range pos {
		a, _ := New()
		gotC, compiled, errC := a.RunCompiled(p.src)
		if noteCompileDefect(t, p.src, gotC, errC) {
			continue
		}
		if !compiled || errC != nil {
			t.Errorf("%q: expected a native compile, got compiled=%v err=%v", p.src, compiled, errC)
		}
		if fmt.Sprint(gotC) != p.want {
			t.Errorf("%q: compiled=%v want=%s", p.src, gotC, p.want)
		}
	}

	// #2 — an effectful expression inside a container value was const-folded by
	// re-running it concretely (twice), performing/doubling the side effect at
	// compile time. The fold now declines an impure expression; a PURE container
	// value still folds and compiles.
	effOut := func(run func(*Boru, string)) string {
		old := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w
		a, _ := New()
		run(a, `{a:(print "x" 1) b:2}`)
		w.Close()
		os.Stdout = old
		out, _ := io.ReadAll(r)
		return string(out)
	}
	ci := effOut(func(a *Boru, s string) { a.RunInterp(s) })
	cc := effOut(func(a *Boru, s string) { a.RunCompiled(s) })
	if ci != cc || ci != "x\n" {
		t.Errorf("#2 effect parity: interp print=%q compiled print=%q (want %q once)", ci, cc, "x\n")
	}
	a, _ := New()
	if gotC, compiled, _ := a.RunCompiled(`{a:(3 mul 2) b:1}`); !compiled || fmt.Sprint(gotC) != "[{a:6 b:1}]" {
		t.Errorf("#2 pure fold regressed: compiled=%v wasCompiled=%v", gotC, compiled)
	}
}

// TestHeterogeneousArityBinaryOpCompiles guards the resolveForwardArgs
// function-word-barrier fix. A binary op whose signature set ALSO includes a
// higher-arity overload (here a 3-arg `add3`, mimicking patrun's
// `add {pattern} value pm`) must still compile a chained call over a repeated
// computed operand. The 3-arg overload never matches the integer chain, but
// its mere presence raised the word's max forward arity to 3; the former
// pre-evaluation scan then evaluated the THIRD group across the intermediate
// `add3` barrier, so the recorded operands laid out non-adjacently and the
// program declined with "operands of add3 not adjacent on top".
func TestHeterogeneousArityBinaryOpCompiles(t *testing.T) {
	const src = `context set 'n' 5 end ( context get 'n' ) add3 ( context get 'n' ) add3 ( context get 'n' )`

	mk := func() *Boru {
		a, err := New()
		if err != nil {
			t.Fatal(err)
		}
		a.Register("add3",
			Signature{
				Args: []*Type{TNumber, TNumber},
				Impl: core.Go(func(args []Value, _ map[string]Value, _ []Value, _ *core.Registry) ([]Value, error) {
					x, _ := core.AsInteger(args[0])
					y, _ := core.AsInteger(args[1])
					return []Value{NewInteger(x + y)}, nil
				}),
				Returns: []*Type{TInteger}, BarrierPos: -1,
			},
			// The higher-arity overload — never matched by the integer chain,
			// present only to make add3's arity profile heterogeneous (max
			// forward arity 3).
			Signature{
				Args: []*Type{TMap, TAny, TAny},
				Impl: core.Go(func(args []Value, _ map[string]Value, _ []Value, _ *core.Registry) ([]Value, error) {
					return []Value{args[0]}, nil
				}),
				Returns: []*Type{TMap}, BarrierPos: -1,
			},
		)
		return a
	}

	prog, reason, _, cerr := mk().CompileCheck(src)
	if cerr != nil || prog == nil {
		t.Fatalf("heterogeneous-arity chain did not compile: reason=%q err=%v", reason, cerr)
	}
	if strings.Contains(prog.Disassemble(), "FALLBACK") {
		t.Fatalf("heterogeneous-arity chain did not compile:\n%s", prog.Disassemble())
	}
	gotC, compiled, errC := mk().RunCompiled(src)
	gotI, _ := mk().RunInterp(src)
	if noteCompileDefect(t, src, gotC, errC) {
		return
	}
	if !compiled || errC != nil || fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotC) != "[15]" {
		t.Fatalf("parity broke: compiled=%v gotC=%v gotI=%v (want [15])", compiled, gotC, gotI)
	}
}

// Step-budget agreement (the RunCompiled LIMITATION note). The interpreter
// meters DefaultStepLimit per tape token; the VM meters it per bytecode
// instruction. The contract is one-directional: a long TERMINATING program both
// engines can finish must produce IDENTICAL results, and the VM must NEVER trip
// evaluation_limit on a program the interpreter completes (the VM stream is
// leaner, so it reaches at least as far). A long counted loop with a tight
// arithmetic body is the canonical long-but-bounded compute: it stays well under
// the cap in both, so it pins the agreement. (A genuine runaway is out of scope
// here — that one trips fast in both by design.)
func TestStepBudgetNoSpuriousLimit(t *testing.T) {
	// A counted loop accumulating a per-iteration arithmetic value. Large enough
	// to span many thousands of dispatch steps, small enough that BOTH engines
	// finish far under DefaultStepLimit (10M).
	const src = `for 6000 [(i mul 2)]`

	ci, _ := New()
	gotC, compiled, errC := ci.RunCompiled(src)
	if noteCompileDefect(t, src, gotC, errC) {
		return
	}
	if !compiled {
		t.Skip("the counted-loop shape no longer compiles; nothing to compare")
	}
	ii, _ := New()
	gotI, errI := ii.RunInterp(src)

	// Neither engine may spuriously raise on a program the other completes.
	if (errC != nil) != (errI != nil) {
		t.Fatalf("step-budget divergence on a terminating loop: compiled err=%v interpreted err=%v", errC, errI)
	}
	if errC != nil {
		t.Fatalf("both engines errored on a bounded loop that should complete: %v", errC)
	}
	if fmt.Sprint(gotC) != fmt.Sprint(gotI) {
		t.Fatalf("compiled/interpreted results differ on a long loop:\n  compiled=%v\n  interpreted=%v", gotC, gotI)
	}
}

// Fn-values-on-the-stack: a fn that RETURNS an anonymous capture-free fn
// (the factory pattern) CONST-BAKES the real FnDefInfo inside its unit
// (re-diagnosed 2026-07-20, PR #295 merge: a captureless returned fn keeps
// the const bake so a native that validates its fn operand's signatures —
// parselang-fn-dispatch's [source opts] contract — reads them, where an
// OpPushClosure ClosurePayload was rejected as "not a usable function
// value"; only a CAPTURING fn takes the closure unit), and a
// [Function]-typed CARRIER leading the residual is applied to its trailing
// args by a stack OpCallDynamic. `(mk2 5) 10` -> 11.
func TestFactoryApplyCompiles(t *testing.T) {
	// Legacy compile failure+fallback-parity contract: pins the one-release
	const factory = `def mk2 fn [[x:Integer] [Function] [([x:Integer] => [x add 1])]] `

	// WAS a positive: `(mk2 5) 10` compiled natively to 11, and this test
	// asserted 11 "on both lanes" — reading gotI from `Run`, which post
	// Stage J IS the compiled lane (NUR106). The interpreter has never
	// answered 11 for this program: `(mk2 5)` is a paren with ONE survivor,
	// so it PLACES, and the 10 lands beside it (NUR101,
	// design/PAREN-RESTEP-RULE.0.md). The native compile was a MISCOMPILE
	// that its own parity assertion could not see.
	//
	// Now a compile failure. Unlike the `((mk2 5) 10)` family this one does not
	// graduate with Stage 3 — there is nothing to apply here; if it ever
	// compiles again it must compile to the PLACED pair.
	// GRADUATED 2026-08-27 (Stage 3): compiles natively to the PLACED pair,
	// which is what the interpreter has always answered. The `11` this test
	// used to assert was the miscompile.
	mustCompileWithParity(t, factory+`(mk2 5) 10`, "[fn (Integer) 10]")
	// …and the bare closure result, out of the negative list below with it.
	mustCompileWithParity(t, factory+`(mk2 5)`, "[fn (Integer)]")

	// A CAPTURING factory (`[y] => [x add y]` closes over the factory's param x)
	// now ALSO compiles natively — tryReturnedClosure threads the resolved capture
	// before OpPushClosure. Full positive coverage (top-level + per-iteration apply)
	// lives in TestReturnedCapturingClosureApply.

	// NEGATIVE — the boundaries that must keep falling back (faithfully):
	for _, neg := range []struct{ name, src string }{
		// `(mk2 5)` — a bare closure RESULT — LEFT this negative list
		// 2026-08-27 (Stage 3). Its premise was that "a VM closure prints
		// differently from the interpreter's FnDefInfo"; measured, it prints
		// `[fn (Integer)]` on both lanes, and this loop's own parity
		// assertion never fired for it. It is a parity row below.
		// A CAPTURING returned fn with MULTIPLE own sigs declines the
		// closure model (FirstOwnSig is not the runtime MatchFnSig pick).
		{"capturing multi-sig returned fn",
			`def mk fn [[x:Integer][Function][(fn [[a:Integer][Integer][a add x] [b:String][String][b]])]] ((mk 1) 2)`},
		// A CAPTURING returned fn with a value-PATTERN param (no bare type —
		// fnValueInputs' Any default) reaches the probe and declines there.
		{"capturing pattern-param returned fn",
			`def mk fn [[x:Integer][Function][(fn [[0][Integer][x]])]] ((mk 5) 0)`},
	} {
		gC, _, eC := mustNew(t).RunCompiled(neg.src)
		gI, eI := mustNew(t).RunInterp(neg.src)
		if noteCompileDefect(t, neg.src, gC, eC) {
			continue
		}
		if (eC != nil) != (eI != nil) || fmt.Sprint(gC) != fmt.Sprint(gI) {
			t.Errorf("%s: fallback diverged: c=%v(%v) i=%v(%v)", neg.name, gC, eC, gI, eI)
		}
	}
}

func mustNew(t *testing.T) *Boru {
	t.Helper()
	a, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return a
}

// Value-def-locals + class mutable-default bake: a `def name (expr)` binding is
// promoted to a frame LOCAL so it re-pushes in any order (not just stack order),
// which lets `def a (make…) def b (make…) a.x … b.x` — a used before b though b
// is on top — compile; and a class body with a mutable default (flex/Array/…)
// bakes as a const TEMPLATE that make freshens per instance. The critical
// property is PER-INSTANCE ISOLATION: a mutation to one instance must NOT leak
// to another (the negative), which holds because make's FreshenDefault runs
// identically in both engines.
func TestValueDefLocalsClassIsolation(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		// flex default + push to a only: b stays empty (isolation).
		{"flex isolation", `def Foo class {items:(flex [])} def a (make Foo {}) def b (make Foo {}) (a.items push 1) b.items`, "[[1] []]"},
		// flex-list default + set on a only: b unchanged (a shows the write).
		{"flex set isolation", `def Bits class {bits:(flex [0 0 0])} def a (make Bits {}) def b (make Bits {}) def _ (set 0 9 a.bits) end b.bits`, "[[0 0 0]]"},
		// nested instance default + set on a.i only: b.i.n unchanged.
		{"nested isolation", `def Inner class {n:0} def Outer class {i:(make Inner {})} def a (make Outer {}) def b (make Outer {}) set n 9 a.i end b.i.n`, "[0]"},
		// two value-defs read OUT of production order (a before b, b on top).
		{"out-of-order defs", `def a (flex [1 2 3]) def b (flex [4 5 6]) a.0 add b.0`, "[5]"},
	} {
		gotC, compiled, errC := mustNew(t).RunCompiled(c.src)
		gotI, errI := mustNew(t).RunInterp(c.src)
		if noteCompileDefect(t, c.src, gotC, errC) {
			continue
		}
		if !compiled {
			t.Errorf("%s: expected compiled, fell back: %s", c.name, c.src)
		}
		if (errC != nil) != (errI != nil) || fmt.Sprint(gotC) != fmt.Sprint(gotI) {
			t.Errorf("%s: divergence c=%v(%v) i=%v(%v)", c.name, gotC, errC, gotI, errI)
		}
		// The want values ARE the negative: a 1 leaking into b's column, or b's
		// field changing, would mean the bake shared a mutable default or the
		// value-def promotion crossed instances. `[[1] []]` (not `[[1] [1]]`) is
		// the per-instance-isolation contract.
		if fmt.Sprint(gotC) != c.want {
			t.Errorf("%s: got %v, want %s (isolation broken?)", c.name, gotC, c.want)
		}
	}

	// The standalone "a mutable instance must NOT bake as a const" negative is
	// pinned in eng/go/bytecode_constbake_test.go; here the isolation wants above
	// are the behavioural negative.
}

// Branch-fragment value-def locals: a value computed in the ENCLOSING scope and
// read INSIDE a branch arm / loop body / clause guard is promoted to a frame
// local (STORE_LOCAL once, PUSH_LOCAL per cross-floor read) instead of declining
// on the closed-fragment scopeFloor rule. This is the enabler for compiling a
// computed-scrutinee `case (expr) […]` (whose desugar re-tests the scrutinee in
// every clause fragment) and any `def x (expr) … if/for … x …`.
func TestEnclosingReadInBranchCompiles(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		// computed scrutinee re-tested in each clause guard/block fragment.
		{"computed-case", `case (1 add 1) [2 "two" "other"]`, "[two]"},
		{"computed-case-def", `def x 5 case (x add 1) [6 "six" "no"]`, "[six]"},
		// enclosing computed value read in an if condition AND then-arm.
		{"if-enclosing", `def y (1 add 2) if (y gt 0) [y mul 2] [0]`, "[6]"},
		// enclosing instance read across a branch.
		{"if-instance", `def a (flex [1 2 3]) if (a.0 gt 0) [a.1] [99]`, "[2]"},
		// The SAME enclosing-read pattern INSIDE A FN BODY now compiles too:
		// planValueDefLocals runs for every unit, not just frame 0, so the fn
		// unit's `def y (n add 1)` promotes to a frame slot read across the arm.
		{"fn-body-enclosing", `def f fn [[n:Integer] [Integer] [def y (n add 1) if (n gt 0) [y mul 2] [0]]] f 5`, "[12]"},
		{"fn-body-enclosing-else", `def f fn [[n:Integer] [Integer] [def y (n add 1) if (n gt 0) [y mul 2] [0]]] f 0`, "[0]"},
	} {
		gotC, compiled, errC := mustNew(t).RunCompiled(c.src)
		gotI, errI := mustNew(t).RunInterp(c.src)
		if noteCompileDefect(t, c.src, gotC, errC) {
			continue
		}
		if !compiled {
			t.Errorf("%s: expected compiled, fell back: %s", c.name, c.src)
		}
		if (errC != nil) != (errI != nil) || fmt.Sprint(gotC) != fmt.Sprint(gotI) {
			t.Errorf("%s: divergence c=%v(%v) i=%v(%v)", c.name, gotC, errC, gotI, errI)
		}
		if fmt.Sprint(gotC) != c.want {
			t.Errorf("%s: got %v, want %s", c.name, gotC, c.want)
		}
	}
}

// Island burn-down (islandCeiling 7 -> 2). Two shapes that used to embed an
// OpFallback island now compile natively:
//
//   - a map each/scan with a 0-NET body (`{a:1} each [drop]`): the body is the
//     handler's OWN each_error/scan_error ("body produced no result"), raised
//     from the InvokeBody loop, so EmptyBodyErrors compiles the body as a
//     count-agnostic closure and the compiled handler raises the byte-identical
//     error instead of islanding.
//   - `case` with a non-list clause argument (`case 1 Integer`): a static
//     case_error the checker is lenient about — compiled as a terminal OpTrap
//     that raises the byte-identical error rather than islanding.
//
// Each must (a) compile without a FALLBACK island and (b) raise the same error
// taxonomy + detail as the interpreter.
func TestIslandBurndownEmptyBodyAndCaseTrap(t *testing.T) {
	cases := []struct {
		name, src, code, mustContain string
	}{
		{"each-drop-map", `{a:1} each [drop]`, "each_error", "each"},
		{"scan-drop-map", `{a:1 b:2} scan [drop drop]`, "scan_error", "scan"},
		{"case-nonlist-clauses", `case 1 Integer`, "case_error", "TRAP"},
	}
	for _, c := range cases {
		prog, reason, _, cerr := mustNew(t).CompileCheck(c.src)
		if cerr != nil || prog == nil {
			t.Errorf("%s: did not compile: reason=%q err=%v", c.name, reason, cerr)
			continue
		}
		dis := prog.Disassemble()
		if strings.Contains(dis, "FALLBACK") {
			t.Errorf("%s: compiled with an island, want a native bake:\n%s", c.name, dis)
		}
		if !strings.Contains(dis, c.mustContain) {
			t.Errorf("%s: disassembly missing %q:\n%s", c.name, c.mustContain, dis)
		}
		// Error parity: same taxonomy code AND same detail as the interpreter.
		_, compiled, errC := mustNew(t).RunCompiled(c.src)
		if noteCompileDefect(t, c.src, nil, errC) {
			continue
		}
		if !compiled {
			t.Errorf("%s: fell back at run time", c.name)
		}
		_, errI := mustNew(t).RunInterp(c.src)
		var aeC, aeI *core.BoruError
		if !errors.As(errC, &aeC) || !errors.As(errI, &aeI) {
			t.Errorf("%s: expected BoruError from both engines, got c=%v i=%v", c.name, errC, errI)
			continue
		}
		if aeC.Code != c.code || aeI.Code != c.code {
			t.Errorf("%s: code mismatch compiled=%q interp=%q want %q", c.name, aeC.Code, aeI.Code, c.code)
		}
		if aeC.Detail != aeI.Detail {
			t.Errorf("%s: detail divergence\n compiled=%q\n interp=%q", c.name, aeC.Detail, aeI.Detail)
		}
	}
}

// Completion item — codequote (code-as-data) compiles. `codequote (expr)`
// captures the paren RAW as a Quoted ParenExpr (`codequote (1 add 2)` →
// `paren([1 word(add) 2])`). A Quoted ParenExpr is immutable data: the
// interpreter's stepLiteral leaves it unevaluated and the VM never re-steps a
// const, so it bakes as an inert PUSH_CONST exactly like the macroexpand token
// list. The negative half: an UNQUOTED ParenExpr is expanded and re-stepped in
// place, so isInertConst must never bake one (gated on v.Quoted).
func TestCodequoteCompilesNative(t *testing.T) {
	for _, c := range []string{
		`codequote (1 add 2)`,
		`codequote (nosuchword)`,
		`codequote (a.b.c)`,
		`codequote (if x [1] [2])`,
	} {
		prog, reason, _, cerr := mustNew(t).CompileCheck(c)
		if cerr != nil || prog == nil {
			t.Fatalf("%q did not compile: reason=%q err=%v", c, reason, cerr)
		}
		dis := prog.Disassemble()
		if strings.Contains(dis, "FALLBACK") || !strings.Contains(dis, "PUSH_CONST") {
			t.Errorf("%q: want a native PUSH_CONST bake (no island):\n%s", c, dis)
		}
		// Byte-identical to the interpreter.
		gotC, compiled, errC := mustNew(t).RunCompiled(c)
		if noteCompileDefect(t, c, gotC, errC) {
			continue
		}
		if !compiled || errC != nil {
			t.Fatalf("%q: compiled=%v err=%v", c, compiled, errC)
		}
		gotI, _ := mustNew(t).RunInterp(c)
		if fmt.Sprint(gotC) != fmt.Sprint(gotI) {
			t.Errorf("%q: compiled=%v interp=%v", c, gotC, gotI)
		}
	}

	// Negative: a Quoted ParenExpr is data, but the standalone isInertConst gate
	// must still reject a non-codequote'd value that LOOKS paren-shaped. A normal
	// paren `(1 add 2)` evaluates to 3 (no ParenExpr survives), so it compiles as
	// arithmetic, never as a baked ParenExpr const — assert the result is the
	// evaluated value, not the code.
	gotC, compiled, errC := mustNew(t).RunCompiled(`(1 add 2)`)
	if noteCompileDefect(t, `(1 add 2)`, gotC, errC) {
		return
	}
	if !compiled || errC != nil {
		t.Fatalf("plain paren: compiled=%v err=%v", compiled, errC)
	}
	if fmt.Sprint(gotC) != "[3]" {
		t.Errorf("plain paren must evaluate, not bake as code: got %v", gotC)
	}
}

// Found via the voxgig-boru/decision project's diverge.sh (--force-compile over
// suites that use `each [var [[v] … 0]]`). `var` is a block-with-locals (let)
// word: its handler SPLICES def/body/undef tokens onto the tape for the engine
// to re-step. RunInCheckMode lets the recorder FOLLOW that splice, so the inline
// let lowers as its body's events with the bound names as promoted value-def
// locals — exactly as a hand-written `def NAME val end … undef NAME` compiles.
// The body's words (including a fn param/loop-var/capture referenced inside it)
// resolve to their VM frame slots because they record into the SAME open unit,
// so the historical frame-local divergence cannot arise.
func TestVarCompilesAsLet(t *testing.T) {
	// Top-level `var` compiles as a let, byte-identical to the interpreter.
	for _, c := range []struct {
		src  string
		want string
	}{
		{`5 var [[v] v add 1]`, "[6]"},
		{`def r (5 var [[v] v 0]) r`, "[0 5]"},
	} {
		prog, _, _, cerr := mustNew(t).CompileCheck(c.src)
		if cerr != nil {
			t.Fatalf("%q: unexpected check error %v", c.src, cerr)
		}
		if prog == nil {
			t.Errorf("%q: expected a compiled Program (var compiles as a let)", c.src)
		}
		got, compiled, err := mustNew(t).RunCompiled(c.src)
		if noteCompileDefect(t, c.src, got, err) {
			continue
		}
		if err != nil {
			t.Fatalf("%q: RunCompiled error %v", c.src, err)
		}
		if !compiled {
			t.Errorf("%q: expected a compiled run, got a compile failure", c.src)
		}
		if fmt.Sprint(got) != c.want {
			t.Errorf("%q: got %v want %s", c.src, got, c.want)
		}
	}

	// A `var` block inside an `each` body compiles as the closure's let, including
	// the CAPTURING case (the body names a fn param `a`): the param records to its
	// closure capture slot, so the compiled run matches the interpreter — the very
	// frame-local divergence the const-bake path could not handle.
	for _, c := range []struct {
		src  string
		want string
	}{
		{`def xs [1 2 3] (xs each [var [[v] v mul 2]])`, "[[2 4 6]]"},
		{`def xs [1 2 3] (xs each [var [[v] v 0]])`, "[[0 0 0]]"},
		{`def f0 fn [[a:Integer] [Integer] [(size ([0] each [var [[v] a 2]]))]] (f0 2)`, "[1]"},
	} {
		got, compiled, err := mustNew(t).RunCompiled(c.src)
		if noteCompileDefect(t, c.src, got, err) {
			continue
		}
		if err != nil {
			t.Fatalf("%q: %v", c.src, err)
		}
		gotI, _ := mustNew(t).RunInterp(c.src)
		if !compiled {
			t.Errorf("%q: an each-var let body should compile", c.src)
		}
		if fmt.Sprint(got) != fmt.Sprint(gotI) || fmt.Sprint(got) != c.want {
			t.Errorf("%q: compiled=%v interp=%v want %s", c.src, got, gotI, c.want)
		}
	}

	// A var-body that REACHES INTO the each element (`s get a/q`, where `s` is the
	// element carrier) compiles to its closure unit and is byte-identical to the
	// interpreter ([1 2]). This row previously DECLINED, but the compile failure was the
	// var-cleanup `undef s` mis-dispatching to the 2-arg `undef name fnUndefSpec`
	// form (the dynamic-Any body residual gradually matched TFnUndef in check
	// mode) and erroring — NOT, as once believed, a `get`-rejects-under-typed-map
	// imprecision. With the cleanup routed through the 1-arg-only `__varundef`
	// (native_definition.go) the body analyses cleanly and compiles. The broader
	// armed-body-error soundness gate in AnalyseFnBody (a GENUINE check-mode body
	// error still marks the program uncompilable) is exercised by the whole-corpus
	// differential.
	const reach = `def data [{a:1} {a:2}] (data each [var [[s] (s dot a/q)]])`
	prog, _, _, cerr := mustNew(t).CompileCheck(reach)
	if cerr != nil {
		t.Fatalf("reach each-var: unexpected check error %v", cerr)
	}
	if prog == nil {
		t.Errorf("reach each-var: expected a compiled Program (the var cleanup no longer mis-dispatches)")
	}
	gotG, compiled, err := mustNew(t).RunCompiled(reach)
	if noteCompileDefect(t, reach, gotG, err) {
		return
	}
	if err != nil {
		t.Fatalf("reach each-var: %v", err)
	}
	gotGI, _ := mustNew(t).RunInterp(reach)
	if !compiled {
		t.Errorf("reach each-var: a clean var-reach body should compile, got a fallback")
	}
	if fmt.Sprint(gotG) != fmt.Sprint(gotGI) || fmt.Sprint(gotG) != "[[1 2]]" {
		t.Errorf("reach each-var: compiled=%v interp=%v want [[1 2]]", gotG, gotGI)
	}
}

// A top-taking higher-order word (each/fold/scan/filter) reads only res[len-1]
// of its body residual, so a body that leaves values BELOW its result — most
// commonly an `each` body that IGNORES its element and computes a result over a
// trailing throwaway (`each [add 1 0]` → the element sits under [3, 0]) — used to
// decline "result above a literal" at the closure RET. trimToTopResult drops the
// unobserved below-top operands (CallableSpec.BodyResultTop) so the body compiles
// as a real closure, byte-identical to the interpreter. The negative half: `do`
// reads the WHOLE residual (no BodyResultTop), so its body is NOT trimmed.
func TestTopTakingClosureTrim(t *testing.T) {
	for _, c := range []struct {
		src         string
		mustCompile bool
	}{
		{`([1 2 3] each [add 1 0])`, true},        // element ignored; result computed below 0
		{`([1 2 3] each [(size [9 9]) 0])`, true}, // computed event below the throwaway
		{`([1 2 3] each [99 0])`, true},           // pure-data residual, top kept
		{`([1 2 3] each [mul 2])`, true},          // single-value body unaffected
		{`(fold [add 1 0] [1 2 3] 0)`, true},      // fold takes top too
		{`(do [10 20 30])`, true},                 // do takes ALL — not trimmed, still compiles
	} {
		prog, reason, _, cerr := mustNew(t).CompileCheck(c.src)
		if cerr != nil {
			t.Fatalf("%q: check error %v", c.src, cerr)
		}
		if c.mustCompile && prog == nil {
			t.Errorf("%q: expected to compile, declined: %s", c.src, reason)
		}
		// Byte-identical to the interpreter either way.
		gotC, _, errC := mustNew(t).RunCompiled(c.src)
		gotI, errI := mustNew(t).RunInterp(c.src)
		if noteCompileDefect(t, c.src, gotC, errC) {
			continue
		}
		if (errC == nil) != (errI == nil) || fmt.Sprint(gotC) != fmt.Sprint(gotI) {
			t.Errorf("%q: compiled=%v (%v) interp=%v (%v)", c.src, gotC, errC, gotI, errI)
		}
	}

	// `do [10 20 30]` must return ALL three values — proof the trim is scoped to
	// top-taking words and does not corrupt a whole-residual handler.
	got, _, gotErr := mustNew(t).RunCompiled(`do [10 20 30]`)
	if noteCompileDefect(t, `do [10 20 30]`, got, gotErr) {
		return
	}
	if fmt.Sprint(got) != "[10 20 30]" {
		t.Errorf("do must keep the whole residual, got %v", got)
	}
}

// A `${expr}` interpolation whose value is only known at RUNTIME (a carrier — a
// fn param, an each-element field read) must not be const-folded: ValToString
// of a carrier renders its type tag ("dynamic(Any)"), which the interpreter
// never produces, so baking it diverges. evalInterpString now returns a String
// CARRIER and declines recording for such a string, so the program falls back to
// the interpreter and builds the real value. Found via voxgig-boru/decision
// prop suites (`  pass: ${nm}` where nm = a get over the each-element carrier).
func TestInterpStringRuntimePartCompiles(t *testing.T) {
	// Legacy compile failure+fallback-parity contract: pins the one-release
	// nm is a field read over the each-element carrier → a runtime hole. It now
	// lowers to OpInterp (the VM rebuilds the string from the popped hole at run
	// time) rather than declining the program. Compiled == interp, no carrier leak,
	// and the interpolation itself is native — no string-interp island.
	const src = `def rs [{name:"a"} {name:"b"}]
(rs each [var [[r] def nm (r "name" get) ` + "`x ${nm}`" + ` ]])`
	prog, reason, _, _ := mustNew(t).CompileCheck(src)
	if prog == nil {
		t.Fatalf("a runtime-valued interpolation must now compile via OpInterp, declined: %s", reason)
	}
	if dis := prog.Disassemble(); !strings.Contains(dis, "INTERP") || strings.Contains(dis, "FALLBACK") {
		t.Errorf("the interpolation must lower to a native INTERP (no island):\n%s", dis)
	}
	gotC, compiled, errC := mustNew(t).RunCompiled(src)
	gotI, errI := mustNew(t).RunInterp(src)
	if noteCompileDefect(t, src, gotC, errC) {
		return
	}
	if !compiled {
		t.Errorf("the program must run compiled, fell back")
	}
	if (errC == nil) != (errI == nil) || fmt.Sprint(gotC) != fmt.Sprint(gotI) {
		t.Errorf("parity broke: compiled=%v (%v) interp=%v (%v)", gotC, errC, gotI, errI)
	}
	if fmt.Sprint(gotI) != "[['x a' 'x b']]" {
		t.Errorf("got %v, want [['x a' 'x b']]", gotI)
	}
	for _, v := range []string{fmt.Sprint(gotC), fmt.Sprint(gotI)} {
		if strings.Contains(v, "dynamic(") {
			t.Errorf("interp string leaked a carrier render: %q", v)
		}
	}

	// POSITIVE: a fully-CONCRETE interpolation still folds to a pooled const.
	prog, reason, _, cerr := mustNew(t).CompileCheck("def n 5\n`value ${n}`")
	if cerr != nil {
		t.Fatalf("concrete interp: check error %v", cerr)
	}
	if prog == nil {
		t.Errorf("a concrete interpolation should still compile, declined: %s", reason)
	}
	gotc, _, gotcErr := mustNew(t).RunCompiled("def n 5\n`value ${n}`")
	if noteCompileDefect(t, "def n 5\n`value ${n}`", gotc, gotcErr) {
		return
	}
	if fmt.Sprint(gotc) != "[value 5]" {
		t.Errorf("concrete interp: got %v want [value 5]", gotc)
	}

	// NEGATIVE: a single hole that yields a DYNAMIC, MULTI-value run cannot map to
	// one stack slot, so OpInterp declines and the program falls back — still with
	// interpreter parity (no silent miscompile).
	const multi = "`v=${1 add 2  3 add 4}`"
	mprog, _, _, _ := mustNew(t).CompileCheck(multi)
	if mprog != nil {
		t.Errorf("a dynamic multi-value hole must fail to compile")
	}
	mC, _, mCErr := mustNew(t).RunCompiled(multi)
	mI, _ := mustNew(t).RunInterp(multi)
	if noteCompileDefect(t, multi, mC, mCErr) {
		return
	}
	if fmt.Sprint(mC) != fmt.Sprint(mI) {
		t.Errorf("fallback parity broke: compiled=%v interp=%v", mC, mI)
	}
}

// TestInterpStringOpInterpParity exercises OpInterp across the shapes that
// distinguish it from a const fold — multiple holes, natively-computed holes,
// None and type-literal holes, and NESTING — asserting (a) the program compiles
// natively (an INTERP, no island), and (b) compiled output is byte-identical to
// the interpreter.
func TestInterpStringOpInterpParity(t *testing.T) {
	// wantInterp marks cases with a genuinely DYNAMIC hole (a native result, which
	// check mode synthesises as a carrier) that must lower to OpInterp. The others
	// have only literal / binding / const-foldable holes, so they collapse to a
	// pooled const (optimal) — they must still compile natively and match, but
	// without an INTERP op.
	cases := []struct {
		src, want  string
		wantInterp bool
	}{
		{src: "`a${1}b${2}c${3}d`", want: "[a1b2c3d]"},                                                  // all literal holes → const
		{src: "def n 3 `${n} items, total ${n mul 10}`", want: "[3 items, total 30]", wantInterp: true}, // n mul 10 carrier
		{src: "`type: ${42 typeof}`", want: "[type: Integer]", wantInterp: true},                        // typeof carrier
		{src: "def x None `got ${x}`", want: "[got None]"},                                              // None binding → const
		{src: "`nested ${ `inner ${1 add 1}` }`", want: "[nested inner 2]", wantInterp: true},           // add carrier
		{src: "def f fn [[n:Integer] [String] [ `got ${n}` ]] 7 f", want: "[got 7]", wantInterp: true},  // param hole
	}
	for _, c := range cases {
		prog, reason, _, cerr := mustNew(t).CompileCheck(c.src)
		if cerr != nil {
			t.Errorf("%q: check error %v", c.src, cerr)
			continue
		}
		if prog == nil {
			t.Errorf("%q: must compile, declined: %s", c.src, reason)
			continue
		}
		dis := prog.Disassemble()
		if strings.Contains(dis, "FALLBACK") {
			t.Errorf("%q: must compile natively, got an island:\n%s", c.src, dis)
		}
		if c.wantInterp && !strings.Contains(dis, "INTERP") {
			t.Errorf("%q: dynamic hole must lower to INTERP:\n%s", c.src, dis)
		}
		gotC, compiled, errC := mustNew(t).RunCompiled(c.src)
		gotI, errI := mustNew(t).RunInterp(c.src)
		if noteCompileDefect(t, c.src, gotC, errC) {
			continue
		}
		if !compiled || errC != nil || errI != nil {
			t.Errorf("%q: run failed compiled=%v errC=%v errI=%v", c.src, compiled, errC, errI)
			continue
		}
		if fmt.Sprint(gotC) != c.want || fmt.Sprint(gotI) != c.want {
			t.Errorf("%q: compiled=%v interp=%v want=%s", c.src, gotC, gotI, c.want)
		}
	}
}

// Completion item 1 — illegal_ref trap programs. `/u` applied to a NON-fn
// binding declined at the downstream Undefined placeholder ("operand
// provenance"), because the checker is lenient (the illegal_ref diagnostic
// is advisory) while the interpreter raises illegal_ref. A top-level
// RecordTrap now compiles a terminal OpTrap raising the byte-identical
// error, so the row produces a Program instead of declining.
//
// `/v` is NOT in this set any more: it is total over binding kinds, so
// `def x 5  x/v` is 5, not a trap. Its positive twin is
// TestValOnNonFnBindingCompilesToTheValue below.
func TestIllegalRefTrapCompiles(t *testing.T) {
	for _, src := range []string{`def x 5  x/u`} {
		prog, reason, _, cerr := mustNew(t).CompileCheck(src)
		if cerr != nil {
			t.Fatalf("%q: check error %v", src, cerr)
		}
		if prog == nil {
			t.Fatalf("%q: expected a trap program, declined: %s", src, reason)
		}
		if dis := prog.Disassemble(); !strings.Contains(dis, "TRAP") || strings.Contains(dis, "FALLBACK") {
			t.Errorf("%q: expected a terminal TRAP, no island:\n%s", src, dis)
		}
		// Parity: the compiled program raises illegal_ref, exactly like the
		// interpreter (same taxonomy).
		_, compiled, errC := mustNew(t).RunCompiled(src)
		if noteCompileDefect(t, src, nil, errC) {
			continue
		}
		if !compiled {
			t.Errorf("%q: trap program did not run compiled (fell back)", src)
		}
		_, errI := mustNew(t).RunInterp(src)
		if codeOf(errC) != "illegal_ref" || codeOf(errI) != "illegal_ref" {
			t.Errorf("%q: compiled=[%s] interp=[%s], want both illegal_ref", src, codeOf(errC), codeOf(errI))
		}
	}

	// NEGATIVE: a LEGAL ref to a real fn binding must NOT trap — it still
	// compiles to a value-producing program (the held fn fires when grouped).
	const ok = `def z fn [[][Integer][42]]  (z/v)`
	prog, reason, _, cerr := mustNew(t).CompileCheck(ok)
	if cerr != nil || prog == nil {
		t.Fatalf("legal /v row did not compile: reason=%q err=%v", reason, cerr)
	}
	if strings.Contains(prog.Disassemble(), "TRAP") {
		t.Errorf("a legal /v must not emit an illegal_ref TRAP:\n%s", prog.Disassemble())
	}
	gotC, compiled, errC := mustNew(t).RunCompiled(ok)
	if noteCompileDefect(t, ok, gotC, errC) {
		return
	}
	if !compiled || errC != nil {
		t.Fatalf("legal /v compiled run: compiled=%v err=%v", compiled, errC)
	}
	gotI, _ := mustNew(t).RunInterp(ok)
	if fmt.Sprint(gotC) != fmt.Sprint(gotI) {
		t.Errorf("legal /v parity: compiled=%v interp=%v", gotC, gotI)
	}
}

// TestValOnNonFnBindingCompilesToTheValue is the `/v` half that used to be
// a trap: with the function-only gate gone, `def x 5  x/v` is an ordinary
// value both compiled and interpreted, and the engines agree.
func TestValOnNonFnBindingCompilesToTheValue(t *testing.T) {
	const src = `def x 5  x/v`
	gotC, compiled, errC := mustNew(t).RunCompiled(src)
	gotI, errI := mustNew(t).RunInterp(src)
	if noteCompileDefect(t, src, gotC, errC) {
		return
	}
	if errC != nil || errI != nil {
		t.Fatalf("%q: errC=%v errI=%v", src, errC, errI)
	}
	if !compiled {
		t.Errorf("%q: expected it to run compiled, it fell back", src)
	}
	if fmt.Sprint(gotC) != "[5]" || fmt.Sprint(gotI) != "[5]" {
		t.Errorf("%q: compiled=%v interp=%v, want [5] from both", src, gotC, gotI)
	}
}

// Completion item 1 (cont.) — expansion-time error traps for mini/parse. A core
// `mini <kind>` / `parse <kind>` whose kind needs an import that is absent
// degrades to a dynamic carrier under the lenient checker (the import "may be
// outside the checked fragment"), so the row declined downstream. A top-level
// RecordTrap in that branch now compiles a terminal OpTrap raising the
// byte-identical *_unknown_lang, exactly as the interpreter does at the call site.
func TestMiniParseUnknownLangTrapCompiles(t *testing.T) {
	cases := []struct{ src, code string }{
		{`mini re 'a'`, "mini_unknown_lang"},
		{`+re/[a-z]+/`, "mini_unknown_lang"},
		{`parse calc 'x'`, "parse_unknown_lang"},
	}
	for _, c := range cases {
		prog, reason, _, cerr := mustNew(t).CompileCheck(c.src)
		if cerr != nil {
			t.Fatalf("%q: check error %v", c.src, cerr)
		}
		if prog == nil {
			t.Fatalf("%q: expected a trap program, declined: %s", c.src, reason)
		}
		if dis := prog.Disassemble(); !strings.Contains(dis, "TRAP") || strings.Contains(dis, "FALLBACK") {
			t.Errorf("%q: expected a terminal TRAP, no island:\n%s", c.src, dis)
		}
		_, compiled, errC := mustNew(t).RunCompiled(c.src)
		if noteCompileDefect(t, c.src, nil, errC) {
			continue
		}
		if !compiled {
			t.Errorf("%q: trap program did not run compiled (fell back)", c.src)
		}
		_, errI := mustNew(t).RunInterp(c.src)
		if codeOf(errC) != c.code || codeOf(errI) != c.code {
			t.Errorf("%q: compiled=[%s] interp=[%s], want both %s", c.src, codeOf(errC), codeOf(errI), c.code)
		}
	}

	// NEGATIVE: with the import present, a VALID kind must NOT trap — it compiles
	// (or faithfully falls back) and produces the real value, never an
	// *_unknown_lang error.
	const ok = `import "boru:minilang"  ("AbcD" mini re '[a-z]+').fst.m`
	gotC, _, errC := mustNew(t).RunCompiled(ok)
	gotI, errI := mustNew(t).RunInterp(ok)
	if noteCompileDefect(t, ok, gotC, errC) {
		return
	}
	if errC != nil || errI != nil {
		t.Fatalf("valid mini re: compiled err=%v interp err=%v", errC, errI)
	}
	if fmt.Sprint(gotC) != fmt.Sprint(gotI) {
		t.Errorf("valid mini re parity: compiled=%v interp=%v", gotC, gotI)
	}
}

// getr-on-namespace not_found: `MathUtil!.nope` (getr of a MISSING export)
// raises not_found at runtime. The compile pass records a top-level not_found
// OpTrap (moduleNSGetrReturns) and MarkUncompilable is a no-op once a trap is
// set, so the getr's own unmaterialisable residual (which declines even valid
// keys) does not decline the program — the trap truncates it.
func TestModuleExportGetrNotFoundTrapCompiles(t *testing.T) {
	// Legacy compile failure+fallback-parity contract: pins the one-release
	const src = `import "boru:math-util"  MathUtil!.nope`
	prog, reason, _, cerr := mustNew(t).CompileCheck(src)
	if cerr != nil {
		t.Fatalf("%q: check error %v", src, cerr)
	}
	if prog == nil {
		t.Fatalf("%q: expected a trap program, declined: %s", src, reason)
	}
	if dis := prog.Disassemble(); !strings.Contains(dis, "TRAP") || strings.Contains(dis, "FALLBACK") {
		t.Errorf("%q: expected a terminal TRAP, no island:\n%s", src, dis)
	}
	_, compiled, errC := mustNew(t).RunCompiled(src)
	if noteCompileDefect(t, src, nil, errC) {
		return
	}
	if !compiled {
		t.Errorf("%q: trap program did not run compiled (fell back)", src)
	}
	_, errI := mustNew(t).RunInterp(src)
	if codeOf(errC) != "not_found" || codeOf(errI) != "not_found" {
		t.Errorf("%q: compiled=[%s] interp=[%s], want both not_found", src, codeOf(errC), codeOf(errI))
	}

	// NEGATIVE 1: a VALID getr export must NOT trap — it compiles and produces
	// the real value, with compiled/interp parity.
	const okGetr = `import "boru:math-util"  MathUtil!.sqrt 16.0`
	if p, _, _, _ := mustNew(t).CompileCheck(okGetr); p != nil {
		if dis := p.Disassemble(); strings.Contains(dis, "TRAP") {
			t.Errorf("valid dotr must not trap:\n%s", dis)
		}
	}
	gotC, _, eC := mustNew(t).RunCompiled(okGetr)
	gotI, eI := mustNew(t).RunInterp(okGetr)
	if noteCompileDefect(t, okGetr, gotC, eC) {
		return
	}
	if eC != nil || eI != nil {
		t.Fatalf("valid getr: compiled err=%v interp err=%v", eC, eI)
	}
	if fmt.Sprint(gotC) != fmt.Sprint(gotI) {
		t.Errorf("valid dotr parity: compiled=%v interp=%v", gotC, gotI)
	}

	// NEGATIVE 2: `get` (not getr) of a MISSING key returns None, never traps —
	// only getr raises not_found, so the get path stays on the shared ReturnsFn.
	const okGet = `import "boru:math-util"  MathUtil.nope`
	if _, _, eGet := mustNew(t).RunCompiled(okGet); eGet != nil {
		t.Errorf("dot of a missing key must not error, got %v", eGet)
	}
}

// Bare-value map-field const-fold: a bare word that is a 0-arg fn auto-fires as a
// map value (`{a:g}` → `{a:42}`, like the parenthesised `{a:(g)}`). The bare form
// previously fell to autoEvalMap's sub-engine eval, leaving a check-mode carrier
// of unknown provenance that declined; it now const-folds to its concrete result
// (identical to the interpreter's sub-engine eval) and the map bakes as a const.
func TestBareFnMapFieldCompiles(t *testing.T) {
	cases := []string{
		`def g fn [[] [Integer] [42]] def m {a:g} m`,
		`def g fn [[] [Integer] [42]] {a:g}`,
	}
	for _, src := range cases {
		prog, reason, _, cerr := mustNew(t).CompileCheck(src)
		if cerr != nil {
			t.Fatalf("%q: check error %v", src, cerr)
		}
		if prog == nil {
			t.Fatalf("%q: expected native compile, declined: %s", src, reason)
		}
		if dis := prog.Disassemble(); strings.Contains(dis, "FALLBACK") {
			t.Errorf("%q: expected no island:\n%s", src, dis)
		}
		gotC, compiled, eC := mustNew(t).RunCompiled(src)
		gotI, eI := mustNew(t).RunInterp(src)
		if noteCompileDefect(t, src, gotC, eC) {
			continue
		}
		if !compiled {
			t.Errorf("%q: did not run compiled", src)
		}
		if eC != nil || eI != nil {
			t.Fatalf("%q: compiled err=%v interp err=%v", src, eC, eI)
		}
		if fmt.Sprint(gotC) != fmt.Sprint(gotI) {
			t.Errorf("%q: parity: compiled=%v interp=%v", src, gotC, gotI)
		}
	}

	// NEGATIVE: a plain literal map and a bare def-bound value still compile with
	// parity — the fold must not change their behaviour.
	for _, src := range []string{`def m {a:5 b:"x"} m`, `def x 7 def m {a:x} m`} {
		gotC, _, eC := mustNew(t).RunCompiled(src)
		gotI, eI := mustNew(t).RunInterp(src)
		if noteCompileDefect(t, src, gotC, eC) {
			continue
		}
		if eC != nil || eI != nil || fmt.Sprint(gotC) != fmt.Sprint(gotI) {
			t.Errorf("%q: compiled=%v(%v) interp=%v(%v)", src, gotC, eC, gotI, eI)
		}
	}
}

// Cluster-5 Gap A — chained variadic-statement-if: a 2-arg `if`'s 0-or-1 result
// is claimed as the else of a following `if`. The variadic depth is only known at
// run time, so the claiming if cannot drop it at a fixed offset — it uses a
// variadic stack region (OpStackMark / OpDropToMark / OpPopMark).
func TestChainedVariadicIfCompiles(t *testing.T) {
	cases := []struct {
		src  string
		want string
	}{
		{`def n 0 if (n eq 0) [98] if (n eq 0) [99] add 1 2`, "[99 3]"},
		{`def n 5 if (n eq 0) [98] if (n eq 0) [99] add 1 2`, "[3]"},
		{`def n 5 if (n eq 0) [98] if (n eq 5) [99] add 1 2`, "[99 3]"},
	}
	for _, c := range cases {
		prog, reason, _, cerr := mustNew(t).CompileCheck(c.src)
		if cerr != nil {
			t.Fatalf("%q: check error %v", c.src, cerr)
		}
		if prog == nil {
			t.Fatalf("%q: expected native compile, declined: %s", c.src, reason)
		}
		if dis := prog.Disassemble(); strings.Contains(dis, "FALLBACK") {
			t.Errorf("%q: expected no island:\n%s", c.src, dis)
		}
		gotC, compiled, eC := mustNew(t).RunCompiled(c.src)
		gotI, eI := mustNew(t).RunInterp(c.src)
		if noteCompileDefect(t, c.src, gotC, eC) {
			continue
		}
		if !compiled {
			t.Errorf("%q: did not run compiled", c.src)
		}
		if eC != nil || eI != nil {
			t.Fatalf("%q: compiled err=%v interp err=%v", c.src, eC, eI)
		}
		if fmt.Sprint(gotC) != c.want || fmt.Sprint(gotI) != c.want {
			t.Errorf("%q: compiled=%v interp=%v want %s", c.src, gotC, gotI, c.want)
		}
	}
}

// Cluster 7a — a 0-net code-body case scrutinee compiles to a case_error trap.
// `case [f 1] [...]` where f produces no value is the runtime case_error
// ("value expression produced no value to dispatch on"); caseReturnsFn detects
// the empty residual via the RECORDING analysis and records a terminal OpTrap
// instead of islanding. (A type-only run would mis-see the empty-body fn as
// 1-value, so the recording residual is load-bearing.)
func TestCaseEmptyScrutineeTrapCompiles(t *testing.T) {
	const src = `def f fn [[x:Integer] [] []] case [f 1] [2 "two" "other"]`
	prog, reason, _, cerr := mustNew(t).CompileCheck(src)
	if cerr != nil {
		t.Fatalf("check error %v", cerr)
	}
	if prog == nil {
		t.Fatalf("expected a trap program, declined: %s", reason)
	}
	if dis := prog.Disassemble(); !strings.Contains(dis, "TRAP") || strings.Contains(dis, "FALLBACK") {
		t.Errorf("expected a terminal TRAP, no island:\n%s", dis)
	}
	_, compiled, errC := mustNew(t).RunCompiled(src)
	if noteCompileDefect(t, src, nil, errC) {
		return
	}
	if !compiled {
		t.Errorf("trap program did not run compiled (fell back)")
	}
	_, errI := mustNew(t).RunInterp(src)
	if codeOf(errC) != "case_error" || codeOf(errI) != "case_error" {
		t.Errorf("compiled=[%s] interp=[%s], want both case_error", codeOf(errC), codeOf(errI))
	}

	// POSITIVE: a VALUE-producing code-body scrutinee with a single clause +
	// default and value blocks now compiles NATIVE (no island) — the body runs
	// once via `do` in the one guard cond — and must NOT trap (it dispatches).
	const ok = `case [1 add 1] [2 "two" "other"]`
	prog2, reason2, _, cerr2 := mustNew(t).CompileCheck(ok)
	if cerr2 != nil || prog2 == nil {
		t.Fatalf("value scrutinee did not compile: reason=%q err=%v", reason2, cerr2)
	}
	if dis := prog2.Disassemble(); strings.Contains(dis, "FALLBACK") || strings.Contains(dis, "TRAP") {
		t.Errorf("value scrutinee: expected native if-chain, no island/trap:\n%s", dis)
	}
	gotC, _, errC2 := mustNew(t).RunCompiled(ok)
	gotI, errI2 := mustNew(t).RunInterp(ok)
	if noteCompileDefect(t, ok, gotC, errC2) {
		return
	}
	if errC2 != nil || errI2 != nil {
		t.Fatalf("value scrutinee: compiled err=%v interp err=%v", errC2, errI2)
	}
	if fmt.Sprint(gotC) != fmt.Sprint(gotI) {
		t.Errorf("value scrutinee parity: compiled=%v interp=%v", gotC, gotI)
	}
}

// Cluster 5 (partial) — a user-fn-call result above a literal in the program
// residual. `def add2 fn […] 1 add2 2 3` leaves residual [1, 5] (literal 1
// below, the add2 CALL_USER result 5 on top); it declined "residual shape beyond
// Stage 1 (call result above a literal)" because the out-of-order residual
// promotion (forceOrder → frame local) handled only native (evCall) results,
// never user calls (evCallUser). Now a user-call result seats to a local and
// re-pushes in order.
func TestUserCallResidualAboveLiteral(t *testing.T) {
	cases := []string{
		`def add2 fn [[x:Integer y:Integer][Integer][x add y]] 1 add2 2 3`,
		// The word-splice spec row: vs splices 2 3, so `1 add2 vs` → residual [1, 5].
		`def vs word [2,3] def add2 fn [[x:Integer y:Integer][Integer][x add y]] 1 add2 vs`,
	}
	for _, src := range cases {
		prog, reason, _, cerr := mustNew(t).CompileCheck(src)
		if cerr != nil || prog == nil {
			t.Fatalf("%q: did not compile: reason=%q err=%v", src, reason, cerr)
		}
		if strings.Contains(prog.Disassemble(), "FALLBACK") {
			t.Errorf("%q: expected native, got an island:\n%s", src, prog.Disassemble())
		}
		gotC, compiled, errC := mustNew(t).RunCompiled(src)
		if noteCompileDefect(t, src, gotC, errC) {
			continue
		}
		if !compiled || errC != nil {
			t.Fatalf("%q: compiled run: compiled=%v err=%v", src, compiled, errC)
		}
		gotI, _ := mustNew(t).RunInterp(src)
		if fmt.Sprint(gotC) != fmt.Sprint(gotI) {
			t.Errorf("%q: parity: compiled=%v interp=%v", src, gotC, gotI)
		}
	}

	// The full Test.run-spec harness (module-test:38) compiles natively on this
	// branch (zero-count `for` pruning over `subs: []` drops the unreachable
	// recursive branch; see TestRunSpecHarnessCompiles). Whatever the tier, the
	// compiled run must stay byte-identical to the interpreter — pin that parity.
	const harness = `import "boru:test"  def double fn [[n:Integer] [Integer] [n 2 mul]] end def s {name: "doubling" subject: double/q cases: [{name: "d3" in: [3] out: 6} {name: "d0" in: [0] out: 0}] subs: []} end s Test.run-spec end Test.summary`
	gotC, _, errC := mustNew(t).RunCompiled(harness)
	gotI, errI := mustNew(t).RunInterp(harness)
	if noteCompileDefect(t, harness, gotC, errC) {
		return
	}
	if (errC == nil) != (errI == nil) || fmt.Sprint(gotC) != fmt.Sprint(gotI) {
		t.Errorf("Test harness parity: compiled=%v(%v) interp=%v(%v)", gotC, errC, gotI, errI)
	}
}

// Zero-count `for` pruning (module-test:38). A `for` whose count operand is a
// CONCRETE non-positive Integer never enters its body — both engines iterate
// zero times and push zero values. The recorder now prunes it: it analyses
// neither the body (so an unreachable branch that only type-checks for a live
// iteration no longer poisons the program) nor records a loop. The lever that
// clears module-test:38 — `run-spec`'s `for (subs size) [subspec run-spec]`
// over `subs: []` recurses into `run-spec` over a carrier `subspec` that cannot
// statically dispatch; pruning the dead branch lets the whole spec runner
// compile (run-spec body → closure, run-cases/run-case → CALL_USER, test-invoke
// → poly). The `size` ReturnsFn folding a concrete container to its static
// count is the prerequisite that makes `subs size` a concrete 0.
func TestRunSpecHarnessCompiles(t *testing.T) {
	// The full Test.run-spec harness must be byte-identical between compiled and
	// interpreter runs — including the test-record side effects ({total:2 passed:2
	// failed:0}), which the compiled pass must NOT double-count.
	//
	// SOUNDNESS (always): compile == interpret. FALLBACK ALLOWED: since the
	// fn-dispatch unification (all fns compile their body as a unit via execMatch,
	// no separate inline CallBoru path), Test.run-spec no longer compiles NATIVELY —
	// the recursive code-body word `test-describe` (run-spec → test-describe →
	// run-spec) hits the recursive-closure-compilation limit the inline path used to
	// mask via data-driven recursion termination. So run-spec falls back to the
	// interpreter (sound). Restoring native compilation is the
	// recursive-code-body-closure follow-up (design/legacy/MODULE-FN-PARAM-SLOT-COMPILATION.0.ignore
	// §8); until then this asserts the soundness invariant, not native coverage.
	const harness = `"boru:test" import end  def double fn [[n:Integer] [Integer] [n 2 mul]] end def s {name: "doubling" subject: double/q cases: [{name: "d3" in: [3] out: 6} {name: "d0" in: [0] out: 0}] subs: []} end s Test.run-spec end Test.summary`
	gotC, _, errC := mustNew(t).RunCompiled(harness) // fallback allowed
	gotI, errI := mustNew(t).RunInterp(harness)
	if errC != nil || errI != nil {
		t.Fatalf("run-spec harness run: errC=%v errI=%v", errC, errI)
	}
	if fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotC) != "[{total:2 passed:2 failed:0}]" {
		t.Errorf("run-spec harness parity: compiled=%v interp=%v want [{total:2 passed:2 failed:0}]", gotC, gotI)
	}

	// POSITIVE (the prune in isolation): a `for` over a statically-zero count
	// compiles even when its body would NOT compile on its own — the body is
	// unreachable. Here the body calls an undefined word, which would decline a
	// live loop; pruned, the program compiles and yields the trailing value.
	for _, c := range []struct{ src, want string }{
		{`for 0 [ nope-undefined ] end 7`, "[7]"},
		{`for (-3) [ nope-undefined ] end 7`, "[7]"},
		{`def xs [] end for (xs size) [ nope-undefined ] end 7`, "[7]"},
		{`for [5 5] [ nope-undefined ] end 7`, "[7]"}, // empty range [start=5 end=5]
	} {
		prog, reason, _, cerr := mustNew(t).CompileCheck(c.src)
		if cerr != nil || prog == nil {
			t.Errorf("%q: zero-count prune did not compile: reason=%q", c.src, reason)
			continue
		}
		gotC, compiled, eC := mustNew(t).RunCompiled(c.src)
		gotI, eI := mustNew(t).RunInterp(c.src)
		if noteCompileDefect(t, c.src, gotC, eC) {
			continue
		}
		if !compiled || eC != nil || eI != nil {
			t.Errorf("%q: run: compiled=%v eC=%v eI=%v", c.src, compiled, eC, eI)
			continue
		}
		if fmt.Sprint(gotC) != c.want || fmt.Sprint(gotI) != c.want {
			t.Errorf("%q: compiled=%v interp=%v want %s", c.src, gotC, gotI, c.want)
		}
	}

	// NEGATIVE: a LIVE loop (concretely non-zero count) is NOT pruned — its
	// body is analysed as usual, so the per-iteration value is produced and the
	// result matches the interpreter. The prune must not leak into live loops.
	for _, c := range []struct{ src, want string }{
		{`for 3 [ i ] end`, "[0 1 2]"},
		{`def xs [9 8] end for (xs size) [ i ] end`, "[0 1]"},
	} {
		gotC, compiled, eC := mustNew(t).RunCompiled(c.src)
		gotI, eI := mustNew(t).RunInterp(c.src)
		if noteCompileDefect(t, c.src, gotC, eC) {
			continue
		}
		if !compiled || eC != nil || eI != nil {
			t.Errorf("%q: live loop run: compiled=%v eC=%v eI=%v", c.src, compiled, eC, eI)
			continue
		}
		if fmt.Sprint(gotC) != c.want || fmt.Sprint(gotI) != c.want {
			t.Errorf("%q: live loop compiled=%v interp=%v want %s", c.src, gotC, gotI, c.want)
		}
	}

	// NEGATIVE (soundness of the prune boundary): a live one-iteration loop
	// whose body declines must still DECLINE — pruning fires only for a concrete
	// zero count, never for count 1.
	prog2, _, _, _ := mustNew(t).CompileCheck(`for 1 [ nope-undefined ] end 7`)
	if prog2 != nil && !strings.Contains(prog2.Disassemble(), "FALLBACK") {
		// An undefined word in a LIVE loop body must not compile natively.
		t.Errorf("live count-1 loop with declining body unexpectedly compiled native:\n%s", prog2.Disassemble())
	}
}

// Stage H — dispatch-recovery operand order. `3 and "x"` types as the
// disjunct Integer|String (`and`'s operand-join return), which straddles
// `add`'s overloads, so matchSignature fails and checkModeAssumeSig
// recovers. The strict-disjunct straddle is a sound runtime re-dispatch
// (each runtime alternative matches one concrete overload), so the recovery
// now records OpCallNativePoly instead of latching the program uncompilable
// — mirroring the normal-path handling in carrierResults.
func TestDispatchRecoveryPolyCompiles(t *testing.T) {
	cases := []struct{ src, want string }{
		{`(3 and "x") add 1`, "[x1]"}, // false guard truthy: and → "x", "x" add 1 → x1
		{`(0 and "x") add 1`, "[1]"},  // 0 falsy: and → 0, 0 add 1 → 1
		{`(5 and 2) add 1`, "[3]"},    // numeric path: and → 2, 2 add 1 → 3
		{`(0 and 2) add 1`, "[1]"},
	}
	for _, c := range cases {
		prog, reason, _, cerr := mustNew(t).CompileCheck(c.src)
		if cerr != nil {
			t.Fatalf("%q: check error %v", c.src, cerr)
		}
		if prog == nil {
			t.Fatalf("%q: expected native compile, declined: %s", c.src, reason)
		}
		if strings.Contains(prog.Disassemble(), "FALLBACK") {
			t.Errorf("%q: expected no island:\n%s", c.src, prog.Disassemble())
		}
		gotC, compiled, eC := mustNew(t).RunCompiled(c.src)
		gotI, eI := mustNew(t).RunInterp(c.src)
		if noteCompileDefect(t, c.src, gotC, eC) {
			continue
		}
		if !compiled {
			t.Errorf("%q: did not run compiled", c.src)
		}
		if eC != nil || eI != nil {
			t.Fatalf("%q: compiled err=%v interp err=%v", c.src, eC, eI)
		}
		if fmt.Sprint(gotC) != c.want || fmt.Sprint(gotI) != c.want {
			t.Errorf("%q: compiled=%v interp=%v want %s", c.src, gotC, gotI, c.want)
		}
	}

	// OPERAND ORDER (the regression guard). The recovered call must consume
	// its operands in the SAME order as the interpreter: `(3 and "x") add 1`
	// is `"x" add 1` → x1 (forward arg 1 = sig[0], stack "x" = sig[1]), which
	// is DISTINCT from the all-forward `add "x" 1` → 1x. The prior poly
	// attempt recorded the raw tape order and produced 1x for the first — pin
	// the distinction so the operand-order rebuild can't silently regress.
	for _, c := range []struct{ src, want string }{
		{`(3 and "x") add 1`, "[x1]"},
		{`add "x" 1`, "[1x]"},
		{`"x" add 1`, "[x1]"},
	} {
		gotC, _, eC := mustNew(t).RunCompiled(c.src)
		gotI, eI := mustNew(t).RunInterp(c.src)
		if noteCompileDefect(t, c.src, gotC, eC) {
			continue
		}
		if eC != nil || eI != nil {
			t.Fatalf("%q: compiled err=%v interp err=%v", c.src, eC, eI)
		}
		if fmt.Sprint(gotC) != c.want || fmt.Sprint(gotI) != c.want {
			t.Errorf("%q: order: compiled=%v interp=%v want %s", c.src, gotC, gotI, c.want)
		}
	}
}

// Dynamic dispatch over an Any-typed operand. A value whose static type is
// unknown — a List/Map element read with `get`, an opaque result — reaches a
// pure core builtin (`get`, `add`, …) that matchSignature cannot bind to any
// overload (a strict Any conforms to no concrete operand type), so it lands in
// the no-signature recovery. Rather than decline, the recovery records a
// runtime-re-matching OpCallNativePoly for a SAFE pure builtin: the VM
// re-matches over the concrete value at run time — the same first-match the
// interpreter takes — so the compiled and interpreted results agree. This is
// the lever for the dynamic-input frontier (e.g. the test-framework bodies'
// `(cases _i get) get "in"`).
func TestDynamicOperandRecoveryPolyCompiles(t *testing.T) {
	cases := []struct{ src, want string }{
		// `get` over an Any (a declared-Any fn return), keyed by a string.
		{`def g fn [[] [Any] [{a:5}]]  (g) get "a"`, "[5]"},
		// `get` of a MISSING key over an Any → None, faithfully.
		{`def g fn [[] [Any] [{a:5}]]  (g) get "z"`, "[None]"},
		// arithmetic over an Any-typed value.
		{`def g fn [[] [Any] [10]]  (g) add 1`, "[11]"},
	}
	for _, c := range cases {
		gotC, compiled, eC := mustNew(t).RunCompiled(c.src)
		gotI, eI := mustNew(t).RunInterp(c.src)
		if noteCompileDefect(t, c.src, gotC, eC) {
			continue
		}
		if eC != nil || eI != nil {
			t.Fatalf("%q: compiled err=%v interp err=%v", c.src, eC, eI)
		}
		if !compiled {
			t.Errorf("%q: expected a compiled run (poly), fell back to interpreter", c.src)
		}
		if fmt.Sprint(gotC) != c.want || fmt.Sprint(gotI) != c.want {
			t.Errorf("%q: compiled=%v interp=%v want %s", c.src, gotC, gotI, c.want)
		}
	}

	// NEGATIVE: a CONCRETE operand whose type genuinely matches no overload is
	// a real type error, NOT a dynamic dispatch — it must still decline the poly
	// (anyAnyCarrier gates on an Any carrier) and the interpreter raises the
	// same signature_error. `get` over an Integer has no signature.
	for _, src := range []string{
		`def f fn [[n:Integer] [Any] [n get "a"]]  f 5`,
		`5 get "a"`,
	} {
		_, _, eC := mustNew(t).RunCompiled(src)
		_, eI := mustNew(t).RunInterp(src)
		if noteCompileDefect(t, src, nil, eC) {
			continue
		}
		if eC == nil || eI == nil {
			t.Errorf("%q: expected a signature error in both engines, compiled=%v interp=%v", src, eC, eI)
			continue
		}
		if !strings.Contains(eC.Error(), "signature") || !strings.Contains(eI.Error(), "signature") {
			t.Errorf("%q: expected signature_error, compiled=%v interp=%v", src, eC, eI)
		}
	}
}

// A 0-output dynamic dispatch over an Any-typed operand. A side-effect / list
// word whose check-mode result carries NO value (`drop` over a dynamic list)
// reaches a pure builtin matchSignature cannot bind, so it is a poly candidate
// with zero outputs. The recorder now admits a 0-result poly (len(outs) <= 1):
// OpCallNativePoly pops the operands, the VM runs the re-matched handler, and
// nothing is pushed — exactly as for the interpreter. This is the dispatch
// shape behind the test framework's `test-record` (a 7-arg, 0-output side
// effect); the lever lands `flex.tsv:138` here.
func TestZeroOutputDynamicPolyCompiles(t *testing.T) {
	const src = `def l [1 2] push 3 l drop l`
	gotC, compiled, eC := mustNew(t).RunCompiled(src)
	gotI, eI := mustNew(t).RunInterp(src)
	if noteCompileDefect(t, src, gotC, eC) {
		return
	}
	if eC != nil || eI != nil {
		t.Fatalf("compiled err=%v interp err=%v", eC, eI)
	}
	if !compiled {
		t.Errorf("expected a compiled run (0-output poly), fell back to interpreter")
	}
	if fmt.Sprint(gotC) != "[[1 2]]" || fmt.Sprint(gotI) != "[[1 2]]" {
		t.Errorf("compiled=%v interp=%v want [[1 2]]", gotC, gotI)
	}
	prog, reason, _, cerr := mustNew(t).CompileCheck(src)
	if cerr != nil || prog == nil {
		t.Fatalf("expected native compile, declined: %s (cerr=%v)", reason, cerr)
	}
	if strings.Contains(prog.Disassemble(), "FALLBACK") {
		t.Errorf("expected no interpreter island:\n%s", prog.Disassemble())
	}
}

// `set` over a dynamic receiver — a mutation / copy-return whose container is
// statically unknown (the IO capability's `context get __sys get fs set mem
// true`, where the fs Store is a dynamic carrier). matchSignature binds the
// dynamic carrier, but the normal path declined "dynamic input at set"; set now
// joins get/getr in the QuoteArgs poly exemption (its atom key bakes as an inert
// const) so it records OpCallNativePoly. The runtime re-match runs the SAME
// handler over the SAME concrete receiver the interpreter mutates, so the side
// effect (Store/Object/Array mutation) and the copy-return (Map/List) are
// faithful — verified by the differential corpus, which compares the post-
// mutation observable result.
func TestSetOverDynamicReceiverPolyCompiles(t *testing.T) {
	const imp = `import "boru:io"  `
	cases := []struct{ src, want string }{
		// 0-output Store mutation over a dynamic context receiver, then a read of
		// the mutated context through IO.write / IO.read.
		{imp + `context dot __sys dot fs set mem true  IO.write (make Pathon "mem://b.txt") "hi"`, "[mem:/b.txt]"},
		{imp + `context dot __sys dot fs set mem true  IO.read (IO.write (make Pathon "mem://a.txt") "hello")`, "[hello]"},
	}
	for _, c := range cases {
		gotC, compiled, eC := mustNew(t).RunCompiled(c.src)
		gotI, eI := mustNew(t).RunInterp(c.src)
		if noteCompileDefect(t, c.src, gotC, eC) {
			continue
		}
		if eC != nil || eI != nil {
			t.Fatalf("%q: compiled err=%v interp err=%v", c.src, eC, eI)
		}
		if !compiled {
			t.Errorf("%q: expected a compiled run (set poly), fell back", c.src)
		}
		if fmt.Sprint(gotC) != c.want || fmt.Sprint(gotI) != c.want {
			t.Errorf("%q: compiled=%v interp=%v want %s", c.src, gotC, gotI, c.want)
		}
	}

	// NEGATIVE: `set` over a CONCRETE non-container (Integer) matches no overload
	// — a genuine type error, not a dynamic dispatch. It must decline the poly and
	// raise the same signature_error in both engines, never silently succeed.
	_, _, eC := mustNew(t).RunCompiled(`5 set a 9`)
	_, eI := mustNew(t).RunInterp(`5 set a 9`)
	if noteCompileDefect(t, `5 set a 9`, nil, eC) {
		return
	}
	if eC == nil || eI == nil {
		t.Fatalf("set over Integer: expected signature error in both engines, compiled=%v interp=%v", eC, eI)
	}
	if !strings.Contains(eC.Error(), "signature") || !strings.Contains(eI.Error(), "signature") {
		t.Errorf("set over Integer: expected signature_error, compiled=%v interp=%v", eC, eI)
	}
}

// TestSetOverDynamicReceiverConsumedCompiles is the mixed-arity gradual-dispatch
// counterpart to the statement-position cases above: when a `set` over a DYNAMIC
// receiver sits in PAREN-TAIL position its result is consumed (here bound by an
// inner `def`), so the checker models the one gradual value the value-returning
// overload yields — `def k2` binds a carrier instead of falsely reporting
// undefined_word, and the whole fn compiles byte-identically. Previously this
// declined under --force-compile (the gradual model was gated to pure check
// mode); the paren-tail signal makes it sound to model under a real compile too,
// because the single value is consumed by the group close and never collected as
// a sibling word's extra arg.
func TestSetOverDynamicReceiverConsumedCompiles(t *testing.T) {
	cases := []struct{ src, want string }{
		// get over a Map yields a dynamic receiver; set on it (paren-tail, bound
		// by def k2) returns the updated map, consumed as the body result.
		{`def f fn [[nd:Map] [Any] [ def k2 ((nd "a" get) set "x" 1) k2 ]]  ({a:{}} f)`, "[{x:1}]"},
		// Forward-arg consumption: the set result feeds an outer word directly.
		{`def g fn [[nd:Map] [Map] [ ((nd "a" get) set "x" 1) ]]  ({a:{}} g)`, "[{x:1}]"},
	}
	for _, c := range cases {
		prog, reason, _, cerr := mustNew(t).CompileCheck(c.src)
		if cerr != nil {
			t.Fatalf("%q: check error %v", c.src, cerr)
		}
		if prog == nil {
			t.Fatalf("%q: expected a compiled program, declined: %s", c.src, reason)
		}
		gotC, compiled, eC := mustNew(t).RunCompiled(c.src)
		gotI, eI := mustNew(t).RunInterp(c.src)
		if noteCompileDefect(t, c.src, gotC, eC) {
			continue
		}
		if eC != nil || eI != nil {
			t.Fatalf("%q: compiled err=%v interp err=%v", c.src, eC, eI)
		}
		if !compiled {
			t.Errorf("%q: expected a compiled run, fell back", c.src)
		}
		if fmt.Sprint(gotC) != fmt.Sprint(gotI) {
			t.Errorf("%q: parity: compiled=%v interp=%v", c.src, gotC, gotI)
		}
		if fmt.Sprint(gotI) != c.want {
			t.Errorf("%q: interp=%v want %s", c.src, gotI, c.want)
		}
	}
}

// Stage B — conditional dynamic apply in a branch. `if (n eq 0) [99]
// MathUtil.sqrt 16`: the else arm is the module-export fn value sqrt and the
// trailing 16 applies ONLY when the branch produced the callable. Two parts:
// (1) the trivial-delegation module fn value bakes as an inert const (so the
// else operand resolves), and (2) the branch result carries a mayBeFn flag so
// resolveDynamicApply lowers the trailing arg to a runtime-conditional
// OpCallDynamic — callDynamic applies sqrt on the else path (→ 4.0) and leaves
// [99 16] on the then path. One compiled program is faithful on both branches.
func TestConditionalBranchApplyCompiles(t *testing.T) {
	const imp = `import "boru:math-util" `
	cases := []struct{ src, want string }{
		{imp + `def n 5 if (n eq 0) [99] MathUtil.sqrt 16`, "[4.0]"},          // else: sqrt 16 → 4.0
		{imp + `def n 0 if (n eq 0) [99] MathUtil.sqrt 16`, "[99 4.0]"},       // NUR038 seal: the trailing-fn+arg is its OWN call — the guard commits, then sqrt 16 runs
		{imp + `def n 0 if (n eq 0) MathUtil.sqrt [99] 16`, "[4.0]"},          // then-arm fn mirror
		{imp + `def n 5 if (n eq 0) MathUtil.sqrt [99] 16`, "[99 16]"},        // else: 99, 16 stays
		{imp + `def n 5 if (n eq 0) [99] MathUtil.sqrt`, "[fn sqrt(Number)]"}, // no trailing: bare fn value
		{imp + `MathUtil.sqrt`, "[fn sqrt(Number)]"},                          // bare module fn value bakes
	}
	for _, c := range cases {
		prog, reason, _, cerr := mustNew(t).CompileCheck(c.src)
		if cerr != nil {
			t.Fatalf("%q: check error %v", c.src, cerr)
		}
		if prog == nil {
			t.Fatalf("%q: expected native compile, declined: %s", c.src, reason)
		}
		if strings.Contains(prog.Disassemble(), "FALLBACK") {
			t.Errorf("%q: expected no island:\n%s", c.src, prog.Disassemble())
		}
		gotC, compiled, eC := mustNew(t).RunCompiled(c.src)
		gotI, eI := mustNew(t).RunInterp(c.src)
		if noteCompileDefect(t, c.src, gotC, eC) {
			continue
		}
		if !compiled {
			t.Errorf("%q: did not run compiled", c.src)
		}
		if eC != nil || eI != nil {
			t.Fatalf("%q: compiled err=%v interp err=%v", c.src, eC, eI)
		}
		// 4.0 renders as "4" via fmt of the []any; compare compiled to interp
		// (the authoritative value) and to the expected rendering.
		if fmt.Sprint(gotC) != fmt.Sprint(gotI) {
			t.Errorf("%q: parity: compiled=%v interp=%v", c.src, gotC, gotI)
		}
		if fmt.Sprint(gotI) != c.want {
			t.Errorf("%q: interp=%v want %s", c.src, gotI, c.want)
		}
	}

	// NEGATIVE 1: a non-fn branch result must NOT apply the trailing arg —
	// `if (n eq 0) ["a"] 7 16` leaves [7 16] (7 is not callable), and the
	// mayBeFn flag is never set, so no OpCallDynamic is emitted.
	// NEGATIVE 2: a CAPTURING fn value as an arm must not be baked/applied
	// unsoundly — it stays off the native path (compiled==interp regardless).
	for _, c := range []struct{ src, want string }{
		{`def n 5 if (n eq 0) ["a"] 7 16`, "[7 16]"},
		{`def n 0 if (n eq 0) ["a"] 7 16`, "[a 16]"},
	} {
		gotC, _, eC := mustNew(t).RunCompiled(c.src)
		gotI, eI := mustNew(t).RunInterp(c.src)
		if noteCompileDefect(t, c.src, gotC, eC) {
			continue
		}
		if eC != nil || eI != nil {
			t.Fatalf("%q: compiled err=%v interp err=%v", c.src, eC, eI)
		}
		if fmt.Sprint(gotC) != c.want || fmt.Sprint(gotI) != c.want {
			t.Errorf("%q: no-apply: compiled=%v interp=%v want %s", c.src, gotC, gotI, c.want)
		}
	}
}

// Stage D — sub-registry poly dispatch. A module word (StructUtil.getpath)
// with a DYNAMIC input declined at "dynamic input at getpath" because
// tryRecordPoly's CORE-dispatch guard required a main-registry builtin, and
// getpath lives in the struct-util sub-registry. The recorder now threads the
// owning sub-registry (via MatchResult.Reg) and records a PolyRef carrying it;
// callPoly re-matches over that registry's signatures. getpath/setpath are pure
// data transforms, value-faithful under runtime re-match.
func TestSubRegistryPolyCompiles(t *testing.T) {
	cases := []struct{ src, want string }{
		// Dynamic input (the setpath result) into getpath — the poly row.
		{`import "boru:struct-util"  StructUtil.getpath $.a.b (StructUtil.setpath $.a.b 7 {a:{b:1}})`, "[7]"},
		// Static-input baseline still compiles (a plain baked CALL_NATIVE).
		{`import "boru:struct-util"  StructUtil.getpath $.a.b {a:{b:1}}`, "[1]"},
		// A dynamic miss returns None — the re-match picks the same overload.
		{`import "boru:struct-util"  StructUtil.getpath $.a.z (StructUtil.setpath $.a.b 7 {a:{b:1}})`, "[None]"},
	}
	for _, c := range cases {
		prog, reason, _, cerr := mustNew(t).CompileCheck(c.src)
		if cerr != nil {
			t.Fatalf("%q: check error %v", c.src, cerr)
		}
		if prog == nil {
			t.Fatalf("%q: expected native compile, declined: %s", c.src, reason)
		}
		if strings.Contains(prog.Disassemble(), "FALLBACK") {
			t.Errorf("%q: expected no island:\n%s", c.src, prog.Disassemble())
		}
		gotC, compiled, eC := mustNew(t).RunCompiled(c.src)
		gotI, eI := mustNew(t).RunInterp(c.src)
		if noteCompileDefect(t, c.src, gotC, eC) {
			continue
		}
		if !compiled {
			t.Errorf("%q: did not run compiled", c.src)
		}
		if eC != nil || eI != nil {
			t.Fatalf("%q: compiled err=%v interp err=%v", c.src, eC, eI)
		}
		if fmt.Sprint(gotC) != fmt.Sprint(gotI) {
			t.Errorf("%q: parity: compiled=%v interp=%v", c.src, gotC, gotI)
		}
		if fmt.Sprint(gotI) != c.want {
			t.Errorf("%q: interp=%v want %s", c.src, gotI, c.want)
		}
	}

	// NEGATIVE: the poly threading must not change a CORE dynamic dispatch — a
	// core word (size) over a dynamic receiver (no sub-registry) still
	// re-matches over the MAIN registry (PolyRef.Reg nil), compiled == interp.
	for _, c := range []struct{ src, want string }{
		{`size (if true ["ab"] ["cde"])`, "[2]"},
	} {
		gotC, _, eC := mustNew(t).RunCompiled(c.src)
		gotI, eI := mustNew(t).RunInterp(c.src)
		if noteCompileDefect(t, c.src, gotC, eC) {
			continue
		}
		if eC != nil || eI != nil {
			t.Fatalf("%q: compiled err=%v interp err=%v", c.src, eC, eI)
		}
		if fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotI) != c.want {
			t.Errorf("%q: core get: compiled=%v interp=%v want %s", c.src, gotC, gotI, c.want)
		}
	}
}

// Stage G — mixed fn-value-call boundary. A dynamic fn value INTERIOR to the
// residual (`3 m.f 2`: forward `2` -> sig[0], stack `3` -> sig[1]) fits neither
// the leading nor the trailing-1-arg apply layout and declined "dynamic value
// precedes residual args". OpCallDynamicMixed islands the whole [before, fn,
// after] window verbatim — the same token sequence the interpreter ran — so the
// fn auto-applies with full fidelity (any arity, callable or not).
func TestMixedFnValueApplyCompiles(t *testing.T) {
	const fn2 = `def m {f: (fn [[a:Integer b:Integer][Integer][(a mul 100) add b]])}  `
	cases := []struct{ src, want string }{
		{fn2 + `3 m.f 2`, "[203]"},                                          // the mixed row: a=2 (forward), b=3 (stack)
		{fn2 + `m.f 2 3`, "[203]"},                                          // two-forward (leading) — unchanged
		{fn2 + `10 m.f 20`, "[2010]"},                                       // a=20 (forward), b=10 (stack)
		{`def m {f: (fn [[a:Integer][Integer][a mul 10]])}  5 m.f`, "[50]"}, // trailing-1 — unchanged
	}
	for _, c := range cases {
		prog, reason, _, cerr := mustNew(t).CompileCheck(c.src)
		if cerr != nil {
			t.Fatalf("%q: check error %v", c.src, cerr)
		}
		if prog == nil {
			t.Fatalf("%q: expected native compile, declined: %s", c.src, reason)
		}
		if strings.Contains(prog.Disassemble(), "FALLBACK") {
			t.Errorf("%q: expected no island:\n%s", c.src, prog.Disassemble())
		}
		gotC, compiled, eC := mustNew(t).RunCompiled(c.src)
		gotI, eI := mustNew(t).RunInterp(c.src)
		if noteCompileDefect(t, c.src, gotC, eC) {
			continue
		}
		if !compiled {
			t.Errorf("%q: did not run compiled", c.src)
		}
		if eC != nil || eI != nil {
			t.Fatalf("%q: compiled err=%v interp err=%v", c.src, eC, eI)
		}
		if fmt.Sprint(gotC) != fmt.Sprint(gotI) {
			t.Errorf("%q: parity: compiled=%v interp=%v", c.src, gotC, gotI)
		}
		if fmt.Sprint(gotI) != c.want {
			t.Errorf("%q: interp=%v want %s", c.src, gotI, c.want)
		}
	}

	// NEGATIVE: an INTERIOR value that is NOT callable must NOT be applied — the
	// island leaves it in place, so the residual is byte-identical to interp.
	// `3 m.g 2` where m.g is a plain integer: the residual stays [3, 42, 2].
	for _, c := range []struct{ src, want string }{
		{`def m {g: 42}  3 m.g 2`, "[3 42 2]"},
	} {
		gotC, _, eC := mustNew(t).RunCompiled(c.src)
		gotI, eI := mustNew(t).RunInterp(c.src)
		if noteCompileDefect(t, c.src, gotC, eC) {
			continue
		}
		if eC != nil || eI != nil {
			t.Fatalf("%q: compiled err=%v interp err=%v", c.src, eC, eI)
		}
		if fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotI) != c.want {
			t.Errorf("%q: non-callable interior: compiled=%v interp=%v want %s", c.src, gotC, gotI, c.want)
		}
	}
}

// Stage (patrun) — a fn VALUE stored in a Patrun dispatch table. The patrun
// `add` overload stashes its value arg and never invokes it on the VM tape, so
// declaring CompileStoresFn lets a PURE fn literal ride as an inert const
// instead of declining "function value reaches add". A genuinely CAPTURING
// closure still declines at isInertConst and falls back faithfully.
func TestPatrunFnValueStoreCompiles(t *testing.T) {
	const mk = `def api (patrun Function)  add {cmd:"sum"} ([m:Map] => [m.x add m.y]) api  `
	// POSITIVE: pure stored fn compiles natively (no island) and dispatches.
	pos := []struct{ src, want string }{
		{mk + `def h (find {cmd:"sum" x:3 y:4} api)  h {x:3 y:4}`, "[7]"},
		{mk, "[]"}, // the add alone (0-result mutation) compiles
	}
	for _, c := range pos {
		prog, reason, _, cerr := mustNew(t).CompileCheck(c.src)
		if cerr != nil {
			t.Fatalf("%q: check error %v", c.src, cerr)
		}
		if prog == nil {
			t.Fatalf("%q: expected native compile, declined: %s", c.src, reason)
		}
		if strings.Contains(prog.Disassemble(), "FALLBACK") {
			t.Errorf("%q: expected no island:\n%s", c.src, prog.Disassemble())
		}
		gotC, compiled, eC := mustNew(t).RunCompiled(c.src)
		gotI, eI := mustNew(t).RunInterp(c.src)
		if noteCompileDefect(t, c.src, gotC, eC) {
			continue
		}
		if !compiled {
			t.Errorf("%q: did not run compiled", c.src)
		}
		if eC != nil || eI != nil {
			t.Fatalf("%q: compiled err=%v interp err=%v", c.src, eC, eI)
		}
		if fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotI) != c.want {
			t.Errorf("%q: compiled=%v interp=%v want %s", c.src, gotC, gotI, c.want)
		}
	}

	// A genuinely CAPTURING closure (captures the enclosing fn param `bse`)
	// stored in a patrun compiles to an OpPushClosure operand — NOT a const
	// bake (a const bake would leave `bse` an unbound Word in the frozen body,
	// the capturing-sink miscompile) — so the stored closure carries bse=100
	// and h {v:5} → 5+100 = 105, compiled == interp.
	capSrc := `def mk fn [[bse:Integer] [Patrun] [def p (patrun Function)  add {cmd:"x"} ([m:Map] => [m.v add bse]) p  p]]  def api (mk 100)  def h (find {cmd:"x" v:5} api)  h {v:5}`
	gotC, capComp, eC := mustNew(t).RunCompiled(capSrc)
	gotI, eI := mustNew(t).RunInterp(capSrc)
	if noteCompileDefect(t, capSrc, gotC, eC) {
		return
	}
	if eC != nil || eI != nil {
		t.Fatalf("capturing: compiled err=%v interp err=%v", eC, eI)
	}
	if !capComp {
		t.Errorf("capturing closure: expected native compile (OpPushClosure operand)")
	}
	if fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotI) != "[105]" {
		t.Errorf("capturing closure: compiled=%v interp=%v want [105]", gotC, gotI)
	}

	// NEGATIVE: arithmetic add is untouched by the patrun overload's flag.
	g2, _, g2Err := mustNew(t).RunCompiled(`add 1 2`)
	if noteCompileDefect(t, `add 1 2`, g2, g2Err) {
		return
	}
	if fmt.Sprint(g2) != "[3]" {
		t.Errorf("arithmetic add: got %v want [3]", g2)
	}
}

// Stage A — multi-value / variadic branch-result modeling (recursion.tsv:53).
// A `[]`-declared fn whose else arm leaves a FIXED value below a runtime-variable
// recursive tail compiles: each frame leaves n*2 below the recursive m result, so
// `m 3` -> [6 4 2]. Verifies compiled == interpreter across the recursion depth.
func TestStageAVariadicBranchResult(t *testing.T) {
	const def = `def m fn [[n:Integer] [] [if (n lte 0) [] [n mul 2 m (n sub 1)]]] `
	for _, c := range []struct {
		arg, want string
	}{
		{"3", "[6 4 2]"},
		{"5", "[10 8 6 4 2]"},
		{"1", "[2]"},
		{"0", "[]"},
	} {
		src := def + "m " + c.arg
		prog, reason, _, cerr := mustNew(t).CompileCheck(src)
		if cerr != nil || prog == nil {
			t.Fatalf("m %s did not compile: reason=%q err=%v", c.arg, reason, cerr)
		}
		if strings.Contains(prog.Disassemble(), "FALLBACK") {
			t.Errorf("m %s: expected a native lowering, got an island", c.arg)
		}
		// Parity AND strict (no fallback masking a VM error).
		got, err := mustNew(t).RunCompiledStrict(src)
		if err != nil {
			t.Fatalf("m %s strict compiled run: %v", c.arg, err)
		}
		if fmt.Sprint(got) != c.want {
			t.Errorf("m %s: compiled %v want %s", c.arg, got, c.want)
		}
		gotI, _ := mustNew(t).RunInterp(src)
		if fmt.Sprint(gotI) != c.want {
			t.Errorf("m %s: interp %v want %s", c.arg, gotI, c.want)
		}
	}
}

// Stage A soundness gate — a VARIADIC fn result must never feed a fixed-arity
// operand (the count is runtime-variable). These must DECLINE (fall back), not
// compile an unsound program: before the gate, `f 3 add 1` diverged
// (internal_error vs the interpreter's signature_error for f 0).
func TestStageAVariadicSoundnessGate(t *testing.T) {
	// Legacy compile failure+fallback-parity contract: pins the one-release
	mustFailToCompile := []string{
		// A 0-or-1 variadic fn result consumed by add.
		`def f fn [[n:Integer] [] [if (n lte 0) [] [n mul 2]]]  f 3 add 1`,
		// The recursive variadic result consumed by add.
		`def m fn [[n:Integer] [] [if (n lte 0) [] [n mul 2 m (n sub 1)]]]  m 3 add 1`,
		// Same, paren form: the variadic result as a forward-collected fixed arg.
		`def m fn [[n:Integer] [] [if (n lte 0) [] [n mul 2 m (n sub 1)]]]  add (m 3) 1`,
		// A value after the variadic recursive call inside the arm.
		`def w fn [[n:Integer] [] [if (n lte 0) [] [n mul 2 w (n sub 1) add 5]]]  w 3`,
	}
	for _, src := range mustFailToCompile {
		prog, _, _, _ := mustNew(t).CompileCheck(src)
		if prog != nil {
			t.Errorf("expected compile failure (variadic→fixed-arity is unsound), but compiled: %s", src)
		}
		// The interpreter is the backstop; RunCompiled falls back and matches it.
		gc, _, ec := mustNew(t).RunCompiled(src)
		gi, ei := mustNew(t).RunInterp(src)
		if noteCompileDefect(t, src, gc, ec) {
			continue
		}
		if fmt.Sprint(gc) != fmt.Sprint(gi) || codeOf(ec) != codeOf(ei) {
			t.Errorf("fallback parity: compiled=%v/%s interp=%v/%s :: %s", gc, codeOf(ec), gi, codeOf(ei), src)
		}
	}
	// GRADUATED 2026-07-15: an all-inert multi-value arm IS reconstructible
	// now — captureInertArmResidual records the arm's operand list and the
	// lowering re-pushes it per taken path, so the merge goes variadic and
	// the PROGRAM RESIDUAL (the one variadic-absorbing consumer) carries it
	// faithfully on either polarity. The variadic->fixed-arity consumers
	// above stay declined — the gate's soundness contract is unchanged.
	for _, c := range []struct{ src, want string }{
		{`if true [1 2 3] [4]`, "[1 2 3]"},
		{`if false [1 2 3] [4]`, "[4]"},
	} {
		got, err := mustNew(t).RunCompiledStrict(c.src)
		if err != nil || fmt.Sprint(got) != c.want {
			t.Errorf("graduated inert-arm merge: compiled %v (err %v), want %s :: %s", got, err, c.want, c.src)
		}
	}
}

// Returned capturing closure + per-iteration dynamic apply (bytecode-combinations.tsv:74).
// A factory fn returns a closure that captures the factory's param; the closure is
// then applied to a trailing arg. Both the top-level immediate apply and the
// per-iteration apply inside a `for` body must compile FULLY NATIVE (the returned
// closure is a ClosurePayload OpCallDynamic invokes VM-natively) and match the
// interpreter.
func TestReturnedCapturingClosureApply(t *testing.T) {
	// Legacy compile failure+fallback-parity contract: pins the one-release
	positive := []struct {
		src  string
		want string
	}{
		// The top-level row `(mk 5) 10` MOVED OUT 2026-08-27: it asserted [15]
		// on "both lanes" while reading gotI from the compiled lane (NUR106).
		// The interpreter places — `(mk 5)` has one survivor — and answers
		// `fn (Integer) 10`; the native compile was a miscompile. It is pinned
		// as a compile failure below. The two per-iteration rows stay positive:
		// a `for` BODY closes through a frame rewind, so both lanes apply.
		// per-iteration apply inside a for body — the landed row.
		{`def mk2 fn [[x:Integer] [Function] [([y:Integer] => [x add y])]]  for 3 [(mk2 i) 10]`, "[10 11 12]"},
		// a multi-arg trailing apply over the per-iteration closure.
		{`def mk4 fn [[x:Integer] [Function] [([a:Integer b:Integer] => [x add a add b])]]  for 3 [(mk4 i) 10 100]`, "[110 111 112]"},
	}
	for _, c := range positive {
		prog, reason, _, cerr := mustNew(t).CompileCheck(c.src)
		if cerr != nil || prog == nil {
			t.Errorf("%q: must compile; reason=%q err=%v", c.src, reason, cerr)
			continue
		}
		if strings.Contains(prog.Disassemble(), "FALLBACK") {
			t.Errorf("%q: must compile native, not island:\n%s", c.src, prog.Disassemble())
			continue
		}
		gotC, compiled, errC := mustNew(t).RunCompiled(c.src)
		gotI, _ := mustNew(t).RunInterp(c.src)
		if noteCompileDefect(t, c.src, gotC, errC) {
			continue
		}
		if !compiled || errC != nil || fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotC) != c.want {
			t.Errorf("%q: parity broke: compiled=%v gotC=%v errC=%v gotI=%v want=%s", c.src, compiled, gotC, errC, gotI, c.want)
		}
	}

	// The relocated top-level row (NUR101): the interpreter PLACES, and as of
	// 2026-08-27 (Stage 3) the compiled lane places it too rather than
	// declining — a capturing factory's result laid out as data.
	mustCompileWithParity(t,
		`def mk fn [[x:Integer] [Function] [([y:Integer] => [x add y])]]  (mk 5) 10`,
		"[fn (Integer) 10]")

	// Negative: a per-iteration body whose trailing apply arg is itself a COMPUTED
	// (event) value is NOT the leading-fn-carrier + re-pushable-args shape — it must
	// DECLINE (the computed arg is already on the sim; seating it would double-push),
	// so the program does not compile, rather than mis-compile.
	negative := []string{
		`def mk fn [[x:Integer] [Function] [([y:Integer] => [x add y])]]  for 3 [(mk i) (add i 1)]`,
	}
	for _, src := range negative {
		prog, _, _, _ := mustNew(t).CompileCheck(src)
		if prog != nil && !strings.Contains(prog.Disassemble(), "FALLBACK") {
			t.Errorf("%q: a computed apply-arg must NOT compile to a native per-iteration apply:\n%s", src, prog.Disassemble())
		}
		gc, _, ec := mustNew(t).RunCompiled(src)
		gi, ei := mustNew(t).RunInterp(src)
		if noteCompileDefect(t, src, gc, ec) {
			continue
		}
		if fmt.Sprint(gc) != fmt.Sprint(gi) || codeOf(ec) != codeOf(ei) {
			t.Errorf("fallback parity: compiled=%v/%s interp=%v/%s :: %s", gc, codeOf(ec), gi, codeOf(ei), src)
		}
	}
}

// TestParseLangFnValueDispatchCompiles pins body-bearing fn-VALUE dispatch
// for a def-bound boru parser (module-parselang:23): the bound parser fn,
// called with args — directly or through the `parse` sugar — lowers to a
// CALL_USER unit (its `__pa` tail captured INSIDE the unit) instead of
// leaking `__pa` into the top-level residual. (The former sub-feature A,
// check-time registration, died with the frozen kind namespace — the
// negative below pins the tombstone's cross-engine parity instead.)
func TestParseLangFnValueDispatchCompiles(t *testing.T) {
	const reg = `"boru:parselang" import end  "boru:string-util" import end  ` +
		`def calc (fn [[source:Any opts:Map] [List] [StringUtil.split ' ' (ParseLang.source source)]])  `

	// POSITIVE: the desugared direct call (row 23) and the `parse` sugar
	// (row 16) compile, run through the VM, and match the interpreter.
	for _, c := range []struct{ src, want string }{
		{reg + `(calc 'x + y' {} end) get 1`, "[+]"},
		{reg + `(calc 'x + y' {} end) get 0`, "[x]"},
		{reg + `(parse calc 'x + y') get 1`, "[+]"},
	} {
		prog, reason, _, cerr := mustNew(t).CompileCheck(c.src)
		if cerr != nil {
			t.Fatalf("%q: check error %v", c.src, cerr)
		}
		if prog == nil {
			t.Fatalf("%q: expected compile, declined: %s", c.src, reason)
		}
		if strings.Contains(prog.Disassemble(), "FALLBACK") {
			t.Errorf("%q: expected no island:\n%s", c.src, prog.Disassemble())
		}
		if !strings.Contains(prog.Disassemble(), "CALL_USER") {
			t.Errorf("%q: expected the parser body to lower to a CALL_USER unit:\n%s", c.src, prog.Disassemble())
		}
		gotC, compiled, eC := mustNew(t).RunCompiled(c.src)
		gotI, eI := mustNew(t).RunInterp(c.src)
		if noteCompileDefect(t, c.src, gotC, eC) {
			continue
		}
		if !compiled {
			t.Errorf("%q: did not run compiled", c.src)
		}
		if eC != nil || eI != nil {
			t.Fatalf("%q: compiled err=%v interp err=%v", c.src, eC, eI)
		}
		if fmt.Sprint(gotC) != fmt.Sprint(gotI) {
			t.Errorf("%q: parity: compiled=%v interp=%v", c.src, gotC, gotI)
		}
		if fmt.Sprint(gotI) != c.want {
			t.Errorf("%q: interp=%v want %s", c.src, gotI, c.want)
		}
	}

	// NEGATIVE: the registration surface is a TOMBSTONE — `ParseLang.register`
	// raises parse_registry_frozen identically in BOTH engines.
	dbl := `"boru:parselang" import end  ParseLang.register`
	_, _, eC := mustNew(t).RunCompiled(dbl)
	_, eI := mustNew(t).RunInterp(dbl)
	if noteCompileDefect(t, dbl, nil, eC) {
		return
	}
	if codeOf(eC) != "parse_registry_frozen" {
		t.Errorf("register tombstone compiled: code=%q want parse_registry_frozen (err=%v)", codeOf(eC), eC)
	}
	if codeOf(eI) != "parse_registry_frozen" {
		t.Errorf("register tombstone interp: code=%q want parse_registry_frozen (err=%v)", codeOf(eI), eI)
	}
}

func TestRandCarrierReceiverClosureCompiles(t *testing.T) {
	clk := capabilities.FixedClock{T: time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)}
	newClocked := func() *Boru {
		a := mustNew(t)
		a.SetClock(clk)
		return a
	}

	// POSITIVE: the seeded-instance closure-word form compiles to PUSH_CLOSURE,
	// is island-free, and is byte-identical to the interpreter across seeds.
	for _, seed := range []int{0, 1, 2, 3, 7, 42, 99, 123} {
		src := fmt.Sprintf(
			`"boru:rand" import end  def r (Rand.with-seed %d)  r.list-of [Rand.int 0 10] 3`, seed)
		prog, reason, _, cerr := newClocked().CompileCheck(src)
		if cerr != nil {
			t.Fatalf("seed %d: check error %v", seed, cerr)
		}
		if prog == nil {
			t.Fatalf("seed %d: expected compile, declined: %s", seed, reason)
		}
		dis := prog.Disassemble()
		if !strings.Contains(dis, "PUSH_CLOSURE") {
			t.Errorf("seed %d: expected PUSH_CLOSURE (closure-word dispatch):\n%s", seed, dis)
		}
		if strings.Contains(dis, "FALLBACK") {
			t.Errorf("seed %d: expected no interpreter island:\n%s", seed, dis)
		}
		gotC, compiled, eC := newClocked().RunCompiled(src)
		gotI, eI := newClocked().RunInterp(src)
		if noteCompileDefect(t, src, gotC, eC) {
			continue
		}
		if !compiled {
			t.Errorf("seed %d: did not run compiled", seed)
		}
		if eC != nil || eI != nil {
			t.Fatalf("seed %d: compiled err=%v interp err=%v", seed, eC, eI)
		}
		if fmt.Sprint(gotC) != fmt.Sprint(gotI) {
			t.Errorf("seed %d: parity: compiled=%v interp=%v", seed, gotC, gotI)
		}
	}

	// POSITIVE: the spec row's exact expected value at the spec clock+seed.
	{
		const src = `"boru:rand" import end  def r (Rand.with-seed 2)  r.list-of [Rand.int 0 10] 3`
		gotC, compiled, eC := newClocked().RunCompiled(src)
		if noteCompileDefect(t, src, gotC, eC) {
			return
		}
		if !compiled || eC != nil {
			t.Fatalf("row 38: compiled=%v err=%v", compiled, eC)
		}
		if fmt.Sprint(gotC) != "[[7 5 4]]" {
			t.Errorf("row 38: compiled=%v want [[7 5 4]]", gotC)
		}
	}

	// NEGATIVE: a RECEIVER-bound body (`r.int`) is seed-specific. It must NOT be
	// baked as a closure-word const-FROZEN draw (the receiver `r` baked as a
	// snapshot const, so every draw repeats). It MAY compile to a closure whose
	// body reads the receiver via LOOKUP_DYN_SCOPE — the enclosing module-scope
	// `def r` binding routes through the dynamic-scope path (a computed enclosing
	// binding read inside a fn/closure unit), so the VM re-resolves the LIVE
	// by-reference carrier per draw exactly as the interpreter does. The frozen-
	// bake shape is PUSH_CLOSURE with the receiver as a const and NO dynamic
	// resolution (neither CALL_DYNAMIC nor LOOKUP_DYN_SCOPE); the parity loop
	// below is the real correctness contract (byte-identical at every seed).
	for _, seed := range []int{2, 5, 42} {
		src := fmt.Sprintf(
			`"boru:rand" import end  def r (Rand.with-seed %d)  r.list-of [r.int 0 10] 3`, seed)
		prog, _, _, _ := newClocked().CompileCheck(src)
		if prog != nil && strings.Contains(prog.Disassemble(), "PUSH_CLOSURE") &&
			!strings.Contains(prog.Disassemble(), "CALL_DYNAMIC") &&
			!strings.Contains(prog.Disassemble(), "LOOKUP_DYN_SCOPE") {
			t.Errorf("seed %d: r.int body wrongly baked as a frozen closure draw:\n%s",
				seed, prog.Disassemble())
		}
		gotC, _, eC := newClocked().RunCompiled(src)
		gotI, eI := newClocked().RunInterp(src)
		if noteCompileDefect(t, src, gotC, eC) {
			continue
		}
		if eC != nil || eI != nil {
			t.Fatalf("seed %d (recv-bound): compiled err=%v interp err=%v", seed, eC, eI)
		}
		if fmt.Sprint(gotC) != fmt.Sprint(gotI) {
			t.Errorf("seed %d (recv-bound): parity: compiled=%v interp=%v", seed, gotC, gotI)
		}
	}
}

// TestDeferredListBodyCompiles pins the LAST compiler compile failure cleared
// (def-node-binding.tsv:54), reaching failures = 0. A fn whose body is a single
// deferred list literal that references a parameter — `def mk fn [[c1:Integer]
// [List] [[c1]]]` — returns the list RAW; the interpreter auto-evaluates it LATE,
// in MODULE scope (a returned list never closes over the param — only a `=>`
// lambda captures), so `c1` resolves to the module binding, not the arg. The fn
// is compiled TRANSPARENTLY: the raw deferred list rides as the call's residual
// and the existing top-level deferred-list fold bakes it in module scope. Every
// consumption shape must fold to the SAME value both engines produce — no fn unit,
// no VM change.
func TestDeferredListBodyCompiles(t *testing.T) {
	const mk = `def c1 1 def mk ([c1:Integer] => [[c1]]) `
	positive := []struct{ src, want string }{
		// Bare top-level: the deferred list folds at end-of-run, module c1 = 1.
		{mk + `mk 9`, "[[1]]"},
		// Rebind AFTER the bare call: the late fold sees the LATEST module binding.
		{mk + `mk 9 def c1 2`, "[[2]]"},
		// def-bind forces the eval at bind time (module c1 = 1), so a later rebind
		// does not move it.
		{mk + `def r (mk 9) def c1 2 r`, "[[1]]"},
		// Consumed downstream: the deferred list folds in module scope at the
		// consumer, exactly as the interpreter auto-evaluates the arg.
		{mk + `mk 9 0 get`, "[1]"},
		{mk + `mk 9 size`, "[1]"},
		// Two independent calls.
		{mk + `mk 9 mk 8`, "[[1] [1]]"},
		// A different module binding flows through.
		{`def c1 7 def mk ([c1:Integer] => [[c1]]) mk 9`, "[[7]]"},
	}
	for _, c := range positive {
		prog, reason, _, cerr := mustNew(t).CompileCheck(c.src)
		if cerr != nil || prog == nil {
			t.Fatalf("did not compile: reason=%q err=%v :: %s", reason, cerr, c.src)
		}
		if strings.Contains(prog.Disassemble(), "FALLBACK") {
			t.Errorf("expected a native lowering, got an island: %s", c.src)
		}
		got, err := mustNew(t).RunCompiledStrict(c.src)
		if err != nil {
			t.Fatalf("strict compiled run: %v :: %s", err, c.src)
		}
		if fmt.Sprint(got) != c.want {
			t.Errorf("compiled %v want %s :: %s", got, c.want, c.src)
		}
		gotI, _ := mustNew(t).RunInterp(c.src)
		if fmt.Sprint(gotI) != c.want {
			t.Errorf("interp %v want %s :: %s", gotI, c.want, c.src)
		}
	}

	// NEGATIVE / soundness — a returned list is NOT a closure: it must resolve in
	// MODULE scope, never the param. The `=>` lambda (which DOES capture) is the
	// contrast and must still return the captured param. Both must match the
	// interpreter exactly.
	contrast := []struct{ src, want string }{
		// `=>` captures the param: f returns the ARG 7, not a module binding.
		{`def mk fn [[c1:Integer] [Function] [([] => [c1])]] def f (mk 7) f`, "[7]"},
		// No module binding for the named param: the deferred list errors at the
		// late module-scope eval (undefined word), exactly as the interpreter does.
		{`def mk ([x:Integer] => [[x add 1]]) mk 5`, ""},
	}
	for _, c := range contrast {
		gotC, _, eC := mustNew(t).RunCompiled(c.src)
		gotI, eI := mustNew(t).RunInterp(c.src)
		if noteCompileDefect(t, c.src, gotC, eC) {
			continue
		}
		if codeOf(eC) != codeOf(eI) {
			t.Errorf("error parity: compiled=%s interp=%s :: %s", codeOf(eC), codeOf(eI), c.src)
		}
		if fmt.Sprint(gotC) != fmt.Sprint(gotI) {
			t.Errorf("value parity: compiled=%v interp=%v :: %s", gotC, gotI, c.src)
		}
		if eC == nil && c.want != "" && fmt.Sprint(gotC) != c.want {
			t.Errorf("compiled %v want %s :: %s", gotC, c.want, c.src)
		}
	}
}

// `inspect` keeps CompileQuoteInert (a bare-word name quotes to an inert
// Atom const, so the dispatch bakes a plain CALL_NATIVE); `has` left the
// quoting family on 2026-08-21 (its key evaluates like get's), so its rows
// here spell the atom keys explicitly with /q — inert consts either way,
// still compiling natively with no island.
func TestQuotedOperandHasInspectCompiles(t *testing.T) {
	pos := []struct{ src, want string }{
		{`{a:1} has b/q`, "[false]"},
		{`{a:None} has a/q`, "[true]"},
		{`none has a/q`, "[false]"},
		{`inspect a/q`, "[{name:'a' kind:unknown signatures:[]}]"},
	}
	for _, c := range pos {
		prog, reason, _, cerr := mustNew(t).CompileCheck(c.src)
		if cerr != nil {
			t.Fatalf("%q: check error %v", c.src, cerr)
		}
		if prog == nil {
			t.Fatalf("%q: expected native compile, declined: %s", c.src, reason)
		}
		if strings.Contains(prog.Disassemble(), "FALLBACK") {
			t.Errorf("%q: expected no island:\n%s", c.src, prog.Disassemble())
		}
		gotC, compiled, eC := mustNew(t).RunCompiled(c.src)
		gotI, eI := mustNew(t).RunInterp(c.src)
		if noteCompileDefect(t, c.src, gotC, eC) {
			continue
		}
		if !compiled {
			t.Errorf("%q: did not run compiled", c.src)
		}
		if eC != nil || eI != nil {
			t.Fatalf("%q: compiled err=%v interp err=%v", c.src, eC, eI)
		}
		if fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotI) != c.want {
			t.Errorf("%q: compiled=%v interp=%v want %s", c.src, gotC, gotI, c.want)
		}
	}

	// NEGATIVE: the evaluated-key overloads (no QuoteArgs) are unaffected — a
	// String key still dispatches the same handler, compiled == interp.
	for _, c := range []struct{ src, want string }{
		{`{a:1} has "a"`, "[true]"},
		{`{a:1} has "z"`, "[false]"},
	} {
		gotC, _, eC := mustNew(t).RunCompiled(c.src)
		gotI, eI := mustNew(t).RunInterp(c.src)
		if noteCompileDefect(t, c.src, gotC, eC) {
			continue
		}
		if eC != nil || eI != nil {
			t.Fatalf("%q: compiled err=%v interp err=%v", c.src, eC, eI)
		}
		if fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotI) != c.want {
			t.Errorf("%q: compiled=%v interp=%v want %s", c.src, gotC, gotI, c.want)
		}
	}
}

// do over a MAP (corpus-core.tsv:60) is a pure value-eval whose arg is
// auto-evaluated BEFORE the handler — unlike the NoEvalArgs LIST body it does
// not re-enter the interpreter, so it bakes a plain CALL_NATIVE instead of an
// OpFallback island. CompileFallbackBody moved from the word to the List sig.
func TestDoMapCompilesNoIsland(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`do {a:1,b:2}`, "[{a:1 b:2}]"},
		{`do {a:(1 add 2) b:3}`, "[{a:3 b:3}]"},
	} {
		prog, reason, _, cerr := mustNew(t).CompileCheck(c.src)
		if cerr != nil {
			t.Fatalf("%q: check error %v", c.src, cerr)
		}
		if prog == nil {
			t.Fatalf("%q: declined: %s", c.src, reason)
		}
		if strings.Contains(prog.Disassemble(), "FALLBACK") {
			t.Errorf("%q: expected no island:\n%s", c.src, prog.Disassemble())
		}
		gotC, _, eC := mustNew(t).RunCompiled(c.src)
		gotI, eI := mustNew(t).RunInterp(c.src)
		if noteCompileDefect(t, c.src, gotC, eC) {
			continue
		}
		if eC != nil || eI != nil {
			t.Fatalf("%q: compiled err=%v interp err=%v", c.src, eC, eI)
		}
		if fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotI) != c.want {
			t.Errorf("%q: compiled=%v interp=%v want %s", c.src, gotC, gotI, c.want)
		}
	}
	// NEGATIVE: the LIST (code-body) sig is unaffected by the Map-sig change —
	// it still compiles its body and runs compiled == interp.
	gotC, _, eC := mustNew(t).RunCompiled(`do [1 add 2]`)
	gotI, eI := mustNew(t).RunInterp(`do [1 add 2]`)
	if noteCompileDefect(t, `do [1 add 2]`, gotC, eC) {
		return
	}
	if eC != nil || eI != nil {
		t.Fatalf("do [body]: compiled err=%v interp err=%v", eC, eI)
	}
	if fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotI) != "[3]" {
		t.Errorf("do [1 add 2]: compiled=%v interp=%v want [3]", gotC, gotI)
	}
}

// outer [body] left right (corpus-core.tsv:101) — the last interpreter island.
// `outer` is a code-body higher-order word that used to run its body via an
// inline sub-engine and island. Routing it through the InvokeBody seam + a
// CallableSpec compiles the `[mul]` body to a closure unit the VM drives per
// pair, so the 2D outer product compiles fully native (islands → 0).
func TestOuterCompilesNoIsland(t *testing.T) {
	src := `outer [mul] [1 2] [3 4]`
	prog, reason, _, cerr := mustNew(t).CompileCheck(src)
	if cerr != nil {
		t.Fatalf("check error %v", cerr)
	}
	if prog == nil {
		t.Fatalf("declined: %s", reason)
	}
	if strings.Contains(prog.Disassemble(), "FALLBACK") {
		t.Errorf("expected no island:\n%s", prog.Disassemble())
	}
	gotC, compiled, eC := mustNew(t).RunCompiled(src)
	gotI, eI := mustNew(t).RunInterp(src)
	if noteCompileDefect(t, src, gotC, eC) {
		return
	}
	if !compiled {
		t.Errorf("did not run compiled")
	}
	if eC != nil || eI != nil {
		t.Fatalf("compiled err=%v interp err=%v", eC, eI)
	}
	if fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotI) != "[[[3 4] [6 8]]]" {
		t.Errorf("compiled=%v interp=%v want [[[3 4] [6 8]]]", gotC, gotI)
	}
	// NEGATIVE: a non-concrete-list arg still errors (outer_error), not a crash.
	_, _, eBad := mustNew(t).RunCompiled(`outer [mul] List [3 4]`)
	_, eBadI := mustNew(t).RunInterp(`outer [mul] List [3 4]`)
	if noteCompileDefect(t, `outer [mul] List [3 4]`, nil, eBad) {
		return
	}
	if (eBad == nil) != (eBadI == nil) {
		t.Errorf("outer over a type-literal list: compiled err=%v interp err=%v (should agree)", eBad, eBadI)
	}
}

// Mechanism A (design/legacy/MISCOMPILE-HUNT-FINDINGS.0.ignore §A) — a compound VALUE
// literal in a fn body is re-constructed per call by the interpreter, so
// compiled code must not leak one pooled identity across calls
// (OpPushConstFresh). Reads of one per-call binding still share within a
// call; an enclosing binding's value keeps its one shared instance; an
// escaping multi-read literal declines (declined, then interpreted).
func TestFnBodyContainerLiteralIdentity(t *testing.T) {
	parity := []struct{ name, src string }{
		{"list literal returned", `def mk fn [[] [List] [[1]]] ((mk) eq (mk))`},
		{"map literal returned", `def mk fn [[] [Map] [{}]] ((mk) eq (mk))`},
		{"def-bound literal returned", `def mk fn [[] [List] [def a [1] a]] ((mk) eq (mk))`},
		{"branch-arm literal returned", `def mk fn [[b:Boolean] [List] [if b [[1]] [[2]]]] ((mk true) eq (mk true))`},
		{"nested literal inner identity", `def mk fn [[] [List] [[[1]]]] (((mk) get 0) eq ((mk) get 0))`},
		{"enclosing binding stays shared", `def c [9]  def get fn [[] [List] [c]]  ((get) eq (get))`},
		{"within-call reads share", `def mk fn [[] [Boolean] [def a [1] (a eq a)]] (mk)`},
		{"value equality unchanged", `def mk fn [[] [List] [[1]]] ((mk) deq (mk))`},
	}
	for _, c := range parity {
		gotC, compiled, errC := mustNew(t).RunCompiled(c.src)
		if noteCompileDefect(t, c.src, gotC, errC) {
			continue
		}
		if errC != nil || !compiled {
			t.Fatalf("%s: compiled run: compiled=%v err=%v", c.name, compiled, errC)
		}
		gotI, errI := mustNew(t).RunInterp(c.src)
		if errI != nil {
			t.Fatalf("%s: interp run: %v", c.name, errI)
		}
		if fmt.Sprint(gotC) != fmt.Sprint(gotI) {
			t.Errorf("%s: compiled=%v interp=%v", c.name, gotC, gotI)
		}
	}

	// A multi-read body literal in an escape-capable fn now COMPILES via the
	// per-call local seat (OpPushConstFreshLocal): one construction per call,
	// seated in a frame local and shared by every read — exactly the
	// interpreter's `def a {…}` semantics. Two soundness properties, each of
	// which a wrong lowering fails:
	//   - WITHIN a call the reads share ONE instance (`a eq a` is true;
	//     a per-site OpPushConstFresh would mint two distinct lists → false);
	//   - ACROSS calls the instance is fresh (`(mk) eq (mk)` stays false;
	//     a shared pooled const would leak one identity → true).
	seat := []struct{ name, src, want string }{
		{"multi-read escaping literal compiles", `def mk fn [[] [List] [def a [1] def _ (a eq a) a]] (mk)`, "[[1]]"},
		{"within-call reads are one instance", `def mk fn [[] [Boolean] [def a [1] def r (a eq a) def _ a r]] (mk)`, "[true]"},
		{"across-call instances are fresh", `def mk fn [[] [List] [def a [1] def _ (a eq a) a]] ((mk) eq (mk))`, "[false]"},
	}
	for _, c := range seat {
		prog, reason, _, cerr := mustNew(t).CompileCheck(c.src)
		if prog == nil || cerr != nil {
			t.Fatalf("%s: expected a native compile, declined: reason=%q err=%v", c.name, reason, cerr)
		}
		gotC, compiled, errC := mustNew(t).RunCompiled(c.src)
		if noteCompileDefect(t, c.src, gotC, errC) {
			continue
		}
		if errC != nil || !compiled {
			t.Fatalf("%s: compiled run: compiled=%v err=%v", c.name, compiled, errC)
		}
		gotI, errI := mustNew(t).RunInterp(c.src)
		if errI != nil {
			t.Fatalf("%s: interp run: %v", c.name, errI)
		}
		if fmt.Sprint(gotC) != fmt.Sprint(gotI) {
			t.Errorf("%s: parity compiled=%v interp=%v", c.name, gotC, gotI)
		}
		if fmt.Sprint(gotC) != c.want {
			t.Errorf("%s: got %v, want %s", c.name, gotC, c.want)
		}
	}
}

// Mechanism E remainders (design/legacy/MISCOMPILE-HUNT-FINDINGS.0.ignore). The
// deferred-field auto-invoke family GRADUATED (the break-2 closure,
// FN-VALUE-OPEN-WORK §4): a pinpointed genuine-0-arg member read compiles
// as an arity-0 OpCallDynMethod — its rows moved to `preserved` below. A
// 0-arg landing the arrival model cannot claim still DECLINES with the
// guard's own reason (declineMemberFnArrival). The nested-factory curried
// chain that declined beside them GRADUATED 2026-09-22 (the curried
// chain: each paren records its re-stepped produced lead's apply at the
// collapse) — TestCurriedChainParity carries the row.
func TestFnValueAutoApplyCompileFailures(t *testing.T) {
	// Legacy compile failure+fallback-parity contract: pins the one-release
	failures := []struct{ name, src, want string }{
		{"multi-overload 0-arg member", `def z fn [[] [Integer] [42] [n:Integer] [Integer] [n]]  def m {k: z/v}  (m.k)`, "auto-dispatches"},
		// The read guard's OWN remaining territory after the break-2 closure:
		// a tracked instance read whose KEY does not resolve statically. The
		// risk is known per-instance (instanceFnFieldRisk's unresolvable-key
		// arm reports any tracked field) but the member is not pinpointed
		// (instanceFnMember reports nothing without a concrete key), so no
		// landing model owns it and the guard declines — the division of
		// labour instanceFnMember's own comment states.
		{"unpinpointable instance key", `def make42 fn [[] [Integer] [42]] def C class {fld:Function} def o (make C {fld:make42/v}) def keyof fn [[n:Integer] [Atom] [if (n gt 0) [fld/q] [other/q]]] (o get (keyof 1))`, "auto-dispatches"},
	}
	for _, c := range failures {
		prog, reason, _, _ := mustNew(t).CompileCheck(c.src)
		if prog != nil {
			t.Errorf("%s: compiled; want compile failure", c.name)
			continue
		}
		if !strings.Contains(reason, c.want) {
			t.Errorf("%s: compile failure reason %q; want substring %q", c.name, reason, c.want)
		}
		// The silent-fallback path must produce the interpreter's value.
		gotC, _, errC := mustNew(t).RunCompiled(c.src)
		gotI, errI := mustNew(t).RunInterp(c.src)
		if noteCompileDefect(t, c.src, gotC, errC) {
			continue
		}
		if errC != nil || errI != nil {
			t.Fatalf("%s: run errs compiled=%v interp=%v", c.name, errC, errI)
		}
		if fmt.Sprint(gotC) != fmt.Sprint(gotI) {
			t.Errorf("%s: fallback=%v interp=%v", c.name, gotC, gotI)
		}
	}

	// The UNWRAPPED twin of the preserved `((mk 5) 10)` row: no enclosing
	// rewind, so the interpreter PLACES and the compiler must not apply
	// (NUR101, design/PAREN-RESTEP-RULE.0.md). GRADUATED to a parity row
	// 2026-08-27 (Stage 3) — placing it is now something the compiled lane
	// can DO, not merely something it declines to get wrong.
	mustCompileWithParity(t, `def mk fn [[a:Integer] [Function] [([b:Integer] => [a add b])]]  (mk 5) 10`, "[fn (Integer) 10]")

	// PRESERVED coverage: fn-free container reads (paren or bare), APPLIED
	// member calls (a multi-param member fed its args — the method-through-map
	// pattern, whose phantom parked 0-arg sig must NOT read as auto-dispatch
	// risk), the single-apply factory, and the GRADUATED break-2 family —
	// pinpointed genuine-0-arg member reads whose courtesy dispatch now
	// compiles as an arity-0 OpCallDynMethod — all keep compiling.
	preserved := []string{
		`({a:1 b:2} get b/q) add 1`,
		`{a:1 b:2} dot b`,
		`def add1 fn [[x:Integer] [Integer] [x add 1]]  {f:add1/v}.f 5`,
		`def mk fn [[a:Integer] [Function] [([b:Integer] => [a add b])]]  ((mk 5) 10)`,
		`def make42 fn [[] [Integer] [42]]  {f:make42/v}.f`,
		`def make42 fn [[] [Integer] [42]]  def m {f:make42/v}  (m get f/q)`,
		`def f fn [[] [Integer] [7]]  {b:f/v} dot b`,
	}
	for _, src := range preserved {
		gotC, compiled, errC := mustNew(t).RunCompiled(src)
		if noteCompileDefect(t, src, gotC, errC) {
			continue
		}
		if errC != nil || !compiled {
			t.Fatalf("preserved %q: compiled=%v err=%v", src, compiled, errC)
		}
		gotI, errI := mustNew(t).RunInterp(src)
		if errI != nil {
			t.Fatalf("preserved %q: interp err=%v", src, errI)
		}
		if fmt.Sprint(gotC) != fmt.Sprint(gotI) {
			t.Errorf("preserved %q: compiled=%v interp=%v", src, gotC, gotI)
		}
	}
}

// Stage E landing (corpus-core.tsv:50, the last tier-2 row) — core `walk`'s
// hooks compile through the closure seam. The descend hook (quotation or
// lambda — LambdaSharesTokenShape, one `{key value path parent depth}` payload
// input) compiles to a closure unit walkClassifyHook drives via InvokeBody; a
// module-scope flex accumulator read in the hook body rides as a closure
// capture (moduleScopeMutableCaptures, now applied to LAMBDA bodies too), so
// the compiled body mutates the SAME pointer-backed FlexList cell the frame
// local holds. The optional ASCEND slot admits exactly one value-operand
// shape: a flex proven EMPTY at dispatch (emptyFlexHookOperand), whose
// classify-time token snapshot is a zero-length list forever — every other
// ascend shape keeps its compile failure (compile failure).
func TestWalkHookClosureCompiles(t *testing.T) {
	// Legacy compile failure+fallback-parity contract: pins the one-release
	parity := []struct{ name, src, want string }{
		{"tier-2 corpus row (empty-flex ascend consumed as 4th arg)",
			`def acc (flex [])  walk {mode: "depth"} {a:1 b:[2 3]} (m:Any => [acc (m.path) append])  acc`,
			"[{a:1 b:[2 3]}]"},
		{"mutation then read-back (second acc reads the mutated cell)",
			`def acc (flex [])  walk {mode: "depth"} {a:1 b:[2 3]} (m:Any => [acc (m.path) append])  acc  acc`,
			"[{a:1 b:[2 3]} ['' 'a' 'b' 'b.0' 'b.1']]"},
		{"paren-bounded 3-arg walk, then read the mutated cell",
			`def acc (flex [])  (walk {mode: "breadth"} {a:1 b:[2 3]} (m:Any => [acc (m.path) append]))  acc`,
			"[{a:1 b:[2 3]} ['' 'a' 'b' 'b.0' 'b.1']]"},
		{"loop-iteration mutation accumulates across iterations",
			`def acc (flex [])  for 2 [ (walk {mode: "depth"} {a:1} (m:Any => [acc (m.path) append])) drop ]  acc`,
			"[['' 'a' '' 'a']]"},
		{"token-quotation hook compiles as a closure",
			`walk {mode: "depth"} {a:1} [drop]`,
			"[{a:1}]"},
		{"two-lambda 4-arg walk (ascend LAMBDA compiles to its own closure unit — Stage M2d, corpus-core.tsv:134)",
			`def acc (flex [])  walk {mode: "depth"} {a:{x:1}} (m:Any => [acc (m.path) append]) (m:Any => [acc (m.path) append])  acc`,
			"[{a:{x:1}} ['' 'a' 'a.x' 'a.x' 'a' '']]"},
		{"each map-lambda with a module-scope flex capture (shared admission)",
			`def acc (flex [])  each ([kv:Any] => [acc (kv.v) append]) {a:1 b:2}  drop  acc`,
			"[[1 2]]"},
	}
	for _, c := range parity {
		prog, reason, _, cerr := mustNew(t).CompileCheck(c.src)
		if cerr != nil {
			t.Fatalf("%s: check error %v", c.name, cerr)
		}
		if prog == nil {
			t.Fatalf("%s: declined: %s", c.name, reason)
		}
		if strings.Contains(prog.Disassemble(), "FALLBACK") {
			t.Errorf("%s: expected no island:\n%s", c.name, prog.Disassemble())
		}
		gotC, compiled, errC := mustNew(t).RunCompiled(c.src)
		gotI, errI := mustNew(t).RunInterp(c.src)
		if noteCompileDefect(t, c.src, gotC, errC) {
			continue
		}
		if !compiled {
			t.Errorf("%s: did not run compiled", c.name)
		}
		if errC != nil || errI != nil {
			t.Fatalf("%s: run errs compiled=%v interp=%v", c.name, errC, errI)
		}
		if fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotI) != c.want {
			t.Errorf("%s: compiled=%v interp=%v want %s", c.name, gotC, gotI, c.want)
		}
	}

	// A flex MAP admitted as the ascend slot ({} is empty too) reaches the
	// handler, whose classification raises walk_error — the compiled path
	// surfaces the byte-identical taxonomy from the VM, no fallback mask.
	{
		src := `def acc (flex {})  walk {mode: "depth"} {a:1} (m:Any => [m.path drop])  acc`
		_, compiled, errC := mustNew(t).RunCompiled(src)
		_, errI := mustNew(t).RunInterp(src)
		if noteCompileDefect(t, src, nil, errC) {
			return
		}
		if !compiled {
			t.Errorf("flex-map ascend: did not run compiled")
		}
		if codeOf(errC) != "walk_error" || codeOf(errC) != codeOf(errI) {
			t.Errorf("flex-map ascend: compiled err=[%s] interp err=[%s]; want walk_error on both", codeOf(errC), codeOf(errI))
		}
	}

	// The neighbouring ascend shapes whose runtime tokens are NOT provably
	// empty used to DECLINE (the code-body compile failure). Since walk
	// declares CompileDynBody (2026-09-25, the fn-value callback cells) the
	// closure path's decline falls to the dyn-body seat: the walk lowers to
	// a CALL_NATIVE under DynEnv, the flex ascend rides as the runtime value
	// it is, and the handler classifies the hooks as the interpreter's walk
	// does — so these compile and agree, the hook-type mismatch raising the
	// same walk_error on both lanes.
	for _, src := range []string{
		`def acc (flex ["s"])  walk {mode: "depth"} {a:1} (m:Any => [acc (m.path) append])  acc`,
		`def acc (flex [])  def z (push "x" acc)  walk {mode: "depth"} {a:1} (m:Any => [acc (m.path) append])  acc`,
		`def acc (flex [])  for 2 [ walk {mode: "depth"} {a:1} (m:Any => [acc (m.path) append]) acc drop ]  acc`,
		`walk {mode: "depth"} {a:1} (s:String => [s drop])`,
	} {
		requireEngineParity(t, src, true)
	}
	// NEGATIVES — the ascend shape that still DECLINES (the compile
	// failure), and the fallback must agree with the interpreter.
	failures := []struct{ name, src, want string }{
		// (The two-lambda ascend shape moved to the PARITY table above at
		// Stage M2d: the ascend lambda now compiles to its own closure unit,
		// per design/legacy/STAGE3-INLINING-DESIGN-ROUND.0.ignore M2d.)
		{"capturing ascend lambda (lexical capture — stays declined)",
			`def f fn [[p:String] [Map] [walk {mode: "depth"} {a:1} (m:Any => [m.path drop]) (m:Any => [p drop])]] f "s"`,
			"code-body word walk"},
	}
	for _, c := range failures {
		prog, reason, _, _ := mustNew(t).CompileCheck(c.src)
		if prog != nil {
			t.Fatalf("%s: compiled; want compile failure", c.name)
		}
		if !strings.Contains(reason, c.want) {
			t.Errorf("%s: compile failure reason %q; want substring %q", c.name, reason, c.want)
		}
		gotC, _, errC := mustNew(t).RunCompiled(c.src)
		gotI, errI := mustNew(t).RunInterp(c.src)
		if noteCompileDefect(t, c.src, gotC, errC) {
			continue
		}
		if (errC == nil) != (errI == nil) || codeOf(errC) != codeOf(errI) {
			t.Fatalf("%s: fallback err=[%s] interp err=[%s] (should agree)", c.name, codeOf(errC), codeOf(errI))
		}
		if fmt.Sprint(gotC) != fmt.Sprint(gotI) {
			t.Errorf("%s: fallback=%v interp=%v", c.name, gotC, gotI)
		}
	}
}

// Error-row trap programs (finish-line cluster 1, the 83-row burn-down): a
// STATICALLY-DEFINITE unmatched dispatch — matchSignature failed and every
// value any candidate signature examined is identical at run time (concrete
// consts, bare type literals, raw word tokens; no carrier / dynamic /
// deferred-expression operand) — compiles to a terminal OpTrap raising the
// interpreter's byte-identical error instead of declining the whole program
// (engine.go tryRecordUnmatchedDispatchTrap). The trap mirrors the
// interpreter's raise exactly: the void-argument-group override (def_error /
// no_value_error) when a paren arg produced no value, else the plain
// signature_error "no matching signature for <word>".
func TestUnmatchedDispatchTrapCompiles(t *testing.T) {
	cases := []struct {
		name, src, code string
	}{
		{"native concrete mismatch", `add true 1`, "signature_error"},
		{"zero-operand dispatch", `canon`, "signature_error"},
		{"user fn class-param mismatch", `def Foo (class {}) def g fn [[f:Foo] [Integer] [99]] 42 g`, "signature_error"},
		{"predicate refine boundary value", `def Big (Integer gt 10) def g fn [[n:Big] [Integer] [99]] 10 g`, "signature_error"},
		{"arity modifier misses every sig", `add/3 2 3`, "signature_error"},
		{"bare type-literal operand", `get 'a' Map`, "signature_error"},
		// (Boolean add is now a defined within-type error, so the popped-
		// clone probe uses a Map tuple — anchored per the rev-2 ownership
		// rule, and still unmatched after the undef.)
		{"undef leaves no overload", `def MM (refine Map)  def add fn [[a:MM b:MM] [MM] [a]]  undef add  add {a:1} {b:2}`, "signature_error"},
		{"map-literal member dispatch (unfinished unit stubbed)", `def f fn [[x:Integer] [Integer] [add x 1]] {f}`, "signature_error"},
		{"void arg group at def", `def f fn [[x:Integer] [] []] def r (f 1)`, "def_error"},
		{"void arg group at consumer", `def f fn [[x:Integer] [] []] 3 add (f 1)`, "no_value_error"},
	}
	for _, c := range cases {
		prog, reason, _, cerr := mustNew(t).CompileCheck(c.src)
		if cerr != nil {
			t.Fatalf("%s: check error %v", c.name, cerr)
		}
		if prog == nil {
			t.Fatalf("%s: expected a trap program, declined: %s", c.name, reason)
		}
		if dis := prog.Disassemble(); !strings.Contains(dis, "TRAP") || strings.Contains(dis, "FALLBACK") {
			t.Errorf("%s: expected a terminal TRAP, no island:\n%s", c.name, dis)
		}
		_, compiled, errC := mustNew(t).RunCompiled(c.src)
		if noteCompileDefect(t, c.src, nil, errC) {
			continue
		}
		if !compiled {
			t.Errorf("%s: trap program did not run compiled (fell back)", c.name)
		}
		_, errI := mustNew(t).RunInterp(c.src)
		if codeOf(errC) != c.code || codeOf(errI) != c.code {
			t.Fatalf("%s: compiled=[%s] interp=[%s], want both %s", c.name, codeOf(errC), codeOf(errI), c.code)
		}
		// Byte-identical taxonomy: code (above) AND detail; position present
		// whenever the interpreter's is (the full-corpus error lane's contract).
		var aeC, aeI *core.BoruError
		if !errors.As(errC, &aeC) || !errors.As(errI, &aeI) {
			t.Fatalf("%s: non-Boru error: compiled=%v interp=%v", c.name, errC, errI)
		}
		if aeC.Detail != aeI.Detail {
			t.Errorf("%s: detail divergence:\n  compiled=%q\n  interp=%q", c.name, aeC.Detail, aeI.Detail)
		}
		if aeI.Row > 0 && aeC.Row == 0 {
			t.Errorf("%s: position lost in compiled mode (interp at %d:%d)", c.name, aeI.Row, aeI.Col)
		}
	}
}

// Side-effect ordering across a dispatch trap: the trap fires at the SAME
// execution point as the interpreter's raise, so an effect BEFORE the failing
// dispatch (a print) must still run in compiled mode — the kept event prefix
// executes, then the TRAP raises.
func TestUnmatchedDispatchTrapPreservesPriorEffects(t *testing.T) {
	const src = `print 'a' raise 42`
	prog, reason, _, cerr := mustNew(t).CompileCheck(src)
	if cerr != nil || prog == nil {
		t.Fatalf("expected a trap program: reason=%q err=%v", reason, cerr)
	}
	if dis := prog.Disassemble(); !strings.Contains(dis, "TRAP") {
		t.Fatalf("expected a TRAP after the print:\n%s", dis)
	}
	var outC, outI strings.Builder
	ac := mustNew(t)
	ac.SetOutput(&outC)
	_, compiled, errC := ac.RunCompiled(src)
	if noteCompileDefect(t, src, nil, errC) {
		return
	}
	if !compiled {
		t.Fatalf("trap program did not run compiled")
	}
	ai := mustNew(t)
	ai.SetOutput(&outI)
	_, errI := ai.RunInterp(src)
	if codeOf(errC) != "signature_error" || codeOf(errI) != "signature_error" {
		t.Fatalf("compiled=[%s] interp=[%s], want both signature_error", codeOf(errC), codeOf(errI))
	}
	if outC.String() != outI.String() || !strings.Contains(outC.String(), "a") {
		t.Errorf("effect ordering: compiled output %q, interp output %q (print must run before the trap)", outC.String(), outI.String())
	}
}

// Negatives for the dispatch trap: a NON-definite failure keeps the blanket
// compile failure and does not compile — the trap must never claim a
// dispatch whose runtime outcome can differ from the static one.
func TestUnmatchedDispatchTrapNegatives(t *testing.T) {
	// Legacy compile failure+fallback-parity contract: pins the one-release
	// (The former "carrier operand declines" negative — `5 inc apply` —
	// became a POSITIVE with the Phase 6 M4 carrier-disjointness extension,
	// and since OpDispatchRematch landed the whole single-carrier-window
	// class compiles to a runtime rematch. The two carrier hazards below —
	// the refinement escape and the value-sensitive predicate — now compile
	// too, and their soundness pin MOVED with them: the rematch MATCHES at
	// run time and DEFERS to the interpreter, which computes the value a
	// static trap would have wrongly raised over. The deferred-token shapes
	// keep the whole-program compile failure.)
	// The REFINEMENT ESCAPE (the original carrier hazard, in its live form):
	// mkb's declared return is Boolean but the runtime value carries the
	// Flag-reparented tag, so the merged [Flag Flag] overload MATCHES at run
	// time. Since `add` gained its within-type CoreDefault overloads, the
	// static Boolean carriers MATCH [Boolean Boolean] at check time — a
	// match that is not a dispatch proof (a subtype-tagged runtime value
	// re-matches the more-specific [Flag Flag]) — so the compile DECLINES up
	// front (recordCallCompileFailure's core-default gate) instead of reaching the
	// dispatch-recovery rematch; the interpreter owns the program and both
	// paths agree on `true`.
	{
		src := `import module [def Flag (refine Boolean) def add fn [[a:Flag b:Flag] [Boolean] [a and b]] def mk fn [[b:Boolean] [Flag] [def v:Flag b v]] def mkb fn [[b:Boolean] [Boolean] [def v:Flag b v]] export "M" {add: add/v mk: mk/v mkb: mkb/v}]  add (M.mkb true) (M.mk true)`
		prog, reason, _, _ := mustNew(t).CompileCheck(src)
		if prog != nil || !strings.Contains(reason, "core-default dispatch over a carrier operand") {
			t.Errorf("refinement escape: want the core-default compile failure, got prog=%v reason=%q", prog != nil, reason)
		}
		gotC, _, errC := mustNew(t).RunCompiled(src)
		gotI, errI := mustNew(t).RunInterp(src)
		if noteCompileDefect(t, src, gotC, errC) {
			return
		}
		if codeOf(errC) != codeOf(errI) || fmt.Sprint(gotC) != fmt.Sprint(gotI) {
			t.Errorf("refinement escape: fallback=%v/%v interp=%v/%v (should agree)", gotC, errC, gotI, errI)
		}
	}
	rematches := []struct{ name, src string }{
		// A value-sensitive predicate param (membershipBeyondNominal): the
		// carrier's runtime VALUE decides membership — this variant PASSES at
		// run time (f 5 → 11 ∈ Big), so the rematch matches and defers.
		{"predicate-param carrier rematch defers (runtime pass)",
			`def Big (Integer gt 10) def g fn [[n:Big] [Integer] [99]] def f fn [[x:Integer] [Integer] [x add 6]] g (f 5)`},
	}
	for _, c := range rematches {
		prog, reason, _, _ := mustNew(t).CompileCheck(c.src)
		if prog == nil {
			t.Fatalf("%s: declined (%q); want a runtime-rematch compile", c.name, reason)
		}
		gotC, _, errC := mustNew(t).RunCompiled(c.src)
		gotI, errI := mustNew(t).RunInterp(c.src)
		if noteCompileDefect(t, c.src, gotC, errC) {
			continue
		}
		if codeOf(errC) != codeOf(errI) {
			t.Errorf("%s: defer err=[%s] interp err=[%s] (should agree)", c.name, codeOf(errC), codeOf(errI))
		}
		if errC == nil && errI == nil && fmt.Sprint(gotC) != fmt.Sprint(gotI) {
			t.Errorf("%s: defer value=%v interp value=%v (should agree)", c.name, gotC, gotI)
		}
	}
	// GRADUATED 2026-07-16 (COMPILE FAILURE-CLOSURE.0 §2 — the dynamic-operand
	// rematch): the flex-cell Reach shape `f m.a` no longer declines. By
	// dispatch-recovery time the check pass has EVALUATED the reach in place
	// (the recorded poly-dot event), so the failed window holds an
	// event-produced DYNAMIC carrier, not a raw Reach token — and the
	// definiteness screen now routes dynamics to the runtime rematch
	// alongside carriers (the rematch reads only the operand's LIVE value,
	// exactly what the interpreter's dispatch examines). The compiled
	// program re-matches at run time: a no-match raises the interpreter's
	// byte-identical rich signature_error (pinned here); a match defers.
	rematchRaises := []struct{ name, src string }{
		{"flex-reach dynamic operand rematch raises",
			`def f fn [[x:List][List][x]] def m (flex {a:1}) f m.a`},
		{"flex-list dynamic operand rematch raises",
			`def f fn [[x:Integer][Integer][x add 1]] def m (flex {a:[9]}) f m.a`},
	}
	for _, c := range rematchRaises {
		prog, reason, _, _ := mustNew(t).CompileCheck(c.src)
		if prog == nil {
			t.Fatalf("%s: declined (%q); want a runtime-rematch compile", c.name, reason)
		}
		if !strings.Contains(prog.Disassemble(), "DISPATCH_REMATCH") {
			t.Errorf("%s: expected a DISPATCH_REMATCH lowering:\n%s", c.name, prog.Disassemble())
		}
		gotC, compiled, errC := mustNew(t).RunCompiled(c.src)
		gotI, errI := mustNew(t).RunInterp(c.src)
		if noteCompileDefect(t, c.src, gotC, errC) {
			continue
		}
		if !compiled {
			t.Errorf("%s: the runtime NO-MATCH must raise compiled, not defer", c.name)
		}
		if fmt.Sprint(errC) != fmt.Sprint(errI) {
			t.Errorf("%s: raise parity —\n  compiled: %v\n  interp:   %v", c.name, errC, errI)
		}
		if fmt.Sprint(gotC) != fmt.Sprint(gotI) {
			t.Errorf("%s: value parity: compiled=%v interp=%v", c.name, gotC, gotI)
		}
	}
	failures := []struct{ name, src, want string }{
		// (The variadic-if shape that used to sit here GRADUATED 2026-07-15
		// — the branch merge seats the 1-vs-2 residual and the terminal
		// rematch seats its const under the live region top; pinned
		// positively in TestDispatchRematchVariadicIfGraduated. The
		// flex-cell Reach shape graduated 2026-07-16 — the §2 dynamic-
		// operand rematch, pinned positively in rematchRaises above.)
		// The class's REMAINING compile failure: the failed window's dynamic sits
		// under a leading stack residual (g's Integer result), so the
		// written-tuple render bound cannot prove a contiguous slice and the
		// rematch declines — the program falls back whole, and the
		// interpreter (which mutated the cell to a List through the opaque
		// fn param) runs it fine.
		{"dynamic under a stack residual declines",
			`def f fn [[x:List][List][x]] def m (flex {a:1}) def g fn [[s:Node][Integer][set a/q [7] s drop 0]] g m f m.a`,
			"unmatched dispatch recovered at f"},
	}
	for _, c := range failures {
		prog, reason, _, _ := mustNew(t).CompileCheck(c.src)
		if prog != nil {
			t.Fatalf("%s: compiled; want compile failure (disasm:\n%s)", c.name, prog.Disassemble())
		}
		if !strings.Contains(reason, c.want) {
			t.Errorf("%s: compile failure reason %q; want substring %q", c.name, reason, c.want)
		}
		gotC, _, errC := mustNew(t).RunCompiled(c.src)
		gotI, errI := mustNew(t).RunInterp(c.src)
		if noteCompileDefect(t, c.src, gotC, errC) {
			continue
		}
		if codeOf(errC) != codeOf(errI) {
			t.Errorf("%s: fallback err=[%s] interp err=[%s] (should agree)", c.name, codeOf(errC), codeOf(errI))
		}
		if errC == nil && errI == nil && fmt.Sprint(gotC) != fmt.Sprint(gotI) {
			t.Errorf("%s: fallback value=%v interp value=%v (should agree)", c.name, gotC, gotI)
		}
	}

	// The soundness pin that shaped the deferred-token decline: a reach over a
	// MUTATED flex cell fails the STATIC match at `drop` (the raw Reach token)
	// but resolves and dispatches fine at run time — a trap here raised where
	// the interpreter returns a value (flex.tsv L88's divergence). The row must
	// not trap; compiled and interpreted results must agree.
	const flexReach = `def m {a:{b:1}} def f (flex m) set b/q 9 f.a drop f.a.b`
	if prog, _, _, _ := mustNew(t).CompileCheck(flexReach); prog != nil {
		if strings.Contains(prog.Disassemble(), "TRAP") {
			t.Fatalf("flex-reach row must not trap:\n%s", prog.Disassemble())
		}
	}
	gotC, _, errC := mustNew(t).RunCompiled(flexReach)
	gotI, errI := mustNew(t).RunInterp(flexReach)
	if noteCompileDefect(t, flexReach, gotC, errC) {
		return
	}
	if errC != nil || errI != nil {
		t.Fatalf("flex-reach row errored: compiled=%v interp=%v", errC, errI)
	}
	if fmt.Sprint(gotC) != fmt.Sprint(gotI) {
		t.Errorf("flex-reach parity: compiled=%v interp=%v", gotC, gotI)
	}
}

// The graduated word-splice trap (GRADUATED 2026-07-14, compile failures 3->2): a
// PARKED __SP marker (def-bound, collected by value — never stepped before
// the failing dispatch) is identical at run time, so the definiteness screen
// admits it and the row compiles to a serialized terminal OpTrap raising the
// interpreter's byte-identical signature_error. A pointer-position splice
// fires before any dispatch on BOTH engines, so no window ever holds a
// would-have-fired marker (the flex-reach pin above guards the Reach family
// that stays screened).
func TestUnmatchedDispatchTrapSpliceGraduated(t *testing.T) {
	const src = `def p word [1 add 2] def f fn [[x:Integer][Integer][x mul 10]] f p`
	prog, reason, _, cerr := mustNew(t).CompileCheck(src)
	if cerr != nil || prog == nil {
		t.Fatalf("the splice trap must compile, got compile failure %q / err %v", reason, cerr)
	}
	if !strings.Contains(prog.Disassemble(), "TRAP") {
		t.Fatalf("the row must lower to a terminal trap:\n%s", prog.Disassemble())
	}
	gotC, compiled, errC := mustNew(t).RunCompiled(src)
	if noteCompileDefect(t, src, gotC, errC) {
		return
	}
	if !compiled {
		t.Fatal("the trap program must run compiled")
	}
	gotI, errI := mustNew(t).RunInterp(src)
	if errC == nil || errI == nil {
		t.Fatalf("both engines must raise: compiled=%v interp=%v", errC, errI)
	}
	if errC.Error() != errI.Error() {
		t.Errorf("diagnostic drift:\n compiled: %s\n interp:   %s", errC, errI)
	}
	if fmt.Sprint(gotC) != fmt.Sprint(gotI) {
		t.Errorf("residuals must agree: compiled=%v interp=%v", gotC, gotI)
	}
}

// Typed-def store-with-reparent (the compiled closure of miscompile B,
// design/legacy/MISCOMPILE-HUNT-FINDINGS.0.ignore §B): a typed value-def whose refinement
// constraint guards a DYNAMIC body (`def v:Flag b` over a param / computed
// carrier) compiles to an OpBindTyped that runs the interpreter's own
// validate/reparent (RunTypedBind) at run time instead of declining the whole
// program — pass stores the (reparented) value, FAIL raises the byte-identical
// plain error, typeof renders the newtype, and sig dispatch keys off the
// reparented tag.
func TestTypedDefBindCompiles(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"predicate validate-pass stores and reads back (multi-read forces the frame local)",
			`def Positive fnpred [[n:Integer] [n gt 0]] def f fn [[x:Integer] [Integer] [def v:Positive x (v add v)]] f 5`},
		{"predicate reparent: typeof renders the predicate type",
			`def Positive fnpred [[n:Integer] [n gt 0]] def f fn [[x:Integer] [Integer] [def v:Positive x v]] typeof (f 5)`},
		{"newtype reparent: typeof renders the newtype in compiled mode (the §B divergence)",
			`def Flag (refine Boolean) def mk fn [[b:Boolean] [Flag] [def v:Flag b v]] typeof (mk true)`},
		{"sig-dispatch on the reparented local",
			`def Pos (refine Integer) def need-pos fn [[p:Pos] [Integer] [p add 100]] def f fn [[n:Integer] [Integer] [def x:Pos n need-pos x]] f 5`},
		{"top-level dynamic refine def (the other live class)",
			`def Pos (refine Integer) def g fn [[] [Integer] [41]] def n (g) def x:Pos (add n 1) typeof x`},
		{"named DepScalar dynamic pass keeps the base tag (no reparent: typeof stays Integer)",
			`def Big (Integer gt 10) def f fn [[x:Integer] [Type] [def v:Big x typeof v]] f 50`},
	}
	for _, c := range cases {
		prog, reason, _, cerr := mustNew(t).CompileCheck(c.src)
		if cerr != nil || prog == nil {
			t.Fatalf("%s: did not compile: reason=%q err=%v", c.name, reason, cerr)
		}
		if !strings.Contains(prog.Disassemble(), "BIND_TYPED") {
			t.Errorf("%s: no BIND_TYPED in the program (the dynamic bind must run at the VM):\n%s",
				c.name, prog.Disassemble())
		}
		gotC, compiled, errC := mustNew(t).RunCompiled(c.src)
		if noteCompileDefect(t, c.src, gotC, errC) {
			continue
		}
		if !compiled || errC != nil {
			t.Fatalf("%s: compiled run failed: compiled=%v err=%v", c.name, compiled, errC)
		}
		gotI, errI := mustNew(t).RunInterp(c.src)
		if errI != nil {
			t.Fatalf("%s: interp run failed: %v", c.name, errI)
		}
		if fmt.Sprint(gotC) != fmt.Sprint(gotI) {
			t.Errorf("%s: compiled=%v interp=%v", c.name, gotC, gotI)
		}
	}

	// Validation failures raise the byte-identical error — the interpreter's
	// defTypedHandler raises a PLAIN position-less error (fmt.Errorf; probe:
	// stampErrPos only positions BoruErrors), so byte-identical INCLUDING
	// position means the full rendered text matches exactly, with no position
	// added by either engine. Asserted through RunCompiledStrict so the error
	// provably comes from the VM's OpBindTyped, not from a silent interpreter
	// fallback re-run.
	fails := []struct {
		name string
		src  string
	}{
		{"predicate validate-FAIL",
			`def Positive fnpred [[n:Integer] [n gt 0]] def f fn [[x:Integer] [Integer] [def v:Positive x v]] f 0`},
		{"inline DepScalar validate-FAIL",
			`def f fn [[x:Integer] [Integer] [def v:(Integer gt 10) x v]] f 5`},
		{"newtype validate-FAIL on a laundered gradual value",
			`def Pos (refine Integer) def g fn [[] [Any] ['nope']] def f fn [[x:Any] [Integer] [def v:Pos x 1]] f (g)`},
	}
	for _, c := range fails {
		_, errC := mustNew(t).RunCompiledStrict(c.src)
		_, errI := mustNew(t).RunInterp(c.src)
		if errC == nil || errI == nil {
			t.Fatalf("%s: expected both engines to raise: compiled=%v interp=%v", c.name, errC, errI)
		}
		// Code and message must match exactly. The rendered POSITION is
		// compared separately below (NUR108): two of these three rows carry
		// the identical code and detail on both lanes and differ only in
		// where the diagnostic points, which `.Error()` folds in — and this
		// assertion read its interp side from `Run`, the compiled lane, until
		// 2026-08-27 (NUR106), so it never compared anything at all.
		if noteCompileDefect(t, c.src, nil, errC) {
			continue
		}
		var aeC, aeI *core.BoruError
		okC, okI := errors.As(errC, &aeC), errors.As(errI, &aeI)
		if okC != okI {
			t.Errorf("%s: one lane raised a structured diagnostic and the other did not:\n"+
				"  compiled: %v\n  interp:   %v", c.name, errC, errI)
			continue
		}
		if !okC {
			// A predicate failure raises a plain error on both lanes; there is
			// no position to diverge over, so compare the text and move on.
			if errC.Error() != errI.Error() {
				t.Errorf("%s: error divergence:\n  compiled: %s\n  interp:   %s", c.name, errC, errI)
			}
			continue
		}
		if aeC.Code != aeI.Code || aeC.Detail != aeI.Detail {
			t.Errorf("%s: error divergence:\n  compiled: [%s] %s\n  interp:   [%s] %s",
				c.name, aeC.Code, aeC.Detail, aeI.Code, aeI.Detail)
		}
		// NUR108 RESOLVED 2026-08-30: the compiled lane's BIND_TYPED validate
		// failure now points where the interpreter does — at the `def`. It used
		// to render "source position unknown", because the RECORD carried no
		// position: the typed-name map is synthesised by the `name:Type`
		// desugar and is 0:0 at every typed-def site, so nothing reached
		// stampAt to stamp. CheckState.CurWordPos publishes the dispatching
		// word's position — the same value stampErrPos gives the interpreter —
		// and the def handler falls back to it.
		//
		// The fence this replaces was written to fail the day it was fixed.
		// Position is now part of the equality, both row AND column, so a
		// future drift in either lane is a failure rather than a shrug.
		if aeC.Row != aeI.Row || aeC.Col != aeI.Col {
			t.Errorf("%s: diagnostic position divergence: compiled %d:%d, interp %d:%d",
				c.name, aeC.Row, aeC.Col, aeI.Row, aeI.Col)
		}
		if aeI.Row == 0 {
			t.Errorf("%s: neither lane carries a position — a raise inside a fn body must point somewhere", c.name)
		}
	}

	// Negatives — what must NOT change:
	// (a) a STATIC (concrete) typed-def body keeps the proven const-pool
	//     reparent — no BIND_TYPED is emitted;
	// (b) a statically-failing typed-def over a CONCRETE body compiles to
	//     the TERMINAL TRAP, never a bind (FLIPPED by NUR058: the failed
	//     unify over an exactly-known operand is a guaranteed-error
	//     MIRROR, so the row takes the correct-error disposition — the
	//     compiled program raises the interpreter-identical
	//     [boru/type_error] instead of failing to compile).
	staticSrc := `def Pt (refine Integer) def p:Pt 5 typeof p`
	prog, reason, _, cerr := mustNew(t).CompileCheck(staticSrc)
	if cerr != nil || prog == nil {
		t.Fatalf("static typed-def did not compile: reason=%q err=%v", reason, cerr)
	}
	if strings.Contains(prog.Disassemble(), "BIND_TYPED") {
		t.Errorf("static typed-def must ride the const pool, not BIND_TYPED:\n%s", prog.Disassemble())
	}
	gotC, compiled, errC := mustNew(t).RunCompiled(staticSrc)
	gotI, errI := mustNew(t).RunInterp(staticSrc)
	if noteCompileDefect(t, staticSrc, gotC, errC) {
		return
	}
	if !compiled || errC != nil || errI != nil || fmt.Sprint(gotC) != fmt.Sprint(gotI) {
		t.Errorf("static typed-def parity: compiled=%v/%v/%v interp=%v/%v", gotC, compiled, errC, gotI, errI)
	}
	const failingSrc = `def x:(Integer gt 10) 5 x`
	fprog, freason, _, ferr := mustNew(t).CompileCheck(failingSrc)
	if ferr != nil || fprog == nil {
		t.Fatalf("statically-failing DepScalar typed-def must compile the trap (NUR058): reason=%q err=%v", freason, ferr)
	}
	if !strings.Contains(fprog.Disassemble(), "TRAP") {
		t.Errorf("statically-failing typed-def must be the terminal trap, got:\n%s", fprog.Disassemble())
	}
	if strings.Contains(fprog.Disassemble(), "BIND_TYPED") {
		t.Errorf("statically-failing typed-def must never emit a bind:\n%s", fprog.Disassemble())
	}
	_, errFC := mustNew(t).RunCompiledStrict(failingSrc)
	_, errFI := mustNew(t).RunInterp(failingSrc)
	var aeFC, aeFI *core.BoruError
	if !errors.As(errFC, &aeFC) || !errors.As(errFI, &aeFI) {
		t.Fatalf("failing typed-def trap parity: compiled=%v interp=%v", errFC, errFI)
	}
	if aeFC.Code != aeFI.Code || aeFC.Detail != aeFI.Detail {
		t.Errorf("failing typed-def trap parity: compiled=[%s] %s interp=[%s] %s",
			aeFC.Code, aeFC.Detail, aeFI.Code, aeFI.Detail)
	}
	// NUR108's other half, also RESOLVED 2026-08-30. This one was never a
	// LOSS — both lanes carried a position and disagreed about which token to
	// blame: the terminal trap fired at the VALUE it rejected (1:23), the
	// interpreter at the `def` (1:1). The same CurWordPos fallback settles it,
	// because the trap is serialized from the same handler-side record.
	if aeFC.Row != aeFI.Row || aeFC.Col != aeFI.Col {
		t.Errorf("failing typed-def trap position divergence: compiled %d:%d, interp %d:%d",
			aeFC.Row, aeFC.Col, aeFI.Row, aeFI.Col)
	}
}

// PR #225 P1 review findings — two auto-dispatch/identity escapes, both
// probe-confirmed divergences before the fix; (1) compiles since
// 2026-09-24 (the selective freshen), (2) is still a compile failure.
func TestPR225P1CompileFailures(t *testing.T) {
	// (1) A fn-body literal EMBEDDING an enclosing binding's container:
	// interp = fresh spine + SHARED member. Neither a deep-clone freshen
	// nor a shared const modelled it, so it declined until the fresh push
	// learned to keep the embedded member (Program.ConstKeep): the shape
	// compiles, and both identities hold — fn_body_embed_keep_test.go pins
	// the family.
	const embeds = `def c [9] def mk fn [[] [List] [[c]]] ((mk) get 0) eq c`
	gotC, compiled, errC := mustNew(t).RunCompiled(embeds)
	gotI, errI := mustNew(t).RunInterp(embeds)
	if !compiled || errC != nil || errI != nil || fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotI) != "[true]" {
		t.Errorf("embedded-binding literal: compiled=%v cErr=%v iErr=%v got %v vs %v (want [true])",
			compiled, errC, errI, gotC, gotI)
	}

	// (2) A class-instance field holding a genuinely-0-param fn: the
	// interpreter auto-dispatches the read (42); the instance receiver is a
	// schema CARRIER at record time, so the hazard is tracked at make time
	// by instance ID (noteFnRiskFields) and the member rides the
	// construction-time note (instanceFnMember). GRADUATED (the break-2
	// closure): the pinpointed genuine-0-arg member's landing compiles as
	// an arity-0 OpCallDynMethod with parity.
	const classFn = `def make42 fn [[] [Integer] [42]] def C class {f:Function} def o (make C {f:make42/v}) o.f`
	gotC2, compiled2, errC2 := mustNew(t).RunCompiled(classFn)
	gotI2, errI2 := mustNew(t).RunInterp(classFn)
	if noteCompileDefect(t, classFn, gotC2, errC2) {
		return
	}
	if !compiled2 || errC2 != nil || errI2 != nil || fmt.Sprint(gotC2) != fmt.Sprint(gotI2) || fmt.Sprint(gotI2) != "[42]" {
		t.Errorf("compiled parity: compiled=%v cErr=%v iErr=%v got %v vs %v (want compiled [42])",
			compiled2, errC2, errI2, gotC2, gotI2)
	}

	// NEGATIVES: a NON-fn field of the same instance keeps compiling with
	// parity (key precision), and the applied multi-param member-call
	// pattern is untouched.
	const nonFn = `def make42 fn [[] [Integer] [42]] def C class {f:Function x:0} def o (make C {f:make42/v x:5}) o.x`
	gotC3, compiled3, errC3 := mustNew(t).RunCompiled(nonFn)
	if noteCompileDefect(t, nonFn, gotC3, errC3) {
		return
	}
	if !compiled3 || errC3 != nil || fmt.Sprint(gotC3) != "[5]" {
		t.Errorf("non-fn field read: compiled=%v err=%v got=%v (want compiled [5])", compiled3, errC3, gotC3)
	}
}

// Filter-lambda closures with LEXICAL captures + computed collections
// (design/legacy/RUNTIME-STAMPING.0.ignore Phase 3): the BODY lambda admits captures —
// resolved to compiled homes, threaded at OpPushClosure, bound to trailing
// unit slots — and a typed non-dynamic carrier data operand. Positives
// compile natively (no island) with compiled == interpreted parity; the
// negative twins pin what must KEEP declining.
func TestFilterLambdaCaptureCompiles(t *testing.T) {
	// Legacy compile failure+fallback-parity contract: pins the one-release
	parity := []struct{ name, src, want string }{
		{"enclosing-fn param capture (list pair shape)",
			`def f (fn [[y:Integer] [List] [ filter ([e:Any] => [ e.value gte y ]) [3 7 9] ]]) f 5`,
			"[[7 9]]"},
		{"computed collection (keys result carrier)",
			`def f (fn [[m:Map] [List] [ filter ([e:Any] => [ (e.value eq "b") not ]) (keys m) ]]) f {a:1 b:2}`,
			"[[a]]"},
		{"body-local capture over a computed collection (the mini-redis KEYS shape)",
			`def g (fn [[m:Map] [List] [ def kv m  filter ([e:Any] => [ ((kv get e.value) eq None) not ]) (keys kv) ]]) g {x:1}`,
			"[[x]]"},
		{"map KeyVal shape with a param capture",
			`def f (fn [[y:Integer] [Map] [ filter ([kv:Any] => [ kv.v gte y ]) {a:3 b:7} ]]) f 5`,
			"[map[b:7]]"},
	}
	for _, c := range parity {
		prog, reason, _, cerr := mustNew(t).CompileCheck(c.src)
		if cerr != nil {
			t.Fatalf("%s: check error %v", c.name, cerr)
		}
		if prog == nil {
			t.Fatalf("%s: declined: %s", c.name, reason)
		}
		if strings.Contains(prog.Disassemble(), "FALLBACK") {
			t.Errorf("%s: expected no island:\n%s", c.name, prog.Disassemble())
		}
		gotC, compiled, errC := mustNew(t).RunCompiled(c.src)
		gotI, errI := mustNew(t).RunInterp(c.src)
		if noteCompileDefect(t, c.src, gotC, errC) {
			continue
		}
		if !compiled {
			t.Errorf("%s: did not run compiled", c.name)
		}
		if errC != nil || errI != nil {
			t.Fatalf("%s: run errs compiled=%v interp=%v", c.name, errC, errI)
		}
		if fmt.Sprint(gotC) != fmt.Sprint(gotI) {
			t.Errorf("%s: compiled=%v interp=%v (must agree)", c.name, gotC, gotI)
		}
	}

	// NEGATIVES — each keeps its compile failure, and the compile failure keeps
	// the value contract (RunCompiled degrades, never diverges).

	// A MULTI-overload lambda still declines (MatchFnSig picks at runtime;
	// compiling one overload could run the wrong body).
	{
		src := `def two (fn [[x:Integer] [Integer] [x add 1] [x:String] [String] [x]])
def f (fn [[] [List] [ filter two [1 2 3] ]]) f`
		prog, _, _, cerr := mustNew(t).CompileCheck(src)
		if cerr == nil && prog != nil && !strings.Contains(prog.Disassemble(), "FALLBACK") {
			// The multi-overload operand must not compile to a single closure
			// unit; islanding, compile failure, or (since S1a) a dyn-body dispatch
			// whose handler picks the overload at run time are all sound.
			// Both engines RAISE on this program — filter delivers a
			// {key,value} pair that neither overload of `two` takes — so
			// parity here is error parity, code and message alike.
			gotC, _, errC := mustNew(t).RunCompiled(src)
			gotI, errI := mustNew(t).RunInterp(src)
			if noteCompileDefect(t, src, gotC, errC) {
				return
			}
			if codeOf(errC) != codeOf(errI) || fmt.Sprint(errC) != fmt.Sprint(errI) || fmt.Sprint(gotC) != fmt.Sprint(gotI) {
				t.Errorf("multi-overload operand diverged: compiled=%v/%v interp=%v/%v", gotC, errC, gotI, errI)
			}
		}
	}

	// A DYNAMIC (gradual) collection still declines the lambda path — the
	// callback convention (pair vs KeyVal) is ambiguous. The program falls
	// back and values agree.
	{
		src := `def f (fn [[x:Any] [Any] [ filter ([e:Any] => [ e.value ]) x.items ]]) f {items: [1 2]}`
		gotC, _, errC := mustNew(t).RunCompiled(src)
		gotI, errI := mustNew(t).RunInterp(src)
		if noteCompileDefect(t, src, gotC, errC) {
			return
		}
		if fmt.Sprint(errC) != fmt.Sprint(errI) || fmt.Sprint(gotC) != fmt.Sprint(gotI) {
			t.Errorf("dynamic-collection lambda diverged: compiled=%v/%v interp=%v/%v", gotC, errC, gotI, errI)
		}
	}

	// A capturing HOOK lambda (walk's ascend slot — the extras path) keeps
	// declining the closure compile: allowCaptures is body-lambda-only. The
	// program islands or declines; values agree either way.
	{
		src := `def f (fn [[tag:String] [Any] [
  def acc (flex [])
  walk {mode: "depth"} {a:1} (m:Any => [acc (m.path) append]) (m:Any => [acc (tag) append])
  acc
]]) f "z"`
		gotC, _, errC := mustNew(t).RunCompiled(src)
		gotI, errI := mustNew(t).RunInterp(src)
		if noteCompileDefect(t, src, gotC, errC) {
			return
		}
		if fmt.Sprint(errC) != fmt.Sprint(errI) || fmt.Sprint(gotC) != fmt.Sprint(gotI) {
			t.Errorf("capturing hook lambda diverged: compiled=%v/%v interp=%v/%v", gotC, errC, gotI, errI)
		}
	}
}
