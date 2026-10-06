package native

import (
	"strings"
	"testing"
)

// Wave-4 coverage for the wide tail: filter.go (all three forms),
// native_error_raise.go (raise + Error field access), the boru:struct
// words merge/items/jsonify/parse (struct_module.go + merge.go +
// jsonify.go + parse_text.go), native_fileinfo.go (__folder/__file),
// and the small check-mode ReturnsFns (size, clone, io_stream,
// transform.valueToMap).

// w4StructReg returns a registry with the boru:struct natives registered
// bare (merge, items, jsonify, getpath, …) — the same set
// modules.BuildStructModule wraps.
func w4StructReg(t *testing.T) *Registry {
	t.Helper()
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range StructModuleNatives {
		r.RegisterNativeFunc(n)
	}
	return r
}

// ---- filter: quotation / lens / Function forms ------------------------------

func TestW4FilterBodyForm(t *testing.T) {
	w3MiscWant(t, `filter [2 gt] [1 2 3 4]`, `[3 4]`)
	w3MiscWant(t, `filter [0 gt] []`, `[]`)
	w3MiscWant(t, `{a:1 b:5 c:3} filter [gt 2]`, `{b:5 c:3}`)
	// Loud errors: no result, non-Boolean result, non-collection input.
	w3MiscErr(t, `filter [drop] [1]`, "produced no result")
	w3MiscErr(t, `filter [1 add] [1]`, "must produce a Boolean")
	w3MiscErr(t, `filter [1 add] {a:1}`, "must produce a Boolean")
	w3MiscErr(t, `filter [drop] {a:1}`, "produced no result")
	w3MiscErr(t, `filter [2 gt] 5`, "expects a concrete list or map")
}

func TestW4FilterLensForm(t *testing.T) {
	w3MiscWant(t, `filter $.on [{n:"a" on:true} {n:"b" on:false} {n:"c" on:true}] size`, `2`)
	w3MiscWant(t, `filter $.ok {a:{ok:true} b:{ok:false}} keys`, `['a']`)
	// A lens that reads a non-Boolean keeps nothing.
	w3MiscWant(t, `filter $.n [{n:1} {n:2}]`, `[]`)
	w3MiscErr(t, `filter $.x 5`, "lens form expects a concrete list or map")
}

func TestW4FilterFunctionForm(t *testing.T) {
	w3MiscWant(t, `filter ([p:Integer] => [p gt 3]) [1 2 3 4 5]`, `[4 5]`)
	w3MiscWant(t, `filter ([p:Integer] => [p gt 0]) []`, `[]`)
	w3MiscWant(t, `filter ([v:Integer] => [v gt 2]) {a:1 b:5 c:3}`, `{b:5 c:3}`)
	w3MiscWant(t, `filter ([kv:KeyVal] => [kv.v gt 2]) {a:1 b:5 c:3}`, `{b:5 c:3}`)
	// A non-Boolean map predicate is a loud error; an arity-mismatched
	// callback has no matching signature.
	w3MiscErr(t, `{a:1 b:5} filter ([kv:KeyVal] => [kv.v])`, "must produce a Boolean")
	w3MiscErr(t, `{a:1} filter ([kv:KeyVal] => [kv.v drop])`, "produced no result")
	w3MiscErr(t, `filter ([a:Integer b:Integer] => [true]) [1]`, "no matching callback signature")
	w3MiscErr(t, `filter ([p:Any] => [w4-no-such-word]) [1]`, "element 0: ")
}

func TestW4FilterReturnsFn(t *testing.T) {
	r := w3TypeReg()
	fn := NewList(nil)
	if out := filterReturnsFn([]Value{fn}, r); len(out) != 1 {
		t.Fatalf("short args = %v", out)
	}
	out := filterReturnsFn([]Value{fn, NewList([]Value{NewInteger(1)})}, r)
	if len(out) != 1 || !out[0].Parent.ConformsTo(TList) {
		t.Errorf("list input should narrow to List, got %v", out)
	}
	m := NewOrderedMap()
	out = filterReturnsFn([]Value{fn, NewMap(m)}, r)
	if len(out) != 1 || !out[0].Parent.ConformsTo(TMap) {
		t.Errorf("map input should narrow to Map, got %v", out)
	}
	out = filterReturnsFn([]Value{fn, NewInteger(5)}, r)
	if len(out) != 1 {
		t.Errorf("scalar input = %v", out)
	}
}

// ---- raise + Error field access ----------------------------------------------

func TestW4RaiseForms(t *testing.T) {
	w3MiscErr(t, `raise "boom"`, "user_error")
	w3MiscErr(t, `raise bad_input "expected a list"`, "bad_input")
	w3MiscErr(t, `raise {code: nope/q, message: "m"}`, "nope")
	w3MiscErr(t, `raise {code: "strcode", message: "m"}`, "strcode")
	w3MiscErr(t, `raise {message: "no code"}`, "raise_error")
	w3MiscErr(t, `raise {code: 42, message: "m"}`, "code must be an atom or string")
	w3MiscErr(t, `raise {code: ok/q, message: 42}`, "message must be a string")
}

func TestW4ErrorFieldAccess(t *testing.T) {
	w3MiscWant(t, `def e (do [raise bad_input "nope"]) e dot code`, `bad_input/q`)
	w3MiscWant(t, `def e (do [raise bad_input "nope"]) e.message`, `'nope'`)
	w3MiscWant(t, `def e (do [raise {code: too_big/q, message: "m", got: 42}]) e dot got`, `42`)
	w3MiscWant(t, `def e (do [raise "x"]) e dot nothere`, `None`)
	// String-key get on an Error value.
	w3MiscWant(t, `def e (do [raise "x"]) e "message" get`, `'x'`)
	w3MiscWant(t, `def e (do [raise "x"]) e "code" get`, `user_error/q`)
}

func TestW4RaiseHandlerGuards(t *testing.T) {
	r := w3TypeReg()
	strLit := NewTypeLiteral(TString)
	atomLit := NewTypeLiteral(TAtom)
	if _, err := raiseMessageHandler([]Value{strLit}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "must be a concrete string") {
		t.Errorf("raiseMessageHandler guard: %v", err)
	}
	if _, err := raiseCodeMessageHandler([]Value{atomLit, NewString("m")}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "code must be an atom") {
		t.Errorf("raiseCodeMessageHandler code guard: %v", err)
	}
	if _, err := raiseCodeMessageHandler([]Value{NewAtom("c"), strLit}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "message must be a concrete string") {
		t.Errorf("raiseCodeMessageHandler message guard: %v", err)
	}
	if _, err := raiseSpecHandler([]Value{NewTypeLiteral(TMap)}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "concrete map") {
		t.Errorf("raiseSpecHandler guard: %v", err)
	}
}

func TestW4ErrorFieldReturns(t *testing.T) {
	// nil registry (or a compiling one) reproduces the declared-Any carrier.
	if out := errorFieldReturns([]Value{NewAtom("code")}, nil); len(out) != 1 {
		t.Fatalf("nil registry = %v", out)
	}
	r := w3TypeReg()
	if out := errorFieldReturns([]Value{NewAtom("code")}, r); len(out) != 1 {
		t.Errorf("code key = %v", out)
	}
	if out := errorFieldReturns([]Value{NewString("message")}, r); len(out) != 1 ||
		!out[0].Parent.ConformsTo(TString) {
		t.Errorf("message key should narrow to String, got %v", out)
	}
	if out := errorFieldReturns([]Value{NewAtom("payload")}, r); len(out) != 1 {
		t.Errorf("payload key = %v", out)
	}
	if out := errorFieldReturns([]Value{NewTypeLiteral(TAtom)}, r); len(out) != 1 {
		t.Errorf("non-concrete key = %v", out)
	}
	if out := errorFieldReturns(nil, r); len(out) != 1 {
		t.Errorf("no args = %v", out)
	}
}

// ---- struct words: merge / items / jsonify / parse -----------------------------

func TestW4StructMerge(t *testing.T) {
	r := w4StructReg(t)
	// Deep merge (later wins).
	w4LogWant(t, r, `merge {a:1 b:2} {b:3 c:4}`, `{a:1 b:3 c:4}`)
	w4LogWant(t, r, `merge {a:{x:1 y:2}} {a:{y:3 z:4}}`, `{a:{x:1 y:3 z:4}}`)
	// Index-merge: list ⊕ map of integer keys.
	w4LogWant(t, r, `merge ["a" "b" "c"] {"1":"d"}`, `['a' 'd' 'c']`)
	w4LogWant(t, r, `merge ["a"] {"1":"z"}`, `['a' 'z']`) // append at end
	w4LogWant(t, r, `merge ["a"] {"5":"z"}`, `['a']`)     // gap ignored
	w4LogWant(t, r, `merge ["a"] {"-1":"z"}`, `['a']`)    // negative ignored
	w4LogWant(t, r, `merge ["a"] {x:"z"}`, `['a']`)       // non-integer ignored
	// Index-merge: map ⊕ list.
	w4LogWant(t, r, `merge {"3":"d" x:"X"} ["a" "b" "c"]`, `['a' 'b' 'c' 'd']`)
	w4LogWant(t, r, `merge {"0":"z" "9":"q" "-2":"n"} ["a" "b"]`, `['z' 'b']`)
	// Guards: a type-literal Map is not a concrete container.
	if _, err := mergeListMapHandler([]Value{NewList(nil), NewTypeLiteral(TMap)}, nil, nil, r); err == nil {
		t.Error("mergeListMapHandler should reject a type-literal map")
	}
	if _, err := mergeMapListHandler([]Value{NewTypeLiteral(TMap), NewList(nil)}, nil, nil, r); err == nil {
		t.Error("mergeMapListHandler should reject a type-literal map")
	}
}

func TestW4StructReturnsFns(t *testing.T) {
	r := w4StructReg(t)
	mapV := NewMap(NewOrderedMap())
	listV := NewList([]Value{NewInteger(1)})

	// mergeReturns
	if out := mergeReturns([]Value{mapV, mapV}, r); !out[0].Parent.ConformsTo(TMap) {
		t.Errorf("merge map/map = %v", out)
	}
	if out := mergeReturns([]Value{listV, listV}, r); !out[0].Parent.ConformsTo(TList) {
		t.Errorf("merge list/list = %v", out)
	}
	if out := mergeReturns([]Value{mapV, listV}, r); len(out) != 1 {
		t.Errorf("merge mixed = %v", out)
	}
	if out := mergeReturns([]Value{mapV}, r); len(out) != 1 {
		t.Errorf("merge argc = %v", out)
	}

	// structDataShapeReturns
	if out := structDataShapeReturns(0)([]Value{mapV}, r); !out[0].Parent.ConformsTo(TMap) {
		t.Errorf("shape map = %v", out)
	}
	if out := structDataShapeReturns(0)([]Value{listV}, r); !out[0].Parent.ConformsTo(TList) {
		t.Errorf("shape list = %v", out)
	}
	if out := structDataShapeReturns(0)([]Value{NewInteger(1)}, r); len(out) != 1 {
		t.Errorf("shape scalar = %v", out)
	}
	if out := structDataShapeReturns(3)([]Value{mapV}, r); len(out) != 1 {
		t.Errorf("shape out-of-range = %v", out)
	}

	// setpathReturns
	if out := setpathReturns([]Value{NewString("a"), mapV, NewInteger(1)}, r); !out[0].Parent.ConformsTo(TMap) {
		t.Errorf("setpath path-first = %v", out)
	}
	if out := setpathReturns([]Value{NewInteger(9), NewString("a"), mapV}, r); !out[0].Parent.ConformsTo(TMap) {
		t.Errorf("setpath data-last = %v", out)
	}
	if out := setpathReturns([]Value{NewInteger(1), NewInteger(2), NewInteger(3)}, r); len(out) != 1 {
		t.Errorf("setpath no path = %v", out)
	}
	if out := setpathReturns([]Value{NewString("a"), mapV}, r); len(out) != 1 {
		t.Errorf("setpath argc = %v", out)
	}

	// reifyReturns
	if out := reifyReturns([]Value{NewTypeLiteral(TInteger), mapV}, r); len(out) != 1 ||
		!out[0].Parent.ConformsTo(TInteger) {
		t.Errorf("reify bare type = %v", out)
	}
	if out := reifyReturns([]Value{NewCarrier(TAny), mapV}, r); len(out) != 1 {
		t.Errorf("reify carrier = %v", out)
	}
	if out := reifyReturns([]Value{NewInteger(1), mapV}, r); len(out) != 1 {
		t.Errorf("reify non-type = %v", out)
	}
	if out := reifyReturns([]Value{mapV}, r); len(out) != 1 {
		t.Errorf("reify argc = %v", out)
	}
}

func TestW4StructItemsAndJsonify(t *testing.T) {
	r := w4StructReg(t)
	w4LogWant(t, r, `items {a:1 b:2}`, `[['a' 1] ['b' 2]]`)
	// jsonify: default form serialises; the "$" escape doubles the prefix.
	got, err := w3Run(t, r, `jsonify {a:1}`)
	if err != nil || !strings.Contains(got, "a") {
		t.Errorf("jsonify {a:1} = %q (%v)", got, err)
	}
	got, err = w3Run(t, r, `jsonify {"$class":"spoof"}`)
	if err != nil || !strings.Contains(got, "$$class") {
		t.Errorf("jsonify $-escape = %q (%v)", got, err)
	}
	got, err = w3Run(t, r, `jsonify [1 {b:2} "s"]`)
	if err != nil || !strings.Contains(got, "b") {
		t.Errorf("jsonify list = %q (%v)", got, err)
	}
	// Flags form via its handler (an empty flags map keeps defaults).
	out, err := jsonifyFlagsHandler([]Value{NewMap(NewOrderedMap()), covEmitMap("a", NewInteger(1))}, nil, nil, r)
	if err != nil || len(out) != 1 {
		t.Fatalf("jsonifyFlagsHandler: %v", err)
	}
	if _, err := jsonifyFlagsHandler([]Value{NewInteger(1), covEmitMap("a", NewInteger(1))}, nil, nil, r); err == nil {
		t.Error("non-map flags should error")
	}
	// escapeSerKey / valueToAnySer edges.
	if escapeSerKey("$x") != "$$x" || escapeSerKey("x") != "x" || escapeSerKey("") != "" {
		t.Error("escapeSerKey wrong")
	}
	if valueToAnySer(NewTypeLiteral(TMap)) != nil {
		t.Error("non-concrete value should serialise to nil")
	}
}

func TestW4ParseTextWord(t *testing.T) {
	r := w4StructReg(t)
	out, err := parseTextHandler([]Value{NewString(`{a:1 b:"x"}`)}, nil, nil, r)
	if err != nil || len(out) != 1 {
		t.Fatalf("parseTextHandler: %v", err)
	}
	if got := Canon(out); got != `{a:1 b:'x'}` {
		t.Errorf("parse result = %q", got)
	}
	if _, err := parseTextHandler([]Value{NewString(`{{{`)}, nil, nil, r); err == nil {
		t.Error("malformed text should raise parse_error")
	}
	// parseTextReturns folds a concrete parse and falls back otherwise.
	if out := parseTextReturns([]Value{NewString(`[1 2]`)}, r); len(out) != 1 ||
		!out[0].Parent.ConformsTo(TList) {
		t.Errorf("parseTextReturns concrete = %v", out)
	}
	if out := parseTextReturns([]Value{NewString(`{{{`)}, r); len(out) != 1 {
		t.Errorf("parseTextReturns malformed = %v", out)
	}
	if out := parseTextReturns([]Value{NewTypeLiteral(TString)}, r); len(out) != 1 {
		t.Errorf("parseTextReturns non-concrete = %v", out)
	}
}

func TestW4StructGetpath(t *testing.T) {
	r := w4StructReg(t)
	w4LogWant(t, r, `getpath "a.b" {a:{b:5}}`, `5`)
	w4LogWant(t, r, `getpath "nope.x" {a:1}`, `None`)
}

// ---- __folder / __file -----------------------------------------------------------

func TestW4FileInfoWords(t *testing.T) {
	r := w3TypeReg()
	r.BaseFile = "/tmp/mods/m.boru"
	w3TypeWantReg := func(src, want string) {
		t.Helper()
		got, err := w3Run(t, r, src)
		if err != nil {
			t.Fatalf("%q: %v", src, err)
		}
		if got != want {
			t.Errorf("%q = %q, want %q", src, got, want)
		}
	}
	w3TypeWantReg(`__file`, `'m.boru'`)
	got, err := w3Run(t, r, `__folder`)
	if err != nil || !strings.Contains(got, "mods") {
		t.Errorf("__folder = %q (%v)", got, err)
	}

	// BaseDir fallback (no BaseFile), relative directory.
	r2 := w3TypeReg()
	r2.BaseDir = "relmods/sub"
	out, err := srcFolderHandler(nil, nil, nil, r2)
	if err != nil || len(out) != 1 {
		t.Fatalf("srcFolderHandler: %v", err)
	}
	if got := Canon(out); !strings.Contains(got, "relmods") {
		t.Errorf("relative folder = %q", got)
	}
	// No source location at all: empty Path / empty String, never an error.
	r3 := w3TypeReg()
	if out, err := srcFolderHandler(nil, nil, nil, r3); err != nil || len(out) != 1 {
		t.Errorf("empty folder: %v %v", out, err)
	}
	if out, err := srcFileHandler(nil, nil, nil, r3); err != nil || Canon(out) != `''` {
		t.Errorf("empty file = %q (%v)", Canon(out), err)
	}
	// splitPathSegments drops empties and the leading separator.
	if got := splitPathSegments("/a//b/c/"); len(got) != 3 || got[0] != "a" || got[2] != "c" {
		t.Errorf("splitPathSegments = %v", got)
	}
}

// ---- small ReturnsFns and helpers -------------------------------------------------

func TestW4SizeAndCloneReturns(t *testing.T) {
	r := w3TypeReg()
	lst := NewList([]Value{NewInteger(1), NewInteger(2), NewInteger(3)})
	if out := sizeReturns([]Value{lst}, r); Canon(out) != `3` {
		t.Errorf("sizeReturns list = %q", Canon(out))
	}
	m := NewOrderedMap()
	m.Set("a", NewInteger(1))
	if out := sizeReturns([]Value{NewMap(m)}, r); Canon(out) != `1` {
		t.Errorf("sizeReturns map = %q", Canon(out))
	}
	if out := sizeReturns([]Value{NewString("abc")}, r); len(out) != 1 || IsConcrete(out[0]) {
		t.Errorf("sizeReturns string should stay a carrier: %v", out)
	}
	if out := sizeReturns([]Value{NewTypeLiteral(TList)}, r); len(out) != 1 || IsConcrete(out[0]) {
		t.Errorf("sizeReturns literal should stay a carrier: %v", out)
	}
	if out := sizeReturns(nil, r); len(out) != 1 {
		t.Errorf("sizeReturns argc = %v", out)
	}

	if out := cloneReturnsFn(nil, r); len(out) != 1 {
		t.Errorf("cloneReturnsFn argc = %v", out)
	}
	if out := cloneReturnsFn([]Value{NewInteger(1)}, r); len(out) != 1 ||
		!out[0].Parent.ConformsTo(TInteger) {
		t.Errorf("cloneReturnsFn should preserve the input type: %v", out)
	}
}

func TestW4StreamKindMint(t *testing.T) {
	sk := NewStreamKind()
	if sk == nil || sk.Name() != "StreamKind" {
		t.Fatalf("NewStreamKind = %v", sk)
	}
	if !NewAtom("stdout").Is(sk) || !NewAtom("stdin").Is(sk) || !NewAtom("stderr").Is(sk) {
		t.Error("standard stream atoms should inhabit StreamKind")
	}
	if NewAtom("other").Is(sk) || NewString("stdout").Is(sk) {
		t.Error("non-stream values must not inhabit StreamKind")
	}
}

func TestW4ValueToMap(t *testing.T) {
	m := NewOrderedMap()
	m.Set("a", NewInteger(1))
	m.Set("b", NewString("x"))
	got := valueToMap(NewMap(m))
	if len(got) != 2 || got["a"] == nil || got["b"] != "x" {
		t.Errorf("valueToMap = %v", got)
	}
	if valueToMap(NewTypeLiteral(TMap)) != nil {
		t.Error("valueToMap of a type literal should be nil")
	}
	if valueToMap(NewInteger(1)) != nil {
		t.Error("valueToMap of a scalar should be nil")
	}
}
