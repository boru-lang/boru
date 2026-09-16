package lang

import (
	"fmt"
	"strings"
	"testing"
)

// NUR144, resolved by refusal and retired (the sixty-seventh increment): an
// `undef` of an ENCLOSING binding from inside a speculative region — a loop
// body, a branch arm, an each or while body, an error handler, a `do` inside
// a loop, a fn body — is one the check pass keeps in its model (the
// wrapped-undef FP class: a region that never runs must raise nothing), so
// the compiled program never popped the binding and every later read stayed
// the pass's bake: NUR144's loop-body undef answered `7 7` for the
// interpreter's undefined_word, `if true [undef k] [] k` answered 5, and
// `while [k eq 5] [undef k]` never terminated. The recorder now refuses the
// program at the carried-undef site (one site, two shapes), and the
// interpreter owns every row — parity under the one-release hatch. The
// binder half places the transition and makes the reads live; until then a
// refusal is the sound answer, as the `each` body's transitions already had.
// An undef of a binding made INSIDE the region (a body def, a fn-local) is
// untouched and still compiles, and so is an undef of a name never bound at
// all (the gate silences the speculative diagnostic; nothing pops on either
// lane). The handler passes the fact to the recorder, whose own registry a
// module call in the same body can leave bound to the module (review of
// #463: the `M.m` row).
func TestSpeculativeUndefOfEnclosingBindingRefuses(t *testing.T) {
	t.Setenv("BORU_COMPILE_FALLBACK", "1")
	const reason = "undef of the enclosing binding `"
	refused := []string{
		// NUR144's two rows: the stack operand and the routed forward slot.
		`def k 5 end def f fn [[][Integer][k add 2]] end for 2 [ f  undef k ]`,
		`def k 5 end def go fn [[][Integer][add k 1]] end for 2 [ go  undef k ]`,
		// A top-level read after the loop, a branch arm inside the loop, a
		// `do` inside the loop (recording suspended), a plain branch arm, an
		// each body, a while body (the compiled program looped for ever), a
		// fn body, and the never-running error handler the leniency exists for.
		`def k 5 end for 2 [ undef k ] k`,
		`def k 5 end for 2 [ if (k eq 5) [undef k] [] ] 9`,
		`def k 5 end def f fn [[][Integer][k add 2]] end for 2 [ f  do [undef k] ]`,
		`def k 5 end if true [undef k] [] k`,
		`def k 5 end [1 2] each [undef k] k`,
		`def k 5 end while [k eq 5] [undef k] 9`,
		`def k 5 end def f fn [[][Integer][undef k 1]] end f k`,
		`def x 1 end do [7] error [undef x 9] x`,
		// A module call before the undef, in the same body (review of #463).
		`import module [def m fn [[] [Integer] [1]] export "M" {m:m/v}] end def k 5 end if true [M.m drop undef k 1] [1] k`,
	}
	for _, src := range refused {
		a, err := New()
		if err != nil {
			t.Fatal(err)
		}
		prog, got, _, cerr := a.CompileCheck(src)
		if cerr != nil {
			t.Fatalf("%q: %v", src, cerr)
		}
		if prog != nil || !strings.Contains(got, reason) {
			t.Errorf("%q: want the refusal %q…, got compiled=%v reason=%q", src, reason, prog != nil, got)
		}
		gotC, _, errC, gotI, errI := runBothEngines(t, src)
		requireParity(t, src, gotC, errC, gotI, errI)
	}
	// A binding made inside the region pops as it always did, compiled.
	compiled := []struct{ src, want string }{
		{`def k 5 end for 2 [ def k 6 undef k ] 9`, "[9]"},
		{`def f fn [[][Integer][def j 1 undef j 2]] end f`, "[2]"},
		{`def k 5 end for 2 [ def j 1 undef j ] 9`, "[9]"},
		{`def k 5 end undef k end def k 6 end k`, "[6]"},
		// A name never bound: nothing to pop on either lane.
		{`def f fn [[][Integer][undef nope 1]] end f`, "[1]"},
		{`def k 5 end if true [undef nope 1] [2] k`, "[1 5]"},
	}
	for _, c := range compiled {
		gotC, ran, errC, gotI, errI := runBothEngines(t, c.src)
		if !ran || errC != nil || errI != nil {
			t.Errorf("%q: an in-region undef compiles: ran=%v errC=%v errI=%v", c.src, ran, errC, errI)
			continue
		}
		if fmt.Sprint(gotC) != c.want || fmt.Sprint(gotI) != c.want {
			t.Errorf("%q: compiled=%v interp=%v, want %s", c.src, gotC, gotI, c.want)
		}
	}
}
