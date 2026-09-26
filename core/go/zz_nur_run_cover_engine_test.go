package core

// Core-suite pins for the NUR run's engine additions: the loop-scope
// unwinds (NUR204 / NUR201's loop twin), the in-region residual
// evaluation (NUR197), the positioned while-condition error (NUR130), the
// no-match window and anchors (NUR171 / NUR172), the fn-return parks on the
// foreign-registry paths (NUR191), the carrier bookkeeping for the
// compiler (NUR213 / NUR210), the dispatch-modifier probe (NUR212), the
// folded-fn queueing (NUR105) and the unit-scoped definite trap (NUR134).
// Programs are hand-built token slices (core has no parser) or hand-built
// tape states driving one step helper, positive and negative paired.

import (
	"errors"
	"strings"
	"testing"
)

// nrcEngineRegistry is a fresh registry carrying the handful of natives
// the engine pins below dispatch: a raising word, an Integer increment,
// and two parked fn-value factories (the first returns the increment
// alone, the second the increment plus a 10).
func nrcEngineRegistry(t *testing.T) *Registry {
	t.Helper()
	r := newTestRegistry(t)
	nrcRegisterNative(r, "nrc-boom", nil, []*Type{}, func([]Value) ([]Value, error) {
		return nil, errors.New("nrc boom")
	})
	nrcRegisterNative(r, "nrcinc", []*Type{TInteger}, []*Type{TInteger}, func(a []Value) ([]Value, error) {
		n, _ := AsInteger(a[0])
		return []Value{NewInteger(n + 1)}, nil
	})
	inc, ok := r.Defs.Top("nrcinc")
	if !ok {
		t.Fatal("nrcinc did not register")
	}
	nrcRegisterNative(r, "nrcmk", nil, []*Type{TFunction}, func([]Value) ([]Value, error) {
		return []Value{inc}, nil
	}, Park())
	nrcRegisterNative(r, "nrcmk2", nil, []*Type{TFunction, TInteger}, func([]Value) ([]Value, error) {
		return []Value{inc, NewInteger(10)}, nil
	}, Park())
	if err := r.Err(); err != nil {
		t.Fatalf("registration: %v", err)
	}
	return r
}

// --- popIterLevels / unwindLiveLoops (NUR204, NUR201) -------------------------------

// Pins popIterLevels over a recorded entry depth: an iteration boundary pops
// only the body's levels above the index, the loop's end pops the index level
// too (the pre-loop binding shows again), and a continuation with no recorded
// depth pops exactly one level.
func TestNurRunPopIterLevels(t *testing.T) {
	r := newTestRegistry(t)
	r.Defs.Push("i", NewInteger(100)) // pre-loop binding
	r.Defs.Push("i", NewInteger(0))   // the loop's index level
	r.Defs.Push("i", NewInteger(9))   // a body `def i 9`
	cont := &ForCont{Registry: r, IterName: "i", IterDepth: 2}

	popIterLevels(cont, false)
	if d := r.Defs.Depth("i"); d != 2 {
		t.Fatalf("an iteration boundary keeps the index level: depth %d, want 2", d)
	}
	r.Defs.Push("i", NewInteger(9))
	popIterLevels(cont, true)
	if v, _ := r.Defs.Top("i"); r.Defs.Depth("i") != 1 || v.String() != "100" {
		t.Fatalf("the loop's end must restore the pre-loop binding, got %v at depth %d", v, r.Defs.Depth("i"))
	}

	// Negative: no recorded depth — one level, whatever sits above it.
	r.Defs.Push("j", NewInteger(1))
	r.Defs.Push("j", NewInteger(2))
	popIterLevels(&ForCont{Registry: r, IterName: "j"}, true)
	if d := r.Defs.Depth("j"); d != 1 {
		t.Fatalf("an unrecorded depth pops one level: depth %d, want 1", d)
	}
}

// Pins that a run faulting inside a live `for` region (its mark stepped, its
// move not yet reached) uninstalls the loop's iterator, while a fault BEFORE
// the loop's mark has stepped leaves the binding alone.
func TestNurRunFaultUnwindsLiveLoopIterator(t *testing.T) {
	loop := func(r *Registry) *ForCont {
		return &ForCont{Registry: r, IterName: "i", Current: 0, End: 2, Step: 1, IterDepth: 1}
	}

	r := nrcEngineRegistry(t)
	r.Defs.Push("i", NewInteger(0)) // the loop's installed index
	_, err := NewTop(r).Run([]Value{
		NewMark("nrc-live"), NewWord("nrc-boom"), NewMoveCont("nrc-live", "for loop", loop(r)),
	})
	if err == nil || !strings.Contains(err.Error(), "nrc boom") {
		t.Fatalf("want the body's error, got %v", err)
	}
	if d := r.Defs.Depth("i"); d != 0 {
		t.Fatalf("a live loop's iterator must be uninstalled on the fault: depth %d", d)
	}

	// Negative: the fault precedes the mark — the loop never went live.
	r2 := nrcEngineRegistry(t)
	r2.Defs.Push("i", NewInteger(0))
	if _, err := NewTop(r2).Run([]Value{
		NewWord("nrc-boom"), NewMark("nrc-dead"), NewMoveCont("nrc-dead", "for loop", loop(r2)),
	}); err == nil {
		t.Fatal("want the error")
	}
	if d := r2.Defs.Depth("i"); d != 1 {
		t.Fatalf("a loop that never went live keeps its binding: depth %d", d)
	}
}

// --- collectLoopRegion (NUR197) --------------------------------------------------

// Pins that a `for` region's pending residual list evaluates IN the region,
// with the iterator still bound (the loop's own `i`, not an outer one), while
// a quoted list is collected as it stands; and that an evaluation error
// surfaces from the move.
func TestNurRunForRegionEvaluatesPendingResidual(t *testing.T) {
	r := nrcEngineRegistry(t)
	r.Defs.Push("i", NewInteger(100)) // an OUTER i, beneath the loop
	r.Defs.Push("i", NewInteger(7))   // the loop's index for this iteration
	cont := &ForCont{Registry: r, IterName: "i", Current: 0, End: 1, Step: 1, IterDepth: 2}
	quoted := NewEvalList([]Value{NewWord("i")})
	quoted.Quoted = true
	e := NewTop(r)
	e.Tape = NewTape([]Value{
		NewMark("m1"), NewEvalList([]Value{NewWord("i")}), quoted, NewMoveCont("m1", "for loop", cont),
	}, StackHeadroom)
	e.marks = map[string]bool{"m1": true}
	if err := e.stepMoveCont(0, 3, MoveInfo{To: "m1", Cont: cont}); err != nil {
		t.Fatalf("stepMoveCont: %v", err)
	}
	if got := renderAll(e.Tape.Snapshot()); got != "[7] | [word(i)]" {
		t.Fatalf("region results = %q, want the evaluated [7] beside the quoted [i]", got)
	}
	if v, _ := r.Defs.Top("i"); r.Defs.Depth("i") != 1 || v.String() != "100" {
		t.Fatalf("the finished loop must unbind its index and leave the outer i, got %v at depth %d", v, r.Defs.Depth("i"))
	}

	// Negative: the residual's evaluation raises — the move surfaces it.
	cont2 := &ForCont{Registry: r, IterName: "k", Current: 0, End: 1, Step: 1}
	e2 := NewTop(r)
	e2.Tape = NewTape([]Value{
		NewMark("m2"), NewEvalList([]Value{NewWord("nrc-boom")}), NewMoveCont("m2", "for loop", cont2),
	}, StackHeadroom)
	e2.marks = map[string]bool{"m2": true}
	if err := e2.stepMoveCont(0, 2, MoveInfo{To: "m2", Cont: cont2}); err == nil || !strings.Contains(err.Error(), "nrc boom") {
		t.Fatalf("want the residual's error from the for move, got %v", err)
	}
}

// Pins the while twin: a body region's pending residual evaluates at the
// move (its value collected), and an evaluation error surfaces from it.
func TestNurRunWhileBodyRegionEvaluatesPendingResidual(t *testing.T) {
	r := nrcEngineRegistry(t)
	r.Defs.Push("w", NewInteger(3))
	cont := whileCont(r, []Value{NewBoolean(false)}, []Value{NewInteger(1)})
	cont.WhileInBody = true
	e := NewTop(r)
	e.Tape = NewTape([]Value{NewMark("w1"), NewEvalList([]Value{NewWord("w")}), NewMoveCont("w1", "while loop", cont)}, StackHeadroom)
	e.marks = map[string]bool{"w1": true}
	if err := e.stepMoveCont(0, 2, MoveInfo{To: "w1", Cont: cont}); err != nil {
		t.Fatalf("while body move: %v", err)
	}
	if got := renderAll(cont.Results); got != "[3]" || cont.WhileInBody {
		t.Fatalf("body results = %q (in body: %v), want [3] and the condition phase next", got, cont.WhileInBody)
	}

	bad := whileCont(r, []Value{NewBoolean(false)}, []Value{NewInteger(1)})
	bad.WhileInBody = true
	e2 := NewTop(r)
	e2.Tape = NewTape([]Value{NewMark("w2"), NewEvalList([]Value{NewWord("nrc-boom")}), NewMoveCont("w2", "while loop", bad)}, StackHeadroom)
	e2.marks = map[string]bool{"w2": true}
	if err := e2.stepMoveCont(0, 2, MoveInfo{To: "w2", Cont: bad}); err == nil || !strings.Contains(err.Error(), "nrc boom") {
		t.Fatalf("want the residual's error from the while move, got %v", err)
	}
}

// Pins that a while condition producing no value raises AT the condition
// operand's recorded position (CondPos), where the unpositioned form keeps
// the pointer-derived report.
func TestNurRunWhileEmptyConditionAnchorsAtCondition(t *testing.T) {
	r := covRegistry(t, nil)
	cont := whileCont(r, []Value{}, []Value{NewInteger(1)})
	cont.CondPos = SrcPos{Row: 2, Col: 7}
	e := NewTop(r)
	e.Tape = NewTape([]Value{NewMark("c1"), NewMoveCont("c1", "while loop", cont)}, StackHeadroom)
	e.marks = map[string]bool{"c1": true}
	err := e.stepMoveCont(0, 1, MoveInfo{To: "c1", Cont: cont})
	var ae *BoruError
	if !errors.As(err, &ae) || ae.Row != 2 || ae.Col != 7 || !strings.Contains(ae.Detail, "condition produced no value") {
		t.Fatalf("want the no-value error at 2:7, got %v", err)
	}

	// Negative: no recorded condition position — not anchored there.
	plain := whileCont(r, []Value{}, []Value{NewInteger(1)})
	e2 := NewTop(r)
	e2.Tape = NewTape([]Value{NewMark("c2"), NewMoveCont("c2", "while loop", plain)}, StackHeadroom)
	e2.marks = map[string]bool{"c2": true}
	err = e2.stepMoveCont(0, 1, MoveInfo{To: "c2", Cont: plain})
	if !errors.As(err, &ae) || ae.Row == 2 {
		t.Fatalf("an unpositioned condition must not borrow a position, got %v", err)
	}
}

// --- the no-match report: window and anchor (NUR171 / NUR172) ------------------

// Pins that a poly no-match spec whose word carries no position borrows the
// first written operand that has one, keeps its own position when it has
// one, and stays unpositioned when no operand has one either.
func TestNurRunPolyNoMatchSpecBorrowsOperandPosition(t *testing.T) {
	fn := &FnDefInfo{Name: "nrcpoly", Signatures: []Signature{{Params: []FnParam{{Name: "s", Type: TString}}}}}
	positioned := NewInteger(5)
	positioned.ID = "nrc-poly-a"
	positioned.SetPos(SrcPos{Row: 1, Col: 1})

	p := polyNoMatchProbe{ok: true, written: []Value{positioned}, stackVals: []Value{positioned}}
	spec := p.Spec(fn, []Value{positioned})
	if spec == nil || spec.Pos.Row != 1 || spec.Pos.Col != 1 {
		t.Fatalf("an unpositioned word must borrow the operand's 1:1, got %+v", spec)
	}

	p.pos = SrcPos{Row: 9, Col: 3}
	if spec := p.Spec(fn, []Value{positioned}); spec == nil || spec.Pos.Row != 9 {
		t.Fatalf("the word's own position wins, got %+v", spec)
	}

	bare := NewInteger(6)
	bare.ID = "nrc-poly-b"
	q := polyNoMatchProbe{ok: true, written: []Value{bare}, stackVals: []Value{bare}}
	if spec := q.Spec(fn, []Value{bare}); spec == nil || spec.Pos.Row != 0 {
		t.Fatalf("no positioned operand: the spec stays unpositioned, got %+v", spec)
	}
}

// Pins attemptedWindowOver's bare-word rule: with no forward candidate, a
// bare word right after the failed word counts as the Atom a `/q` slot of
// some (non-fallback) overload would capture, positioned at the word; a
// `/v` word, or an fn with no quoting slot, contributes nothing.
func TestNurRunAttemptedWindowQuotesBareWord(t *testing.T) {
	quoting := &FnDefInfo{Name: "nrcq", Signatures: []Signature{
		{Params: []FnParam{{Name: "k", Type: TAtom}}, QuoteArgs: map[int]bool{0: true}, Fallback: true},
		{Params: []FnParam{{Name: "k", Type: TAtom}}, QuoteArgs: map[int]bool{0: true}},
	}}
	name := WithPosAt(NewWord("name"), SrcPos{Row: 1, Col: 6})
	tape := NewTape([]Value{NewWord("nrcq"), name}, StackHeadroom)
	got := attemptedWindowOver(tape, 0, quoting, nil, nil)
	if len(got) != 1 || !IsAtom(got[0]) || got[0].String() != "name" || got[0].Pos().Col != 6 {
		t.Fatalf("want the quoted 'name at 1:6, got %s", renderAll(got))
	}

	// Negative: a `/v` word is a reference, not a bare name.
	refTape := NewTape([]Value{NewWord("nrcq"), NewWordRef("name")}, StackHeadroom)
	if got := attemptedWindowOver(refTape, 0, quoting, nil, nil); len(got) != 0 {
		t.Fatalf("a /v word must not be quoted: %s", renderAll(got))
	}
	// Negative: no overload quotes its first slot.
	plain := &FnDefInfo{Name: "nrcp", Signatures: []Signature{{Params: []FnParam{{Name: "k", Type: TAtom}}}}}
	if got := attemptedWindowOver(tape, 0, plain, nil, nil); len(got) != 0 {
		t.Fatalf("an fn with no quoting slot must not quote: %s", renderAll(got))
	}
}

// Pins lowerReach's anchor: a receiver without a position hands the dispatch
// word the segment key's position; a positioned receiver keeps its own.
func TestNurRunLowerReachAnchorsAtKeyWhenReceiverUnpositioned(t *testing.T) {
	key := WithPosAt(NewAtom("name"), SrcPos{Row: 2, Col: 4})
	out := lowerReach(ReachInfo{Receiver: []Value{NewWord("__reach_recv")}, Segments: []ReachSeg{{KeyLit: key}}})
	if len(out) != 3 || out[1].String() != "word(dot)" || out[1].Pos().Row != 2 || out[1].Pos().Col != 4 {
		t.Fatalf("an unpositioned receiver must anchor the dot at the key, got %s at %+v", renderAll(out), out[1].Pos())
	}

	recv := WithPosAt(NewWord("m"), SrcPos{Row: 1, Col: 1})
	out = lowerReach(ReachInfo{Receiver: []Value{recv}, Segments: []ReachSeg{{KeyLit: key, Getr: true}}})
	if out[1].String() != "word(dotr)" || out[1].Pos().Row != 1 {
		t.Fatalf("a positioned receiver keeps its own anchor, got %s at %+v", renderAll(out), out[1].Pos())
	}
}

// --- the foreign-registry fn-return parks (NUR191) -------------------------------

// Pins that a module fn dispatched from an outer engine (execMatch over a
// handler built for the module registry) PARKS a single returned fn value
// instead of re-stepping it over the values beneath the call — while a
// multi-value residual is not parked, and its fn value re-steps.
func TestNurRunExecMatchParksForeignFnReturn(t *testing.T) {
	module := nrcEngineRegistry(t)
	factory := func(body string) FnDefInfo {
		return FnDefInfo{Name: "nrc" + body, Registry: module, Signatures: []Signature{{
			Impl:       Boru([]Value{NewWord(body)}),
			BarrierPos: BarrierAllForward,
		}}}
	}

	main := newTestRegistry(t)
	InstallFnDef(main, "nrcfactory", factory("nrcmk"))
	out, err := NewTop(main).Run([]Value{NewInteger(3), NewWord("nrcfactory")})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(out) != 2 || out[0].String() != "3" || !out[1].Parent.Equal(TFunction) {
		t.Fatalf("the returned fn must be parked beside the 3, got %s", renderAll(out))
	}

	// Negative: two survivors — no park; the fn re-steps over the 10.
	main2 := newTestRegistry(t)
	InstallFnDef(main2, "nrcfactory2", factory("nrcmk2"))
	out, err = NewTop(main2).Run([]Value{NewInteger(3), NewWord("nrcfactory2")})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := renderAll(out); got != "3 | 11" {
		t.Fatalf("a multi-value residual re-steps its fn: got %q, want \"3 | 11\"", got)
	}
}

// Pins execFnDefSig's cross-registry arm: a module fn VALUE whose body
// returns one fn value leaves the pointer past it (parked), while a
// non-callable result leaves the pointer on it.
func TestNurRunExecFnDefSigParksForeignFnReturn(t *testing.T) {
	module := nrcEngineRegistry(t)
	main := newTestRegistry(t)
	run := func(body Value) *Engine {
		t.Helper()
		sig := &FnSig{
			Params:     []FnParam{{Name: "x", Type: TInteger}},
			Impl:       Boru([]Value{body}),
			BarrierPos: BarrierAllForward,
		}
		e := NewTop(main)
		e.Tape = NewTape([]Value{NewInteger(1), NewFunction(FnDefInfo{Name: "nrcxfn", Registry: module})}, StackHeadroom)
		e.Pointer = 1
		if err := e.execFnDefSig(1, sig, []Value{NewInteger(1)}, module, false); err != nil {
			t.Fatalf("execFnDefSig: %v", err)
		}
		return e
	}

	e := run(NewWord("nrcmk"))
	if e.Tape.Len() != 1 || !e.Tape.At(0).Parent.Equal(TFunction) || e.Pointer != 1 {
		t.Fatalf("a returned fn must be parked: tape %s, pointer %d", renderAll(e.Tape.Snapshot()), e.Pointer)
	}
	e = run(NewInteger(42))
	if e.Tape.Len() != 1 || e.Tape.At(0).String() != "42" || e.Pointer != 0 {
		t.Fatalf("a plain result is not parked: tape %s, pointer %d", renderAll(e.Tape.Snapshot()), e.Pointer)
	}
}

// --- carrier bookkeeping for the compiler (NUR213 / NUR210) -----------------------

// Pins that a dispatch-modifier marker reaching the pointer standalone quotes
// a preceding carrier (fn-typed or dynamic) before it is dropped, and leaves
// a concrete value — or a first-position marker — alone.
func TestNurRunDispatchModQuotesPrecedingCarrier(t *testing.T) {
	r := newTestRegistry(t)
	step := func(prev ...Value) *Engine {
		t.Helper()
		e := NewTop(r)
		e.Tape = NewTape(append(prev, NewDispatchMod(DispatchModInfo{Val: true})), StackHeadroom)
		e.Pointer = len(prev)
		if err := e.stepLiteral(); err != nil {
			t.Fatalf("stepLiteral: %v", err)
		}
		if e.Tape.Len() != len(prev) {
			t.Fatalf("the marker must be dropped: %s", renderAll(e.Tape.Snapshot()))
		}
		return e
	}
	if e := step(NewCarrier(TFunction)); !e.Tape.At(0).Quoted {
		t.Fatal("a fn-typed carrier before the marker must be quoted")
	}
	if e := step(NewDynamicCarrier(TAny)); !e.Tape.At(0).Quoted {
		t.Fatal("a dynamic carrier before the marker must be quoted")
	}
	if e := step(NewInteger(5)); e.Tape.At(0).Quoted {
		t.Fatal("a concrete value before the marker stays unquoted")
	}
	step() // a marker with nothing before it is simply dropped
}

// Pins that under analysis a reach group collapsing to a lone fn-typed or
// dynamic carrier records it as a re-stepping survivor, while a quoted
// carrier, a concrete value, a non-reach collapse and an inactive pass
// record nothing.
func TestNurRunTagReachCollapsedCarrierSurvivor(t *testing.T) {
	r := newTestRegistry(t)
	tag := func(v Value, wasReach bool) {
		e := NewTop(r)
		e.Tape = NewTape([]Value{v, NewInteger(0), NewInteger(0)}, StackHeadroom)
		e.tagReachCollapsedFn(0, 2, wasReach)
	}
	withID := func(v Value, id string) Value { v.ID = id; return v }

	tag(withID(NewCarrier(TFunction), "nrc-inactive"), true)
	if r.Check.ReachSurvivorFnIDs != nil {
		t.Fatal("an inactive pass records nothing")
	}

	done := r.Check.Begin()
	defer done()
	tag(withID(NewCarrier(TFunction), "nrc-fn-carrier"), true)
	tag(withID(NewDynamicCarrier(TAny), "nrc-dyn"), true)
	quoted := withID(NewCarrier(TFunction), "nrc-quoted")
	quoted.Quoted = true
	tag(quoted, true)
	tag(withID(NewInteger(4), "nrc-int"), true)
	tag(withID(NewCarrier(TFunction), "nrc-not-reach"), false)

	got := r.Check.ReachSurvivorFnIDs
	if !got["nrc-fn-carrier"] || !got["nrc-dyn"] || len(got) != 2 {
		t.Fatalf("want exactly the fn-typed and dynamic survivors, got %v", got)
	}
}

// Pins the forward-claim probe: a dispatch-modifier marker is never an
// argument (probeNone), where a literal is a claimable value.
func TestNurRunForwardClaimProbeDispatchMod(t *testing.T) {
	r := newTestRegistry(t)
	tape := NewTape([]Value{NewDispatchMod(DispatchModInfo{Val: true}), NewInteger(5)}, StackHeadroom)
	if _, kind := ForwardClaimProbeOn(tape, r, 0); kind != probeNone {
		t.Fatalf("a dispatch-modifier marker must probe as none, got %d", kind)
	}
	if v, kind := ForwardClaimProbeOn(tape, r, 1); kind != probeValue || v.String() != "5" {
		t.Fatalf("a literal must probe as its value, got %d %v", kind, v)
	}
}

// --- noteFoldedFnBodies (NUR105) ------------------------------------------------

// Pins that a folded constant's fn values — at the top, inside a map, inside
// a list, nested — are queued for the end-of-pass body check, and that a
// carrier or a plain scalar queues nothing.
func TestNurRunNoteFoldedFnBodies(t *testing.T) {
	r := newTestRegistry(t)
	done := r.Check.Begin()
	defer done()

	inner := NewOrderedMap()
	inner.Set("c", NewFunction(FnDefInfo{Name: "nrcC"}))
	outer := NewOrderedMap()
	outer.Set("a", NewFunction(FnDefInfo{Name: "nrcA"}))
	outer.Set("n", NewInteger(1))
	outer.Set("l", NewList([]Value{NewFunction(FnDefInfo{Name: "nrcB"}), NewMap(inner)}))

	noteFoldedFnBodies(r, NewMap(outer))
	noteFoldedFnBodies(r, NewFunction(FnDefInfo{Name: "nrcTop"}))
	var got []string
	for _, pb := range r.Check.PendingFnBodies {
		got = append(got, pb.Fn.Name)
	}
	if strings.Join(got, " ") != "nrcA nrcB nrcC nrcTop" {
		t.Fatalf("queued %v, want every fn value in walk order", got)
	}

	// Negative: nothing to queue in a carrier or a scalar.
	noteFoldedFnBodies(r, NewCarrier(TList))
	noteFoldedFnBodies(r, NewInteger(3))
	if n := len(r.Check.PendingFnBodies); n != 4 {
		t.Fatalf("a carrier or a scalar must queue nothing, queue is %d long", n)
	}
}

// --- the unit-scoped definite trap (NUR134) ------------------------------------

// nrcTrapRecorder counts the unit-scoped traps a compile pass records; every
// other recorder method is the inactive one.
type nrcTrapRecorder struct {
	EmitRecorder
	unitTraps []*BoruError
}

func (s *nrcTrapRecorder) RecordUnitTrapErr(ae *BoruError, _ SrcPos) bool {
	s.unitTraps = append(s.unitTraps, ae)
	return true
}

// Pins that a DEFINITE failed named-fn dispatch below the uncaught top level
// of a compiling pass records the interpreter's error as a unit-scoped trap
// (and hits the enclosing raise watch), while a plain check pass and an
// inexact operand record none.
func TestNurRunDefiniteFailureRecordsUnitTrap(t *testing.T) {
	for _, tc := range []struct {
		name      string
		compiling bool
		arg       Value
		wantTraps int
		wantHit   bool
	}{
		{"compiling, definite", true, NewString("nope"), 1, true},
		{"a plain check records no trap", false, NewString("nope"), 0, true},
		{"an inexact operand", true, NewDynamicCarrier(TString), 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := covRegistry(t, nil)
			r.Check.Mode = true
			r.Check.Compiling = tc.compiling
			rec := &nrcTrapRecorder{EmitRecorder: TheInactiveEmit}
			prevEmit := r.Check.Emit
			r.Check.Emit = rec
			t.Cleanup(func() { r.Check.Mode, r.Check.Compiling, r.Check.Emit = false, false, prevEmit })
			prevTop := AnalysisImpl.AtUncaughtTopLevel
			AnalysisImpl.AtUncaughtTopLevel = func(*Registry) bool { return false }
			t.Cleanup(func() { AnalysisImpl.AtUncaughtTopLevel = prevTop })
			// The failure sits in a `do` body one nesting level down.
			r.Check.PushRaiseWatch()
			r.Check.NestedBodyDepth = 1

			fnDef := FnDefInfo{Name: "nrcfw", Signatures: []Signature{{
				Params:     []FnParam{{Name: "n", Type: TInteger}},
				Impl:       Boru([]Value{NewInteger(1)}),
				BarrierPos: BarrierAllForward,
			}}}
			arg := tc.arg
			arg.SetPos(SrcPos{Row: 3, Col: 2})
			e := NewTop(r)
			e.Tape = NewTape([]Value{arg, NewFunction(fnDef)}, StackHeadroom)
			e.Pointer = 1
			if err := e.ExecFnDefSigStackMatch(1, fnDef, []Value{arg}); err != nil {
				t.Fatalf("analysis must not raise: %v", err)
			}
			if len(rec.unitTraps) != tc.wantTraps {
				t.Fatalf("unit traps = %d, want %d", len(rec.unitTraps), tc.wantTraps)
			}
			if tc.wantTraps > 0 {
				ae := rec.unitTraps[0]
				if ae.Code != "uncalled_function" || ae.Row != 3 || !strings.Contains(ae.Detail, "call to 'nrcfw' matched no signature") {
					t.Fatalf("trapped error = %+v", ae)
				}
			}
			if len(r.Check.Diagnostics) != 0 {
				t.Fatalf("below the top level no diagnostic is emitted here: %+v", r.Check.Diagnostics)
			}
			if hit, _ := r.Check.PopRaiseWatch(); hit != tc.wantHit {
				t.Fatalf("raise watch hit = %v, want %v", hit, tc.wantHit)
			}
		})
	}
}
