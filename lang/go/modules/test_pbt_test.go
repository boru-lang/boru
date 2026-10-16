package modules

import (
	"strings"
	"testing"

	"github.com/boru-lang/boru/lang/go/native"
	parser "github.com/boru-lang/boru/parser/go"
)

// Helper: run boru source and extract a top-of-stack Map field by key.
func runTestAndGetField(t *testing.T, r *native.Registry, src, key string) native.Value {
	t.Helper()
	res := runTestBoru(t, r, src)
	if len(res) == 0 {
		t.Fatalf("no result for %q", src)
	}
	m, _ := native.AsMap(res[len(res)-1])
	if m == nil {
		t.Fatalf("top of stack is not a map (was %s)", res[len(res)-1].Parent().String())
	}
	v, ok := m.Get(key)
	if !ok {
		t.Fatalf("result missing key %q in %s", key, res[len(res)-1].String())
	}
	return v
}

// TestCheckProp_AlwaysPasses confirms a property that's always true
// reports ok=true after `runs` iterations.
func TestCheckProp_AlwaysPasses(t *testing.T) {
	r := testRegistry(t)
	// Property: any drawn Integer in [0, 100) is >= 0.
	// gen body: draws one Integer.
	// property body: top-of-stack >= 0.
	src := `Test.check-prop "non-negative" [r.int 0 100] [0 gte] 20 1 0`
	ok := runTestAndGetField(t, r, src, "ok")
	b, err := ok.AsConcreteBoolean()
	if err != nil {
		t.Fatalf("ok field not Boolean: %v", err)
	}
	if !b {
		t.Errorf("expected ok=true for always-true property")
	}
}

// TestCheckProp_PropertyBodyCanImportNativeModule pins §11b.1: a
// property body may `import "boru:<name>"` and use the namespace across
// MULTIPLE runs. The module is resolved once and cached; because each
// property iteration runs via CallBoru — whose def-cleanup strips the
// namespace binding installed during the body — a naive load-once guard
// left the module "loaded" but `pkg` unbound from iteration 2 on. The
// fix re-binds the cached desc's namespace when absent.
func TestCheckProp_PropertyBodyCanImportNativeModule(t *testing.T) {
	r := testRegistry(t)
	InstallResolver(r) // enable `import "boru:math-util"` resolution

	// Property body imports boru:math every iteration and uses MathUtil.sqrt;
	// runs > 1 is the case that regressed.
	src := `Test.check-prop "sqrt-ok" [r.int 1 10] ` +
		`[drop import "boru:math-util" 4.0 MathUtil.sqrt end 2.0 eq] 5 1 0`
	ok := runTestAndGetField(t, r, src, "ok")
	b, err := ok.AsConcreteBoolean()
	if err != nil {
		t.Fatalf("ok field not Boolean: %v", err)
	}
	if !b {
		// Surface the recorded error to make a regression obvious.
		errField := runTestAndGetField(t, r, src, "error")
		t.Errorf("property body importing boru:math should pass across runs; got ok=false, error=%s", errField.String())
	}
}

// TestSkip_RecordsSkippedWithoutRunning pins §11b.4: Test.skip is a
// drop-in for Test.check-prop that records a skipped result (ok=true)
// without running the bodies — a [false] property that WOULD fail if run
// must not contribute a failure.
func TestSkip_RecordsSkippedWithoutRunning(t *testing.T) {
	r := testRegistry(t)
	res := runTestAndGetField(t, r, `Test.skip "wip" [r.int 0 9] [false] 10 1 0`, "skipped")
	if b, err := res.AsConcreteBoolean(); err != nil || !b {
		t.Fatalf("Test.skip should mark the result skipped=true, got %v (err %v)", res, err)
	}
	// The skipped [false] property would fail if run; it must not raise
	// the run's failure count.
	out := runTestBoru(t, r, `Test.fail-count`)
	if n, _ := native.AsInteger(out[len(out)-1]); n != 0 {
		t.Errorf("skipped properties must not count as failures; fail-count = %d, want 0", n)
	}
}

// TestReport_OneLinePerProperty pins §11b.5: Test.report renders one
// line per recorded property (pass/FAIL/skip) plus a tally.
func TestReport_OneLinePerProperty(t *testing.T) {
	r := testRegistry(t)
	runTestBoru(t, r, `Test.check-prop "good" [r.int 0 9] [0 gte] 5 1 0`)
	runTestBoru(t, r, `Test.check-prop "bad" [r.int 0 9] [false] 5 1 0`)
	runTestBoru(t, r, `Test.skip "parked" [r.int 0 9] [false] 5 1 0`)

	out := runTestBoru(t, r, `Test.report`)
	s, err := out[len(out)-1].AsConcreteString()
	if err != nil {
		t.Fatalf("Test.report should return a String, got %v", out[len(out)-1])
	}
	for _, want := range []string{"pass: good", "FAIL: bad", "skip: parked", "1 passed", "1 failed", "1 skipped"} {
		if !strings.Contains(s, want) {
			t.Errorf("Test.report output missing %q:\n%s", want, s)
		}
	}
}

// TestCheckProp_FailsOnSpecificInput introduces a property that's
// very likely to fail within 100 iterations: "value > 0". The
// generator draws from [0, 100), so 0 will eventually appear and
// the property will report failure with the failing input.
func TestCheckProp_FailsOnSpecificInput(t *testing.T) {
	r := testRegistry(t)
	src := `Test.check-prop "positive" [r.int 0 100] [0 gt] 200 1 0`
	res := runTestBoru(t, r, src)
	m, _ := native.AsMap(res[0])
	okV, _ := m.Get("ok")
	ok, _ := okV.AsConcreteBoolean()
	if ok {
		t.Fatal("expected failure for positive property (0 should be drawn within 200 runs)")
	}
	failing, _ := m.Get("failing-input")
	n, err := failing.AsConcreteInteger()
	if err != nil {
		t.Fatalf("failing-input not Integer: %v", err)
	}
	if n != 0 {
		t.Errorf("failing-input=%d, want 0 (the only value in [0,100) that fails `0 gt`)", n)
	}
}

// TestCheckProp_DeterministicAcrossRuns confirms same seed → same
// failing input. The Stage-5 reducer will rely on this.
func TestCheckProp_DeterministicAcrossRuns(t *testing.T) {
	// Property "value != 7" — fails the moment the gen draws 7.
	// With seed=42 + 200 iterations drawing from [0, 100), this
	// is very likely to fail somewhere; the iteration AND value at
	// which it fails must be identical across registries.
	draw := func() int64 {
		r := testRegistry(t)
		src := `Test.check-prop "not-seven" [r.int 0 100] [7 neq] 200 42 0`
		res := runTestBoru(t, r, src)
		m, _ := native.AsMap(res[0])
		v, _ := m.Get("failing-input")
		n, _ := v.AsConcreteInteger()
		return n
	}
	a := draw()
	b := draw()
	if a != b {
		t.Errorf("seed=42 produced different failing inputs across registries: %d vs %d", a, b)
	}
}

// TestCheckProp_RunsCount confirms the result records the actual
// number of iterations executed (which equals `runs` for passing
// properties, and the iteration that failed for failing ones).
func TestCheckProp_RunsCount(t *testing.T) {
	r := testRegistry(t)
	src := `Test.check-prop "always-true" [42] [true] 7 1 0`
	res := runTestBoru(t, r, src)
	m, _ := native.AsMap(res[0])
	v, _ := m.Get("runs")
	n, _ := v.AsConcreteInteger()
	if n != 7 {
		t.Errorf("runs=%d, want 7", n)
	}
}

// TestCheckProp_RejectsNonBooleanProperty: a property body that
// produces a non-Boolean value should fail with an error, not pass
// silently.
func TestCheckProp_RejectsNonBooleanProperty(t *testing.T) {
	r := testRegistry(t)
	src := `Test.check-prop "bad-prop" [r.int 0 100] [42] 1 1 0`
	res := runTestBoru(t, r, src)
	m, _ := native.AsMap(res[0])
	okV, _ := m.Get("ok")
	ok, _ := okV.AsConcreteBoolean()
	if ok {
		t.Fatal("expected non-Boolean property body to fail")
	}
	errV, _ := m.Get("error")
	if native.IsNone(&errV) {
		t.Errorf("error field should be populated when property returns non-Boolean")
	}
}

// TestRunProperty_ViaSpecMap confirms the boru `run-property` fn
// destructures a PropertySpec map and dispatches the same way as
// the imperative `Test.check-prop` call.
func TestRunProperty_ViaSpecMap(t *testing.T) {
	r := testRegistry(t)
	// Use Test.prop (Go native) to build the spec — its NoEvalArgs
	// boundary preserves the quoted gen/property bodies so the
	// embedded list literal [r.int 0 100] survives without being
	// auto-evaluated at construction time.
	// Forward form for Test.prop so sig positions align with the
	// canonical String/List/List declaration.
	src := `
		def p (Test.prop "non-negative-via-spec" [r.int 0 100] [0 gte])
		p Test.run-property
	`
	ok := runTestAndGetField(t, r, src, "ok")
	b, _ := ok.AsConcreteBoolean()
	if !b {
		t.Errorf("run-property should report ok=true for always-true property")
	}
	nameV := runTestAndGetField(t, r, src, "name")
	name, _ := nameV.AsConcreteString()
	if name != "non-negative-via-spec" {
		t.Errorf("result name=%q, want non-negative-via-spec", name)
	}
}

// TestCheckProp_ResultsAccumulated confirms PropertyResults flow
// into the same testRun results bucket as table-driven tests, so a
// single Test.summary covers both.
func TestCheckProp_ResultsAccumulated(t *testing.T) {
	r := testRegistry(t)
	// One passing, one failing.
	runTestBoru(t, r, `Test.check-prop "ok-prop" [r.int 0 10] [0 gte] 5 1 0`)
	runTestBoru(t, r, `Test.check-prop "bad-prop" [r.int 0 10] [false] 5 1 0`)

	resV := runTestBoru(t, r, `Test.results`)
	if len(resV) == 0 {
		t.Fatal("Test.results returned nothing")
	}
	// Just confirm we have entries and at least one failure recorded.
	failV := runTestBoru(t, r, `Test.fail-count`)
	if len(failV) == 0 {
		t.Fatal("Test.fail-count returned nothing")
	}
	n, _ := failV[0].AsConcreteInteger()
	if n < 1 {
		t.Errorf("fail-count=%d, want >=1", n)
	}
}

// TestCheckProp_ShrinksFailingInput confirms the Stage-5 reducer
// integration: when a property fails on a "large" input, the
// reducer minimises that input toward 0 (for the predicate "n < 10")
// — the failing input ends up as the SMALLEST integer ≥ 10 that
// still violates `n < 10`, namely 10 itself.
func TestCheckProp_ShrinksFailingInput(t *testing.T) {
	r := testRegistry(t)
	// Generate large ints in [0, 1000). Property: n < 10. Fails on
	// every value ≥ 10. Reducer should shrink any failing input
	// down to 10 (smallest violator).
	src := `Test.check-prop "n-lt-10" [r.int 0 1000] [10 lt] 50 1 200`
	res := runTestBoru(t, r, src)
	m, _ := native.AsMap(res[0])
	okV, _ := m.Get("ok")
	if ok, _ := okV.AsConcreteBoolean(); ok {
		t.Skip("property unexpectedly passed — no shrink to verify")
	}
	failingV, _ := m.Get("failing-input")
	shrunkV, _ := m.Get("shrunk-input")
	failing, _ := failingV.AsConcreteInteger()
	shrunk, _ := shrunkV.AsConcreteInteger()
	if shrunk > failing {
		t.Errorf("shrunk-input=%d should be <= failing-input=%d", shrunk, failing)
	}
	if shrunk != 10 {
		t.Errorf("shrunk-input=%d, want 10 (smallest violator of n<10)", shrunk)
	}
	// shrunk-source should be non-empty when shrinking actually ran.
	srcV, _ := m.Get("shrunk-source")
	srcStr, _ := srcV.AsConcreteString()
	if srcStr == "" {
		t.Error("shrunk-source is empty; expected the pretty form of the shrunk literal")
	}
	t.Logf("failing=%d → shrunk=%d (source=%q)", failing, shrunk, srcStr)
}

// TestCheckProp_ShrinkDisabledByMaxShrinks confirms maxShrinks=0
// bypasses the reducer — shrunk-input equals failing-input verbatim.
func TestCheckProp_ShrinkDisabledByMaxShrinks(t *testing.T) {
	r := testRegistry(t)
	src := `Test.check-prop "skip-shrink" [r.int 0 1000] [10 lt] 50 1 0`
	res := runTestBoru(t, r, src)
	m, _ := native.AsMap(res[0])
	okV, _ := m.Get("ok")
	if ok, _ := okV.AsConcreteBoolean(); ok {
		t.Skip("property passed — no failure to skip-shrink")
	}
	failingV, _ := m.Get("failing-input")
	shrunkV, _ := m.Get("shrunk-input")
	failing, _ := failingV.AsConcreteInteger()
	shrunk, _ := shrunkV.AsConcreteInteger()
	if shrunk != failing {
		t.Errorf("max-shrinks=0 should leave shrunk=failing; got shrunk=%d failing=%d",
			shrunk, failing)
	}
}

// TestCheckProp_GenProgramShrinkingReachesSmallerSource confirms that
// gen-program shrinking — not just value-level shrinking — runs when
// the gen body has structure to mutate. With a gen body of
// `[r.int 0 1000]` and a property that fails on every value, the
// reducer can shrink BOTH the range literals (1000 → smaller) AND
// the produced value (depending on what range the reducer settles).
// The shrunk-source is non-empty and the shrunk-input is <= the
// original failing input.
func TestCheckProp_GenProgramShrinkingReachesSmallerSource(t *testing.T) {
	r := testRegistry(t)
	src := `Test.check-prop "always-fail" [r.int 0 1000] [false] 5 7 200`
	res := runTestBoru(t, r, src)
	m, _ := native.AsMap(res[0])
	okV, _ := m.Get("ok")
	if ok, _ := okV.AsConcreteBoolean(); ok {
		t.Fatal("property `false` should fail every iteration")
	}
	sourceV, _ := m.Get("shrunk-source")
	sourceStr, _ := sourceV.AsConcreteString()
	if sourceStr == "" {
		t.Error("expected non-empty shrunk-source from gen-program shrinking path")
	}
	// failing-input came from the original gen run; shrunk-input from
	// the reduced form's eval. With the gen body `[r.int 0 1000]`,
	// the reducer can drop ops and/or shrink the range literals.
	failV, _ := m.Get("failing-input")
	shrunkV, _ := m.Get("shrunk-input")
	t.Logf("failing=%s  shrunk=%s  source=%q", failV.String(), shrunkV.String(), sourceStr)
}

// Stub to keep parser referenced when no other test in this file
// uses it directly.
var _ = parser.Parse
