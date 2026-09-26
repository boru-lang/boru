package lang

import (
	"fmt"
	"strings"
	"testing"
)

// The recorder paths this branch added, pinned end to end on both lanes: each
// program reaches one of them, compiles, and agrees with the interpreter (or,
// for the one sound decline, declines with its reason while the interpreter
// answers).

// pr505DecMod exports a module fn whose call over `(true 5 M.dec)` is a
// DEFINITE no-match (bad binds 5, not a Boolean): inside a `do` body it is a
// unit-scoped trap (NUR134) — the body raises there, and `do`'s handler
// catches it.
const pr505DecMod = `import module [ def dec fn [[bad:Boolean x:Any] [Any] [ if bad [raise bad_input "boom"] [x] ]] export "M" {dec: dec/v} ] end `

// TestUnitTrapTailIsUnreachable pins the unit trap's two follow-ups: a second
// definite no-match in the same body is unreachable (the first trap wins), and
// a construct after the trap the recorder cannot lower does not decline the
// program — the unit raises at its trap and never reaches it. The loop body
// here (`1 fm.a`, a flex member read the pass types gradual) declines on its
// own (TestLoopReachSurvivorMultiValueDeclines); after the trap it is dead code
// on both lanes.
func TestUnitTrapTailIsUnreachable(t *testing.T) {
	for _, src := range []string{
		pr505DecMod + `def msg (do [(true 5 M.dec) (true 6 M.dec) "no-raise"] error [dot code]) msg`,
		pr505DecMod + `def fm (flex {a:7}) end def msg (do [(true 5 M.dec) for 2 [1 fm.a]] error [dot code]) msg`,
	} {
		agreeOnBothLanes(t, src, "[uncalled_function]")
	}
}

// TestLoopReachSurvivorMultiValueDeclines pins the multi-value arm of a loop
// body's per-iteration hazards (NUR129): a reach group's gradual survivor —
// `fm.a`, typed dynamic(Integer), so the body's Function screen passes it —
// beside another per-iteration value declines through the shared residual
// site, as the single-value body does. The interpreter re-steps the survivor
// at each iteration's end; here it holds an Integer, so it answers the plain
// values.
func TestLoopReachSurvivorMultiValueDeclines(t *testing.T) {
	const src = `def fm (flex {a:7}) end for 2 [1 fm.a]`
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	prog, reason, _, cerr := a.CompileCheck(src)
	if cerr != nil || prog != nil || !strings.Contains(reason, "reach group's survivor") || !strings.Contains(reason, "NUR129") {
		t.Errorf("%s: want the NUR129 survivor decline, got prog=%v reason=%q err=%v", src, prog != nil, reason, cerr)
	}
	b, _ := New()
	if got, errI := b.RunInterp(src); errI != nil || fmt.Sprint(got) != "[1 7 1 7]" {
		t.Errorf("%s: interpreted %v / %v, want [1 7 1 7]", src, got, errI)
	}
}

// TestLoopRebindOfBranchBoundNameCompiles pins NoteLoopCarried's standing-aside
// arm: a pre-loop binding the unit holds bound-checked under the loop body's
// def name (a branch-join binding one arm may have skipped) is not carried, and
// a body that does not read the name after the loop compiles and agrees on
// either arm.
func TestLoopRebindOfBranchBoundNameCompiles(t *testing.T) {
	const f = `def f fn [[c:Boolean][Any][if c [def x 1] [] for 3 [def x 5] 7]] end `
	agreeOnBothLanes(t, f+`f true`, "[7]")
	agreeOnBothLanes(t, f+`f false`, "[7]")
}

// TestDynamicKeepDefsBodyRefreshesTheCarriedSlot pins noteDynKeepDefsLeak's
// carried-slot store (NUR203 inside a loop): a keep-defs word over a DYNAMIC
// body inside a fn's loop may rebind a name the loop carries in a frame slot,
// so the slot is refreshed from the registry right after the call — the next
// iteration and the read after the loop see the body's per-element installs.
func TestDynamicKeepDefsBodyRefreshesTheCarriedSlot(t *testing.T) {
	agreeOnBothLanes(t,
		`def f fn [[b:List xs:List][Integer][def t 0 for 2 [def t (t add 1) each b xs drop] t]] end f (quote [def t (t add 1) t]) [1 2 3]`,
		"[8]")
}

// TestLoopRoundRollbackDropsArgsProjection pins Rollback's trim of the `args`
// projection map: a loop inside a fn whose body rebinds a name runs a second
// analysis round, and the first round's projection (recorded past the
// checkpoint) is dropped with its events, so the surviving round's own
// projection is the one the unit lowers.
func TestLoopRoundRollbackDropsArgsProjection(t *testing.T) {
	agreeOnBothLanes(t, `def f fn [[a:Integer b:Integer][Any][def acc 0 for 2 [def acc (acc add (args size))] acc]] end f 1 2`, "[4]")
}

// TestStoredFnBodyLocalReadIsTheWord pins NUR217's non-param arm: a stored fn's
// body result that is a bare read of a BODY-LOCAL def (`j`, bound to a member
// read the pass types gradual) is the interpreter's word dispatch when it holds
// a fn, and no seam can refuse a body-local's value — the stored unit declines,
// and the apply through the container member takes the interpreter's own
// dispatch at the seam. Both lanes call the fn (42) and pass data through (5).
func TestStoredFnBodyLocalReadIsTheWord(t *testing.T) {
	const g = `def g fn [[m:Map][Any][def j (m get "f") j]] end def w {g: g/v} end `
	agreeOnBothLanes(t, g+`w.g {f: ([] => [42])}`, "[42]")
	agreeOnBothLanes(t, g+`w.g {f: 5}`, "[5]")
}
