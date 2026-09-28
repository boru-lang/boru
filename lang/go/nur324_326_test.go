package lang

import (
	"strings"
	"testing"
)

// TestNUR324NoneLiteralIsAUnionMember pins NUR324's close. A missing member
// reads as the None literal, which dispatch treats as a value — yet a union
// that names None refused it (its Match took any bare node for "the type
// itself", and rejectsTypeLiteral carved only a None slot), so `7 f m.b`
// over `x:Maybe` filled x from the stack's 7 while the check pass, whose
// None carrier the union admits, committed m.b to x and raised. Inline
// unions and `is` already answered true: the spelling of a union must not
// change its membership. The None literal is now a member wherever `none`
// is; a type literal is still no member of a value union.
func TestNUR324NoneLiteralIsAUnionMember(t *testing.T) {
	const maybe = `def Maybe (Integer tor None) end def f fn [[x:Maybe][Any][[x]]] end def m {a: 1} end `
	for _, c := range []struct{ src, want string }{
		{maybe + `7 f m.b`, "[7 [None]]"},
		{maybe + `7 f (m.b)`, "[7 [None]]"},
		{maybe + `m.b f`, "[[None]]"},
		{maybe + `7 m.b f`, "[7 [None]]"},
		{maybe + `m.b is Maybe`, "[true]"},
		{`def m {a: 1} end (m.b) is (Integer tor None)`, "[true]"},
		{maybe + `7 f none`, "[7 [none]]"},
		{maybe + `7 f 5`, "[7 [5]]"},
		// Refused where None is no member: a union without it, a value slot,
		// and a type literal at the value union.
		{`def U (Integer tor String) end def f fn [[x:U][Any][[x]]] end def m {a: 1} end m.b f`, "ERROR:cannot call `f`"},
		{`def U (Integer tor String) end def m {a: 1} end m.b is U`, "[false]"},
		{maybe + `f Integer`, "ERROR:cannot call `f`"},
		{maybe + `Integer is Maybe`, "[false]"},
		// Past the union, a narrower return contract names the None it met.
		{`def Maybe (Integer tor None) end def m {a: 1} end def f fn [[x:Maybe][Integer][x]] end f m.b`, "ERROR:expected Integer, got None"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
}

// TestNUR325RunDependentAnnotationDeclines pins NUR325's close. A
// parenthesised annotation is RUN to build the signature, and the check pass
// ran it over carriers: `x:(1 add 2)` — the run's value pattern 3 — became an
// Integer type, `x:(typeof 5)` the Type carrier's slot, and the bind twin
// replayed that fn, so the compiled lane bound 4 or 's' where the run
// refuses them. A carrier with no type content is a value only the run
// computes: the fn word declines as the compile-time word it is (loud),
// while a carrier holding the type itself (`tnot`) still compiles.
func TestNUR325RunDependentAnnotationDeclines(t *testing.T) {
	const why = "compile-time word"
	for _, c := range []struct{ src, want string }{
		{`def f fn [[x:(1 add 2)][Any][[x]]] end f 3`, "[[3]]"},
		{`def f fn [[x:(typeof 5)][Any][[x]]] end f 7`, "[[7]]"},
		{`def f fn [[x:(not Integer)][Any][[x]]] end f true`, "[[true]]"},
		{`def f fn [[x:(1 add 2)][Any][[x]] [x:Integer][Any][['other' x]]] end f 3`, "[[3]]"},
		{`def f fn [[x:Integer][(typeof 1)][x]] end f 3`, "[3]"},
	} {
		requireLoudDecline(t, c.src, why, c.want)
	}
	for _, src := range []string{
		`def f fn [[x:(1 add 2)][Any][[x]]] end f 4`,
		`def f fn [[x:(typeof 5)][Any][[x]]] end f 's'`,
		`def k 5 end def f fn [[x:(k add 0)][Any][[x]]] end f 4`,
		`def f fn [[x:(String tor (typeof 1))][Any][[x]]] end f 4.5`,
	} {
		requireLoudDeclineErr(t, src, why, "signature_error")
	}
	requireLoudDeclineErr(t, `def f fn [[x:Integer][(1 add 2)][x]] end f 4`, why, "type_error")
	// Annotations the pass holds exactly still compile.
	for _, c := range []struct{ src, want string }{
		{`def f fn [[x:(tnot Integer)][Any][[x]]] end f 's'`, "[['s']]"},
		{`def f fn [[x:(Integer tor String)][Any][[x]]] end 7 f 's'`, "[7 ['s']]"},
		{`def f fn [[x:(5 gt 3)][Any][[x]]] end f true`, "[[true]]"},
		{`def f fn [[x:(5 gt 3)][Any][[x]]] end f 's'`, "ERROR:cannot call `f`"},
		{`def f fn [[x:(size [1 2])][Any][[x]]] end f 5`, "ERROR:cannot call `f`"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
}

// TestNUR326InlineNegationParamIsEnforced pins NUR326's close. An inline
// negation annotation (`x:(tnot Integer)`) resolved to the TAny tail of
// ResolveSigType — the silent wildcard the inline union closed long ago — so
// both lanes bound 5, which `5 is (tnot Integer)` and the named form refuse.
// It now constrains through the pattern, as the union does; the check pass
// holds tnot's exact negation as that pattern (not an abstract carrier).
func TestNUR326InlineNegationParamIsEnforced(t *testing.T) {
	const tnot = `def f fn [[x:(tnot Integer)][Any][[x]]] end `
	for _, c := range []struct{ src, want string }{
		{tnot + `f 5`, "ERROR:5 does not satisfy its declared pattern tnot Integer"},
		{tnot + `'s' f 5`, "ERROR:cannot call `f`"},
		{tnot + `f 5.5`, "[[5.5]]"},
		{tnot + `7 f 's'`, "[7 ['s']]"},
		{`def f fn [[x:(tnot (Integer tor String))][Any][[x]]] end f 's'`, "ERROR:cannot call `f`"},
		{`def f fn [[x:(tnot (Integer tor String))][Any][[x]]] end f 5.5`, "[[5.5]]"},
		{`def f fn [[x:(tnot Integer) y:Integer][Any][[x y]]] end f 's' 5`, "[['s' 5]]"},
		{`def N (tnot Integer) end def f fn [[x:N][Any][[x]]] end f 5`, "ERROR:cannot call `f`"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
}

// TestNUR311CallWindowStopsAtAMissingRead pins NUR311's third path: a user
// call's contract failure renders the attempted window, and the check pass
// held a missing member's read as a carrier there; the interpreter's walk
// over the written operands stops at the None it reads and fills from the
// stack beneath. The contract window records which operands were written
// (CallWindowOperand.Fwd) and the prefix it fills from (PrefixOnly), so the
// VM stops where the interpreter does — `9 f m.b 3` names 9, not m.b and 3.
func TestNUR311CallWindowStopsAtAMissingRead(t *testing.T) {
	const f2 = `def f fn [[x:Integer y:Integer][Any][x]] end def m {a: 1} end `
	const noF = "ERROR:cannot call `f`"
	for _, c := range []struct{ src, want string }{
		{f2 + `9 f m.b 3`, noF},
		{f2 + `f m.b 3`, noF},
		{f2 + `f 3 m.b`, noF},
		{`def f fn [[x:Integer][Any][x]] end def m {a: 1} end f m.b`, noF},
		// The stack's 5 fills x where the read type stops the walk.
		{`def f fn [[x:Integer][Any][x]] end def m {e: Integer} end 5 f m.e`, "[5 Integer]"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
}

// TestNUR327TypedContainerParenChild pins NUR327's close. A typed
// container's paren child (`[:(Integer tor None)]`) reaches `is` and a type
// def unevaluated — only a fn parameter and a typed def resolved it — so no
// element satisfied the raw paren: `['a' 1] is [:(Integer tor String)]` was
// false on the interpreter, and `def T [:(Integer tor None)]` refused every
// list on both lanes. The inline `is` form declines compiled (its operand
// has no compiled home), as it did.
func TestNUR327TypedContainerParenChild(t *testing.T) {
	const noHome = "operand of unknown provenance"
	for _, c := range []struct{ src, want string }{
		{`[5 1] is [:(Integer tor None)]`, "[true]"},
		{`['a' 1] is [:(Integer tor String)]`, "[true]"},
		{`[5.5 1] is [:(Integer tor String)]`, "[false]"},
		{`{a: 5} is {:(Integer tor None)}`, "[true]"},
	} {
		requireLoudDecline(t, c.src, noHome, c.want)
	}
	const tl = `def T [:(Integer tor None)] end `
	for _, c := range []struct{ src, want string }{
		{tl + `[5 1] is T`, "[true]"},
		{tl + `[5.5] is T`, "[false]"},
		{tl + `def f fn [[xs:T][Any][xs]] end f [5 1]`, "[[5 1]]"},
		{tl + `def f fn [[xs:T][Any][xs]] end f ['s']`, "ERROR:cannot call `f`"},
		{`def T {:(Integer tor String)} end {a: 5 b: 's'} is T`, "[true]"},
		{`def T {:(Integer tor String)} end {a: 5.5} is T`, "[false]"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
	// A child only the run computes declines the def that builds the
	// container, a type def's and a typed def's alike (the pass replayed a
	// Type-carrier child: `def xs:[:(typeof y)] [5]` raised compiled).
	for _, c := range []struct{ src, want string }{
		{`def x 5 end def T [:(typeof x)] end [5] is T`, "[true]"},
		{`def T [:(1 add 2)] end [3] is T`, "[true]"},
		{`def y 5 end def xs:[:(typeof y)] [5] xs`, "[[5]]"},
		{`def xs:(typeof 5) 5 xs`, "[5]"},
		{`def f fn [[xs:[:(typeof 5)]][Any][xs]] end f [5]`, "[[5]]"},
	} {
		requireLoudDecline(t, c.src, "compile-time word", c.want)
	}
	requireLoudDeclineErr(t, `def xs:[:(typeof 5)] ["s"] xs`, "compile-time word", "type_error")
	// A child that cannot run is an error where it was a silent refusal.
	for _, src := range []string{`[5] is [:(nosuchword)]`, `def T [:(nosuchword)] end 1`} {
		if _, err := mustNew(t).RunInterp(src); err == nil || !strings.Contains(err.Error(), "undefined word: nosuchword") {
			t.Errorf("%s: want the child's undefined_word, got %v", src, err)
		}
	}
}

// TestNUR328TypeValuesAtUserParams pins NUR328's close. A type value or a
// None read met a user fn's parameter three ways the lanes disagreed on:
// the compiled contract asked the type match alone and bound the Integer
// TYPE at `x:Integer` (the interpreter's plan refuses a type at a value
// slot); the check pass refused its own Type and None carriers at a Type
// slot, which takes the run's type literal and None; and a root node (None)
// passed into a fn unit carried no type at all.
func TestNUR328TypeValuesAtUserParams(t *testing.T) {
	const pre = `def Maybe (Integer tor None) end def m {a: 1 e: Integer} end def xs [1 Integer] end `
	const fi = `def f fn [[x:Integer][Any][[x]]] end `
	const ft = `def f fn [[x:Type][Any][[x]]] end `
	const fm = `def f fn [[x:Maybe][Any][[x]]] end `
	const noF = "ERROR:cannot call `f`"
	for _, c := range []struct{ src, want string }{
		{pre + fi + `7 m.e f`, noF},
		{pre + fi + `def g fn [[][Any][f m.e]] end g`, noF},
		{pre + fi + `def h fn [[y:Any][Any][f y]] end h m.e`, noF},
		{pre + fi + `def h fn [[y:Any][Any][f y]] end h Integer`, noF},
		{pre + fi + `def h fn [[y:Any][Any][f y]] end h (xs get 1)`, noF},
		{pre + fi + `def h fn [[y:Any][Any][f y]] end h 5`, "[[5]]"},
		{pre + ft + `7 f m.e`, "[7 [Integer]]"},
		{pre + ft + `f m.e`, "[[Integer]]"},
		{pre + ft + `m.e f`, "[[Integer]]"},
		{pre + ft + `7 f m.b`, "[7 [None]]"},
		{pre + fm + `def k fn [[y:Any][Any][[7 f y]]] end k None`, "[[7 [None]]]"},
		{pre + fm + `def k fn [[y:Any][Any][[f y]]] end k None`, "[[[None]]]"},
		{pre + fm + `def k fn [[y:Any][Any][[7 f y]]] end k 's'`, "[[[7] 's']]"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
}
