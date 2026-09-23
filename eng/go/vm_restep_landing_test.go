package eng

import (
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// OpReStepLanding (NUR173) is the guarded landing of a reach-lowered group's
// single survivor: the interpreter's rewind lands on it and RE-STEPS it, so a
// callable value dispatches there and anything else stays put. The op's ladder
// is callDynTrailTop's over an EMPTY window, minus the no-match raise — a fn
// whose signatures do not take zero arguments is what the interpreter leaves
// as DATA at a landing, so a failed match is an answer, not an error.
//
// The corpus reaches the two rungs that matter for the fix (data, and entering
// a matched compiled unit); the rest are pinned here at the seam, because
// which rung a landing takes is a property of the runtime VALUE and the corpus
// cannot be written to produce one of each on demand.

// landingProg builds a program whose single op is the landing, over whatever
// the caller has already put on the stack.
func landingProg() *compiler.Program {
	return &compiler.Program{
		Code:  []compiler.Instr{{Op: compiler.OpReStepLanding}},
		Debug: []core.SrcPos{{}},
	}
}

// landingNativeReg registers 0-arg NATIVE words and parks a reference to one,
// which is the shape a landing actually dispatches. A NAMED 0-arg fn value is
// the interpreter's property call; an ANONYMOUS one PARKS (execFnDefLiteral's
// anonymous-0-arg rule), so a self-contained Go fn value — anonymous by
// definition — can never reach the native rung here, and a fixture built from
// one would be pinning behaviour the interpreter does not have.
func landingNativeReg(t *testing.T, name string, out func() ([]core.Value, error)) (*core.Registry, core.Value) {
	t.Helper()
	r := seam7Reg(t)
	r.Register(name, core.Signature{
		BarrierPos: -1,
		Returns:    []*core.Type{core.TInteger},
		Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
			return out()
		}),
	})
	if err := r.Err(); err != nil {
		t.Fatalf("registration: %v", err)
	}
	inner := r.Lookup(name)
	if inner == nil {
		t.Fatalf("%s did not register", name)
	}
	fd := core.FnDefInfo{Name: name, Registry: r, Signatures: inner.Signatures}
	if !vmNativeApplicable(r, fd) {
		t.Fatalf("%s fixture does not read as natively applicable", name)
	}
	return r, core.NewFunction(fd)
}

// TestReStepLandingClosureArm: a compiled CLOSURE at the landing is invoked
// through invokeClosure with no arguments, exactly as the interpreter's
// re-step dispatches a 0-arg closure value.
func TestReStepLandingClosureArm(t *testing.T) {
	r := seam7Reg(t)
	p := landingProg()
	p.Consts = []core.Value{core.NewInteger(7)}
	p.Fns = []compiler.CompiledFn{{
		Name: "k7", NParams: 0, NLocals: 0, Returns: []*core.Type{core.TAny},
		Code:  []compiler.Instr{{Op: compiler.OpPushConst, Arg: 0}, {Op: compiler.OpRet}},
		Debug: []core.SrcPos{{}, {}},
	}}
	vc := &vmContext{p: p, r: r, ceiling: 1 << 20, stepLimit: 1 << 20}
	cl := core.NewValueRaw(core.TFunction, core.ClosurePayload{Prog: p, Unit: 0, Ident: core.NewFnIdentity()})
	got, ent, err := vc.reStepLanding(r, 0, 0, []core.Value{cl}, seam7Dbg, 0, compiler.LandingWord{})
	if err != nil || ent != nil {
		t.Fatalf("closure landing: %v %v", ent, err)
	}
	if len(got) != 1 {
		t.Fatalf("closure landing residual = %v, want one value", got)
	}
	if n, _ := got[0].AsConcreteInteger(); n != 7 {
		t.Errorf("closure landing produced %v, want the closure's 7", got[0])
	}
}

// TestReStepLandingNativeArm: a parked NATIVE fn value with a 0-arg signature
// applies natively, without an island — an island is an interpreter entry and
// the censuses count every one.
func TestReStepLandingNativeArm(t *testing.T) {
	r, fn := landingNativeReg(t, "zz-landing-eleven", func() ([]core.Value, error) {
		return []core.Value{core.NewInteger(11)}, nil
	})
	vc := &vmContext{p: landingProg(), r: r, ceiling: 1 << 20, stepLimit: 1 << 20}
	got, ent, err := vc.reStepLanding(r, 0, 0, []core.Value{fn}, seam7Dbg, 0, compiler.LandingWord{})
	if err != nil || ent != nil {
		t.Fatalf("native landing: %v %v", ent, err)
	}
	if len(got) != 1 {
		t.Fatalf("native landing residual = %v, want one value", got)
	}
	if n, _ := got[0].AsConcreteInteger(); n != 11 {
		t.Errorf("native landing produced %v, want the handler's 11", got[0])
	}
}

// TestReStepLandingAnonymousParks: the interpreter's ANONYMOUS-0-ARG PARK,
// which the landing must mirror in BOTH representations a lambda arrives in.
// `def f ([] => [body])` binds the function and not the body's result, and a
// landing that applied it anyway turned `def p (FnUtil.partial f/v 10) end (p)`
// into a run-time claim failure (module-fn.tsv:L47) — the wrapper had already
// been spent by the time the user's own paren called it.
func TestReStepLandingAnonymousParks(t *testing.T) {
	r := seam7Reg(t)

	// The FnDefInfo representation: a self-contained Go fn value, which is
	// anonymous by definition (IsSelfContainedGoFnDef requires it).
	fd := core.FnDefInfo{
		Anonymous: true,
		Signatures: []core.Signature{{
			BarrierPos: -1,
			Returns:    []*core.Type{core.TInteger},
			Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
				return []core.Value{core.NewInteger(11)}, nil
			}),
		}},
	}
	if !core.IsSelfContainedGoFnDef(fd) {
		t.Fatal("fixture is not a self-contained Go fn value")
	}
	vc := &vmContext{p: landingProg(), r: r, ceiling: 1 << 20, stepLimit: 1 << 20}
	got, ent, err := vc.reStepLanding(r, 0, 0, []core.Value{core.NewFunction(fd)}, seam7Dbg, 0, compiler.LandingWord{})
	if err != nil || ent != nil {
		t.Fatalf("anonymous landing: %v %v", ent, err)
	}
	if len(got) != 1 || !got[0].Parent.Equal(core.TFunction) {
		t.Errorf("anonymous landing = %v, want the value parked as data", got)
	}

	// The CLOSURE representation: a lambda UNIT (CompiledFn.Lambda), which is
	// what closureFnDef reads FnDefInfo.Anonymous off for the interpreter.
	p := landingProg()
	p.Consts = []core.Value{core.NewInteger(7)}
	p.Fns = []compiler.CompiledFn{{
		Name: "lam", NParams: 0, NLocals: 0, Lambda: true, Returns: []*core.Type{core.TAny},
		Code:  []compiler.Instr{{Op: compiler.OpPushConst, Arg: 0}, {Op: compiler.OpRet}},
		Debug: []core.SrcPos{{}, {}},
	}}
	vcl := &vmContext{p: p, r: r, ceiling: 1 << 20, stepLimit: 1 << 20}
	cl := core.NewValueRaw(core.TFunction, core.ClosurePayload{
		Prog: p, Unit: 0, InShape: compiler.ClosureInValue, Ident: core.NewFnIdentity()})
	if !compiler.ClosureIsFnValue(cl) {
		t.Fatal("fixture is not a fn-VALUE closure — the lambda rung would not consult it")
	}
	gotc, entc, errc := vcl.reStepLanding(r, 0, 0, []core.Value{cl}, seam7Dbg, 0, compiler.LandingWord{})
	if errc != nil || entc != nil {
		t.Fatalf("lambda landing: %v %v", entc, errc)
	}
	if len(gotc) != 1 || !gotc[0].Parent.Equal(core.TFunction) {
		t.Errorf("lambda landing = %v, want the closure parked as data", gotc)
	}
}

// TestReStepLandingDriftDefers: the recorded landing claims ONE value, so a
// member whose apply nets another count is a claim failure — loud, at the
// landing, rather than a stack the caller cannot reconcile.
//
// The fixture declares ONE return and hands back TWO, which is the only way to
// reach this arm now that NUR175 screens on the DECLARED count before applying:
// a member whose signature admits a count other than one stands aside earlier
// and never gets here. Drift is what is left — the handler disagreeing with its
// own signature — and that is a fault, not a shape to stand aside from.
func TestReStepLandingDriftDefers(t *testing.T) {
	r, fn := landingNativeReg(t, "zz-landing-two", func() ([]core.Value, error) {
		return []core.Value{core.NewInteger(1), core.NewInteger(2)}, nil
	})
	vc := &vmContext{p: landingProg(), r: r, ceiling: 1 << 20, stepLimit: 1 << 20}
	_, _, err := vc.reStepLanding(r, 0, 0, []core.Value{fn}, seam7Dbg, 0, compiler.LandingWord{})
	wantInternal(t, err, "recorded landing claims one")
}

// TestReStepLandingNonFnValueClosureInvokes: the closure rung is NOT the whole
// closure family. Only a fn-VALUE closure parks (see above); a unit that is not
// one — a token body, an iteration handler — has no anonymous-value rule to
// mirror and is invoked, which is what makes `def m (mk) end m.f` reach its
// member at all when the member compiled to a unit.
func TestReStepLandingNonFnValueClosureInvokes(t *testing.T) {
	r := seam7Reg(t)
	p := landingProg()
	p.Consts = []core.Value{core.NewInteger(7)}
	p.Fns = []compiler.CompiledFn{{
		Name: "body", NParams: 0, NLocals: 0, Returns: []*core.Type{core.TAny},
		Code:  []compiler.Instr{{Op: compiler.OpPushConst, Arg: 0}, {Op: compiler.OpRet}},
		Debug: []core.SrcPos{{}, {}},
	}}
	vc := &vmContext{p: p, r: r, ceiling: 1 << 20, stepLimit: 1 << 20}
	cl := core.NewValueRaw(core.TFunction, core.ClosurePayload{Prog: p, Unit: 0, Ident: core.NewFnIdentity()})
	if compiler.ClosureIsFnValue(cl) {
		t.Fatal("fixture reads as a fn-VALUE closure — it would park, not invoke")
	}
	got, ent, err := vc.reStepLanding(r, 0, 0, []core.Value{cl}, seam7Dbg, 0, compiler.LandingWord{})
	if err != nil || ent != nil {
		t.Fatalf("non-fn-value closure landing: %v %v", ent, err)
	}
	if n, _ := got[0].AsConcreteInteger(); n != 7 {
		t.Errorf("non-fn-value closure landing = %v, want the unit's 7", got)
	}
}

// TestReStepLandingStaysData: the three ways a landing leaves the value
// exactly where it is, which is what makes the op affordable on every
// container read. A QUOTED fn is inert on both lanes; a non-fn is not a
// callee at all; and a fn with no signature satisfiable at ZERO arguments is
// what the interpreter's own re-step leaves as data — no no-match raised,
// which is the clause OpCallDynTrailTop over zero args could not give.
func TestReStepLandingStaysData(t *testing.T) {
	r := seam7Reg(t)
	vc := &vmContext{p: landingProg(), r: r, ceiling: 1 << 20, stepLimit: 1 << 20}
	// A real 1-arg fn value: MatchFnSig reads Params, which is what a `def fn
	// [[n:Integer] …]` value carries, so this is the shape a container read
	// actually surfaces.
	oneArg := core.NewFunction(core.FnDefInfo{
		Name: "needs-one",
		Signatures: []core.Signature{{
			Args: []*core.Type{core.TInteger}, BarrierPos: -1,
			Params: []core.FnParam{{Name: "n", Type: core.TInteger}},
		}},
	})
	quoted := oneArg
	quoted.Quoted = true
	for _, tc := range []struct {
		name string
		v    core.Value
		why  string
	}{
		{"a quoted fn", quoted, "a quoted value is inert on both lanes — stepLiteral pushes it"},
		{"a non-callable", core.NewInteger(3), "not a callee at all; the op costs it one type test"},
		{"no 0-arg overload", oneArg, "the interpreter's re-step leaves it as data, and raises nothing"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := []core.Value{core.NewString("below"), tc.v}
			got, ent, err := vc.reStepLanding(r, 0, 0, in, seam7Dbg, 0, compiler.LandingWord{})
			if err != nil || ent != nil {
				t.Fatalf("%s must not apply: %v %v — %s", tc.name, ent, err, tc.why)
			}
			if len(got) != 2 || !got[1].Parent.Equal(tc.v.Parent) {
				t.Errorf("%s: stack = %v, want it untouched — %s", tc.name, got, tc.why)
			}
		})
	}
}

// TestReStepLandingIslandArm: a value the VM cannot take falls to the island,
// which is the interpreter's own one-token re-step and therefore always the
// right answer. It is the LAST rung precisely because an island is an
// interpreter entry and the censuses count every one.
//
// A bare `Function` TYPE LITERAL is the honest fixture: IsAppliableFn admits
// it by lattice tag, it carries no FnDefInfo to match or enter against, and
// the island answers exactly as the interpreter does — stepLiteral pushes a
// Function value with no payload rather than dispatching it.
func TestReStepLandingIslandArm(t *testing.T) {
	r := seam7Reg(t)
	vc := &vmContext{p: landingProg(), r: r, ceiling: 1 << 20, stepLimit: 1 << 20}
	lit := core.Value{Parent: core.TFunction}
	if !core.IsAppliableFn(lit) {
		t.Fatal("fixture is not appliable — the landing would leave it as data before the island")
	}
	if _, isFn := lit.Data.(core.FnDefInfo); isFn {
		t.Fatal("fixture carries an FnDefInfo — it would take an earlier rung")
	}
	got, ent, err := vc.reStepLanding(r, 0, 0, []core.Value{lit}, seam7Dbg, 0, compiler.LandingWord{})
	if err != nil || ent != nil {
		t.Fatalf("island landing: %v %v", ent, err)
	}
	if len(got) != 1 || !got[0].Parent.Equal(core.TFunction) {
		t.Errorf("island landing = %v, want the value the interpreter pushes", got)
	}
}

// TestReStepLandingUnderflow: the landing reads the value the producing event
// just left, so an empty stack is a bytecode-level fault rather than a shape
// the op can answer. It carried a //covergate:allow pragma claiming to be
// unreachable until the merged ADR-008 gate reported it COVERED — an
// allowlisted guard that something reaches is a claim the gate is right to
// reject, so the arm is asserted here instead of excused.
func TestReStepLandingUnderflow(t *testing.T) {
	r := seam7Reg(t)
	vc := &vmContext{p: landingProg(), r: r, ceiling: 1 << 20, stepLimit: 1 << 20}
	if _, _, err := vc.reStepLanding(r, 0, 0, nil, seam7Dbg, 0, compiler.LandingWord{}); err == nil {
		t.Error("an empty stack at the landing must error, not answer")
	}
}

// TestReStepLandingErrorArms: a raise from inside the applied member is the
// member's own error and surfaces stamped at the landing, on every rung that
// can run code. The interpreter raises the same at the same point, prior side
// effects included.
func TestReStepLandingErrorArms(t *testing.T) {
	r := seam7Reg(t)

	// The CLOSURE rung: a 0-param unit whose body raises.
	p := landingProg()
	p.Traps = []compiler.TrapSpec{{Code: "bad_input", Detail: "landing closure raised", Word: "boom"}}
	p.Fns = []compiler.CompiledFn{{
		Name: "raiser", NParams: 0, NLocals: 0, Returns: []*core.Type{core.TAny},
		Code:  []compiler.Instr{{Op: compiler.OpTrap, Arg: 0}},
		Debug: []core.SrcPos{{}},
	}}
	vc := &vmContext{p: p, r: r, ceiling: 1 << 20, stepLimit: 1 << 20}
	cl := core.NewValueRaw(core.TFunction, core.ClosurePayload{Prog: p, Unit: 0, Ident: core.NewFnIdentity()})
	if _, _, err := vc.reStepLanding(r, 0, 0, []core.Value{cl}, seam7Dbg, 0, compiler.LandingWord{}); err == nil {
		t.Error("a raising closure member must surface its error, not a stack")
	}

	// The NATIVE rung: a parked native whose handler raises.
	rb, boom := landingNativeReg(t, "zz-landing-boom", func() ([]core.Value, error) {
		return nil, core.MakeBoruError("bad_input", "landing handler raised", "boom", "", "")
	})
	vcb := &vmContext{p: landingProg(), r: rb, ceiling: 1 << 20, stepLimit: 1 << 20}
	if _, _, err := vcb.reStepLanding(rb, 0, 0, []core.Value{boom}, seam7Dbg, 0, compiler.LandingWord{}); err == nil {
		t.Error("a raising member must surface its error at the landing")
	}

	// The ISLAND rung: a boru-bodied 0-arg value the VM cannot take, whose
	// body does not resolve here. The island IS the interpreter's re-step, so
	// whatever it raises is the member's own error and surfaces stamped at the
	// landing — the same discipline as the two rungs above, at the rung that
	// runs the interpreter rather than the VM.
	islandRaiser := core.FnDefInfo{
		Name: "island-raiser",
		Signatures: []core.Signature{{
			BarrierPos: -1,
			Returns:    []*core.Type{core.TInteger},
			// NOT a single-word body: that shape is a trivial DELEGATION
			// wrapper, which vmNativeApplicable admits, and the value would
			// take the native rung instead of this one.
			Impl: core.Boru([]core.Value{core.NewInteger(9)}),
		}},
	}
	if vmNativeApplicable(r, islandRaiser) {
		t.Fatal("fixture reads as natively applicable — it would take the native arm, not the island")
	}
	if _, _, err := vc.reStepLanding(r, 0, 0, []core.Value{core.NewFunction(islandRaiser)}, seam7Dbg, 0, compiler.LandingWord{}); err == nil {
		t.Error("an island that raises must surface the error, not a stack")
	}
}
