package eng

import (
	"errors"
	"strings"
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// cover_pr505_eng_test.go — the VM arms PR #505 added whose trigger is a
// property of the runtime state rather than of a program the corpus can be
// written to produce on demand: the NAMED callback entry on a busy registry,
// the no-match anchor of a named closure that carries no reference position,
// the landing's count and entry guards, the dyn-bind adoption's refusal, and
// the poly no-match's runtime anchor. Each is driven through the VM's own
// seams, the way vm_seam7_test.go and vm_foreign_unit_test.go drive theirs.

// namedPairProg is a program whose unit 0 ("pair") declares ONE Integer
// return and leaves TWO values, and whose main code calls a native that
// invokes that unit through call — the shape a module-fn dispatch reaches
// the compiled runtime in while the program's own run is live.
func namedPairProg(call func(reg *core.Registry, sig *core.Signature) ([]core.Value, error)) *compiler.Program {
	p := &compiler.Program{
		Consts: []core.Value{core.NewInteger(7), core.NewInteger(8)},
		Code:   []compiler.Instr{{Op: compiler.OpCallNative, Arg: 0}},
		Debug:  []core.SrcPos{{}},
		Fns: []compiler.CompiledFn{{
			Name: "pair", Returns: []*core.Type{core.TInteger},
			Code:  []compiler.Instr{{Op: compiler.OpPushConst, Arg: 0}, {Op: compiler.OpPushConst, Arg: 1}, {Op: compiler.OpRet}},
			Debug: make([]core.SrcPos, 3),
		}},
	}
	pairSig := pairSigOf(p)
	native := core.Signature{BarrierPos: -1, Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, reg *core.Registry) ([]core.Value, error) {
		return call(reg, pairSig)
	})}
	p.Sigs = []compiler.SigRef{{Word: "zz-call-pair", Sig: &native}}
	return p
}

// pairSigOf is the stamped signature of p's "pair" unit: the ref the
// compiled runtime runs, over a boru body the interpreter would run instead.
func pairSigOf(p *compiler.Program) *core.Signature {
	body := []core.Value{core.NewInteger(7), core.NewInteger(8)}
	return &core.Signature{Returns: []*core.Type{core.TInteger}, Impl: core.NewBoruImplCompiled(body, &compiler.CompiledFnRef{Prog: p, Unit: 0})}
}

// wantPairCountError asserts the frame's count error for "pair".
func wantPairCountError(t *testing.T, lane string, res []core.Value, err error) {
	t.Helper()
	var be *core.BoruError
	if !errors.As(err, &be) || be.Code != "type_error" || !strings.Contains(be.Detail, "pair: expected 1 return value(s), got 2") {
		t.Errorf("%s: got %v / %v, want the frame's count error", lane, res, err)
	}
}

// wantPairResidual asserts the fn-VALUE seam's untrimmed residual [7 8].
func wantPairResidual(t *testing.T, lane string, res []core.Value, err error) {
	t.Helper()
	if err != nil || len(res) != 2 {
		t.Fatalf("%s: got %v / %v, want the residual [7 8]", lane, res, err)
	}
	if a, _ := res[0].AsConcreteInteger(); a != 7 {
		t.Errorf("%s: got %v, want [7 8]", lane, res)
	}
}

// TestNamedCallbackRootRetTakesTheFramesCount pins NUR191's entry on the
// compiled runtime: a unit entered as a NAMED fn call (InvokeCallbackStrict —
// the module-fn dispatch) returns under the frame's own contract, so a body
// leaving two values under a one-return declaration raises the count error
// the interpreter's CallBoruStrict raises; the fn-VALUE seam (InvokeCallback)
// keeps the CallBoru discipline, types over the aligned residual and never
// the count. Both lanes of the entry are pinned: an idle registry starts a
// fresh run, and a BUSY one — the call arriving from a native while the
// program's own run is live, where a fresh run would trip the concurrency
// guard — hands the unit to the live run's nested runner, the named entry
// riding there as a namedUnitRef.
func TestNamedCallbackRootRetTakesTheFramesCount(t *testing.T) {
	strict := func(reg *core.Registry, sig *core.Signature) ([]core.Value, error) {
		if reg.CanHostVM() {
			t.Error("the native must run while the program's own run holds the registry")
		}
		return core.InvokeCallbackStrict(reg, sig, nil, nil, "pair", core.SrcPos{})
	}
	plain := func(reg *core.Registry, sig *core.Signature) ([]core.Value, error) {
		return core.InvokeCallback(reg, sig, nil, nil)
	}

	res, err := RunProgram(namedPairProg(strict), runUnitReg(t))
	wantPairCountError(t, "busy registry, named call", res, err)
	res, err = RunProgram(namedPairProg(plain), runUnitReg(t))
	wantPairResidual(t, "busy registry, fn-value call", res, err)

	idle := runUnitReg(t)
	sig := pairSigOf(namedPairProg(plain))
	res, err = core.InvokeCallbackStrict(idle, sig, nil, nil, "pair", core.SrcPos{})
	wantPairCountError(t, "idle registry, named call", res, err)
	res, err = core.InvokeCallback(idle, sig, nil, nil)
	wantPairResidual(t, "idle registry, fn-value call", res, err)
}

// TestUnmatchedNamedBodyAnchorsAtTheValue pins the anchor of a NAMED
// callback value's no-match (NUR211): the interpreter raises
// uncalled_function at the reference's own token, which the closure carries
// as RetPos. A closure that carries a name but no reference position — a
// binding renames the value it binds (nameClosureValue) without writing a
// reference — anchors at the value's own position instead.
func TestUnmatchedNamedBodyAnchorsAtTheValue(t *testing.T) {
	r := seam7Reg(t)
	p := &compiler.Program{Fns: []compiler.CompiledFn{
		{Name: "h$body", NParams: 1, NArgs: 1, NLocals: 1, Params: []*core.Type{core.TInteger}, Code: []compiler.Instr{{Op: compiler.OpRet}}, Debug: []core.SrcPos{{}}},
	}}
	vc := &vmContext{p: p, r: r, ceiling: 1 << 20, stepLimit: 1 << 20}
	at := core.SrcPos{Row: 2, Col: 5}
	for _, c := range []struct {
		name         string
		retPos, want core.SrcPos
	}{
		{"the reference's own token", core.SrcPos{Row: 1, Col: 60}, core.SrcPos{Row: 1, Col: 60}},
		{"no reference position: the value's own", core.SrcPos{}, at},
	} {
		cl := core.ClosurePayload{Prog: p, Unit: 0, InShape: compiler.ClosureInValue, RetName: "h", RetPos: c.retPos, Ident: core.NewFnIdentity()}
		body := core.WithPosAt(core.NewValueRaw(core.TFunction, cl), at)
		res, err, ran := vc.unmatchedLambdaBody(r, body, cl, []core.Value{core.NewString("s")})
		var be *core.BoruError
		if !ran || !errors.As(err, &be) || be.Code != "uncalled_function" || be.Detail != "call to 'h' matched no signature" {
			t.Errorf("%s: got %v / %v (ran=%v), want the word's uncalled_function", c.name, res, err, ran)
			continue
		}
		if be.Row != c.want.Row || be.Col != c.want.Col {
			t.Errorf("%s: anchored at %d:%d, want %d:%d", c.name, be.Row, be.Col, c.want.Row, c.want.Col)
		}
	}
}

// capTwiceReg registers a native whose `/q` slot CAPTURES the word after it
// and answers the atom twice, and returns the fn value a landing holds for
// it.
func capTwiceReg(t *testing.T) (*core.Registry, core.Value) {
	t.Helper()
	r := walkReg(t)
	r.Register("zz-cap-twice", core.Signature{
		Args: []*core.Type{core.TAtom}, QuoteArgs: map[int]bool{0: true}, BarrierPos: 1,
		Returns: []*core.Type{core.TAtom, core.TAtom},
		Impl: core.Go(func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
			return []core.Value{args[0], args[0]}, nil
		}),
	})
	if err := r.Err(); err != nil {
		t.Fatalf("registration: %v", err)
	}
	inner := r.Lookup("zz-cap-twice")
	if inner == nil {
		t.Fatal("zz-cap-twice did not register")
	}
	return r, core.NewFunction(core.FnDefInfo{Name: "zz-cap-twice", Registry: r, Signatures: inner.Signatures})
}

// TestLandingSkipClaimsTheCaptureCount pins the landing's SKIP (NUR190,
// LandingWord.SkipTo): the capture runs over the value and the word alone and
// its results take the place of the word's call and the apply after it,
// which claimed a count. A capture answering the claimed count jumps past
// both; one answering another count cannot be represented and defers loudly.
func TestLandingSkipClaimsTheCaptureCount(t *testing.T) {
	r, v := capTwiceReg(t)
	vc := &vmContext{p: landingProg(), r: r, ceiling: 1 << 20, stepLimit: 1 << 20}
	w := compiler.LandingWord{Name: "z", Pos: core.SrcPos{Row: 1, Col: 9}, SkipTo: 4, SkipOut: 2}

	got, ent, err := vc.landingSkip(r, v, w, []core.Value{core.NewInteger(1), v}, 1, seam7Dbg, 0)
	if err != nil || ent == nil || !ent.jump || ent.jumpPC != 4 || len(got) != 3 {
		t.Fatalf("a met claim: got %v %+v %v, want [1 z z] and the jump to 4", got, ent, err)
	}
	if name, aerr := core.AsAtom(got[1]); aerr != nil || name != "z" {
		t.Errorf("the capture takes the word as an atom, got %v", got[1])
	}

	w.SkipOut = 1
	got, ent, err = vc.landingSkip(r, v, w, []core.Value{v}, 0, seam7Dbg, 0)
	if got != nil || ent != nil {
		t.Errorf("a drifted claim must not land anything, got %v %+v", got, ent)
	}
	wantInternal(t, err, "the `/q` capture of `z` left 2 value(s) where the apply after it claims 1")
}

// TestLandingDeoptRefusesABadEntry pins the landing DEOPT's entry guard
// (NUR190): an island with no body to resume, no resume point, or a frame
// base above the value is a lowering fault, reported as the internal error it
// is rather than run; a well-formed entry runs the island (the value, then
// walkReg's native producer) over the frame region and resumes at RetPC.
func TestLandingDeoptRefusesABadEntry(t *testing.T) {
	r := walkReg(t)
	vc := &vmContext{p: landingProg(), r: r, ceiling: 1 << 20, stepLimit: 1 << 20}
	v := core.NewInteger(1)
	island := []core.Value{core.NewWord("zz-walk-native")}
	for _, c := range []struct {
		name      string
		w         compiler.LandingWord
		frameBase int
	}{
		{"no island", compiler.LandingWord{Name: "zz-walk-native", Deopt: true, RetPC: 3}, 0},
		{"no resume pc", compiler.LandingWord{Name: "zz-walk-native", Deopt: true, Island: island, RetPC: -1}, 0},
		{"a frame base above the value", compiler.LandingWord{Name: "zz-walk-native", Deopt: true, Island: island, RetPC: 3}, 1},
	} {
		got, ent, err := vc.landingDeopt(r, v, c.w, c.frameBase, []core.Value{v}, 0, seam7Dbg, 0)
		if got != nil || ent != nil {
			t.Errorf("%s: got %v %+v, want no landing", c.name, got, ent)
		}
		wantInternal(t, err, "RESTEP_LANDING bad deopt entry")
	}

	got, ent, err := vc.landingDeopt(r, v, compiler.LandingWord{Name: "zz-walk-native", Deopt: true, Island: island, RetPC: 3}, 0, []core.Value{v}, 0, seam7Dbg, 0)
	if err != nil || ent == nil || !ent.jump || ent.jumpPC != 3 || len(got) != 2 {
		t.Fatalf("a well-formed entry: got %v %+v %v, want [1 0] and the jump to 3", got, ent, err)
	}
	if n, _ := got[1].AsConcreteInteger(); n != 0 {
		t.Errorf("the island runs the producer after the value: got %v", got)
	}
}

// TestAdoptDynBindTakesOnlyItsOwnInstall pins the write-back's adoption of a
// dyn-scope bind (NUR168's second finding): a write-back paired with the
// dyn-scope bind of the SAME def takes that install off the trail and pushes
// nothing — one entry under the name — while one whose trail top is another
// install (another name's, another registry's, or none) pushes as before and
// leaves the trail to its own unwind.
func TestAdoptDynBindTakesOnlyItsOwnInstall(t *testing.T) {
	p := &compiler.Program{Consts: []core.Value{core.NewString("x"), core.NewString("y")}}
	gb := &compiler.GlobalBindSpec{Name: "x", AfterDynScope: true, Pop: true}

	r := seam7Reg(t)
	vc := seam7VC(r)
	stack, err := vc.bindDynScopeMode(r, p, 0, []core.Value{core.NewInteger(5)}, seam7Dbg, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if stack, err = vc.bindGlobal(r, gb, stack, seam7Dbg, 0); err != nil || len(stack) != 0 {
		t.Fatalf("paired write-back: %v / %v", stack, err)
	}
	if d := r.Defs.Depth("x"); d != 1 || len(vc.dynBinds) != 0 {
		t.Errorf("paired: depth %d, trail %d — want the install adopted as the one entry", d, len(vc.dynBinds))
	}

	r2 := seam7Reg(t)
	vc2 := seam7VC(r2)
	if _, err := vc2.bindDynScopeMode(r2, p, 1, []core.Value{core.NewInteger(6)}, seam7Dbg, 0, true); err != nil {
		t.Fatal(err)
	}
	if _, err := vc2.bindGlobal(r2, gb, []core.Value{core.NewInteger(5)}, seam7Dbg, 0); err != nil {
		t.Fatal(err)
	}
	top, _ := r2.Defs.Top("x")
	if n, _ := top.AsConcreteInteger(); n != 5 || r2.Defs.Depth("x") != 1 || len(vc2.dynBinds) != 1 || vc2.dynBinds[0].name != "y" {
		t.Errorf("unpaired: x=%v depth %d, trail %v — want the write-back pushed and y's install left", top, r2.Defs.Depth("x"), vc2.dynBinds)
	}

	vc3 := seam7VC(r)
	if vc3.adoptDynBind(r, "x") {
		t.Error("an empty trail adopts nothing")
	}
	vc3.dynBinds = []dynBindEntry{{reg: r2, name: "x"}}
	if vc3.adoptDynBind(r, "x") || len(vc3.dynBinds) != 1 {
		t.Error("another registry's install of the name is not this write-back's")
	}
}

// TestPolyNoMatchRaiseAnchorsAtAWrittenValue pins the runtime anchor of the
// poly's faithful no-match raise (NUR171): the recorded anchor when there is
// one, otherwise the first WRITTEN value that carries a position — the
// interpreter's own fallback for a positionless site — and none at all when
// no written value carries one, leaving the op's debug entry to stamp it.
func TestPolyNoMatchRaiseAnchorsAtAWrittenValue(t *testing.T) {
	r := pnmRegistry(t, []core.Signature{pnmSig(-1, core.TInteger, core.TString)})
	vc := seam7VC(r)
	fn := r.Lookup("pnmw")
	if fn == nil {
		t.Fatal("pnmw not registered")
	}
	at := core.SrcPos{Row: 1, Col: 4}
	positioned := []core.Value{core.NewBoolean(true), core.WithPosAt(core.NewBoolean(false), at)}
	bare := []core.Value{core.NewBoolean(true), core.NewBoolean(false)}
	for _, c := range []struct {
		name      string
		pos, want core.SrcPos
		window    []core.Value
	}{
		{"no recorded anchor: the first written value with one", core.SrcPos{}, at, positioned},
		{"the recorded anchor wins", core.SrcPos{Row: 2, Col: 1}, core.SrcPos{Row: 2, Col: 1}, positioned},
		{"nothing carries a position", core.SrcPos{}, core.SrcPos{}, bare},
	} {
		spec := &core.PolyNoMatchSpec{Written: []int{0, 1}, StackTuple: []int{0, 1}, NSigs: 1, Pos: c.pos}
		err := vc.polyNoMatchRaise(r, &compiler.PolyRef{Word: "pnmw", Arity: 2, NoMatch: spec}, fn, c.window, seam7Dbg, 0)
		var be *core.BoruError
		if !errors.As(err, &be) || be.Code != "signature_error" {
			t.Errorf("%s: got %v, want the no-match signature_error", c.name, err)
			continue
		}
		if be.Row != c.want.Row || be.Col != c.want.Col {
			t.Errorf("%s: anchored at %d:%d, want %d:%d", c.name, be.Row, be.Col, c.want.Row, c.want.Col)
		}
	}
}

// The tests below reach VM arms that predate PR #505 (main-side lines the
// merged gate also needs) and that no program reaches with both lanes in
// agreement; each drives the arm through the VM's own entry.

// TestReStepLandingIslandHonoursTheStepBudget pins the landing's island arm
// for an appliable value that is neither a closure nor an FnDefInfo — a
// Function-tagged value with no payload (TestReStepLandingIslandArm's
// fixture): the island pushes it as the interpreter's stepLiteral does. The
// island runs under the host's step budget (Registry.StepLimit,
// lang.Options.Steps) like every interpreter run, so a budget it exhausts
// raises the interpreter's own evaluation_limit at the landing instead of
// landing anything.
func TestReStepLandingIslandHonoursTheStepBudget(t *testing.T) {
	fnTagged := core.Value{Parent: core.TFunction}

	r := seam7Reg(t)
	got, ent, err := seam7VC(r).reStepLanding(r, 0, 0, []core.Value{fnTagged}, seam7Dbg, 0, compiler.LandingWord{})
	if err != nil || ent != nil || len(got) != 1 || !got[0].Parent.Equal(core.TFunction) {
		t.Fatalf("the island pushes the value: got %v %+v %v", got, ent, err)
	}

	tight := seam7Reg(t)
	tight.StepLimit = 1
	got, ent, err = seam7VC(tight).reStepLanding(tight, 0, 0, []core.Value{fnTagged}, seam7Dbg, 0, compiler.LandingWord{})
	var be *core.BoruError
	if got != nil || ent != nil || !errors.As(err, &be) || be.Code != "evaluation_limit" {
		t.Errorf("an exhausted budget: got %v %+v %v, want the island's evaluation_limit", got, ent, err)
	}
}

// TestTrailTopNamelessNoMatchRaises pins noMatchIfSigged's nameless arm
// (NUR107): a fn value with its own signatures, none of which admits the
// window, raises the no-match when the head carries no seated name, where a
// value-delivered head parks the window as data instead. (Every program that
// reaches this arm today is a top-level `/v` read the lowering does not seat
// as a delivery, and there the interpreter parks the value or raises
// uncalled_function: a lane divergence reported with this change.)
func TestTrailTopNamelessNoMatchRaises(t *testing.T) {
	r := seam7Reg(t)
	vc := seam7VC(r)
	g := core.NewFunction(core.FnDefInfo{Name: "g", Registry: r, Signatures: []core.Signature{{
		Params: []core.FnParam{{Type: core.TString}}, BarrierPos: 1, Returns: []*core.Type{core.TString},
		Impl: core.Go(func(a []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
			return []core.Value{a[0]}, nil
		}),
	}}})

	got, ent, err := vc.callDynTrailTop(r, 1, []core.Value{core.NewInteger(5), g}, seam7Dbg, 0, compiler.DynApplyHead{})
	var be *core.BoruError
	if got != nil || ent != nil || !errors.As(err, &be) || be.Code != "signature_error" || !strings.Contains(be.Detail, "cannot call `g`") {
		t.Errorf("a nameless head: got %v %+v %v, want the no-match signature_error", got, ent, err)
	}

	got, ent, err = vc.callDynTrailTop(r, 1, []core.Value{core.NewInteger(5), g}, seam7Dbg, 0, compiler.DynApplyHead{ValueDelivery: true})
	if err != nil || ent != nil || len(got) != 2 || !got[1].Parent.Equal(core.TFunction) {
		t.Errorf("a value-delivered head parks the window: got %v %+v %v", got, ent, err)
	}
}

// TestPolyRematchEscapedFlowNeedsALoop pins the flow check after a poly
// re-match (OpCallNativePoly): a handler that ran a body raising a
// break/continue with no loop of its own leaves the registry's FlowCtrl set,
// and with no enclosing loop in the compiled run either, the VM raises the
// loop-less internal error the interpreter's canonical raise is deferred to,
// never continuing as though nothing escaped; a handler that leaves the flag
// clear returns its result.
func TestPolyRematchEscapedFlowNeedsALoop(t *testing.T) {
	// nout is the recorded result-count claim: one for the answering
	// handler, none for the escaping one (its body produced nothing).
	prog := func(nout int) *compiler.Program {
		return &compiler.Program{
			Consts:   []core.Value{core.NewInteger(4)},
			Code:     []compiler.Instr{{Op: compiler.OpPushConst, Arg: 0}, {Op: compiler.OpCallNativePoly, Arg: 0}},
			Debug:    make([]core.SrcPos, 2),
			PolyRefs: []compiler.PolyRef{{Word: "zz-flow-poly", Arity: 1, NOut: nout}},
		}
	}
	reg := func(escape bool) *core.Registry {
		r := seam7Reg(t)
		r.Register("zz-flow-poly", core.Signature{
			Args: []*core.Type{core.TInteger}, Returns: []*core.Type{core.TInteger}, BarrierPos: -1,
			Impl: core.Go(func(a []core.Value, _ map[string]core.Value, _ []core.Value, reg *core.Registry) ([]core.Value, error) {
				if escape {
					reg.FlowCtrl = core.FlowBreak
					return nil, nil
				}
				return []core.Value{a[0]}, nil
			}),
		})
		if err := r.Err(); err != nil {
			t.Fatalf("registration: %v", err)
		}
		return r
	}

	res, err := RunProgram(prog(1), reg(false))
	if err != nil || len(res) != 1 {
		t.Fatalf("no flow escaped: got %v / %v, want [4]", res, err)
	}
	_, err = RunProgram(prog(0), reg(true))
	wantInternal(t, err, "flow signal with no enclosing loop")
}

// TestDynApplyForeignStaleRefRestamps pins dynApplyForeign's freshness dance
// (the §7c JIT re-stamp, InvokeCompiled's twin on the dynamic apply): a
// detached unit whose dependency was REBOUND since its stamp is re-compiled
// against the live binding and hosted, answering the live value; a re-stamp
// that declines (stamping disarmed) leaves the apply to the island, which
// resolves the live binding the interpreter's way.
func TestDynApplyForeignStaleRefRestamps(t *testing.T) {
	r := stampReg(t)
	r.EnableRuntimeStamping()
	r.Defs.Push("dep", core.NewInteger(1))
	fd := BoruBodyFd(core.NewWord("dep"))
	fd.Name = "reader"
	ref, ok := compiler.StampDetachedFn(r, fd, core.SrcPos{Row: 1, Col: 1})
	if !ok {
		t.Fatalf("initial stamp declined: %+v", r.StampEvents())
	}
	reader := core.NewFunction(core.FnDefInfo{Name: "reader", Registry: r, Signatures: []core.Signature{{
		Impl: core.NewBoruImplCompiled([]core.Value{core.NewWord("dep")}, ref),
	}}})
	vc := seam7VC(r)
	apply := func(want int64, where string) {
		t.Helper()
		got, ent, err := vc.callDynamic(r, 0, false, []core.Value{reader}, seam7Dbg, 0)
		if err != nil || ent != nil || len(got) != 1 {
			t.Fatalf("%s: got %v %+v %v", where, got, ent, err)
		}
		if n, _ := core.AsInteger(got[0]); n != want {
			t.Errorf("%s: got %v, want %d", where, got[0], want)
		}
	}

	apply(1, "fresh: the stamped unit")
	r.Defs.Pop("dep")
	r.Defs.Push("dep", core.NewInteger(2))
	apply(2, "stale: the re-stamped unit reads the live binding")
	if ref.Restamp == nil || ref.Restamp.Cur == nil {
		t.Fatal("the stale apply must have re-stamped")
	}

	ref.Restamp.Tries, ref.Restamp.Cur = 0, nil
	r.DisableRuntimeStamping()
	r.Defs.Pop("dep")
	r.Defs.Push("dep", core.NewInteger(9))
	apply(9, "a declined re-stamp: the island's live resolution")
}
