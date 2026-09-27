package native

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// behave_stepless_body_test.go pins runBehaviorBody's engine-free arm: a
// behaviour body of scalar literals is its own residual under the NewTop
// regime, EXCEPT where a top engine's run of it is not the identity — an
// atom (its referent is stamped) or a `gen` spec left pending (the end-of-run
// drain raises). Both exclusions, and every non-scalar token, keep the engine.

func TestBehaviorBodySteplessClassifies(t *testing.T) {
	r := w9Reg(t)
	atom := core.NewAtom("x")
	cases := []struct {
		label string
		body  []Value
		want  bool
	}{
		{"a string literal", []Value{NewString("T")}, true},
		{"several scalar literals", []Value{NewInteger(1), NewString("b"), NewBoolean(true)}, true},
		{"the empty body", nil, true},
		{"an atom", []Value{atom}, false},
		{"an atom after a scalar", []Value{NewInteger(1), atom}, false},
		{"a word", []Value{NewWord("drop"), NewString("x")}, false},
		{"a list", []Value{NewList([]Value{NewInteger(1)})}, false},
	}
	for _, c := range cases {
		if got := behaviorBodyStepless(r, c.body); got != c.want {
			t.Errorf("%s: behaviorBodyStepless = %v, want %v", c.label, got, c.want)
		}
	}
	// A pending gen spec: the top engine's end-of-run drain would raise on
	// it, so even a scalar body keeps the engine.
	if err := r.SetPendingGen(&core.GenSpecInfo{}); err != nil {
		t.Fatal(err)
	}
	defer r.TakePendingGen()
	if behaviorBodyStepless(r, []Value{NewString("T")}) {
		t.Error("a scalar body with a gen spec pending must keep the engine")
	}
}

// TestRunBehaviorBodyStepless: the engine-free answer is the body itself, a
// copy the caller may not alias, and a body the arm declines still runs —
// and still raises — on the engine.
func TestRunBehaviorBodyStepless(t *testing.T) {
	r := w9Reg(t)
	body := []Value{NewInteger(1), NewString("T")}
	got, err := runBehaviorBody(r, body)
	if err != nil || len(got) != 2 || got[1].String() != body[1].String() {
		t.Fatalf("stepless body: got %v, %v", got, err)
	}
	got[0] = NewInteger(99)
	if n, _ := body[0].AsConcreteInteger(); n != 1 {
		t.Error("the stepless answer aliases the installed body")
	}
	// Declined: a word runs on the engine, which raises its error.
	if _, err := runBehaviorBody(r, []Value{NewWord("drop")}); err == nil {
		t.Error("`drop` on an empty behaviour stack must raise on the engine")
	}
	// Declined: a word that computes answers from the engine.
	got, err = runBehaviorBody(r, []Value{NewInteger(1), NewWord("add"), NewInteger(2)})
	if err != nil || len(got) != 1 || got[0].String() != "3" {
		t.Errorf("computed body: got %v, %v", got, err)
	}
}
