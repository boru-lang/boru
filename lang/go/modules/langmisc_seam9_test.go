package modules

import (
	"errors"
	"testing"

	core "github.com/boru-lang/boru/core/go"
	"github.com/boru-lang/boru/lang/go/native"
)

// w9TestHandler extracts a named test-module native handler (sig 0) for a
// white-box call that bypasses the compiler so the interpreter arms run.
func w9TestHandler(t *testing.T, parent *native.Registry, word string) native.Handler {
	t.Helper()
	for _, nf := range testNatives(parent) {
		if nf.Name == word {
			return nf.Signatures[0].DispatchHandler()
		}
	}
	t.Fatalf("test native %q not found", word)
	return nil
}

// --- minilang_query.go -----------------------------------------------------

// w9ListConv is a custom Ideal Behavior whose ToList is test-controlled, to
// drive idealListToAny's two error arms.
type w9ListConv struct {
	list native.Value
	err  error
}

func (w9ListConv) Match(v native.Value, t *native.Type) bool {
	return native.DefaultBehavior.Match(v, t)
}
func (w9ListConv) Format(v native.Value) string { return native.DefaultBehavior.Format(v) }
func (w9ListConv) Equal(a, b native.Value) bool { return native.DefaultBehavior.Equal(a, b) }
func (w9ListConv) ToMap(native.Value) (native.Value, error) {
	return native.NewMap(native.NewOrderedMap()), nil
}
func (c w9ListConv) ToList(native.Value) (native.Value, error) { return c.list, c.err }

// TestW9IdealListToAnyErrorArms drives idealListToAny's ConvertIdealToList
// error (minilang_query.go:75) and its AsList-error (79) fallbacks.
func TestW9IdealListToAnyErrorArms(t *testing.T) {
	r := mcovReg(t)

	// 75: a converter whose ToList returns a real error.
	tErr := r.Types.MintTypeWithBehavior("W9ListErr", native.TIdeal,
		w9ListConv{err: errors.New("boom")})
	if out := idealListToAny(core.NewExtension(tErr, "x")); out == nil {
		t.Error("idealListToAny(convert-error) should fall back to ValueToAny")
	}

	// 79: a converter whose ToList succeeds but returns a non-list value.
	tNL := r.Types.MintTypeWithBehavior("W9ListNL", native.TIdeal,
		w9ListConv{list: native.NewInteger(7)})
	if out := idealListToAny(core.NewExtension(tNL, "x")); out == nil {
		t.Error("idealListToAny(non-list result) should fall back to ValueToAny")
	}
}

// --- test.go interpreter-path body compile failures --------------------------------

// TestW9TestBodyInterpreterRefusals drives the interpreter-path
// RequireConcreteList rejections of Test.describe (251), Test.test (303) and
// Assert.throws (447) by invoking the handlers directly with a type-literal
// List (the compiler never runs, so the closure arm is bypassed).
func TestW9TestBodyInterpreterRefusals(t *testing.T) {
	parent := mcovReg(t)
	litL := native.NewTypeLiteral(native.TList)

	if _, err := w9TestHandler(t, parent, "test-describe")(
		[]native.Value{native.NewString("g"), litL}, nil, nil, parent); err == nil {
		t.Error("Test.describe interpreter path: non-concrete body should error")
	}
	if _, err := w9TestHandler(t, parent, "test-test")(
		[]native.Value{native.NewString("n"), litL}, nil, nil, parent); err == nil {
		t.Error("Test.test interpreter path: non-concrete body should error")
	}
	if _, err := w9TestHandler(t, parent, "assert-throws")(
		[]native.Value{litL}, nil, nil, parent); err == nil {
		t.Error("Assert.throws interpreter path: non-concrete body should error")
	}
}

// TestW9TestRecordPathAndSkip drives test-record's path-collection loop
// (537-540) and its check-mode side-effect suppression (526).
func TestW9TestRecordPathAndSkip(t *testing.T) {
	parent := mcovReg(t)
	h := w9TestHandler(t, parent, "test-record")
	pathList := native.NewList([]native.Value{native.NewString("group"), native.NewString("sub")})
	args := []native.Value{
		native.NewString("case"), pathList, native.NewBoolean(false),
		native.NewNone(), native.NewNone(), native.NewNone(), native.NewInteger(3),
	}

	// 537-540: normal (non-check) record with a two-element path list.
	if _, err := h(args, nil, nil, parent); err != nil {
		t.Fatalf("test-record: %v", err)
	}

	// 526-528: check mode suppresses the side-effecting write.
	restore := parent.Check.Begin()
	defer restore()
	if !parent.Check.SkipsSideEffect() {
		t.Fatal("expected check mode to skip side effects")
	}
	if _, err := h(args, nil, nil, parent); err != nil {
		t.Fatalf("test-record (check mode): %v", err)
	}
}

// TestW9ReportSkipsNonMapResult drives report()'s non-map result skip
// (test.go:1176): a value appended to the run.results that AsMap declines.
func TestW9ReportSkipsNonMapResult(t *testing.T) {
	parent := mcovReg(t)
	run := activeRun(parent)
	run.mu.Lock()
	run.results = append(run.results, native.NewInteger(0)) // not a map
	run.mu.Unlock()
	_ = run.report() // must not panic; the non-map entry is skipped
}
