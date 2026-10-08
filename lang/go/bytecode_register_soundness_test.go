package lang

import (
	"fmt"
	"testing"
)

// A def-bound parser fn is dispatched later through the bound name — bare
// direct call, the `parse` sugar, or the desugared `<name> <src> <opts> end`
// form. Historically (when the binding came from the removed
// ParseLang.register) the bytecode recorder const-folded the export lookup
// to a missing-key None using the check-time module snapshot, dropped the
// dispatch, and left the call's args on the stack — the compiled program
// silently DIVERGED from the interpreter. The kind namespace is frozen now
// and the binding is an ordinary def, but the invariant this test guards is
// unchanged: whichever path RunCompiled takes (compile or fall back), the
// result (value + error presence) must match the interpreter exactly.
func TestBoundParserDispatchNeverDiverges(t *testing.T) {
	const bind = `import "boru:parselang"  import "boru:string-util"  ` +
		`def calc (fn [[source:Any opts:Map] [List] [StringUtil.split ' ' (ParseLang.source source)]])  `
	cases := []struct {
		name string
		src  string
	}{
		{
			"parselang bare dispatch (no trailing get)",
			bind + `calc 'x + y' {} end`,
		},
		{
			"parselang sugar + get (already a spec row)",
			bind + `(parse calc 'x + y') get 1`,
		},
		{
			"parselang desugared + get",
			bind + `(calc 'x + y' {} end) get 1`,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ia, _ := New()
			want, werr := ia.RunInterp(c.src)

			ca, _ := New()
			got, _, gerr := ca.RunCompiled(c.src)
			if noteCompileDefect(t, c.src, got, gerr) {
				return
			}

			// Whether the program compiled or fell back, its result must match
			// the interpreter exactly (value + error presence).
			if fmt.Sprint(want) != fmt.Sprint(got) || (werr == nil) != (gerr == nil) {
				t.Fatalf("compiled result diverged from interpreter\n  interp: %v (err=%v)\n  comp:   %v (err=%v)",
					want, werr, got, gerr)
			}
		})
	}
}

// Control: a plain static export read (ParseLang.kinds) must COMPILE — the
// frozen-registry tombstone does not taint the rest of the module.
func TestNonRegisterModuleWordStillCompiles(t *testing.T) {
	src := `import "boru:parselang"  ParseLang.kinds`
	a, _ := New()
	got, err := a.RunCompiledStrict(src)
	if err != nil {
		t.Fatalf("ParseLang.kinds should compile, got compile failure: %v", err)
	}
	b, _ := New()
	want, _ := b.RunInterp(src)
	if fmt.Sprint(want) != fmt.Sprint(got) {
		t.Fatalf("kinds compiled result %v != interpreter %v", got, want)
	}
}
