package core

import (
	"errors"
	"testing"
)

// A condition region's LAST value decides a conditional loop and an `if`;
// a region that leaves NO value is the error. Whether it left one is read
// off the region. It was read off the value — "its Parent is nil" — and a
// root type literal has no lattice parent, so `Any`, `None` and `Never`
// standing as the condition were each reported as no value at all
// (NUR399). They are values like any other type literal, and the one
// truthiness rule reads them: `None` is false, `Any` and `Never` are true.
func TestARootTypeLiteralIsAConditionValue(t *testing.T) {
	for _, c := range []struct{ expr, want string }{
		// None is false: the loop runs no round
		{"run whileq [q None ] [q 7 ]", ""},
		// Any and Never are true: one round, then the binding beneath
		// (false) ends the loop
		{"run def c false ; def c Any ; whileq [q c ] [q undef c 7 ]", "7"},
		{"run def c false ; def c Never ; whileq [q c ] [q undef c 7 ]", "7"},
		// the other type literals were never affected
		{"run whileq [q Integer ] [q 7 ]", ""},
		{"run def c false ; def c String ; whileq [q c ] [q undef c 7 ]", "7"},
	} {
		if got := evalCoreSpec(t, coreSpecRegistry(t), c.expr); got != c.want {
			t.Errorf("%s\n   got  %q\n   want %q", c.expr, got, c.want)
		}
	}
}

// A region that leaves nothing is still the error it was.
func TestAnEmptyConditionIsStillRefused(t *testing.T) {
	if got := evalCoreSpec(t, coreSpecRegistry(t), "run whileq [q ] [q 7 ]"); got != "ERROR:runtime_error" {
		t.Errorf("an empty while condition: got %q, want ERROR:runtime_error", got)
	}
	_, err := runIf(t)
	var be *BoruError
	if !errors.As(err, &be) || be.Code != "runtime_error" {
		t.Errorf("an empty if condition: got %v, want runtime_error", err)
	}
}

// runIf steps an `if` continuation — `mark cond… move` — whose branches are
// 1 and 2, as basic's `if` word lays it out.
func runIf(t *testing.T, cond ...Value) ([]Value, error) {
	t.Helper()
	id := NextMarkID()
	toks := append([]Value{NewMark(id)}, cond...)
	toks = append(toks, NewMoveIf(id, "if", &IfCont{
		Then: []Value{NewInteger(1)},
		Else: []Value{NewInteger(2)},
	}))
	return NewTop(coreSpecRegistry(t)).Run(toks)
}

func TestARootTypeLiteralIsAnIfCondition(t *testing.T) {
	for _, c := range []struct {
		name string
		cond Value
		want string
	}{
		{"Any", NewTypeLiteral(TAny), "1"},
		{"Never", NewTypeLiteral(TNever), "1"},
		{"None", NewTypeLiteral(TNone), "2"},
		// unaffected neighbours
		{"Integer", NewTypeLiteral(TInteger), "2"},
		{"String", NewTypeLiteral(TString), "1"},
		{"true", NewBoolean(true), "1"},
		{"the none value", NewNone(), "2"},
	} {
		out, err := runIf(t, c.cond)
		if err != nil || len(out) != 1 || CanonValue(out[0]) != c.want {
			t.Errorf("if [%s]: got %v, %v; want %s", c.name, out, err, c.want)
		}
	}
}
