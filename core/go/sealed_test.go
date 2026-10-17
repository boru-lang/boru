package core

import (
	"strings"
	"testing"
)

// Core-suite pins for the sealed literal region (sealed.go): a pending
// container literal's elements evaluate on the running tape as a
// one-iteration driven loop, with the sub-engine protocol's observable
// contract — the context layer, a step budget of the region's own, the
// residual sweep, the NUR358 escape, the fault attribution, the unmatched-
// paren reports — and no interpreter entry of their own.

// sealedLoopIn is a hand-built counted loop region — mark, body, move —
// the body tokens between, with its index name installed.
func sealedLoopIn(r *Registry, name string, body ...Value) []Value {
	cont := &Loop{Registry: r, IterName: name, Current: 0, End: 1_000_000, Step: 1, Body: body, Results: []Value{NewInteger(42)}}
	InstallDef(r, name, NewInteger(0))
	out := []Value{NewMark("s" + name)}
	out = append(out, body...)
	return append(out, NewMoveCont("s"+name, "for loop", cont))
}

func TestSealedLiteralEscapes(t *testing.T) {
	// A loop inside the literal takes the signal in place: the literal is
	// the loop's results.
	r := loopReg(t)
	prog := []Value{NewEvalList(sealedLoopIn(r, "si", NewInteger(9), NewWord("nbrk"))), NewWord("nidl")}
	out, err := NewTop(r).Run(prog)
	if err != nil || renderAll(out) != "[42]" || r.FlowCtrl != FlowNone {
		t.Fatalf("Run = %s / %v (flow %v), want [42]", renderAll(out), err, r.FlowCtrl)
	}
	// A driven loop live inside the literal passes the signal through, out
	// of the literal (NUR358): the region is abandoned whole, the loop's
	// context layer with it, and the run raises.
	r = loopReg(t)
	depth := r.Contexts.Depth()
	wantFlowError(t, r, []Value{NewEvalList([]Value{NewWord("tl"), NewEvalList([]Value{NewWord("nbrk")}), NewEvalList([]Value{NewInteger(1), NewInteger(2)})}), NewWord("nidl")}, "break")
	if r.Contexts.Depth() != depth {
		t.Fatalf("context layers after the escape: %d, want %d", r.Contexts.Depth(), depth)
	}
}

func TestSealedLiteralFaults(t *testing.T) {
	// A fault inside a driven loop inside the literal carries the loop's
	// attribution exactly once, and the trace is told at the raise, with
	// the region still on the tape.
	r := loopReg(t)
	depth := r.Contexts.Depth()
	var faults []string
	e := NewTop(r)
	e.SetTrace(func(step, _ int, stack []Value, note string) {
		if step == -1 {
			faults = append(faults, note+" @ "+renderAll(stack))
		}
	})
	_, err := e.Run([]Value{NewEvalList([]Value{NewWord("tl"), NewEvalList([]Value{NewWord("nosuch")}), NewEvalList([]Value{NewInteger(1)})}), NewWord("nidl")})
	if err == nil || strings.Count(err.Error(), "tl: element 0:") != 1 || !strings.Contains(err.Error(), "nosuch") {
		t.Fatalf("err = %v, want the loop's attribution once", err)
	}
	if len(faults) != 2 || !strings.Contains(faults[0], "fault: [boru/undefined_word]") || !strings.Contains(faults[0], "list literal") {
		t.Fatalf("fault notes = %q, want the region's raise then the run's", faults)
	}
	if r.Contexts.Depth() != depth {
		t.Fatalf("context layers after the fault: %d, want %d", r.Contexts.Depth(), depth)
	}
	// A counted loop live inside the literal at the fault loses its index.
	r = loopReg(t)
	_, err = NewTop(r).Run([]Value{NewEvalList(sealedLoopIn(r, "sj", NewWord("nosuch"))), NewWord("nidl")})
	if err == nil || !strings.Contains(err.Error(), "nosuch") || r.Defs.Depth("sj") != 0 {
		t.Fatalf("err = %v (sj depth %d), want the fault with the index uninstalled", err, r.Defs.Depth("sj"))
	}
	// The region's step budget: a loop that never ends inside the literal
	// trips the evaluation limit, and the tape is left as it stood.
	r = loopReg(t)
	e = NewTop(r)
	e.stepLimit = 40
	_, err = e.Run([]Value{NewEvalList(sealedLoopIn(r, "sk", NewInteger(1))), NewWord("nidl")})
	if be, ok := err.(*BoruError); !ok || be.Code != "evaluation_limit" || e.sealDepth != 0 {
		t.Fatalf("err = %v (sealDepth %d), want evaluation_limit", err, e.sealDepth)
	}
	// The tape ceiling: a literal the tape cannot hold.
	r = loopReg(t)
	r.TapeConfig = TapeConfig{InitialSize: 32, MaxGrows: 1, GrowthFactor: 2.0}
	wide := make([]Value, 100)
	for i := range wide {
		wide[i] = NewInteger(1)
	}
	wide[1] = NewWord("cdub") // one stepping element: the literal takes a region
	_, err = NewTop(r).Run([]Value{NewEvalList(wide), NewWord("nidl")})
	if be, ok := err.(*BoruError); !ok || be.Code != "tape_exhausted" {
		t.Fatalf("err = %v, want tape_exhausted", err)
	}
	// Unbalanced parens the parser never writes: a stray close meets the
	// seal, an unmatched open survives the region, a token that steps past
	// the region leaves its seal open — each the elements' own run's report.
	for name, lit := range map[string]Value{
		"stray close":   NewEvalList([]Value{NewInteger(1), NewCloseParen()}),
		"open survives": NewEvalList([]Value{NewOpenParen(), NewInteger(1), NewInteger(2)}),
		"steps past":    NewEvalList([]Value{NewLoopOpen(&Loop{}, 99), NewInteger(1)}),
	} {
		want := "unmatched opening parenthesis"
		if name == "stray close" {
			want = "unmatched closing parenthesis"
		}
		r := loopReg(t)
		_, err := NewTop(r).Run([]Value{lit, NewWord("nidl")})
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: err = %v, want %s", name, err, want)
		}
		if r.Contexts.Depth() != depth {
			t.Errorf("%s: context layers %d, want %d", name, r.Contexts.Depth(), depth)
		}
	}
	// A loop region's close paren at the tape's very end has no move to
	// fire: the same stray-close report.
	_, err = NewTop(loopReg(t)).Run([]Value{NewLoopOpen(&Loop{}, 0), NewInteger(1), NewCloseParen()})
	if err == nil || !strings.Contains(err.Error(), "unmatched closing parenthesis") {
		t.Fatalf("err = %v, want unmatched closing parenthesis", err)
	}
}

// TestSealedLiteralHoldsTheDispatchState: the dispatch holding the literal
// keeps its per-dispatch state across the region — its match's resolved
// stack while a pattern evaluates a member that dispatches (stack and
// forward operands), and a tail call inside the literal replaces its frame
// even under a forward paren group's evaluation.
func TestSealedLiteralHoldsTheDispatchState(t *testing.T) {
	r := loopReg(t)
	intPat, strPat := NewOrderedMap(), NewOrderedMap()
	intPat.Set("f", NewTypeLiteral(TInteger))
	strPat.Set("f", NewTypeLiteral(TString))
	answer := func(s string) *GoImpl {
		return Go(func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			return []Value{NewString(s)}, nil
		})
	}
	r.RegisterNativeFunc(NativeFunc{Name: "npmf", Signatures: []Signature{
		{Args: []*Type{TMap}, Patterns: map[int]Value{0: NewMap(intPat)}, Impl: answer("int"), Returns: []*Type{TString}, BarrierPos: 1},
		{Args: []*Type{TMap}, Patterns: map[int]Value{0: NewMap(strPat)}, Impl: answer("str"), Returns: []*Type{TString}, BarrierPos: 1},
	}})
	// nidlf is nidl collecting its operand forward.
	same := Go(func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		return []Value{args[0]}, nil
	})
	forwardList := func(r *Registry) {
		r.RegisterNativeFunc(NativeFunc{Name: "nidlf", Signatures: []Signature{{Args: []*Type{TList}, Impl: same, Returns: []*Type{TList}, BarrierPos: 1}}})
	}
	lit := func() Value { return evalMap(NewString("f"), group(NewInteger(1), NewWord("cadd"), NewInteger(1))) }
	out, err := NewTop(r).Run([]Value{lit(), NewWord("npm"), NewWord("npmf"), lit()})
	if err != nil || renderAll(out) != "'int' | 'int'" {
		t.Fatalf("Run = %s / %v, want 'int' | 'int'", renderAll(out), err)
	}
	// TCO inside a literal consumed under a forward paren group's
	// evaluation: the tail loop runs in constant space and the step limit
	// names the resource (fn_frame_taxonomy_test's taxonomy).
	r = loopReg(t)
	forwardList(r)
	r.TapeConfig = TapeConfig{InitialSize: 512, MaxGrows: 1, GrowthFactor: 2.0}
	InstallFnDef(r, "spin", FnDefInfo{Signatures: []Signature{{
		Params: []FnParam{{Name: "n", Type: TInteger}}, Returns: []*Type{TInteger},
		Impl: Boru([]Value{NewOpenParen(), NewWord("spin"), NewWord("n"), NewCloseParen()}), BarrierPos: BarrierAllForward,
	}}})
	e := NewTop(r)
	e.stepLimit = 50_000
	_, err = e.Run([]Value{NewWord("nidlf"), group(NewWord("nidlf"), NewEvalList([]Value{NewWord("spin"), NewInteger(0)}))})
	if be, ok := err.(*BoruError); !ok || be.Code != "evaluation_limit" || r.TCO.Replaced == 0 {
		t.Fatalf("err = %v (replaced %d), want evaluation_limit with frames replaced", err, r.TCO.Replaced)
	}
}

// TestSealedSteplessStripsAscription: the stepless fast path stores a
// literal's elements as the stepped path stores them — a dispatch ascription
// a Go-built element carries is consumed, never kept in the container.
func TestSealedSteplessStripsAscription(t *testing.T) {
	r := loopReg(t)
	v := NewInteger(1)
	v.SetAscribed(TString)
	out, err := NewTop(r).Run([]Value{NewEvalList([]Value{v, NewInteger(2)}), NewWord("nidl")})
	if err != nil || len(out) != 1 {
		t.Fatalf("Run = %s / %v", renderAll(out), err)
	}
	lst, _ := AsList(out[0])
	if lst.Len() != 2 || addrOf(lst.Get(0)).AscribedType() != nil {
		t.Fatalf("the stored element keeps its ascription: %v", addrOf(lst.Get(0)).AscribedType())
	}
}
