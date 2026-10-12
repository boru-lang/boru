package check_test

// The check corpus's fixture vocabulary.
//
// A bare core.NewRegistry() carries NO words at all — the base language
// vocabulary lives in basic/go, and the corpus-wide fixture set lives in
// test/specfix, neither of which check may import (basic is a sibling that
// would add a production edge; specfix REQUIRES check, so importing it
// closes a cycle). So the corpus declares its own.
//
// This set is deliberately minimal. It is not a copy of specfix's
// vocabulary: it carries only the shapes check's own analysis needs to be
// driven through — a monomorphic word, an overloaded word (dispatch
// selection), a word over a type union (carrier joins), and a predicate
// (guard narrowing). Words are named with a `q` suffix, the repo's
// convention marking a spec fixture rather than a production word.

import (
	check "github.com/boru-lang/boru/check/go"
	core "github.com/boru-lang/boru/core/go"
)

// goImpl adapts a plain function to the native-func signature.
func goImpl(f func(args []core.Value) []core.Value) *core.GoImpl {
	return core.Go(func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
		return f(args), nil
	})
}

// registerCheckFixtures installs the corpus vocabulary on r.
func registerCheckFixtures(r *core.Registry) {
	numberPair := []*core.Type{core.TNumber, core.TNumber}

	// addq — monomorphic over Number: the simplest dispatch shape, and the
	// baseline every row builds on.
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "addq",
		Signatures: []core.Signature{{
			Args:       numberPair,
			Returns:    []*core.Type{core.TNumber},
			BarrierPos: -1,
			Impl: goImpl(func(args []core.Value) []core.Value {
				a, _ := core.AsInteger(args[0])
				b, _ := core.AsInteger(args[1])
				return []core.Value{core.NewInteger(a + b)}
			}),
		}},
	})

	// twoq — two overloads on distinct argument types. This is the shape
	// that drives overload selection, and with a non-concrete operand the
	// dynamic-reachable-overload counting in carrier.go.
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "twoq",
		Signatures: []core.Signature{
			{
				Args: []*core.Type{core.TInteger}, Returns: []*core.Type{core.TInteger},
				BarrierPos: 1,
				Impl:       goImpl(func(args []core.Value) []core.Value { return []core.Value{args[0]} }),
			},
			{
				Args: []*core.Type{core.TString}, Returns: []*core.Type{core.TString},
				BarrierPos: 1,
				Impl:       goImpl(func(args []core.Value) []core.Value { return []core.Value{args[0]} }),
			},
		},
	})

	// strq — monomorphic over String, for rows that need a second concrete
	// leaf type to join against.
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "strq",
		Signatures: []core.Signature{{
			Args: []*core.Type{core.TString}, Returns: []*core.Type{core.TString},
			BarrierPos: 1,
			Impl:       goImpl(func(args []core.Value) []core.Value { return []core.Value{args[0]} }),
		}},
	})

	// anyq — Integer -> Any. The point of this word is its RETURN: Any is
	// not a concrete leaf, so its result is a non-concrete operand. Feeding
	// that into an overloaded word is what drives the dynamic-reachable
	// overload counting and the "is any operand non-concrete" family in
	// carrier.go, none of which a fully concrete row can reach.
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "anyq",
		Signatures: []core.Signature{{
			Args: []*core.Type{core.TInteger}, Returns: []*core.Type{core.TAny},
			BarrierPos: 1,
			Impl:       goImpl(func(args []core.Value) []core.Value { return []core.Value{args[0]} }),
		}},
	})

	// defq — `defq name [triples]` installs a user fn: the list is walked
	// by core.ParseFnDef in [input-sig, output-sig, body] triples and
	// installed with core.InstallFnDef, mirroring the shape of specfix's
	// `def name fn [...]` fixture (test/specfix/words.go) minus the keyword
	// slot. This is the word that opens the fn-body analysis surface:
	// installing a boru-bodied fn is what drives check_fnbody.go's
	// construction pass and the AnalyseFnBody family in carrier.go, none of
	// which a native-only vocabulary can reach. RunInCheck makes the
	// definition fire during the check pass itself.
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "defq",
		Signatures: []core.Signature{{
			Args:       []*core.Type{core.TAtom, core.TList},
			QuoteArgs:  map[int]bool{0: true},
			NoEvalArgs: map[int]bool{1: true},
			Returns:    []*core.Type{}, BarrierPos: -1,
			Impl: core.Go(func(args []core.Value, _ map[string]core.Value, _ []core.Value, reg *core.Registry) ([]core.Value, error) {
				name, _ := args[0].AsConcreteAtom()
				if err := core.ValidateWordName(name); err != nil {
					return nil, err
				}
				if !core.IsConcrete(&args[1]) {
					return nil, &core.BoruError{Code: "type_error", Detail: "defq: body must be a concrete list"}
				}
				lst, _ := core.AsList(args[1])
				elems := lst.Slice()
				if len(elems) == 0 || len(elems)%3 != 0 {
					return nil, &core.BoruError{Code: "fn_invalid_spec", Detail: "defq: list length must be a non-zero multiple of 3"}
				}
				fnDef, err := core.ParseFnDef(reg, elems)
				if err != nil {
					return nil, err
				}
				reg.Check.RecordDef(name, core.SrcPos{})
				core.InstallFnDef(reg, name, fnDef)
				return nil, nil
			}, core.RunInCheck()),
		}},
	})

	// predq — Integer -> Boolean, the predicate shape guard narrowing and
	// the fn-predicate overload hazard analysis look for.
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "predq",
		Signatures: []core.Signature{{
			Args: []*core.Type{core.TInteger}, Returns: []*core.Type{core.TBoolean},
			BarrierPos: 1,
			Impl: goImpl(func(args []core.Value) []core.Value {
				n, _ := core.AsInteger(args[0])
				return []core.Value{core.NewBoolean(n > 0)}
			}),
		}},
	})
}

// registerControlFixtures installs a minimal if and for, the corpus's
// door into the branch/loop analysis. They orchestrate the SAME core
// seam functions basic's production control words do — guard narrowing,
// carrier body runs, the def join, and check.AnalyseLoopBody — but
// deliberately none of the recording protocol: check's own suite runs
// with the DispatchBraid inactive, so there is no fragment to capture and
// no branch record to emit, and the check semantics (narrowing, joins,
// diagnostics, the loop body model) are exactly what this corpus is here
// to pin.
//
// Shapes (forward form): `ifq [cond] [then] [else]`, `forq N [body]`.
func registerControlFixtures(r *core.Registry) {
	runBody := func(reg *core.Registry, body core.Value) ([]core.Value, error) {
		lst, err := core.AsList(body)
		if err != nil {
			return nil, &core.BoruError{Code: "type_error", Detail: "control fixture: body must be a list"}
		}
		return core.NewTop(reg).Run(lst.Slice())
	}

	ifqHandler := func(args []core.Value, _ map[string]core.Value, _ []core.Value, reg *core.Registry) ([]core.Value, error) {
		condOut, err := runBody(reg, args[0])
		if err != nil {
			return nil, err
		}
		takeThen := len(condOut) > 0 && core.CoerceBoolean(condOut[len(condOut)-1])
		if takeThen {
			return runBody(reg, args[1])
		}
		return runBody(reg, args[2])
	}
	// The check model: the literal-condition reduction with its
	// unreachable-branch diagnostic, per-arm body capture under guard /
	// complement narrowing, the def join, and the carrier-stack join.
	ifqReturnsFn := func(args []core.Value, reg *core.Registry) []core.Value {
		if lit, ok := core.LiteralCondValue(args[0]); ok {
			branch := "else"
			if !lit {
				branch = "then"
			}
			reg.Check.AddDiagnostic(core.CheckDiagnostic{
				Code:   "unreachable_branch",
				Detail: "ifq condition is a constant " + core.BoolWord(lit) + "; " + branch + "-branch is unreachable",
				Word:   "ifq",
				Row:    reg.Check.CurCallPos.Row,
				Col:    reg.Check.CurCallPos.Col,
			})
			var stk []core.Value
			var defs map[string]core.Value
			if lit {
				restore := core.ApplyGuardNarrowing(reg, args[0])
				stk, defs = core.RunCarrierBodyWithDefs(reg, args[1])
				restore()
				core.InstallJoinedDefs(reg, defs, nil)
			} else {
				restore := core.ApplyComplementNarrowing(reg, args[0])
				stk, defs = core.RunCarrierBodyWithDefs(reg, args[2])
				restore()
				core.InstallJoinedDefs(reg, nil, defs)
			}
			return stk
		}
		condStk, _ := core.RunCarrierBodyWithDefs(reg, args[0])
		_ = condStk
		restore := core.ApplyGuardNarrowing(reg, args[0])
		thenStk, thenDefs := core.RunCarrierBodyWithDefs(reg, args[1])
		restore()
		restore = core.ApplyComplementNarrowing(reg, args[0])
		elseStk, elseDefs := core.RunCarrierBodyWithDefs(reg, args[2])
		restore()
		core.InstallJoinedDefs(reg, thenDefs, elseDefs)
		return core.JoinCarrierStacks(thenStk, elseStk)
	}
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "ifq",
		Signatures: []core.Signature{{
			Args:       []*core.Type{core.TList, core.TList, core.TList},
			NoEvalArgs: map[int]bool{0: true, 1: true, 2: true},
			Impl:       core.Go(ifqHandler),
			ReturnsFn:  ifqReturnsFn, BarrierPos: -1,
		}},
	})

	forqHandler := func(args []core.Value, _ map[string]core.Value, _ []core.Value, reg *core.Registry) ([]core.Value, error) {
		n, _ := core.AsInteger(args[0])
		var out []core.Value
		for i := int64(0); i < n; i++ {
			res, err := runBody(reg, args[1])
			if err != nil {
				return nil, err
			}
			out = res
		}
		return out, nil
	}
	// The check model: the loop body analysed with the iterator bound as a
	// typed carrier. A LITERAL count > 0 proves at least one trip; a
	// computed count must not (a runtime count of 0 runs the body never).
	forqReturnsFn := func(args []core.Value, reg *core.Registry) []core.Value {
		proven := false
		if core.IsConcrete(&args[0]) {
			if n, err := core.AsInteger(args[0]); err == nil && n > 0 {
				proven = true
			}
		}
		return check.AnalyseLoopBody(reg, args[1], []string{"iq"}, []core.Value{core.NewCarrier(core.TInteger)}, proven)
	}
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "forq",
		Signatures: []core.Signature{{
			Args:       []*core.Type{core.TInteger, core.TList},
			NoEvalArgs: map[int]bool{1: true},
			Impl:       core.Go(forqHandler),
			ReturnsFn:  forqReturnsFn, BarrierPos: -1,
		}},
	})
}

// registerDryFixtures installs dryq, the corpus's door into the
// check-mode dry pass (drypass.go): a PURE word over concrete arguments
// runs its real handler once during analysis on the top-level straight
// line, so a guaranteed runtime error surfaces as a check diagnostic.
// The handler errors on a negative operand, which is what lets a row
// show the difference between a fault that is unconditionally reached
// (top level: diagnosed) and one whose reachability is conditional (a
// fn or loop body: silent).
func registerDryFixtures(r *core.Registry) {
	dryHandler := func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
		n, err := core.AsInteger(args[0])
		if err != nil {
			return nil, err
		}
		if n < 0 {
			return nil, &core.BoruError{Code: "domain_error", Detail: "dryq: negative operand"}
		}
		return []core.Value{core.NewInteger(n)}, nil
	}
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "dryq",
		Signatures: []core.Signature{{
			Args: []*core.Type{core.TInteger}, Returns: []*core.Type{core.TInteger},
			BarrierPos: 1,
			Impl:       core.Go(dryHandler),
			ReturnsFn:  check.DryPassReturns(dryHandler, core.TInteger),
		}},
	})
}

// registerIndexFixtures installs atq, the corpus's door into the static
// index bounds check (indexcheck.go): `[indices] [data] atq` selects the
// indexed elements, and its check model runs check.CheckAtIndices so a
// provably out-of-range index over a statically-known list length is
// flagged during analysis. Unknown lengths and unknown indices are
// deliberately silent — the checker only speaks when it can prove the
// fault.
func registerIndexFixtures(r *core.Registry) {
	// Dispatch hands operands TOS-first: for `[indices] [data] atq` the
	// data list (pushed last) is args[0] and the index list args[1].
	atqHandler := func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
		data, err := core.AsList(args[0])
		if err != nil {
			return nil, err
		}
		idxs, err := core.AsList(args[1])
		if err != nil {
			return nil, err
		}
		var out []core.Value
		for i := 0; i < idxs.Len(); i++ {
			n, err := addrOf(idxs.Get(i)).AsConcreteInteger()
			if err != nil || n < 0 || int(n) >= data.Len() {
				return nil, &core.BoruError{Code: "index_error", Detail: "atq: index out of range"}
			}
			out = append(out, data.Get(int(n)))
		}
		return []core.Value{core.NewList(out)}, nil
	}
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "atq",
		Signatures: []core.Signature{{
			Args: []*core.Type{core.TList, core.TList}, Returns: []*core.Type{core.TList},
			BarrierPos: -1,
			Impl:       core.Go(atqHandler),
			ReturnsFn: func(args []core.Value, reg *core.Registry) []core.Value {
				check.CheckAtIndices(reg, args[1], args[0], "atq")
				return []core.Value{core.NewCarrier(core.TList)}
			},
		}},
	})
}

// registerMopupFixtures installs letq and tri3q, the last two corpus
// doors.
//
// letq is the plain def binding (`letq name value`, RunInCheck): reading
// the bound name back is what drives the check-mode def-read tagging —
// a dynamic bound value tags its origin, so diagnostics can say which
// def a dynamic operand came from.
//
// tri3q is a 3-arg all-Any word: dispatching it in MIXED form (some
// operands from the stack, some forward-collected) with an Any-typed
// stack slot is the precondition for the mixed_form_call advisory — the
// genuine footgun where the mixed reading binds differently from the
// all-forward one.
func registerMopupFixtures(r *core.Registry) {
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "letq",
		Signatures: []core.Signature{{
			Args:      []*core.Type{core.TAtom, core.TAny},
			QuoteArgs: map[int]bool{0: true},
			Returns:   []*core.Type{}, BarrierPos: -1,
			Impl: core.Go(func(args []core.Value, _ map[string]core.Value, _ []core.Value, reg *core.Registry) ([]core.Value, error) {
				name, _ := args[0].AsConcreteAtom()
				if err := core.ValidateWordName(name); err != nil {
					return nil, err
				}
				reg.Check.RecordDef(name, core.SrcPos{})
				reg.Defs.Push(name, args[1])
				return nil, nil
			}, core.RunInCheck()),
		}},
	})

	r.RegisterNativeFunc(core.NativeFunc{
		Name: "tri3q",
		Signatures: []core.Signature{{
			Args:    []*core.Type{core.TAny, core.TAny, core.TAny},
			Returns: []*core.Type{core.TInteger}, BarrierPos: -1,
			Impl: goImpl(func(args []core.Value) []core.Value {
				return []core.Value{core.NewInteger(int64(len(args)))}
			}),
		}},
	})
}
