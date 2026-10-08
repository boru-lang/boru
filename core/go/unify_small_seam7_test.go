package core

// Seam-7 (cluster A4): in-package unit tests for previously-unreached
// blocks in the small unify_* helpers, membership.go, member_behavior.go,
// policy_hook.go, flowctrl.go, capability.go and unify_error.go. All calls
// are direct (guard / error / opt-out arms) per design/TEST-SEAMS.10.md.

import "testing"

// --- capability.go --------------------------------------------------------

func TestS7CapabilitySetLazyInitStore(t *testing.T) {
	// A zero-value registry (store==nil) must lazily allocate on Set.
	c := &CapabilityRegistry{}
	if err := c.Set("k", 7); err != nil {
		t.Fatalf("Set on nil store: %v", err)
	}
	v, ok, err := c.Get("k")
	if err != nil || !ok || v != 7 {
		t.Errorf("Get after lazy-init Set = %v %v %v, want 7 true nil", v, ok, err)
	}
}

// --- flowctrl.go ----------------------------------------------------------

func TestS7FlowCtrlStringAll(t *testing.T) {
	cases := map[FlowCtrl]string{
		FlowNone:      "none",
		FlowBreak:     "break",
		FlowContinue:  "continue",
		FlowCtrl(200): "FlowCtrl(200)",
	}
	for f, want := range cases {
		if got := f.String(); got != want {
			t.Errorf("FlowCtrl(%d).String() = %q, want %q", uint8(f), got, want)
		}
	}
}

// --- unify_error.go -------------------------------------------------------

func TestS7UnifyErrorNilReceivers(t *testing.T) {
	var e *UnifyError
	if got := e.Error(); got != "" {
		t.Errorf("nil Error() = %q, want empty", got)
	}
	if got := e.withPath("x"); got != nil {
		t.Errorf("nil withPath() = %v, want nil", got)
	}
}

// --- unify_lca.go ---------------------------------------------------------

func TestS7DispatchUnifierNilDenoted(t *testing.T) {
	// A Value{} denotes no lattice type (nil Parent, empty ID): dispatch
	// must decline immediately.
	if _, _, ok := dispatchUnifier(Value{}, NewInteger(1), nil); ok {
		t.Error("dispatchUnifier with a nil-denoted operand must decline")
	}
}

func TestS7DispatchUnifierNilBehaviorInChain(t *testing.T) {
	// Both operands denote an ad-hoc type whose Behavior is nil: the walk
	// skips the nil-Behavior node and, finding no Unifier, declines.
	adhoc := &Type{tmeta: &typeMeta{Name: "S7Adhoc"}, ID: "S7Adhoc"}
	a := Value{Parent: adhoc, Data: IntPayload{N: 1}}
	b := Value{Parent: adhoc, Data: IntPayload{N: 2}}
	if _, _, ok := dispatchUnifier(a, b, nil); ok {
		t.Error("a nil-Behavior chain has no Unifier; dispatch must decline")
	}
}

// --- member_behavior.go ---------------------------------------------------

func TestS7MemberBehaviorFormatAndDelegate(t *testing.T) {
	b := MemberUnifier{member: func(Value) bool { return true }}
	if got := b.Format(NewInteger(3)); got != "3" {
		t.Errorf("MemberUnifier.Format(3) = %q, want 3", got)
	}
	b.FormatDelegate() // marker method, must not panic
}

// --- membership.go --------------------------------------------------------

// s7Comparer is a TypeBehavior that also satisfies Comparer, used to drive
// baseCompare's delegation arm.
type s7Comparer struct{}

func (s7Comparer) Match(v Value, t *Type) bool     { return DefaultBehavior.Match(v, t) }
func (s7Comparer) Format(v Value) string           { return DefaultBehavior.Format(v) }
func (s7Comparer) Equal(a, b Value) bool           { return DefaultBehavior.Equal(a, b) }
func (s7Comparer) Compare(a, b Value) (int, error) { return 42, nil }

func TestS7BaseCompareDelegatesToComparerPrev(t *testing.T) {
	got, err := baseCompare(s7Comparer{}, NewInteger(1), NewInteger(2))
	if err != nil || got != 42 {
		t.Errorf("baseCompare with Comparer prev = %d (%v), want 42 nil", got, err)
	}
}

func TestS7MatchMembershipCarrierClosurePayload(t *testing.T) {
	// A non-concrete carrier whose payload is an FnDefInfo is payload-
	// decidable: the predicate is consulted directly.
	v := Value{Parent: TFunction, Carrier: true, Data: FnDefInfo{}}
	if IsConcrete(v) {
		t.Fatal("test premise: carrier is not concrete")
	}
	called := false
	got := matchMembership(v, TFunction, nil, func(Value) bool { called = true; return true })
	if !got || !called {
		t.Errorf("carrier closure payload must reach the predicate (got=%v called=%v)", got, called)
	}
}

func TestS7UnifyMembershipBothConcrete(t *testing.T) {
	// Both concrete -> value-vs-value, settled structurally, predicate
	// never consulted.
	called := false
	out, uerr := unifyMembership(NewInteger(5), NewInteger(5), "t", func(v Value) (Value, bool, error) {
		called = true
		return v, true, nil
	})
	if uerr != nil {
		t.Fatalf("both-concrete unify: %v", uerr)
	}
	if called {
		t.Error("predicate must not run when both operands are concrete")
	}
	if n, _ := AsInteger(out); n != 5 {
		t.Errorf("both-concrete unify result = %v, want 5", out)
	}
}

func TestS7UnifyMembershipAdmitError(t *testing.T) {
	// Exactly one concrete operand and the oracle errors: the error is
	// wrapped with the type name.
	abstract := NewCarrier(TInteger)
	_, uerr := unifyMembership(NewInteger(1), abstract, "mytype", func(Value) (Value, bool, error) {
		return Value{}, false, errS7Oracle
	})
	if uerr == nil {
		t.Fatal("expected an error from the oracle")
	}
	if uerr.Reason == "" {
		t.Error("wrapped error must carry a reason")
	}
}

var errS7Oracle = &s7Err{}

type s7Err struct{}

func (*s7Err) Error() string { return "oracle boom" }

// --- unify_fold.go --------------------------------------------------------

func TestS7UnifyFlexLiteralBothLiterals(t *testing.T) {
	a := NewTypeLiteral(TInteger)
	b := NewTypeLiteral(TInteger)
	out, handled, uerr := unifyFlexLiteral(a, ShapeTypeLiteral, b, ShapeTypeLiteral, TInteger,
		func(Value) bool { return false }, "int")
	if !handled || uerr != nil {
		t.Fatalf("both-literal flex arm: handled=%v err=%v", handled, uerr)
	}
	if !out.Equal(TInteger) {
		t.Errorf("both-literal flex result = %v, want the Integer literal", out)
	}
}

// --- unify_fnsig.go -------------------------------------------------------

func TestS7UnifyFnUndefShapeUndefOnRight(t *testing.T) {
	// sa != ShapeFnUndef takes the else branch (undef on the right); a
	// non-function other side then fails.
	a := NewInteger(1)
	b := NewInteger(2)
	if _, uerr := unifyFnUndefShape(a, ShapeScalar, b, ShapeFnUndef, nil); uerr == nil {
		t.Error("FnUndef vs non-function must fail to unify")
	}
}

// --- unify_negation.go / unify_refine.go ----------------------------------

func TestS7NegationUnifierMatchBareLiteral(t *testing.T) {
	n := &NegationUnifier{
		behaviorWrapper: behaviorWrapper{prev: DefaultBehavior},
		inner:           NewTypeLiteral(TString),
		typeName:        "NotStr",
	}
	// A bare type literal passes through to the prev/default walk (does
	// not panic); the boolean is the default's verdict.
	_ = n.Match(NewTypeLiteral(TInteger), TInteger)
}

func TestS7BareRefineUnifierMatchBareLiteral(t *testing.T) {
	adhoc := &Type{tmeta: &typeMeta{Name: "S7Pos"}, ID: "S7Pos", Parent: TInteger}
	b := &bareRefineUnifier{
		behaviorWrapper: behaviorWrapper{prev: DefaultBehavior},
		typeName:        "S7Pos",
	}
	// Bare literal -> prev/default walk (does not panic).
	_ = b.Match(NewTypeLiteral(TInteger), adhoc)
}

// --- unify_predicate.go ---------------------------------------------------

func TestS7PredicateUnifierNilRegistry(t *testing.T) {
	p := &PredicateUnifier{
		behaviorWrapper: behaviorWrapper{prev: DefaultBehavior},
		registry:        nil,
		constraint:      Value{},
		typeName:        "P",
	}
	// Match with a concrete value and nil registry -> default-walk fallback.
	_ = p.Match(NewInteger(1), TInteger)
	// Unify with exactly one concrete operand and nil registry -> error.
	abstract := NewCarrier(TInteger)
	if _, uerr := p.Unify(NewInteger(1), abstract, nil); uerr == nil {
		t.Error("predicateUnifier.Unify with nil registry must fail")
	}
}

// --- unify_dep.go ---------------------------------------------------------

func TestS7UnifyDepScalarPayloadErrors(t *testing.T) {
	// Both DepScalar shape, same denoted base, but the left payload is not
	// a DepScalarInfo -> "could not read ... on left".
	badLeft := Value{Parent: TInteger}
	badRight := Value{Parent: TInteger}
	if _, uerr := unifyDepScalar(badLeft, ShapeDepScalar, badRight, ShapeDepScalar); uerr == nil {
		t.Error("unreadable left DepScalar payload must fail")
	}

	// Left is a real DepScalar, right is same base but unreadable payload
	// -> "could not read ... on right".
	realDep := NewDepScalar(DepGT, NewInteger(10))
	rightBad := Value{Parent: denotedType(realDep)}
	if _, uerr := unifyDepScalar(realDep, ShapeDepScalar, rightBad, ShapeDepScalar); uerr == nil {
		t.Error("unreadable right DepScalar payload must fail")
	}

	// DepScalar vs concrete scalar, base conforms, but the dep payload is
	// unreadable -> "could not read DepScalar payload".
	depBad := Value{Parent: TInteger}
	conc := NewInteger(5)
	if _, uerr := unifyDepScalar(depBad, ShapeDepScalar, conc, ShapeScalar); uerr == nil {
		t.Error("unreadable dep payload in the concrete branch must fail")
	}
}

// --- policy_hook.go -------------------------------------------------------

func TestS7LookupWordCheckerNil(t *testing.T) {
	if got := LookupWordChecker(nil); got != nil {
		t.Errorf("LookupWordChecker(nil) = %v, want nil", got)
	}
}
