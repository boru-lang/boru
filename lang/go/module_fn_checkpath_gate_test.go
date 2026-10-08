package lang

import (
	"fmt"
	"testing"
)

// The in-repo decision-grade gate for the module-fn check path
// (design/module-fn-checkstate-ownership.{0,1}.md, step 0).
//
// WHY THIS EXISTS. The module-fn body-compilation refactor (the CheckState /
// EmitState ownership split) is gated on a gate the in-repo corpus did NOT
// previously provide: `.0` is a negative result precisely BECAUSE `make test`,
// TestSpecCompiledOrFallback, and the 5k fuzzer were all GREEN while a naive
// version of that change was broken — the breakage (+29 / +39 spurious
// `uncalled_function` errors from same-named parent/module fns colliding across
// the registry boundary) only surfaced under an external `diverge.sh`. This test
// closes that hole inside the repo: it asserts, over programs that drive the
// native sub-registry module-fn check path, that (a) the compiled result is
// byte-identical to the interpreter (or falls back — both sound) and (b) the
// check pass emits ZERO error-severity diagnostics. A pure output differential
// would miss (b) when the program falls back; the diagnostic-count baseline is
// what catches the collision breakage.
//
// WHY boru:test. boru:test IS a native sub-registry module (BuildTestModule) whose
// preamble defines boru fns with named params + real bodies (`run-spec`,
// `run-cases`, `run-case`) — the exact CallBoru-across-the-module-boundary path
// the refactor touches, and the literal locus of module-test.tsv:38. The cases
// below cover the §7 surface reachable with an in-repo module: a SUBJECT-style
// dynamic dispatch (run-case invokes the spec's subject), CROSS-BOUNDARY
// RECURSION (run-spec recurses over sub-specs), and a SAME-NAMED parent/module
// fn collision (a program-level `run-case` shadowing the module's preamble
// `run-case` — the §4 scopeID collision). A future purpose-built fixture could
// add generic `decide gen [...]` dispatch + a predicate-refine calling a module
// fn; this gate already pins the cross-registry surface that actually broke.
//
// GATE SEMANTICS. The assertions are refactor-agnostic: they pass TODAY (the
// programs fall back) AND after a correct refactor (they compile to the same
// result). They fail only on a real DIVERGENCE or on SPURIOUS check-pass errors
// — exactly the two failure modes the ownership refactor risks.
func TestModuleFnCheckPathGate(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{
			// subject dispatch + multiple cases (module-test.tsv:38)
			"run-spec subject dispatch, 2 cases",
			`import "boru:test"  def double fn [[n:Integer][Integer][n 2 mul]] end ` +
				`def s {name:"d" subject:double/q cases:[{name:"a" in:[3] out:6} {name:"b" in:[0] out:0}] subs:[]} end ` +
				`s Test.run-spec end Test.summary`,
		},
		{
			// cross-boundary recursion: run-spec recurses over sub-specs
			"run-spec recursion over sub-specs",
			`import "boru:test"  def inc fn [[n:Integer][Integer][n 1 add]] end ` +
				`def child {name:"c" subject:inc/q cases:[{name:"a" in:[1] out:2}] subs:[]} end ` +
				`def top {name:"t" subject:inc/q cases:[{name:"b" in:[2] out:3}] subs:[child]} end ` +
				`top Test.run-spec end Test.summary`,
		},
		{
			// a program fn whose NAME collides with a module preamble fn
			// (run-case) — the §4 same-name-across-the-boundary case
			"program fn name-colliding with module preamble fn",
			`import "boru:test"  def run-case fn [[x:Integer][Integer][x 100 add]] end ` +
				`def sq fn [[n:Integer][Integer][n n mul]] end ` +
				`def s {name:"q" subject:sq/q cases:[{name:"a" in:[3] out:9}] subs:[]} end ` +
				`s Test.run-spec end (run-case 5)`,
		},
		{
			// a FAILING case is recorded (not a crash / not a spurious error)
			"run-spec with a failing case",
			`import "boru:test"  def double fn [[n:Integer][Integer][n 2 mul]] end ` +
				`def s {name:"d" subject:double/q cases:[{name:"a" in:[3] out:999}] subs:[]} end ` +
				`s Test.run-spec end Test.summary`,
		},
		{
			// MERGED-WORD seam (Stage M1): a module-defined `add` transplanted
			// onto the importer's bare word dispatches through the ReturnsFn-
			// boundary check-state share (shareCheckStateFrom). The historical
			// failure mode of check-state sharing was +29/+39 spurious
			// diagnostics corpus-wide (invisible to the differential) — this
			// fixture extends the (b) zero-error gate to the merged seam.
			"merged-word extension dispatch (open words)",
			`import module [def Flag (refine Boolean) ` +
				`def add fn [[a:Flag b:Flag] [Boolean] [a and b]] ` +
				`def mk fn [[b:Boolean] [Flag] [def v:Flag b v]] ` +
				`export "M" {add: add/v mk: mk/v Flag: Flag}] end ` +
				`add (M.mk true) (M.mk false)`,
		},
		{
			// merged-word seam + same-name collision: the importer ALSO calls
			// the builtin `add` and the module's merged overload in one
			// program — the scopeID-keyed memos must keep the two apart.
			"merged-word extension beside the builtin word",
			`import module [def Flag (refine Boolean) ` +
				`def add fn [[a:Flag b:Flag] [Boolean] [a and b]] ` +
				`def mk fn [[b:Boolean] [Flag] [def v:Flag b v]] ` +
				`export "M" {add: add/v mk: mk/v Flag: Flag}] end ` +
				`(add 1 2) end add (M.mk true) (M.mk true)`,
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

			// (a) compiled (or fallback) result is byte-identical to the
			// interpreter — value AND error taxonomy.
			if fmt.Sprint(want) != fmt.Sprint(got) || (werr == nil) != (gerr == nil) {
				t.Fatalf("module-fn check path diverged\n  interp: %v (err=%v)\n  comp:   %v (err=%v)",
					want, werr, got, gerr)
			}

			// (b) the check pass emits NO error-severity diagnostics. The
			// ownership-refactor breakage manifested as +29/+39 spurious
			// uncalled_function errors here; a non-zero count catches it.
			cc, _ := New()
			res, _ := cc.Check(c.src)
			var nErr int
			for _, d := range res.Diagnostics {
				if d.Severity == SeverityError {
					nErr++
				}
			}
			if nErr != 0 {
				t.Fatalf("module-fn check path emitted %d spurious error-diagnostic(s) (want 0): %+v",
					nErr, res.Diagnostics)
			}
		})
	}
}
