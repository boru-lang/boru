package help

import "testing"

// example_static_precedence_test.go pins the precedence rule inside
// GenerateDynamicExamples: which of the two recorded results for an example
// expression — the one baked in at build time, or the one the live engine
// would produce now — the generator trusts.
//
// The rule has an exception, and both halves matter. Trusting the snapshot
// too little re-runs every example on every registration for byte-identical
// output; trusting it too much ships a refusal that a later overload has
// already falsified. TestGenerateDynamicExamplesCorrectsStaleRefusal covers
// the exception on whatever expression the generator happens to produce;
// this pins BOTH halves on one expression the test itself has checked is
// generated, so neither half can go vacuous.

// staticPrecedenceInfo is a two-Boolean word — the shape whose single
// generated example is `<name> true false`.
func staticPrecedenceInfo(name string) FuncInfo {
	return FuncInfo{
		Name:        name,
		ForwardArgs: true,
		Sigs:        []SigInfo{{Args: []string{"Boolean", "Boolean"}}},
	}
}

// TestStaticExampleResultWinsUnlessRefused pins the normal case: a
// build-time result stands, and the live engine is never asked for it. That
// is what makes example generation cheap — the same engine computed that
// string already, and re-running it is pure cost.
func TestStaticExampleResultWinsUnlessRefused(t *testing.T) {
	info := staticPrecedenceInfo("wsp-kept")
	exprs := ExampleExprs(info)
	if len(exprs) != 1 {
		t.Fatalf("ExampleExprs = %v, want exactly one expression to pin", exprs)
	}
	expr := exprs[0]

	origStatic, origDyn := exampleResults, dynamicExampleResults
	t.Cleanup(func() { exampleResults, dynamicExampleResults = origStatic, origDyn })

	// A good build-time result: nothing is re-evaluated, nothing is recorded.
	exampleResults = map[string]string{expr: "true"}
	dynamicExampleResults = map[string]string{}
	GenerateDynamicExamples(info, func(e string) (string, error) {
		t.Errorf("the live engine was asked to re-evaluate %q, whose build-time result stands", e)
		return "false", nil
	})
	if got, ok := dynamicExampleResults[expr]; ok {
		t.Errorf("dynamic[%q] = %q, want no entry: a good static result is not shadowed", expr, got)
	}
	if got := evalExample(expr, info.Name, info.Sigs[0], nil); got != "true" {
		t.Errorf("evalExample = %q, want the build-time result %q", got, "true")
	}

	// The exception, on the same expression: a recorded REFUSAL is exactly
	// the kind of result a new definition can falsify, so it IS re-evaluated
	// and the live answer replaces it. Without this half the assertion above
	// would also pass with the refusal exception deleted.
	exampleResults = map[string]string{expr: "error [boru/type_error]"}
	dynamicExampleResults = map[string]string{}
	calls := 0
	GenerateDynamicExamples(info, func(string) (string, error) {
		calls++
		return "true", nil
	})
	if calls != 1 {
		t.Fatalf("eval called %d times for a recorded refusal, want 1", calls)
	}
	if got := dynamicExampleResults[expr]; got != "true" {
		t.Errorf("dynamic[%q] = %q, want the corrected live result %q", expr, got, "true")
	}
	if got := evalExample(expr, info.Name, info.Sigs[0], nil); got != "true" {
		t.Errorf("evalExample = %q, want the correction to be what a reader sees", got)
	}
}
