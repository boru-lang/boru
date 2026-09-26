package lang

import (
	"fmt"
	"strings"
	"testing"
)

// agreeOnBothLanes runs src on both lanes: it must compile, and the two must
// agree — on the value, or on the error (want "ERROR:<substring>").
func agreeOnBothLanes(t *testing.T, src, want string) {
	t.Helper()
	gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
	if !compiled {
		t.Errorf("%s: must compile, got %v", src, errC)
		return
	}
	if fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(errC) != fmt.Sprint(errI) {
		t.Errorf("%s: compiled %v / %v, interpreter %v / %v", src, gotC, errC, gotI, errI)
		return
	}
	if sub, isErr := strings.CutPrefix(want, "ERROR:"); isErr {
		if errI == nil || !strings.Contains(errI.Error(), sub) {
			t.Errorf("%s: got %v / %v, want an error containing %q", src, gotI, errI, sub)
		}
	} else if errI != nil || fmt.Sprint(gotI) != want {
		t.Errorf("%s: got %v / %v, want %s", src, gotI, errI, want)
	}
}

// TestNUR009BytesRefinements pins NUR009's close: Bytes declares itself a
// refinement base (core.DeclareRefinementBase, where basic registers it), so
// the comparison words refine it like every other ordered scalar leaf —
// value use, named type, typed def, fn parameter and return — on both lanes,
// with the refinement rendered in the comparison vocabulary. A Bytes bound
// is always computed (there is no Bytes literal); `convert Bytes <String>`
// folds at compile time over a const string, so a named Bytes refinement
// compiles.
func TestNUR009BytesRefinements(t *testing.T) {
	const hi = `def Hi (Bytes gte (convert Bytes "m")) `
	for _, tc := range []struct{ src, want string }{
		{`(convert Bytes "z") is (Bytes gte (convert Bytes "m"))`, "[true]"},
		{`(convert Bytes "a") is (Bytes gte (convert Bytes "m"))`, "[false]"},
		{`(convert Bytes "m") is (Bytes gt (convert Bytes "m"))`, "[false]"},
		{`(convert Bytes "c") is (between (convert Bytes "a") (convert Bytes "d") Bytes)`, "[true]"},
		{`(convert Bytes "e") is (between (convert Bytes "a") (convert Bytes "d") Bytes)`, "[false]"},
		{hi + `(convert Bytes "z") is Hi`, "[true]"},
		{hi + `(convert Bytes "a") is Hi`, "[false]"},
		{hi + `def v:Hi (convert Bytes "z") v`, "[Bytes<7a>]"},
		{hi + `def v:Hi (convert Bytes "a") v`, "ERROR:does not unify with declared type Hi"},
		{hi + `def f fn [[b:Hi] [Any] [b]] f (convert Bytes "q")`, "[Bytes<71>]"},
		{hi + `def f fn [[b:Hi] [Any] [b]] f (convert Bytes "b")`, "ERROR:no signature matches"},
		// An INLINE Bytes refinement slots at Bytes, not a wildcard (the
		// resolver hand-listed five bases and sent Bytes to the TAny tail).
		{`def g fn [[b:(Bytes gt (convert Bytes "m"))] [Any] [b]] g (convert Bytes "q")`, "[Bytes<71>]"},
		{`def g fn [[b:(Bytes gt (convert Bytes "m"))] [Any] [b]] g (convert Bytes "a")`, "ERROR:no signature matches"},
		{`def g fn [[b:Bytes] [(Bytes gt (convert Bytes "m"))] [b]] g (convert Bytes "a")`, "ERROR:expected (Bytes gt Bytes<6d>)"},
		{`def R (between (convert Bytes "b") (convert Bytes "d") Bytes) [(convert Bytes "a") (convert Bytes "c") (convert Bytes "e")] each [is R]`, "[[false true false]]"},
		// The refinement renders as one — not `Bytes<?>`, the base's value
		// form over a payload it cannot read.
		{`(Bytes lt (convert Bytes "m"))`, "[(Bytes lt Bytes<6d>)]"},
		{`(between (convert Bytes "a") (convert Bytes "d") Bytes)`, "[(Bytes gte Bytes<61> lte Bytes<64>)]"},
		// Two Bytes refinements are two consts: keyed on that rendering, the
		// compile pass's pool merged them and the second `is` read the first.
		{`(convert Bytes "c") is (Bytes lt (convert Bytes "b")) (convert Bytes "c") is (Bytes gt (convert Bytes "a"))`, "[false true]"},
	} {
		agreeOnBothLanes(t, tc.src, tc.want)
	}
}

// TestNUR009BytesRefinementRefusesOtherBounds is the paired negative: the
// Bytes base takes a Bytes bound only, and no other base takes one — each
// refused with the constructor's own message on both lanes (the compiled
// lane's check pass stops at it).
func TestNUR009BytesRefinementRefusesOtherBounds(t *testing.T) {
	for src, want := range map[string]string{
		`(Bytes gt 3)`:                          "bound Integer does not match dependent base Bytes",
		`(Integer gt (convert Bytes "a"))`:      "bound Bytes does not match dependent base Integer",
		`between (convert Bytes "a") 5 Bytes`:   "high bound Integer does not match base Bytes",
		`(convert Bytes "a") is (Bytes gt "a")`: "bound ProperString does not match dependent base Bytes",
	} {
		_, _, errC, gotI, errI := runBothEngines(t, src)
		if errI == nil || !strings.Contains(errI.Error(), want) {
			t.Errorf("%s: interpreter %v / %v, want %q", src, gotI, errI, want)
		}
		if errC == nil || !strings.Contains(errC.Error(), want) {
			t.Errorf("%s: compiled lane %v, want %q", src, errC, want)
		}
	}
}

// TestNUR231ComputedBoundValuesCompile pins NUR231's value half: a
// refinement over a computed bound (`Integer gt (size s)`, a factory's
// param) is built by the RUN — the compile pass records the constructor as
// the call it is — so it agrees with the interpreter. The pass had baked
// its own refinement, the bound a carrier that orders below every value:
// `3 is (Integer gt (size "abc"))` was true compiled, and `between` over a
// computed bound was Never whatever the bound.
func TestNUR231ComputedBoundValuesCompile(t *testing.T) {
	const mk = `def mk fn [[n:Integer] [Any] [Integer gt n]] `
	for _, tc := range []struct{ src, want string }{
		{`3 is (Integer gt (size "abc"))`, "[false]"},
		{`4 is (Integer gt (size "abc"))`, "[true]"},
		{`(Integer gt (size "abc"))`, "[(Integer gt 3)]"},
		{`7 is (between 1 (size "abcdefghij") Integer)`, "[true]"},
		{`7 is (between 1 (size "abc") Integer)`, "[false]"},
		{mk + `3 is (mk 5)`, "[false]"},
		{mk + `6 is (mk 5)`, "[true]"},
		{mk + `(mk 5)`, "[(Integer gt 5)]"},
		{`7 is ((Integer lt (size "abcdefghij")) tand (Integer gt 5))`, "[true]"},
		{`def s "q" (convert Bytes s) is (Bytes gt (convert Bytes "m"))`, "[true]"},
		// Known bounds keep their const path.
		{`5 is (Integer gte 0)`, "[true]"},
		{`7 is (between 10 1 Integer)`, "[false]"},
	} {
		agreeOnBothLanes(t, tc.src, tc.want)
	}
}

// TestNUR231ComputedBoundTypesCompile pins NUR231's type half: a TYPE over
// a computed bound — a named type, a typed def, a fn parameter or return
// typed by such a name, a class field, a generic bound — compiles to the
// run-time install. The run installs the type from the body it computed
// (OpBindTypeRun), the node the check pass minted forwards to the run's, and
// a typed def records the run's own membership check (OpBindTyped over
// TypedBindRunMembership, against the named node or the constraint the run
// computed). Unions, negations and intersections holding such a refinement,
// and an empty interval the run computes (Never), agree on both lanes; an
// overload set over such a type re-matches at run time (the pass's match
// over an unknown bound admits every value). The check pass raises no
// diagnostic of its own: it admits, gradually — `Integer lte (size s)` had
// refused 3 whatever s held.
func TestNUR231ComputedBoundTypesCompile(t *testing.T) {
	const tt = `def T (Integer gt (size "abc")) `
	const bb = `def B (Bytes gt (convert Bytes (convert String (size "ab")))) `
	for _, tc := range []struct{ src, want string }{
		{`def T (Integer gte (size "abcd")) def v:T 3 v`, "ERROR:does not unify with declared type T"},
		{`def T (Integer lte (size "abcd")) def v:T 3 v`, "[3]"},
		{`def x:(Integer gt (size "abc")) 2 x`, "ERROR:does not unify with declared type (Integer gt 3)"},
		{`def x:(Integer gt (size "abc")) 5 x`, "[5]"},
		{`def x:(between 1 (size "abc") Integer) 7 x`, "ERROR:does not unify with declared type (Integer gte 1 lte 3)"},
		{`def s "abc" def x:(Integer gt (size s)) 5 x add 1`, "[6]"},
		{tt + `2 is T`, "[false]"},
		{tt + `5 is T`, "[true]"},
		{tt + `T`, "[T]"},
		{tt + `def U T 1 is U`, "[false]"},
		{tt + `def T (Integer gt (size "abcdef")) 5 is T`, "[false]"},
		{tt + `T tcmp (Integer gt 3)`, "[1]"},
		{`def T ((Integer gt (size "abc")) tor String) 2 is T`, "[false]"},
		{`def T ((Integer gt (size "abc")) tor String) "s" is T`, "[true]"},
		{`def x:((Integer gt (size "abc")) tor String) 1 x`, "ERROR:does not unify with declared type (Integer gt 3) tor String"},
		{`def T (tnot (Integer gt (size "abc"))) 1 is T`, "[true]"},
		{`def T (tnot (Integer gt (size "abc"))) 9 is T`, "[false]"},
		{`def x:(tnot (Integer gt (size "abc"))) 9 x`, "ERROR:does not unify"},
		{`def T ((Integer lt (size "abcdefghij")) tand (Integer gt 5)) 7 is T`, "[true]"},
		{`def T ((Integer lt (size "abcdefghij")) tand (Integer gt 5)) T`, "[T]"},
		{`def T (between 5 (size "ab") Integer) T`, "[Never]"},
		{`def T (between 5 (size "ab") Integer) T eq Never`, "[true]"},
		{tt + `def f fn [[n:T] [Integer] [n add 1]] f 5`, "[6]"},
		{tt + `def f fn [[n:T] [Integer] [n add 1]] f 2`, "ERROR:no signature matches"},
		{tt + `def f fn [[n:T] [Integer] [n add 1]] def f fn [[n:Integer] [Integer] [n sub 1]] f 2`, "[1]"},
		{tt + `def f fn [[n:T] [Integer] [n add 1]] def f fn [[n:Integer] [Integer] [n sub 1]] f 5`, "[6]"},
		{tt + `def g fn [[n:Integer] [T] [n]] g 5`, "[5]"},
		{tt + `def g fn [[n:Integer] [T] [n]] g 2`, "ERROR:expected T, got Integer"},
		{tt + `def S class {x:T} (make S {x:50}) dot x`, "[50]"},
		{tt + `def S class {x:T} make S {x:2}`, "ERROR:does not satisfy DepScalar bounds"},
		{tt + `def f gen [(X extends T)] fn [[x:X] [X] [x]] f 5`, "[5]"},
		{tt + `def f gen [(X extends T)] fn [[x:X] [X] [x]] f 2`, "ERROR:no signature matches"},
		{bb + `(convert Bytes "3") is B`, "[true]"},
		{bb + `def v:B (convert Bytes "1") v`, "ERROR:does not unify with declared type B"},
		// Inside a callback or loop body, the typed def's own check is the
		// run's too; the named type installed at the root is read there.
		{`each ([e:Integer] => [def x:(Integer gt (size "abc")) e x]) [4 5]`, "[[4 5]]"},
		{`each ([e:Integer] => [def x:(Integer gt (size "abc")) e x]) [1 5]`, "ERROR:does not unify with declared type (Integer gt 3)"},
		{tt + `each ([e:Integer] => [def x:T e x]) [1 5]`, "ERROR:does not unify with declared type T"},
		{`for 2 [def x:(Integer gt (size "abc")) 2]`, "ERROR:does not unify with declared type (Integer gt 3)"},
	} {
		agreeOnBothLanes(t, tc.src, tc.want)
		a, err := New()
		if err != nil {
			t.Fatal(err)
		}
		if res, err := a.Check(tc.src); err != nil || res.Summary.Errors != 0 {
			t.Errorf("%s: the check pass decides nothing over an unknown bound: %v %+v", tc.src, err, res.Diagnostics)
		}
	}
	// The known-bound twins compile and agree.
	agreeOnBothLanes(t, `def T (Integer gte 4) def v:T 3 v`, "ERROR:does not unify with declared type T")
	agreeOnBothLanes(t, `def T (Integer lte 4) def v:T 3 v`, "[3]")
	agreeOnBothLanes(t, `def x:(Integer gt 3) 2 x`, "ERROR:does not unify")
	agreeOnBothLanes(t, `def g fn [[n:(Integer gt 3)] [Any] [n]] g 2`, "ERROR:no signature matches")
}

// TestNUR231OutsideTheBaseRefusesAtCheck: a value the refinement's BASE
// refuses is refused whatever the run computes for the bound, so that verdict
// is the pass's to give — the check pass flags it (a runtime mirror) and the
// interpreter raises the same text.
func TestNUR231OutsideTheBaseRefusesAtCheck(t *testing.T) {
	const src = `def x:(Integer gt (size "abc")) "s" x`
	const want = `def x: value 's' does not unify with declared type (Integer gt 3)`
	_, _, _, gotI, errI := runBothEngines(t, src)
	if errI == nil || !strings.Contains(errI.Error(), want) {
		t.Errorf("interpreter %v / %v, want %q", gotI, errI, want)
	}
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	res, err := a.Check(src)
	if err != nil || res.Summary.Errors != 1 || !strings.Contains(res.Diagnostics[0].Detail, "does not unify with declared type") {
		t.Errorf("the pass flags a value outside the base: %v %+v", err, res.Diagnostics)
	}
}

// TestNUR231TypeRunDisassembles pins the run-time install's shape in the
// bytecode: the body the run computes, the def's twin (written back, so it
// replays nothing) and BIND_TYPE_RUN at the def's position.
func TestNUR231TypeRunDisassembles(t *testing.T) {
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	prog, reason, _, err := a.CompileCheck(`def T (Integer gt (size "abc")) 5 is T`)
	if prog == nil || err != nil {
		t.Fatalf("compiles: %q %v", reason, err)
	}
	dis := prog.Disassemble()
	for _, want := range []string{"CALL_NATIVE s1   ; gt", "BIND_TWIN   w0   ; bind twin type-install T", "BIND_TYPE_RUN v0   ; run-time type install T"} {
		if !strings.Contains(dis, want) {
			t.Errorf("disassembly lacks %q:\n%s", want, dis)
		}
	}
}

// TestNUR231RunBuiltSignaturesDecline pins what NUR231's type half leaves:
// a signature the RUN builds — an inline parameter or return type, or a
// typed container's child, over a computed bound — and a type def the run
// re-installs per call (a fn body's) decline as the compile-time word they
// are, through the generic site, and the interpreter's answer stands. A
// NAMED type over the same bound compiles (TestNUR231ComputedBoundTypesCompile).
func TestNUR231RunBuiltSignaturesDecline(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`def g fn [[n:(Integer gt (size "abc"))] [Any] [n]] g 2`, "ERROR:no signature matches"},
		{`def g fn [[n:(Integer gt (size "abc"))] [Any] [n]] g 5`, "[5]"},
		{`def g fn [[n:Integer] [(Integer gt (size "abc"))] [n]] g 2`, "ERROR:expected (Integer gt 3)"},
		{`def g fn [[n:((Integer gt (size "abc")) tor String)] [Any] [n]] g 2`, "ERROR:no signature matches"},
		{`def g fn [[xs:[:(Integer gt (size "abc"))]] [Any] [xs]] g [5]`, "[[5]]"},
		{`def xs:[:(Integer gt (size "abc"))] [5] xs`, "[[5]]"},
		{`def h fn [[s:String] [Boolean] [def T (Integer gt (size s)) 5 is T]] h "abc"`, "[true]"},
	} {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, tc.src)
		if sub, isErr := strings.CutPrefix(tc.want, "ERROR:"); isErr {
			if errI == nil || !strings.Contains(errI.Error(), sub) {
				t.Errorf("%s: interpreter %v / %v, want an error containing %q", tc.src, gotI, errI, sub)
			}
		} else if errI != nil || fmt.Sprint(gotI) != tc.want {
			t.Errorf("%s: interpreter %v / %v, want %s", tc.src, gotI, errI, tc.want)
		}
		if compiled || errC == nil || !strings.Contains(errC.Error(), "compile-time word def") {
			t.Errorf("%s: compiled %v / %v (compiled=%v), want the compile-time word's decline", tc.src, gotC, errC, compiled)
		}
	}
}

// TestNUR232InlineRefinementReturnDefers pins NUR232: the check pass refused
// an abstract Integer returned against an inline refinement return type —
// `def g fn [[n:Integer] [(Integer gt 3)] [n]] g 5` was a check-time
// type_error that both lanes then returned 5 for — where the named spelling
// (`[Big]`) defers the value-level membership to the RET check. The inline
// form now defers the same case; a residual provably outside the base, or a
// compile-time-known scalar that fails, is still flagged, exactly as named.
func TestNUR232InlineRefinementReturnDefers(t *testing.T) {
	for src, want := range map[string]string{
		`def g fn [[n:Integer] [(Integer gt 3)] [n]] g 5`:                 "",
		`def g fn [[n:Integer] [((Integer gt 3) tor String)] [n]] g 5`:    "",
		`def g fn [[] [(Integer gt 3)] [5]] g`:                            "",
		`def Big (Integer gt 3) def g fn [[n:Integer] [Big] [n]] g 5`:     "",
		`def g fn [[n:String] [(Integer gt 3)] [n]] g "x"`:                "expected (Integer gt 3), got ProperString",
		`def g fn [[] [(Integer gt 3)] [2]] g`:                            "expected (Integer gt 3), got Integer",
		`def g fn [[n:Boolean] [((Integer gt 3) tor String)] [n]] g true`: "expected (Integer gt 3) tor String, got Boolean",
		`def Big (Integer gt 3) def g fn [[n:String] [Big] [n]] g "x"`:    "expected Big, got ProperString",
	} {
		a, err := New()
		if err != nil {
			t.Fatal(err)
		}
		res, err := a.Check(src)
		if err != nil {
			t.Fatalf("%s: %v", src, err)
		}
		var got []string
		for _, d := range res.Diagnostics {
			got = append(got, d.Detail)
		}
		if want == "" {
			if res.Summary.Errors != 0 {
				t.Errorf("%s: the run decides this return, the pass must not: %q", src, got)
			}
			continue
		}
		if res.Summary.Errors != 1 || !strings.Contains(strings.Join(got, "\n"), want) {
			t.Errorf("%s: diagnostics %q, want one containing %q", src, got, want)
		}
	}
	// Both lanes agree on the deferred shape, either way the value falls.
	agreeOnBothLanes(t, `def g fn [[n:Integer] [(Integer gt 3)] [n]] g 5`, "[5]")
	agreeOnBothLanes(t, `def g fn [[n:Integer] [(Integer gt 3)] [n]] g 2`, "ERROR:expected (Integer gt 3), got Integer")
}
