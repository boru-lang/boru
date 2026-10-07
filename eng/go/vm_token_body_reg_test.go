package eng

import (
	"strings"
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// TestTokenBodyHostsOnCallingRegistry: a run-time-stamped token body's unit
// is OWNERLESS (CompiledFn.Reg nil — the synthetic fn it compiled as has no
// home), and its home is the CALLING registry: the one RunResolved would step
// the tokens on, where the enclosing unit installed the names the body reads.
// Hosted on the RUNNING registry instead, a dyn-scope read of a name a module
// fn's unit installed on its module registry missed — kg/validate.boru's
// check-code-units, `filter [eq ev.source_id] code-source-ids` inside a `[ev]`
// lambda of a module fn, raised `undefined word: ev` compiled where the
// interpreter answered (2026-10-07). The seam hosts the unit for the calling
// registry (hostForeignOn); the plain host keeps the running registry, and
// the same read misses there.
func TestTokenBodyHostsOnCallingRegistry(t *testing.T) {
	root, err := core.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	mod, err := core.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	mod.Defs.Push("ev", core.NewInteger(7))
	p := &compiler.Program{
		Consts:        []core.Value{core.NewString("ev")},
		LiveReadNames: map[string]bool{"ev": true},
		Fns: []compiler.CompiledFn{{
			Name:  "codebody",
			Code:  []compiler.Instr{{Op: compiler.OpLookupDynScope, Arg: 0}, {Op: compiler.OpRet}},
			Debug: []core.SrcPos{{}, {}},
		}},
	}
	vc := &vmContext{p: &compiler.Program{}, r: root, ceiling: 1 << 20, stepLimit: 1 << 20}
	res, err := vc.hostForeignOn(p, mod, 0, nil, nil, true, mod)
	if err != nil || len(res) != 1 || res[0].String() != "7" {
		t.Fatalf("hosted for the calling registry: got %v, %v; want [7]", res, err)
	}
	// The running registry never saw the install.
	if _, err := vc.hostForeign(p, mod, 0, nil, nil, true); err == nil || !strings.Contains(err.Error(), "undefined word: ev") {
		t.Fatalf("hosted on the running registry: err = %v, want the undefined-word miss", err)
	}

	// `args` read live through a native on the calling registry sees the
	// enclosing call's list — the VM keeps every frame's list on the running
	// registry, so the top one is bridged onto the calling registry's stack
	// for the duration and truncated back afterwards (NUR346's `each (mk)
	// [x 5]` with `mk` = `quote [args]` inside a module fn).
	probe := core.Signature{BarrierPos: -1, Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, r *core.Registry) ([]core.Value, error) {
		top, ok, err := r.Args.Top()
		if err != nil || !ok {
			return []core.Value{core.NewString("none")}, nil
		}
		return []core.Value{top}, nil
	})}
	ap := &compiler.Program{
		Sigs: []compiler.SigRef{{Word: "argsprobe", Sig: &probe}},
		Fns: []compiler.CompiledFn{{
			Name:  "codebody",
			Code:  []compiler.Instr{{Op: compiler.OpCallNative, Arg: 0}, {Op: compiler.OpRet}},
			Debug: []core.SrcPos{{}, {}},
		}},
	}
	if res, err := vc.hostForeignOn(ap, mod, 0, nil, nil, true, mod); err != nil || len(res) != 1 || res[0].String() != "'none'" {
		t.Fatalf("no enclosing call: got %v, %v; want ['none']", res, err)
	}
	if err := root.Args.Push(core.NewList([]core.Value{core.NewInteger(7)})); err != nil {
		t.Fatal(err)
	}
	res, err = vc.hostForeignOn(ap, mod, 0, nil, nil, true, mod)
	if err != nil || len(res) != 1 || res[0].String() != "[7]" {
		t.Fatalf("inside a call: got %v, %v; want [[7]]", res, err)
	}
	if mod.Args.Depth() != 0 || root.Args.Depth() != 1 {
		t.Fatalf("the bridged frame must be truncated back: mod depth %d, root depth %d", mod.Args.Depth(), root.Args.Depth())
	}
}
