package check

// zz_lane_removal_cover_test.go — the check pass's OWN-suite coverage of
// what the compiler lane's removal (design/COMPILER-LANE-REMOVAL.0.md) left
// on the plain pass: the scalar constant fold and the code-effect analysis
// ported here from the compiler module, and the arms whose tests used to
// drive them through the recorder — the `args` frame projection, the
// mixed-arity gradual model, the loop join's bind-name and narrowing-only
// skips and its speculative-undef re-round, the TAny wildcard in the
// recovery's best-fit scoring, the bare-Map record residual, the shaped
// read's chain level, the dynamic fn-value window and the shaped-method
// window's declines. Fixture words carry an `lr` prefix.

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// lrFoldSig is an unregistered CompileScalarFold signature over nargs
// Integer params whose Go handler is f — the shape tryFoldScalarConst is
// handed by CarrierResults, without a registry round trip.
func lrFoldSig(nargs int, f func(args []core.Value) ([]core.Value, error)) *core.Signature {
	args := make([]*core.Type, nargs)
	for i := range args {
		args[i] = core.TInteger
	}
	return &core.Signature{
		Args:          args,
		CompileEffect: core.CompileScalarFold,
		Impl: core.Go(func(a []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
			return f(a)
		}),
	}
}

// lrQuoted is a stored code body: a quoted list of the tokens.
func lrQuoted(elems ...core.Value) core.Value {
	body := core.NewList(elems)
	core.SetQuoted(&body, true)
	return body
}

// ─── scalar_fold.go ──────────────────────────────────────────────────

// A CompileScalarFold dispatch over compile-time-known scalars runs for
// real and its concrete result replaces the declared-type carrier
// (`(n eq 0)` with a const n reads as a known Boolean downstream); a
// carrier operand keeps the declared carrier.
func TestLaneRemovalScalarFoldReplacesDeclaredCarrier(t *testing.T) {
	r := covRegistry(t, func(r *core.Registry) {
		r.RegisterNativeFunc(core.NativeFunc{
			Name: "lreq",
			Signatures: []core.Signature{{
				Args: []*core.Type{core.TInteger, core.TInteger}, Returns: []*core.Type{core.TBoolean},
				BarrierPos: -1, CompileEffect: core.CompileScalarFold,
				Impl: core.Go(func(a []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
					x, _ := core.AsInteger(a[0])
					y, _ := core.AsInteger(a[1])
					return []core.Value{core.NewBoolean(x == y)}, nil
				}),
			}},
		})
	})
	defer r.Check.Begin()()
	sig := &r.Lookup("lreq").Signatures[0]
	pos := core.SrcPos{Row: 3, Col: 1}

	out := CarrierResults(r, "lreq", sig, []core.Value{core.NewInteger(4), core.NewInteger(4)}, pos, r, false)
	if len(out) != 1 || !core.IsConcrete(&out[0]) {
		t.Fatalf("folded dispatch = %#v, want one CONCRETE result in place of the Boolean carrier", out)
	}
	if b, err := core.AsBoolean(out[0]); err != nil || !b {
		t.Errorf("folded value = (%v, %v), want the comparison's true", b, err)
	}
	if !r.Check.Mode {
		t.Error("check mode is switched back on after the concrete run")
	}

	out = CarrierResults(r, "lreq", sig, []core.Value{core.NewCarrier(core.TInteger), core.NewInteger(4)}, pos, r, false)
	if len(out) != 1 || !out[0].Carrier() || !out[0].Parent().Equal(core.TBoolean) {
		t.Errorf("a carrier operand is not compile-time known: got %#v, want the declared Boolean carrier", out)
	}
}

// The fold's guards: the dispatch shape (no sig, no fold effect, a quoted
// body arg, no operands), the handler's arity (a short or wide window
// never reaches the handler — NUR265), a non-scalar operand, an erroring,
// a nondeterministic and a non-inert result all decline; the sig's arity
// of inert scalars folds after two agreeing runs, and a type literal at a
// declared type slot is known by construction.
func TestLaneRemovalScalarFoldGuards(t *testing.T) {
	r := newTestRegistry(t)
	calls := 0
	second := lrFoldSig(2, func(a []core.Value) ([]core.Value, error) {
		calls++
		return []core.Value{a[1]}, nil
	})
	one, two := core.NewInteger(1), core.NewInteger(2)

	plain := lrFoldSig(2, func(a []core.Value) ([]core.Value, error) { return []core.Value{a[1]}, nil })
	plain.CompileEffect = 0
	noEval := lrFoldSig(2, func(a []core.Value) ([]core.Value, error) { return []core.Value{a[1]}, nil })
	noEval.NoEvalArgs = map[int]bool{0: true}
	for name, c := range map[string]struct {
		sig  *core.Signature
		args []core.Value
	}{
		"nil signature":   {nil, []core.Value{one, two}},
		"no fold effect":  {plain, []core.Value{one, two}},
		"quoted body arg": {noEval, []core.Value{one, two}},
		"no operands":     {second, nil},
		"short window":    {second, []core.Value{one}},
		"wide window":     {second, []core.Value{one, two, one}},
		"carrier operand": {second, []core.Value{core.NewCarrier(core.TInteger), two}},
	} {
		if _, ok := tryFoldScalarConst(r, c.sig, c.args); ok || calls != 0 {
			t.Errorf("%s: must decline without running the handler (ok=%v calls=%d)", name, ok, calls)
		}
	}

	v, ok := tryFoldScalarConst(r, second, []core.Value{one, two})
	if n, err := core.AsInteger(v); !ok || err != nil || n != 2 || calls != 2 {
		t.Fatalf("fold over the sig's arity = (%v, %v) after %d runs, want 2 after two agreeing runs", v, ok, calls)
	}

	calls = 0
	typed := lrFoldSig(2, func(a []core.Value) ([]core.Value, error) {
		calls++
		return []core.Value{a[1]}, nil
	})
	typed.TypeArgs = map[int]bool{0: true}
	v, ok = tryFoldScalarConst(r, typed, []core.Value{core.NewTypeLiteral(core.TInteger), two})
	if n, err := core.AsInteger(v); !ok || err != nil || n != 2 || calls != 2 {
		t.Errorf("a type literal at a declared type slot folds: (%v, %v) after %d runs", v, ok, calls)
	}

	boom := lrFoldSig(1, func([]core.Value) ([]core.Value, error) {
		return nil, &core.BoruError{Code: "lr_boom", Detail: "the handler fails"}
	})
	if _, ok := tryFoldScalarConst(r, boom, []core.Value{one}); ok {
		t.Error("an erroring handler declines the fold (the diagnostic path keeps the dispatch)")
	}
	n := 0
	drift := lrFoldSig(1, func([]core.Value) ([]core.Value, error) {
		n++
		return []core.Value{core.NewInteger(int64(n))}, nil
	})
	if _, ok := tryFoldScalarConst(r, drift, []core.Value{one}); ok {
		t.Error("a nondeterministic handler declines the fold")
	}
	typeNode := lrFoldSig(1, func([]core.Value) ([]core.Value, error) {
		tl := core.NewTypeLiteral(core.TInteger)
		core.SetID(&tl, "lr-type-literal")
		return []core.Value{tl}, nil
	})
	if _, ok := tryFoldScalarConst(r, typeNode, []core.Value{one}); ok {
		t.Error("a type-literal result is concrete to the handler run but not an inert const")
	}
	mutable := lrFoldSig(1, func([]core.Value) ([]core.Value, error) {
		return []core.Value{core.NewFlexList([]core.Value{core.NewInteger(1)})}, nil
	})
	if _, ok := tryFoldScalarConst(r, mutable, []core.Value{one}); ok {
		t.Error("a mutable container result is not an inert const")
	}
}

// concreteHandlerEval runs the matched native over the sig's full arity
// with check mode off and the def stack snapshotted: a short window never
// reaches the handler, a multi-value, carrier or erroring result declines,
// and a def the handler pushes does not leak.
func TestLaneRemovalConcreteHandlerEval(t *testing.T) {
	r := newTestRegistry(t)
	calls := 0
	sig := lrFoldSig(2, func(a []core.Value) ([]core.Value, error) {
		calls++
		return []core.Value{a[1]}, nil
	})
	if _, ok := concreteHandlerEval(r, sig, []core.Value{core.NewInteger(1)}); ok || calls != 0 {
		t.Errorf("a window short of the arity never reaches the handler (ok=%v calls=%d)", ok, calls)
	}
	defer r.Check.Begin()()
	v, ok := concreteHandlerEval(r, sig, []core.Value{core.NewInteger(1), core.NewInteger(2)})
	if n, err := core.AsInteger(v); !ok || err != nil || n != 2 || calls != 1 {
		t.Fatalf("the full window runs the handler once: (%v, %v) calls=%d", v, ok, calls)
	}
	if !r.Check.Mode {
		t.Error("check mode is switched back on after the concrete run")
	}

	pair := lrFoldSig(1, func(a []core.Value) ([]core.Value, error) { return []core.Value{a[0], a[0]}, nil })
	if _, ok := concreteHandlerEval(r, pair, []core.Value{core.NewInteger(1)}); ok {
		t.Error("two results are not one concrete value")
	}
	carrier := lrFoldSig(1, func([]core.Value) ([]core.Value, error) {
		return []core.Value{core.NewCarrier(core.TInteger)}, nil
	})
	if _, ok := concreteHandlerEval(r, carrier, []core.Value{core.NewInteger(1)}); ok {
		t.Error("a carrier result is neither concrete nor a typed literal")
	}
	leak := lrFoldSig(1, func(a []core.Value) ([]core.Value, error) {
		r.Defs.Push("lrleak", a[0])
		return []core.Value{a[0]}, nil
	})
	if _, ok := concreteHandlerEval(r, leak, []core.Value{core.NewInteger(1)}); !ok {
		t.Error("the leaking handler's own result is concrete")
	}
	if _, bound := r.Defs.Top("lrleak"); bound {
		t.Error("the def stack is restored after the run")
	}
}

// ─── carrier.go: specialWordResults / applyGradualContagion ──────────

// `args` inside a fn body projects the frame's params (pushed by
// AnalyseFnBody) as a list value; an empty args stack or a carrier frame
// top falls through to the ordinary return resolution.
func TestLaneRemovalArgsFrameProjection(t *testing.T) {
	r := newTestRegistry(t)
	pos := core.SrcPos{Row: 1, Col: 1}
	if out, ok := specialWordResults(r, "args", nil, pos); ok || out != nil {
		t.Fatalf("an empty args stack falls through, got (%v, %v)", out, ok)
	}
	if err := r.Args.Push(core.NewList([]core.Value{core.NewInteger(4)})); err != nil {
		t.Fatalf("Args.Push: %v", err)
	}
	out, ok := specialWordResults(r, "args", nil, pos)
	if !ok || len(out) != 1 {
		t.Fatalf("a concrete frame top projects, got (%v, %v)", out, ok)
	}
	l, err := core.AsList(out[0])
	if err != nil || len(l.Slice()) != 1 {
		t.Fatalf("projection = %#v, want the pushed one-param list", out[0])
	}
	if n, err := core.AsInteger(l.Slice()[0]); err != nil || n != 4 {
		t.Errorf("projected param = (%d, %v), want 4", n, err)
	}
	if err := r.Args.Push(core.NewCarrier(core.TList)); err != nil {
		t.Fatalf("Args.Push: %v", err)
	}
	if out, ok := specialWordResults(r, "args", nil, pos); ok || out != nil {
		t.Fatalf("a carrier frame top has nothing to project, got (%v, %v)", out, ok)
	}
}

// `set`-shaped mixed arity: the matched overload returns NOTHING (an
// in-place mutator) but a dynamic receiver also reaches value-returning
// siblings, so the model is ONE dynamic value — the union of the reachable
// value returns, or that one type when a single sibling returns a value.
func TestLaneRemovalGradualContagionMixedArityMutator(t *testing.T) {
	r := newTestRegistry(t)
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "lrmutq",
		Signatures: []core.Signature{
			{Args: []*core.Type{core.TMap, core.TInteger}, Returns: []*core.Type{core.TMap}, BarrierPos: 2, Impl: covCDImpl()},
			{Args: []*core.Type{core.TList, core.TInteger}, Returns: []*core.Type{core.TList}, BarrierPos: 2, Impl: covCDImpl()},
			{Args: []*core.Type{core.TString, core.TInteger}, Returns: []*core.Type{}, BarrierPos: 2, Impl: covCDImpl()},
		},
	})
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "lrmut1q",
		Signatures: []core.Signature{
			{Args: []*core.Type{core.TMap, core.TInteger}, Returns: []*core.Type{core.TMap}, BarrierPos: 2, Impl: covCDImpl()},
			{Args: []*core.Type{core.TString, core.TInteger}, Returns: []*core.Type{}, BarrierPos: 2, Impl: covCDImpl()},
		},
	})
	pos := core.SrcPos{Row: 8, Col: 1}
	args := []core.Value{core.NewDynamicCarrier(core.TAny), core.NewCarrier(core.TInteger)}

	out := applyGradualContagion(r, "lrmutq", args, nil, pos, false)
	if len(out) != 1 || !core.IsDisjunct(&out[0]) || !core.IsDynamic(&out[0]) {
		t.Fatalf("mixed-arity model = %#v, want one dynamic Map|List disjunct", out)
	}
	if got := len(core.FlattenAlternatives(out[0])); got != 2 {
		t.Errorf("modelled alternatives = %d, want 2 (Map and List)", got)
	}
	out = applyGradualContagion(r, "lrmut1q", args, nil, pos, false)
	if len(out) != 1 || out[0].Parent() == nil || !out[0].Parent().Equal(core.TMap) || !core.IsDynamic(&out[0]) {
		t.Fatalf("single-sibling model = %#v, want one dynamic Map carrier", out)
	}
}

// ─── carrier.go: AnalyseLoopBody ─────────────────────────────────────

// The loop join applies the same narrowing-only skip as the branch join
// (core.InstallJoinedDefs): a body add whose ID equals the pre-loop
// binding's is the pass's own refinement — no join push after the loop.
func TestLaneRemovalLoopJoinSkipsNarrowingOnlyAdd(t *testing.T) {
	r := zcaRegistry(t)
	defer r.Check.Begin()()
	pre := core.NewDynamicCarrier(core.TAny)
	pre.SetDynFrom("lrnq")
	r.Defs.Push("lrnq", pre)
	// `lrrefq` stands in for narrowDynamicUses firing inside the body: it
	// pushes a SAME-ID refinement of the enclosing binding, the stack shape a
	// typed-slot consumption leaves.
	refined := core.NewDynamicCarrierValue(core.NewCarrier(core.TMap))
	core.SetID(&refined, core.IDOf(&pre))
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "lrrefq",
		Signatures: []core.Signature{{
			Returns: []*core.Type{core.TInteger},
			Impl: core.Go(func([]core.Value, map[string]core.Value, []core.Value, *core.Registry) ([]core.Value, error) {
				r.Defs.Push("lrnq", refined)
				return []core.Value{core.NewInteger(1)}, nil
			}, core.RunInCheck()),
		}},
	})
	AnalyseLoopBody(r, core.NewList([]core.Value{core.NewWord("lrrefq")}), nil, nil, false)
	if d := r.Defs.Depth("lrnq"); d != 1 {
		t.Fatalf("a narrowing-only body add is not re-pushed after the loop: depth = %d, want 1", d)
	}
}

// A body def of one of the loop's OWN bind names rebinds the iteration's
// binding and ends with it (NUR204): the pre-loop binding of the same name
// survives the loop untouched, where a non-bind name the body rebinds is
// joined with its pre-loop binding.
func TestLaneRemovalLoopBindNameDefNotJoined(t *testing.T) {
	r := zcaRegistry(t)
	zcaRegisterPushq(r)
	defer r.Check.Begin()()
	outer := core.NewString("outer")
	r.Defs.Push("lriq", outer)
	r.Defs.Push("lraccq", core.NewString("acc"))
	body := core.NewList([]core.Value{
		core.NewWord("pushq"), core.NewWord("lriq"), core.NewInteger(9),
		core.NewWord("pushq"), core.NewWord("lraccq"), core.NewInteger(7),
		core.NewWord("pushq"), core.NewWord("lrfreshq"), core.NewInteger(3),
	})
	AnalyseLoopBody(r, body, []string{"lriq"}, []core.Value{core.NewCarrier(core.TInteger)}, true)
	if got, ok := r.Defs.Top("lriq"); !ok || !core.IsConcrete(&got) || core.IDOf(&got) != core.IDOf(&outer) {
		t.Errorf("the pre-loop binding of the loop's own bind name survives untouched, got %#v", got)
	}
	if acc, _ := r.Defs.Top("lraccq"); !acc.Parent().Equal(core.TDisjunct) {
		t.Errorf("a non-bind rebind joins with its pre-loop binding, got %v", acc.Parent())
	}
	// A FRESH name the body binds has nothing to join: the loop leaves the
	// body's own binding (a loop that provably runs binds it once).
	if fresh, ok := r.Defs.Top("lrfreshq"); !ok || r.Defs.Depth("lrfreshq") != 1 || fresh.Parent() == nil || !fresh.Parent().Equal(core.TInteger) {
		t.Errorf("a fresh body binding is installed after the loop, got %#v at depth %d", fresh, r.Defs.Depth("lrfreshq"))
	}
}

// A speculative undef inside the body generalises an enclosing binding IN
// PLACE and moves the pass's generation (core.GeneraliseSpecUndef): no depth
// grows, so the join sees nothing, yet the round is not stable. The loop
// re-rounds once (the second undef re-mints nothing), where a body that
// leaves the binding alone settles in one round.
func TestLaneRemovalLoopSpecUndefReRounds(t *testing.T) {
	rounds := func(undef bool) int {
		r := zcaRegistry(t)
		calls := 0
		r.RegisterNativeFunc(core.NativeFunc{
			Name: "lrundefq",
			Signatures: []core.Signature{{
				Args: []*core.Type{core.TAtom}, QuoteArgs: map[int]bool{0: true},
				Returns: []*core.Type{}, BarrierPos: -1,
				Impl: core.Go(func(args []core.Value, _ map[string]core.Value, _ []core.Value, reg *core.Registry) ([]core.Value, error) {
					calls++
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
		defer r.Check.Begin()()
		r.Defs.Push("lrk", core.NewInteger(5))
		body := core.NewList([]core.Value{core.NewWord("lrundefq"), core.NewWord("lrk")})
		AnalyseLoopBody(r, body, []string{"lri"}, []core.Value{core.NewCarrier(core.TInteger)}, true)
		if undef {
			if k, ok := r.Defs.Top("lrk"); !ok || core.IsConcrete(&k) {
				t.Errorf("the speculative undef keeps the binding as a carrier, got %#v", k)
			}
		}
		return calls
	}
	if n := rounds(false); n != 1 {
		t.Errorf("a body leaving its bindings alone settles in one round, ran %d", n)
	}
	if n := rounds(true); n != 2 {
		t.Errorf("a speculative undef forces exactly one more round, ran %d", n)
	}
}

// ─── check_recovery.go: checkModeAssumeSig ───────────────────────────

// An Any carrier operand is a wildcard in the recovery's best-fit scoring.
// Over zzr-two's sole {Integer, Integer} overload the Any carrier is
// skipped and the String beside it breaks the fit, so no best fit remains:
// the plain pass reports no_signature and still splices the assumed
// results. Over zzr-poly2's two one-param overloads the lone Any carrier
// contradicts neither, and the recovery models one result.
func TestLaneRemovalAssumeSigAnyCarrierIsAWildcard(t *testing.T) {
	r := zzAssumeReg(t)
	defer r.Check.Begin()()
	e, _ := zzAssumeCall(t, r, "zzr-two",
		[]core.Value{core.NewCarrier(core.TAny), core.NewString("x"), core.NewWord("zzr-two")}, 2, nil)
	if !zzHasDiag(r, "no_signature") {
		t.Error("a plain pass with no best fit reports no_signature")
	}
	if e.Tape.Len() != 1 || !e.Tape.At(0).Carrier() {
		t.Errorf("the assumed results are still spliced, tape len = %d", e.Tape.Len())
	}

	r2 := zzAssumeReg(t)
	defer r2.Check.Begin()()
	e2, _ := zzAssumeCall(t, r2, "zzr-poly2",
		[]core.Value{core.NewCarrier(core.TAny), core.NewWord("zzr-poly2")}, 1, nil)
	if e2.Tape.Len() != 1 || !e2.Tape.At(0).Carrier() {
		t.Errorf("an Any carrier fits either overload and the recovery models one result, tape len = %d", e2.Tape.Len())
	}
}

// ─── check_fnmodel.go: nur068ReturnCarrier ───────────────────────────

// NUR068's residual-surfacing twin: a fn declared to return a BARE Map whose
// body residual is a record-SCHEMA carrier surfaces the schema (cloned, with
// a fresh identity) in place of the shapeless declared carrier.
func TestLaneRemovalBareMapReturnSurfacesRecordResidual(t *testing.T) {
	r := newTestRegistry(t)
	defer r.Check.Begin()()
	fields := core.NewOrderedMap()
	fields.Set("name", core.NewTypeLiteral(core.TString))
	res := core.NewDynamicCarrierValue(core.NewRecordType(fields))
	sig := core.FnSig{Returns: []*core.Type{core.TMap}, Impl: core.Boru([]core.Value{res})}
	out := BuildFnBodyReturnsFn(r, "lrsurf", sig, core.FnDefInfo{})(nil, r)
	if len(out) != 1 || !out[0].Carrier() {
		t.Fatalf("surfaced result = %#v, want one carrier", out)
	}
	rt, ok := out[0].Data().(core.RecordTypeInfo)
	if !ok || rt.Fields == nil {
		t.Fatalf("surfaced Data = %#v, want the residual's RecordTypeInfo", out[0].Data())
	}
	if core.IDOf(&out[0]) == "" || core.IDOf(&out[0]) == core.IDOf(&res) {
		t.Error("the surfaced residual carries a freshly minted identity")
	}
}

// ─── method_shape.go ─────────────────────────────────────────────────

// A claim whose RESULT is a claimed fn (a curried chain's level) makes the
// read's one result a Function carrier carrying that shape, so the next
// read models its own dispatch the same way (`def c10 (c 10)  (c10 3)`).
func TestLaneRemovalShapedReadChainLevel(t *testing.T) {
	r, c, done := frwFix(t, 1)
	defer done()
	r.Check.NoteFnShape(c, core.FnShape{Arity: 1, Result: &core.FnShape{Arity: 1}})
	e := zzmsEngine(r, []core.Value{c, core.NewInteger(10), core.NewEnd()})
	if !tryDynamicFnValueDispatch(e, 0) {
		t.Fatal("the plain check collapses the claimed window")
	}
	got := e.Tape.At(0)
	if !core.IsFnTypedCarrier(&got) || core.IsDynamic(&got) {
		t.Fatalf("the result of a chain level is a Function carrier, got %v", got)
	}
	if n, ok := r.Check.FnShapeArity(core.IDOf(&got)); !ok || n != 1 {
		t.Errorf("the result carries the next level's claim, got %d/%v", n, ok)
	}
}

// A dynamic Function-bound carrier with an inert forward window collapses,
// window and all, to a single dynamic(Any); with no args it stays data, and
// a word inside the window declines the model.
func TestLaneRemovalDynamicFnValueWindow(t *testing.T) {
	r := zzmsReg(t)
	e := zzmsEngine(r, []core.Value{
		core.NewDynamicCarrier(core.TFunction), core.NewInteger(3), core.NewString("x"), core.NewEnd(),
	})
	if !tryDynamicFnValueDispatch(e, 0) {
		t.Fatal("the dynamic-fn model consumes an inert forward window")
	}
	if e.Tape.Len() != 2 {
		t.Fatalf("the collapse consumes the whole window, tape len = %d", e.Tape.Len())
	}
	out := e.Tape.At(0)
	if !core.IsDynamic(&out) || !out.Carrier() || !out.Parent().Equal(core.TAny) {
		t.Errorf("the collapse result is dynamic(Any), got %v", out)
	}
	bare := zzmsEngine(r, []core.Value{core.NewDynamicCarrier(core.TFunction), core.NewEnd()})
	if tryDynamicFnValueDispatch(bare, 0) || bare.Tape.Len() != 2 {
		t.Error("an empty window declines: a bare dynamic fn value stays data")
	}
	worded := zzmsEngine(r, []core.Value{core.NewDynamicCarrier(core.TFunction), core.NewWord("lrw"), core.NewEnd()})
	if tryDynamicFnValueDispatch(worded, 0) || worded.Tape.Len() != 3 {
		t.Error("a word inside the window declines the model")
	}
}

// The shaped-method window declines a word inside the statement window
// (the interpreter could dispatch through it) and an inert window none of
// the member's overloads matches, leaving the tape untouched.
func TestLaneRemovalShapedMethodWindowDeclines(t *testing.T) {
	r := zzmsReg(t)
	zzmsRegisterInner(t, r, "lrmsadd", core.Signature{
		Args: []*core.Type{core.TInteger}, Returns: []*core.Type{core.TInteger},
		BarrierPos: core.BarrierAllForward, Impl: zzmsGoImpl(),
	})
	member := zzmsMember("lrmsadd", r, core.TInteger)
	carrier, done := zzmsAnnotate(t, r, member)
	defer done()

	worded := zzmsEngine(r, []core.Value{carrier, core.NewWord("lrmsw"), core.NewEnd()})
	if _, _, ok := shapedMethodApplyWindow(worded, 0, member); ok {
		t.Error("a word inside the statement window is not an inert window")
	}
	if TryShapedMethodDispatch(worded, 0) || worded.Tape.Len() != 3 {
		t.Error("the declined window leaves the tape untouched")
	}
	unmatched := zzmsEngine(r, []core.Value{carrier, core.NewString("x"), core.NewEnd()})
	if _, _, ok := shapedMethodApplyWindow(unmatched, 0, member); ok {
		t.Error("an inert window no overload matches declines")
	}
	if TryShapedMethodDispatch(unmatched, 0) || unmatched.Tape.Len() != 3 {
		t.Error("the unmatched window leaves the tape untouched")
	}
}

// ─── code_effect.go ──────────────────────────────────────────────────

// A stored code body types by its stack effect: the check-mode body
// analysis runs over the concrete tokens and the residual's types become
// the list carrier's CodeEffectInfo. INFO advisories the body raises do
// not invalidate the effect, and the dry pass truncates everything it
// raised. Every decline — outside check mode, nested, a non-body value, a
// sentinel, a failing or empty body, a residual the []*Type domain cannot
// represent — leaves no diagnostic behind.
func TestLaneRemovalCodeEffectCarrier(t *testing.T) {
	r := covRegistry(t, func(r *core.Registry) {
		r.RegisterNativeFunc(core.NativeFunc{Name: "lrnoop", Signatures: []core.Signature{{
			Returns: []*core.Type{}, BarrierPos: -1,
			Impl: core.Go(func([]core.Value, map[string]core.Value, []core.Value, *core.Registry) ([]core.Value, error) {
				return nil, nil
			}),
		}}})
		r.RegisterNativeFunc(core.NativeFunc{Name: "lrnote", Signatures: []core.Signature{{
			Args: []*core.Type{core.TInteger}, Returns: []*core.Type{core.TInteger}, BarrierPos: -1,
			Impl: core.Go(func(a []core.Value, _ map[string]core.Value, _ []core.Value, reg *core.Registry) ([]core.Value, error) {
				reg.Check.AddDiagnostic(core.CheckDiagnostic{Code: "lr_note", Detail: "an advisory", Severity: core.SeverityInfo})
				return []core.Value{a[0]}, nil
			}, core.RunInCheck()),
		}}})
	})
	stored := lrQuoted(core.NewInteger(1), core.NewWord("cdub"))
	if _, ok := AnalyseCodeEffectCarrier(r, stored); ok {
		t.Error("outside check mode the analysis declines")
	}
	if _, ok := AnalyseCodeEffectCarrier(nil, stored); ok {
		t.Error("a nil registry declines")
	}

	defer r.Check.Begin()()
	c, ok := AnalyseCodeEffectCarrier(r, stored)
	if !ok || !c.Carrier() || !c.Parent().Equal(core.TList) {
		t.Fatalf("a stored body types as a List carrier, got (%#v, %v)", c, ok)
	}
	info, isInfo := c.Data().(core.CodeEffectInfo)
	if !isInfo || !info.Analysed || len(info.Out) != 1 || !info.Out[0].Equal(core.TInteger) {
		t.Fatalf("effect = %#v, want Code[→ Integer]", c.Data())
	}
	if r.Check.CodeEffectDepth != 0 {
		t.Error("the nesting depth is restored")
	}

	base := len(r.Check.Diagnostics)
	if _, ok := AnalyseCodeEffectCarrier(r, lrQuoted(core.NewInteger(2), core.NewWord("lrnote"))); !ok {
		t.Error("an INFO advisory raised by the body does not invalidate the effect")
	}
	if len(r.Check.Diagnostics) != base {
		t.Errorf("the dry pass truncates what the body raised, %d left", len(r.Check.Diagnostics)-base)
	}

	r.Check.CodeEffectDepth = 1
	if _, ok := AnalyseCodeEffectCarrier(r, stored); ok {
		t.Error("nested inside another code-effect analysis declines")
	}
	r.Check.CodeEffectDepth = 0
	for name, body := range map[string]core.Value{
		"a carrier":               core.NewCarrier(core.TList),
		"a scalar":                core.NewInteger(1),
		"an empty list":           lrQuoted(),
		"a data-only list":        lrQuoted(core.NewInteger(1), core.NewInteger(2)),
		"a sentinel body":         lrQuoted(core.NewWord("break")),
		"an undefined word":       lrQuoted(core.NewWord("lrnowhere")),
		"a failing dispatch":      lrQuoted(core.NewString("s"), core.NewWord("cneg")),
		"an empty residual":       lrQuoted(core.NewWord("lrnoop")),
		"a type literal residual": lrQuoted(core.NewWord("lrnoop"), core.NewTypeLiteral(core.TInteger)),
	} {
		if _, ok := AnalyseCodeEffectCarrier(r, body); ok {
			t.Errorf("%s: the analysis declines", name)
		}
		if len(r.Check.Diagnostics) != base {
			t.Errorf("%s: a declined analysis leaves no diagnostic, %d left", name, len(r.Check.Diagnostics)-base)
		}
		if r.Check.CodeEffectDepth != 0 {
			t.Errorf("%s: the nesting depth is restored", name)
		}
	}
}
