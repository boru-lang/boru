package native

import (
	"bytes"
	"strings"
	"testing"
)

// TestMarkMoveBasic tests that mark/move jumps the pointer back and prints twice.
// Equivalent to: [11, mark(A), 22, printstr "x", 33, move(A)]
// Expected: prints "xx", stack result is [11, 22, 33]
func TestMarkMoveBasic(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	var buf bytes.Buffer
	r.Output = &buf

	id := NextMarkID()
	// printstr is forward-collecting: it collects arg from after the word.
	// The body between mark and move is what gets replayed.
	body := []Value{
		NewInteger(22),
		NewWord("printstr"),
		NewString("x"),
		NewInteger(33),
	}
	input := []Value{
		NewInteger(11),
		NewMark(id, body...),
		NewInteger(22),
		NewWord("printstr"),
		NewString("x"),
		NewInteger(33),
		NewMove(id, "test loop"),
	}

	result := runBoru(t, r, input)

	got := buf.String()
	if got != "xx" {
		t.Errorf("expected output %q, got %q", "xx", got)
	}

	// After mark/move are removed and loop completes, stack should be [11, 22, 33].
	if len(result) != 3 {
		t.Fatalf("expected 3 values on stack, got %d: %v", len(result), result)
	}
	_as0, _ := AsInteger(result[0])
	if !result[0].Parent().ConformsTo(TInteger) || _as0 != 11 {
		t.Errorf("result[0] = %v, want 11", result[0])
	}
	_as1, _ := AsInteger(result[1])
	if !result[1].Parent().ConformsTo(TInteger) || _as1 != 22 {
		t.Errorf("result[1] = %v, want 22", result[1])
	}
	_as2, _ := AsInteger(result[2])
	if !result[2].Parent().ConformsTo(TInteger) || _as2 != 33 {
		t.Errorf("result[2] = %v, want 33", result[2])
	}
}

// TestMarkMoveOneShotRemoval verifies mark/move are removed after one trigger,
// preventing infinite loops.
func TestMarkMoveOneShotRemoval(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	var buf bytes.Buffer
	r.Output = &buf

	id := NextMarkID()
	// mark, printstr "a", move → prints "aa", then mark/move gone, no infinite loop.
	body := []Value{
		NewWord("printstr"),
		NewString("a"),
	}
	input := []Value{
		NewMark(id, body...),
		NewWord("printstr"),
		NewString("a"),
		NewMove(id, "one-shot test"),
	}

	result := runBoru(t, r, input)

	got := buf.String()
	if got != "aa" {
		t.Errorf("expected output %q, got %q", "aa", got)
	}

	// Stack should be empty (printstr consumes its arg, mark/move removed).
	if len(result) != 0 {
		t.Errorf("expected empty stack, got %d items: %v", len(result), result)
	}
}

// TestMarkMoveNotFound tests error when move references nonexistent mark.
func TestMarkMoveNotFound(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	input := []Value{
		NewMove("nonexistent", "test: dangling move"),
	}

	err = runBoruError(t, r, input)
	if err == nil {
		t.Fatal("expected error for move with missing mark")
	}
	if !strings.Contains(err.Error(), "nonexistent") {
		t.Errorf("error should mention mark ID: %v", err)
	}
	if !strings.Contains(err.Error(), "test: dangling move") {
		t.Errorf("error should mention reason: %v", err)
	}
}

// TestMarkMoveMultiplePairs tests multiple independent mark/move pairs.
func TestMarkMoveMultiplePairs(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	var buf bytes.Buffer
	r.Output = &buf

	id1 := NextMarkID()
	id2 := NextMarkID()
	// Two nested mark/move pairs:
	// mark(A), mark(B), printstr "x", move(B), printstr "y", move(A)
	// First pass: prints "x", hits move(B) → jump to after mark(B), removes B pair
	//   Stack becomes: mark(A), printstr "x", printstr "y", move(A)
	//   Wait, after B removal: mark(A), "x" printstr "y" printstr move(A)
	// Actually let me trace more carefully...
	// Let's use a simpler nested test.

	// mark(A), "x" printstr, mark(B), "y" printstr, move(B), move(A)
	// Pass 1: "x"→printstr prints "x", hits mark(B) skip, "y"→printstr prints "y",
	//   hits move(B)→jump to after mark(B), removes B pair.
	//   Stack: mark(A), "x" printstr, "y" printstr, move(A)
	//   Pointer at where mark(B) was = position of "y" printstr (now)
	//   Continues: "y"→printstr prints "y", hits move(A)→jump to after mark(A),
	//   removes A pair. Stack: "x" printstr "y" printstr
	//   Pointer at position 0. Continues: "x"→printstr prints "x", "y"→printstr prints "y"
	// Total output: "x" "y" "y" "x" "y" = "xyyx y"... this is getting complex.

	// Simpler test: two sequential (non-nested) mark/move pairs.
	body1 := []Value{NewWord("printstr"), NewString("a")}
	body2 := []Value{NewWord("printstr"), NewString("b")}
	input := []Value{
		NewMark(id1, body1...),
		NewWord("printstr"),
		NewString("a"),
		NewMove(id1, "first loop"),
		NewMark(id2, body2...),
		NewWord("printstr"),
		NewString("b"),
		NewMove(id2, "second loop"),
	}

	result := runBoru(t, r, input)

	got := buf.String()
	if got != "aabb" {
		t.Errorf("expected output %q, got %q", "aabb", got)
	}

	if len(result) != 0 {
		t.Errorf("expected empty stack, got %d items: %v", len(result), result)
	}
}

// TestMarkMoveWithLiterals tests that literals between mark/move survive.
func TestMarkMoveWithLiterals(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)

	id := NextMarkID()
	// [1, mark, 2, 3, add, move] → first pass: 2+3=5, move triggers,
	// replays body (2, 3, add) replacing mark..move range → 2+3=5, result is [1, 5]
	body := []Value{NewInteger(2), NewInteger(3), NewWord("add")}
	input := []Value{
		NewInteger(1),
		NewMark(id, body...),
		NewInteger(2),
		NewInteger(3),
		NewWord("add"),
		NewMove(id, "literal test"),
	}

	result := runBoru(t, r, input)

	// First pass: 1, mark, 2+3=5, move → replays body replacing mark..move range
	// Stack becomes: [1, 2, 3, add], pointer at index 1
	// Second pass: 2+3=5 → stack is [1, 5]
	if len(result) != 2 {
		t.Fatalf("expected 2 values, got %d: %v", len(result), result)
	}
	_as3, _ := AsInteger(result[0])
	if _as3 != 1 {
		t.Errorf("result[0] = %v, want 1", result[0])
	}
	_as4, _ := AsInteger(result[1])
	if _as4 != 5 {
		t.Errorf("result[1] = %v, want 5", result[1])
	}
}

// TestMarkMoveString tests Value.String() for mark and move.
func TestMarkMoveString(t *testing.T) {
	m := NewMark("test123", NewInteger(1))
	if got := m.String(); got != "mark(test123)" {
		t.Errorf("mark string = %q, want %q", got, "mark(test123)")
	}

	mv := NewMove("test123", "for loop")
	if got := mv.String(); got != "move(test123,for loop)" {
		t.Errorf("move string = %q, want %q", got, "move(test123,for loop)")
	}
}

// TestMarkMoveIsMethods tests IsMark/IsMove type checks.
func TestMarkMoveIsMethods(t *testing.T) {
	m := NewMark("x", NewInteger(1))
	if !IsMark(&m) {
		t.Error("NewMark should be IsMark()")
	}
	if IsMove(&m) {
		t.Error("NewMark should not be IsMove()")
	}

	mv := NewMove("x", "reason")
	if !IsMove(&mv) {
		t.Error("NewMove should be IsMove()")
	}
	if IsMark(&mv) {
		t.Error("NewMove should not be IsMark()")
	}

	// Marks and moves should match "any" (Any matches everything now)
	if !m.Parent().ConformsTo(TAny) {
		t.Error("mark should match TAny")
	}
	if !mv.Parent().ConformsTo(TAny) {
		t.Error("move should match TAny")
	}
}

// TestNextMarkIDUnique tests that NextMarkID generates unique IDs.
func TestNextMarkIDUnique(t *testing.T) {
	ids := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id := NextMarkID()
		if ids[id] {
			t.Fatalf("duplicate mark ID: %s", id)
		}
		ids[id] = true
	}
}

// TestHaltOnUndefinedStackEntry tests that the engine halts on a zero-value entry.
func TestHaltOnUndefinedStackEntry(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	input := []Value{
		NewInteger(1),
		{}, // undefined/zero Value
		NewInteger(3),
	}

	err = runBoruError(t, r, input)
	if err == nil {
		t.Fatal("expected error for undefined stack entry")
	}
	if !strings.Contains(err.Error(), "halt") {
		t.Errorf("error should mention halt: %v", err)
	}
}
