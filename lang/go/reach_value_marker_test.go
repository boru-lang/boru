package lang

import (
	"fmt"
	"testing"
)

// TestReachValueMarkerIsNoArgument pins NUR262's close. The parser emits a
// dotted path's `/v` as the reach followed by a dispatch-modifier marker;
// inside a forward window the marker fell to the claim probe's literal arm,
// where an `Any` parameter matched it, so the reach's fn value read as a
// call head that would claim its own marker and `def g M.up1/v` collected
// nothing ("cannot call def … none were supplied"). The marker qualifies the
// value before it and is never an argument (ForwardClaimProbeOn's
// dispatch-modifier arm), so the value binds as `def g up1/v` always did.
func TestReachValueMarkerIsNoArgument(t *testing.T) {
	const mod1 = `import module [ def up1 fn [[value:Any] [String] ['UP']] export "M" {up1: up1/v} ] end `
	const mod2 = `import module [ def up fn [[value:Any opts:Map] [String] ['UP']] export "M" {up: up/v} ] end `
	for _, src := range []string{
		mod1 + "def g M.up1/v end g 1",
		mod1 + "def g M.up1/v",
		mod1 + "def g (M.up1/v) end g 1",
		mod2 + "def g M.up/v end g 1 {}",
		mod2 + "def g M.up/v",
		mod1 + "M.up1/v",
		mod1 + "7 M.up1/v",
		mod1 + "M.up1 5",
		`import module [ def up2 fn [[value:Integer opts:Map] [String] ['UP']] export "M" {up2: up2/v} ] end def g M.up2/v end g 1 {}`,
		"def up1 fn [[value:Any] [String] ['UP']] end def g up1/v end g 1",
		"def m {z: (fn [[] [Integer] [7]])} m.z/v",
		"(1 add 2)/s",
		// NUR213: a `/v`-marked MAP member with arguments beside it is data on
		// both lanes — the pass quotes the dynamic member read the marker
		// qualifies, and the residual layout leaves a quoted lead alone.
		"def m {f: (fn [[a:Integer] [Integer] [a add 1]])} m.f/v 5",
		"def m {f: (fn [[a:Integer] [Integer] [a add 1]])} m.f/v",
		"def m {f: (fn [[a:Integer] [Integer] [a add 1]])} m.f 5",
		"def m {f: (fn [[a:Integer] [Integer] [a add 1]])} 5 m.f",
	} {
		requireSameVerdict(t, src)
	}
	// The trailing spelling declines at an existing residual limit ("call
	// result above a literal") and the fallback answers as the interpreter.
	requireEngineParity(t, "def m {f: (fn [[a:Integer] [Integer] [a add 1]])} 5 m.f/v", false)
	for _, c := range []struct{ src, want string }{
		{mod1 + "def g M.up1/v end g 1", "[UP]"},
		{mod2 + "def g M.up/v end g 1 {}", "[UP]"},
		{mod1 + "def g M.up1/v", "[]"},
		{"def m {f: (fn [[a:Integer] [Integer] [a add 1]])} m.f/v 5", "[fn (Integer) 5]"},
		{"def m {f: (fn [[a:Integer] [Integer] [a add 1]])} 5 m.f/v", "[5 fn (Integer)]"},
	} {
		got, err := mustNew(t).RunInterp(c.src)
		if err != nil || fmt.Sprint(got) != c.want {
			t.Errorf("%s: interpreter = %v / %v, want %s", c.src, got, err, c.want)
		}
	}
}

// TestNamedValueNoMatchOnTheSeamRaises pins NUR261's close. A NAMED fn value
// driving fold whose signature stops matching past the first step raises
// uncalled_function on both lanes now — the token seam's unmatched-lambda
// arm applied the anonymous value's data rule (NUR155) to a named value,
// whose no-match is the word's raise (execFnDefLiteral). An anonymous
// lambda keeps the data rule.
func TestNamedValueNoMatchOnTheSeamRaises(t *testing.T) {
	for _, src := range []string{
		"def h fn [[a:Integer b:Integer] [List] [[a b]]] end 0 fold h/v [1 2]",
		"def h fn [[a:Integer b:Integer] [Any] [do [args]]] end 0 fold h/v [1 2]",
		"def h fn [[a:Integer b:Integer] [Integer] [a add b]] end 0 fold h/v ['x']",
		"def h fn [[a:Integer b:Integer] [Integer] [a add b]] end 0 fold h/v [1 2]",
		"def h fn [[a:Integer b:Integer] [List] [[a b]]] end 0 scan h/v [1 2]",
		"each ([x:Integer] => [typeof x]) [1 'a']",
		"def g ([x:Integer] => [typeof x]) end each g/v [1 'a']",
		"def g fn [[x:Integer] [Any] [typeof x]] end each g/v [1 'a']",
	} {
		requireSameVerdict(t, src)
	}
	for _, src := range []string{
		"def h fn [[a:Integer b:Integer] [List] [[a b]]] end 0 fold h/v [1 2]",
		"def g fn [[x:Integer] [Any] [typeof x]] end each g/v [1 'a']",
	} {
		if _, err := mustNew(t).RunInterp(src); err == nil {
			t.Errorf("%s: the interpreter must raise the named value's no-match", src)
		}
	}
	got, err := mustNew(t).RunInterp("each ([x:Integer] => [typeof x]) [1 'a']")
	if err != nil || fmt.Sprint(got) != "[[Integer fn (Integer)]]" {
		t.Errorf("anonymous lambda: interpreter = %v / %v, want [[Integer fn (Integer)]]", got, err)
	}
}

// TestClassMemberValueMarkerIsData pins NUR216's close — NUR213's twin for a
// CLASS member and for the window spellings. A `/v` after a member read says
// DATA: the run-time peek quotes the concrete fn and leaves it beside its
// neighbours. The pass quotes the carrier it holds (the standalone-marker
// drop), and now no residual arm applies a quoted value: the fn-typed
// carrier lead arm (`c.op/v 5` compiled 6 for `fn (Integer) 5`) and the
// verbatim window islands (`3 c.op/v 2` compiled [3 3], `3 4 c.op/v`
// compiled [3 5]; the map twins likewise). The class shapes compile and
// agree; the map window twins decline at the existing residual limit ("call
// result above a literal"), as NUR213's `5 m.f/v` does, and the fallback
// answers as the interpreter. The unmarked reads still apply.
func TestClassMemberValueMarkerIsData(t *testing.T) {
	const cls = `def T fnsig Integer Integer def C class {op:T} def c (make C {op:(fn [[x:Integer] [Integer] [x add 1]])}) `
	const anon = `def C class {op:(fnsig Integer Integer)} def c (make C {op:(fn [[x:Integer] [Integer] [x add 1]])}) `
	const m = `def m {f: (fn [[x:Integer] [Integer] [x add 1]])} `
	for _, c := range []struct{ src, want string }{
		{cls + "c.op/v 5", "[fn (Integer) 5]"},
		{cls + "5 c.op/v", "[5 fn (Integer)]"},
		{cls + "3 c.op/v 2", "[3 fn (Integer) 2]"},
		{cls + "3 4 c.op/v", "[3 4 fn (Integer)]"},
		{anon + "c.op/v 5", "[fn (Integer) 5]"},
		// The unmarked reads are calls, on both lanes.
		{cls + "c.op 5", "[6]"},
		{cls + "3 c.op 2", "[3 3]"},
		{m + "3 m.f 2", "[3 3]"},
		{m + "3 4 m.f", "[3 5]"},
	} {
		requireSameVerdict(t, c.src)
		if got, err := mustNew(t).RunInterp(c.src); err != nil || fmt.Sprint(got) != c.want {
			t.Errorf("%s: interpreter = %v / %v, want %s", c.src, got, err, c.want)
		}
	}
	for _, c := range []struct{ src, want string }{
		{m + "3 m.f/v 2", "[3 fn (Integer) 2]"},
		{m + "3 4 m.f/v", "[3 4 fn (Integer)]"},
	} {
		requireEngineParity(t, c.src, false)
		if got, err := mustNew(t).RunInterp(c.src); err != nil || fmt.Sprint(got) != c.want {
			t.Errorf("%s: interpreter = %v / %v, want %s", c.src, got, err, c.want)
		}
	}
}
