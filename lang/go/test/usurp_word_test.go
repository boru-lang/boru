package test

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
	"github.com/boru-lang/boru/lang/go"
	"github.com/boru-lang/boru/lang/go/native"
)

// The `usurp` word and `/u` suffix wrap a function so its signature
// argument order is reversed: a usurped fn called `usurped a b c`
// dispatches the original as `f c b a`. Like `ref`/`/v`, the wrapper is an
// unquoted Function value — it dispatches when args follow, and stays inert
// under `quote` or when reffed (`/uv`).

// lastInt returns the integer value of the final stack entry.
func lastInt(t *testing.T, res []native.Value) int64 {
	t.Helper()
	if len(res) == 0 {
		t.Fatalf("empty result stack")
	}
	n, err := native.AsInteger(res[len(res)-1])
	if err != nil {
		t.Fatalf("final value is not an integer: %v (%s)", err, res[len(res)-1].String())
	}
	return n
}

// lastString returns the string value of the final stack entry.
func lastString(t *testing.T, res []native.Value) string {
	t.Helper()
	if len(res) == 0 {
		t.Fatalf("empty result stack")
	}
	s, err := native.AsString(res[len(res)-1])
	if err != nil {
		t.Fatalf("final value is not a string: %v (%s)", err, res[len(res)-1].String())
	}
	return s
}

// TestUsurpWordReversesArgs checks the `usurp` word reverses arg delivery
// for a non-commutative fn: sub computes a-b, so usurp(sub) 10 3 = 3-10.
func TestUsurpWordReversesArgs(t *testing.T) {
	res, err := runNativeSteps(t, nil, []string{
		`def sub2 fn [[a:Integer b:Integer] [Integer] [a sub b]]`,
		`usurp (valof sub2) 10 3`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := lastInt(t, res); got != -7 {
		t.Errorf("usurp (valof sub2) 10 3 = %d, want -7", got)
	}
}

// TestUsurpModifierReversesArgs checks the /u suffix reverses the same way.
func TestUsurpModifierReversesArgs(t *testing.T) {
	res, err := runNativeSteps(t, nil, []string{
		`def sub2 fn [[a:Integer b:Integer] [Integer] [a sub b]]`,
		`sub2/u 10 3`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := lastInt(t, res); got != -7 {
		t.Errorf("sub2/u 10 3 = %d, want -7", got)
	}
}

// TestUsurpThreeArgFullReversal checks every position is reversed.
func TestUsurpThreeArgFullReversal(t *testing.T) {
	res, err := runNativeSteps(t, nil, []string{
		`def cat3 fn [[a:String b:String c:String] [String] [a add b add c]]`,
		`cat3/u 'x' 'y' 'z'`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := lastString(t, res); got != "zyx" {
		t.Errorf("cat3/u 'x' 'y' 'z' = %q, want \"zyx\"", got)
	}
}

// TestUsurpHeterogeneousSig checks the usurped sig matches reversed types:
// f's sig is [a:Integer b:String], so f/u expects [String Integer] and
// still binds a=Integer, b=String on the original.
func TestUsurpHeterogeneousSig(t *testing.T) {
	res, err := runNativeSteps(t, nil, []string{
		`def f fn [[a:Integer b:String] [String] [b add (a convert String)]]`,
		`f/u 'hi' 7`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := lastString(t, res); got != "hi7" {
		t.Errorf("f/u 'hi' 7 = %q, want \"hi7\"", got)
	}
}

// TestUsurpOneArgIsNoOp checks usurp of a 1-arg fn dispatches normally.
func TestUsurpOneArgIsNoOp(t *testing.T) {
	res, err := runNativeSteps(t, nil, []string{
		`def inc fn [[n:Integer] [Integer] [n add 1]]`,
		`inc/u 5`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := lastInt(t, res); got != 6 {
		t.Errorf("inc/u 5 = %d, want 6", got)
	}
}

// TestUsurpReffedIsInertData checks `/uv` (usurp + ref) with no following
// args leaves the wrapper on the stack as a Function value rather than
// dispatching it.
func TestUsurpReffedIsInertData(t *testing.T) {
	res, err := runNativeSteps(t, nil, []string{
		`def inc fn [[n:Integer] [Integer] [n add 1]]`,
		`inc/uv`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(res) != 1 {
		t.Fatalf("inc/uv left %d values, want 1 (the wrapper)", len(res))
	}
	if !core.ParentOf(&res[0]).Equal(core.TFunction) {
		t.Errorf("inc/uv top is %s, want Function", core.ParentOf(&res[0]).String())
	}
}

// TestUsurpReffedStaysInertWithTrailingArgs checks `/uv` mirrors `/v`: the
// reffed wrapper is left on the stack as data and bare trailing values just
// pile up beside it (no auto-dispatch). To invoke a reffed wrapper, wrap it
// in a paren — `(usurp (valof f)) a b` — or store and dot-dispatch it.
func TestUsurpReffedStaysInertWithTrailingArgs(t *testing.T) {
	res, err := runNativeSteps(t, nil, []string{
		`def sub2 fn [[a:Integer b:Integer] [Integer] [a sub b]]`,
		`sub2/uv 10 3`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	// [wrapper 10 3] — the wrapper sits inert below the two literals.
	if len(res) != 3 {
		t.Fatalf("sub2/uv 10 3 left %d values, want 3 (wrapper + 2 literals)", len(res))
	}
	if !core.ParentOf(&res[0]).Equal(core.TFunction) {
		t.Errorf("bottom value is %s, want the inert Function wrapper", core.ParentOf(&res[0]).String())
	}
}

// TestUsurpReffedInvokesViaParen checks a reffed wrapper dispatches when
// grouped in a paren with its args (the canonical way to call a captured
// function value).
func TestUsurpReffedInvokesViaParen(t *testing.T) {
	res, err := runNativeSteps(t, nil, []string{
		`def sub2 fn [[a:Integer b:Integer] [Integer] [a sub b]]`,
		`def usub (usurp (valof sub2))`,
		`usub 10 3`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := lastInt(t, res); got != -7 {
		t.Errorf("(usurp (valof sub2)) 10 3 = %d, want -7", got)
	}
}

// TestUsurpQuotedIsInertData checks `quote (usurp …)` captures the wrapper
// as data without invoking it.
func TestUsurpQuotedIsInertData(t *testing.T) {
	res, err := runNativeSteps(t, nil, []string{
		`def inc fn [[n:Integer] [Integer] [n add 1]]`,
		`quote (usurp (valof inc))`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(res) != 1 {
		t.Fatalf("quoted usurp left %d values, want 1", len(res))
	}
	if !core.ParentOf(&res[0]).Equal(core.TFunction) {
		t.Errorf("quoted usurp top is %s, want Function", core.ParentOf(&res[0]).String())
	}
}

// TestUsurpSurvivesRedefinition checks a usurped wrapper captured into a
// map is a stable handle to the fn it was minted from: dot-dispatching it
// after the original name is redefined still reverses the ORIGINAL
// subtraction (not the new addition).
func TestUsurpSurvivesRedefinition(t *testing.T) {
	res, err := runNativeSteps(t, nil, []string{
		`def sub2 fn [[a:Integer b:Integer] [Integer] [a sub b]]`,
		`def ops {rev: (usurp (valof sub2))}`,
		`def sub2 fn [[a:Integer b:Integer] [Integer] [a add b]]`,
		`ops.rev 10 3`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := lastInt(t, res); got != -7 {
		t.Errorf("captured usurped sub2 after redef = %d, want -7 (reversed subtraction)", got)
	}
}

// TestUsurpStoredInMapDispatches checks the canonical store-and-reuse path:
// a usurped wrapper captured in a map and dot-dispatched against args.
func TestUsurpStoredInMapDispatches(t *testing.T) {
	res, err := runNativeSteps(t, nil, []string{
		`def sub2 fn [[a:Integer b:Integer] [Integer] [a sub b]]`,
		`def ops {rev: (usurp (valof sub2))}`,
		`ops.rev 10 3`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := lastInt(t, res); got != -7 {
		t.Errorf("ops.rev 10 3 = %d, want -7", got)
	}
}

// TestUsurpOnUnboundNameErrors checks /u on an unbound name is an error.
func TestUsurpOnUnboundNameErrors(t *testing.T) {
	if _, err := runNativeSteps(t, nil, []string{`nope/u`}); err == nil {
		t.Fatalf("nope/u should error (undefined_word)")
	}
}

// TestUsurpOnNonFunctionIsIllegal checks /u on a plain value is rejected.
func TestUsurpOnNonFunctionIsIllegal(t *testing.T) {
	if _, err := runNativeSteps(t, nil, []string{`def x 5`, `x/u`}); err == nil {
		t.Fatalf("x/u on a non-fn binding should error (illegal_ref)")
	}
}

// TestUsurpWordOnNonFunctionIsIllegal checks the usurp word rejects a
// non-function argument.
func TestUsurpWordOnNonFunctionIsIllegal(t *testing.T) {
	if _, err := runNativeSteps(t, nil, []string{`def x 5`, `usurp x`}); err == nil {
		t.Fatalf("usurp x on a non-fn value should error (illegal_ref)")
	}
}

// TestUsurpCheckModeClean verifies the /u modifier flows through static
// check mode without an error-severity diagnostic (the /u stepWord branch
// mirrors /v's check-mode handling). Like the whole ref family, /u does not
// itself count as a "use" of the name, so an unused_def *warning* for the
// defined fn is expected and tolerated — we only reject errors.
func TestUsurpCheckModeClean(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	seedBoru(a)
	res, err := a.Check(`
		def sub2 fn [[a:Integer b:Integer] [Integer] [a sub b]]
		sub2/u 10 3
	`)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	for _, d := range res.Diagnostics {
		if d.Severity == "error" {
			t.Errorf("clean /u program produced an error diagnostic: %+v", d)
		}
	}
}

// TestUsurpCheckModeUndefined verifies /u on an unbound name surfaces a
// diagnostic in check mode rather than panicking.
func TestUsurpCheckModeUndefined(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	seedBoru(a)
	res, err := a.Check(`nope/u`)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if len(res.Diagnostics) == 0 {
		t.Errorf("nope/u should produce a diagnostic in check mode")
	}
}

// TestUsurpByNameReversesArgs checks the [Atom] overload: `usurp f`
// captures the bare word, resolves it to its bound function, and returns
// the argument-reversed wrapper — equivalent to `usurp (valof f)` and to
// the `f/u` suffix, without needing a paren-grouped value.
func TestUsurpByNameReversesArgs(t *testing.T) {
	res, err := runNativeSteps(t, nil, []string{
		`def sub2 fn [[a:Integer b:Integer] [Integer] [a sub b]]`,
		`usurp sub2 10 3`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := lastInt(t, res); got != -7 {
		t.Errorf("usurp sub2 10 3 = %d, want -7", got)
	}
}

// TestUsurpByNameMatchesValueForm pins that the by-name and by-value
// overloads agree across arities and dispatch positions.
func TestUsurpByNameMatchesValueForm(t *testing.T) {
	cases := []struct {
		name    string
		def     string
		byName  string
		byValue string
		wantInt int64
		isStr   bool
		wantStr string
	}{
		{"2-arg", `def sub2 fn [[a:Integer b:Integer] [Integer] [a sub b]]`,
			`usurp sub2 10 3`, `usurp (valof sub2) 10 3`, -7, false, ""},
		{"3-arg str", `def cat3 fn [[a:String b:String c:String] [String] [a add b add c]]`,
			`usurp cat3 'x' 'y' 'z'`, `usurp (valof cat3) 'x' 'y' 'z'`, 0, true, "zyx"},
		{"1-arg noop", `def inc fn [[n:Integer] [Integer] [n add 1]]`,
			`usurp inc 5`, `usurp (valof inc) 5`, 6, false, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rn, err := runNativeSteps(t, nil, []string{c.def, c.byName})
			if err != nil {
				t.Fatalf("by-name run: %v", err)
			}
			rv, err := runNativeSteps(t, nil, []string{c.def, c.byValue})
			if err != nil {
				t.Fatalf("by-value run: %v", err)
			}
			if c.isStr {
				if g1, g2 := lastString(t, rn), lastString(t, rv); g1 != c.wantStr || g2 != c.wantStr {
					t.Errorf("by-name=%q by-value=%q, want %q", g1, g2, c.wantStr)
				}
			} else {
				if g1, g2 := lastInt(t, rn), lastInt(t, rv); g1 != c.wantInt || g2 != c.wantInt {
					t.Errorf("by-name=%d by-value=%d, want %d", g1, g2, c.wantInt)
				}
			}
		})
	}
}

// TestUsurpByNameHeldWrapper checks `usurp f` with no trailing args holds
// the reversed wrapper as an inert Function value (no auto-fire), just
// like the by-value form.
func TestUsurpByNameHeldWrapper(t *testing.T) {
	res, err := runNativeSteps(t, nil, []string{
		`def sub2 fn [[a:Integer b:Integer] [Integer] [a sub b]]`,
		`usurp sub2`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(res) != 1 || !core.ParentOf(&res[len(res)-1]).Equal(core.TFunction) {
		t.Errorf("usurp sub2 = %v, want a single held Function value", res)
	}
}

// TestUsurpByNameNonFnRejected checks the by-name overload shares ref's
// rule: a non-function binding raises illegal_ref (matching `/u`), not a
// bare signature_error.
func TestUsurpByNameNonFnRejected(t *testing.T) {
	_, err := runNativeSteps(t, nil, []string{`def x 5`, `usurp x`})
	if err == nil {
		t.Fatalf("usurp x (x=5) should error")
	}
	if ae, ok := err.(*core.BoruError); !ok || ae.Code != "illegal_ref" {
		t.Errorf("usurp x error = %v, want illegal_ref", err)
	}
}

// TestUsurpByNameUnboundRejected checks an unbound name raises
// undefined_word (matching ref/`/u`).
func TestUsurpByNameUnboundRejected(t *testing.T) {
	_, err := runNativeSteps(t, nil, []string{`usurp nope`})
	if err == nil {
		t.Fatalf("usurp nope should error")
	}
	if ae, ok := err.(*core.BoruError); !ok || ae.Code != "undefined_word" {
		t.Errorf("usurp nope error = %v, want undefined_word", err)
	}
}

// TestUsurpWrapperBoundByName pins that a usurp wrapper bound to a name
// and called as a word dispatches correctly (the `def ifu (usurp if)`
// alias idiom). Regression for the InstallFnDef path that used to wrap
// the body-less wrapper sig in a boru body-runner, producing zero values
// and failing the fn return check.
func TestUsurpWrapperBoundByName(t *testing.T) {
	cases := []struct {
		name  string
		steps []string
		want  int64
	}{
		{"by value", []string{
			`def sub2 fn [[a:Integer b:Integer] [Integer] [a sub b]]`,
			`def fu (usurp (valof sub2))`,
			`fu 10 3`,
		}, -7},
		{"by name", []string{
			`def sub2 fn [[a:Integer b:Integer] [Integer] [a sub b]]`,
			`def fu (usurp sub2)`,
			`fu 10 3`,
		}, -7},
		{"infix form against named wrapper", []string{
			`def sub2 fn [[a:Integer b:Integer] [Integer] [a sub b]]`,
			`def fu (usurp sub2)`,
			`10 fu 3`,
		}, 7},
		{"anonymous original", []string{
			`def f ([a:Integer b:Integer] => [a sub b])`,
			`def g (usurp f)`,
			`g 10 3`,
		}, -7},
		{"cond-first if alias true", []string{`def ifu (usurp if)`, `true ifu 88 99`}, 99},
		{"cond-first if alias false", []string{`def ifu (usurp if)`, `false ifu 88 99`}, 88},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res, err := runNativeSteps(t, nil, c.steps)
			if err != nil {
				t.Fatalf("run: %v", err)
			}
			if got := lastInt(t, res); got != c.want {
				t.Errorf("got %d, want %d", got, c.want)
			}
		})
	}
}

// TestRefNativeBoundByName checks the sibling case: a referenced NATIVE
// fn bound to a name dispatches the native (InstallFnDef preserves the
// body-less native handler rather than running an empty boru body).
func TestRefNativeBoundByName(t *testing.T) {
	res, err := runNativeSteps(t, nil, []string{`def myadd (valof add)`, `myadd 2 3`})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := lastInt(t, res); got != 5 {
		t.Errorf("def myadd (valof add); myadd 2 3 = %d, want 5", got)
	}
}

// TestNamedBoruFnUnaffected guards that an ordinary named boru fn (and a
// boru fn re-bound through ref) still runs its body via the body-runner —
// the preserve-handler branch must NOT swallow Body-bearing sigs.
func TestNamedBoruFnUnaffected(t *testing.T) {
	res, err := runNativeSteps(t, nil, []string{
		`def sub2 fn [[a:Integer b:Integer] [Integer] [a sub b]]`,
		`def fu (valof sub2)`,
		`fu 10 3`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := lastInt(t, res); got != 7 {
		t.Errorf("def fu (valof sub2); fu 10 3 = %d, want 7 (body-runner preserved)", got)
	}
}

// TestUsurpRespectsOriginalBarrier checks that usurp reverses correctly for
// originals that are NOT all-forward: stack-only (`[| a b]`) and mixed
// (`[a | b]`) signatures. Regression for PR #111 review (codex P2): the
// re-dispatch used to always place the original before its args (forward
// form), so a stack-only original could not collect them and the wrapper
// was left inert. The handler now lays args out around the original per its
// BarrierPos, so every barrier reverses faithfully.
func TestUsurpRespectsOriginalBarrier(t *testing.T) {
	cases := []struct {
		name  string
		steps []string
		want  int64
	}{
		// stack-only original [| a b]: baseline `10 3 s` → a=3,b=10 → -7
		{"stackonly baseline", []string{
			`def s fn [[| a:Integer b:Integer] [Integer] [a sub b]]`, `10 3 s`}, -7},
		{"stackonly usurp word", []string{
			`def s fn [[| a:Integer b:Integer] [Integer] [a sub b]]`, `10 3 usurp s`}, 7},
		{"stackonly /u", []string{
			`def s fn [[| a:Integer b:Integer] [Integer] [a sub b]]`, `10 3 s/u`}, 7},
		{"stackonly named wrapper", []string{
			`def s fn [[| a:Integer b:Integer] [Integer] [a sub b]]`,
			`def su (usurp s)`, `10 3 su`}, 7},
		// mixed barrier [a | b]: baseline `3 m 10` → a=10,b=3 → 7
		{"mixed baseline", []string{
			`def m fn [[a:Integer | b:Integer] [Integer] [a sub b]]`, `3 m 10`}, 7},
		{"mixed usurp", []string{
			`def m fn [[a:Integer | b:Integer] [Integer] [a sub b]]`, `usurp m 10 3`}, -7},
		// all-forward control: unchanged
		{"forward control", []string{
			`def sub2 fn [[a:Integer b:Integer] [Integer] [a sub b]]`, `usurp sub2 10 3`}, -7},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res, err := runNativeSteps(t, nil, c.steps)
			if err != nil {
				t.Fatalf("run: %v", err)
			}
			if got := lastInt(t, res); got != c.want {
				t.Errorf("got %d, want %d", got, c.want)
			}
		})
	}
}
