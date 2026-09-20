package native

// Contract tests for the unified helpers introduced by the
// dedup/refactor pass: the array-word concrete-list guards
// (native_array.go), the shared scalar-leaf renderer (emit.go), and
// the delegating sortedMapKeys alias (fileio.go → transform.go). The
// error codes/details and emit outputs asserted here are pinned by the
// langspec suites; these tests guard the helper-level contract
// directly, negative cases included (see CLAUDE.md "Test discipline").

import (
	"strings"
	"testing"
)

func TestArrayWordErrCode(t *testing.T) {
	cases := map[string]string{
		"shape":     "shape_error",
		"insert-at": "insert_at_error",
		"remove-at": "remove_at_error",
		"for-each":  "for_each_error",
	}
	for word, want := range cases {
		if got := arrayWordErrCode(word); got != want {
			t.Errorf("arrayWordErrCode(%q) = %q, want %q", word, got, want)
		}
	}
}

func TestRequireListArg(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatalf("DefaultRegistry: %v", err)
	}

	// Negative: a type literal must be rejected with the word's pinned
	// per-word code and detail (NOT eng's RequireConcreteList format).
	_, err = requireListArg(r, NewTypeLiteral(TList), "insert-at", "a concrete data list")
	if err == nil {
		t.Fatal("expected an error for a type literal arg")
	}
	ae, ok := err.(*BoruError)
	if !ok {
		t.Fatalf("expected *BoruError, got %T (%v)", err, err)
	}
	if ae.Code != "insert_at_error" {
		t.Errorf("code = %q, want %q", ae.Code, "insert_at_error")
	}
	if !strings.Contains(ae.Detail, "insert-at: expected a concrete data list") {
		t.Errorf("detail = %q, want it to contain the pinned message", ae.Detail)
	}

	// Positive: a concrete list unwraps.
	lst, err := requireListArg(r, NewList([]Value{NewInteger(1), NewInteger(2)}), "take", "concrete list")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if lst.Len() != 2 {
		t.Errorf("Len = %d, want 2", lst.Len())
	}
}

func TestRequireConcreteArgs(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatalf("DefaultRegistry: %v", err)
	}

	// Positive: all-concrete passes.
	if err := requireConcreteArgs(r, "each", "concrete lists",
		NewList(nil), NewList(nil)); err != nil {
		t.Errorf("unexpected error for concrete args: %v", err)
	}

	// Negative: any non-concrete value raises the combined pinned error.
	err = requireConcreteArgs(r, "for-each", "concrete lists",
		NewList(nil), NewTypeLiteral(TList))
	if err == nil {
		t.Fatal("expected an error for a type-literal arg")
	}
	ae, ok := err.(*BoruError)
	if !ok {
		t.Fatalf("expected *BoruError, got %T (%v)", err, err)
	}
	if ae.Code != "for_each_error" {
		t.Errorf("code = %q, want %q", ae.Code, "for_each_error")
	}
	if !strings.Contains(ae.Detail, "for-each: expected concrete lists") {
		t.Errorf("detail = %q, want it to contain the pinned message", ae.Detail)
	}
}

// TestScalarLeafSharedArms pins the format-independent arms (Float /
// Integer / Boolean render identically through every leaf renderer) and
// the per-format divergences (string quoting; None supported by
// json/yaml, rejected by toml, String()-fallback for ini).
func TestScalarLeafSharedArms(t *testing.T) {
	shared := []struct {
		v    Value
		want string
	}{
		{NewInteger(42), "42"},
		{NewFloat(1.5), "1.5"},
		{NewBoolean(true), "true"},
		{NewBoolean(false), "false"},
	}
	for _, tc := range shared {
		if got := emitScalarText(tc.v); got != tc.want {
			t.Errorf("emitScalarText(%s) = %q, want %q", tc.v.String(), got, tc.want)
		}
		if got := yamlLeaf(tc.v); got != tc.want {
			t.Errorf("yamlLeaf(%s) = %q, want %q", tc.v.String(), got, tc.want)
		}
		if got, err := tomlScalar(tc.v); err != nil || got != tc.want {
			t.Errorf("tomlScalar(%s) = %q, %v, want %q, nil", tc.v.String(), got, err, tc.want)
		}
		if got := iniScalar(tc.v); got != tc.want {
			t.Errorf("iniScalar(%s) = %q, want %q", tc.v.String(), got, tc.want)
		}
	}

	// String quoting stays per-format.
	s := NewString("hi")
	if got := emitScalarText(s); got != `"hi"` {
		t.Errorf("emitScalarText(string) = %q, want %q", got, `"hi"`)
	}
	if got, err := tomlScalar(s); err != nil || got != `"hi"` {
		t.Errorf("tomlScalar(string) = %q, %v, want %q, nil", got, err, `"hi"`)
	}
	if got := yamlLeaf(s); got != "hi" {
		t.Errorf("yamlLeaf(string) = %q, want %q", got, "hi")
	}
	if got := iniScalar(s); got != "hi" {
		t.Errorf("iniScalar(string) = %q, want %q", got, "hi")
	}

	// None: null in json/yaml; toml must DECLINE it (no null in TOML —
	// falling to a string would corrupt the round-trip).
	none := NewNone()
	if got := emitScalarText(none); got != "null" {
		t.Errorf("emitScalarText(none) = %q, want %q", got, "null")
	}
	if got := yamlLeaf(none); got != "null" {
		t.Errorf("yamlLeaf(none) = %q, want %q", got, "null")
	}
	if _, err := tomlScalar(none); err == nil {
		t.Error("tomlScalar(none) must error — TOML has no null")
	} else if EmitErrorCode(err) != "emit_unsupported" {
		t.Errorf("tomlScalar(none) error code = %q, want emit_unsupported", EmitErrorCode(err))
	}
}

// TestSortedAnyMapKeys pins the shared key-sorting helper (the fileio
// sortedMapKeys alias delegates to it; see also TestSortedMapKeys in
// fileio_test.go).
func TestSortedAnyMapKeys(t *testing.T) {
	m := map[string]any{"zz": 1, "a": 2, "m": 3, "": 4}
	keys := sortedAnyMapKeys(m)
	want := []string{"", "a", "m", "zz"}
	if len(keys) != len(want) {
		t.Fatalf("got %v, want %v", keys, want)
	}
	for i := range want {
		if keys[i] != want[i] {
			t.Fatalf("got %v, want %v", keys, want)
		}
	}
	if got := sortedAnyMapKeys(map[string]any{}); len(got) != 0 {
		t.Errorf("empty map: got %v, want []", got)
	}
}
