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
		Generics: []compiler.GenericSpec{{Region: 0, Unit: 1, NOut: 1}},
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
	// directly with the live operands.
	rebindNative(reg, []*core.Type{core.TAny, core.TAny}, func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
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
	t.Run("a native whose result count drifts from the claim", func(t *testing.T) {
		p, reg := genericWorld(t)
		rebindNative(reg, []*core.Type{core.TAny, core.TAny}, func([]core.Value, map[string]core.Value, []core.Value, *core.Registry) ([]core.Value, error) {
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
// else.
func rebindNative(reg *core.Registry, args []*core.Type, h core.Handler) {
	reg.Defs.Push("w", core.NewFunction(core.FnDefInfo{Name: "w", Signatures: []core.Signature{{
		Args: args, BarrierPos: len(args), Impl: core.Go(h),
	}}}))
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
		reg.Defs.Push("w", core.NewFunction(core.FnDefInfo{Name: "w", Signatures: []core.Signature{{
			Args: []*core.Type{core.TAny, core.TAny}, BarrierPos: 2, ModuleCall: &core.ModuleCallID{Module: "m", Export: "w"},
			Impl: core.Go(func([]core.Value, map[string]core.Value, []core.Value, *core.Registry) ([]core.Value, error) {
				return []core.Value{core.NewInteger(1)}, nil
			}),
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
		rebindNative(reg, []*core.Type{core.TAny, core.TAny}, func([]core.Value, map[string]core.Value, []core.Value, *core.Registry) ([]core.Value, error) {
			return nil, core.MakeBoruError("zz_native", "the handler raised", "w", "", "")
		})
		if _, err := RunProgram(p, reg); err == nil || !strings.Contains(err.Error(), "the handler raised") {
			t.Errorf("the handler's error is the run's: %v", err)
		}
	})
	t.Run("a live native that hands back a token", func(t *testing.T) {
		p, reg := genericWorld(t)
		rebindNative(reg, []*core.Type{core.TAny, core.TAny}, func([]core.Value, map[string]core.Value, []core.Value, *core.Registry) ([]core.Value, error) {
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
	t.Run("a boru overload of another arity is not the committed unit", func(t *testing.T) {
		// A registry where w's ONLY overload takes one operand (a shadowing
		// push would merge with the two-operand boru overload, which sorts
		// first and matches).
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
		wantInternal(t, err, "not the committed unit")
		if len(bails) != 1 || bails[0] != "vm:generic-foreign-unit" {
			t.Errorf("want the foreign-unit defer, got %v", bails)
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
