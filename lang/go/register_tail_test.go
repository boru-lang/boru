package lang

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

// NUR171: a compiled no-match diagnostic inside a lens's own run carries a
// source position — the lens unit's synthesized `dot` takes its segment's
// key token when its receiver has none — where it used to read "source
// position unknown" against the interpreter's underlined receiver.
func TestLensNoMatchKeepsAPositionCompiled(t *testing.T) {
	src := `5 $.name apply`
	gotC, _, errC, _, errI := runBothEngines(t, src)
	if errI == nil || codeOf(errI) != "signature_error" {
		t.Fatalf("%q: interpreted %v, want signature_error", src, errI)
	}
	if errC == nil || codeOf(errC) != "signature_error" || len(gotC) != 0 {
		t.Fatalf("%q: compiled %v / %v, want the interpreter's signature_error", src, gotC, errC)
	}
	if strings.Contains(errC.Error(), "source position unknown") {
		t.Fatalf("%q: the compiled raise carries no position:\n%s", src, errC.Error())
	}
}

// NUR141: the check pass's admission of a CONCRETE candidate to a
// predicate-typed parameter agrees with the runtime's — the predicate runs
// for real under analysis (effect-free body, analysis suspended around the
// run), so `f 5` over `n:Even` is refused on both lanes with one verdict
// and `f 4` passes.
func TestPredicateAdmissionAgreesWithTheRuntime(t *testing.T) {
	const pred = `def Even fnpred n:Integer [eq 0 (mod 2 n)] end def f fn [[n:Even] [Integer] [n]] end `
	requireSameVerdict(t, pred+`f 5`)
	requireEngineParity(t, pred+`f 4`, true)
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	res, err := a.Check(pred + `f 5`)
	if err != nil {
		t.Fatal(err)
	}
	if res.Summary.Errors == 0 {
		t.Fatalf("the check pass admitted 5 to n:Even: %+v", res.Diagnostics)
	}
}

// NUR129 (the dynamic half): a value-producing loop whose body leaves a
// reach group's DYNAMIC survivor — a member fn read the pass cannot type —
// declines soundly, because the interpreter re-steps the survivor at every
// iteration (a named fn matching nothing raises; an anonymous arg-taking one
// stays data), and a typed member read keeps its compile.
func TestLoopReachSurvivorDeclinesSoundly(t *testing.T) {
	src := `def g fn [[x:Integer][Integer][x add 1]] end def m {f: g/v} end for 2 [m.f]`
	gotC, _, errC, _, errI := runBothEngines(t, src)
	if errI == nil || codeOf(errI) != "uncalled_function" {
		t.Fatalf("%q: interpreted %v, want uncalled_function", src, errI)
	}
	requireCompileDefect(t, src, gotC, errC)
	if errC == nil || !strings.Contains(errC.Error(), "NUR129") {
		t.Fatalf("%q: want the reach survivor's decline, got %v", src, errC)
	}
	anon := `def m {f: ([n:Integer] => [n add 1])} end for 2 [m.f]`
	gotC, _, errC, gotI, errI := runBothEngines(t, anon)
	if errI != nil || fmt.Sprint(gotI) != "[fn (Integer) fn (Integer)]" {
		t.Fatalf("%q: interpreted %v / %v", anon, gotI, errI)
	}
	requireCompileDefect(t, anon, gotC, errC)
	requireEngineParity(t, `def m {count: 5} end for 2 [m.count]`, true)
	requireEngineParity(t, `def m {f: ([n:Integer] => [n add 1])} end 5 m.f`, true)
}

// NUR128: an exported module fn's body gets the declaration-shaped analysis
// at EXPORT time, on the importing pass, in the module registry it was
// written in — so its dead branch warns whether or not anyone calls it,
// as a top-level fn's does.
func TestModuleExportedFnBodyAnalysedAtExport(t *testing.T) {
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	for _, src := range []string{
		`import module [def mf fn [[n:Integer][Integer][if true [n] [0]]] export "M" {mf: mf/v}]  1`,
		`import module [def mf fn [[n:Integer][Integer][if true [n] [0]]] export "M" {mf: mf/v}]  M.mf 4`,
		`def f fn [[n:Integer][Integer][if true [n] [0]]]  1`,
	} {
		res, err := a.Check(src)
		if err != nil {
			t.Fatalf("%q: %v", src, err)
		}
		found := false
		for _, d := range res.Diagnostics {
			if d.Code == "unreachable_branch" {
				found = true
			}
		}
		if !found {
			t.Errorf("%q: no unreachable_branch among %+v", src, res.Diagnostics)
		}
	}
}

// NUR097 (Allowed, with this hint as the mitigation): a fn body that reads a
// module-scope name a LATER root def in the same file rebinds gets an
// info-severity `late_binding` hint — module names resolve late, so the fn
// computes with the later binding; a program that never rebinds gets none.
func TestLateBindingHint(t *testing.T) {
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	hinted := func(src string) bool {
		res, err := a.Check(src)
		if err != nil {
			t.Fatalf("%q: %v", src, err)
		}
		for _, d := range res.Diagnostics {
			if d.Code == "late_binding" {
				return true
			}
		}
		return false
	}
	if !hinted(`def n 1 end def c fn [[][Integer][n]] end def n 2 end c`) {
		t.Error("a later re-def of a name the fn reads must hint")
	}
	if hinted(`def n 1 end def c fn [[][Integer][n]] end c`) {
		t.Error("no re-def, no hint")
	}
	requireEngineParity(t, `def n 1 end def c fn [[][Integer][n]] end def n 2 end c`, true)
}

// A gradual captured read with a value pending beneath it (`5 j typeof`,
// NUR123's last open shape): no island can take the statement over (the
// literal is deferred), so the read keeps its slot push and a GUARD tests
// the value at the push — when it holds a fn the interpreter dispatches,
// the compiled run raises a designed defer (loud, in the bail ledger) where
// it used to compute `typeof` over the fn silently; when it does not, the
// slot push is the right answer and the lanes agree (here: the same
// return-count error over the same two values).
func TestGradualReadWithPendingValueGuards(t *testing.T) {
	const fnHeld = `def h fn [[m:Map][Function][def j (m get "f") ( fn [[x:Integer][Any][5 j typeof]] )]] def q (h {f: ([] => [42])}) (q 7)`
	const held = `def h fn [[m:Map][Function][def j (m get "f") ( fn [[x:Integer][Any][5 j typeof]] )]] def q (h {f: 3}) (q 7)`
	a := mustNew(t)
	gotC, _, errC := a.RunCompiled(fnHeld)
	if !noteCompileDefect(t, fnHeld, gotC, errC) || !strings.Contains(fmt.Sprint(errC), "NUR123") {
		t.Errorf("a fn-holding gradual read under a pending value must bail on its guard: got=%v err=%v", gotC, errC)
	}
	b := mustNew(t)
	if _, err := b.RunInterp(fnHeld); codeOf(err) != "type_error" || !strings.Contains(err.Error(), "[5 Integer]") {
		t.Errorf("the interpreter dispatches the read (42, then its type): err=%v", err)
	}
	requireSameVerdict(t, held)
	dis := compileDisasm(t, held)
	if !strings.Contains(dis, "bail if the read holds a fn (guard)") {
		t.Errorf("the read's push must carry the guard:\n%s", dis)
	}
}

// NUR104: a record type constrains a slot the same way however it is
// spelt — the named spelling dispatches its field map, the inline `o:{…}`
// rides inert inside the fn-spec list and is resolved at sig install
// (ResolveSigRecordFields) — so a bare type word, an expression field, a
// Boolean field and an optional field answer alike on both lanes.
func TestRecordTypeSpellingsAgree(t *testing.T) {
	for _, src := range []string{
		`def R (refine Record [{a:(Integer tor String)}]) def f fn [[o:R][Any][o.a]]  f {a:7}`,
		`def f fn [[o:{a:(Integer tor String)}][Any][o.a]]  f {a:7}`,
		`def f fn [[o:{pretty:Boolean}][Any][o.pretty]]  f {pretty:true}`,
		`def f fn [[o:{y?:String}][Any][o.y]]  f {}`,
	} {
		requireEngineParity(t, src, true)
	}
	if got, err := mustNew(t).RunInterp(`def f fn [[o:{a:(Integer tor String)}][Any][o.a]]  f {a:7}`); err != nil || fmt.Sprint(got) != "[7]" {
		t.Errorf("the inline expression field dispatches: got %v err=%v", got, err)
	}
}

// NUR102: an effectful predicate body runs ONCE per dispatch on both lanes
// — the interpreter memoises the verdict across one pending word's
// planning phases (RunPredicate), the VM skips the entry guard after its
// runtime poly re-match. And a CARRIER candidate keeps a predicate-typed
// arm reachable (the call goes poly) instead of committing the base arm:
// `we (f 2)` is even-arm on both lanes where the compiled lane answered
// int-arm without running the predicate.
func TestPredicateRunsOncePerDispatch(t *testing.T) {
	const preds = "def Even fnpred n:Integer [ print \"P\" eq 0 (mod 2 n) ] end\ndef we fn [[a:Even][String][\"even-arm\"]] end\ndef we fn [[a:Integer][String][\"int-arm\"]] end\n"
	for _, c := range []struct{ src, out string }{
		{preds + "print (we 4)", "P\neven-arm\n"},
		{preds + "print (we 3)", "P\nint-arm\n"},
		{preds + "def f fn [[x:Integer][Integer][x add 2]] end\nprint (we (f 2))", "P\neven-arm\n"},
	} {
		a := mustNew(t)
		var ao bytes.Buffer
		a.SetOutput(&ao)
		gotC, compiled, errC := a.RunCompiled(c.src)
		if noteCompileDefect(t, c.src, gotC, errC) {
			continue
		}
		if !compiled || errC != nil {
			t.Fatalf("compiled: compiled=%v err=%v", compiled, errC)
		}
		b := mustNew(t)
		var bo bytes.Buffer
		b.SetOutput(&bo)
		if _, err := b.RunInterp(c.src); err != nil {
			t.Fatalf("interp: %v", err)
		}
		if ao.String() != c.out || bo.String() != c.out {
			t.Errorf("%q: compiled out=%q interp out=%q, want %q on both lanes", c.src, ao.String(), bo.String(), c.out)
		}
	}
}

// NUR134: a module export's definite no-match inside a `do` body is caught
// like any body error. The compiled do-body unit raises it in place (a
// unit-scoped trap) instead of lowering the failed call's wreckage, and the
// do's model is the caught Error (the raise watch) — the wreckage no longer
// escapes the bracket to be re-dispatched, and reported as an uncaught
// program error, on the enclosing tape. Found with it and fixed the same
// way: a def AFTER an unconditional raise in a do body leaked into the
// model (`do [raise … def x 1] … x` compiled to 1 where the interpreter
// raises undefined_word, or keeps the earlier binding).
func TestModuleNoMatchInDoBodyIsCaught(t *testing.T) {
	const mod = `import module [ def dec fn [[bad:Boolean x:Any] [Any] [ if bad [raise bad_input "boom"] [x] ]] export "M" {dec: dec/v} ] end `
	for _, src := range []string{
		mod + `do [(true 5 M.dec) "no-raise"] error [dot code]`,
		mod + `def msg (do [(true 5 M.dec) "no-raise"] error [dot code])  msg`,
		mod + `do [(true 5 M.dec) "no-raise"] typeof`,
		mod + `do [(false true M.dec) "ok"] error [dot code]`,
		`def x 0 do [raise bad_input "boom" def x 1] error [dot code] x`,
		`def x 0 do [def x 2 raise bad_input "boom" def x 1] error [dot code] x`,
		`do [def y 3 raise bad_input "boom"] error [dot code] y`,
	} {
		requireEngineParity(t, src, true)
	}
	if got, err := mustNew(t).RunInterp(mod + `do [(true 5 M.dec) "no-raise"] error [dot code]`); err != nil || fmt.Sprint(got) != "[uncalled_function]" {
		t.Errorf("the interpreter catches the no-match: got %v err=%v", got, err)
	}
	// The effects before the raise run once on both lanes; the one after it
	// never does.
	src := mod + `do [print "a" (true 5 M.dec) print "b"] error [dot code]`
	a := mustNew(t)
	var ao bytes.Buffer
	a.SetOutput(&ao)
	if _, compiled, err := a.RunCompiled(src); !compiled || err != nil || ao.String() != "a\n" {
		t.Errorf("compiled: compiled=%v err=%v out=%q, want a\\n", compiled, err, ao.String())
	}
	// A read of a def the raise skipped is the interpreter's undefined_word,
	// never the leaked binding.
	for _, src := range []string{
		`do [raise bad_input "boom" def x 1] error [dot code] x`,
		mod + `do [(true 5 M.dec) def x 1] error [dot code] x`,
	} {
		gotC, _, errC, _, errI := runBothEngines(t, src)
		if codeOf(errI) != "undefined_word" || len(gotC) != 0 || errC == nil {
			t.Errorf("%q: the skipped def must not leak: compiled=%v/%v interp err=%v", src, gotC, errC, errI)
		}
	}
}
