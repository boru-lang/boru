package parser

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// --- typed lists ([:T]) with concrete elements ---

func TestTypedListWave3Elements(t *testing.T) {
	for _, src := range []string{"[:Integer 1 2]", "[:Integer, 1, 2]", "[1, 2, :Integer]"} {
		vals := mustParseWave3(t, src)
		if len(vals) != 1 || !core.IsTypedList(&vals[0]) {
			t.Fatalf("%q: expected one typed list, got %v", src, vals)
		}
		ci, err := core.AsChildType(vals[0])
		if err != nil {
			t.Fatalf("%q: AsChildType: %v", src, err)
		}
		// The child constraint is the OPAQUE name Word (ADR-012 rule 4);
		// the engine's canonical cascade resolves it at consumption.
		if w, err := core.AsWord(&ci.Child); err != nil || w.Name != "Integer" {
			t.Errorf("%q: child constraint %v, want word(Integer)", src, ci.Child)
		}
		if len(ci.Elements) != 2 {
			t.Fatalf("%q: got %d elements, want 2", src, len(ci.Elements))
		}
		for i, want := range []int64{1, 2} {
			if n, err := core.AsInteger(ci.Elements[i]); err != nil || n != want {
				t.Errorf("%q: element %d = %v, want %d", src, i, ci.Elements[i], want)
			}
		}
	}
}

func TestTypedMapWave3Entries(t *testing.T) {
	// Source order is b, a — NOT alphabetical — so this pins D1 insertion
	// order for the typed-map entries path (the `ko` order channel reaches
	// convertTypedMap, not just plain maps). design/FLEX-ATTRS.1.md §3.
	vals := mustParseWave3(t, "{:Integer b:2 a:1}")
	if len(vals) != 1 || !core.IsTypedMap(&vals[0]) {
		t.Fatalf("expected one typed map, got %v", vals)
	}
	ci, err := core.AsChildType(vals[0])
	if err != nil {
		t.Fatalf("AsChildType: %v", err)
	}
	if w, err := core.AsWord(&ci.Child); err != nil || w.Name != "Integer" {
		t.Errorf("child constraint %v, want word(Integer)", ci.Child)
	}
	if len(ci.Entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(ci.Entries))
	}
	// Entries are collected in SOURCE order (b before a), not sorted.
	for i, want := range []struct {
		key string
		n   int64
	}{{"b", 2}, {"a", 1}} {
		e := ci.Entries[i]
		if e.Key != want.key {
			t.Errorf("entry %d key %q, want %q", i, e.Key, want.key)
		}
		if n, err := core.AsInteger(e.Value); err != nil || n != want.n {
			t.Errorf("entry %d value %v, want %d", i, e.Value, want.n)
		}
	}
}

func TestTypedContainerWave3Errors(t *testing.T) {
	// Malformed child constraints and malformed elements/entries all fail
	// cleanly, at the top level and nested.
	for _, src := range []string{
		"[:1__0]",           // bad child (top-level typed list)
		"{:1__0}",           // bad child (top-level typed map)
		"[:Integer 1__0]",   // bad element alongside the constraint
		"{:Integer a:1__0}", // bad entry alongside the constraint
	} {
		wantParseErrWave3(t, src, "misplaced `_`")
	}
}

func TestTypedContainerWave3Nested(t *testing.T) {
	// A typed map as a word-context list element.
	vals := mustParseWave3(t, "[{:String}]")
	if len(vals) != 1 {
		t.Fatalf("[{:String}]: got %d values, want 1", len(vals))
	}
	lst, err := core.AsList(vals[0])
	if err != nil || lst.Len() != 1 {
		t.Fatalf("[{:String}]: expected a 1-element list, got %v (err %v)", vals[0], err)
	}
	if !core.IsTypedMap(lst.Ref(0)) {
		t.Errorf("[{:String}]: element is not a typed map: %s", lst.Get(0).String())
	}
}

func TestTypedContainerWave3DepthLimit(t *testing.T) {
	// Pathologically deep TYPED nesting takes the convertTypedList /
	// convertTypedMap recursion and must fail with the clean depth error.
	deep := func(open, close string) string {
		var b strings.Builder
		for i := 0; i < maxParseNestingDepth+50; i++ {
			b.WriteString(open)
		}
		b.WriteString("Integer")
		for i := 0; i < maxParseNestingDepth+50; i++ {
			b.WriteString(close)
		}
		return b.String()
	}
	for name, src := range map[string]string{
		"typed-lists": deep("[:", "]"),
		"typed-maps":  deep("{:", "}"),
	} {
		_, err := parseWave3(t, src)
		if err == nil {
			t.Fatalf("%s: deep typed nesting parsed without error", name)
		}
		if !strings.Contains(err.Error(), "nesting") {
			t.Errorf("%s: error %q is not the nesting-depth diagnostic", name, err)
		}
	}
}

// --- data-context container error propagation ---

func TestDataContainerWave3Errors(t *testing.T) {
	for _, src := range []string{
		"[{a:1__0}]", // map element of a word-context list
		"[a:1__0]",   // raw pair-map element
		"{a:[1__0]}", // list value inside a map
	} {
		wantParseErrWave3(t, src, "misplaced `_`")
	}
}

// --- generics angle sugar ---

func TestAngleWave3DefHead(t *testing.T) {
	// `def Name<T>` parses to `def <angle marker> …` — the parser does
	// NOT recognise `def` (ADR-012 rule 3 amendment); the engine picks
	// the head form at the binder's /q slot from the precomputed Head.
	vals := mustParseWave3(t, "def Box<T> 1")
	if len(vals) != 3 {
		t.Fatalf("def Box<T> 1: got %d values, want 3: %v", len(vals), vals)
	}
	if w := wordNameWave3(t, vals[0], "def head"); w.Name != "def" {
		t.Errorf("def Box<T> 1: value 0 word %q, want def", w.Name)
	}
	info, ok := core.AsSugar(vals[1])
	if !ok || info.Kind != core.SugarAngle || info.Name != "Box" {
		t.Fatalf("def Box<T> 1: expected an angle marker for Box, got %v", vals[1])
	}
	params, err := core.AsList(info.Head)
	if err != nil || params.Len() != 1 {
		t.Fatalf("def Box<T> 1: head params %v (err %v), want a 1-element list", info.Head, err)
	}
	if w := wordNameWave3(t, params.Get(0), "param"); w.Name != "T" {
		t.Errorf("def Box<T> 1: param %q, want T", w.Name)
	}
	// Multiple plain params.
	vals = mustParseWave3(t, "def Box<T Q> 1")
	info, _ = core.AsSugar(vals[1])
	params, err = core.AsList(info.Head)
	if err != nil || params.Len() != 2 {
		t.Fatalf("def Box<T Q> 1: head params %v (err %v), want 2 entries", info.Head, err)
	}
}

func TestAngleWave3DefHeadConstraints(t *testing.T) {
	// `extends` and `=` entries become (T extends C) / (T default D) spans.
	for src, kw := range map[string]string{
		"def Box<T extends Integer> 1": "extends",
		"def Box<T = Integer> 1":       "sugar(gen-default)",
	} {
		vals := mustParseWave3(t, src)
		if len(vals) != 3 {
			t.Fatalf("%q: got %d values, want 3: %v", src, len(vals), vals)
		}
		info, ok := core.AsSugar(vals[1])
		if !ok || info.Kind != core.SugarAngle {
			t.Fatalf("%q: expected an angle marker, got %v", src, vals[1])
		}
		params, err := core.AsList(info.Head)
		if err != nil || params.Len() != 1 {
			t.Fatalf("%q: head params %v (err %v), want 1 entry", src, info.Head, err)
		}
		entry := params.Get(0)
		if !core.IsParenExpr(&entry) {
			t.Fatalf("%q: param entry is not a ParenExpr: %v", src, entry)
		}
		if s := entry.String(); !strings.Contains(s, kw) {
			t.Errorf("%q: entry %q lacks the %q keyword", src, s, kw)
		}
	}
}

func TestAngleWave3DefHeadErrors(t *testing.T) {
	// A head-only defect (a lowercase param, a dangling operator) no
	// longer fails the PARSE — the marker parses with HeadErr, and the
	// error surfaces if (and only if) a binder selects the head form.
	for src, sub := range map[string]string{
		"def Box<t> 1":         "capitalised",
		"def Box<T extends> 1": "needs a value",
	} {
		vals := mustParseWave3(t, src)
		info, ok := core.AsSugar(vals[1])
		if !ok || info.HeadErr == "" || !strings.Contains(info.HeadErr, sub) {
			t.Errorf("%q: expected HeadErr containing %q, got %+v", src, sub, info)
		}
	}
	// A malformed constraint OPERAND fails both forms — still a parse error.
	wantParseErrWave3(t, "def Box<T extends 1__0> 1", "misplaced `_`")
}

func TestAngleWave3UseSites(t *testing.T) {
	// Word-context list element and data-context map value both desugar to
	// the canonical `(Name of [args])` paren span.
	for _, src := range []string{"[Box<Integer>]", "{x: Box<Integer>}"} {
		vals := mustParseWave3(t, src)
		if len(vals) != 1 {
			t.Fatalf("%q: got %d values, want 1", src, len(vals))
		}
		if s := vals[0].String(); !strings.Contains(s, "sugar(angle Box") {
			t.Errorf("%q: rendering %q lacks the angle marker", src, s)
		}
	}
	// Empty group.
	vals := mustParseWave3(t, "Box<>")
	if len(vals) != 1 || !core.IsSugar(&vals[0]) {
		t.Fatalf("Box<>: expected one angle marker, got %v", vals)
	}
	// An error inside the argument list surfaces.
	wantParseErrWave3(t, "Box<1__0>", "misplaced `_`")
}

func TestAngleWave3Unclosed(t *testing.T) {
	// Unclosed at EOF, in word and data context, names the receiver.
	wantParseErrWave3(t, "Box<Integer", "unclosed angle bracket: Box<")
	wantParseErrWave3(t, "{x: Box<Integer", "unclosed angle bracket: Box<")
}

// TestAngleWave3GateRejections pins the contextual gate: `<` after a
// non-capitalised / quoted / non-name value is NOT generics, and falls to
// the XML consumer, which rejects it — an error, never a panic.
func TestAngleWave3GateRejections(t *testing.T) {
	for _, src := range []string{
		"'Box'<1>",  // quoted receiver: not a type name
		"1<2",       // numeric receiver
		"Box<A><B>", // second angle group after a folded one
	} {
		wantParseErrWave3(t, src, "")
	}
}

// --- pair-key edge cases ---

func TestPairKeyWave3Edges(t *testing.T) {
	// A NUMERIC optional key (`{1?:Integer}`): the key comes from the raw
	// token source and the optional desugar still applies.
	vals := mustParseWave3(t, "{1?:Integer}")
	if len(vals) != 1 {
		t.Fatalf("{1?:Integer}: got %d values, want 1", len(vals))
	}
	m, err := core.AsMap(vals[0])
	if err != nil {
		t.Fatalf("{1?:Integer}: AsMap: %v", err)
	}
	v, ok := m.Get("1")
	if !ok {
		t.Fatalf("{1?:Integer}: map lacks key \"1\": %s", vals[0].String())
	}
	if s := v.String(); !strings.Contains(s, "tor") {
		t.Errorf("{1?:Integer}: optional value %q is not a disjunct", s)
	}
	// Empty quoted keys (plain and optional) parse without error.
	for _, src := range []string{"{'':1}", "{''?:1}"} {
		vals := mustParseWave3(t, src)
		if len(vals) != 1 {
			t.Fatalf("%q: got %d values, want 1", src, len(vals))
		}
	}
	// Optional list params alongside plain ones, plain values, and other
	// optionals.
	for _, src := range []string{
		"[a?:Integer b:String]",
		"[x?:Integer 1]",
		"[a?:Integer, b?:String]",
	} {
		vals := mustParseWave3(t, src)
		if len(vals) != 1 {
			t.Fatalf("%q: got %d values, want 1", src, len(vals))
		}
		if s := vals[0].String(); !strings.Contains(s, "tor None tor Absent") {
			t.Errorf("%q: %q lacks the parser-ordered optional disjunct", src, s)
		}
	}
}
