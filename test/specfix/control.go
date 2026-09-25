package specfix

// Control-flow fixtures: a minimal `if` (3-arg) and `for` (count form)
// that orchestrate the SAME eng mechanisms the production handlers in
// basic/go/native_control.go do — the mark/move continuation tokens at
// run time, and the branch/loop capture + record protocol
// (ArmBranchCapture / TakeFragment / RecordBranch, ArmLoopCapture /
// AnalyseLoopBody / RecordLoop) under an active recorder — so the
// standalone lanes drive the kernel's branch/loop analysis, lowering,
// and VM jump/loop opcodes (design/ENG-COVERAGE-PARITY.0.md stage-4
// plan, step 1). Deliberately narrower than basic's words: no static-if
// reduction, no clause lists, no computed arms (those mark the program
// uncompilable rather than model something the runtime would not do),
// and only the integer-count `for`.

import (
	check "github.com/boru-lang/boru/check/go"
	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// fixSpliceArg mirrors basic's spliceArg: a plain concrete list is a
// code body — wrap it in parens so the engine evaluates it inline —
// and anything else is a value used as-is.
func fixSpliceArg(v core.Value) []core.Value {
	if fixIsCodeBody(v) {
		elems, _ := core.AsList(v)
		result := make([]core.Value, 0, elems.Len()+2)
		result = append(result, core.NewOpenParen())
		for i := 0; i < elems.Len(); i++ {
			result = append(result, elems.Get(i))
		}
		result = append(result, core.NewCloseParen())
		return result
	}
	return []core.Value{v}
}

func fixIsCodeBody(v core.Value) bool {
	return v.Parent.Equal(core.TList) && v.Data != nil && !core.IsTypedList(v) && !core.IsTableType(v)
}

// fixIfMarkMoveTokens builds the lazy-condition token stream: mark +
// condition body + move-if carrying the two branch continuations.
func fixIfMarkMoveTokens(cond core.Value, thenBranch, elseBranch []core.Value) ([]core.Value, bool) {
	if !fixIsCodeBody(cond) {
		return nil, false
	}
	lst, _ := core.AsList(cond)
	condSlice := lst.Slice()
	id := core.NextMarkID()
	tokens := make([]core.Value, 0, len(condSlice)+2)
	tokens = append(tokens, core.NewMark(id, condSlice...))
	tokens = append(tokens, condSlice...)
	tokens = append(tokens, core.NewMoveIf(id, "if", &core.IfCont{
		Then: thenBranch,
		Else: elseBranch,
	}))
	return tokens, true
}

func fixIf3Handler(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
	cond := args[0]
	thenBranch := fixSpliceArg(args[1])
	elseBranch := fixSpliceArg(args[2])
	if tokens, ok := fixIfMarkMoveTokens(cond, thenBranch, elseBranch); ok {
		return tokens, nil
	}
	if core.CoerceBoolean(cond) {
		return thenBranch, nil
	}
	return elseBranch, nil
}

// fixCondFragment captures the condition body as its own fragment when
// emitting, so the lowering runs it inline before JMP_IF_FALSE.
func fixCondFragment(r *core.Registry, cond core.Value) (core.EmitFragmentRef, []core.Value) {
	es, _ := r.Check.Recorder().(*compiler.EmitState)
	if !es.Armed() || !core.IsConcrete(cond) || !cond.Parent.ConformsTo(core.TList) {
		return nil, nil
	}
	es.ArmBranchCapture()
	stk, _ := core.RunCarrierCondBody(r, cond)
	return es.TakeFragment(), stk
}

// fixIf3ReturnsFn is the check/emit model: the literal-condition
// reduction, per-arm body capture under guard narrowing, the carrier
// join, and the BranchRecord — the core of basic's if3ReturnsFn with
// the plain-check-only optimisations left out.
func fixIf3ReturnsFn(args []core.Value, r *core.Registry) []core.Value {
	es := r.Check
	if lit, ok := core.LiteralCondValue(args[0]); ok {
		branch := "else"
		if !lit {
			branch = "then"
		}
		r.Check.AddDiagnostic(core.CheckDiagnostic{
			Code:   "unreachable_branch",
			Detail: "if condition is a constant " + core.BoolWord(lit) + "; " + branch + "-branch is unreachable",
			Word:   "if",
			Row:    r.Check.CurCallPos.Row,
			Col:    r.Check.CurCallPos.Col,
		})
		var stk []core.Value
		var defs map[string]core.Value
		if lit {
			restore := core.ApplyGuardNarrowing(r, args[0])
			es.Recorder().ArmBranchCapture()
			stk, defs = core.RunCarrierBodyWithDefs(r, args[1])
			restore()
			core.InstallJoinedDefs(r, defs, nil)
		} else {
			restore := core.ApplyComplementNarrowing(r, args[0])
			es.Recorder().ArmBranchCapture()
			stk, defs = core.RunCarrierBodyWithDefs(r, args[2])
			restore()
			core.InstallJoinedDefs(r, nil, defs)
		}
		frag := recorderState(es).TakeFragment()
		if len(stk) == 0 {
			es.Recorder().MarkUncompilable("fixture if: constant-cond branch produces no value")
			return nil
		}
		out := stk[len(stk)-1]
		taken := lit
		recorderState(es).RecordBranch(core.BranchRecord{
			ConstCond: &taken, HasElse: true,
			Then: frag, ThenStk: stk, Out: out, Pos: args[0].Pos(),
		})
		return []core.Value{out}
	}

	condFrag, condStk := fixCondFragment(r, args[0])

	arm := func(v core.Value, narrow func() func()) (frag core.EmitFragmentRef, stk []core.Value, defs map[string]core.Value, value *core.Value) {
		if fixIsCodeBody(v) {
			restore := narrow()
			es.Recorder().ArmBranchCapture()
			stk, defs = core.RunCarrierBodyWithDefs(r, v)
			frag = recorderState(es).TakeFragment()
			restore()
			return frag, stk, defs, nil
		}
		if es.Recorder().Active() && !core.IsConcrete(v) && v.Parent != nil && v.Parent.ConformsTo(core.TList) {
			// A COMPUTED list arm is the interpreter's spliced code body;
			// the fixture does not model it — decline the compile.
			es.Recorder().MarkUncompilable("fixture if: computed list arm")
		}
		vv := v
		return nil, []core.Value{v}, nil, &vv
	}
	thenFrag, thenStk, thenDefs, thenValue := arm(args[1], func() func() { return core.ApplyGuardNarrowing(r, args[0]) })
	elseFrag, elseStk, elseDefs, elseValue := arm(args[2], func() func() { return core.ApplyComplementNarrowing(r, args[0]) })

	core.InstallJoinedDefs(r, thenDefs, elseDefs)
	joined := core.JoinCarrierStacks(thenStk, elseStk)
	if len(joined) == 0 {
		out := core.NewCarrier(core.TNone)
		recorderState(es).RecordBranch(core.BranchRecord{
			Cond: args[0], CondFrag: condFrag, CondStk: condStk, HasElse: true,
			Then: thenFrag, Els: elseFrag, ThenStk: thenStk, ElsStk: elseStk,
			ThenValue: thenValue, ElsValue: elseValue, Out: out, Pos: args[0].Pos(),
		})
		if !es.Recorder().Active() {
			return nil
		}
		return []core.Value{out}
	}
	out := joined[len(joined)-1]
	recorderState(es).RecordBranch(core.BranchRecord{
		Cond: args[0], CondFrag: condFrag, CondStk: condStk, HasElse: true,
		Then: thenFrag, Els: elseFrag, ThenStk: thenStk, ElsStk: elseStk,
		ThenValue: thenValue, ElsValue: elseValue, Out: out, Pos: args[0].Pos(),
	})
	return []core.Value{out}
}

// fixRunForLoop mirrors basic's RunForLoop for the count form: install
// the iterator, build mark + body + move-cont tokens; the engine's
// stepMoveCont drives the remaining iterations through the ForCont.
func fixRunForLoop(r *core.Registry, start, end, step int64, iterName string, body core.Value) ([]core.Value, error) {
	if step > 0 && start >= end {
		return nil, nil
	}
	if !core.IsConcrete(body) { //covergate:allow both callers pass a dispatch-matched List argument and handlers never run on carriers; a bare List literal cannot bind the sig slot
		return nil, &core.BoruError{Code: "for_error", Detail: "for: body must be a concrete list, got type literal"}
	}
	lst, _ := core.AsList(body)
	bodySlice := lst.Slice()

	core.InstallDef(r, iterName, core.NewInteger(start))

	bodyCopy := make([]core.Value, len(bodySlice))
	copy(bodyCopy, bodySlice)
	cont := &core.ForCont{
		Registry: r,
		IterName: iterName,
		Current:  start,
		End:      end,
		Step:     step,
		Body:     bodyCopy,
	}

	id := core.NextMarkID()
	tokens := make([]core.Value, 0, len(bodySlice)+2)
	tokens = append(tokens, core.NewMark(id, bodySlice...))
	bodyTokens := make([]core.Value, len(bodySlice))
	copy(bodyTokens, bodySlice)
	tokens = append(tokens, bodyTokens...)
	tokens = append(tokens, core.NewMoveCont(id, "for loop", cont))
	return tokens, nil
}

func fixForCountHandler(args []core.Value, _ map[string]core.Value, _ []core.Value, r *core.Registry) ([]core.Value, error) {
	n, err := args[0].AsConcreteInteger()
	if err != nil { //covergate:allow the count is a dispatch-matched Integer argument and handlers never run on carriers; a bare Integer literal cannot bind the sig slot
		return nil, &core.BoruError{Code: "for_error", Detail: "for: count must be a concrete Integer"}
	}
	return fixRunForLoop(r, 0, n, 1, "i", args[1])
}

// fixForReturnsFn is the count-form loop model: fixed-point body
// analysis with the iterator as a typed carrier, the statically-zero
// prune, and the RecordLoop event with its static region size.
func fixForReturnsFn(args []core.Value, r *core.Registry) []core.Value {
	body := args[1]
	iter := core.NewCarrier(core.TInteger)
	es, _ := r.Check.Recorder().(*compiler.EmitState)

	cv := args[0]
	staticCount := int64(-1)
	if core.IsConcrete(cv) && cv.Parent.ConformsTo(core.TInteger) {
		if n, err := core.AsInteger(cv); err == nil {
			staticCount = n
			if n <= 0 {
				return []core.Value{}
			}
		}
	}

	startV, endV, stepV := core.NewInteger(0), cv, core.NewInteger(1)
	lowerable := cv.Parent.ConformsTo(core.TInteger)
	if lowerable {
		es.ArmLoopCapture()
	}
	provenTrips := staticCount >= 1
	stk := check.AnalyseLoopBody(r, body, []string{"i"}, []core.Value{iter}, provenTrips)
	out := core.NewCarrier(core.TList)
	if len(stk) > 0 {
		top := stk[len(stk)-1]
		if core.IsDisjunct(top) {
			out = core.NewCarrierTypedListValue(top)
		} else {
			out = core.NewCarrierTypedList(top.Parent)
		}
	}
	if lowerable {
		regionN := 0
		if staticCount >= 1 && len(stk) > 0 {
			regionN = int(staticCount) * len(stk)
		}
		frag := es.TakeFragment()
		es.RecordLoop(startV, endV, stepV, frag, stk, iter.ID, "i", out, regionN, args[0].Pos())
	}
	if len(stk) == 0 && (!es.Active() || !lowerable) {
		return []core.Value{}
	}
	// Plain check: a statically-counted loop leaves the SPREAD of its
	// per-iteration residual, exactly like the runtime.
	const spreadCap = 256
	if !es.Active() && staticCount >= 0 && len(stk) > 0 && staticCount*int64(len(stk)) <= spreadCap {
		spread := make([]core.Value, 0, int(staticCount)*len(stk))
		for i := int64(0); i < staticCount; i++ {
			spread = append(spread, stk...)
		}
		return spread
	}
	return []core.Value{out}
}

// registerEngSpecControl installs the fixture `if` and `for`.
func registerEngSpecControl(r *core.Registry) {
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "if",
		Signatures: []core.Signature{{
			Args:       []*core.Type{core.TAny, core.TAny, core.TAny},
			NoEvalArgs: map[int]bool{0: true, 1: true, 2: true},
			Impl:       core.Go(fixIf3Handler),
			ReturnsFn:  fixIf3ReturnsFn, BarrierPos: -1,
		}},
	})
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "for",
		Signatures: []core.Signature{
			{
				Args:       []*core.Type{core.TInteger, core.TList},
				NoEvalArgs: map[int]bool{1: true},
				Impl:       core.Go(fixForCountHandler),
				ReturnsFn:  fixForReturnsFn, BarrierPos: -1,
			},
			{
				Args:       []*core.Type{core.TList, core.TList},
				NoEvalArgs: map[int]bool{1: true},
				Impl:       core.Go(fixForRangeHandler),
				ReturnsFn:  fixForRangeReturnsFn, BarrierPos: -1,
			},
		},
	})
}

// fixDoHandler mirrors basic's DoListHandler: run the body with no
// inputs through the InvokeBody seam (so the VM can run it as a
// compiled closure) and surface a body error as an Error VALUE.
func fixDoHandler(args []core.Value, _ map[string]core.Value, _ []core.Value, r *core.Registry) ([]core.Value, error) {
	if !core.IsConcrete(args[0]) { //covergate:allow the body is a dispatch-matched List argument and handlers never run on carriers; a bare List literal cannot bind the sig slot
		return nil, &core.BoruError{Code: "do_error", Detail: "doq: argument must be a concrete list, got type literal"}
	}
	result, err := core.InvokeBody(r, args[0], nil)
	if err != nil {
		return []core.Value{core.NewError(err)}, nil
	}
	return result, nil
}

// fixDoReturnsFn is the closure model: a computed body takes the
// bounded dynamic hatch, and a concrete body's residual is analysed
// under the caught-body bracket with do's keep-defs leak fidelity.
// Check dispatch resolves def'd words before the ReturnsFn runs (only
// builtin TYPE names pass through raw — resolveTypeNameArgs), so the
// body arrives as a value, never a Word. Raising bodies are outside
// the fixture's model — battery rows keep bodies infallible.
func fixDoReturnsFn(args []core.Value, r *core.Registry) []core.Value {
	body := args[0]
	if !(core.IsConcrete(body) && body.Parent.ConformsTo(core.TList)) {
		return []core.Value{core.NewDynamicCarrier(core.TAny)}
	}
	r.Check.CaughtBodyDepth++
	stk := core.RunCarrierBodyKeepDefs(r, body)
	r.Check.CaughtBodyDepth--
	if len(stk) == 0 {
		if bl, err := core.AsList(body); err == nil && !bl.IsNil() && bl.Len() > 0 {
			return []core.Value{core.NewCarrier(core.TError)}
		}
		return nil
	}
	return stk
}

// registerEngSpecCallable installs `doq` — the battery's Callable
// body-runner, carrying basic's do CallableSpec so the kernel's
// closure dispatch/record/lower pipeline is driven standalone — and
// `dofbq`, the fallback-only variant (CompileFallbackBody without
// CompileDynBody, the each/fold shape): a body the closure path
// declines lowers to an interpreter ISLAND instead of a DynEnv call,
// driving the fallback-span record/lower/VM path.
func registerEngSpecCallable(r *core.Registry) {
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "dofbq",
		Callable: &core.CallableSpec{BodyPos: 0, BodyOut: core.BodyOutResidual, Inputs: func(_ []core.Value) []core.Value {
			return []core.Value{}
		}},
		Signatures: []core.Signature{{
			Args:       []*core.Type{core.TList},
			NoEvalArgs: map[int]bool{0: true},
			Impl:       core.Go(fixDoHandler),
			ReturnsFn:  fixDoReturnsFn, BarrierPos: -1,
			CompileEffect: core.CompileFallbackBody,
		}},
	})
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "doq",
		Callable: &core.CallableSpec{BodyPos: 0, BodyOut: core.BodyOutResidual, Inputs: func(_ []core.Value) []core.Value {
			return []core.Value{}
		}},
		Signatures: []core.Signature{{
			Args:       []*core.Type{core.TList},
			NoEvalArgs: map[int]bool{0: true},
			Impl:       core.Go(fixDoHandler),
			ReturnsFn:  fixDoReturnsFn, BarrierPos: -1,
			CompileEffect: core.CompileFallbackBody | core.CompileDynBody,
		}},
	})
}

// fixParseRange decodes a for-range list mirroring basic's ParseRange:
// [end] / [start end] / [start end step], every bound a concrete
// Integer (the VM's OpForSetup taxonomy).
func fixParseRange(elems []core.Value) (start, end, step int64, err error) {
	intAt := func(v core.Value) (int64, error) {
		n, e := v.AsConcreteInteger()
		if e != nil {
			return 0, &core.BoruError{Code: "for_error", Detail: "for: range: expected a concrete integer"}
		}
		return n, nil
	}
	switch len(elems) {
	case 1:
		if end, err = intAt(elems[0]); err != nil {
			return 0, 0, 0, err
		}
		return 0, end, 1, nil
	case 2:
		if start, err = intAt(elems[0]); err != nil {
			return 0, 0, 0, err
		}
		if end, err = intAt(elems[1]); err != nil {
			return 0, 0, 0, err
		}
		return start, end, 1, nil
	case 3:
		if start, err = intAt(elems[0]); err != nil {
			return 0, 0, 0, err
		}
		if end, err = intAt(elems[1]); err != nil {
			return 0, 0, 0, err
		}
		if step, err = intAt(elems[2]); err != nil {
			return 0, 0, 0, err
		}
		if step == 0 {
			return 0, 0, 0, &core.BoruError{Code: "for_error", Detail: "for: step cannot be zero"}
		}
		return start, end, step, nil
	}
	return 0, 0, 0, &core.BoruError{Code: "for_error", Detail: "for: range must have 1-3 elements"}
}

// fixLoopIterations mirrors basic's LoopIterations for the static
// zero-prune and region sizing.
func fixLoopIterations(start, end, step int64) int64 {
	if step > 0 {
		if start >= end {
			return 0
		}
		return (end - start + step - 1) / step
	}
	if step < 0 {
		if start <= end {
			return 0
		}
		return (start - end - step - 1) / -step
	}
	return -1 //covergate:allow fixParseRange rejects step==0, so both signed arms above are exhaustive
}

func fixForRangeHandler(args []core.Value, _ map[string]core.Value, _ []core.Value, r *core.Registry) ([]core.Value, error) {
	if !core.IsConcrete(args[0]) { //covergate:allow the range is a dispatch-matched List argument and handlers never run on carriers; a bare List literal cannot bind the sig slot
		return nil, &core.BoruError{Code: "for_error", Detail: "for: range must be a concrete list, got type literal"}
	}
	lst, _ := core.AsList(args[0])
	start, end, step, err := fixParseRange(lst.Slice())
	if err != nil {
		return nil, err
	}
	if step < 0 && start <= end {
		return nil, nil
	}
	return fixRunForLoop(r, start, end, step, "i", args[1])
}

// fixForRangeReturnsFn is the range-form model: a fully-static range
// decomposes for RecordLoop; a range whose bounds are computed keeps
// const start/step and resolves the end operand (basic's
// computedRangeBounds shape) so the lowering still emits FOR_SETUP.
func fixForRangeReturnsFn(args []core.Value, r *core.Registry) []core.Value {
	body := args[1]
	iter := core.NewCarrier(core.TInteger)
	es, _ := r.Check.Recorder().(*compiler.EmitState)

	var startV, endV, stepV core.Value
	lowerable, staticBounds := false, false
	staticCount := int64(-1)
	if cv := args[0]; core.IsConcrete(cv) && cv.Parent.ConformsTo(core.TList) {
		if lst, err := core.AsList(cv); err == nil && !lst.IsNil() {
			elems := lst.Slice()
			if st, en, sp, perr := fixParseRange(elems); perr == nil {
				startV, endV, stepV = core.NewInteger(st), core.NewInteger(en), core.NewInteger(sp)
				lowerable, staticBounds = true, true
				staticCount = fixLoopIterations(st, en, sp)
				if staticCount == 0 {
					return []core.Value{}
				}
			} else {
				switch len(elems) {
				case 1:
					startV, endV, stepV = core.NewInteger(0), elems[0], core.NewInteger(1)
					lowerable = true
				case 2:
					startV, endV, stepV = elems[0], elems[1], core.NewInteger(1)
					lowerable = true
				case 3:
					startV, endV, stepV = elems[0], elems[1], elems[2]
					lowerable = true
				}
			}
		}
	}
	if lowerable {
		es.ArmLoopCapture()
	}
	provenTrips := staticBounds && staticCount >= 1
	stk := check.AnalyseLoopBody(r, body, []string{"i"}, []core.Value{iter}, provenTrips)
	out := core.NewCarrier(core.TList)
	if len(stk) > 0 {
		top := stk[len(stk)-1]
		if core.IsDisjunct(top) {
			out = core.NewCarrierTypedListValue(top)
		} else {
			out = core.NewCarrierTypedList(top.Parent)
		}
	}
	if lowerable {
		regionN := 0
		if staticBounds && staticCount >= 1 && len(stk) > 0 {
			regionN = int(staticCount) * len(stk)
		}
		frag := es.TakeFragment()
		es.RecordLoop(startV, endV, stepV, frag, stk, iter.ID, "i", out, regionN, args[0].Pos())
	}
	if len(stk) == 0 && (!es.Active() || !lowerable) {
		return []core.Value{}
	}
	const spreadCap = 256
	if !es.Active() && staticBounds && staticCount >= 0 && len(stk) > 0 && staticCount*int64(len(stk)) <= spreadCap {
		spread := make([]core.Value, 0, int(staticCount)*len(stk))
		for i := int64(0); i < staticCount; i++ {
			spread = append(spread, stk...)
		}
		return spread
	}
	return []core.Value{out}
}

// fixBakeFnHandler applies its fn argument to 7 — the runtime side of
// the battery's stored-fn word.
func fixBakeFnHandler(args []core.Value, _ map[string]core.Value, _ []core.Value, r *core.Registry) ([]core.Value, error) {
	info, ok := args[0].Data.(core.FnDefInfo)
	if !ok || len(info.Signatures) == 0 {
		return nil, &core.BoruError{Code: "type_error", Detail: "bakefnq: argument must be a fn"}
	}
	return r.CallBoru(&info.Signatures[0], []core.Value{core.NewInteger(7)}, nil)
}

// registerEngSpecBaking installs `bakefnq` and `bakebodyq` — battery
// fixtures declaring the stored-fn / stored-body compile capabilities
// (the spawn / service / codec-handler shapes), so recordCallOperands'
// baking arms and compileStoredBody run standalone.
func registerEngSpecBaking(r *core.Registry) {
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "bakefnq",
		Signatures: []core.Signature{{
			Args:          []*core.Type{core.TFunction},
			Impl:          core.Go(fixBakeFnHandler),
			Returns:       []*core.Type{core.TAny},
			BarrierPos:    -1,
			CompileEffect: core.CompileStoresFn,
		}},
	})
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "bakebodyq",
		Signatures: []core.Signature{{
			Args:       []*core.Type{core.TList},
			NoEvalArgs: map[int]bool{0: true},
			Impl:       core.Go(fixDoHandler),
			ReturnsFn:  fixDoReturnsFn, BarrierPos: -1,
			CompileEffect: core.CompileStoresBody,
		}},
	})
}

// fixEachHandler is the battery's higher-order word: run the body (a
// code list or a Function value) once per data element via the
// InvokeBody seam — the minimal mirror of lang's each, registered so
// the LAMBDA-HOOK compile machinery (tryRecordLambdaClosure,
// lambdaHookCompatible, lambdaCallbackInputs) has a standalone driver.
func fixEachHandler(args []core.Value, _ map[string]core.Value, _ []core.Value, r *core.Registry) ([]core.Value, error) {
	lst, err := core.AsList(args[1])
	if err != nil || lst.IsNil() { //covergate:allow the data arg is a dispatch-matched concrete List; AsList cannot fail and a parsed list is never nil-backed
		return []core.Value{core.NewList(nil)}, nil
	}
	out := make([]core.Value, 0, lst.Len())
	for i := 0; i < lst.Len(); i++ {
		res, err := core.InvokeBody(r, args[0], []core.Value{lst.Get(i)})
		if err != nil {
			return nil, err
		}
		if len(res) == 0 {
			return nil, &core.BoruError{Code: "each_error", Detail: "eachq: body produced no result"}
		}
		out = append(out, res[len(res)-1])
	}
	return []core.Value{core.NewList(out)}, nil
}

// fixEachReturnsFn models the residual: a typed list of the body's
// modeled result over one element carrier (the fixture twin of lang's
// eachReturnsFn — recorder suspended for the nested model).
func fixEachReturnsFn(args []core.Value, r *core.Registry) []core.Value {
	defer r.Check.Recorder().Suspend()()
	body := args[0]
	if !(core.IsConcrete(body) && body.Parent.ConformsTo(core.TList)) {
		return []core.Value{core.NewCarrier(core.TList)}
	}
	bl, err := core.AsList(body)
	if err != nil || bl.IsNil() { //covergate:allow guarded by IsConcrete+ConformsTo(TList) above; AsList cannot fail and a concrete list is never nil-backed
		return []core.Value{core.NewCarrier(core.TList)}
	}
	toks := append([]core.Value{check.ElementCarrierFromValue(args[1])}, bl.Slice()...)
	stk := core.RunCarrierBody(r, core.NewList(toks))
	if len(stk) == 0 {
		return []core.Value{core.NewCarrier(core.TList)}
	}
	return []core.Value{core.NewCarrierTypedList(stk[len(stk)-1].Parent)}
}

// registerEngSpecHigherOrder installs eachq — the battery's
// lambda-hook Callable (quotation and Function forms).
func registerEngSpecHigherOrder(r *core.Registry) {
	r.RegisterNativeFunc(core.NativeFunc{
		Name:          "eachq",
		CompileEffect: core.CompileFallbackBody,
		Callable: &core.CallableSpec{BodyPos: 0, BodyOut: 1, EmptyBodyErrors: true, BodyResultTop: true, CrossCollectionTokenShape: true, Inputs: func(a []core.Value) []core.Value {
			return []core.Value{check.NewElementCarrier(check.DataListElemTypeFromValue(a[1]))}
		}},
		Signatures: []core.Signature{
			{
				Args:       []*core.Type{core.TList, core.TList},
				NoEvalArgs: map[int]bool{0: true},
				Impl:       core.Go(fixEachHandler),
				ReturnsFn:  fixEachReturnsFn, BarrierPos: -1,
			},
			{
				Args:      []*core.Type{core.TFunction, core.TList},
				Impl:      core.Go(fixEachHandler),
				ReturnsFn: fixEachReturnsFn, BarrierPos: -1,
			},
		},
	})
}

// recorderState unwraps the concrete recording backend behind the check
// state's recorder. A plain-check pass runs the inactive no-op recorder;
// the nil *EmitState this returns then no-ops every method (the kernel's
// nil-receiver guards), exactly as the interface no-ops did before the
// S2 surface reduction removed the fragment/branch methods from the
// check-facing interface.
func recorderState(c *core.CheckState) *compiler.EmitState {
	es, _ := c.Recorder().(*compiler.EmitState)
	return es
}
