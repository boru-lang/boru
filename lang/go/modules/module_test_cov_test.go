package modules

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/boru-lang/boru/lang/go/native"
	parser "github.com/boru-lang/boru/parser/go"
)

// (Uses the testRegistry / runTestBoru helpers from test_test.go and the
// runTestAndGetField helper from test_pbt_test.go — same package.)

// tcovFailCount reads Test.fail-count as an int64.
func tcovFailCount(t *testing.T, r *native.Registry) int64 {
	t.Helper()
	out := runTestBoru(t, r, `Test.fail-count`)
	n, _ := native.AsInteger(out[len(out)-1])
	return n
}

// TestTestCovAssertNotEqualFails pins the negative arm of
// Assert.not-equal: equal values fail the enclosing case.
func TestTestCovAssertNotEqualFails(t *testing.T) {
	r := testRegistry(t)
	runTestBoru(t, r, `Test.test "same" [1 1 Assert.not-equal]`)
	if n := tcovFailCount(t, r); n != 1 {
		t.Errorf("fail-count = %d, want 1", n)
	}
}

// TestTestCovAssertOkTruthiness drives isTruthy through every arm:
// booleans, None, bare type literals, and plain truthy values.
func TestTestCovAssertOkTruthiness(t *testing.T) {
	r := testRegistry(t)
	runTestBoru(t, r, `
		Test.test "int is truthy" [1 Assert.ok]
		Test.test "string is truthy" ['x' Assert.ok]
		Test.test "true is truthy" [true Assert.ok]
		Test.test "false is falsy" [false Assert.ok]
		Test.test "none is falsy" [None Assert.ok]
		Test.test "type literal is falsy" [Integer Assert.ok]
	`)
	if n := tcovFailCount(t, r); n != 3 {
		t.Errorf("fail-count = %d, want 3 (false, None, Integer)", n)
	}
}

// TestTestCovAssertThrowsNegative pins Assert.throws' failure when the
// body does NOT throw.
func TestTestCovAssertThrowsNegative(t *testing.T) {
	r := testRegistry(t)
	runTestBoru(t, r, `Test.test "no throw" [[1 1 add] Assert.throws]`)
	if n := tcovFailCount(t, r); n != 1 {
		t.Errorf("fail-count = %d, want 1", n)
	}
}

// TestTestCovAssertMatch pins both arms of Assert.match.
func TestTestCovAssertMatch(t *testing.T) {
	r := testRegistry(t)
	runTestBoru(t, r, `
		Test.test "contains" [Assert.match 'ell' 'hello']
		Test.test "missing" [Assert.match 'zz' 'hello']
	`)
	if n := tcovFailCount(t, r); n != 1 {
		t.Errorf("fail-count = %d, want 1", n)
	}
}

// TestTestCovReset pins Test.reset: results, failures and path all clear.
func TestTestCovReset(t *testing.T) {
	r := testRegistry(t)
	runTestBoru(t, r, `Test.test "bad" [1 2 Assert.equal]`)
	if n := tcovFailCount(t, r); n != 1 {
		t.Fatalf("precondition: fail-count = %d, want 1", n)
	}
	runTestBoru(t, r, `Test.reset`)
	out := runTestBoru(t, r, `Test.summary`)
	m, _ := native.AsMap(out[0])
	totalV, _ := m.Get("total")
	total, _ := native.AsInteger(totalV)
	if total != 0 {
		t.Errorf("after reset, total = %d, want 0", total)
	}
	if n := tcovFailCount(t, r); n != 0 {
		t.Errorf("after reset, fail-count = %d, want 0", n)
	}
}

// TestTestCovFailLineOnStderr pins runCase's loud per-case FAIL line —
// named with the describe path — on the registry's ErrOutput.
func TestTestCovFailLineOnStderr(t *testing.T) {
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	r.ErrOutput = &buf
	r.SetParseFunc(parser.Parse)
	desc, err := BuildTestModule(r)
	if err != nil {
		t.Fatalf("BuildTestModule: %v", err)
	}
	for name, exp := range desc.Exports {
		r.Defs.Push(name, native.NewMap(exp))
	}
	runTestBoru(t, r, `Test.describe "outer" [Test.test "boom" [1 2 Assert.equal]]`)
	if !strings.Contains(buf.String(), "FAIL outer / boom") {
		t.Errorf("stderr should carry the pathed FAIL line, got %q", buf.String())
	}
}

// TestTestCovDescribeBodyError pins that a non-assertion error inside a
// describe body aborts the describe loudly (the runErr branch).
func TestTestCovDescribeBodyError(t *testing.T) {
	r := testRegistry(t)
	vals, err := parser.Parse(`Test.describe "g" [no-such-word-xyz]`)
	if err != nil {
		t.Fatal(err)
	}
	if _, rerr := native.NewTop(r).Run(vals); rerr == nil {
		t.Error("a describe body error should propagate")
	}
}

// TestTestCovInvokeDotted covers Test.invoke with a dotted subject name
// (dottedWordTokens' multi-segment arm) via a module export.
func TestTestCovInvokeDotted(t *testing.T) {
	r := testRegistry(t)
	InstallResolver(r)
	runTestBoru(t, r, `import "boru:math-util"`)
	out := runTestBoru(t, r, `Test.invoke "MathUtil.sqrt" [4.0]`)
	f, _ := native.AsNumber(out[0])
	if f != 2.0 {
		t.Errorf("Test.invoke MathUtil.sqrt [4.0] = %v, want 2.0", out[0])
	}
}

// TestTestCovInvokeAtomAndOutcomes covers the Atom overload plus the
// error-value and empty-stack outcomes of invokeSubject.
func TestTestCovInvokeAtomAndOutcomes(t *testing.T) {
	r := testRegistry(t)
	// Atom subject, plain word.
	out := runTestBoru(t, r, `Test.invoke add/q [1 2]`)
	n, _ := native.AsInteger(out[0])
	if n != 3 {
		t.Errorf("Test.invoke add/q [1 2] = %v, want 3", out[0])
	}
	// A raising subject becomes an Error VALUE, not a run error.
	out2 := runTestBoru(t, r, `Test.invoke "no-such-word-zz" [1]`)
	if !native.IsError(out2[0]) {
		t.Errorf("invoking an unknown word should return an Error value, got %v", out2[0])
	}
	// A subject that leaves nothing returns None.
	out3 := runTestBoru(t, r, `def noop fn [[] [] []]  Test.invoke noop/q []`)
	if !native.IsNone(out3[len(out3)-1]) {
		t.Errorf("empty-stack invoke should return None, got %v", out3[len(out3)-1])
	}
}

// TestTestCovInvokeNonConcreteList pins invokeSubject's compile failure of a
// non-concrete inputs list (white-box: the type-literal carrier).
func TestTestCovInvokeNonConcreteList(t *testing.T) {
	r := testRegistry(t)
	if _, err := invokeSubject(r, "add", native.NewTypeLiteral(native.TList)); err == nil {
		t.Error("a type-literal inputs list must be declined")
	}
}

// TestTestCovCheckPropErrorArms drives runCheckProp's failure arms: a
// raising generator, a generator that produces nothing, a property that
// produces nothing, and a raising property.
func TestTestCovCheckPropErrorArms(t *testing.T) {
	cases := []struct {
		name    string
		src     string
		errWant string
	}{
		{"gen raises", `Test.check-prop "g1" [raise 'gboom'] [true] 3 1 0`, "gboom"},
		{"gen empty", `Test.check-prop "g2" [] [true] 3 1 0`, "generator produced no value"},
		{"property empty", `Test.check-prop "g3" [42] [drop] 3 1 0`, "property produced no value"},
		{"property raises", `Test.check-prop "g4" [42] [raise 'pboom'] 3 1 5`, "pboom"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := testRegistry(t)
			res := runTestBoru(t, r, c.src)
			m, _ := native.AsMap(res[0])
			okV, _ := m.Get("ok")
			ok, _ := okV.AsConcreteBoolean()
			if ok {
				t.Fatalf("%s should fail the property", c.name)
			}
			errV, _ := m.Get("error")
			if native.IsNone(errV) {
				t.Fatalf("%s should record an error", c.name)
			}
			if !strings.Contains(errV.String(), c.errWant) {
				t.Errorf("error %q should contain %q", errV.String(), c.errWant)
			}
		})
	}
}

// TestTestCovCheckPropUnrecordableGen covers the gen-program shrink
// fallback: a gen body the stackform recorder cannot compile (control
// flow) falls back to value-level shrinking, which still minimises.
func TestTestCovCheckPropUnrecordableGen(t *testing.T) {
	r := testRegistry(t)
	res := runTestBoru(t, r, `Test.check-prop "cf" [if true [15] [16]] [10 lt] 2 1 50`)
	m, _ := native.AsMap(res[0])
	okV, _ := m.Get("ok")
	if ok, _ := okV.AsConcreteBoolean(); ok {
		t.Fatal("15 < 10 is false — the property must fail")
	}
	failV, _ := m.Get("failing-input")
	shrunkV, _ := m.Get("shrunk-input")
	failing, _ := failV.AsConcreteInteger()
	shrunk, _ := shrunkV.AsConcreteInteger()
	if failing != 15 {
		t.Errorf("failing-input = %d, want 15", failing)
	}
	if shrunk > failing {
		t.Errorf("shrunk-input %d must be <= failing-input %d", shrunk, failing)
	}
}

// TestTestCovShapeReturnsDirect covers the two check-mode shape hooks —
// both return exactly one record-shaped carrier.
func TestTestCovShapeReturnsDirect(t *testing.T) {
	if out := testPropResultShapeReturns(nil, nil); len(out) != 1 {
		t.Errorf("testPropResultShapeReturns returned %d values, want 1", len(out))
	}
	if out := testSummaryShapeReturns(nil, nil); len(out) != 1 {
		t.Errorf("testSummaryShapeReturns returned %d values, want 1", len(out))
	}
}

// TestTestCovResolveTestExportDirect walks resolveTestExport's arms:
// fnDef values (registry nil vs set, Function vs FnDef parent), word /
// string / atom names (bound and unbound), and pass-through values.
func TestTestCovResolveTestExportDirect(t *testing.T) {
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}

	// A plain value passes through untouched.
	if v := resolveTestExport(r, native.NewInteger(5)); func() bool {
		n, _ := native.AsInteger(v)
		return n != 5
	}() {
		t.Errorf("plain value should pass through, got %v", v)
	}

	// A Function value without a registry gains one.
	fn := native.NewFunction(native.FnDefInfo{Name: "f"})
	got := resolveTestExport(r, fn)
	if info, ok := got.Data.(native.FnDefInfo); !ok || info.Registry != r {
		t.Errorf("Function without registry should be stamped with modReg")
	}

	// An FnDef-parented value keeps its parent.
	fd := native.NewFunction(native.FnDefInfo{Name: "g"})
	got2 := resolveTestExport(r, fd)
	if !got2.Parent.Equal(native.TFunction) {
		t.Errorf("FnDef parent should be preserved, got %s", got2.Parent.String())
	}

	// A Function that already carries a registry is returned as-is.
	fn3 := native.NewFunction(native.FnDefInfo{Name: "h", Registry: r})
	got3 := resolveTestExport(r, fn3)
	if info, ok := got3.Data.(native.FnDefInfo); !ok || info.Registry != r {
		t.Errorf("already-stamped Function should keep its registry")
	}

	// A bound word name resolves through the def stack.
	r.Defs.Push("bound-x", native.NewInteger(9))
	res := resolveTestExport(r, native.NewWord("bound-x"))
	if n, _ := native.AsInteger(res); n != 9 {
		t.Errorf("bound word should resolve to its def, got %v", res)
	}

	// A bound def holding a registry-less Function gets stamped too.
	r.Defs.Push("bound-fn", native.NewFunction(native.FnDefInfo{Name: "bf"}))
	res2 := resolveTestExport(r, native.NewWord("bound-fn"))
	if info, ok := res2.Data.(native.FnDefInfo); !ok || info.Registry != r {
		t.Errorf("bound Function should be stamped with modReg")
	}

	// Unbound word / string / atom names fall through unchanged.
	for _, v := range []native.Value{
		native.NewWord("nope-w"),
		native.NewString("nope-s"),
		native.NewAtom("nope-a"),
	} {
		out := resolveTestExport(r, v)
		if out.String() != v.String() {
			t.Errorf("unbound name %s should pass through, got %s", v.String(), out.String())
		}
	}
}

// TestTestCovBuildModuleNoParser pins BuildTestModule's compile failure when the
// parent registry carries no parser.
func TestTestCovBuildModuleNoParser(t *testing.T) {
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := BuildTestModule(r); err == nil ||
		!strings.Contains(err.Error(), "parser not configured") {
		t.Errorf("BuildTestModule without a parser should decline, got %v", err)
	}
}

// TestTestCovResolveExportNonConcrete pins resolveExport's compile failure of a
// non-concrete export map.
func TestTestCovResolveExportNonConcrete(t *testing.T) {
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	exports := map[string]*native.OrderedMap{}
	if _, err := resolveExport(r, exports, "X", native.NewTypeLiteral(native.TMap)); err == nil {
		t.Error("a type-literal export map must be declined")
	}
}

// TestTestCovResolveTestExportTypeBinding covers the type-binding arm of
// resolveTestExport: a capitalised def resolves through TopTypeBody.
func TestTestCovResolveTestExportTypeBinding(t *testing.T) {
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.SetParseFunc(parser.Parse)
	vals, err := parser.Parse(`def CovT Integer`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := native.NewTop(r).Run(vals); err != nil {
		t.Fatal(err)
	}
	res := resolveTestExport(r, native.NewWord("CovT"))
	if !native.IsBareTypeNode(res) {
		t.Errorf("a type binding should resolve to its type body, got %s", res.String())
	}
}

// TestTestCovFirstErrLineDirect pins the one-line error extraction.
func TestTestCovFirstErrLineDirect(t *testing.T) {
	if got := firstErrLine(errors.New("top\nrest")); got != "top" {
		t.Errorf("firstErrLine = %q, want top", got)
	}
	if got := firstErrLine(errors.New("solo")); got != "solo" {
		t.Errorf("firstErrLine = %q, want solo", got)
	}
}

// TestTestCovResultStringFieldDirect pins resultStringField's arms:
// absent key, None value, a String, and a non-String rendering.
func TestTestCovResultStringFieldDirect(t *testing.T) {
	m := native.NewOrderedMap()
	m.Set("none", native.NewNone())
	m.Set("str", native.NewString("hello"))
	m.Set("num", native.NewInteger(7))
	if got := resultStringField(m, "absent"); got != "" {
		t.Errorf("absent key = %q, want empty", got)
	}
	if got := resultStringField(m, "none"); got != "" {
		t.Errorf("None value = %q, want empty", got)
	}
	if got := resultStringField(m, "str"); got != "hello" {
		t.Errorf("str = %q, want hello", got)
	}
	if got := resultStringField(m, "num"); got != "7" {
		t.Errorf("num = %q, want 7 (String render)", got)
	}
}

// TestTestCovIsTruthyDirect pins the truthiness table.
func TestTestCovIsTruthyDirect(t *testing.T) {
	cases := []struct {
		v    native.Value
		want bool
	}{
		{native.NewBoolean(true), true},
		{native.NewBoolean(false), false},
		{native.NewNone(), false},
		{native.NewTypeLiteral(native.TInteger), false},
		{native.NewInteger(0), true},
		{native.NewString(""), true},
	}
	for _, c := range cases {
		if got := isTruthy(c.v); got != c.want {
			t.Errorf("isTruthy(%s) = %v, want %v", c.v.String(), got, c.want)
		}
	}
}

// TestTestCovDottedWordTokensDirect pins the token expansion for plain
// and dotted subject names.
func TestTestCovDottedWordTokensDirect(t *testing.T) {
	plain := dottedWordTokens("sqrt")
	if len(plain) != 1 || !native.IsWord(plain[0]) {
		t.Fatalf("plain name should expand to one Word, got %v", plain)
	}
	dotted := dottedWordTokens("A.b.c")
	if len(dotted) != 5 {
		t.Fatalf("A.b.c should expand to 5 tokens, got %v", dotted)
	}
	w, _ := native.AsWord(dotted[1])
	if w.Name != "dot" {
		t.Errorf("second token should be the dot word, got %v", dotted[1])
	}
}
