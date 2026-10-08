package lang

import (
	"fmt"
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// bytecode_do_error_arity_test.go pins the §8.2(6) ZERO-NETTING handler
// graduation (completeness-review §9.13): a `do … error` whose body PROVABLY
// raises (a strict Error do-result — the pass-through arm is statically
// dead) and whose handler nets NO value now compiles natively — the
// dispatch records a 0-output call (errorReturnsFn returns the true zero
// arity) and the strip-input shape screen admits the empty residual
// (stripResidualShapeOK's want-0 arm).
func TestZeroNettingHandlerCompiles(t *testing.T) {
	fnValueM2Native(t, "the ledgered frontier row",
		`do [1 div 0] error [drop] end 2 add 3`,
		"[5]")
	fnValueM2Native(t, "the 1-netting twin keeps compiling",
		`do [1 div 0] error [drop 9] end 2 add 3`,
		"[9 5]")
}

// The DYNAMIC Error bound used to be pinned here as an edge the 2026-08-03
// graduation kept: a body that may not raise has variable arity — the
// pass-through nets one where the caught path nets zero — so it declined.
// The forty-eighth increment graduated it, and the diagnosis is what
// changed rather than the shape: a run whose length is a runtime value is a
// REGION, not an unrepresentable seat. See
// TestMaybeRaisingZeroNettingHandlerIsARegion below, which pins both arms.

// TestFullStackHostOverloadParity pins the Codex-review claim (PR #327,
// eng/go/emit.go FoldFullStack) that a HOST-registered overload of a
// full-stack word diverges under compilation. It does not: check-mode
// dispatch and the runtime share one dispatch table, so both engines pick
// the host sig and the fold (gated on the DISPATCHED sig being a FullStack
// sig) never fires. Verified non-reproducing 2026-08-03; this pin keeps it
// that way.
func TestFullStackHostOverloadParity(t *testing.T) {
	host := func(a *Boru) {
		a.Register("depth", Signature{
			Args:       []*core.Type{core.TInteger},
			Returns:    []*core.Type{core.TInteger, core.TInteger},
			BarrierPos: 0,
			Impl: core.Go(func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
				return []core.Value{args[0], core.NewInteger(99)}, nil
			}),
		})
	}
	src := `10 1 depth`
	a, _ := New()
	host(a)
	gotI, errI := a.RunInterp(src)
	c, _ := New()
	host(c)
	gotC, _, errC := c.RunCompiled(src)
	if noteCompileDefect(t, src, gotC, errC) {
		return
	}
	if errI != nil || errC != nil || fmt.Sprint(gotI) != fmt.Sprint(gotC) {
		t.Errorf("host-overload depth diverged: interp=%v/%v compiled=%v/%v", gotI, errI, gotC, errC)
	}
	if fmt.Sprint(gotI) != "[10 1 99]" {
		t.Errorf("host-overload depth = %v, want [10 1 99] (the host sig owns the dispatch)", gotI)
	}
}

// TestLeadApplyNoMatchTwoReturnParity pins the Codex-review claim (PR #327,
// engine.go leading apply) that a type-mismatched 1-arg callee under a
// TWO-value declared return diverges.
//
// IT DID (NUR107), and this is the shape where the defect was NAKED. The
// declared TWO-value return is what made it so: the residual [fn, arg] has two
// values, so the frame's return-count check accepted it and no error fired at
// all. Narrower return arities merely surfaced a type_error instead, which is
// why the whole thing read as a taxonomy quibble rather than a silent wrong
// answer.
//
//	interpreted  signature_error
//	compiled     [fn (String) 14]        <- no error whatsoever
//
// The test previously asserted the OPPOSITE — "verified non-reproducing
// 2026-08-03" — reading its interp side from `Run`, which post-Stage-J IS the
// compiled lane (NUR106): it compared the compiled answer to itself and passed
// unconditionally. The review claim was right.
//
// CLOSED 2026-08-28: the VM's dynamic apply now distinguishes "not callable"
// (leave the window as data, which the interpreter also does) from "a Function
// no overload of which admits these arguments" (raise). Both lanes raise
// signature_error, and this pin flips to a PARITY assertion.
// TestTrailingApplyBareFunctionStaysData is NUR107's negative: the guard that
// raises for "a Function no overload of which admits these arguments" must NOT
// raise for a value that is Function-TYPED but carries no signatures at all.
//
// A bare `Function` type literal is exactly that shape — appliable by tag
// (IsAppliableFn reads the lattice parent), with no FnDefInfo payload and so
// no own signatures to consult. MatchFnSig answers nil there, and reading that
// nil as "no overload matched" would raise on a program both engines leave
// alone. So the guard checks OwnSigs() length first, and this row is what
// keeps that check honest.
func TestTrailingApplyBareFunctionStaysData(t *testing.T) {
	for _, src := range []string{
		`(14 Function)`,
		`def m {f: Function}  (14 (m dot f))`,
	} {
		gotC, compiled, errC := mustNew(t).RunCompiled(src)
		gotI, errI := mustNew(t).RunInterp(src)
		if noteCompileDefect(t, src, gotC, errC) {
			continue
		}
		if !compiled {
			t.Fatalf("%s: did not run compiled (%v)", src, errC)
		}
		if errC != nil || errI != nil {
			t.Errorf("%s: a signature-less Function value must stay DATA, not raise "+
				"(errC=%v errI=%v)", src, errC, errI)
		}
		if fmt.Sprint(gotC) != fmt.Sprint(gotI) {
			t.Errorf("%s: compiled=%v interp=%v", src, gotC, gotI)
		}
	}
}

func TestLeadApplyNoMatchTwoReturnParity(t *testing.T) {
	const src = `def ld fn [[g:Function x:Integer] [Function Integer] [(g x)]] ld ([k:String] => [k]) 14`
	gotC, compiled, errC := mustNew(t).RunCompiled(src)
	gotI, errI := mustNew(t).RunInterp(src)
	if noteCompileDefect(t, src, gotC, errC) {
		return
	}
	if !compiled {
		t.Fatalf("no-match lead apply: did not run compiled (errC=%v)", errC)
	}
	if errI == nil || codeOf(errI) != "signature_error" {
		t.Errorf("interpreted: err=%v got=%v, want a signature_error", errI, gotI)
	}
	if errC == nil || codeOf(errC) != "signature_error" {
		t.Errorf("compiled: err=%v got=%v, want a signature_error — a Function whose "+
			"overloads do not admit the argument must RAISE, not sit as data (NUR107)", errC, gotC)
	}
}

// TestCondBodyFreshDefRaisesLikeInterpreter is NUR110 CLOSED: a FRESH `def`
// inside a branch that did not run binds nothing afterwards, on both lanes.
//
// The compiled lane used to bind it anyway — `if false [def op 1] [0] end op`
// answered `0 1` where the interpreter raises undefined_word — because the
// join folded the arm's own value back as a definite binding and a later read
// baked it. The branch-carried def (compiler/go/branch_carried.go) seats the
// name in a frame slot instead: the arm's def stores into it when the arm
// runs, and a read past the merge is BOUND-CHECKED (OpPushLocalBound) — the
// zero slot is the arm that did not run, and the read raises the
// interpreter's own undefined_word. Both lanes now agree, and the fn-body
// shapes the miscompile record measured (design/SESSION-HANDOVER.0.md,
// 2026-09-21) are pinned beside the record's own three.
func TestCondBodyFreshDefRaisesLikeInterpreter(t *testing.T) {
	for _, src := range []string{
		`if false [def op 1] [0]  end  op`,
		`if false [def op 1] []   end  op`,
		`if false [def op 1] [0]  end  typeof op`,
		`if false [def z (1 add 8)] [] end z`,
		`def f fn [[b:Boolean] [Integer] [if b [def z 9] [] end z]]  f false`,
		`def f fn [[b:Boolean] [Integer] [if b [] [def z 9] end z]]  f true`,
	} {
		gotC, compiled, errC := mustNew(t).RunCompiled(src)
		gotI, errI := mustNew(t).RunInterp(src)
		if noteCompileDefect(t, src, gotC, errC) {
			continue
		}
		if !compiled {
			t.Fatalf("%s: did not run compiled (%v)", src, errC)
		}
		if codeOf(errI) != "undefined_word" {
			t.Errorf("%s: interpreted err=[%s] got=%v, want undefined_word — the oracle moved, "+
				"re-derive this pin", src, codeOf(errI), gotI)
		}
		if codeOf(errC) != "undefined_word" || len(gotC) != 0 {
			t.Errorf("%s: compiled err=[%s] got=%v, want undefined_word and no result — "+
				"a def in an untaken arm must not bind (NUR110)", src, codeOf(errC), gotC)
		}
	}
	// The taken path of the same shapes answers the arm's value on both
	// lanes — the bound check costs nothing when the arm ran.
	for _, tc := range []struct{ src, want string }{
		{`def f fn [[b:Boolean] [Integer] [if b [def z 9] [] end z]]  f true`, "[9]"},
		{`def f fn [[b:Boolean] [Integer] [if b [] [def z 9] end z]]  f false`, "[9]"},
		{`def c true  if c [def op 1] [0]  end  op`, "[1]"},
	} {
		gotC, _, errC := mustNew(t).RunCompiled(tc.src)
		if noteCompileDefect(t, tc.src, gotC, errC) {
			continue
		}
		if errC != nil || fmt.Sprint(gotC) != tc.want {
			t.Errorf("%s: compiled err=%v got=%v, want %s", tc.src, errC, gotC, tc.want)
		}
	}
}

// TestCondBodyZeroIterationLoopAgrees is NUR110's control, and the reason its
// verdict says the machinery exists: the same question — does a `def` in a body
// that never ran leave a binding? — is already answered identically by both
// lanes for a loop that runs zero times.
func TestCondBodyZeroIterationLoopAgrees(t *testing.T) {
	const src = `for 0 [def op 1]  end  op`
	_, _, errC := mustNew(t).RunCompiled(src)
	_, errI := mustNew(t).RunInterp(src)
	if noteCompileDefect(t, src, nil, errC) {
		return
	}
	if codeOf(errC) != "undefined_word" || codeOf(errI) != "undefined_word" {
		t.Errorf("zero-iteration loop: compiled=[%s] interp=[%s], want both undefined_word",
			codeOf(errC), codeOf(errI))
	}
}

// TestMaybeRaisingZeroNettingHandlerIsARegion — the forty-eighth increment,
// and §8.2(6)'s other half.
//
// A PROVEN-raise body's zero-netting handler was graduated in 2026-08-03 by
// telling the truth about a FIXED arity: the pass-through arm is statically
// dead, so the count is zero on every run. A MAYBE-raising body has no such
// fixed count — the caught path nets zero where the pass-through nets one —
// and that was read as unrepresentable.
//
// It is representable, by the device that was already there: a runtime-
// variadic REGION, one recorded slot standing for whatever the run delivers.
// The island's single simulated slot IS that representation — runFallback
// appends the re-run's real results — so no opcode changed. What changed is
// that errorReturnsFn returns the honest 0-or-more spread instead of marking
// the program uncompilable.
func TestMaybeRaisingZeroNettingHandlerIsARegion(t *testing.T) {
	for _, tc := range []struct{ src, want, note string }{
		// The ledger row. This input DIVIDES BY ZERO, so the handler runs
		// and nets nothing: the region delivers zero values.
		{`def xs [0] do [1 div (xs 0 getr)] error [drop] end 2 add 3`, "[5]", "caught: the run is empty"},
		// The SAME program on an input that does not raise: the body's one
		// value passes through and the region delivers it.
		{`def xs [1] do [1 div (xs 0 getr)] error [drop] end 2 add 3`, "[1 5]", "pass-through: the run is one value"},
		// With nothing after the do, both arms still answer.
		{`def xs [0] do [1 div (xs 0 getr)] error [drop]`, "[]", "caught, nothing following"},
		{`def xs [1] do [1 div (xs 0 getr)] error [drop]`, "[1]", "pass-through, nothing following"},
		// The proven-raise twin is untouched — it keeps its fixed 0-output
		// call rather than becoming a region.
		{`do [1 div 0] error [drop] end 2 add 3`, "[5]", "the proven-raise row keeps its fixed arity"},
		// A handler that NETS a value is not a region at all.
		{`5 do [7] error [drop 9] add 1`, "[5 8]", "a one-netting handler is an ordinary result"},
	} {
		t.Run(tc.note, func(t *testing.T) {
			a, err := New()
			if err != nil {
				t.Fatal(err)
			}
			out, compiled, cerr := a.RunCompiled(tc.src)
			if noteCompileDefect(t, tc.src, out, cerr) {
				return
			}
			if cerr != nil {
				t.Fatalf("RunCompiled: %v", cerr)
			}
			if !compiled {
				t.Fatal("the variable-arity handler must compile as a region")
			}
			if got := fmt.Sprintf("%v", out); got != tc.want {
				t.Errorf("compiled %s, want %s", got, tc.want)
			}
			b, err := New()
			if err != nil {
				t.Fatal(err)
			}
			outi, ierr := b.RunInterp(tc.src)
			if ierr != nil {
				t.Fatalf("RunInterp: %v", ierr)
			}
			if got := fmt.Sprintf("%v", outi); got != tc.want {
				t.Errorf("interpreter %s, want %s — the oracle moved", got, tc.want)
			}
		})
	}
}

// TestRegionHandlerDoesNotLowerAFixedSeatConsumer is the line: a region can be
// absorbed by a residual, and it cannot fill a slot that needs exactly one
// value. `def x (do … error […])` binds a name, which needs a count — and
// both lanes raise the same def_error when the run turns out to be empty.
func TestRegionHandlerDoesNotLowerAFixedSeatConsumer(t *testing.T) {
	const src = `def x (do [1 div 0] error [drop]) end 5`
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	_, _, cerr := a.RunCompiled(src)
	if noteCompileDefect(t, src, nil, cerr) {
		return
	}
	b, err := New()
	if err != nil {
		t.Fatal(err)
	}
	_, ierr := b.RunInterp(src)
	if cerr == nil || ierr == nil {
		t.Fatalf("both lanes must raise: compiled=%v interp=%v", cerr, ierr)
	}
	if fmt.Sprint(cerr) != fmt.Sprint(ierr) {
		t.Errorf("the raise must be byte-identical:\ncompiled=%v\ninterp=%v", cerr, ierr)
	}
	if !strings.Contains(cerr.Error(), "produced no value to bind") {
		t.Errorf("error drifted: %v", cerr)
	}
}
