package specfix

// Control-flow fixtures: a minimal `if` (3-arg) and `for` (count form)
// that orchestrate the SAME core mechanisms the production handlers in
// basic/go/native_control.go do — the mark/move continuation tokens at
// run time, and the branch/loop body analysis (guard narrowing, the
// carrier join, AnalyseLoopBody) on the check pass — so the standalone
// lanes drive the kernel's branch/loop machinery. Deliberately narrower
// than basic's words: no static-if reduction, no clause lists, no
// computed arms, and only the integer-count `for`.

import (
	check "github.com/boru-lang/boru/check/go"
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
	return v.Parent().Equal(core.TList) && v.Data() != nil && !core.IsTypedList(&v) && !core.IsTableType(v)
}

// fixIfMarkMoveTokens builds the lazy-condition token stream: mark +
// condition body + move-if carrying the two branch continuations.
func fixIfMarkMoveTokens(cond core.Value, thenBranch, elseBranch []core.Value) ([]core.Value, bool) {
	if !fixIsCodeBody(cond) {
		return nil, false
	}
	lst, _ := core.AsList(cond)
	condSlice := lst.Elems()
	id := core.NextMarkID()
	tokens := make([]core.Value, 0, len(condSlice)+2)
	tokens = append(tokens, core.NewMark(id))
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

// fixIf3ReturnsFn is the check model: the literal-condition reduction,
// per-arm body analysis under guard narrowing, and the carrier join — the
// core of basic's if3ReturnsFn with the plain-check-only optimisations
// left out.
func fixIf3ReturnsFn(args []core.Value, r *core.Registry) []core.Value {
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
			stk, defs = core.RunCarrierBodyWithDefs(r, args[1])
			restore()
			core.InstallJoinedDefs(r, defs, nil)
		} else {
			restore := core.ApplyComplementNarrowing(r, args[0])
			stk, defs = core.RunCarrierBodyWithDefs(r, args[2])
			restore()
			core.InstallJoinedDefs(r, nil, defs)
		}
		if len(stk) == 0 {
			return nil
		}
		return []core.Value{stk[len(stk)-1]}
	}

	arm := func(v core.Value, narrow func() func()) (stk []core.Value, defs map[string]core.Value) {
		if fixIsCodeBody(v) {
			restore := narrow()
			stk, defs = core.RunCarrierBodyWithDefs(r, v)
			restore()
			return stk, defs
		}
		return []core.Value{v}, nil
	}
	thenStk, thenDefs := arm(args[1], func() func() { return core.ApplyGuardNarrowing(r, args[0]) })
	elseStk, elseDefs := arm(args[2], func() func() { return core.ApplyComplementNarrowing(r, args[0]) })

	core.InstallJoinedDefs(r, thenDefs, elseDefs)
	joined := core.JoinCarrierStacks(thenStk, elseStk)
	if len(joined) == 0 {
		return nil
	}
	return []core.Value{joined[len(joined)-1]}
}

// fixRunForLoop mirrors basic's RunForLoop for the count form: install
// the iterator, build mark + body + move-cont tokens; the engine's
// stepMoveCont drives the remaining iterations through the Loop.
func fixRunForLoop(r *core.Registry, start, end, step int64, iterName string, body core.Value) ([]core.Value, error) {
	if step > 0 && start >= end {
		return nil, nil
	}
	if !core.IsConcrete(&body) { //covergate:allow both callers pass a dispatch-matched List argument and handlers never run on carriers; a bare List literal cannot bind the sig slot
		return nil, &core.BoruError{Code: "for_error", Detail: "for: body must be a concrete list, got type literal"}
	}
	lst, _ := core.AsList(body)
	bodySlice := lst.Elems()

	core.InstallDef(r, iterName, core.NewInteger(start))

	bodyCopy := make([]core.Value, len(bodySlice))
	copy(bodyCopy, bodySlice)
	cont := &core.Loop{
		Registry: r,
		Word:     "for",
		Iter:     1,
		Count:    -1,
		IterName: iterName,
		Current:  start,
		End:      end,
		Step:     step,
		Body:     bodyCopy,
	}

	id := core.NextMarkID()
	tokens := make([]core.Value, 0, len(bodySlice)+2)
	tokens = append(tokens, core.NewMark(id))
	tokens = append(tokens, bodySlice...)
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
// analysis with the iterator as a typed carrier and the statically-zero
// prune.
func fixForReturnsFn(args []core.Value, r *core.Registry) []core.Value {
	body := args[1]
	iter := core.NewCarrier(core.TInteger)

	cv := args[0]
	staticCount := int64(-1)
	if core.IsConcrete(&cv) && cv.Parent().ConformsTo(core.TInteger) {
		if n, err := core.AsInteger(cv); err == nil {
			staticCount = n
			if n <= 0 {
				return []core.Value{}
			}
		}
	}

	provenTrips := staticCount >= 1
	stk := check.AnalyseLoopBody(r, body, []string{"i"}, []core.Value{iter}, provenTrips)
	out := core.NewCarrier(core.TList)
	if len(stk) > 0 {
		top := stk[len(stk)-1]
		if core.IsDisjunct(&top) {
			out = core.NewCarrierTypedListValue(top)
		} else {
			out = core.NewCarrierTypedList(top.Parent())
		}
	}
	if len(stk) == 0 {
		return []core.Value{}
	}
	// A statically-counted loop leaves the SPREAD of its per-iteration
	// residual, exactly like the runtime.
	const spreadCap = 256
	if staticCount >= 0 && len(stk) > 0 && staticCount*int64(len(stk)) <= spreadCap {
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
// inputs through the InvokeBody seam and surface a body error as an
// Error VALUE.
func fixDoHandler(args []core.Value, _ map[string]core.Value, _ []core.Value, r *core.Registry) ([]core.Value, error) {
	if !core.IsConcrete(&args[0]) { //covergate:allow the body is a dispatch-matched List argument and handlers never run on carriers; a bare List literal cannot bind the sig slot
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
	if !(core.IsConcrete(&body) && body.Parent().ConformsTo(core.TList)) {
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

// registerEngSpecCallable installs `doq` — the battery's body-runner,
// carrying basic's do effect flags — and `dofbq`, the fallback-body
// variant (CompileFallbackBody without CompileDynBody, the each/fold
// shape).
func registerEngSpecCallable(r *core.Registry) {
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "dofbq",
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
	if !core.IsConcrete(&args[0]) { //covergate:allow the range is a dispatch-matched List argument and handlers never run on carriers; a bare List literal cannot bind the sig slot
		return nil, &core.BoruError{Code: "for_error", Detail: "for: range must be a concrete list, got type literal"}
	}
	lst, _ := core.AsList(args[0])
	start, end, step, err := fixParseRange(lst.Elems())
	if err != nil {
		return nil, err
	}
	if step < 0 && start <= end {
		return nil, nil
	}
	return fixRunForLoop(r, start, end, step, "i", args[1])
}

// fixForRangeReturnsFn is the range-form model: a fully-static range
// gives the body analysis its proven trip count and the statically-zero
// prune.
func fixForRangeReturnsFn(args []core.Value, r *core.Registry) []core.Value {
	body := args[1]
	iter := core.NewCarrier(core.TInteger)

	staticBounds := false
	staticCount := int64(-1)
	if cv := args[0]; core.IsConcrete(&cv) && cv.Parent().ConformsTo(core.TList) {
		if lst, err := core.AsList(cv); err == nil && !lst.IsNil() {
			if st, en, sp, perr := fixParseRange(lst.Elems()); perr == nil {
				staticBounds = true
				staticCount = fixLoopIterations(st, en, sp)
				if staticCount == 0 {
					return []core.Value{}
				}
			}
		}
	}
	provenTrips := staticBounds && staticCount >= 1
	stk := check.AnalyseLoopBody(r, body, []string{"i"}, []core.Value{iter}, provenTrips)
	out := core.NewCarrier(core.TList)
	if len(stk) > 0 {
		top := stk[len(stk)-1]
		if core.IsDisjunct(&top) {
			out = core.NewCarrierTypedListValue(top)
		} else {
			out = core.NewCarrierTypedList(top.Parent())
		}
	}
	if len(stk) == 0 {
		return []core.Value{}
	}
	const spreadCap = 256
	if staticBounds && staticCount >= 0 && len(stk) > 0 && staticCount*int64(len(stk)) <= spreadCap {
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
	info, ok := args[0].Data().(*core.FnDefInfo)
	if !ok || len(info.Signatures) == 0 {
		return nil, &core.BoruError{Code: "type_error", Detail: "bakefnq: argument must be a fn"}
	}
	return r.CallBoru(&info.Signatures[0], []core.Value{core.NewInteger(7)}, nil)
}

// registerEngSpecBaking installs `bakefnq` and `bakebodyq` — battery
// fixtures declaring the stored-fn / stored-body effect flags (the spawn /
// service / codec-handler shapes).
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
// InvokeBody seam — the minimal mirror of lang's each.
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
// eachReturnsFn).
func fixEachReturnsFn(args []core.Value, r *core.Registry) []core.Value {
	body := args[0]
	if !(core.IsConcrete(&body) && body.Parent().ConformsTo(core.TList)) {
		return []core.Value{core.NewCarrier(core.TList)}
	}
	bl, err := core.AsList(body)
	if err != nil || bl.IsNil() { //covergate:allow guarded by IsConcrete+ConformsTo(TList) above; AsList cannot fail and a concrete list is never nil-backed
		return []core.Value{core.NewCarrier(core.TList)}
	}
	toks := append([]core.Value{check.ElementCarrierFromValue(args[1])}, bl.Elems()...)
	stk := core.RunCarrierBody(r, core.NewList(toks))
	if len(stk) == 0 {
		return []core.Value{core.NewCarrier(core.TList)}
	}
	return []core.Value{core.NewCarrierTypedList(stk[len(stk)-1].Parent())}
}

// registerEngSpecHigherOrder installs eachq — the battery's higher-order
// word (quotation and Function forms).
func registerEngSpecHigherOrder(r *core.Registry) {
	r.RegisterNativeFunc(core.NativeFunc{
		Name:          "eachq",
		CompileEffect: core.CompileFallbackBody,
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
