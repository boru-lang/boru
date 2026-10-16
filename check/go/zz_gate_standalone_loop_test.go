package check

// Standalone-gate tests for AnalyseLoopBody's join decisions (carrier.go)
// and the helpers behind them: a body def of the loop's OWN bind name, a
// fresh binding of a loop that may run zero times, a speculative undef that
// moves the pass's generation, and a module bind the compiled lane's one
// replay cannot stand for. The loop-capture protocol is observed through
// an EmitRecorder stub (zcaEmit, zz_cover_carrier_analysis_test.go) that
// additionally records NoteLoopFresh and Suspend.

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// gsLoopEmit extends the loop-capture stub with the two calls the join
// decisions below make.
type gsLoopEmit struct {
	*zcaEmit
	fresh    map[string]core.Value
	suspends int
}

func newGsLoopEmit() *gsLoopEmit {
	return &gsLoopEmit{zcaEmit: zcaNewEmit(), fresh: map[string]core.Value{}}
}

func (s *gsLoopEmit) NoteLoopFresh(name string, joined core.Value) { s.fresh[name] = joined }
func (s *gsLoopEmit) Suspend() func() {
	s.suspends++
	return func() {}
}

// gsModuleValue is a module NAMESPACE value: a plain export Map carrying
// the module-namespace facet (core.IsModuleFamilyValue).
func gsModuleValue(exports *core.OrderedMap) core.Value {
	return core.WithModuleNS(core.NewMap(exports), "zgsmod", core.Value{})
}

// isBindName: a name is one of the loop's own bind variables exactly when
// it is listed.
func TestGateStandaloneIsBindName(t *testing.T) {
	if !isBindName([]string{"i", "j"}, "j") {
		t.Error("a listed bind name is a bind name")
	}
	if isBindName([]string{"i"}, "k") || isBindName(nil, "i") {
		t.Error("an unlisted name is not a bind name")
	}
}

// loopFreshCarriable: only a PLAIN value can ride a frame slot; a type
// binding (a capitalised name or a bare type node), a fn value or a
// Function carrier, and a module keep their own machinery.
func TestGateStandaloneLoopFreshCarriable(t *testing.T) {
	if !loopFreshCarriable("acc", core.NewInteger(1)) {
		t.Error("a plain value under a lower-case name rides a slot")
	}
	for name, c := range map[string]struct {
		name string
		v    core.Value
	}{
		"capitalised name": {"Acc", core.NewInteger(1)},
		"fn value":         {"f", bsFn(core.NewInteger(1))},
		"bare type node":   {"t", core.NewTypeLiteral(core.TInteger)},
		"module namespace": {"m", gsModuleValue(core.NewOrderedMap())},
		"Function carrier": {"g", core.NewCarrier(core.TFunction)},
		"dynamic Function": {"h", core.NewDynamicCarrier(core.TFunction)},
	} {
		if loopFreshCarriable(c.name, c.v) {
			t.Errorf("%s: must keep its own machinery, not ride a slot", name)
		}
	}
}

// loopModuleUnplaceable: a module the body binds is unplaceable when the
// loop may run zero times, or when the last two rounds bound DIFFERENT
// export maps (an inline module re-run per iteration). A cached module (the
// same export map both rounds) under a proven loop keeps its one replay;
// non-module bindings never decide anything.
func TestGateStandaloneLoopModuleUnplaceable(t *testing.T) {
	exports := core.NewOrderedMap()
	cached := gsModuleValue(exports)
	rerun := gsModuleValue(core.NewOrderedMap())
	plain := map[string]core.Value{"acc": core.NewInteger(1)}

	if loopModuleUnplaceable([]string{"acc"}, plain, plain, false) {
		t.Error("a non-module binding is never unplaceable")
	}
	if !loopModuleUnplaceable([]string{"m"}, map[string]core.Value{"m": cached}, nil, false) {
		t.Error("a module bound by a loop that may run zero times is unplaceable")
	}
	if loopModuleUnplaceable([]string{"m"}, map[string]core.Value{"m": cached}, map[string]core.Value{"m": cached}, true) {
		t.Error("a cached module under a proven loop keeps its one replay")
	}
	if loopModuleUnplaceable([]string{"m"}, map[string]core.Value{"m": cached}, nil, true) {
		t.Error("a proven loop's single round has nothing to compare: placeable")
	}
	if !loopModuleUnplaceable([]string{"m"}, map[string]core.Value{"m": rerun}, map[string]core.Value{"m": cached}, true) {
		t.Error("a module whose export map differs across rounds is re-run per iteration: unplaceable")
	}
}

// A body def of one of the loop's OWN bind names rebinds the iteration's
// binding and ends with it (NUR204): it is neither joined into the
// post-loop binding nor loop-carried. The pre-loop binding of the same name
// survives the loop untouched — where a non-bind name the body rebinds is
// joined and carried (the pairing form).
func TestGateStandaloneLoopBindNameDefNotJoined(t *testing.T) {
	r := zcaRegistry(t)
	zcaRegisterPushq(r)
	done := r.Check.Begin()
	defer done()
	es := newGsLoopEmit()
	es.loopArm = true
	r.Check.Emit = es

	outer := core.NewString("outer")
	r.Defs.Push("iq", outer)
	r.Defs.Push("zgsaccq", core.NewString("acc"))
	body := core.NewList([]core.Value{
		core.NewWord("pushq"), core.NewWord("iq"), core.NewInteger(9),
		core.NewWord("pushq"), core.NewWord("zgsaccq"), core.NewInteger(7),
	})
	AnalyseLoopBody(r, body, []string{"iq"}, []core.Value{core.NewCarrier(core.TInteger)}, true)

	if got, ok := r.Defs.Top("iq"); !ok || core.IDOf(&got) != core.IDOf(&outer) {
		t.Errorf("the pre-loop iq must survive the loop untouched, got %#v", got)
	}
	if es.carried["iq"] {
		t.Error("a def of the loop's own bind name is not loop-carried")
	}
	if !es.carried["zgsaccq"] {
		t.Error("the rebind of a non-bind pre-loop name is loop-carried")
	}
	if acc, _ := r.Defs.Top("zgsaccq"); !acc.Parent().Equal(core.TDisjunct) {
		t.Errorf("the non-bind rebind joins with its pre-loop binding, got %v", acc.Parent())
	}
}

// A FRESH name in a loop that may run zero times is bound after the loop
// only if the body ran (NUR214): the post-loop binding is a carrier with its
// own identity — no read can fold the body's value into it — noted for a
// slot with no init. A loop with a proven trip count binds the body's value
// itself and notes nothing.
func TestGateStandaloneLoopFreshUnprovenCarried(t *testing.T) {
	run := func(proven bool) (*gsLoopEmit, core.Value) {
		r := zcaRegistry(t)
		zcaRegisterPushq(r)
		done := r.Check.Begin()
		defer done()
		es := newGsLoopEmit()
		es.loopArm = true
		r.Check.Emit = es
		body := core.NewList([]core.Value{core.NewWord("pushq"), core.NewWord("zgsfreshq"), core.NewInteger(7)})
		AnalyseLoopBody(r, body, []string{"iq"}, []core.Value{core.NewCarrier(core.TInteger)}, proven)
		v, ok := r.Defs.Top("zgsfreshq")
		if !ok {
			t.Fatalf("proven=%v: the fresh binding must be installed post-loop", proven)
		}
		return es, v
	}

	es, v := run(false)
	noted, ok := es.fresh["zgsfreshq"]
	if !ok {
		t.Fatal("a fresh name of a may-run-zero-times loop must be noted for a slot")
	}
	if core.IsConcrete(&v) {
		t.Errorf("the post-loop binding must be a carrier, not the body's value: %#v", v)
	}
	if core.IDOf(&noted) != core.IDOf(&v) {
		t.Error("the noted slot value is the installed post-loop binding")
	}

	es, v = run(true)
	if len(es.fresh) != 0 {
		t.Errorf("a proven loop notes no fresh slot, got %v", es.fresh)
	}
	if n, err := core.AsInteger(v); err != nil || n != 7 {
		t.Errorf("a proven loop binds the body's value itself, got %#v", v)
	}
}

// A speculative undef inside the body generalises an enclosing binding IN
// PLACE and moves the pass's generation (core.GeneraliseSpecUndef): no
// depth grows, so the join sees nothing, yet the round is not stable — reads
// recorded before the undef baked a value the next iteration no longer has.
// The loop re-rounds once (the second undef re-mints nothing), where a body
// that leaves the binding alone settles in one round.
func TestGateStandaloneLoopSpecUndefReRounds(t *testing.T) {
	rounds := func(undef bool) int {
		r := zcaRegistry(t)
		r.RegisterNativeFunc(core.NativeFunc{
			Name: "zgsundefq",
			Signatures: []core.Signature{{
				Args:       []*core.Type{core.TAtom},
				QuoteArgs:  map[int]bool{0: true},
				Returns:    []*core.Type{},
				BarrierPos: -1,
				Impl: core.Go(func(args []core.Value, _ map[string]core.Value, _ []core.Value, reg *core.Registry) ([]core.Value, error) {
					name, err := args[0].AsConcreteAtom()
					if err != nil {
						return nil, err
					}
					if undef {
						core.GeneraliseSpecUndef(reg, name)
					}
					return nil, nil
				}, core.RunInCheck()),
			}},
		})
		done := r.Check.Begin()
		defer done()
		es := newGsLoopEmit()
		es.loopArm = true
		r.Check.Emit = es
		r.Defs.Push("zgsk", core.NewInteger(5))
		body := core.NewList([]core.Value{core.NewWord("zgsundefq"), core.NewWord("zgsk")})
		AnalyseLoopBody(r, body, []string{"iq"}, []core.Value{core.NewCarrier(core.TInteger)}, true)
		if undef {
			if k, ok := r.Defs.Top("zgsk"); !ok || core.IsConcrete(&k) {
				t.Errorf("the speculative undef keeps the binding as a carrier, got %#v", k)
			}
		}
		return es.checkpoints
	}
	if n := rounds(false); n != 1 {
		t.Errorf("a body leaving its bindings alone settles in one round, ran %d", n)
	}
	if n := rounds(true); n != 2 {
		t.Errorf("a speculative undef forces exactly one more round, ran %d", n)
	}
}

// A loop that may run zero times binding a MODULE is unplaceable for the
// compiled lane's one replay (NUR205): its bind-ledger notes are made with
// the recorder SUSPENDED (placement withheld), so the program declines at
// the twin regime's gate. A plain fresh value under the same loop is
// noted live.
func TestGateStandaloneLoopModuleBindSuspendsNotes(t *testing.T) {
	run := func(v core.Value) int {
		r := zcaRegistry(t)
		zcaRegisterPushq(r)
		done := r.Check.Begin()
		defer done()
		es := newGsLoopEmit()
		r.Check.Emit = es
		body := core.NewList([]core.Value{core.NewWord("pushq"), core.NewWord("zgsmodq"), v})
		AnalyseLoopBody(r, body, []string{"iq"}, []core.Value{core.NewCarrier(core.TInteger)}, false)
		if _, ok := r.Defs.Top("zgsmodq"); !ok {
			t.Fatal("the body's binding must be installed post-loop")
		}
		return es.suspends
	}
	if n := run(core.NewInteger(3)); n != 0 {
		t.Errorf("a plain binding notes live, suspended %d times", n)
	}
	if n := run(gsModuleValue(core.NewOrderedMap())); n != 1 {
		t.Errorf("a module bound by a may-run-zero-times loop must suspend its notes once, suspended %d", n)
	}
}
