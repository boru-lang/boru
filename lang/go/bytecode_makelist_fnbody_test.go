package lang

import (
	"fmt"
	"strings"
	"testing"
)

// TestComputedListInFnBody pins the fix for the difficulty-5 make/list provenance
// leaf (VOXGIG-COMPILE-COMPLETION-PLAN.0.md §1.3): a COMPUTED list literal CONSUMED
// as an argument inside a fn body (`flex [i 99]`, `f [j (g x)]`) now lowers to
// OpMakeList instead of declining — the interpreter auto-evaluates the consumed arg
// against the live def stack, so its element locals/params resolve exactly as
// OpMakeList re-pushes them per call.
//
// SOUNDNESS (the plan's explicit miscompile warning): a fn body RETURNING a bare-word
// list (`[y y]` as the RESIDUAL) raises undefined_word at run time (the residual
// sub-engine lacks the body's def-locals), so it must NOT bake OpMakeList (which
// would resolve y to its frame slot → [6,6], diverging). It stays unresolved and
// falls back. Only the CONSUMED arg eval records.
func TestComputedListInFnBody(t *testing.T) {
	// MUST compile natively (no island) + RunCompiledStrict == Run.
	strict := []struct{ name, src, want string }{
		{"flex over a computed list (param element)",
			`def f fn [[i:Integer][Any][ (flex [i 99]) ]] (f 7)`, "[[7 99]]"},
		{"computed list bound then sized",
			`def f fn [[i:Integer][Integer][ def xs [i 99] (xs size) ]] (f 7)`, "[2]"},
		{"computed list with a nested call element",
			`def g fn [[x:Integer][Integer][(x add 1)]] def f fn [[i:Integer][Integer][ def xs [i (g i)] (xs size) ]] (f 4)`, "[2]"},
	}
	for _, c := range strict {
		t.Run("strict/"+c.name, func(t *testing.T) {
			a, _ := New()
			prog, reason, _, _ := a.CompileCheck(c.src)
			if prog == nil {
				t.Fatalf("must compile natively, declined: %q", reason)
			}
			if strings.Contains(prog.Disassemble(), "FALLBACK") {
				t.Errorf("%s must compile native (no island)", c.name)
			}
			got, err := a.RunCompiledStrict(c.src)
			if err != nil {
				t.Fatalf("RunCompiledStrict: %v", err)
			}
			b, _ := New()
			want, _ := b.RunInterp(c.src)
			if fmt.Sprint(got) != fmt.Sprint(want) {
				t.Errorf("compiled %v != interpreter %v (MISCOMPILE)", got, want)
			}
			if fmt.Sprint(got) != c.want {
				t.Errorf("got %v, want %s", got, c.want)
			}
		})
	}

	// SOUNDNESS (fallback allowed): compile == interpret for shapes that must NOT
	// record OpMakeList — the residual bare-word list (the plan's [y y] miscompile),
	// and a stateful generator (must re-run per element, never freeze one roll).
	sound := []struct{ name, src string }{
		// A fn RETURNING `[y y]` raises undefined_word; compiled must ALSO error
		// (decline → fallback raises the same), never bake [6,6].
		{"residual bare-word list must not bake", `def f fn [[x:Integer][List][ def y (x add 1) [y y] ]] def r (f 5) (r size)`},
		// A stateful generator list element re-runs per element; freezing one roll
		// would diverge — must stay a fallback.
		{"stateful generator list not frozen", `def f fn [[][List][ list-of [Rand.int 0 100] 3 ]] def xs (f) (xs size)`},
	}
	for _, c := range sound {
		t.Run("sound/"+c.name, func(t *testing.T) {
			a, _ := New()
			got, _, err := a.RunCompiled(c.src) // fallback allowed
			b, _ := New()
			want, werr := b.RunInterp(c.src)
			if (err == nil) != (werr == nil) {
				t.Fatalf("error mismatch: compiled=%v interp=%v", err, werr)
			}
			if err == nil && fmt.Sprint(got) != fmt.Sprint(want) {
				t.Errorf("compiled %v != interpreter %v (MISCOMPILE)", got, want)
			}
		})
	}
}
