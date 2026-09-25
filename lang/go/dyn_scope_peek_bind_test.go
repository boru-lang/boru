package lang

import (
	"strings"
	"testing"
)

// The dyn-body trigger the pins below share: a `do … error …` body whose
// closure probe declines (`rng`'s `for [a b]` over two computed bounds is
// the "computed range start/step" decline, utils/cut.boru's cut-pick-rng),
// so the dispatch takes the dyn-body path (tryRecordDynBody) and arms the
// program-wide DynEnv mode — every def in every unit then owes a
// registry-visible BIND_DYN_SCOPE twin.
const dynEnvTrigger = `def rng fn [[n:Integer][Integer][ def a (n add 1) def b (n add 3) def c (flex {n:0}) for [a b] [def _n (c set (quote n) ((c.n) add 1))] c.n ]] end `

// TestDynScopeDefOfBranchValueCompiles pins the fn-body def of a BRANCH
// value under DynEnv — `def ap2 (if (ap eq "") [""] [(join "" [" " ap])])`,
// boru:cli's cli-usage-line, which declined "dynamic-scope def `ap2` of
// unpromoted computed value" once utils/cut.boru's `do … error …` bodies
// armed DynEnv (cut.boru was the real-program ledger's last dyn-scope
// entry). A branch merge has no frame slot the promotion machinery can seat
// (planValueDefLocals promotes calls; promoteLateDynBind seats single-output
// calls only), but the def binds IMMEDIATELY after its value event, so the
// value is live on the sim top: the bind peeks it in place
// (OpBindDynScopePeek, the twin of the root write-back's fast path) and
// leaves it for its downstream readers.
func TestDynScopeDefOfBranchValueCompiles(t *testing.T) {
	for _, src := range []string{
		// The unit compiled BEFORE the arming (the late path).
		dynEnvTrigger + `def u fn [[s:String][String][ def ap2 (if (s eq "") [""] [(join "" [" " s])]) join "" ["usage:" ap2] ]] end def one fn [[n:Integer][Integer][ def a (do [ def _p (rng n) 0 ] error [def e end 1]) a ]] end (u "x") (u "") (one 3)`,
		// Two chained branch defs, the first read twice.
		dynEnvTrigger + `def u fn [[s:String][String][ def ap (if (s eq "") [""] [(join "" ["<" s ">"])]) def ap2 (if (ap eq "") [""] [(join "" [" " ap])]) join "" ["usage:" ap2 ap] ]] end def one fn [[n:Integer][Integer][ def a (do [ def _p (rng n) 0 ] error [def e end 1]) a ]] end (u "x") (u "") (one 3)`,
		// The branch value as the fn's own result.
		dynEnvTrigger + `def u fn [[s:String][String][ def ap2 (if (s eq "") [""] [(join "" [" " s])]) ap2 ]] end def one fn [[n:Integer][Integer][ def a (do [ def _p (rng n) 0 ] error [def e end 1]) a ]] end (u "x") (u "") (one 3)`,
		// The unit compiled AFTER the arming (the planned path, unchanged).
		dynEnvTrigger + `def one fn [[n:Integer][Integer][ def a (do [ def _p (rng n) 0 ] error [def e end 1]) a ]] end (one 3) def u fn [[s:String][String][ def ap2 (if (s eq "") [""] [(join "" [" " s])]) join "" ["usage:" ap2] ]] end (u "x") (u "")`,
	} {
		requireEngineParity(t, src, true)
	}
	dis := compileDisasm(t, dynEnvTrigger+`def u fn [[s:String][String][ def ap2 (if (s eq "") [""] [(join "" [" " s])]) join "" ["usage:" ap2] ]] end def one fn [[n:Integer][Integer][ def a (do [ def _p (rng n) 0 ] error [def e end 1]) a ]] end (u "x") (u "") (one 3)`)
	if !strings.Contains(dis, "BIND_DYN_SCOPE_PEEK") || strings.Contains(dis, "FALLBACK") {
		t.Errorf("the branch value's dyn-scope bind must peek the live value natively:\n%s", dis)
	}
}

// TestRetPinnedVariadicBranchFnCompiles pins a fn whose residual is a BRANCH
// over catch-variadic calls under a DECLARED return tuple — utils/cut.boru's
// cut-one, `if c [def a (do […] error […]) a] [def b (do […] error […]) b]`
// under `[Integer]` — called as a computed def under DynEnv (`def rc (one
// n)`, cut-walk), which declined "one: variadic fn result promoted to a
// frame slot". The `error` strip over a dyn-body result is real stack
// values before the frame's RET, where the declared tuple pins the count
// (the VM raises the interpreter's "expected N return value(s)"), so the
// branch inherits the RET-pinned marking (callVariadic) exactly as the
// direct dyn-body residual does, and the call site seats one value.
func TestRetPinnedVariadicBranchFnCompiles(t *testing.T) {
	for _, src := range []string{
		dynEnvTrigger + `def one fn [[n:Integer][Integer][ if (n gt 1) [def a (do [ def _p (rng n) 0 ] error [def e end 1]) a] [def b (do [ def _p (rng n) 2 ] error [def e end 1]) b] ]] end def wlk fn [[n:Integer acc:Integer][Integer][ if (n lt 0) [acc] [def rc (one n) def r (wlk (n sub 1) (acc add rc)) r] ]] end (wlk 3 0)`,
		// A diverging arm beside the catch arm.
		dynEnvTrigger + `def one fn [[n:Integer][Integer][ if (n gt 1) [def a (do [ def _p (rng n) 0 ] error [def e end 1]) a] [def b (do [ def _p (rng n) (raise bad_input "x") ] error [def e end 7]) b] ]] end def wlk fn [[n:Integer acc:Integer][Integer][ if (n lt 0) [acc] [def rc (one n) def r (wlk (n sub 1) (acc add rc)) r] ]] end (wlk 3 0)`,
		// The computed def at the unit root.
		dynEnvTrigger + `def one fn [[n:Integer][Integer][ if (n gt 1) [def a (do [ def _p (rng n) 0 ] error [def e end 1]) a] [def b (do [ def _p (rng n) 2 ] error [def e end 1]) b] ]] end def wlk fn [[n:Integer][Integer][ def rc (one n) (rc add 10) ]] end (wlk 3) (wlk 0)`,
	} {
		requireEngineParity(t, src, true)
	}
	// A catch arm whose body leaves TWO values under the one-value tuple is
	// the interpreter's count error; the shape keeps declining rather than
	// seating one value of two (the store would strand the other).
	src := dynEnvTrigger + `def one fn [[n:Integer][Integer][ if (n gt 1) [def a (do [ def _p (rng n) 0 ] error [def e end 1]) a] [def b (do [ def _p (rng n) 2 3 ] error [def e end 1]) b] ]] end def wlk fn [[n:Integer acc:Integer][Integer][ if (n lt 0) [acc] [def rc (one n) def r (wlk (n sub 1) (acc add rc)) r] ]] end (wlk 3 0)`
	gotC, _, errC, _, errI := runBothEngines(t, src)
	if errI == nil || !strings.Contains(errI.Error(), "expected 1 return value(s), got 2") {
		t.Errorf("the interpreter's count error is the oracle: %v", errI)
	}
	requireCompileDefect(t, src, gotC, errC)
	if errC == nil || !strings.Contains(errC.Error(), "variadic fn result promoted") {
		t.Errorf("a two-value catch arm must keep declining: %v", errC)
	}
}
