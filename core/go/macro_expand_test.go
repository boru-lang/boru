package core

import "testing"

// Phase 1e (design/legacy/MACROS-PHASE1.10.ignore §8): the expansion cache primitive. The
// integration (a macro re-applied to the same operands expands once) is
// covered by the macro spec/Go tests; this pins the registry-level memo.
func TestMacroCachePrimitive(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	if _, ok := r.macroCacheGet("k"); ok {
		t.Error("fresh registry: expected a miss")
	}
	r.macroCachePut("k", []Value{NewInteger(1), NewWord("add")})
	got, ok := r.macroCacheGet("k")
	if !ok || len(got) != 2 {
		t.Fatalf("after put: expected a 2-token hit, got ok=%v len=%d", ok, len(got))
	}
	// A different key is independent.
	if _, ok := r.macroCacheGet("other"); ok {
		t.Error("unrelated key should miss")
	}
	// Clear (what the `macro` definer calls on redefinition) drops everything.
	r.MacroCacheClear()
	if _, ok := r.macroCacheGet("k"); ok {
		t.Error("after MacroCacheClear: expected a miss")
	}
}

// The cache key folds the macro name + operand canon, so identical operand
// forms collide (a hit) and different ones don't.
func TestMacroOperandsKey(t *testing.T) {
	a := macroOperandsKey("m", []Value{NewInteger(1), NewWord("x")})
	b := macroOperandsKey("m", []Value{NewInteger(1), NewWord("x")})
	if a != b {
		t.Errorf("same name+operands must produce the same key:\n %q\n %q", a, b)
	}
	if a == macroOperandsKey("m", []Value{NewInteger(2), NewWord("x")}) {
		t.Error("different operands must produce different keys")
	}
	if a == macroOperandsKey("n", []Value{NewInteger(1), NewWord("x")}) {
		t.Error("different macro name must produce different keys")
	}
}
