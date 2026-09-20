package eng

import (
	"strings"
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// oneConstProg is a standalone one-unit program that returns n — the shape
// StampDetachedSig produces for a capture-free body, minus the compile.
func oneConstProg(n int64) *compiler.Program {
	return &compiler.Program{
		Consts: []core.Value{core.NewInteger(n)},
		Fns: []compiler.CompiledFn{{
			Name:  "constN",
			Code:  []compiler.Instr{{Op: compiler.OpPushConst, Arg: 0}, {Op: compiler.OpRet, Arg: 0}},
			Debug: []core.SrcPos{{Row: 1, Col: 1}, {Row: 1, Col: 1}},
		}},
	}
}

func foreignVC(t *testing.T, running *compiler.Program) (*vmContext, *core.Registry) {
	t.Helper()
	r := stampReg(t)
	return &vmContext{
		p: running, r: r,
		ceiling:   vmStackCeiling(r),
		stepLimit: core.DefaultStepLimit,
	}, r
}

// A ref belonging to ANOTHER program is hosted, not declined — the fix for the
// seam that made every runtime-stamped body report Stamped:true and then run on
// the interpreter. The enclosing run's body seams are restored on the way out,
// and the step budget it spent comes back with it (a per-callback reset would
// hand a hot callback a fresh runaway budget on every invoke).
func TestRunUnitNestedHostsForeignProgram(t *testing.T) {
	vc, r := foreignVC(t, oneConstProg(1))
	prevInvoker, prevNested := r.Invoker, r.NestedRunner
	ref := &compiler.CompiledFnRef{Prog: oneConstProg(42), Unit: 0}

	res, handled, err := vc.runUnitNested(ref, nil)
	if err != nil || !handled {
		t.Fatalf("runUnitNested: handled=%v err=%v", handled, err)
	}
	if len(res) != 1 {
		t.Fatalf("got %d results, want 1", len(res))
	}
	if n, _ := res[0].AsConcreteInteger(); n != 42 {
		t.Fatalf("foreign unit must run its OWN program (42), got %v", res[0])
	}
	if vc.steps == 0 {
		t.Error("the foreign run's steps must be handed back to the enclosing context")
	}
	if r.Invoker != nil || r.NestedRunner != nil {
		t.Error("the foreign run must restore the enclosing body seams")
	}
	_, _ = prevInvoker, prevNested
}

// A DynEnv foreign program takes the args-bracket rebalance. The depth is
// unchanged either way here; what the row pins is that the bracket is ARMED for
// a foreign unit, so an error unwind out of one cannot leak args entries.
func TestRunUnitNestedForeignDynEnvRebalances(t *testing.T) {
	vc, r := foreignVC(t, oneConstProg(1))
	foreign := oneConstProg(7)
	foreign.DynEnv = true
	depth := r.Args.Depth()

	res, handled, err := vc.runUnitNested(&compiler.CompiledFnRef{Prog: foreign, Unit: 0}, nil)
	if err != nil || !handled {
		t.Fatalf("runUnitNested: handled=%v err=%v", handled, err)
	}
	if n, _ := res[0].AsConcreteInteger(); n != 7 {
		t.Fatalf("got %v, want 7", res[0])
	}
	if got := r.Args.Depth(); got != depth {
		t.Fatalf("args depth %d, want %d", got, depth)
	}
}

// A soundness bailout inside ONE foreign callback is contained to THAT
// callback, which reports it. Without the local recover the panic would unwind
// to the enclosing runVMEntry and abort the whole program instead.
func TestRunUnitNestedForeignPanicIsContained(t *testing.T) {
	vc, _ := foreignVC(t, oneConstProg(1))
	broken := oneConstProg(1)
	broken.Fns[0].Code[0].Arg = 99 // const index past the table → index panic

	res, handled, err := vc.runUnitNested(&compiler.CompiledFnRef{Prog: broken, Unit: 0}, nil)
	if !handled {
		t.Fatal("a panicking foreign unit is handled (as an internal_error), not declined")
	}
	if res != nil {
		t.Fatalf("got %v results, want none", res)
	}
	if !core.IsInternalError(err) || !strings.Contains(err.Error(), "internal bytecode VM error") {
		t.Fatalf("err = %v, want an internal_error VM bailout", err)
	}
}

// The declines: a payload that is not a ref at all, and a ref whose unit index
// is outside its OWN program's table (a compile/run drift). Both leave the
// callback to the interpreter rather than indexing a table that cannot answer.
func TestRunUnitNestedDeclines(t *testing.T) {
	vc, _ := foreignVC(t, oneConstProg(1))
	for _, tc := range []struct {
		name string
		h    any
	}{
		{"not a ref", 42},
		{"nil program", &compiler.CompiledFnRef{Unit: 0}},
		{"unit index past the table", &compiler.CompiledFnRef{Prog: oneConstProg(1), Unit: 9}},
		{"negative unit index", &compiler.CompiledFnRef{Prog: oneConstProg(1), Unit: -1}},
	} {
		res, handled, err := vc.runUnitNested(tc.h, nil)
		if handled || res != nil || err != nil {
			t.Errorf("%s: got (%v, %v, %v), want a clean decline", tc.name, res, handled, err)
		}
	}
}

// The foreign run installs the body-closure invoker on any module registry its
// unit dispatches into, and must clear those on the way out. Skipping the
// cleanup would leave a module registry holding an invoker bound to a program
// that is no longer running — a later interpreter dispatch there would enter a
// dead unit table. runVMEntry's own cleanup cannot help: it clears ITS list,
// not the nested run's.
func TestRunUnitNestedForeignClearsForeignInvokers(t *testing.T) {
	vc, _ := foreignVC(t, oneConstProg(1))
	modReg := stampReg(t) // stands in for a module sub-registry

	installed := false
	sig := core.Signature{
		Args: []*core.Type{core.TInteger}, Returns: []*core.Type{core.TInteger}, BarrierPos: -1,
		Impl: core.Go(func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
			// ensureInvoker runs just before the handler, so this observes the
			// install the cleanup below has to undo — without it the assertion
			// at the end would pass vacuously.
			installed = modReg.Invoker != nil
			return []core.Value{args[0]}, nil
		}),
	}
	foreign := &compiler.Program{
		Consts: []core.Value{core.NewInteger(3)},
		Sigs:   []compiler.SigRef{{Word: "zz-foreign-native", Sig: &sig}},
		Fns: []compiler.CompiledFn{{
			Name: "callsIntoModule",
			Reg:  modReg, // a FOREIGN dispatch registry → ensureInvoker records it
			Code: []compiler.Instr{
				{Op: compiler.OpPushConst, Arg: 0},
				{Op: compiler.OpCallNative, Arg: 0},
				{Op: compiler.OpRet, Arg: 0},
			},
			Debug: make([]core.SrcPos, 3),
		}},
	}

	res, handled, err := vc.runUnitNested(&compiler.CompiledFnRef{Prog: foreign, Unit: 0}, nil)
	if err != nil || !handled {
		t.Fatalf("runUnitNested: handled=%v err=%v", handled, err)
	}
	if n, _ := res[0].AsConcreteInteger(); n != 3 {
		t.Fatalf("got %v, want 3", res[0])
	}
	if !installed {
		t.Fatal("the foreign run never installed an invoker on the module registry — " +
			"the cleanup this row exists for was not exercised")
	}
	if modReg.Invoker != nil {
		t.Error("the foreign run left its invoker installed on a module registry")
	}
}

// A closure VALUE carries the program its Unit indexes, and the invoker routes
// by it. Both rows use unit 0 of a DIFFERENT program returning a different
// constant, so an invoke that ignored the identity would answer the running
// program's body — a valid index naming the wrong body, which is the silent
// wrong answer this field exists to make impossible.
//
// Before nested foreign hosting a closure could only be invoked under the
// program that pushed it, so the payload carried no identity and none was
// needed. It is needed now: while a detached unit runs, the registry's Invoker
// points at the foreign program's context.
func TestInvokeClosureRoutesByItsOwnProgram(t *testing.T) {
	vc, r := foreignVC(t, oneConstProg(11))
	foreign := oneConstProg(22)

	res, err := vc.invokeClosureOn(r, compiler.NewClosure(foreign, 0, nil), nil)
	if err != nil {
		t.Fatalf("invokeClosureOn (foreign): %v", err)
	}
	if n, _ := res[0].AsConcreteInteger(); n != 22 {
		t.Fatalf("a foreign closure must run ITS program's unit (22), got %v", res[0])
	}
	if r.Invoker != nil {
		t.Error("the foreign closure run must restore the enclosing body seam")
	}

	// The same-program twin still enters directly, with no nesting.
	res, err = vc.invokeClosureOn(r, compiler.NewClosure(vc.p, 0, nil), nil)
	if err != nil {
		t.Fatalf("invokeClosureOn (own): %v", err)
	}
	if n, _ := res[0].AsConcreteInteger(); n != 11 {
		t.Fatalf("an own-program closure runs vc.p's unit (11), got %v", res[0])
	}

	// No identity recorded reads as the running program — what every closure
	// did before the field existed.
	res, err = vc.invokeClosureOn(r, compiler.NewClosure(nil, 0, nil), nil)
	if err != nil {
		t.Fatalf("invokeClosureOn (no identity): %v", err)
	}
	if n, _ := res[0].AsConcreteInteger(); n != 11 {
		t.Fatalf("an identity-less closure runs the RUNNING program (11), got %v", res[0])
	}
}
