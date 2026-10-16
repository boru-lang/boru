package modules

import (
	"strings"
	"testing"

	"github.com/boru-lang/boru/lang/go/native"
	parser "github.com/boru-lang/boru/parser/go"
)

// testRegistry returns a registry with the boru:test module installed
// as defs (mirrors what `import "boru:test"` would do).
func testRegistry(t *testing.T) *native.Registry {
	t.Helper()
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.SetParseFunc(parser.Parse)
	desc, err := BuildTestModule(r)
	if err != nil {
		t.Fatalf("BuildTestModule: %v", err)
	}
	for name, exp := range desc.Exports {
		r.Defs.Push(name, native.NewMap(exp))
	}
	return r
}

func runTestBoru(t *testing.T, r *native.Registry, src string) []native.Value {
	t.Helper()
	values, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	out, err := native.NewTop(r).Run(values)
	if err != nil {
		t.Fatalf("run: %v\n--- src ---\n%s", err, src)
	}
	return out
}

// Test.skip "name" [body] parks a CASE: the body is NEVER run (a raise inside
// it must not fire), and it records ok=true + skipped=true without failing the
// run. (Stack form `[body] "name" Test.skip` — the case sig is [String, List].)
// The case is parked inside a Test.describe so run.path is non-empty, which
// also exercises skipCase's path-copy into the recorded result.
func TestSkipCase(t *testing.T) {
	r := testRegistry(t)
	// The body would raise if run; runTestBoru asserts no error, so a clean run
	// proves skip did not execute the body.
	runTestBoru(t, r, `Test.describe "grp" [ [ raise kaboom "x" ] "parked" Test.skip ]`)
	run := activeRun(r)
	if len(run.results) != 1 {
		t.Fatalf("expected 1 recorded result, got %d", len(run.results))
	}
	if run.failures != 0 {
		t.Errorf("a skipped case must not count as a failure; failures=%d", run.failures)
	}
	m, err := native.AsMap(run.results[0])
	if err != nil {
		t.Fatalf("result is not a map: %v", err)
	}
	skipped, _ := m.Get("skipped")
	if b, _ := skipped.AsConcreteBoolean(); !b {
		t.Errorf("skipped = %v, want true", skipped)
	}
	okv, _ := m.Get("ok")
	if b, _ := okv.AsConcreteBoolean(); !b {
		t.Errorf("ok = %v, want true", okv)
	}
	name, _ := m.Get("name")
	if s, _ := name.AsConcreteString(); s != "parked" {
		t.Errorf("name = %q, want parked", s)
	}
	// The recorded path carries the enclosing describe group — this exercises
	// skipCase's path-copy loop (empty at top level, one entry here).
	pathV, _ := m.Get("path")
	pathL, err := native.AsList(pathV)
	if err != nil {
		t.Fatalf("path is not a list: %v", err)
	}
	pv := pathL.Slice()
	if len(pv) != 1 {
		t.Fatalf("path length = %d, want 1", len(pv))
	}
	if s, _ := pv[0].AsConcreteString(); s != "grp" {
		t.Errorf("path[0] = %q, want grp", s)
	}
}

func TestTestModuleExports(t *testing.T) {
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.SetParseFunc(parser.Parse)
	desc, err := BuildTestModule(r)
	if err != nil {
		t.Fatalf("BuildTestModule: %v", err)
	}
	testExp, ok := desc.Exports["Test"]
	if !ok {
		t.Fatal("missing 'test' export")
	}
	for _, name := range []string{"test", "describe", "it", "results", "summary", "reset", "TestCase", "TestSet", "TestSpec", "TestResult", "spec", "case", "run-spec"} {
		if _, ok := testExp.Get(name); !ok {
			t.Errorf("missing test export %q", name)
		}
	}
	assertExp, ok := desc.Exports["Assert"]
	if !ok {
		t.Fatal("missing 'assert' export")
	}
	for _, name := range []string{"equal", "not-equal", "ok", "throws", "match"} {
		if _, ok := assertExp.Get(name); !ok {
			t.Errorf("missing assert export %q", name)
		}
	}
}

func TestImperativePass(t *testing.T) {
	r := testRegistry(t)
	runTestBoru(t, r, `
		Test.test "math works" [(3 4 add) 7 Assert.equal]
	`)
	result := runTestBoru(t, r, `Test.summary`)
	m, _ := native.AsMap(result[0])
	total, _ := m.Get("total")
	passed, _ := m.Get("passed")
	failed, _ := m.Get("failed")
	tn, _ := native.AsInteger(total)
	pn, _ := native.AsInteger(passed)
	fn, _ := native.AsInteger(failed)
	if tn != 1 || pn != 1 || fn != 0 {
		t.Errorf("summary: total=%d passed=%d failed=%d, want 1/1/0", tn, pn, fn)
	}
}

func TestImperativeFail(t *testing.T) {
	r := testRegistry(t)
	runTestBoru(t, r, `
		Test.test "math is wrong" [(3 4 add) 8 Assert.equal]
	`)
	result := runTestBoru(t, r, `Test.fail-count`)
	n, _ := native.AsInteger(result[0])
	if n != 1 {
		t.Errorf("fail-count = %d, want 1", n)
	}
}

func TestImperativeDescribeNesting(t *testing.T) {
	r := testRegistry(t)
	runTestBoru(t, r, `
		Test.describe "outer" [
			Test.test "a" [true Assert.ok]
			Test.describe "inner" [
				Test.test "b" [false true Assert.not-equal]
			]
		]
	`)
	result := runTestBoru(t, r, `Test.results`)
	td, ok := native.DataOf(&result[0]).(native.TableData)
	if !ok {
		t.Fatalf("results not a TableData: %T", native.DataOf(&result[0]))
	}
	if len(td.Rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(td.Rows))
	}
	// Second row should have path = ["outer", "inner"]
	m, _ := native.AsMap(td.Rows[1])
	pathV, _ := m.Get("path")
	pathL, _ := native.AsList(pathV)
	if pathL.Len() != 2 {
		t.Errorf("inner path length = %d, want 2", pathL.Len())
	}
}

func TestAssertThrows(t *testing.T) {
	r := testRegistry(t)
	runTestBoru(t, r, `
		Test.test "div by zero" [[1 0 div] Assert.throws]
	`)
	result := runTestBoru(t, r, `Test.fail-count`)
	n, _ := native.AsInteger(result[0])
	if n != 0 {
		t.Errorf("Assert.throws should pass; fail-count = %d", n)
	}
}

func TestResultsThroughReport(t *testing.T) {
	r := testRegistry(t)
	desc, err := BuildReportModule(r)
	if err != nil {
		t.Fatal(err)
	}
	for name, exp := range desc.Exports {
		r.Defs.Push(name, native.NewMap(exp))
	}
	runTestBoru(t, r, `
		Test.test "ok" [1 1 Assert.equal]
		Test.test "bad" [1 2 Assert.equal]
	`)
	result := runTestBoru(t, r, `Test.results Report.value`)
	s, _ := native.AsString(result[0])
	if !strings.Contains(s, "name") || !strings.Contains(s, "ok") {
		t.Errorf("results table missing expected columns:\n%s", s)
	}
}

func TestSpecRunnerHappyPath(t *testing.T) {
	r := testRegistry(t)
	runTestBoru(t, r, `
		def double fn [[n:Integer] [Integer] [n 2 mul]]
		def my-spec {
		  name: "doubling"
		  subject: double/q
		  cases: [
		    {name: "doubles 3" in: [3] out: 6}
		    {name: "doubles 0" in: [0] out: 0}
		    {name: "doubles negative" in: [-5] out: -10}
		  ]
		  subs: []
		}
		my-spec Test.run-spec
	`)
	result := runTestBoru(t, r, `Test.summary`)
	m, _ := native.AsMap(result[0])
	total, _ := m.Get("total")
	failed, _ := m.Get("failed")
	tn, _ := native.AsInteger(total)
	fn, _ := native.AsInteger(failed)
	if tn != 3 || fn != 0 {
		t.Errorf("spec summary: total=%d failed=%d, want 3/0", tn, fn)
	}
}

func TestSpecRunnerDetectsFailures(t *testing.T) {
	r := testRegistry(t)
	runTestBoru(t, r, `
		def double fn [[n:Integer] [Integer] [n 2 mul]]
		def bad-spec {
		  name: "bad doubles"
		  subject: double/q
		  cases: [
		    {name: "wrong" in: [3] out: 7}
		    {name: "right" in: [4] out: 8}
		  ]
		  subs: []
		}
		bad-spec Test.run-spec
	`)
	result := runTestBoru(t, r, `Test.summary`)
	m, _ := native.AsMap(result[0])
	failed, _ := m.Get("failed")
	fn, _ := native.AsInteger(failed)
	if fn != 1 {
		t.Errorf("spec summary: failed=%d, want 1", fn)
	}
}
