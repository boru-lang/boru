package native

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// Seam coverage for the three capability slots' fall-through arms —
// delegate-to-prev, decline, re-entrancy, and the body-result rejections.
// The slots are driven through the wrapper directly because most of these
// arms are unreachable from boru source: a body that returns the wrong
// type is refused at INSTALL time by the validator, so the run-time
// rejection exists only as the belt to that braces.

// bsPrev is a previous-Behavior stub that implements all three
// capabilities, so the delegate arms have something to delegate to.
type bsPrev struct{ core.TypeBehavior }

func (bsPrev) Truthy(core.Value) (bool, error)                { return true, nil }
func (bsPrev) DeepEqualValues(a, b core.Value) (bool, error)  { return true, nil }
func (bsPrev) ExactEqualValues(a, b core.Value) (bool, error) { return true, nil }
func (bsPrev) Size(core.Value) int                            { return 7 }

// bsBare is a previous-Behavior stub implementing NONE of them.
type bsBare struct{ core.TypeBehavior }

func bsWrapper(t *testing.T, prev core.TypeBehavior) *userBehavior {
	t.Helper()
	return &userBehavior{prev: prev, registry: w9Reg(t), typeName: "Seam"}
}

func TestBehaveTruthySeam(t *testing.T) {
	// No body, prev has the capability → delegate.
	u := bsWrapper(t, bsPrev{core.DefaultBehavior})
	got, err := u.Truthy(NewInteger(0))
	if err != nil || !got {
		t.Errorf("delegate arm: got %v, %v; want true, nil", got, err)
	}

	// No body, prev has not → decline so the cascade answers.
	u = bsWrapper(t, bsBare{core.DefaultBehavior})
	if _, err := u.Truthy(NewInteger(0)); err != core.ErrNoTruther {
		t.Errorf("decline arm: err = %v, want ErrNoTruther", err)
	}

	// Re-entrancy: a body that puts the value back in a boolean position
	// must not loop — the guard declines on the inner call.
	u.truthyBody = []Value{NewBoolean(true)}
	u.inTruthy = true
	if _, err := u.Truthy(NewInteger(0)); err != core.ErrNoTruther {
		t.Errorf("re-entrancy arm: err = %v, want ErrNoTruther", err)
	}
	u.inTruthy = false

	// Success.
	if got, err := u.Truthy(NewInteger(0)); err != nil || !got {
		t.Errorf("body arm: got %v, %v; want true, nil", got, err)
	}

	// A body whose result is not a Boolean is refused at run time too.
	u.truthyBody = []Value{NewInteger(1)}
	if _, err := u.Truthy(NewInteger(0)); err == nil ||
		!strings.Contains(err.Error(), "must return Boolean") {
		t.Errorf("wrong-result arm: err = %v", err)
	}

	// A body that fails to run propagates its error.
	u.truthyBody = []Value{NewWord("no-such-word-here")}
	if _, err := u.Truthy(NewInteger(0)); err == nil {
		t.Error("a failing body must propagate its error")
	}
}

func TestBehaveDeqSeam(t *testing.T) {
	a, b := NewInteger(1), NewInteger(2)

	u := bsWrapper(t, bsPrev{core.DefaultBehavior})
	if got, err := u.DeepEqualValues(a, b); err != nil || !got {
		t.Errorf("delegate arm: got %v, %v; want true, nil", got, err)
	}

	u = bsWrapper(t, bsBare{core.DefaultBehavior})
	if _, err := u.DeepEqualValues(a, b); err != core.ErrNoDeepEqualer {
		t.Errorf("decline arm: err = %v, want ErrNoDeepEqualer", err)
	}

	u.deqBody = []Value{NewBoolean(true)}
	u.inDeq = true
	if _, err := u.DeepEqualValues(a, b); err != core.ErrNoDeepEqualer {
		t.Errorf("re-entrancy arm: err = %v, want ErrNoDeepEqualer", err)
	}
	u.inDeq = false

	if got, err := u.DeepEqualValues(a, b); err != nil || !got {
		t.Errorf("body arm: got %v, %v; want true, nil", got, err)
	}

	u.deqBody = []Value{NewInteger(1)}
	if _, err := u.DeepEqualValues(a, b); err == nil ||
		!strings.Contains(err.Error(), "must return Boolean") {
		t.Errorf("wrong-result arm: err = %v", err)
	}

	u.deqBody = []Value{NewWord("no-such-word-here")}
	if _, err := u.DeepEqualValues(a, b); err == nil {
		t.Error("a failing body must propagate its error")
	}

	// An empty result stack is its own arm.
	u.deqBody = []Value{}
	u.deqBody = append(u.deqBody, NewBoolean(true))
	u.deqBody = u.deqBody[:0]
	if _, err := u.DeepEqualValues(a, b); err != core.ErrNoDeepEqualer {
		t.Errorf("an emptied body falls back to the decline arm, got %v", err)
	}

	// No registry: the body cannot run at all.
	u = &userBehavior{prev: bsBare{core.DefaultBehavior}, typeName: "Seam",
		deqBody: []Value{NewBoolean(true)}}
	if _, err := u.DeepEqualValues(a, b); err == nil ||
		!strings.Contains(err.Error(), "no registry") {
		t.Errorf("nil-registry arm: err = %v", err)
	}
}

// TestBehaveEqSeam is deq's seam test for the reference half (NUR075): the
// eq slot delegates, declines, guards re-entry and reads its verdict exactly
// as the deq slot does.
func TestBehaveEqSeam(t *testing.T) {
	a, b := NewInteger(1), NewInteger(2)

	u := bsWrapper(t, bsPrev{core.DefaultBehavior})
	if got, err := u.ExactEqualValues(a, b); err != nil || !got {
		t.Errorf("delegate arm: got %v, %v; want true, nil", got, err)
	}

	u = bsWrapper(t, bsBare{core.DefaultBehavior})
	if _, err := u.ExactEqualValues(a, b); err != core.ErrNoExactEqualer {
		t.Errorf("decline arm: err = %v, want ErrNoExactEqualer", err)
	}

	u.eqBody = []Value{NewBoolean(true)}
	u.inEq = true
	if _, err := u.ExactEqualValues(a, b); err != core.ErrNoExactEqualer {
		t.Errorf("re-entrancy arm: err = %v, want ErrNoExactEqualer", err)
	}
	u.inEq = false

	if got, err := u.ExactEqualValues(a, b); err != nil || !got {
		t.Errorf("body arm: got %v, %v; want true, nil", got, err)
	}

	u.eqBody = []Value{NewInteger(1)}
	if _, err := u.ExactEqualValues(a, b); err == nil ||
		!strings.Contains(err.Error(), "behave eq") || !strings.Contains(err.Error(), "must return Boolean") {
		t.Errorf("wrong-result arm: err = %v", err)
	}
}

func TestBehaveSizeSeam(t *testing.T) {
	v := NewInteger(3)

	// No body, prev has a Sizer → delegate.
	u := bsWrapper(t, bsPrev{core.DefaultBehavior})
	if got := u.Size(v); got != 7 {
		t.Errorf("delegate arm: got %d, want 7", got)
	}

	// No body, prev has none → 0, matching SizeOf's own no-Sizer answer.
	u = bsWrapper(t, bsBare{core.DefaultBehavior})
	if got := u.Size(v); got != 0 {
		t.Errorf("no-sizer arm: got %d, want 0", got)
	}

	// Re-entrancy.
	u.sizeBody = []Value{NewInteger(5)}
	u.inSize = true
	if got := u.Size(v); got != 0 {
		t.Errorf("re-entrancy arm: got %d, want 0", got)
	}
	u.inSize = false

	if got := u.Size(v); got != 5 {
		t.Errorf("body arm: got %d, want 5", got)
	}

	// A non-Integer result is 0 — Size is total and has no error channel.
	u.sizeBody = []Value{NewString("nope")}
	if got := u.Size(v); got != 0 {
		t.Errorf("wrong-result arm: got %d, want 0", got)
	}

	// A failing body is 0 for the same reason.
	u.sizeBody = []Value{NewWord("no-such-word-here")}
	if got := u.Size(v); got != 0 {
		t.Errorf("failing-body arm: got %d, want 0", got)
	}
}

// TestBehaveUnaryBodySeam drives runUnaryBody's own arms — the shared
// runner behind the truthy and size slots.
func TestBehaveUnaryBodySeam(t *testing.T) {
	u := &userBehavior{prev: bsBare{core.DefaultBehavior}, typeName: "Seam"}
	if _, err := u.runUnaryBody([]Value{NewInteger(1)}, NewInteger(0), "truthy"); err == nil ||
		!strings.Contains(err.Error(), "no registry") {
		t.Errorf("nil-registry arm: err = %v", err)
	}

	u.registry = w9Reg(t)
	// A body that leaves nothing on the stack.
	if _, err := u.runUnaryBody(nil, NewInteger(0), "size"); err == nil ||
		!strings.Contains(err.Error(), "produced no result") {
		t.Errorf("empty-result arm: err = %v", err)
	}
	// The slot name reaches the message, so a failure says which capability.
	_, err := u.runUnaryBody([]Value{NewWord("no-such-word-here")}, NewInteger(0), "size")
	if err == nil || !strings.Contains(err.Error(), "size") {
		t.Errorf("the slot name must appear in the error: %v", err)
	}
}

// TestBehaveValidatorNilTypeArms drives the validators directly with a
// nil-Type param — the boru surface can't produce one (ParseFnParams
// degrades an untyped param to Any, not nil), so the guard is reachable
// only from Go callers handing in a hand-built FnSig.
func TestBehaveValidatorNilTypeArms(t *testing.T) {
	unary := core.FnSig{Params: []core.FnParam{{}}, Returns: []*core.Type{TBoolean}}
	if _, err := validateTruthySig(unary); err == nil ||
		!strings.Contains(err.Error(), "must declare a type") {
		t.Errorf("truthy nil-type arm: err = %v", err)
	}
	unary.Returns = []*core.Type{TInteger}
	if _, err := validateSizeSig(unary); err == nil ||
		!strings.Contains(err.Error(), "must declare a type") {
		t.Errorf("size nil-type arm: err = %v", err)
	}
	binary := core.FnSig{
		Params:  []core.FnParam{{Type: TInteger}, {}},
		Returns: []*core.Type{TBoolean},
	}
	if _, err := validateDeqSig(binary); err == nil ||
		!strings.Contains(err.Error(), "must declare a type") {
		t.Errorf("deq nil-type arm: err = %v", err)
	}
}

// TestBehaveBodyResultEdgeArms drives the two remaining body-result
// arms: a deq body that runs but leaves an empty stack, and a size body
// whose top CONFORMS to Integer without carrying one (a bare type
// literal) so the accessor's own failure path reports 0.
func TestBehaveBodyResultEdgeArms(t *testing.T) {
	u := bsWrapper(t, bsBare{core.DefaultBehavior})

	// `1 drop` runs fine and produces nothing.
	u.deqBody = []Value{NewInteger(1), NewWord("drop")}
	if _, err := u.DeepEqualValues(NewInteger(1), NewInteger(2)); err == nil ||
		!strings.Contains(err.Error(), "produced no result") {
		t.Errorf("deq empty-result arm: err = %v", err)
	}

	// A bare `Integer` literal: Parent conforms, AsInteger refuses.
	u.sizeBody = []Value{NewTypeLiteral(TInteger)}
	if got := u.Size(NewInteger(3)); got != 0 {
		t.Errorf("size literal-top arm: got %d, want 0", got)
	}

	// A DepScalar over Integer (`Integer gt 5`) CONFORMS to Integer but
	// carries DepScalarInfo, so it passes the ConformsTo gate and fails
	// in AsInteger itself — the arm the literal above cannot reach.
	u.sizeBody = []Value{core.NewDepScalar(core.DepGT, NewInteger(5))}
	if got := u.Size(NewInteger(3)); got != 0 {
		t.Errorf("size depscalar-top arm: got %d, want 0", got)
	}
}
