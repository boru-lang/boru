package modules

import (
	"testing"

	"github.com/boru-lang/boru/lang/go/native"
	parser "github.com/boru-lang/boru/parser/go"
)

// coverRun loads boru:test + boru:sift and runs src on a fresh registry in
// INTERPRETER mode (coverage is a tree-walk measurement).
func coverRun(t *testing.T, src string) ([]native.Value, error) {
	t.Helper()
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.SetParseFunc(parser.Parse)
	InstallResolver(r)
	vals, perr := parser.Parse(src)
	if perr != nil {
		t.Fatalf("parse: %v", perr)
	}
	return native.NewTop(r).Run(vals)
}

// Test.cover [body] records the rows the module-under-test executes; Test.coverage
// reports {covered,total,percent,uncovered} against the module's denominator.
func TestCoverAPIEndToEnd(t *testing.T) {
	// The module-under-test is imported INSIDE Test.cover so its module-load
	// def lines are counted alongside the runtime execution its ops drive.
	out, err := coverRun(t, `
import "boru:test"
Test.cover [
  import "boru:sift"
  Sift.families
  Sift.parse kv/q {sep:':'} "a: 1"
]
Test.coverage "boru:sift"`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 result (the report), got %d: %v", len(out), out)
	}
	rep, rerr := native.AsMap(out[0])
	if rerr != nil || !native.IsConcrete(&out[0]) {
		t.Fatalf("report is not a map: %v", out[0])
	}
	get := func(k string) native.Value { v, _ := rep.Get(k); return v }
	total, _ := addrOf(get("total")).AsConcreteInteger()
	covered, _ := addrOf(get("covered")).AsConcreteInteger()
	pct, _ := addrOf(get("percent")).AsConcreteFloat()
	if total <= 0 {
		t.Fatalf("total denominator rows = %d, want > 0", total)
	}
	if covered <= 0 {
		t.Fatalf("covered rows = %d, want > 0 (the hook must fire for sift)", covered)
	}
	if covered > total {
		t.Fatalf("covered %d exceeds total %d", covered, total)
	}
	if pct <= 0 || pct > 100 {
		t.Fatalf("percent = %v, want (0,100]", pct)
	}
	// uncovered is a list; its length is total-covered.
	uncov, uerr := native.AsList(get("uncovered"))
	if uerr != nil {
		t.Fatalf("uncovered is not a list: %v", get("uncovered"))
	}
	if int64(uncov.Len()) != total-covered {
		t.Fatalf("uncovered len %d != total-covered %d", uncov.Len(), total-covered)
	}
	t.Logf("sift coverage from a 2-op exercise: %d/%d rows (%.1f%%)", covered, total, pct)
}

// Test.coverage on an unregistered id raises test_cover_no_source.
func TestCoverAPINoSource(t *testing.T) {
	_, err := coverRun(t, `import "boru:test"  Test.coverage "boru:nonesuch"`)
	if err == nil {
		t.Fatalf("expected an error for an unregistered coverage id")
	}
	if got := err.Error(); !contains(got, "test_cover_no_source") {
		t.Fatalf("error = %q, want test_cover_no_source", got)
	}
}

// CoverageFor + ArmCoverageCollector are the Go seams the `boru test --coverage`
// runner uses: an unregistered id reports ok=false; a registered source reports
// covered/total against the rows the armed collector recorded.
func TestCoverageForAPI(t *testing.T) {
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.SetParseFunc(parser.Parse)

	if _, _, ok := CoverageFor(r, "nope"); ok {
		t.Fatal("unregistered id must report ok=false")
	}

	// A 2-executable-row source; arm the collector, record one row, report.
	r.RegisterCoverSource("m", "def a 1\ndef b 2\n")
	disarm := ArmCoverageCollector(r)
	defer disarm()
	activeCover(r).record("m", 1)

	covered, uncovered, ok := CoverageFor(r, "m")
	if !ok {
		t.Fatal("registered id must report ok=true")
	}
	// Row 1 was recorded (covered), so row 2 is the sole uncovered line.
	if len(covered) != 1 || covered[0] != 1 {
		t.Fatalf("covered = %v, want [1]", covered)
	}
	if len(uncovered) != 1 || uncovered[0] != 2 {
		t.Fatalf("uncovered = %v, want [2]", uncovered)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
