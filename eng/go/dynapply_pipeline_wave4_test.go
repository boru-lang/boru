package eng

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// Coverage for the dynamic fn-value apply seam across the pipeline:
// emit.go resolveDynamicApply / trailingApply / trailingWindowApplyShape /
// mixedDynamicApplyShape / anyDynamicTail / anyFnOrDynamicTail /
// RecordMakeMap, vm.go callDynamic / callDynamicOp / callDynamicMixed /
// invokeClosure / vmMakeMap / callPoly / matchUserPoly, and engine.go
// checkModeAssumeSig's plain-check summaries (argTypeSummary /
// sigTypeSummary). Reuses the harness from compile_pipeline_cov_test.go.

// registerDynWords adds:
//   - cany:  0-arg, declared Any (a dynamic carrier statically), returns 7.
//   - cmk1:  0-arg factory returning a 1-arity closure (adds 100).
//   - cmk2:  0-arg factory returning a 2-arity closure (csub-like, order
//     sensitive: returns args[1]-args[0]).
//   - upoly: user fn with Integer and String overloads.
func registerDynWords(r *core.Registry) {
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "cany",
		Signatures: []core.Signature{{
			Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
				return []core.Value{core.NewInteger(7)}, nil
			}),
			Returns: []*core.Type{core.TAny}, BarrierPos: -1,
		}},
	})
	mkClosure := func(params []core.FnParam, body []core.Value) core.Value {
		return core.NewFunction(core.FnDefInfo{
			Anonymous: true,
			Signatures: []core.Signature{{
				Params:     params,
				Returns:    []*core.Type{core.TInteger},
				Impl:       core.Boru(body),
				BarrierPos: core.BarrierAllForward,
			}},
		})
	}
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "cmk1",
		Signatures: []core.Signature{{
			Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
				return []core.Value{mkClosure(
					[]core.FnParam{{Name: "x", Type: core.TInteger}},
					parenBody(core.NewWord("cadd"), core.NewWord("x"), core.NewInteger(100)),
				)}, nil
			}),
			Returns: []*core.Type{core.TFunction}, BarrierPos: -1,
		}},
	})
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "cmk2",
		Signatures: []core.Signature{{
			Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
				return []core.Value{mkClosure(
					[]core.FnParam{{Name: "a", Type: core.TInteger}, {Name: "b", Type: core.TInteger}},
					parenBody(core.NewWord("cmul"), core.NewWord("a"), core.NewInteger(10),
						core.NewEnd(), core.NewWord("cadd"), core.NewWord("b")),
				)}, nil
			}),
			Returns: []*core.Type{core.TFunction}, BarrierPos: -1,
		}},
	})
	core.InstallFnDef(r, "upoly", core.FnDefInfo{
		Signatures: []core.Signature{
			{
				Params:     []core.FnParam{{Name: "n", Type: core.TInteger}},
				Returns:    []*core.Type{core.TInteger},
				Impl:       core.Boru(parenBody(core.NewWord("cadd"), core.NewWord("n"), core.NewInteger(1))),
				BarrierPos: core.BarrierAllForward,
			},
			{
				Params:     []core.FnParam{{Name: "s", Type: core.TString}},
				Returns:    []*core.Type{core.TString},
				Impl:       core.Boru(parenBody(core.NewWord("ccat"), core.NewWord("s"), core.NewString("!"))),
				BarrierPos: core.BarrierAllForward,
			},
		},
	})
}

// runTolerant executes tokens interpreted, asserts the expected render,
// then compiles; a compile failure is tolerated (logged), a compiled program must
// agree with the interpreter.
func runTolerant(t *testing.T, extra func(*core.Registry), tokens func() []core.Value, want string) {
	t.Helper()
	ri := covRegistry(t, extra)
	iOut, iErr := core.NewTop(ri).Run(tokens())
	if iErr != nil {
		t.Fatalf("interpreted: %v", iErr)
	}
	if got := renderAll(iOut); got != want {
		t.Fatalf("interpreted = %q, want %q", got, want)
	}
	rc := covRegistry(t, extra)
	prog, reason := compileTokens(t, rc, tokens())
	if prog == nil {
		t.Logf("compile declined (%s)", reason)
		return
	}
	cOut, cErr := RunProgram(prog, rc)
	if cErr != nil {
		t.Fatalf("compiled: %v", cErr)
	}
	if got := renderAll(cOut); got != want {
		t.Errorf("compiled = %q, want %q", got, want)
	}
}

// --- leading / trailing / mixed dynamic apply -------------------------------------

func TestDynApplyLeadingClosure(t *testing.T) {
	// Factory result applied to a following static arg: OpCallDynamic.
	runTolerant(t, registerDynWords, func() []core.Value {
		return []core.Value{core.NewWord("cmk1"), core.NewInteger(5)}
	}, "105")
}

func TestDynApplyTrailingClosure(t *testing.T) {
	// Fn value lands ON TOP of its single arg: OpCallDynamicTrailing.
	runTolerant(t, registerDynWords, func() []core.Value {
		return []core.Value{core.NewInteger(5), core.NewWord("cmk1")}
	}, "105")
}

func TestDynApplyLeadingNonCallable(t *testing.T) {
	// A dynamic Any value that is NOT callable: the value stays below the
	// arg on both engines.
	runTolerant(t, registerDynWords, func() []core.Value {
		return []core.Value{core.NewWord("cany"), core.NewInteger(5)}
	}, "7 | 5")
}

func TestDynApplyTrailingNonCallable(t *testing.T) {
	// Trailing non-callable: rotation puts the value back on top.
	runTolerant(t, registerDynWords, func() []core.Value {
		return []core.Value{core.NewInteger(5), core.NewWord("cany")}
	}, "5 | 7")
}

func TestDynApplyMixedWindow(t *testing.T) {
	// Interior fn value between static args: `3 (cmk1) 2` — the closure
	// takes its forward arg; the leading 3 stays.
	runTolerant(t, registerDynWords, func() []core.Value {
		return []core.Value{core.NewInteger(3), core.NewWord("cmk1"), core.NewInteger(2)}
	}, "3 | 102")
}

func TestDynApplyTrailingWindow(t *testing.T) {
	// Two static args below one trailing 2-arity closure: `10 3 (cmk2)`.
	// Closure body: (a mul 10); (add b) with a=top=3, b=10 → 40.
	runTolerant(t, registerDynWords, func() []core.Value {
		return []core.Value{core.NewInteger(10), core.NewInteger(3), core.NewWord("cmk2")}
	}, "40")
}

func TestDynApplyTwoArgLeading(t *testing.T) {
	// Leading 2-arity closure over two static forward args.
	runTolerant(t, registerDynWords, func() []core.Value {
		return []core.Value{core.NewWord("cmk2"), core.NewInteger(3), core.NewInteger(10)}
	}, "40")
}

func TestDynApplyChainedFactories(t *testing.T) {
	// Two independent applications in one program.
	runTolerant(t, registerDynWords, func() []core.Value {
		return []core.Value{
			core.NewWord("cmk1"), core.NewInteger(1), core.NewEnd(),
			core.NewWord("cmk1"), core.NewInteger(2),
		}
	}, "101 | 102")
}

// --- poly dispatch over dynamic operands --------------------------------------------

func TestPolyDispatchDynamicOperandInt(t *testing.T) {
	// cdub has Integer and String overloads; its arg is a dynamic Any
	// (cany). The compiled program re-matches at run time.
	runTolerant(t, registerDynWords, func() []core.Value {
		return []core.Value{
			core.NewWord("cdub"),
			core.NewOpenParen(), core.NewWord("cany"), core.NewCloseParen(),
		}
	}, "14")
}

func TestUserPolyDispatchDynamicOperand(t *testing.T) {
	// A user fn with two overloads over a dynamic operand: the VM
	// re-derives the overload subset and matches at run time.
	runTolerant(t, registerDynWords, func() []core.Value {
		return []core.Value{
			core.NewWord("upoly"),
			core.NewOpenParen(), core.NewWord("cany"), core.NewCloseParen(),
		}
	}, "8")
}

func TestUserSingleSigDynamicOperand(t *testing.T) {
	// Single-overload user fn over a dynamic operand: guarded CALL_USER.
	runTolerant(t, registerDynWords, func() []core.Value {
		return []core.Value{
			core.NewWord("ctwice"),
			core.NewOpenParen(), core.NewWord("cany"), core.NewCloseParen(),
		}
	}, "14")
}

// --- computed map assembly (RecordMakeMap / vmMakeMap) --------------------------------

func registerMapWords(r *core.Registry) {
	registerDynWords(r)
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "cmsize",
		Signatures: []core.Signature{{
			Args: []*core.Type{core.TMap},
			Impl: core.Go(func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
				m, err := core.AsMap(args[0])
				if err != nil {
					return nil, err
				}
				return []core.Value{core.NewInteger(int64(len(m.Keys())))}, nil
			}),
			Returns: []*core.Type{core.TInteger}, BarrierPos: -1,
		}},
	})
	// A pure identity named `print` — exprHasEffect treats the NAME as
	// effectful, so the container const-fold declines and the map records
	// an OpMakeMap assembly instead of baking a const.
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "print",
		Signatures: []core.Signature{{
			Args: []*core.Type{core.TInteger},
			Impl: core.Go(func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
				return []core.Value{args[0]}, nil
			}),
			Returns: []*core.Type{core.TInteger}, BarrierPos: -1,
		}},
	})
}

func TestCompiledComputedMapArg(t *testing.T) {
	got := runDifferential(t, registerMapWords, func() []core.Value {
		om := core.NewOrderedMap()
		om.Set("a", core.NewParenExpr([]core.Value{core.NewWord("print"), core.NewInteger(5)}))
		om.Set("b", core.NewInteger(2))
		mv := core.NewMap(om)
		mv.Eval = true
		return []core.Value{core.NewWord("cmsize"), mv}
	})
	if got != "2" {
		t.Errorf("computed map size = %q", got)
	}
}

func TestCompiledConstFoldedMapArg(t *testing.T) {
	// A deterministic computed value const-folds at the top frame
	// (constFoldContainerVal / concreteEvalOnce / constFoldAgrees) and
	// the map bakes as a const.
	got := runDifferential(t, registerMapWords, func() []core.Value {
		om := core.NewOrderedMap()
		om.Set("a", core.NewParenExpr([]core.Value{core.NewWord("cadd"), core.NewInteger(1), core.NewInteger(2)}))
		mv := core.NewMap(om)
		mv.Eval = true
		return []core.Value{core.NewWord("cmsize"), mv}
	})
	if got != "1" {
		t.Errorf("folded map size = %q", got)
	}
}

func TestCompiledMapResidual(t *testing.T) {
	// A computed map as the PROGRAM residual (not consumed): parity.
	runTolerant(t, registerMapWords, func() []core.Value {
		om := core.NewOrderedMap()
		om.Set("k", core.NewParenExpr([]core.Value{core.NewWord("print"), core.NewInteger(9)}))
		mv := core.NewMap(om)
		mv.Eval = true
		return []core.Value{mv}
	}, "{k:9}")
}

// --- loop-body list assembly (RecordMakeListInner) --------------------------------------

func TestCompiledLoopBodyListArg(t *testing.T) {
	// A computed list consumed by clen INSIDE a loop body re-assembles
	// per iteration (OpMakeList in the loop unit).
	setup := func(r *core.Registry) {
		registerLoopWords(r)
	}
	runTolerant(t, setup, func() []core.Value {
		lv := core.NewList([]core.Value{core.NewWord("i"), core.NewInteger(5)})
		lv.Eval = true
		return []core.Value{
			core.NewWord("cfor"), core.NewInteger(3),
			codeBody(core.NewWord("clen"), lv),
		}
	}, "2 | 2 | 2")
}

// --- plain-check diagnostics (argTypeSummary / sigTypeSummary) ---------------------------

func TestPlainCheckNoSignatureSummaries(t *testing.T) {
	// A plain check pass (Mode on, NOT compiling) over a concrete
	// mismatched dispatch emits the got/nearest summary diagnostic.
	r := covRegistry(t, nil)
	done := r.Check.Begin()
	_, err := core.NewTop(r).Run([]core.Value{core.NewWord("cadd"), core.NewString("a"), core.NewString("b")})
	if err != nil {
		t.Fatalf("plain check errored hard: %v", err)
	}
	var found string
	for _, d := range r.Check.Diagnostics {
		if d.Code == "no_signature" && strings.Contains(d.Detail, "cadd") {
			found = d.Detail
			break
		}
	}
	done()
	if found == "" {
		t.Fatal("no no_signature diagnostic for cadd")
	}
	if !strings.Contains(found, "got (") || !strings.Contains(found, "nearest [") {
		t.Errorf("summary missing got/nearest: %q", found)
	}
}

// --- carrier no-match dispatch: interpreter-fallback parity ------------------------------

func TestCompiledCarrierDisjointTrapParity(t *testing.T) {
	// The failing operand is a CARRIER (a ccat result — String) in an
	// Integer-only slot. A carrier is not concrete at compile time, so a
	// trap built here could not carry a diagnostic matching the
	// interpreter's runtime (concrete-value) one; the compile therefore
	// DECLINES to trap and the program does not compile. Both
	// engines raise the identical signature_error either way (phase 7).
	runErrParity(t, nil, func() []core.Value {
		return []core.Value{
			core.NewWord("cadd"),
			core.NewOpenParen(), core.NewWord("ccat"), core.NewString("a"), core.NewString("b"), core.NewCloseParen(),
			core.NewInteger(2),
		}
	}, "signature_error")
}

func TestCompiledConcreteTrapVoidGroup(t *testing.T) {
	// A void argument group (a paren producing no value) starves the
	// dispatch: the trap carries the void-group override taxonomy.
	setup := func(r *core.Registry) {
		r.RegisterNativeFunc(core.NativeFunc{
			Name: "cvoid",
			Signatures: []core.Signature{{
				Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
					return nil, nil
				}),
				Returns: []*core.Type{}, BarrierPos: -1,
			}},
		})
	}
	ri := covRegistry(t, setup)
	_, iErr := core.NewTop(ri).Run([]core.Value{
		core.NewWord("cneg"),
		core.NewOpenParen(), core.NewWord("cvoid"), core.NewCloseParen(),
	})
	if iErr == nil {
		t.Fatal("void arg group did not error")
	}
	rc := covRegistry(t, setup)
	prog, reason := compileTokens(t, rc, []core.Value{
		core.NewWord("cneg"),
		core.NewOpenParen(), core.NewWord("cvoid"), core.NewCloseParen(),
	})
	if prog == nil {
		t.Logf("compile declined (%s)", reason)
		return
	}
	_, cErr := RunProgram(prog, rc)
	if cErr == nil {
		t.Fatal("compiled void arg group did not error")
	}
}

// --- direct VM helper probes -----------------------------------------------------------

func TestInvokeClosureRawTokens(t *testing.T) {
	// invokeClosure with a NON-closure body value runs the raw tokens in
	// a sub-engine (the island path).
	r := covRegistry(t, nil)
	prog, reason := compileTokens(t, r, []core.Value{core.NewInteger(1)})
	if prog == nil {
		t.Fatalf("trivial compile declined: %s", reason)
	}
	vc := &vmContext{p: prog, r: r}
	body := core.NewList([]core.Value{core.NewWord("cadd")})
	out, err := vc.invokeClosure(r, body, []core.Value{core.NewInteger(2), core.NewInteger(3)})
	if err != nil {
		t.Fatalf("invokeClosure raw: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("raw closure out = %v", out)
	}
	if n, _ := core.AsInteger(out[0]); n != 5 {
		t.Errorf("raw closure result = %v", out[0])
	}
}

func TestIsAppliableFnShapes(t *testing.T) {
	fn := core.NewFunction(core.FnDefInfo{Name: "f"})
	if !core.IsAppliableFn(fn) {
		t.Error("Function value not appliable")
	}
	if core.IsAppliableFn(core.NewInteger(1)) {
		t.Error("integer appliable")
	}
	carrier := core.NewCarrier(core.TFunction)
	if !core.IsAppliableFn(carrier) {
		t.Error("Function-typed carrier not appliable")
	}
	if !core.IsFnTypedCarrier(carrier) {
		t.Error("Function carrier not fn-typed")
	}
	if core.IsFnTypedCarrier(core.NewCarrier(core.TInteger)) {
		t.Error("Integer carrier fn-typed")
	}
	if !core.IsFnValueResidual(core.NewFunction(core.FnDefInfo{Name: "g"})) {
		t.Error("FnDef value not a fn residual")
	}
	if core.IsFnValueResidual(core.NewString("s")) {
		t.Error("string a fn residual")
	}
}
