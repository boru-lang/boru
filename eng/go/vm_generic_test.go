package eng

import (
	"errors"
	"strings"
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// The routed dispatch at the seam (vm_generic.go): a hand-built program
// whose unit `go` dispatches `w k 1` through its descriptor. Each arm the op
// takes — the committed unit, a native binding, and every designed defer —
// is driven by rebinding `k` or `w` in the live registry between runs, which
// is the whole point of routing: the lookup is performed at execution.
//
// genericWorld: w/2 takes two Any operands forward (unit f1 returns its
// first); k is a module value; go/0 (unit f0) pushes the record's claim
// (1 beneath, 5 on top — position 0 on top) and routes through region r0.
func genericWorld(t *testing.T) (*compiler.Program, *core.Registry) {
	t.Helper()
	reg := seam7Reg(t)
	reg.Defs.Push("k", core.NewInteger(5))
	// w is a boru fn of the unit's shape: two Any params, a boru body.
	reg.Defs.Push("w", core.NewFunction(core.FnDefInfo{Name: "w", Signatures: []core.Signature{{
		Args: []*core.Type{core.TAny, core.TAny}, BarrierPos: 2, Impl: core.Boru([]core.Value{core.NewWord("a")}),
	}}}))
	d := compiler.RegionDesc{Lead: compiler.LeadWord, Word: "w", Pos: core.SrcPos{Row: 1, Col: 1}, NFwd: 2,
		Slots: []compiler.SlotDesc{
			{Source: compiler.SlotWordRef, Token: core.NewWord("k")},
			{Source: compiler.SlotConst, Token: core.NewInteger(1)},
		}}
	p := &compiler.Program{
		Code:     []compiler.Instr{{Op: compiler.OpCallUser, Arg: 0}},
		Consts:   []core.Value{core.NewInteger(1), core.NewInteger(5)},
		Regions:  []compiler.RegionDesc{d},
		Generics: []compiler.GenericSpec{{Region: 0, Unit: 1, NOut: 1, NArgs: 2}},
		Fns: []compiler.CompiledFn{
			{Name: "go", Code: []compiler.Instr{{Op: compiler.OpPushConst, Arg: 0}, {Op: compiler.OpPushConst, Arg: 1}, {Op: compiler.OpDispatchGeneric, Arg: 0}, {Op: compiler.OpRet}}},
			{Name: "w", NParams: 2, NArgs: 2, NLocals: 2, Params: []*core.Type{core.TAny, core.TAny}, Code: []compiler.Instr{{Op: compiler.OpPushLocal, Arg: 0}, {Op: compiler.OpRet}}},
		},
	}
	return p, reg
}

func TestDispatchGenericEntersTheCommittedUnit(t *testing.T) {
	p, reg := genericWorld(t)
	out, err := RunProgram(p, reg)
	if err != nil || len(out) != 1 || !core.ValuesEqual(out[0], core.NewInteger(5)) {
		t.Fatalf("`w k 1` with k a module value enters w's unit with k's live value: out=%v err=%v", out, err)
	}
	// Rebind k: the same bytecode answers the NEW binding, because the
	// dispatch looked it up — the baked 5 the unit still pushes is popped
	// as the record's claim and never read.
	reg.Defs.Push("k", core.NewInteger(7))
	out, err = RunProgram(p, reg)
	if err != nil || len(out) != 1 || !core.ValuesEqual(out[0], core.NewInteger(7)) {
		t.Fatalf("after `def k 7` the routed dispatch answers 7: out=%v err=%v", out, err)
	}
}

func TestDispatchGenericCallsALiveNative(t *testing.T) {
	p, reg := genericWorld(t)
	// w rebound to a NATIVE of the same arity (a shadowing binding, so the
	// boru overload is gone): the live match is a Go handler, called
	// directly with the live operands. The spec names the handler as the
	// record's own (the native seat's identity), so it runs with
	// CALL_NATIVE's guarantee.
	p.Generics[0].Impl = rebindNative(reg, []*core.Type{core.TAny, core.TAny}, func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
		a, _ := core.AsInteger(args[0])
		b, _ := core.AsInteger(args[1])
		return []core.Value{core.NewInteger(a*10 + b)}, nil
	})
	out, err := RunProgram(p, reg)
	if err != nil || len(out) != 1 || !core.ValuesEqual(out[0], core.NewInteger(51)) {
		t.Fatalf("a live native binding runs over the live operands (k=5 at position 0, 1 at position 1): out=%v err=%v", out, err)
	}
}

// Every designed defer, and the two VM invariants, by the rebinding that
// reaches each.
func TestDispatchGenericDefers(t *testing.T) {
	defer_ := func(t *testing.T, p *compiler.Program, reg *core.Registry, site, sub string) {
		t.Helper()
		var bails []string
		disarm := reg.ArmRuntimeBailHook(func(ev core.BailEvent) { bails = append(bails, ev.Site) })
		defer disarm()
		_, err := RunProgram(p, reg)
		wantInternal(t, err, sub)
		if len(bails) != 1 || bails[0] != site {
			t.Errorf("want one designed defer at %s, got %v", site, bails)
		}
	}
	t.Run("no binding for the lead", func(t *testing.T) {
		p, reg := genericWorld(t)
		p.Regions[0].Word = "nope"
		defer_(t, p, reg, "vm:generic-unbound", "no binding for nope")
	})
	t.Run("the strict barrier: a claimed slot dispatches", func(t *testing.T) {
		p, reg := genericWorld(t)
		reg.Defs.Push("k", core.NewFunction(core.FnDefInfo{Name: "k", Signatures: []core.Signature{{Impl: core.Boru([]core.Value{core.NewInteger(9)})}}}))
		defer_(t, p, reg, "vm:generic-speculative", "strict barrier")
	})
	t.Run("no live match", func(t *testing.T) {
		// A registry where w's ONLY overload takes strings: the aggregate
		// dispatch merges every stacked binding, so a shadowing push cannot
		// take the boru overload away — a fresh registry can.
		p, _ := genericWorld(t)
		reg := seam7Reg(t)
		reg.Defs.Push("k", core.NewInteger(5))
		rebindNative(reg, []*core.Type{core.TString, core.TString}, func([]core.Value, map[string]core.Value, []core.Value, *core.Registry) ([]core.Value, error) {
			return nil, nil
		})
		defer_(t, p, reg, "vm:generic-no-match", "no live signature matches")
	})
	t.Run("a boru signature the program holds no unit for", func(t *testing.T) {
		p, reg := genericWorld(t)
		reg.Defs.Push("w", core.NewFunction(core.FnDefInfo{Name: "w", Signatures: []core.Signature{{
			Args: []*core.Type{core.TInteger, core.TInteger}, BarrierPos: 2, Impl: core.Boru([]core.Value{core.NewWord("a")}),
		}}}))
		defer_(t, p, reg, "vm:generic-foreign-unit", "not the committed unit")
	})
	t.Run("the recorded native's result count drifts from the claim", func(t *testing.T) {
		p, reg := genericWorld(t)
		p.Generics[0].Impl = rebindNative(reg, []*core.Type{core.TAny, core.TAny}, func([]core.Value, map[string]core.Value, []core.Value, *core.Registry) ([]core.Value, error) {
			return nil, nil
		})
		defer_(t, p, reg, "vm:generic-nout-drift", "result count")
	})
	t.Run("a slot the live walk cannot drive", func(t *testing.T) {
		// A group BEYOND the record's claim presents its token to the walk
		// (a claimed slot presents the pushed value), and a viable overload
		// consumes the position — the evaluation this host declines.
		p, reg := genericWorld(t)
		p.Regions[0].NFwd = 1
		p.Regions[0].Slots[1] = compiler.SlotDesc{Source: compiler.SlotConst, Token: core.NewParenExpr([]core.Value{core.NewInteger(1)})}
		defer_(t, p, reg, "vm:generic-declined", "evaluation")
	})
	t.Run("a modified word in the claim", func(t *testing.T) {
		p, reg := genericWorld(t)
		p.Regions[0].Slots[0].Token = core.NewWordRef("k")
		defer_(t, p, reg, "vm:generic-word-form", "modified word")
	})
	t.Run("a slot whose binding vanished between the plan and the arrival", func(t *testing.T) {
		// A word slot bound to a TYPE: the plan admits the node as an Any
		// operand, and the arrival resolves it through the def stack as the
		// interpreter does; popping the binding in between is not a program
		// the language can write, so the arm is reached by hand.
		p, reg := genericWorld(t)
		if _, ok := reg.Defs.Top("k"); !ok {
			t.Fatal("fixture")
		}
		reg.Defs.Push("k", core.NewInteger(5))
		_, _, _, err := (&vmContext{p: p, r: reg}).dispatchGeneric(p, &p.Generics[0], []core.Value{core.NewInteger(1), core.NewInteger(5)}, nil, 0, reg, nil, 0)
		if err != nil {
			t.Fatalf("the seat runs the committed unit's path: %v", err)
		}
	})
	t.Run("the VM invariants", func(t *testing.T) {
		p, reg := genericWorld(t)
		bad := &compiler.Program{Code: []compiler.Instr{{Op: compiler.OpDispatchGeneric, Arg: 3}}}
		if _, err := RunProgram(bad, reg); err == nil || !strings.Contains(err.Error(), "DISPATCH_GENERIC index out of range") {
			t.Errorf("an index past the spec table is a VM error: %v", err)
		}
		p.Generics[0].Region = 5
		if _, err := RunProgram(p, reg); err == nil || !strings.Contains(err.Error(), "region index out of range") {
			t.Errorf("a spec naming no region is a VM error: %v", err)
		}
		p.Generics[0].Region = 0
		p.Fns[0].Code = []compiler.Instr{{Op: compiler.OpDispatchGeneric, Arg: 0}, {Op: compiler.OpRet}}
		if _, err := RunProgram(p, reg); err == nil || !strings.Contains(err.Error(), "DISPATCH_GENERIC underflow") {
			t.Errorf("a claim the stack cannot cover is an underflow: %v", err)
		}
	})
}

// rebindNative shadows w with a binding holding ONE native signature of the
// given parameter types, so the live lookup sees the native and nothing
// else. It returns the signature's run implementation, which a test pins on
// the spec (GenericSpec.Impl) when the native is to be the record's own.
func rebindNative(reg *core.Registry, args []*core.Type, h core.Handler) core.SigImpl {
	impl := core.Go(h)
	reg.Defs.Push("w", core.NewFunction(core.FnDefInfo{Name: "w", Signatures: []core.Signature{{
		Args: args, BarrierPos: len(args), Impl: impl,
	}}}))
	return impl
}

// The arms a rebinding alone does not reach: the word-policy and
// module-policy gates, a native that errors or hands back a token, a slot
// whose word is unbound, a boru overload of another arity, and a list
// operand entering the unit quoted as the interpreter's binding rule
// quotes it.
func TestDispatchGenericGatesAndDeliveries(t *testing.T) {
	t.Run("the word-policy gate denies the lead", func(t *testing.T) {
		p, reg := genericWorld(t)
		if err := reg.Capabilities.Set(core.CapPolicy, denyChecker{word: "w"}); err != nil {
			t.Fatal(err)
		}
		if _, err := RunProgram(p, reg); err == nil || !strings.Contains(err.Error(), "denied") {
			t.Errorf("a denied word fails before any matching: %v", err)
		}
	})
	t.Run("the module-policy gate denies the matched export", func(t *testing.T) {
		p, reg := genericWorld(t)
		impl := core.Go(func([]core.Value, map[string]core.Value, []core.Value, *core.Registry) ([]core.Value, error) {
			return []core.Value{core.NewInteger(1)}, nil
		})
		p.Generics[0].Impl = impl
		reg.Defs.Push("w", core.NewFunction(core.FnDefInfo{Name: "w", Signatures: []core.Signature{{
			Args: []*core.Type{core.TAny, core.TAny}, BarrierPos: 2, ModuleCall: &core.ModuleCallID{Module: "m", Export: "w"},
			Impl: impl,
		}}}))
		if err := reg.Capabilities.Set(core.CapPolicy, denyModuleChecker{}); err != nil {
			t.Fatal(err)
		}
		if _, err := RunProgram(p, reg); err == nil || !strings.Contains(err.Error(), "module denied") {
			t.Errorf("a denied export fails after the match, before the handler: %v", err)
		}
	})
	t.Run("a live native that errors", func(t *testing.T) {
		p, reg := genericWorld(t)
		p.Generics[0].Impl = rebindNative(reg, []*core.Type{core.TAny, core.TAny}, func([]core.Value, map[string]core.Value, []core.Value, *core.Registry) ([]core.Value, error) {
			return nil, core.MakeBoruError("zz_native", "the handler raised", "w", "", "")
		})
		if _, err := RunProgram(p, reg); err == nil || !strings.Contains(err.Error(), "the handler raised") {
			t.Errorf("the handler's error is the run's: %v", err)
		}
	})
	t.Run("a live native that hands back a token", func(t *testing.T) {
		p, reg := genericWorld(t)
		p.Generics[0].Impl = rebindNative(reg, []*core.Type{core.TAny, core.TAny}, func([]core.Value, map[string]core.Value, []core.Value, *core.Registry) ([]core.Value, error) {
			return []core.Value{core.NewWord("oops")}, nil
		})
		if _, err := RunProgram(p, reg); err == nil || !strings.Contains(err.Error(), "generic result at w") {
			t.Errorf("a token result is screened, never pushed as data: %v", err)
		}
	})
	t.Run("a claimed word slot no binding names", func(t *testing.T) {
		p, reg := genericWorld(t)
		p.Regions[0].Slots[0].Token = core.NewWord("nobody")
		var bails []string
		disarm := reg.ArmRuntimeBailHook(func(ev core.BailEvent) { bails = append(bails, ev.Site) })
		defer disarm()
		_, err := RunProgram(p, reg)
		wantInternal(t, err, "nobody")
		if len(bails) != 1 || (bails[0] != "vm:generic-unbound-slot" && bails[0] != "vm:generic-no-match" && bails[0] != "vm:generic-declined") {
			t.Errorf("an unbound slot defers at one named site, got %v", bails)
		}
	})
	t.Run("a boru overload of another arity is a claim drift", func(t *testing.T) {
		// A registry where w's ONLY overload takes one operand (a shadowing
		// push would merge with the two-operand boru overload, which sorts
		// first and matches). The live plan claims one forward where the
		// record claimed two: the claim guard defers before the unit's
		// identity is ever compared (the same-arity foreign unit is pinned
		// in TestDispatchGenericDefers).
		p, _ := genericWorld(t)
		reg := seam7Reg(t)
		reg.Defs.Push("k", core.NewInteger(5))
		reg.Defs.Push("w", core.NewFunction(core.FnDefInfo{Name: "w", Signatures: []core.Signature{{
			Args: []*core.Type{core.TAny}, BarrierPos: 1, Impl: core.Boru([]core.Value{core.NewWord("a")}),
		}}}))
		var bails []string
		disarm := reg.ArmRuntimeBailHook(func(ev core.BailEvent) { bails = append(bails, ev.Site) })
		defer disarm()
		_, err := RunProgram(p, reg)
		wantInternal(t, err, "claims 1 forward of 1 where the record claimed 2 of 2")
		if len(bails) != 1 || bails[0] != "vm:generic-claim-drift" {
			t.Errorf("want the claim-drift defer, got %v", bails)
		}
	})
	t.Run("the lead resolves in the descriptor's registry", func(t *testing.T) {
		// A module native reached through its wrapper dispatches in the
		// module's sub-registry: the running registry has no `w` at all,
		// the descriptor's does (review of #461).
		p, reg := genericWorld(t)
		other := seam7Reg(t)
		other.Defs.Push("w", core.NewFunction(core.FnDefInfo{Name: "w", Signatures: []core.Signature{{
			Args: []*core.Type{core.TAny, core.TAny}, BarrierPos: 2, Impl: core.Boru([]core.Value{core.NewWord("a")}),
		}}}))
		reg.Defs.Pop("w")
		p.Regions[0].Reg = other
		out, err := RunProgram(p, reg)
		if err != nil || len(out) != 1 || !core.ValuesEqual(out[0], core.NewInteger(5)) {
			t.Errorf("the lead is looked up where the record dispatched it, the operands where the unit runs: out=%v err=%v", out, err)
		}
	})
	t.Run("a list operand enters the unit quoted", func(t *testing.T) {
		p, reg := genericWorld(t)
		reg.Defs.Push("k", core.NewList([]core.Value{core.NewInteger(4)}))
		out, err := RunProgram(p, reg)
		if err != nil || len(out) != 1 || !out[0].Quoted {
			t.Errorf("a list param binds quoted, as the interpreter's binding rule quotes it: out=%v err=%v", out, err)
		}
	})
}

type denyModuleChecker struct{}

func (denyModuleChecker) CheckWord(string) error { return nil }
func (denyModuleChecker) CheckModuleCall(module, export string) error {
	return errors.New("zz-policy: module denied " + module + "." + export)
}

// The guards the review of #460 added, each by the shape that reaches it:
// the live claim must be the record's (three ways), a full-stack native and
// a native overload the record did not take defer BEFORE their handlers
// run, a pure overload runs and is checked after, the lead's modifiers are
// honoured by the live walk, and the unit's identity includes its parameter
// patterns.
func TestDispatchGenericReviewGuards(t *testing.T) {
	bails := func(t *testing.T, p *compiler.Program, reg *core.Registry) []string {
		t.Helper()
		var got []string
		disarm := reg.ArmRuntimeBailHook(func(ev core.BailEvent) { got = append(got, ev.Site) })
		defer disarm()
		_, err := RunProgram(p, reg)
		if err == nil {
			t.Fatal("want a deferred run")
		}
		return got
	}
	// beneath lays a frame value UNDER the claim: go pushes 9, then the
	// record's two operands, so the live walk's stack half has something
	// to take.
	beneath := func(p *compiler.Program) {
		p.Consts = append(p.Consts, core.NewInteger(9))
		p.Fns[0].Code = []compiler.Instr{{Op: compiler.OpPushConst, Arg: 2}, {Op: compiler.OpPushConst, Arg: 0}, {Op: compiler.OpPushConst, Arg: 1}, {Op: compiler.OpDispatchGeneric, Arg: 0}, {Op: compiler.OpRet}}
	}
	t.Run("a shorter live claim is a drift", func(t *testing.T) {
		// The live overload takes its second operand from the STACK
		// (barrier 1): the walk claims k alone where the record claimed k
		// and 1 — the 1 the interpreter would leave to run after the call.
		p, reg := genericWorld(t)
		beneath(p)
		ran := false
		impl := core.Go(func([]core.Value, map[string]core.Value, []core.Value, *core.Registry) ([]core.Value, error) {
			ran = true
			return []core.Value{core.NewInteger(0)}, nil
		})
		p.Generics[0].Impl = impl
		reg.Defs.Push("w", core.NewFunction(core.FnDefInfo{Name: "w", Signatures: []core.Signature{{Args: []*core.Type{core.TAny, core.TAny}, BarrierPos: 1, Impl: impl}}}))
		if got := bails(t, p, reg); len(got) != 1 || got[0] != "vm:generic-claim-drift" || ran {
			t.Errorf("want the claim-drift defer before the handler runs, got %v ran=%v", got, ran)
		}
	})
	t.Run("a longer live claim is a drift", func(t *testing.T) {
		// The record claimed k alone (NFwd 1; the 1 beyond the claim has its
		// own lowered code) with its second operand from the stack; the live
		// all-forward overload claims k AND the 1 — which would then run
		// twice.
		p, reg := genericWorld(t)
		beneath(p)
		p.Regions[0].NFwd = 1
		p.Fns[0].Code = []compiler.Instr{{Op: compiler.OpPushConst, Arg: 2}, {Op: compiler.OpPushConst, Arg: 1}, {Op: compiler.OpDispatchGeneric, Arg: 0}, {Op: compiler.OpRet}}
		if got := bails(t, p, reg); len(got) != 1 || got[0] != "vm:generic-claim-drift" {
			t.Errorf("want the claim-drift defer, got %v", got)
		}
	})
	t.Run("a live plan of another arity is a drift", func(t *testing.T) {
		// The record's arity is 2 (k forward, one from the stack); the live
		// overload takes k alone — the same forward claim, a stack half the
		// following code was lowered to find consumed.
		p, _ := genericWorld(t)
		reg := seam7Reg(t)
		reg.Defs.Push("k", core.NewInteger(5))
		reg.Defs.Push("w", core.NewFunction(core.FnDefInfo{Name: "w", Signatures: []core.Signature{{Args: []*core.Type{core.TAny}, BarrierPos: 1, Impl: core.Boru([]core.Value{core.NewWord("a")})}}}))
		beneath(p)
		p.Regions[0].NFwd = 1
		p.Fns[0].Code = []compiler.Instr{{Op: compiler.OpPushConst, Arg: 2}, {Op: compiler.OpPushConst, Arg: 1}, {Op: compiler.OpDispatchGeneric, Arg: 0}, {Op: compiler.OpRet}}
		if got := bails(t, p, reg); len(got) != 1 || got[0] != "vm:generic-claim-drift" {
			t.Errorf("want the claim-drift defer, got %v", got)
		}
	})
	t.Run("a full-stack native defers before its handler runs", func(t *testing.T) {
		p, reg := genericWorld(t)
		ran := false
		impl := core.Go(func([]core.Value, map[string]core.Value, []core.Value, *core.Registry) ([]core.Value, error) {
			ran = true
			return []core.Value{core.NewInteger(0)}, nil
		}, core.FullStack())
		p.Generics[0].Impl = impl
		reg.Defs.Push("w", core.NewFunction(core.FnDefInfo{Name: "w", Signatures: []core.Signature{{Args: []*core.Type{core.TAny, core.TAny}, BarrierPos: 2, Impl: impl}}}))
		if got := bails(t, p, reg); len(got) != 1 || got[0] != "vm:generic-full-stack" || ran {
			t.Errorf("a handler that reads the whole resolved stack is never called with a window it did not get: got %v ran=%v", got, ran)
		}
	})
	t.Run("a native overload the record did not take defers before it runs", func(t *testing.T) {
		// No identity on the spec (the user seat), an ordinary native: its
		// result count is unknown until it runs, and an effect it performed
		// would fence the fallback — so it does not run.
		p, reg := genericWorld(t)
		ran := false
		rebindNative(reg, []*core.Type{core.TAny, core.TAny}, func([]core.Value, map[string]core.Value, []core.Value, *core.Registry) ([]core.Value, error) {
			ran = true
			return nil, nil
		})
		if got := bails(t, p, reg); len(got) != 1 || got[0] != "vm:generic-foreign-native" || ran {
			t.Errorf("want the foreign-native defer before the handler, got %v ran=%v", got, ran)
		}
	})
	t.Run("a poly record's live table is its set", func(t *testing.T) {
		// The record was a poly native dispatch (LiveSet): any overload the
		// live match selects runs, as CALL_NATIVE_POLY would run it, and the
		// result count is checked after — the seat's own discipline.
		p, reg := genericWorld(t)
		p.Generics[0].LiveSet = true
		rebindNative(reg, []*core.Type{core.TAny, core.TAny}, func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
			a, _ := core.AsInteger(args[0])
			return []core.Value{core.NewInteger(a * 2)}, nil
		})
		out, err := RunProgram(p, reg)
		if err != nil || len(out) != 1 || !core.ValuesEqual(out[0], core.NewInteger(10)) {
			t.Errorf("a live overload of a poly record runs over the live operands: out=%v err=%v", out, err)
		}
		p, reg = genericWorld(t)
		p.Generics[0].LiveSet = true
		rebindNative(reg, []*core.Type{core.TAny, core.TAny}, func([]core.Value, map[string]core.Value, []core.Value, *core.Registry) ([]core.Value, error) {
			return nil, nil
		})
		if got := bails(t, p, reg); len(got) != 1 || got[0] != "vm:generic-nout-drift" {
			t.Errorf("its result count is checked after it runs: got %v", got)
		}
	})
	t.Run("a pure native overload runs and is checked after", func(t *testing.T) {
		pure := func(reg *core.Registry, h core.Handler) {
			reg.Defs.Push("w", core.NewFunction(core.FnDefInfo{Name: "w", Signatures: []core.Signature{{
				Args: []*core.Type{core.TAny, core.TAny}, BarrierPos: 2, Impl: core.Go(h), CompileEffect: core.CompileIslandPure,
			}}}))
		}
		p, reg := genericWorld(t)
		pure(reg, func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
			a, _ := core.AsInteger(args[0])
			return []core.Value{core.NewInteger(a + 100)}, nil
		})
		out, err := RunProgram(p, reg)
		if err != nil || len(out) != 1 || !core.ValuesEqual(out[0], core.NewInteger(105)) {
			t.Errorf("a pure overload has nothing to fence: it runs over the live operands: out=%v err=%v", out, err)
		}
		p, reg = genericWorld(t)
		pure(reg, func([]core.Value, map[string]core.Value, []core.Value, *core.Registry) ([]core.Value, error) {
			return nil, nil
		})
		if got := bails(t, p, reg); len(got) != 1 || got[0] != "vm:generic-nout-drift" {
			t.Errorf("its result count is checked after it runs: got %v", got)
		}
	})
	t.Run("the lead's modifiers are honoured", func(t *testing.T) {
		// w's only overload takes its second operand from the stack
		// (barrier 1), a 9 lies beneath the claim, and the record was
		// `9 w/f k 1`: both operands forward, the 9 left alone. With the
		// modifier on the descriptor the live walk claims both and enters
		// the unit over the 9; without it the walk takes the 9 for the
		// second operand — one forward where the record claimed two, the
		// claim guard's defer.
		mixed := func() (*compiler.Program, *core.Registry) {
			p, reg := genericWorld(t)
			beneath(p)
			reg.Defs.Push("w", core.NewFunction(core.FnDefInfo{Name: "w", Signatures: []core.Signature{{
				Args: []*core.Type{core.TAny, core.TAny}, BarrierPos: 1, Impl: core.Boru([]core.Value{core.NewWord("a")}),
			}}}))
			return p, reg
		}
		p, reg := mixed()
		p.Regions[0].Mods = &core.WordInfo{Name: "w", ArgCount: -1, ForceForward: true}
		out, err := RunProgram(p, reg)
		if err != nil || len(out) != 2 || !core.ValuesEqual(out[0], core.NewInteger(9)) || !core.ValuesEqual(out[1], core.NewInteger(5)) {
			t.Errorf("`9 w/f k 1` over a mixed barrier claims both forward, as the record did: out=%v err=%v", out, err)
		}
		p, reg = mixed()
		if got := bails(t, p, reg); len(got) != 1 || got[0] != "vm:generic-claim-drift" {
			t.Errorf("the plain lead over the same overload takes the 9 and drifts: %v", got)
		}
	})
	t.Run("the unit's identity includes its parameter patterns", func(t *testing.T) {
		zero, one := core.NewInteger(0), core.NewInteger(1)
		with := func(unitPat *core.Value, sigPat map[int]core.Value) (*compiler.Program, *core.Registry) {
			p, reg := genericWorld(t)
			p.Fns[1].ParamPatterns = []*core.Value{unitPat, nil}
			reg.Defs.Push("k", zero)
			reg.Defs.Push("w", core.NewFunction(core.FnDefInfo{Name: "w", Signatures: []core.Signature{{
				Args: []*core.Type{core.TAny, core.TAny}, Patterns: sigPat, BarrierPos: 2, Impl: core.Boru([]core.Value{core.NewWord("a")}),
			}}}))
			return p, reg
		}
		p, reg := with(&zero, map[int]core.Value{0: zero})
		if out, err := RunProgram(p, reg); err != nil || len(out) != 1 || !core.ValuesEqual(out[0], zero) {
			t.Errorf("the same pattern on both sides is the committed unit: out=%v err=%v", out, err)
		}
		for name, c := range map[string]struct {
			unit *core.Value
			sig  map[int]core.Value
		}{
			"the unit's pattern where the live signature has none": {&zero, nil},
			"a pattern on the live signature the unit lacks":       {nil, map[int]core.Value{0: zero}},
			"a different pattern":                                  {&zero, map[int]core.Value{0: one}},
		} {
			p, reg := with(c.unit, c.sig)
			if got := bails(t, p, reg); len(got) != 1 || got[0] != "vm:generic-foreign-unit" {
				t.Errorf("%s: not the committed unit, got %v", name, got)
			}
		}
	})
}
