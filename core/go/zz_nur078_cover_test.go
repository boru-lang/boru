package core

// Core-suite pins for NUR078's two engine helpers: a reach group's `/v`
// marker consumed by the forward scan (it qualifies the group's value and
// is never an argument), and a modifier sugar's reach operand placed in a
// paren (the modifier transforms the function VALUE; a dot read calls).
// Programs are hand-built token slices (core has no parser), positive and
// negative paired.

import (
	"testing"
)

// nur078Registry carries `cref2`: (Function, Integer) -> the Integer, with
// the Function slot's quote state and name reported back through got.
func nur078Registry(t *testing.T, got *Value) *Registry {
	t.Helper()
	return covRegistry(t, func(r *Registry) {
		r.RegisterNativeFunc(NativeFunc{
			Name: "cref2",
			Signatures: []Signature{{
				Args: []*Type{TFunction, TInteger},
				Impl: Go(func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
					*got = args[0]
					return []Value{args[1]}, nil
				}),
				Returns:    []*Type{TInteger},
				BarrierPos: BarrierAllForward,
			}},
		})
	})
}

// A reach-lowered group, its `/v` marker, then a written Integer: the scan
// consumes the marker (it is no argument), so cref2 collects the fn and the
// 5, and the fn arrives UNQUOTED — the reference, delivered as `inc/v` is.
func TestNUR078ReachMarkerIsNoArgument(t *testing.T) {
	var got Value
	r := nur078Registry(t, &got)
	fnv := namedFnVal("nm", []FnParam{{Name: "n", Type: TInteger}}, []*Type{TInteger},
		parenBody(NewWord("cadd"), NewWord("n"), NewWord("n")))
	op := NewOpenParen()
	op.ReachGroup = true
	out, err := NewTop(r).Run([]Value{
		NewWord("cref2"), op, fnv, NewCloseParen(),
		NewDispatchMod(DispatchModInfo{Val: true}), NewInteger(5),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if renderAll(out) != "5" {
		t.Errorf("cref2 over the marked group: %s, want 5", renderAll(out))
	}
	if fd, ok := got.Data.(FnDefInfo); !ok || fd.Name != "nm" || got.Quoted || got.ReachGroup {
		t.Errorf("the reference must arrive unquoted and untagged: %v (quoted=%v tagged=%v)", got, got.Quoted, got.ReachGroup)
	}

	// Negative: without the marker the group's fn WOULD CLAIM the 5 — a
	// call head whatever cref2's slot type (NUR078) — so cref2 is left
	// with nothing and raises.
	r2 := nur078Registry(t, &got)
	op2 := NewOpenParen()
	op2.ReachGroup = true
	if _, err := NewTop(r2).Run([]Value{NewWord("cref2"), op2, fnv, NewCloseParen(), NewInteger(5)}); err == nil {
		t.Error("an unmarked member read that would claim the next token must be a call head")
	}
}

// placeModifierOperand wraps a reach following a function-modifier sugar in
// a paren; anything else is left exactly as it was.
func TestNUR078PlaceModifierOperand(t *testing.T) {
	reach := NewReach(ReachInfo{Receiver: []Value{NewWord("m")}, Segments: []ReachSeg{{KeyLit: NewAtom("a")}}, Eval: true})
	for _, kind := range []SugarKind{SugarUsurp, SugarStackArgs, SugarForwardArgs, SugarForceArity} {
		tape := NewTape([]Value{NewWord("usurp"), reach}, StackHeadroom)
		placeModifierOperand(tape, kind, 1)
		items, err := AsParenExpr(tape.At(1))
		if err != nil || len(items) != 1 || !IsReach(items[0]) {
			t.Errorf("kind %v: the reach operand must be wrapped in a paren, got %v", kind, tape.At(1))
		}
	}
	// Negatives: another sugar kind, a non-reach operand, an index past
	// the tape, an inert (non-evaluating) reach.
	tape := NewTape([]Value{reach}, StackHeadroom)
	placeModifierOperand(tape, SugarAngle, 0)
	if !IsReach(tape.At(0)) {
		t.Error("a non-modifier sugar must leave the reach alone")
	}
	placeModifierOperand(tape, SugarUsurp, 1)
	if tape.Len() != 1 || !IsReach(tape.At(0)) {
		t.Error("an index past the tape must change nothing")
	}
	lit := NewTape([]Value{NewInteger(3)}, StackHeadroom)
	placeModifierOperand(lit, SugarUsurp, 0)
	if v, _ := AsInteger(lit.At(0)); v != 3 {
		t.Error("a non-reach operand must be left as it was")
	}
	inert := NewTape([]Value{NewReachFromKeys(NewWord("m"), []Value{NewAtom("a")})}, StackHeadroom)
	placeModifierOperand(inert, SugarUsurp, 0)
	if !IsReach(inert.At(0)) {
		t.Error("an inert reach is data already and must be left alone")
	}
}
