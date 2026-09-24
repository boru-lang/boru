package lang

import (
	"bytes"
	"fmt"
	"sync"
	"testing"
)

// A COMPILABLE check-prop gen/property body compiles as a same-program
// stored-param-body unit (Signature.StoredBodies → compileStoredParamBody)
// and runs nested on the VM via InvokeCallback. The sharp pin: ITERATION
// COUNT ADDS NO INTERPRETER ENTRIES — every unattributed entry left is
// module-load boru (invariant in the run count).
//
// The gen body here is `[ 3 4 add ]` (a compilable constant expression), NOT
// a direct rand call `[r.int 1 9]`: a member-fn read from the opaque Map
// param `r` applied to forward args (`r.int LO HI`) is the fn-value-call
// boundary — it cannot compile correctly as a unit and MUST DECLINE to the
// interpreter (TestCheckPropMemberFnGenVariesNotConstant pins that; before
// the decline fix the unit silently returned the trailing arg — a CONSTANT
// generator, the r.int→9 miscompile the trie/sort PBT suites tripped over).
// So the invariance holds only for bodies that genuinely compile; the
// member-fn gen is ledgered red (p6/check-prop-body-on-vm). The fn-scope
// ${frame-local} guard (TestCheckPropInterpStringFnScopeFailsToCompile) is the
// standing negative: stored-param-body compiles are module-scope only.
func TestCheckPropIterationsAddNoInterpEntries(t *testing.T) {
	entryCensus := func(runs int) map[string]int {
		t.Helper()
		a, err := New()
		if err != nil {
			t.Fatal(err)
		}
		a.SetOutput(&bytes.Buffer{})
		var mu sync.Mutex
		counts := map[string]int{}
		disarm := a.ArmInterpEntryHook(func(e InterpEntry) {
			if e.Attribution == "" {
				mu.Lock()
				counts[e.Seam]++
				mu.Unlock()
			}
		})
		defer disarm()
		src := fmt.Sprintf("import \"boru:test\" end\ndef res (Test.check-prop \"x\" [ 3 4 add ] [ drop true ] %d 1 0)\nres get \"ok\"", runs)
		if _, err := a.RunCompiledStrict(src); err != nil {
			t.Fatalf("run (%d iterations): %v", runs, err)
		}
		mu.Lock()
		defer mu.Unlock()
		out := make(map[string]int, len(counts))
		for k, v := range counts {
			out[k] = v
		}
		return out
	}
	few, many := entryCensus(2), entryCensus(60)
	if fmt.Sprintf("%v", few) != fmt.Sprintf("%v", many) {
		t.Errorf("interpreter entries scale with the iteration count — the bodies are NOT running as units:\n  2 runs:  %v\n  60 runs: %v", few, many)
	}
	// The per-iteration seams specifically must contribute nothing: before
	// the stored-param-body units the census grew by one CallBoru per body
	// per iteration.
	if few["CallBoru"] != 0 || many["CallBoru"] != 0 {
		t.Errorf("per-iteration CallBoru entries present (2 runs: %d, 60 runs: %d) — the throwaway-sig path is back", few["CallBoru"], many["CallBoru"])
	}
}

// TestCheckPropShrinkAddsNoInterpEntries: a FAILING property's shrink dispatches
// the property through the same carrier the driver's loop does (checkPropBody —
// InvokeCallback over the stored-param-body unit), not a throwaway CallBoru
// frame per candidate, and the gen-program recorder's run reports itself as
// the observation it is ("stackform-record"). So a compiled check-prop whose
// bodies compile adds NO unattributed interpreter entry, shrinking included
// (the interp-entry census's corpus-modules.tsv L164), and the result map —
// the shrunk input, its source and cost — is the interpreter's.
func TestCheckPropShrinkAddsNoInterpEntries(t *testing.T) {
	src := `import "boru:test" end
def res (Test.check-prop "f" [ 3 4 add ] [ drop false ] 3 1 4)
res`
	for _, c := range []struct{ src, want string }{
		{src, "[{name:'f' ok:false runs:1 failing-input:7 shrunk-input:0 shrunk-source:'0' shrunk-cost:1 error:none}]"},
		// The corpus row (corpus-modules.tsv L164): a property that leaves its
		// String input beneath two atoms declines the recorder's Any-typed
		// carrier and is stamped at run time over the String it meets
		// (native.StampBodySig), in the loop and in the shrink alike.
		{`import "boru:test" end Test.check-prop 'a' ['a','b'] ['c','d'] 2 3 4`,
			`[{name:'a' ok:false runs:1 failing-input:'b' shrunk-input:'b' shrunk-source:'"b"' shrunk-cost:2 error:error(property returned non-Boolean (ProperString))}]`},
		// Bodies read RAW from a PropertySpec map (`Test.prop`, `run-property`):
		// no carrier at all, both stamped at run time.
		{`import "boru:test"  def p (Test.prop "x" [3 4 add] [5 gte]) end (p Test.run-property) get "ok"`, "[true]"},
	} {
		seen, gotC := unattributedEntries(t, c.src)
		if len(seen) != 0 {
			t.Errorf("%q: unattributed interpreter entries %v — a body is back on the interpreter", c.src, seen)
		}
		if fmt.Sprint(gotC) != c.want {
			t.Errorf("%q: compiled %v, want %s", c.src, gotC, c.want)
		}
		a := mustNew(t)
		a.SetOutput(&bytes.Buffer{})
		gotI, err := a.RunInterp(c.src)
		if err != nil || fmt.Sprint(gotI) != c.want {
			t.Errorf("%q: interp %v / %v, want %s", c.src, gotI, err, c.want)
		}
	}
	// The recorder's run is attributed, and it happens: the shrink of a
	// failing gen program records the program once.
	b := mustNew(t)
	b.SetOutput(&bytes.Buffer{})
	recorded := 0
	disarm := b.ArmInterpEntryHook(func(ev InterpEntry) {
		if ev.Attribution == "stackform-record" {
			recorded++
		}
	})
	if _, _, err := b.RunCompiled(src); err != nil {
		t.Fatal(err)
	}
	disarm()
	if recorded == 0 {
		t.Error("the gen-program recorder's run reported no stackform-record entry")
	}
}

// TestCheckPropMemberFnGenVariesNotConstant — the regression pin for the
// generator miscompile (bisected to 853dcaa4, fixed 2026-07-18). A direct
// rand-call gen body `[r.int 1 9]` reads a member fn from the opaque Map
// param `r` and applies it to the forward args. The stored-param unit
// silently RET'd the trailing arg (`9`) instead of applying `r.int` — a
// CONSTANT generator that turned every PBT suite using the member-call gen
// form vacuous (weak `is Integer` properties passed on the constant) or
// broke it (trie/sort properties sensitive to the value). The decline fix
// makes the gen body interpret, producing genuine varying values.
func TestCheckPropMemberFnGenVariesNotConstant(t *testing.T) {
	// `n eq 9` is true iff the generator returns the trailing arg 9 every
	// run. A correct varying generator refutes it (ok=false); the miscompile
	// left it un-refuted (ok=true — always 9). Pinned under BOTH pipelines.
	src := `import "boru:test" end
def res (Test.check-prop "varies" [r.int 1 9] [ var [[n] (n eq 9) ] ] 12 1 0)
res get "ok"`
	for _, mode := range []string{"compiled", "interp"} {
		a, err := New()
		if err != nil {
			t.Fatal(err)
		}
		a.SetOutput(&bytes.Buffer{})
		var got []any
		if mode == "compiled" {
			got, _, err = a.RunCompiled(src)
		} else {
			got, err = a.RunInterp(src)
		}
		if err != nil {
			t.Fatalf("%s: %v", mode, err)
		}
		if fmt.Sprint(got) == "[true]" {
			t.Errorf("%s: the generator is CONSTANT (always the trailing arg 9) — the r.int→9 miscompile is back", mode)
		}
	}
}

// TestCheckPropCompiledParity — the compiled and interpreted pipelines agree
// on check-prop results across the pass / fail / generator-error shapes (the
// units must preserve CallBoru frame semantics exactly).
func TestCheckPropCompiledParity(t *testing.T) {
	cases := []string{
		`import "boru:test" end
def res (Test.check-prop "pass" [r.int 1 9] [ 0 gte ] 6 1 0)
(res get "ok") (res get "runs")`,
		`import "boru:test" end
def res2 (Test.check-prop "fail" [r.int 1 9] [ drop false ] 6 1 0)
(res2 get "ok") (res2 get "runs")`,
		`import "boru:test" end
def res (Test.check-prop "gen-raises" [raise bad_input "boom"] [ 0 gte ] 3 1 0)
(res get "ok")`,
	}
	for _, src := range cases {
		a, err := New()
		if err != nil {
			t.Fatal(err)
		}
		a.SetOutput(&bytes.Buffer{})
		gotC, compiled, err := a.RunCompiled(src)
		if noteCompileDefect(t, src, gotC, err) {
			continue
		}
		if err != nil {
			t.Fatalf("RunCompiled: %v\nsrc: %s", err, src)
		}
		if !compiled {
			t.Fatalf("check-prop program must run compiled\nsrc: %s", src)
		}
		b, err := New()
		if err != nil {
			t.Fatal(err)
		}
		b.SetOutput(&bytes.Buffer{})
		gotI, err := b.RunInterp(src)
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		if fmt.Sprintf("%v", gotC) != fmt.Sprintf("%v", gotI) {
			t.Errorf("parity: compiled %v != interp %v\nsrc: %s", gotC, gotI, src)
		}
	}
}

// TestCheckPropFailedToCompileBodyFallsBackSound — a gen body the stored-param
// compile declines (a capitalised type install doesn't lower in a closure
// unit) falls through to the standing NoEvalArgs replay-hazard gates, which
// decline the program: it does not compile, and that is a defect —
// not wrong, exactly the do-registry-replay discipline.
func TestCheckPropFailedToCompileBodyFallsBackSound(t *testing.T) {
	const src = `import "boru:test" end
def res (Test.check-prop "hazard" [def Big Integer 9] [ 0 gte ] 2 1 0)
res get "ok"`
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	a.SetOutput(&bytes.Buffer{})
	gotC, compiled, err := a.RunCompiled(src)
	if noteCompileDefect(t, src, gotC, err) {
		return
	}
	if err != nil {
		t.Fatalf("RunCompiled: %v", err)
	}
	if compiled {
		t.Fatal("the replay-hazard gen body must decline the compile (the interpreter owns it)")
	}
	b, err := New()
	if err != nil {
		t.Fatal(err)
	}
	b.SetOutput(&bytes.Buffer{})
	gotI, err := b.RunInterp(src)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if fmt.Sprintf("%v", gotC) != fmt.Sprintf("%v", gotI) {
		t.Errorf("parity: compiled %v != interp %v", gotC, gotI)
	}
}
