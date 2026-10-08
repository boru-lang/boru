package lang

import (
	"fmt"
	"strings"
	"testing"
)

// TestNUR210ComputedDoRunBeneathAndCollected pins NUR210's silent half. A
// computed `do` body (a body a fn returned) runs to its residual, and the
// interpreter splices the results back and re-steps them: each value lands
// above what lies beneath, and a trailing fn value applies over it. The
// compiled lane seated the run as its one recorded value. The trailing apply's
// rotation answered [1 9 2] for `9 do (mk)`'s [9 1 2], a list over the run
// assembled [1 [9 2]] and [1 [2]], and a lambda the body placed last stayed
// data. The prefix island re-steps the run through the interpreter's own
// machinery, over the constants beneath it or inside the list that collects
// it.
func TestNUR210ComputedDoRunBeneathAndCollected(t *testing.T) {
	mk := func(body string) string { return `def mk fn [[][List][quote [` + body + `]]] end ` }
	for _, c := range []struct{ src, want string }{
		{mk(`1 2`) + `9 do (mk)`, "[9 1 2]"},
		{mk(`1 2`) + `9 8 do (mk)`, "[9 8 1 2]"},
		{mk(`5 ([x:Integer] => [x add 1])`) + `9 do (mk)`, "[9 6]"},
		{mk(`([x:Integer] => [x add 1])`) + `9 do (mk)`, "[10]"},
		{mk(`7`) + `9 do (mk)`, "[9 7]"},
		{mk(``) + `9 do (mk)`, "[9]"},
		{mk(`1 2`) + `[9 do (mk)]`, "[[9 1 2]]"},
		{mk(`1 2`) + `[do (mk)]`, "[[1 2]]"},
		{mk(`1 2`) + `size [do (mk)]`, "[2]"},
		{mk(`5 ([x:Integer] => [x add 1])`) + `[do (mk)]`, "[[6]]"},
		{mk(`1 2`) + `[9 do (mk)] size`, "[3]"},
		{mk(`1 2`) + `def xs [9 do (mk)] end xs xs`, "[[9 1 2] [9 1 2]]"},
	} {
		requireEngineParity(t, c.src, true)
		if got, err := mustNew(t).RunInterp(c.src); err != nil || fmt.Sprint(got) != c.want {
			t.Errorf("%s: the interpreter answers %s, got %v / %v", c.src, c.want, got, err)
		}
	}
	// The lowering is the island: a mark before the run's chain, the
	// interpreter's re-step over the window, and a list's collect from its
	// own mark.
	if dis := compileDisasm(t, mk(`1 2`)+`9 do (mk)`); !strings.Contains(dis, "STACK_MARK") || !strings.Contains(dis, "CALL_DYN_MIXED_FROM_MARK") {
		t.Errorf("9 do (mk) islands its run over the prefix; got:\n%s", dis)
	}
	if dis := compileDisasm(t, mk(`1 2`)+`[9 do (mk)]`); !strings.Contains(dis, "MAKE_LIST_TO_MARK") {
		t.Errorf("[9 do (mk)] collects the island's results; got:\n%s", dis)
	}

	// Negative: a list over a run the island cannot seat (inside a fn body,
	// or reached through a branch) declines loudly, never assembling the
	// wrong count; the interpreter's answers stand.
	// (Main's region rule, #514, names the decline first on the merged tree.)
	const runtimeCount = "consumes loop results"
	requireLoudDecline(t, mk(`1 2`)+`def f fn [[][List][[9 do (mk)]]] end f`, runtimeCount, "[[9 1 2]]")
	requireLoudDecline(t, mk(`1 2`)+`def f fn [[][List][[do (mk)]]] end f`, runtimeCount, "[[1 2]]")
	requireLoudDecline(t, mk(`1 2`)+`def c true end [9 if c [do (mk)] [3]]`, runtimeCount, "[[9 1 2]]")
	// …and the seatings that never needed the island keep their answers.
	requireEngineParity(t, mk(`1 2`)+`do (mk) end 9`, true)
	requireEngineParity(t, `def ops [quote [1 add 2]] 9 do (ops get 0)`, true)
}

// TestNUR210ComputedBodyRebindsTheRoot pins NUR210's rebinding half: a
// computed `do` body at the root runs in the root's scope, so its defs and
// undefs are what a later read of the name sees. The compiled lane baked the
// read as the check pass's binding and led a dynamic apply over it that
// underflowed. After such a body a root read of a value binding seats live
// on the registry the body installed into (EmitState.rootDynLeak), a statement
// boundary between the body's run and the read is proven (NUR266), and a body
// that unbinds runs on the interpreter (NUR267). Since the merge of main's
// #514 main's kept-defs latch declines a later observer of a binding such a
// body may have changed; a read seated live, as these are, observes nothing
// stale and passes it (NUR282).
func TestNUR210ComputedBodyRebindsTheRoot(t *testing.T) {
	mk := func(body string) string { return `def mk fn [[][List][quote [` + body + `]]] end def x 99 end ` }
	for _, src := range []string{
		mk(`def x 5`) + `do (mk) end x`,   // [5]: the body's def
		mk(`def y 5`) + `do (mk) end x`,   // [99]: another name
		mk(`undef x`) + `do (mk) end x`,   // undefined_word: the body's undef
		mk(`def x 5 7`) + `do (mk) end x`, // [7 5]: the run, then the read
		mk(`1 2`) + `do (mk) end x`,       // a body that binds nothing
	} {
		requireEngineParity(t, src, true)
	}
	// Negative: the shapes the run's count still cannot seat stay loud —
	// never the read's stale value.
	for _, src := range []string{
		mk(`def x 5`) + `do (mk) end [x]`,
		mk(`def x 5`) + `do (mk) x`,
	} {
		gi, ei := mustNew(t).RunInterp(src)
		gc, ec := mustNew(t).Run(src)
		if fmt.Sprint(gc) == fmt.Sprint(gi) && fmt.Sprint(ec) == fmt.Sprint(ei) {
			continue
		}
		if ec == nil || !(strings.Contains(ec.Error(), "internal_error") || strings.Contains(ec.Error(), "compile_failed")) {
			t.Errorf("%s: the interpreter's answer or a loud failure, never another answer; got %v / %v", src, gc, ec)
		}
	}
}

// TestNUR210ComputedBodyGeneralisesTheRoot pins NUR210's last silent reads:
// after a computed `do` body at the root, a read the bare-read rule does not
// reach — a list or map literal's element, a def's operand, a fn unit's
// read — folded the value the pass held BEFORE the body (`[7 [99]]` for
// `[7 [5]]` over `[def x 5 7]`). The pass now stops knowing root values
// there: each root value binding is generalised in place (the speculative
// undef's transition), so every later read is live. Since the merge of main's
// #514 main's kept-defs latch passes these reads because they are seated live
// (NUR282); a read that is not — a fn unit's call, which the body may have
// redefined — declines.
func TestNUR210ComputedBodyGeneralisesTheRoot(t *testing.T) {
	mk := func(body string) string { return `def x 99 end def mk fn [[][List][quote [` + body + `]]] end ` }
	for _, c := range []struct{ src, want string }{
		{mk(`def x 5 7`) + `do (mk) end [x]`, "[7 [5]]"},
		{mk(`def x 5 7`) + `do (mk) end {a: x}`, "[7 {a:5}]"},
		{mk(`def x 5 7`) + `do (mk) end def y x end y`, "[7 5]"},
		{mk(`def x 5 7`) + `do (mk) end [x x]`, "[7 [5 5]]"},
		// Negative: a body that leaves x alone reads the pre-body value live.
		{mk(`7`) + `do (mk) end [x]`, "[7 [99]]"},
		{mk(`7`) + `do (mk) end def y x end y`, "[7 99]"},
	} {
		requireEngineParity(t, c.src, true)
		if got, err := mustNew(t).RunInterp(c.src); err != nil || fmt.Sprint(got) != c.want {
			t.Errorf("%s: the interpreter answers %s, got %v %v", c.src, c.want, got, err)
		}
	}
	src := mk(`undef x 7`) + `do (mk) end [x]`
	_, _, errC, _, errI := runBothEngines(t, src)
	if codeOf(errI) != "undefined_word" || codeOf(errC) != codeOf(errI) {
		t.Errorf("%s: undefined_word on both lanes, got compiled %v, interp %v", src, errC, errI)
	}
	requireDeclineReason(t, mk(`def x 5 7`)+`def f fn [[][Any][[x]]] end do (mk) end f`, keptDefsLatchReason)
}
