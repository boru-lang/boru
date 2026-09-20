package basic

import (
	"strings"
	"testing"
)

// TestRunWhileLoopTypeLiteralGuards — both operands must be concrete
// lists; type literals are declined loudly.
func TestRunWhileLoopTypeLiteralGuards(t *testing.T) {
	r := newTestRegistry(t)
	lit := NewTypeLiteral(TList)
	body := NewList([]Value{NewInteger(1)})
	if _, err := RunWhileLoop(r, lit, body); err == nil || !strings.Contains(err.Error(), "condition must be a concrete list") {
		t.Fatalf("type-literal condition must decline, got %v", err)
	}
	if _, err := RunWhileLoop(r, body, lit); err == nil || !strings.Contains(err.Error(), "body must be a concrete list") {
		t.Fatalf("type-literal body must decline, got %v", err)
	}
}

// TestRunWhileLoopTokens — the built region is mark + condition + move,
// with a while-mode continuation carrying copies of both regions.
func TestRunWhileLoopTokens(t *testing.T) {
	r := newTestRegistry(t)
	cond := NewList([]Value{NewBoolean(false)})
	body := NewList([]Value{NewInteger(9)})
	tokens, err := RunWhileLoop(r, cond, body)
	if err != nil {
		t.Fatal(err)
	}
	if len(tokens) != 3 {
		t.Fatalf("want mark+cond+move, got %d tokens", len(tokens))
	}
	info, merr := AsMove(tokens[2])
	if merr != nil || info.Cont == nil || info.Cont.WhileCond == nil {
		t.Fatalf("move must carry a while-mode continuation: %v %v", merr, info.Cont)
	}
	if info.Cont.WhileInBody {
		t.Error("the loop must start in the condition phase")
	}
}

// TestWhileReturnsFnModel — in a basic-only registry the analysis seam
// is the named inactive default, so both regions analyse to an empty
// residual and the model contributes nothing (the zero-net arm). The
// typed and disjunct arms need the full checker braid and are driven by
// lang/go/while_check_test.go through the merged profile.
func TestWhileReturnsFnModel(t *testing.T) {
	r := newTestRegistry(t)
	defer r.Check.Begin()()
	cond := NewList([]Value{NewBoolean(true)})
	if out := whileReturnsFn([]Value{cond, NewList(nil)}, r); len(out) != 0 {
		t.Fatalf("zero-net body must contribute nothing, got %v", out)
	}
}
