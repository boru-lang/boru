package native

import (
	"strings"
	"testing"

	parser "github.com/boru-lang/boru/parser/go"
)

// Wave-3 coverage for native_array.go: the rare overloads (group/1,
// insert-at, remove-at, expand, the Reach lens forms of each/sortby, the
// 2D inner product), the pinned error arms of the array words, and the
// check-mode ReturnsFn family (iota length refinement, window/pairs
// nesting, the fold/scan accumulator fixed point). Positive and negative
// rows are paired throughout per lang/go/CLAUDE.md test discipline.

// w3Run parses src and runs it on r, returning the canonical stack render
// and any run error. Parse errors fail the test immediately: every source
// string in this file is expected to parse.
func w3Run(t *testing.T, r *Registry, src string) (string, error) {
	t.Helper()
	toks, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	out, runErr := NewTop(r).Run(toks)
	return Canon(out), runErr
}

// w3Array runs src against a registry with the boru:array-util words
// registered bare (arrayTestReg, shared with array_test.go).
func w3Array(t *testing.T, src string) (string, error) {
	t.Helper()
	return w3Run(t, arrayTestReg(), src)
}

// w3ArrayWant asserts the canonical result of src.
func w3ArrayWant(t *testing.T, src, want string) {
	t.Helper()
	got, err := w3Array(t, src)
	if err != nil {
		t.Fatalf("%q: unexpected error: %v", src, err)
	}
	if got != want {
		t.Errorf("%q = %q, want %q", src, got, want)
	}
}

// w3ArrayErr asserts src raises an error containing substr.
func w3ArrayErr(t *testing.T, src, substr string) {
	t.Helper()
	_, err := w3Array(t, src)
	if err == nil {
		t.Fatalf("%q: expected error containing %q, got none", src, substr)
	}
	if !strings.Contains(err.Error(), substr) {
		t.Errorf("%q: error %q does not contain %q", src, err.Error(), substr)
	}
}

// w3ArrayCheck runs src in check mode on an array registry and returns
// the residual carrier stack (as canonical type paths) plus diagnostics.
func w3ArrayCheck(t *testing.T, src string) ([]Value, *Registry) {
	t.Helper()
	r := arrayTestReg()
	r.Check.Mode = true
	toks, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	out, runErr := NewTop(r).Run(toks)
	if runErr != nil {
		t.Fatalf("check run %q: %v", src, runErr)
	}
	return out, r
}

// w3HandlerErr calls a handler directly with the given args and asserts it
// raises an error containing substr — the type-literal guard arms are not
// reachable by dispatch (matchSignature gates a bare `List` away from a
// TList slot), so they are pinned by direct calls.
func w3HandlerErr(t *testing.T, name string, h func([]Value, map[string]Value, []Value, *Registry) ([]Value, error), substr string, args ...Value) {
	t.Helper()
	r := arrayTestReg()
	_, err := h(args, nil, nil, r)
	if err == nil {
		t.Fatalf("%s: expected error containing %q, got none", name, substr)
	}
	if !strings.Contains(err.Error(), substr) {
		t.Errorf("%s: error %q does not contain %q", name, err.Error(), substr)
	}
}

// ---- group (one-arg overload) ----

func TestW3GroupOneArg(t *testing.T) {
	// The map key is the string's CONTENT, not its render (NUR030), so
	// the entries are `a:`/`b:`/`c:` rather than `'a':`/`'b':`/`'c':`.
	w3ArrayWant(t, `group ['a' 'b' 'a' 'c' 'b']`, `{a:[0 2] b:[1 4] c:[3]}`)
	w3ArrayWant(t, `group []`, `{}`)
	w3HandlerErr(t, "group", groupOneHandler, "group: expected",
		NewTypeLiteral(TList))
}

// ---- insert-at / remove-at ----

func TestW3InsertAt(t *testing.T) {
	w3ArrayWant(t, `insert-at 1 99 [1 2 3]`, `[1 99 2 3]`)
	w3ArrayWant(t, `insert-at 0 99 []`, `[99]`)
	w3ArrayWant(t, `insert-at 3 99 [1 2 3]`, `[1 2 3 99]`) // index == len appends
	w3ArrayErr(t, `insert-at 4 99 [1 2 3]`, "out of range")
	w3HandlerErr(t, "insert-at", insertAtHandler, "insert-at: expected a concrete data list",
		NewInteger(1), NewInteger(99), NewTypeLiteral(TList))
	w3HandlerErr(t, "insert-at", insertAtHandler, "concrete Integer index",
		NewTypeLiteral(TInteger), NewInteger(99), NewList([]Value{NewInteger(1)}))
}

func TestW3RemoveAt(t *testing.T) {
	w3ArrayWant(t, `remove-at 1 [1 2 3]`, `[1 3]`)
	w3ArrayWant(t, `remove-at 0 [1]`, `[]`)
	w3ArrayErr(t, `remove-at 0 []`, "empty list")
	w3ArrayErr(t, `remove-at 3 [1 2 3]`, "out of range")
	w3HandlerErr(t, "remove-at", removeAtHandler, "remove-at: expected a concrete data list",
		NewInteger(0), NewTypeLiteral(TList))
	w3HandlerErr(t, "remove-at", removeAtHandler, "concrete Integer index",
		NewTypeLiteral(TInteger), NewList([]Value{NewInteger(1)}))
}

// ---- expand ----

func TestW3Expand(t *testing.T) {
	w3ArrayWant(t, `expand [true false true] [1 2]`, `[1 0 2]`)
	w3ArrayWant(t, `expand [] []`, `[]`)
	w3ArrayErr(t, `expand [true true] [1]`, "not enough data")
	w3HandlerErr(t, "expand", expandHandler, "expand: expected",
		NewTypeLiteral(TList), NewList([]Value{NewInteger(1)}))
	w3HandlerErr(t, "expand", expandHandler, "expand: expected",
		NewList([]Value{NewBoolean(true)}), NewTypeLiteral(TList))
}

// ---- Reach lens forms of each / sortby ----

func TestW3EachReach(t *testing.T) {
	w3ArrayWant(t, `each $.name [{name:'ada'} {name:'bob'}]`, `['ada' 'bob']`)
	// A lens over a type-literal data list is a loud error (direct call:
	// dispatch gates the bare List literal away from the TList slot).
	r := arrayTestReg()
	toks, err := parser.Parse(`$.name`)
	if err != nil {
		t.Fatal(err)
	}
	out, err := NewTop(r).Run(toks)
	if err != nil || len(out) != 1 {
		t.Fatalf("building reach: %v %v", out, err)
	}
	reach := out[0]
	if _, herr := eachReachHandler([]Value{reach, NewTypeLiteral(TList)}, nil, nil, r); herr == nil ||
		!strings.Contains(herr.Error(), "each: expected a concrete data list") {
		t.Errorf("each reach over List: got %v", herr)
	}
	if _, herr := sortbyReachHandler([]Value{reach, NewTypeLiteral(TList)}, nil, nil, r); herr == nil ||
		!strings.Contains(herr.Error(), "sortby: expected a concrete data list") {
		t.Errorf("sortby reach over List: got %v", herr)
	}
	// A non-Reach first arg is rejected by both lens handlers.
	if _, herr := eachReachHandler([]Value{NewInteger(1), NewList(nil)}, nil, nil, r); herr == nil {
		t.Error("each reach with non-Reach arg should error")
	}
	if _, herr := sortbyReachHandler([]Value{NewInteger(1), NewList(nil)}, nil, nil, r); herr == nil {
		t.Error("sortby reach with non-Reach arg should error")
	}
}

func TestW3SortbyReach(t *testing.T) {
	w3ArrayWant(t, `each $.age (sortby $.age [{age:9} {age:1} {age:5}])`, `[1 5 9]`)
}

func TestW3SortbyErrors(t *testing.T) {
	w3ArrayWant(t, `sortby [2 1] ['b' 'a']`, `['a' 'b']`)
	w3ArrayErr(t, `sortby [1] ['a' 'b']`, "does not match data length")
	w3HandlerErr(t, "sortby", sortbyHandler, "sortby: expected",
		NewTypeLiteral(TList), NewList([]Value{NewInteger(1)}))
}

// ---- inner: 1D and 2D forms ----

func TestW3Inner1D(t *testing.T) {
	w3ArrayWant(t, `inner [mul] [add] [1 2 3] [4 5 6]`, `32`)
	w3ArrayErr(t, `inner [mul] [add] [1 2] [1 2 3]`, "vectors must have same length")
	// A body that nets no value is a loud inner_error.
	w3ArrayErr(t, `inner [drop drop] [add] [1 2] [3 4]`, "no result")
	w3ArrayErr(t, `inner [mul] [drop drop] [1 2] [3 4]`, "no result")
}

func TestW3Inner2D(t *testing.T) {
	w3ArrayWant(t, `inner [mul] [add] [[1 2] [3 4]] [[5 6] [7 8]]`,
		`[[19 22] [43 50]]`)
	w3ArrayErr(t, `inner [mul] [add] [[1 2 3]] [[5 6] [7 8]]`, "dimension mismatch")
}

// ---- pinned type-literal error arms (requireListArg, direct calls) ----

func TestW3ArrayTypeLiteralErrArms(t *testing.T) {
	lit := NewTypeLiteral(TList)
	one := NewList([]Value{NewInteger(1)})
	tru := NewList([]Value{NewBoolean(true)})
	type h = func([]Value, map[string]Value, []Value, *Registry) ([]Value, error)
	cases := []struct {
		name    string
		handler h
		substr  string
		args    []Value
	}{
		{"shape", shapeHandler, "shape: expected", []Value{lit}},
		{"rank", rankHandler, "rank: expected", []Value{lit}},
		{"transpose", arrTransposeHandler, "transpose: expected", []Value{lit}},
		{"reverse", reverseHandler, "reverse: expected", []Value{lit}},
		{"take", takeHandler, "take: expected", []Value{NewInteger(1), lit}},
		{"shed", shedHandler, "shed: expected", []Value{NewInteger(1), lit}},
		{"where", whereHandler, "where: expected", []Value{lit}},
		{"unique", uniqueHandler, "unique: expected", []Value{lit}},
		{"at-0", atHandler, "at: expected", []Value{lit, one}},
		{"at-1", atHandler, "at: expected", []Value{one, lit}},
		{"member-0", memberHandler, "member: expected", []Value{lit, one}},
		{"member-1", memberHandler, "member: expected", []Value{one, lit}},
		{"indices-0", indicesHandler, "indices: expected", []Value{lit, one}},
		{"indices-1", indicesHandler, "indices: expected", []Value{one, lit}},
		{"replicate-0", replicateHandler, "replicate: expected", []Value{lit, one}},
		{"replicate-1", replicateHandler, "replicate: expected", []Value{one, lit}},
		{"window", windowHandler, "window: expected", []Value{NewInteger(1), lit}},
		{"pairs", pairsHandler, "pairs: expected", []Value{lit}},
		{"compress-0", compressHandler, "compress: expected", []Value{lit, one}},
		{"compress-1", compressHandler, "compress: expected", []Value{tru, lit}},
		{"reshape-0", reshapeHandler, "reshape: expected", []Value{lit, one}},
		{"reshape-1", reshapeHandler, "reshape: expected", []Value{one, lit}},
		{"grade", gradeHandler, "grade: expected", []Value{lit}},
		{"group-2", groupTwoHandler, "group: expected", []Value{lit, one}},
		{"inner", innerHandler, "inner: expected", []Value{lit, one, one, one}},
		{"eachrank", eachrankHandler, "eachrank: expected",
			[]Value{NewInteger(0), lit, one}},
		{"foldaxis", foldaxisHandler, "foldaxis: expected",
			[]Value{NewInteger(0), lit, one}},
		{"each", eachHandler, "each: expected", []Value{lit, one}},
		{"for-each", forEachHandler, "for-each: expected", []Value{lit, one}},
		{"fold-init", foldWithInitHandler, "fold: expected",
			[]Value{lit, one, NewInteger(0)}},
		{"fold-noinit", foldNoInitHandler, "fold: expected", []Value{lit, one}},
		{"scan", scanHandler, "scan: expected", []Value{lit, one}},
		{"outer", outerHandler, "outer: expected", []Value{lit, one, one}},
	}
	for _, c := range cases {
		w3HandlerErr(t, c.name, c.handler, c.substr, c.args...)
	}
}

// ---- reshape edges ----

func TestW3ReshapeEdges(t *testing.T) {
	w3ArrayWant(t, `reshape [2 2] [1 2 3 4]`, `[[1 2] [3 4]]`)
	w3ArrayErr(t, `reshape [-1] [1]`, "negative dimension")
	w3ArrayErr(t, `reshape [99999999] [1]`, "exceeds the cap")
	w3ArrayErr(t, `reshape [2 3] [1 2 3 4]`, "does not match data length")
	// Overflow-guard: a product that would exceed the cap errors before
	// allocation.
	w3ArrayErr(t, `reshape [10000000 10000000] [1]`, "exceeds the cap")
}

// ---- transpose / window / pairs edges ----

func TestW3TransposeEdges(t *testing.T) {
	w3ArrayWant(t, `transpose []`, `[]`)
	w3ArrayErr(t, `transpose [1 2]`, "rank-2")
	w3ArrayErr(t, `transpose [[1 2] [3]]`, "rectangular")
}

func TestW3WindowEdges(t *testing.T) {
	w3ArrayWant(t, `window 2 [1 2 3]`, `[[1 2] [2 3]]`)
	w3ArrayWant(t, `window 5 [1 2]`, `[]`)
	w3ArrayErr(t, `window 0 [1 2]`, "size must be positive")
}

func TestW3PairsEdges(t *testing.T) {
	w3ArrayWant(t, `pairs [1 2 3]`, `[[1 2] [2 3]]`)
	w3ArrayWant(t, `pairs [1]`, `[]`)
}

// ---- iota / at bounds ----

func TestW3IotaBounds(t *testing.T) {
	w3ArrayErr(t, `iota -1`, "negative count")
	w3ArrayErr(t, `iota 99999999`, "exceeds the cap")
}

func TestW3AtBounds(t *testing.T) {
	w3ArrayWant(t, `at [1 0] [10 20]`, `[20 10]`)
	w3ArrayErr(t, `at [5] [10 20]`, "out of bounds")
}

// ---- replicate negative count ----

func TestW3ReplicateNegative(t *testing.T) {
	w3ArrayWant(t, `replicate [0 2] [1 2]`, `[2 2]`)
	w3ArrayErr(t, `replicate [-1] [1]`, "negative count")
}

// ---- higher-order error propagation ----

func TestW3HigherOrderBodyErrors(t *testing.T) {
	w3ArrayErr(t, `each [drop] [1]`, "body produced no result")
	w3ArrayErr(t, `each [no-such-word-w3] [1]`, "element 0")
	w3ArrayErr(t, `fold [no-such-word-w3] [1 2] 0`, "fold: step")
	w3ArrayErr(t, `fold [drop drop] [1 2] 0`, "body produced no result")
	w3ArrayErr(t, `fold [add] []`, "empty list with no initial value")
	w3ArrayErr(t, `scan [drop drop] [1 2]`, "body produced no result")
	w3ArrayErr(t, `scan [no-such-word-w3] [1 2]`, "scan: step")
	w3ArrayErr(t, `outer [drop drop] [1] [2]`, "body produced no result")
	w3ArrayErr(t, `outer [no-such-word-w3] [1] [2]`, "outer: (0,0)")
	w3ArrayErr(t, `for-each [no-such-word-w3] [1]`, "for-each: element 0")
}

func TestW3ScanEmptyAndForEach(t *testing.T) {
	w3ArrayWant(t, `scan [add] []`, `[]`)
	w3ArrayWant(t, `scan [add] [1 2 3]`, `[1 3 6]`)
	// for-each produces nothing; a body that leaves the stack empty is fine.
	w3ArrayWant(t, `for-each [drop] [1 2 3]`, ``)
}

// ---- eachrank / foldaxis edges ----

func TestW3EachrankEdges(t *testing.T) {
	w3ArrayWant(t, `eachrank 0 [mul 2] [[1 2] [3 4]]`, `[[2 4] [6 8]]`)
	w3ArrayWant(t, `eachrank 1 [reverse] [[1 2] [3 4]]`, `[[2 1] [4 3]]`)
	w3ArrayErr(t, `eachrank -1 [mul 2] [1 2]`, "negative rank")
	w3ArrayErr(t, `eachrank 5 [mul 2] [1 2]`, "exceeds data rank")
	w3ArrayErr(t, `eachrank 0 [drop] [1 2]`, "body produced no result")
}

func TestW3FoldaxisEdges(t *testing.T) {
	w3ArrayWant(t, `foldaxis 0 [add] [[1 2] [3 4]]`, `[4 6]`)
	w3ArrayWant(t, `foldaxis 1 [add] [[1 2] [3 4]]`, `[3 7]`)
	w3ArrayWant(t, `foldaxis 0 [add] []`, `[]`)
	w3ArrayErr(t, `foldaxis 0 [add] [1 2]`, "rank-2")
	w3ArrayErr(t, `foldaxis 0 [add] [[1 2] [3]]`, "rectangular")
	// One row, zero columns: axis 0 has no lane (empty result), axis 1 has
	// one EMPTY lane — nothing to seed the accumulator from, so the word
	// raises its own error where it used to panic on lane[0].
	w3ArrayWant(t, `foldaxis 0 [add] [[]]`, `[]`)
	w3ArrayErr(t, `foldaxis 1 [add] [[]]`, "empty")
}

// ---- compress ----

func TestW3CompressEdges(t *testing.T) {
	w3ArrayWant(t, `compress [true false true] [1 2 3]`, `[1 3]`)
	w3ArrayErr(t, `compress [true] [1 2]`, "does not match data length")
}

// ---- cross-collection delegation (collIsConcrete{Map,List}) ----

func TestW3EachCrossCollection(t *testing.T) {
	// eachHandler with a concrete MAP collection delegates to the map form.
	w3ArrayWant(t, `each [mul 2] {a:1 b:2}`, `{a:2 b:4}`)
	// eachMapHandler with a concrete LIST collection delegates back
	// (collIsConcreteList) — reachable via a direct handler call, as the
	// interpreter's matchSignature gates the shapes apart.
	r := arrayTestReg()
	drive := w9Driven(r)
	body := NewEvalList([]Value{NewWord("mul"), NewInteger(2)})
	out, err := drive(eachMapHandler([]Value{body, NewList([]Value{NewInteger(3)})}, nil, nil, r))
	if err != nil {
		t.Fatalf("eachMapHandler over list: %v", err)
	}
	if len(out) != 1 || Canon(out) != `[6]` {
		t.Errorf("eachMapHandler over list = %q, want [6]", Canon(out))
	}
	// And the fold/scan cross-collection redirects.
	out, err = drive(foldWithInitHandler([]Value{
		NewEvalList([]Value{NewWord("add")}),
		mapOf(t, "a", NewInteger(1), "b", NewInteger(2)),
		NewInteger(0),
	}, nil, nil, r))
	if err != nil {
		t.Fatalf("fold over map: %v", err)
	}
	if Canon(out) != `3` {
		t.Errorf("fold over map = %q, want 3", Canon(out))
	}
}

// mapOf builds a concrete ordered map value for direct handler calls.
func mapOf(t *testing.T, pairs ...any) Value {
	t.Helper()
	om := NewOrderedMap()
	for i := 0; i+1 < len(pairs); i += 2 {
		om.Set(pairs[i].(string), pairs[i+1].(Value))
	}
	return NewMap(om)
}

// ---- check-mode ReturnsFn coverage ----

func TestW3CheckIotaLen(t *testing.T) {
	out, _ := w3ArrayCheck(t, `iota 3`)
	if len(out) != 1 || !out[0].Parent.ConformsTo(TList) {
		t.Fatalf("check iota 3: got %v", out)
	}
	// A statically-unknown count falls back to the unrefined carrier.
	out, _ = w3ArrayCheck(t, `iota (1 add 2)`)
	if len(out) != 1 || !out[0].Parent.ConformsTo(TList) {
		t.Fatalf("check iota (1 add 2): got %v", out)
	}
}

func TestW3CheckArrayReturns(t *testing.T) {
	srcs := []string{
		`range 1 4`,                          // returnsCarrierTypedListInteger
		`member [1] [1 2]`,                   // returnsCarrierTypedListBoolean
		`at [0] [10 20]`,                     // returnsAtChecked
		`window 2 [1 2 3]`,                   // windowReturnsFn
		`pairs [1 2 3]`,                      // pairsReturnsFn
		`each [mul 2] [1 2 3]`,               // eachReturnsFn
		`each [drop] [1 2]`,                  // eachReturnsFn empty-residual arm
		`fold [add] [1 2 3] 0`,               // foldWithInitReturnsFn
		`fold [add 0.5] [1 2] 0`,             // accumulator widening fixed point
		`fold [add] [1 2 3]`,                 // foldNoInitReturnsFn
		`fold [push] [1 2] []`,               // list-seed accumulator carrier
		`fold [drop] [1 2] {}`,               // map-seed accumulator carrier
		`scan [add] [1 2 3]`,                 // scanReturnsFn
		`scan [add] []`,                      // static-empty scan skip
		`foldaxis 1 [add] [[1 2] [3 4]]`,     // foldaxisReturnsFn (analysing, NUR115)
		`foldaxis 1 [add 0.5] [[1 2] [3 4]]`, // foldaxis accumulator widening fixed point
		`foldaxis 0 [add] []`,                // foldaxis empty rank-2: gradual-Any element arm
		`foldaxis 1 [var [[a b] (if (b gt 1) ["s"] [b])]] [[1 2] [3 4]]`, // foldaxis disjunct accumulator arm
		`outer [mul] [1 2] [3 4]`,       // outerReturnsFn
		`inner [mul] [add] [1 2] [3 4]`, // innerReturnsFn
	}
	for _, src := range srcs {
		out, _ := w3ArrayCheck(t, src)
		if len(out) == 0 {
			t.Errorf("check %q: expected a residual carrier, got none", src)
		}
	}
	// for-each produces nothing in check mode too (forEachReturnsFn nil).
	out, _ := w3ArrayCheck(t, `for-each [drop] [1 2]`)
	if len(out) != 0 {
		t.Errorf("check for-each: expected empty residual, got %v", out)
	}
}

func TestW3CheckEmptyFoldDiagnostic(t *testing.T) {
	_, r := w3ArrayCheck(t, `fold [add] []`)
	found := false
	for _, d := range r.Check.Diagnostics {
		if d.Code == "fold_error" && strings.Contains(d.Detail, "empty list") {
			found = true
		}
	}
	if !found {
		t.Errorf("check `fold [add] []`: expected fold_error diagnostic, got %+v",
			r.Check.Diagnostics)
	}
}

// foldaxis mirrors its empty-lane error at check time as fold does for the
// statically-empty no-init list: `[[]]` reduced along axis 1 has one EMPTY
// lane and the handler's error is guaranteed, so the check pass records the
// byte-identical finding (a RuntimeMirror — the program still compiles). The
// negatives: axis 0 over the same data has no lane at all (the result is
// `[]`, nothing to flag), and a non-literal axis is not exactly known.
func TestW3CheckFoldaxisEmptyLaneDiagnostic(t *testing.T) {
	flagged := func(src string) bool {
		_, r := w3ArrayCheck(t, src)
		for _, d := range r.Check.Diagnostics {
			if d.Code == "foldaxis_error" && strings.Contains(d.Detail, "lane 0 is empty") {
				return true
			}
		}
		return false
	}
	if !flagged(`foldaxis 1 [add] [[]]`) {
		t.Error("check `foldaxis 1 [add] [[]]`: expected the empty-lane foldaxis_error mirror")
	}
	if flagged(`foldaxis 0 [add] [[]]`) {
		t.Error("check `foldaxis 0 [add] [[]]`: axis 0 has no lane over zero columns — nothing to flag")
	}
	if flagged(`foldaxis (0 add 1) [add] [[]]`) {
		t.Error("check with a computed axis: the shape is not exactly known — nothing to flag")
	}
}

func TestW3CheckHigherOrderBodyDiagnostic(t *testing.T) {
	// A body that errors during analysis lands a body_error diagnostic
	// (analyseHigherOrderBodyVals' error arm).
	r := arrayTestReg()
	r.Check.Mode = true
	toks, err := parser.Parse(`each [1 raise 'boom'] [1 2]`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if _, runErr := NewTop(r).Run(toks); runErr != nil {
		t.Fatalf("check run: %v", runErr)
	}
}
