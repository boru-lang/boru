package lang

import "testing"

// Pins for the three checker design refits (design/legacy/CHECKER-COMPLETION.0.ignore
// §7): the RuntimeMirror diagnostic classification and the narrowed
// compile refusal, and the central caught-region re-attribution. The
// BeginCompilePass helper is pinned kernel-side
// (eng/go/drypass_test.go::TestBeginCompilePassArmsTheRitual).

// findDiag returns the first diagnostic with the given code, and whether
// one exists.
func findDiag(diags []CheckDiagnostic, code string) (CheckDiagnostic, bool) {
	for _, d := range diags {
		if d.Code == code {
			return d, true
		}
	}
	return CheckDiagnostic{}, false
}

func TestCompileRefusalSkipsRuntimeMirrors(t *testing.T) {
	// A guaranteed-runtime-error MIRROR (exact model — the program
	// compiles and raises identically) must not flip the row to a "check
	// diagnostics" refusal; a MODEL-UNDERMINING error (undefined word —
	// dispatch didn't resolve) must.
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	_, reason, res, cerr := a.CompileCheck("20 div 0")
	if cerr != nil {
		t.Fatalf("CompileCheck(div 0): %v", cerr)
	}
	if reason == "check diagnostics" {
		t.Fatalf("mirror diagnostic wrongly tripped the compile refusal (reason %q)", reason)
	}
	d, ok := findDiag(res.Diagnostics, "arith_error")
	if !ok || !d.RuntimeMirror || d.Severity != SeverityError {
		t.Fatalf("compile pass must still SURFACE the mirror (error severity, RuntimeMirror): %+v", res.Diagnostics)
	}

	b, err := New()
	if err != nil {
		t.Fatal(err)
	}
	_, reason, _, cerr = b.CompileCheck("totally-undefined-zz")
	if cerr != nil {
		t.Fatalf("CompileCheck(undefined): %v", cerr)
	}
	if reason != "check diagnostics" {
		t.Fatalf("model-undermining error must keep refusing, got reason %q", reason)
	}
}

func TestCompileAndCheckDiagnosticsAgree(t *testing.T) {
	// The compile pass and the plain check pass must report the SAME
	// mirror finding for the same source — the divergence the old
	// !Compiling gating introduced is retired.
	src := `{a:1} dotr missing`
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	plain, err := a.Check(src)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	b, err := New()
	if err != nil {
		t.Fatal(err)
	}
	_, _, compiled, cerr := b.CompileCheck(src)
	if cerr != nil {
		t.Fatalf("CompileCheck: %v", cerr)
	}
	pd, pok := findDiag(plain.Diagnostics, "not_found")
	cd, cok := findDiag(compiled.Diagnostics, "not_found")
	if !pok || !cok || pd.Detail != cd.Detail {
		t.Fatalf("check/compile mirror disagreement: plain=%+v compile=%+v", plain.Diagnostics, compiled.Diagnostics)
	}
}

func TestCaughtRegionReattribution(t *testing.T) {
	// Every would-be-error finding inside a `do [...]` body — where the
	// runtime TRAPS the error — is downgraded to info with
	// CaughtAtRuntime stamped, uniformly across families: the
	// guaranteed-error mirrors AND the model-level undefined_word (the
	// program `do [undefined] error [dot code]` RUNS — it reads the
	// caught error's code).
	cases := []struct{ name, src, code string }{
		{"mirror: strict-accessor miss", `do [{a:1} !. b] error [dot message]`, "not_found"},
		{"mirror: unconditional raise", `do [raise "x"] error [dot code]`, "unconditional_raise"},
		{"model: undefined word", `do [totally-undefined-zz] error [dot code]`, "undefined_word"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a, err := New()
			if err != nil {
				t.Fatal(err)
			}
			res, err := a.Check(tc.src)
			if err != nil {
				t.Fatalf("Check: %v", err)
			}
			for _, d := range res.Diagnostics {
				if d.Severity == SeverityError {
					t.Fatalf("caught-region finding kept an ERROR verdict (a working program would be flagged): %+v", d)
				}
			}
			if tc.code == "unconditional_raise" {
				// The raise mirror is reachability-gated to the top level;
				// inside the do body it never fires at all — assert only
				// the absence of an error verdict above.
				return
			}
			d, ok := findDiag(res.Diagnostics, tc.code)
			if !ok {
				t.Fatalf("caught finding %s should survive as info, got %+v", tc.code, res.Diagnostics)
			}
			if !d.CaughtAtRuntime || d.Severity != SeverityInfo {
				t.Fatalf("caught finding not re-attributed (want info + CaughtAtRuntime): %+v", d)
			}
		})
	}
}
