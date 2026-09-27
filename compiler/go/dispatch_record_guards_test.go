package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// dispatch_record_guards_test.go pins the admission guards of the dispatch
// recording family (compiler_dispatch_record.go) directly, each beside the
// admitted twin that proves the guard — not a later check — made the call.

// TestTryFoldStaticIndexArgsProjection: `args.N` folds to the element's own
// home only when the args projection's MAKE_LIST can be retracted — it is
// still the frame's last event. When something was recorded after it (`def
// xs (args)` records the def), the fold stands aside and the read records
// over the produced list; lang's TestArgsProjectionReadAfterDef runs that
// program on both lanes.
func TestTryFoldStaticIndexArgsProjection(t *testing.T) {
	setup := func(t *testing.T, trailing bool) (*core.Registry, *EmitState, core.Value, []core.Value) {
		r := newTestRegistry(t)
		es := armEmit(r)
		recv := core.NewList([]core.Value{core.NewInteger(3), core.NewInteger(4)})
		recv.ID = "ARGS"
		es.argsProjSeq = map[string]int{recv.ID: 7}
		es.producedBy[recv.ID] = producer{seq: 7}
		es.frames[0] = []EmitEvent{{seq: 7, kind: evCall, call: emitCall{word: "args", nout: 1}}}
		if trailing {
			es.frames[0] = append(es.frames[0], EmitEvent{seq: 8, kind: evDynBind, dyn: &emitDynBind{name: "xs", srcSeq: 7, residentTwin: -1}})
		}
		out := core.NewCarrier(core.TInteger)
		out.ID = "OUT"
		return r, es, recv, []core.Value{out}
	}

	r, es, recv, outs := setup(t, true)
	if tryFoldStaticIndex(r, "get", []core.Value{core.NewInteger(1), recv}, outs) {
		t.Fatal("a projection another event follows cannot be retracted: the fold must stand aside")
	}
	if outs[0].ID != "OUT" || len(es.frames[0]) != 2 || es.argsProjSeq[recv.ID] != 7 {
		t.Fatalf("a declined fold must leave the projection and the result alone (out %v, frame %d events)", outs[0], len(es.frames[0]))
	}

	r, es, recv, outs = setup(t, false)
	if !tryFoldStaticIndex(r, "get", []core.Value{core.NewInteger(1), recv}, outs) {
		t.Fatal("a projection that is the frame's last event is retracted and the read folds")
	}
	if n, err := core.AsInteger(outs[0]); err != nil || n != 4 {
		t.Fatalf("the fold must hand on the element, got %v", outs[0])
	}
	if len(es.frames[0]) != 0 || len(es.argsProjSeq) != 0 || es.AlreadyProduced(recv.ID) {
		t.Fatal("the retracted projection must leave no event, note or producer behind")
	}
}

// drgDynBodySig is a CompileDynBody code-body word's signature: one body at 0,
// plus a second code slot (a walk-style hook) when hook is set.
func drgDynBodySig(bodyOut int, hook bool) *core.Signature {
	sig := &core.Signature{Args: []*core.Type{core.TList}, NoEvalArgs: map[int]bool{0: true}, CompileEffect: core.CompileDynBody,
		Callable: &core.CallableSpec{BodyPos: 0, BodyOut: bodyOut}}
	if hook {
		sig.Args = append(sig.Args, core.TList)
		sig.NoEvalArgs[1] = true
	}
	return sig
}

// TestTryRecordDynBodyZeroOutGuard: a dispatch the check pass left with NO
// result is admitted only for a word that declares a 0-out body (`for-each`);
// for any other word 0 results is the divergent-path shape, not the word's
// count, and the backstop declines it.
func TestTryRecordDynBodyZeroOutGuard(t *testing.T) {
	body := []core.Value{core.NewList([]core.Value{core.NewInteger(1)})}
	r := newTestRegistry(t)
	es := armEmit(r)
	if tryRecordDynBody(r, "dobody", drgDynBodySig(1, false), body, nil, core.SrcPos{}) {
		t.Fatal("0 results from a body that declares a value must decline")
	}
	if len(es.frames[0]) != 0 || es.dynEnv {
		t.Fatal("a declined backstop records nothing")
	}
	if !tryRecordDynBody(r, "dobody", drgDynBodySig(0, false), body, nil, core.SrcPos{}) {
		t.Fatal("a declared 0-out body's 0 results are its own count: recorded")
	}
	if len(es.frames[0]) != 1 || es.frames[0][0].kind != evCall {
		t.Fatalf("the admitted backstop records one call, got %v", es.frames[0])
	}
}

// TestTryRecordDynBodyHookScreen: every OTHER code slot the handler re-runs (a
// walk-style hook) is held to the body's own two rules — no flow sentinel
// (break/continue cannot cross the handler boundary) and no replay hazard (a
// capitalised def re-run against a half-rolled-back registry).
func TestTryRecordDynBodyHookScreen(t *testing.T) {
	body := core.NewList([]core.Value{core.NewInteger(1)})
	outs := []core.Value{core.NewCarrier(core.TAny)}
	r := newTestRegistry(t)
	for what, hook := range map[string]core.Value{
		"a break":           core.NewList([]core.Value{core.NewWord("break")}),
		"a continue":        core.NewList([]core.Value{core.NewWord("continue")}),
		"a capitalised def": core.NewList([]core.Value{core.NewWord("def"), core.NewWord("Big"), core.NewInteger(1)}),
	} {
		es := armEmit(r)
		if tryRecordDynBody(r, "hooked", drgDynBodySig(1, true), []core.Value{body, hook}, outs, core.SrcPos{}) {
			t.Errorf("a hook with %s must decline", what)
		}
		if len(es.frames[0]) != 0 {
			t.Errorf("a hook with %s: nothing may be recorded", what)
		}
	}
	es := armEmit(r)
	clean := core.NewList([]core.Value{core.NewInteger(2)})
	if !tryRecordDynBody(r, "hooked", drgDynBodySig(1, true), []core.Value{body, clean}, outs, core.SrcPos{}) || len(es.frames[0]) != 1 {
		t.Fatal("a clean hook passes the screen and the call records")
	}
}

// TestSoleNoEvalSlot: the one NoEvalArgs position of a signature, or none
// when it has none or several (a clause-list word has exactly one).
func TestSoleNoEvalSlot(t *testing.T) {
	for what, c := range map[string]struct {
		noEval map[int]bool
		pos    int
		ok     bool
	}{
		"one code slot":                 {map[int]bool{1: true}, 1, true},
		"none":                          {nil, -1, false},
		"two code slots":                {map[int]bool{0: true, 2: true}, -1, false},
		"a code slot past the operands": {map[int]bool{3: true}, -1, false},
	} {
		pos, ok := soleNoEvalSlot(&core.Signature{NoEvalArgs: c.noEval}, 3)
		if pos != c.pos || ok != c.ok {
			t.Errorf("%s: got (%d, %v), want (%d, %v)", what, pos, ok, c.pos, c.ok)
		}
	}
}

// TestTryRecordFallbackDeclinesReachLens: a higher-order word dispatched on
// its LENS form (`filter $.on data`) has no code body for an island to run —
// the lens is inert data — so it never islands, where the same word over a
// code body does.
func TestTryRecordFallbackDeclinesReachLens(t *testing.T) {
	r := newTestRegistry(t)
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "lensw",
		Signatures: []core.Signature{{
			Args: []*core.Type{core.TAny}, NoEvalArgs: map[int]bool{0: true},
			CompileEffect: core.CompileFallbackBody,
			Callable:      &core.CallableSpec{BodyPos: 0, BodyOut: 1},
			Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
				return []core.Value{core.NewInteger(0)}, nil
			}),
			Returns: []*core.Type{core.TAny}, BarrierPos: -1,
		}},
	})
	if err := r.Err(); err != nil {
		t.Fatalf("registration: %v", err)
	}
	sig := &r.Lookup("lensw").Signatures[0]
	out := func(id string) []core.Value {
		v := core.NewDynamicCarrier(core.TAny)
		v.ID = id
		return []core.Value{v}
	}

	es := armEmit(r)
	lens := core.NewReach(core.ReachInfo{Segments: []core.ReachSeg{{KeyLit: core.NewAtom("on")}}})
	if TryRecordFallback(r, "lensw", sig, []core.Value{lens}, out("L"), core.SrcPos{}) {
		t.Fatal("a reach lens in the body position must not island")
	}
	if len(es.fallbacks) != 0 || len(es.frames[0]) != 0 {
		t.Fatal("a declined island records nothing")
	}

	es = armEmit(r)
	body := core.NewList([]core.Value{core.NewInteger(1)})
	if !TryRecordFallback(r, "lensw", sig, []core.Value{body}, out("B"), core.SrcPos{}) || len(es.fallbacks) != 1 {
		t.Fatal("the same word over a code body islands")
	}
}
