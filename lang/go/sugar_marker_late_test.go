package lang

import (
	"strings"
	"testing"
)

// Late-consumer regression pins for the ADR-012 sugar markers: sites
// that receive an angle marker as DATA (not stepped at the pointer)
// must lower it locally before evaluating. Each test here pinned a
// real post-conversion failure.

// checkErrors returns the error-severity diagnostics of a Check run.
func checkErrors(t *testing.T, src string) []string {
	t.Helper()
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	res, _ := a.Check(src)
	var errs []string
	for _, d := range res.Diagnostics {
		if string(d.Severity) == "error" {
			errs = append(errs, d.Code+": "+d.Detail)
		}
	}
	return errs
}

// A def-head angle marker in ANOTHER word's forward window must not be
// committed to that word's use-site form: import's /q slots keep its
// pre-scan walking past `def`, and the marker at that position belongs
// to def's dispatch (headForm), not import's. The false positive was
// check-mode `undefined_word: Box` on this exact shape (the engine's
// viableConsumes gate in resolveForwardArgs pins the fix).
func TestCheckAngleHeadAfterImportScan(t *testing.T) {
	errs := checkErrors(t, `import "boru:vm" def Box<T> class {value:T}`)
	if len(errs) != 0 {
		t.Errorf("import + generic def head must check clean, got %v", errs)
	}
}

// A typed-list child annotation carrying an angle marker
// (`xs:[:Box<Integer>]`) lowers through ResolveChildTypeExpr; without
// the lowering the declared child stayed the raw marker and the value
// side failed to unify (check-mode type_error false positive).
func TestChildTypeAngleMarkerLowers(t *testing.T) {
	src := `def Box<T> class {value:T} def xs:[:Box<Integer>] [(make Box<Integer> {value:1})] end (xs get 0) dot value`
	if errs := checkErrors(t, src); len(errs) != 0 {
		t.Errorf("angle child annotation must check clean, got %v", errs)
	}
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	out, err := a.RunInterp(src)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(out) != 1 || out[0] != int64(1) {
		t.Errorf("got %v, want [1]", out)
	}
	// Negative: a child annotation naming the wrong instantiation still
	// declines — the lowering must not loosen the unify.
	bad := `def Box<T> class {value:T} def xs:[:Box<String>] [(make Box<Integer> {value:1})] end xs`
	if _, err := a.RunInterp(bad); err == nil || !strings.Contains(err.Error(), "not unify") {
		t.Errorf("wrong child instantiation must decline to bind, got %v", err)
	}
}

// A case-arm angle marker (`case b [Box<Integer> …]`) lowers before
// the paren-arm inline evaluation; without it the raw marker failed to
// unify and every generic-type arm fell to the default.
func TestCaseArmAngleMarkerLowers(t *testing.T) {
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	src := `def Box<T> class {value:T} def b (make Box<Integer> {value:7}) end case b [Box<Integer> 'int-box' 'other']`
	out, err := a.RunInterp(src)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(out) != 1 || out[0] != "int-box" {
		t.Errorf("got %v, want [int-box]", out)
	}
	// Negative: a non-matching instantiation arm still falls through.
	miss := `def Box<T> class {value:T} def b (make Box<Integer> {value:7}) end case b [Box<String> 'str-box' 'other']`
	out, err = a.RunInterp(miss)
	if err != nil {
		t.Fatalf("run miss: %v", err)
	}
	if len(out) != 1 || out[0] != "other" {
		t.Errorf("got %v, want [other]", out)
	}
}
