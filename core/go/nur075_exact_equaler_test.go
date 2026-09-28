package core

import (
	"errors"
	"testing"
)

// ── exact equality (NUR075) ─────────────────────────────────────────

type capEqBehavior struct {
	defaultBehavior
	equal   bool
	decline bool
	fail    bool
}

func (b capEqBehavior) ExactEqualValues(Value, Value) (bool, error) {
	switch {
	case b.decline:
		return false, ErrNoExactEqualer
	case b.fail:
		return true, errors.New("body blew up")
	}
	return b.equal, nil
}

// TestExactEqualCapabilityAnswersAtTheTerminalPoint pins NUR075's close: `eq`
// is extensible per type on the same terms as `deq`. The pair is the one the
// DeepEqualer tests use — values of an ad-hoc Ideal subtype whose payload has
// no pointer to carry an identity — which reaches BOTH terminals, so the two
// capabilities reach the same values.
func TestExactEqualCapabilityAnswersAtTheTerminalPoint(t *testing.T) {
	plainA, plainB := capDeqPair(t, "PlainEqIdeal", nil)
	if ExactEqual(plainA, plainB) {
		t.Fatal("fixture assumption broken: the pair must be eq-false without a capability")
	}
	a, b := capDeqPair(t, "SameThingIdeal", capEqBehavior{equal: true})
	if !ExactEqual(a, b) {
		t.Error("an installed ExactEqualer must answer for its own values")
	}
	if DeepEqual(a, b) {
		t.Error("an ExactEqualer answers eq only — deq keeps its own capability")
	}
	no, no2 := capDeqPair(t, "NotSameIdeal", capEqBehavior{equal: false})
	if ExactEqual(no, no2) {
		t.Error("an ExactEqualer returning false must report not-eq")
	}
}

func TestExactEqualCapabilityDeclineAndFailure(t *testing.T) {
	a, b := capDeqPair(t, "DeclinesEqIdeal", capEqBehavior{decline: true})
	if ExactEqual(a, b) {
		t.Error("a declining ExactEqualer must fall through to the terminal false")
	}
	c, d := capDeqPair(t, "FailsEqIdeal", capEqBehavior{fail: true, equal: true})
	if ExactEqual(c, d) {
		t.Error("an erroring ExactEqualer must fall through, not impose its would-be answer")
	}
}

// TestExactEqualCapabilityIsAdditive is the negative half: the walk sits at
// the terminal point, so no kernel arm above it can be overridden.
func TestExactEqualCapabilityIsAdditive(t *testing.T) {
	lying := capType(t, "LyingEqInteger", TInteger, capEqBehavior{equal: true})
	a, b := NewInteger(1), NewInteger(2)
	a.Parent, b.Parent = lying, lying
	if ExactEqual(a, b) {
		t.Error("the capability must not reach past the scalar arm — 1 eq 2 is false")
	}
	c, d := NewInteger(3), NewInteger(3)
	c.Parent, d.Parent = lying, lying
	if !ExactEqual(c, d) {
		t.Error("3 eq 3 must stay true")
	}
	l := NewList([]Value{NewInteger(1)})
	if !ExactEqual(l, l) || ExactEqual(l, NewList([]Value{NewInteger(1)})) {
		t.Error("list identity regressed")
	}
}
