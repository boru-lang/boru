package lang

import "testing"

// bytecode_chained_apply_test.go pins the chained-forward-apply family
// (design/legacy/checker-compiler-completeness-review.0.ignore §2.1). History: until
// 2026-08-02 the forward spelling `f (g x)` of two chained Function-param
// applications slipped past the pending-apply refusal net — noteDynFrameReplay
// armed the whole-frame replay on a window holding TWO applicable values, the
// flat re-push lost the inner group's collapse, and the compiled program
// raised the RET count error where the interpreter applies both fns. The
// replay was then narrowed to single-applicable windows (refusal), and
// on 2026-08-03 the Stage-G single-arg increment GRADUATED the family: a
// leading Function-typed carrier applied to one argument (`(g x)`, g a param)
// records the same RecordDynApply event the trailing spelling `(x g)` does —
// one-arg leading and trailing collection converge — so the inner group
// collapses to an event and the outer apply rides the single-applicable
// RetReplay body tail. The chain now compiles NATIVELY; corpus rows live in
// lang/spec/fn-value.tsv §8.
func TestChainedForwardApplyCompiles(t *testing.T) {
	fnValueM2Native(t, "compose — outer fn over the inner fn's result",
		`def compose fn [[f:Function g:Function x:Integer] [Integer] [f (g x)]] compose ([a:Integer] => [a mul 2]) ([b:Integer] => [b add 3]) 4`,
		"[14]")
	fnValueM2Native(t, "self-composition f (f x)",
		`def twice fn [[f:Function x:Integer] [Integer] [f (f x)]] twice ([n:Integer] => [n add 1]) 5`,
		"[7]")
	fnValueM2Native(t, "depth-3 chain f (g (f x))",
		`def c3 fn [[f:Function g:Function x:Integer] [Integer] [f (g (f x))]] c3 ([a:Integer] => [a mul 2]) ([b:Integer] => [b add 3]) 4`,
		"[22]")
	fnValueM2Native(t, "mid-body apply: the collapsed event feeds a further word",
		`def m1 fn [[g:Function x:Integer] [Integer] [(g x) add 100]] m1 ([n:Integer] => [n mul 2]) 4`,
		"[108]")
	fnValueM2Native(t, "def-split: apply seats in a def, tail applies the bound name",
		`def stage fn [[f:Function x:Integer] [Integer] [def r (f x) f r]] stage ([n:Integer] => [n add 1]) 5`,
		"[7]")
	fnValueM2Native(t, "trailing-spelling def-split twin",
		`def tds fn [[g:Function x:Integer] [Integer] [def r (x g) g r]] tds ([n:Integer] => [n add 1]) 5`,
		"[7]")
	fnValueM2Native(t, "bare multi-arg body tail (g x y) — the frame replay owns it",
		`def use2 fn [[g:Function x:Integer y:Integer] [Integer] [(g x y)]] use2 ([[a:Integer b:Integer] [Integer] [a sub b]] fn) 10 3`,
		"[7]")

	// CONTROLS — the single-applicable windows keep compiling natively.
	fnValueM2Native(t, "one applicable of two Function params",
		`def sel1 fn [[f:Function g:Function x:Integer] [Integer] [f x]] sel1 ([a:Integer] => [a mul 2]) ([b:Integer] => [b add 3]) 4`,
		"[8]")
	fnValueM2Native(t, "single param-fn apply at the body tail",
		`def apply1 fn [[fnv:Function] [Integer] [(fnv 100)]] apply1 ([y:Integer] => [y add 1])`,
		"[101]")
}

// fnValueM2NativeErrParity pins a fully-native compile whose RUN raises: the
// compiled and interpreted runs must agree on the error code. This is the
// convergence DynApplyLeadEligible relies on — inside a named-param fn unit
// the paren seals the frame off, so a runtime fn the one-arg window cannot
// satisfy no-matches IDENTICALLY under the leading and trailing spellings.
func fnValueM2NativeErrParity(t *testing.T, name, src, wantCode string) {
	t.Helper()
	prog, reason, _, cerr := mustNew(t).CompileCheck(src)
	if cerr != nil || prog == nil {
		t.Fatalf("%s: want a native compile, got refusal %q / err %v", name, reason, cerr)
	}
	_, compiled, errC := mustNew(t).RunCompiled(src)
	_, errI := mustNew(t).RunInterp(src)
	if noteCompileDefect(t, src, nil, errC) {
		return
	}
	if !compiled {
		t.Fatalf("%s: did not run compiled", name)
	}
	if codeOf(errC) != wantCode || codeOf(errI) != wantCode {
		t.Errorf("%s: compiled err=[%s] interp err=[%s], want both [%s]", name, codeOf(errC), codeOf(errI), wantCode)
	}
}

// TestLeadApplyArityMismatchParity pins the leading one-arg apply against
// runtime fns the window CANNOT satisfy — a 2-arg fn, a 0-arg fn, a
// multi-return fn. Both engines raise the identical error, which is what
// makes recording the trailing spelling's event for the leading source
// faithful at every runtime arity (not just the 1-arg happy path).
func TestLeadApplyArityMismatchParity(t *testing.T) {
	lead := `def ld fn [[g:Function x:Integer] [Integer] [(g x)]] ld `
	// The 2-arg row did NOT agree, and this test once asserted that it did
	// while reading its interp side from `Run` — the compiled lane (NUR106).
	// That was NUR107: the interpreter's word dispatch raises signature_error
	// on no-match, while the VM's dynamic apply read "no overload matched" as
	// "not callable" and left [fn, arg] as data, so the frame's return-count
	// check fired instead — or, where the declared return arity happened to
	// accept the two-value residual, nothing fired at all.
	//
	// CLOSED 2026-08-28: the VM now tells "not callable" from "a Function no
	// overload of which admits these arguments" and raises for the second, so
	// this row rejoins the shared parity helper exactly as its own note asked.
	fnValueM2NativeErrParity(t, "2-arg runtime fn under a 1-arg window",
		lead+`([[a:Integer b:Integer] [Integer] [a sub b]] fn) 14`, "signature_error")
	fnValueM2NativeErrParity(t, "0-arg runtime fn under a 1-arg window",
		lead+`(fn [[] [Integer] [42]]) 14`, "signature_error")
	fnValueM2NativeErrParity(t, "multi-return runtime fn under a 1-arg window",
		lead+`(fn [[x:Integer] [Integer Integer] [x x]]) 14`, "type_error")
}

// TestMultiArgChainedApplyRefuses pins the DELIBERATE edge of the Stage-G
// increment: the paren-collapse records only a SINGLE-argument leading apply
// (`(g x)` — the one arity where leading and trailing collection converge), so
// a CHAINED apply over a multi-arg inner group keeps refusing: `(g x y)` never
// collapses to an event, the outer window holds two applicable values, and
// noteDynFrameReplay declines. (The BARE multi-arg body tail `(g x y)` is a
// different shape — the single-applicable whole-frame replay owns it; pinned
// native above.) Sound whole-program refusal with interpreter parity.
func TestMultiArgChainedApplyRefuses(t *testing.T) {
	// Legacy refusal+fallback-parity contract, like TestApplyOverParamFnCompiles.

	fnValueM2Refusal(t, "chained apply over a two-arg inner group f (g x y)",
		`def c2 fn [[f:Function g:Function x:Integer y:Integer] [Integer] [f (g x y)]] c2 ([n:Integer] => [n mul 2]) ([[a:Integer b:Integer] [Integer] [a sub b]] fn) 10 3`,
		"unapplied fn-value in body residual")
}

// TestTailProofNegatives pins what the replayIsBodyTail windowReadsID
// widening (the def-split graduation's third step) must NOT arm: only a
// dyn-BIND of a value the window itself reads may be skipped by the
// event-order proof. An EFFECTFUL event between the apply and the tail, or
// a bind of an UNRELATED value, still declines — the RET-time replay would
// otherwise reorder the tail apply against it. Both refuse with
// interpreter-parity fallback.
func TestTailProofNegatives(t *testing.T) {

	fnValueM2Refusal(t, "an effect event between the def-split and the tail",
		`def ld fn [[g:Function x:Integer] [Integer] [def r (g x) print "mid" g r]] ld ([n:Integer] => [n mul 2]) 14`,
		"unapplied fn-value in body residual")
	fnValueM2Refusal(t, "a later bind of an UNRELATED value",
		`def ld fn [[g:Function x:Integer] [Integer] [g x def q 9 g q]] ld ([n:Integer] => [n mul 2]) 14`,
		"unapplied fn-value in body residual")
}
