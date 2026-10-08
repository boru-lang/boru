package specfix

import (
	"fmt"
	"strconv"
	"strings"

	check "github.com/boru-lang/boru/check/go"
	core "github.com/boru-lang/boru/core/go"
)

// The spec-fixture word set: minimal, single-overload registrations
// tailored for corpus coverage of the dispatch / value / type-lattice
// core, wired to the eng-exported algorithm primitives. Shared by the
// engspec harness (test/go/engspec — full corpus, with basic's Micron
// ideal installed on top) and by eng's own standalone corpus run
// (eng/go, which supplies a mechanism-only stand-in for the content
// layer). Moved from test/go/engspec/engspec_test.go.

// specReplayCounter is bumped per call to the `replayq` test fixture so
// each Mark/Move pair gets a unique ID across a spec file.
var specReplayCounter int

// RegisterSpecWords installs the eng core words plus the spec-runner
// test fixtures on a registry. The fixtures are minimal, single-overload
// variants tailored for spec coverage of the dispatch / value /
// type-lattice core.
func RegisterSpecWords(r *core.Registry) {
	toFloat := func(v core.Value) float64 {
		if v.Parent.ConformsTo(core.TInteger) {
			n, _ := core.AsInteger(v)
			return float64(n)
		}
		f, _ := core.AsFloat(v)
		return f
	}
	numericBinary := func(intOp func(a, b int64) int64, floatOp func(a, b float64) float64) core.Handler {
		return func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
			if args[0].Parent.ConformsTo(core.TInteger) && args[1].Parent.ConformsTo(core.TInteger) {
				a, _ := core.AsInteger(args[0])
				b, _ := core.AsInteger(args[1])
				return []core.Value{core.NewInteger(intOp(a, b))}, nil
			}
			return []core.Value{core.NewFloat(floatOp(toFloat(args[0]), toFloat(args[1])))}, nil
		}
	}
	numberPair := []*core.Type{core.TNumber, core.TNumber}

	r.RegisterNativeFunc(core.NativeFunc{
		Name: "addq",
		Signatures: []core.Signature{{
			Args:    numberPair,
			Impl:    core.Go(numericBinary(func(a, b int64) int64 { return b + a }, func(a, b float64) float64 { return b + a })),
			Returns: []*core.Type{core.TNumber}, BarrierPos: -1,
		}},
	})
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "subq",
		Signatures: []core.Signature{{
			Args:    numberPair,
			Impl:    core.Go(numericBinary(func(a, b int64) int64 { return b - a }, func(a, b float64) float64 { return b - a })),
			Returns: []*core.Type{core.TNumber}, BarrierPos: -1,
		}},
	})
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "mulq",
		Signatures: []core.Signature{{
			Args:    numberPair,
			Impl:    core.Go(numericBinary(func(a, b int64) int64 { return b * a }, func(a, b float64) float64 { return b * a })),
			Returns: []*core.Type{core.TNumber}, BarrierPos: -1,
		}},
	})
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "negq",
		Signatures: []core.Signature{{
			Args: []*core.Type{core.TNumber}, BarrierPos: 1,
			Impl: core.Go(func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
				if args[0].Parent.ConformsTo(core.TInteger) {
					n, _ := core.AsInteger(args[0])
					return []core.Value{core.NewInteger(-n)}, nil
				}
				f, _ := core.AsFloat(args[0])
				return []core.Value{core.NewFloat(-f)}, nil
			}),
			Returns: []*core.Type{core.TNumber},
		}},
	})
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "concatq",
		Signatures: []core.Signature{{
			Args: []*core.Type{core.TString, core.TString},
			Impl: core.Go(func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
				a, _ := core.AsString(args[0])
				b, _ := core.AsString(args[1])
				return []core.Value{core.NewString(b + a)}, nil
			}),
			Returns: []*core.Type{core.TString}, BarrierPos: -1,
		}},
	})
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "describeq",
		Signatures: []core.Signature{
			{
				Args: []*core.Type{core.TInteger},
				Impl: core.Go(func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
					n, _ := core.AsInteger(args[0])
					return []core.Value{core.NewString("int:" + strconv.FormatInt(n, 10))}, nil
				}),
				Returns: []*core.Type{core.TString}, BarrierPos: -1,
			},
			{
				Args: []*core.Type{core.TString},
				Impl: core.Go(func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
					s, _ := core.AsString(args[0])
					return []core.Value{core.NewString("str:" + s)}, nil
				}),
				Returns: []*core.Type{core.TString}, BarrierPos: -1,
			},
		},
	})
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "tagq",
		Signatures: []core.Signature{
			{Args: []*core.Type{core.TAny}, Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
				return []core.Value{core.NewString("any")}, nil
			}), Returns: []*core.Type{core.TString}, BarrierPos: -1},
			{Args: []*core.Type{core.TInteger}, Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
				return []core.Value{core.NewString("specific")}, nil
			}), Returns: []*core.Type{core.TString}, BarrierPos: -1},
		},
	})
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "factq",
		Signatures: []core.Signature{
			{
				Args: []*core.Type{core.TInteger}, Patterns: map[int]core.Value{0: core.NewInteger(0)},
				Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
					return []core.Value{core.NewInteger(1)}, nil
				}),
				Returns: []*core.Type{core.TInteger}, BarrierPos: -1,
			},
			{
				Args: []*core.Type{core.TInteger},
				Impl: core.Go(func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
					n, _ := core.AsInteger(args[0])
					return []core.Value{core.NewInteger(n)}, nil
				}),
				Returns: []*core.Type{core.TInteger}, BarrierPos: -1,
			},
		},
	})
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "codeq",
		Signatures: []core.Signature{
			{
				Args: []*core.Type{core.TInteger}, Patterns: map[int]core.Value{0: core.NewInteger(99)},
				Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
					return []core.Value{core.NewString("ninety-nine")}, nil
				}),
				Returns: []*core.Type{core.TString}, BarrierPos: -1,
			},
			{
				Args: []*core.Type{core.TInteger},
				Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
					return []core.Value{core.NewString("general")}, nil
				}),
				Returns: []*core.Type{core.TString}, BarrierPos: -1,
			},
		},
	})
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "routeq",
		Signatures: []core.Signature{
			{
				Args: []*core.Type{core.TString}, Patterns: map[int]core.Value{0: core.NewString("admin")},
				Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
					return []core.Value{core.NewString("matched-admin")}, nil
				}),
				Returns: []*core.Type{core.TString}, BarrierPos: -1,
			},
			{
				Args: []*core.Type{core.TString},
				Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
					return []core.Value{core.NewString("other")}, nil
				}),
				Returns: []*core.Type{core.TString}, BarrierPos: -1,
			},
		},
	})
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "tripq",
		Signatures: []core.Signature{{
			Args: []*core.Type{core.TInteger, core.TInteger, core.TInteger},
			Impl: core.Go(func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
				a, _ := core.AsInteger(args[0])
				b, _ := core.AsInteger(args[1])
				c, _ := core.AsInteger(args[2])
				return []core.Value{core.NewString(fmt.Sprintf("%d,%d,%d", a, b, c))}, nil
			}),
			Returns: []*core.Type{core.TString}, BarrierPos: -1,
		}},
	})
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "pairq",
		Signatures: []core.Signature{{
			Args:       []*core.Type{core.TInteger, core.TInteger},
			BarrierPos: 1,
			Impl: core.Go(func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
				a, _ := core.AsInteger(args[0])
				b, _ := core.AsInteger(args[1])
				return []core.Value{core.NewString(fmt.Sprintf("%d:%d", a, b))}, nil
			}),
			Returns: []*core.Type{core.TString},
		}},
	})

	// ── Barrier / arity fixtures (for barrier.tsv) ────────────────
	// nilq — a 0-arg word. Exercises 0-arity sigs and the `/0`
	// argCount filter (the fallback-section match path).
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "nilq",
		Signatures: []core.Signature{{
			Args: []*core.Type{},
			Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
				return []core.Value{core.NewString("nil")}, nil
			}),
			Returns: []*core.Type{core.TString}, BarrierPos: 0,
		}},
	})

	// flexq — two overloads of different arity, [Integer] and
	// [Integer, Integer], both forward-eligible (BarrierPos = N). The
	// 1-arg sig is tried first, so a bare `flexq` always picks it; the
	// `/N` argCount modifier (flexq/1, flexq/2) selects the overload
	// explicitly, and `/1f`, `/2s` etc. combine arity selection with a
	// forced forward/stack boundary.
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "flexq",
		Signatures: []core.Signature{
			{
				Args: []*core.Type{core.TInteger},
				Impl: core.Go(func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
					a, _ := core.AsInteger(args[0])
					return []core.Value{core.NewString(fmt.Sprintf("one:%d", a))}, nil
				}),
				Returns: []*core.Type{core.TString}, BarrierPos: -1,
			},
			{
				Args: []*core.Type{core.TInteger, core.TInteger},
				Impl: core.Go(func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
					a, _ := core.AsInteger(args[0])
					b, _ := core.AsInteger(args[1])
					return []core.Value{core.NewString(fmt.Sprintf("two:%d,%d", a, b))}, nil
				}),
				Returns: []*core.Type{core.TString}, BarrierPos: -1,
			},
		},
	})

	// Fixed-arity Integer formatters with an intrinsic barrier at the
	// position named by the numeric suffix (the un-suffixed "main"
	// series — pairq=2/B1, tripq=3/B3, quadq=4/B2, quintq=5/B3,
	// hexq=6/B3, septq=7/B4 — plus tri1q/tri2q and quad1q/quad3q for
	// the off-centre boundaries). Each handler renders its args in
	// signature order, comma-separated, so a row's output reveals
	// exactly which source token bound to which sig position.
	// Combined with /s, /f and /N the rows reach every boundary
	// position 0..N for arities 1..7.
	intArgsFmt := func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
		parts := make([]string, len(args))
		for i, a := range args {
			n, _ := core.AsInteger(a)
			parts[i] = strconv.FormatInt(n, 10)
		}
		return []core.Value{core.NewString(strings.Join(parts, ","))}, nil
	}
	intArity := func(name string, n, barrier int) {
		args := make([]*core.Type, n)
		for i := range args {
			args[i] = core.TInteger
		}
		r.RegisterNativeFunc(core.NativeFunc{
			Name: name,
			Signatures: []core.Signature{{
				Args: args, BarrierPos: barrier,
				Impl:    core.Go(intArgsFmt),
				Returns: []*core.Type{core.TString},
			}},
		})
	}
	intArity("tri1q", 3, 1)
	intArity("tri2q", 3, 2)
	intArity("quad1q", 4, 1)
	intArity("quadq", 4, 2)
	intArity("quad3q", 4, 3)
	intArity("quintq", 5, 3)
	intArity("hexq", 6, 3)
	intArity("septq", 7, 4)

	r.RegisterNativeFunc(core.NativeFunc{
		Name: "lengthq",
		Signatures: []core.Signature{{
			Args: []*core.Type{core.TList},
			Impl: core.Go(func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
				lst, _ := core.AsList(args[0])
				return []core.Value{core.NewInteger(int64(lst.Len()))}, nil
			}),
			Returns: []*core.Type{core.TInteger}, BarrierPos: -1,
		}},
	})
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "firstq",
		Signatures: []core.Signature{{
			Args: []*core.Type{core.TList},
			Impl: core.Go(func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
				lst, _ := core.AsList(args[0])
				if lst.Len() == 0 {
					return []core.Value{core.NewNone()}, nil
				}
				return []core.Value{lst.Get(0)}, nil
			}),
			Returns: []*core.Type{core.TAny}, BarrierPos: -1,
		}},
	})
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "replayq",
		Signatures: []core.Signature{{
			Args:       []*core.Type{core.TList},
			NoEvalArgs: map[int]bool{0: true},
			Impl: core.Go(func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
				_lst, _ := core.AsList(args[0])
				body := _lst.Slice()
				specReplayCounter++
				id := fmt.Sprintf("__replayq_%d", specReplayCounter)
				out := make([]core.Value, 0, len(body)+2)
				out = append(out, core.NewMark(id, body...))
				out = append(out, body...)
				out = append(out, core.NewMove(id, "replayq"))
				return out, nil
			}), BarrierPos: -1,
		}},
	})

	r.Defs.Push("pi", core.NewInteger(3))
	r.Defs.Push("tau", core.NewInteger(6))
	r.Defs.Push("greeting", core.NewString("hello"))

	// break / continue — the production words live in lang
	// (lang/go/engine/native_control.go); for engspec we register
	// kernel-side stubs that signal Registry.FlowCtrl so the
	// interp.tsv "break outside loop" rows exercise the Run-loop
	// dispatch (which IS kernel territory) without dragging the
	// whole lang word set into the engspec setup.
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "break",
		Signatures: []core.Signature{{
			Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, r *core.Registry) ([]core.Value, error) {
				r.FlowCtrl = core.FlowBreak
				return nil, nil
			}),
			Returns: []*core.Type{}, BarrierPos: 0,
		}},
	})
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "continue",
		Signatures: []core.Signature{{
			Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, r *core.Registry) ([]core.Value, error) {
				r.FlowCtrl = core.FlowContinue
				return nil, nil
			}),
			Returns: []*core.Type{}, BarrierPos: 0,
		}},
	})

	// not / and / or — production registrations live in
	// lang/go/engine/native_boolean.go. The eng kernel exposes only the
	// CoerceBoolean primitive; engspec wires it into bare not/and/or
	// names so the existing eng/spec tsvs (forth.tsv, inspect.tsv,
	// types.tsv, …) keep exercising the dispatch path.
	registerEngSpecBoolean(r)
	registerEngSpecTypeOps(r)
	registerEngSpecDo(r)
	registerEngSpecFnSig(r)
	registerEngSpecObjectRecord(r)
	registerEngSpecStorage(r)
	registerEngSpecInspect(r)
	registerEngSpecMake(r)
	registerEngSpecTypeWords(r)
	registerEngSpecDefinition(r)
	registerEngSpecStack(r)
}

// registerEngSpecDefinition installs def / fn / quote / args as
// spec-runner fixtures. Production registrations live in
// lang/go/engine/native_definition.go; engspec delegates to the same
// algorithm helpers in eng (registerCoreDef etc. retained in
// eng/go/core_words.go as in-package test infrastructure are not
// reachable from this package — so engspec wires its own NativeFunc
// dispatch through the public eng API).
func registerEngSpecDefinition(r *core.Registry) {
	// def name body — plain and typed forms. The production version in
	// lang handles richer FnDef installation; engspec ships the bare
	// "push body onto def stack after name validation" semantics, which
	// is enough for the eng/spec tsv rows that exercise simple bindings.
	plainDef := func(args []core.Value, _ map[string]core.Value, _ []core.Value, reg *core.Registry) ([]core.Value, error) {
		name, _ := args[0].AsConcreteAtom()
		// `def` is the universal binder (TYPE-UNIFORM Phase 2): a
		// capitalised name is a TYPE binding — delegate to InstallType,
		// the same path the `refine` word uses.
		if core.IsCapitalisedName(name) {
			return nil, core.InstallType(reg, name, args[1])
		}
		if err := core.ValidateWordName(name); err != nil {
			return nil, err
		}
		reg.Check.RecordDef(name, core.SrcPos{})
		if info, ok := args[1].Data.(core.FnDefInfo); ok {
			core.InstallFnDef(reg, name, info)
			return nil, nil
		}
		// Through the LEDGERED installer, never a bare Defs.Push: the twin
		// regime rolls the check pass's installs back before the run and
		// replays them from the recorded transitions, so an install the
		// ledger never saw is simply lost (the standalone lanes went
		// `undefined word: x` the day RunProgram inherited the rollback).
		core.InstallDef(reg, name, args[1])
		return nil, nil
	}
	typedDef := func(args []core.Value, _ map[string]core.Value, _ []core.Value, reg *core.Registry) ([]core.Value, error) {
		nameMap, _ := core.AsMap(args[0])
		if nameMap == nil || nameMap.Len() != 1 {
			return nil, &core.BoruError{Code: "type_error", Detail: "def: typed-name map must have exactly one key"}
		}
		name := nameMap.Keys()[0]
		if err := core.ValidateWordName(name); err != nil {
			return nil, err
		}
		if reg.Defs.IsType(name) {
			return nil, &core.BoruError{Code: "type_error", Detail: "def " + name + ": name clash — already a type"}
		}
		constraint, _ := nameMap.Get(name)
		if resolved, _, _ := reg.ResolveTypedNameValue(constraint); resolved.Data != nil || resolved.Parent != nil {
			constraint = resolved
		}
		body := args[1]
		if !core.IsValueOfType(body, constraint) {
			return nil, &core.BoruError{
				Code:   "type_error",
				Detail: "def " + name + ": value " + body.String() + " does not satisfy declared type " + constraint.String(),
			}
		}
		if info, ok := body.Data.(core.FnDefInfo); ok {
			core.InstallFnDef(reg, name, info)
			return nil, nil
		}
		core.InstallDef(reg, name, body) // ledgered — see plainDef
		return nil, nil
	}
	// `def name word BODY` — the Forth-style splice binder. `word` is a
	// KEYWORD SLOT (quoted, pattern-matched), so def captures it structurally
	// instead of stranding on it under the strict forward-barrier. Mirrors
	// the lang layer's synthesized keyword form (registerDefKeywordForms);
	// the eng fixture spells it directly since it has no lang registry.
	defWord := func(args []core.Value, _ map[string]core.Value, _ []core.Value, reg *core.Registry) ([]core.Value, error) {
		name, _ := args[0].AsConcreteAtom()
		if err := core.ValidateWordName(name); err != nil {
			return nil, err
		}
		reg.Check.RecordDef(name, core.SrcPos{})
		core.InstallDef(reg, name, core.NewSplice(args[2])) // ledgered — see plainDef
		return nil, nil
	}
	// `def name fn [triples]` — the fn KEYWORD SLOT. Captures `fn`
	// structurally and binds the FnDef by name, mirroring the lang keyword
	// form. Without it, `def name fn […]` would strand on `fn` under strict,
	// and paren-wrapping (`def name (fn […])`) is WRONG for a fn with a
	// 0-arg overload — the group auto-invokes the nullary sig instead of
	// yielding the FnDef value.
	defFn := func(args []core.Value, _ map[string]core.Value, _ []core.Value, reg *core.Registry) ([]core.Value, error) {
		name, _ := args[0].AsConcreteAtom()
		if err := core.ValidateWordName(name); err != nil {
			return nil, err
		}
		if !core.IsConcrete(args[2]) { //covergate:allow raw NoEval List slot: the matcher only binds a parsed list literal here, which is always concrete
			return nil, &core.BoruError{Code: "type_error", Detail: "def fn: body must be a concrete list"}
		}
		lst, _ := core.AsList(args[2])
		elems := lst.Slice()
		if len(elems) == 0 || len(elems)%3 != 0 {
			return nil, &core.BoruError{Code: "fn_invalid_spec", Detail: "fn: list length must be a non-zero multiple of 3"}
		}
		fnDef, err := core.ParseFnDef(reg, elems)
		if err != nil {
			return nil, err
		}
		reg.Check.RecordDef(name, core.SrcPos{})
		core.InstallFnDef(reg, name, fnDef)
		return nil, nil
	}
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "def",

		Signatures: []core.Signature{
			{
				Args:       []*core.Type{core.TAtom, core.TAtom, core.TAny},
				QuoteArgs:  map[int]bool{0: true, 1: true},
				Patterns:   map[int]core.Value{1: core.NewAtom("word")},
				NoEvalArgs: map[int]bool{2: true},
				Impl:       core.Go(defWord, core.RunInCheck()),
				Returns:    []*core.Type{}, BarrierPos: -1,
			},
			{
				Args:       []*core.Type{core.TAtom, core.TAtom, core.TList},
				QuoteArgs:  map[int]bool{0: true, 1: true},
				Patterns:   map[int]core.Value{1: core.NewAtom("fn")},
				NoEvalArgs: map[int]bool{2: true},
				Impl:       core.Go(defFn, core.RunInCheck()),
				Returns:    []*core.Type{}, BarrierPos: -1,
			},
			{
				Args:          []*core.Type{core.TMap, core.TAny},
				NoEvalMapArgs: map[int]bool{0: true},
				Impl:          core.Go(typedDef),
				Returns:       []*core.Type{}, BarrierPos: -1,
			},
			{
				Args:      []*core.Type{core.TAtom, core.TAny},
				QuoteArgs: map[int]bool{0: true},
				Impl:      core.Go(plainDef, core.RunInCheck()),
				Returns:   []*core.Type{}, BarrierPos: -1,
			},
		},
	})

	// word VALUE — wrap the (unevaluated) arg in an __SP splice marker (the
	// kernel splice mechanism). `def name word value` binds the marker so a
	// reference splices its payload. Production registration lives in
	// lang/go/native/natives.go; engspec ships its own fixture so eng/spec
	// rows (forth.tsv etc.) can express splices after the def-list flip.
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "word",
		Signatures: []core.Signature{{
			Args:       []*core.Type{core.TAny},
			NoEvalArgs: map[int]bool{0: true},
			Impl: core.Go(func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
				return []core.Value{core.NewSplice(args[0])}, nil
			}),
			Returns: []*core.Type{core.TAny}, BarrierPos: -1,
		}},
	})

	// fn [triples] — uses ParseFnDef from eng to build the FnDef.
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "fn",

		Signatures: []core.Signature{{
			Args:       []*core.Type{core.TList},
			NoEvalArgs: map[int]bool{0: true},
			Impl: core.Go(func(args []core.Value, _ map[string]core.Value, _ []core.Value, reg *core.Registry) ([]core.Value, error) {
				if !core.IsConcrete(args[0]) { //covergate:allow raw NoEval List slot: the matcher only binds a parsed list literal here, which is always concrete
					return nil, &core.BoruError{Code: "type_error", Detail: "fn: argument must be a concrete list"}
				}
				lst, _ := core.AsList(args[0])
				elems := lst.Slice()
				if len(elems) == 0 || len(elems)%3 != 0 {
					return nil, &core.BoruError{Code: "fn_invalid_spec", Detail: "fn: list length must be a non-zero multiple of 3"}
				}
				fnDef, err := core.ParseFnDef(reg, elems)
				if err != nil {
					return nil, err
				}
				return []core.Value{core.NewFunction(fnDef)}, nil
			}, core.RunInCheck()),
			Returns:    []*core.Type{core.TFunction},
			BarrierPos: -1,
		}},
	})

	// quote VALUE — atom-quoted and any-value forms.
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "quote",

		Signatures: []core.Signature{
			{
				Args:      []*core.Type{core.TAtom},
				QuoteArgs: map[int]bool{0: true},
				Impl: core.Go(func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
					return []core.Value{args[0]}, nil
				}),
				Returns: []*core.Type{core.TAtom}, BarrierPos: -1,
			},
			{
				Args:       []*core.Type{core.TAny},
				NoEvalArgs: map[int]bool{0: true},
				Impl: core.Go(func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
					v := args[0]
					if v.Parent.Equal(core.TList) && v.Data != nil {
						v.Quoted = true
					}
					return []core.Value{v}, nil
				}),
				Returns: []*core.Type{core.TAny}, BarrierPos: -1,
			},
		},
	})

	// args — return the current fn-call's args frame as a List.
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "args",
		Signatures: []core.Signature{{
			Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, reg *core.Registry) ([]core.Value, error) {
				v, ok, err := reg.Args.Top()
				if err != nil {
					return nil, err
				}
				if !ok {
					return []core.Value{core.NewList(nil)}, nil
				}
				return []core.Value{v}, nil
			}),
			Returns: []*core.Type{core.TList}, BarrierPos: 0,
		}},
	})

	// __pa — engine-internal args-frame cleanup marker emitted by
	// core.InstallFnDef's expansion. Pops the top args frame from the
	// per-fn-call argsStack. The production version lives at
	// lang/go/engine/native_definition.go::popArgsHandler.
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "__pa",

		Signatures: []core.Signature{{
			Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, reg *core.Registry) ([]core.Value, error) {
				if _, err := reg.Args.Pop(); err != nil {
					return nil, err
				}
				return nil, nil
			}),
			Returns: []*core.Type{}, BarrierPos: -1,
		}},
	})

	// undef NAME — pop the named binding from the def stack. Also
	// emitted by core.InstallFnDef's body expansion to clean up
	// per-call parameter bindings.
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "undef",

		Signatures: []core.Signature{{
			Args:      []*core.Type{core.TAtom},
			QuoteArgs: map[int]bool{0: true},
			Impl: core.Go(func(args []core.Value, _ map[string]core.Value, _ []core.Value, reg *core.Registry) ([]core.Value, error) {
				name, _ := args[0].AsConcreteAtom()
				// Universal unbinder: a capitalised name pops the type
				// binding from the single store, mirroring `def`.
				if core.IsCapitalisedName(name) {
					entry, ok := reg.Defs.PopEntry(name)
					if !ok {
						return nil, &core.BoruError{Code: "type_error", Detail: "undef " + name + ": no such type binding"}
					}
					if entry.TypeDef != nil {
						reg.Types.Retire(entry.TypeDef)
					}
					// Ledgered after the pop, as basic's undef and
					// core.UninstallDef record it (the twin reproduces the
					// post-transition depth).
					reg.NoteBindTransition(core.BindUndef, name, core.SrcPos{})
					return nil, nil
				}
				core.UninstallDef(reg, name)
				return nil, nil
			}),
			Returns: []*core.Type{}, BarrierPos: -1,
		}},
	})
}

// registerEngSpecStack installs the Forth-style stack manipulators —
// dup / swap / drop / over / rot / nip / tuck / dup2 / swap2 / drop2
// / over2 — as spec-runner fixtures. Production registrations live
// in lang/go/engine/native_stack.go.
//
// All ops are stack-only (every sig sets `BarrierPos: 0`) —
// args[0] is the top of stack, args[1] the next-deeper, etc. The
// handler returns the splice-form replacement for the consumed
// args, in source order.
func registerEngSpecStack(r *core.Registry) {
	// The declared shapes mirror the REAL stack words (basic/go
	// native_stack.go): ReturnsIdentity wires each output to the input
	// position it copies, which is what the check/compile model reads —
	// an empty Returns here would compile splice bodies to programs
	// whose operands vanish (the model believed dup produced nothing).
	op := func(name string, argCount int, fn func(args []core.Value) []core.Value, identity ...int) {
		args := make([]*core.Type, argCount)
		for i := range args {
			args[i] = core.TAny
		}
		sig := core.Signature{
			Args: args,
			Impl: core.Go(func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
				return fn(args), nil
			}),
			BarrierPos: 0,
		}
		if len(identity) == 0 {
			sig.Returns = []*core.Type{}
		} else {
			sig.ReturnsFn = core.ReturnsIdentity(identity...)
		}
		r.RegisterNativeFunc(core.NativeFunc{Name: name, Signatures: []core.Signature{sig}})
	}
	op("dup", 1, func(a []core.Value) []core.Value { return []core.Value{a[0], a[0]} }, 0, 0)
	op("drop", 1, func(_ []core.Value) []core.Value { return nil })
	op("swap", 2, func(a []core.Value) []core.Value { return []core.Value{a[0], a[1]} }, 0, 1)
	op("over", 2, func(a []core.Value) []core.Value { return []core.Value{a[1], a[0], a[1]} }, 1, 0, 1)
	op("rot", 3, func(a []core.Value) []core.Value { return []core.Value{a[1], a[0], a[2]} }, 1, 0, 2)
	op("nip", 2, func(a []core.Value) []core.Value { return []core.Value{a[0]} }, 0)
	op("tuck", 2, func(a []core.Value) []core.Value { return []core.Value{a[0], a[1], a[0]} }, 0, 1, 0)
	op("dup2", 2, func(a []core.Value) []core.Value { return []core.Value{a[1], a[0], a[1], a[0]} }, 1, 0, 1, 0)
	op("swap2", 4, func(a []core.Value) []core.Value { return []core.Value{a[1], a[0], a[3], a[2]} }, 1, 0, 3, 2)
	op("drop2", 2, func(_ []core.Value) []core.Value { return nil })
	op("over2", 4, func(a []core.Value) []core.Value { return []core.Value{a[3], a[2], a[1], a[0], a[3], a[2]} }, 3, 2, 1, 0, 3, 2)
}

// registerEngSpecTypeWords installs `type` / `untype` / `typeof` /
// `pathof` / `is` / `enum` as spec-runner fixtures using the eng-
// exported algorithm primitives (InstallType, TypeOf, PathOf,
// IsValueOfType, NewEnum). Production registrations live in
// lang/go/engine/native_type.go.
func registerEngSpecTypeWords(r *core.Registry) {
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "typeof",

		Signatures: []core.Signature{{
			Args: []*core.Type{core.TAny},
			Impl: core.Go(func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
				return []core.Value{core.TypeOf(args[0])}, nil
			}),
			Returns: []*core.Type{core.TType}, BarrierPos: -1,
		}},
	})
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "pathof",

		Signatures: []core.Signature{{
			Args:     []*core.Type{core.TAny},
			TypeArgs: map[int]bool{0: true},
			Impl: core.Go(func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
				return []core.Value{core.PathOf(args[0])}, nil
			}),
			Returns: []*core.Type{core.TList}, BarrierPos: -1,
		}},
	})
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "is",

		Signatures: []core.Signature{{
			Args:       []*core.Type{core.TAny, core.TAny},
			BarrierPos: 1,
			Impl: core.Go(func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
				return []core.Value{core.NewBoolean(core.IsValueOfType(args[1], args[0]))}, nil
			}),
			Returns: []*core.Type{core.TBoolean},
		}},
	})
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "enum",

		Signatures: []core.Signature{{
			Args:       []*core.Type{core.TList},
			NoEvalArgs: map[int]bool{0: true},
			Impl: core.Go(func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
				list := args[0]
				if !core.IsConcrete(list) { //covergate:allow raw NoEval List slot: the matcher only binds a parsed list literal here, which is always concrete
					return nil, &core.BoruError{Code: "type_error", Detail: "enum: argument must be a concrete list"}
				}
				var childType core.Value
				hasChild := false
				if core.IsTypedList(list) {
					ci, _ := core.AsChildType(list)
					childType = ci.Child
					hasChild = childType.Parent != nil
				}
				elems, _ := core.AsList(list)
				alts := make([]core.Value, 0, elems.Len())
				for i := 0; i < elems.Len(); i++ {
					e := elems.Get(i)
					if core.IsWord(e) {
						w, _ := core.AsWord(e)
						e = core.NewAtom(w.Name)
					}
					if hasChild && !core.IsValueOfType(e, childType) {
						return nil, &core.BoruError{
							Code:   "type_error",
							Detail: "enum: element " + e.String() + " does not satisfy child type " + childType.String(),
						}
					}
					alts = append(alts, e)
				}
				return []core.Value{core.NewEnum(alts)}, nil
			}),
			Returns: []*core.Type{core.TEnum}, BarrierPos: -1,
		}},
	})
}

// registerEngSpecMake installs `make` as a spec-runner fixture
// using the eng-exported algorithm handlers. Production
// registration lives in lang/go/engine/native_make.go.
func registerEngSpecMake(r *core.Registry) {
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "make",

		Signatures: []core.Signature{
			{Args: []*core.Type{core.TScalar, core.TMap, core.TAny}, TypeArgs: map[int]bool{0: true}, Impl: core.Go(core.MakeScalarOptsHandler), ReturnsFn: check.ReturnsFreshInstance(0), BarrierPos: -1},
			{Args: []*core.Type{core.TIdeal, core.TMap}, TypeArgs: map[int]bool{0: true}, Impl: core.Go(core.MakeObjHandler), ReturnsFn: check.ReturnsFreshInstance(0), BarrierPos: -1},
			{Args: []*core.Type{core.TScalar, core.TAny}, TypeArgs: map[int]bool{0: true}, Impl: core.Go(core.MakeScalarHandler), ReturnsFn: check.ReturnsFreshInstance(0), BarrierPos: -1},
			{Args: []*core.Type{core.TAny, core.TAny, core.TMap}, Impl: core.Go(core.MakeWithOpts), Returns: []*core.Type{core.TAny}, BarrierPos: -1},
			{Args: []*core.Type{core.TAny, core.TAny}, Impl: core.Go(core.MakeHandler), Returns: []*core.Type{core.TAny}, BarrierPos: -1},
		},
	})
}

// registerEngSpecStorage installs the kernel-container `get` and
// `set` signatures (Node / Object / Array / None) as spec-runner
// fixtures. The production registration in
// lang/go/engine/native_storage.go adds Store-side sigs on top of
// these; engspec mirrors only the kernel slice so eng/spec rows
// that inspect `get` / `set` see the expected shape.
func registerEngSpecStorage(r *core.Registry) {
	setObjectH := func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
		container := args[2]
		// Reachable since the Stage 2 flip: a class NAME in the value
		// slot dispatches as its minted node (`set x 5 P`), and the
		// type-literal guard declines it — mirroring production.
		if !core.IsConcrete(container) {
			return nil, fmt.Errorf("set: cannot set field on type literal")
		}
		key := core.StoreKey(args[0])
		oi, ok := container.Data.(core.ClassInstanceInfo)
		if !ok {
			return nil, fmt.Errorf("set: expected an Object instance, got %s", container.Parent.String())
		}
		oi.Fields.Set(key, args[1])
		return nil, nil
	}
	getNodeH := func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
		key := args[0]
		container := args[1]
		if !core.IsConcrete(container) { //covergate:allow runtime-only handler: dispatch never binds a bare type node into a value slot and runtime values are never carriers; mirrors the production storage word where check mode can reach this
			return nil, fmt.Errorf("get: cannot access property on type literal")
		}
		if key.Parent.ConformsTo(core.TInteger) {
			idx, _ := core.AsInteger(key)
			if list, _ := core.AsList(container); !list.IsNil() && container.Parent.ConformsTo(core.TList) {
				i := int(idx)
				if i < 0 || i >= list.Len() {
					return []core.Value{core.NewTypeLiteral(core.TNone)}, nil
				}
				return []core.Value{list.Get(i)}, nil
			}
		}
		k := core.GetKey(key)
		if m, _ := core.AsMap(container); m != nil {
			val, ok := m.Get(k)
			if !ok {
				return []core.Value{core.NewTypeLiteral(core.TNone)}, nil
			}
			return []core.Value{val}, nil
		}
		return []core.Value{core.NewTypeLiteral(core.TNone)}, nil
	}
	getObjectH := func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
		key := args[0]
		container := args[1]
		if !core.IsConcrete(container) { //covergate:allow runtime-only handler: dispatch never binds a bare type node into a value slot and runtime values are never carriers; mirrors the production storage word where check mode can reach this
			return nil, fmt.Errorf("get: cannot access property on type literal")
		}
		k := core.GetKey(key)
		if m, err := core.AsMutableMap(container); err == nil {
			val, found := m.Get(k)
			if !found {
				return []core.Value{core.NewTypeLiteral(core.TNone)}, nil
			}
			return []core.Value{val}, nil
		}
		oi, _ := core.AsClassInstance(container)
		val, ok := oi.GetField(k)
		if !ok {
			return []core.Value{core.NewTypeLiteral(core.TNone)}, nil
		}
		return []core.Value{val}, nil
	}
	getNoneH := func(_ []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
		return []core.Value{core.NewTypeLiteral(core.TNone)}, nil
	}
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "set",

		Signatures: []core.Signature{
			{Args: []*core.Type{core.TString, core.TAny, core.TClass}, Impl: core.Go(setObjectH), Returns: []*core.Type{}, BarrierPos: -1},
			{Args: []*core.Type{core.TAtom, core.TAny, core.TClass}, QuoteArgs: map[int]bool{0: true}, Impl: core.Go(setObjectH), Returns: []*core.Type{}, BarrierPos: -1},
		},
	})
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "get",

		Signatures: []core.Signature{
			{Args: []*core.Type{core.TAtom, core.TNode}, QuoteArgs: map[int]bool{0: true}, BarrierPos: 1, Impl: core.Go(getNodeH), Returns: []*core.Type{core.TAny}},
			{Args: []*core.Type{core.TString, core.TNode}, BarrierPos: 1, Impl: core.Go(getNodeH), Returns: []*core.Type{core.TAny}},
			{Args: []*core.Type{core.TInteger, core.TNode}, BarrierPos: 1, Impl: core.Go(getNodeH), Returns: []*core.Type{core.TAny}},
			{Args: []*core.Type{core.TAtom, core.TClass}, QuoteArgs: map[int]bool{0: true}, BarrierPos: 1, Impl: core.Go(getObjectH), Returns: []*core.Type{core.TAny}},
			{Args: []*core.Type{core.TString, core.TClass}, BarrierPos: 1, Impl: core.Go(getObjectH), Returns: []*core.Type{core.TAny}},
			{Args: []*core.Type{core.TInteger, core.TClass}, BarrierPos: 1, Impl: core.Go(getObjectH), Returns: []*core.Type{core.TAny}},
			{Args: []*core.Type{core.TAny, core.TNone}, BarrierPos: 1, Impl: core.Go(getNoneH), Returns: []*core.Type{core.TNone}},
		},
	})
}

// registerEngSpecObjectRecord installs `record` and `object` as
// spec-runner fixtures so the eng/spec/inspect.tsv rows around
// record / object inspection can run against the kernel. Production
// registrations live in lang/go/engine/native_object_record.go.
func registerEngSpecObjectRecord(r *core.Registry) {
	recordH := func(args []core.Value, _ map[string]core.Value, _ []core.Value, r *core.Registry) ([]core.Value, error) {
		list := args[0]
		if !list.Parent.Equal(core.TList) { //covergate:allow sole caller refineCtorH pre-checks Parent==List; kept to mirror the production record word, which is dispatched directly
			return nil, fmt.Errorf("record: argument must be a list")
		}
		if !core.IsConcrete(list) {
			return nil, fmt.Errorf("record: argument must be a concrete list, got type literal")
		}
		elems, _ := core.AsList(list)
		if elems.Len() == 0 {
			return nil, fmt.Errorf("record: list must have at least one field")
		}
		fields := core.NewOrderedMap()
		for _, elem := range elems.Slice() {
			if !elem.Parent.Equal(core.TMap) {
				return nil, fmt.Errorf("record: each element must be a pair (map), got %s", elem.String())
			}
			m, err := core.AsMutableMap(elem)
			if err != nil {
				return nil, fmt.Errorf("record: each element must be a concrete pair, got %s", elem.String())
			}
			for _, key := range m.Keys() {
				val, _ := m.Get(key)
				val = core.ResolveFieldType(r, val)
				fields.Set(key, val)
			}
		}
		return []core.Value{core.NewRecordType(fields)}, nil
	}
	parseObjectFields := func(fieldsMap *core.OrderedMap, r *core.Registry) *core.OrderedMap {
		fields := core.NewOrderedMap()
		for _, key := range fieldsMap.Keys() {
			val, _ := fieldsMap.Get(key)
			val = core.ResolveFieldType(r, val)
			fields.Set(key, val)
		}
		return fields
	}
	objectWithParentH := func(args []core.Value, _ map[string]core.Value, _ []core.Value, r *core.Registry) ([]core.Value, error) {
		fieldsVal := args[0]
		parentVal := args[1]
		if !fieldsVal.Parent.Equal(core.TMap) {
			return nil, fmt.Errorf("object: first argument must be a map of field definitions, got %s", fieldsVal.String())
		}
		m, err := core.AsMutableMap(fieldsVal)
		if err != nil {
			return nil, fmt.Errorf("object: first argument must be a concrete map, got %s", fieldsVal.String())
		}
		if !core.IsClassType(parentVal) { //covergate:allow sole caller refineCtorH pre-checks IsClassType(base); kept to mirror the production object word, which is dispatched directly
			return nil, fmt.Errorf("object: parent must be an object type, got %s", parentVal.String())
		}
		parentInfo, _ := core.AsClassType(parentVal)
		fields := parseObjectFields(m, r)
		parentAllFields := parentInfo.AllFields()
		for _, key := range fields.Keys() {
			childConstraint, _ := fields.Get(key)
			parentConstraint, exists := parentAllFields.Get(key)
			if !exists {
				continue
			}
			if _, ok := core.Unify(parentConstraint, childConstraint); !ok {
				return nil, fmt.Errorf("object: field %q in child type cannot expand parent type %s (child: %s, parent: %s)",
					key, parentInfo.Name, childConstraint.String(), parentConstraint.String())
			}
		}
		id := core.GenerateObjectTypeID()
		info := core.ClassTypeInfo{Fields: fields, Parent: &parentInfo, ID: id, Name: ""}
		parentDef := parentInfo.Type
		if parentDef == nil {
			parentDef = core.TClass
		}
		def := r.Types.MintType(id, parentDef)
		return []core.Value{core.NewClassType(def, info)}, nil
	}
	// refine — the uniform type constructor.
	// Mirrors lang/go/native/native_type.go::refineHandler; dispatches
	// to the record/object handler functions above. eng/spec exercises
	// only the Object and Record bases.
	refineCtorH := func(args []core.Value, _ map[string]core.Value, _ []core.Value, reg *core.Registry) ([]core.Value, error) {
		base := args[0]
		arg := args[1]
		// A NAMED base/argument evaluates to its minted node (the Stage 2
		// flip); the kind dispatch operates on the declared structural
		// content the node records — mirroring refinePlain.
		if body, ok := core.TypeContentOf(base); ok && core.IsBareTypeNode(base) {
			base = body
		}
		if body, ok := core.TypeContentOf(arg); ok && core.IsBareTypeNode(arg) {
			arg = body
		}
		if core.IsClassType(base) {
			return objectWithParentH([]core.Value{arg, base}, nil, nil, reg)
		}
		if core.IsBareTypeNode(base) && base.Equal(core.TRecord) {
			if !arg.Parent.Equal(core.TList) {
				return nil, fmt.Errorf("refine Record: a record takes a list of field pairs")
			}
			return recordH([]core.Value{arg}, nil, nil, reg)
		}
		return nil, fmt.Errorf("refine: base must be Record or an object type, got %s", base.String())
	}
	// refine BaseType — the 1-arg bare-subtype form. Returns the base
	// type unchanged; the paired `def` mints a fresh subtype of it.
	refineBareCtorH := func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
		base := args[0]
		if !core.IsTypeBody(base) {
			return nil, fmt.Errorf("refine: argument must be a type, got %s", base.String())
		}
		return []core.Value{base}, nil
	}
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "refine",

		Signatures: []core.Signature{
			{
				Args:       []*core.Type{core.TAny, core.TNode},
				Impl:       core.Go(refineCtorH, core.RunInCheck()),
				Returns:    []*core.Type{core.TType},
				BarrierPos: -1,
			},
			{
				Args:       []*core.Type{core.TAny},
				Impl:       core.Go(refineBareCtorH, core.RunInCheck()),
				Returns:    []*core.Type{core.TType},
				BarrierPos: -1,
			},
		},
	})
}

// registerEngSpecInspect installs `inspect` as a spec-runner fixture
// that mirrors lang/go/engine/native_inspect.go. The production
// registration lives in lang; engspec keeps a copy so the
// eng/spec/inspect.tsv rows continue to exercise the kernel-only
// dispatch and value-introspection paths.
func registerEngSpecInspect(r *core.Registry) {
	atomH := func(args []core.Value, _ map[string]core.Value, _ []core.Value, r *core.Registry) ([]core.Value, error) {
		name, _ := args[0].AsConcreteAtom()
		if tv, ok := r.TopTypeBody(name); ok {
			return []core.Value{buildTypeInspection(name, tv)}, nil
		}
		if top, ok := r.Defs.Top(name); ok {
			if core.IsTypeBody(top) && !top.Parent.Equal(core.TFunction) && !top.Parent.Equal(core.TFunction) {
				return []core.Value{buildTypeInspection(name, top)}, nil
			}
		}
		// Builtin arm of the canonical cascade (ADR-012 rule 4) —
		// mirrors lang's inspectAtomHandler: post-opacity every type
		// name reaches this handler as a captured Atom.
		if t, ok := core.ResolveBuiltinTypeName(name); ok {
			return []core.Value{buildTypeInspection("", core.NewTypeLiteral(t))}, nil
		}
		return []core.Value{buildInspection(r, name)}, nil
	}
	typeH := func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
		return []core.Value{buildTypeInspection("", args[0])}, nil
	}
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "inspect",

		Signatures: []core.Signature{
			{Args: []*core.Type{core.TAtom}, QuoteArgs: map[int]bool{0: true}, Impl: core.Go(atomH), Returns: []*core.Type{core.TInspect}, BarrierPos: -1},
			{Args: []*core.Type{core.TAny}, Impl: core.Go(typeH), Returns: []*core.Type{core.TInspect}, BarrierPos: -1},
		},
	})
}

// buildInspection constructs a word-inspection map for the named word.
func buildInspection(r *core.Registry, name string) core.Value {
	result := core.NewOrderedMap()
	result.Set("name", core.NewString(name))

	fn := r.Lookup(name)
	if fn == nil {
		if r.Defs.Has(name) {
			result.Set("kind", core.NewAtom("defined"))
			if v, ok := r.Defs.Top(name); ok {
				result.Set("value", v)
			}
			result.Set("signatures", core.NewList(nil))
			return core.NewValueRaw(core.TInspect, core.MapPayload{M: result})
		}
		result.Set("kind", core.NewAtom("unknown"))
		result.Set("signatures", core.NewList(nil))
		return core.NewValueRaw(core.TInspect, core.MapPayload{M: result})
	}

	isDefined := false
	for _, s := range fn.OwnSigs() {
		if len(s.Body()) > 0 {
			isDefined = true
			break
		}
	}
	if isDefined {
		result.Set("kind", core.NewAtom("defined"))
	} else {
		result.Set("kind", core.NewAtom("native"))
	}

	var sigMaps []core.Value
	for _, sig := range fn.Signatures {
		sm := core.NewOrderedMap()
		var argVals []core.Value
		for _, argType := range sig.Args {
			argVals = append(argVals, core.NewString(argType.Leaf()))
		}
		if argVals == nil {
			argVals = []core.Value{}
		}
		sm.Set("args", core.NewList(argVals))
		sigMaps = append(sigMaps, core.NewMap(sm))
	}
	if sigMaps == nil {
		sigMaps = []core.Value{}
	}
	result.Set("signatures", core.NewList(sigMaps))

	return core.NewValueRaw(core.TInspect, core.MapPayload{M: result})
}

// buildTypeInspection constructs a type-inspection map for a type value.
func buildTypeInspection(name string, tv core.Value) core.Value {
	result := core.NewOrderedMap()

	if name != "" {
		result.Set("name", core.NewString(name))
	}

	if core.IsBareTypeNode(tv) || core.IsTypeBody(tv) || core.IsRecordShape(tv) {
		result.Set("type", core.NewString("Type"))
		result.Set("struct", core.NewString(core.TypeNameOf(tv)))
	} else {
		result.Set("type", core.NewString(tv.Parent.Leaf()))
	}

	switch {
	case core.IsRecordType(tv):
		result.Set("kind", core.NewAtom("record"))
		rt, _ := core.AsRecordType(tv)
		fields := core.NewOrderedMap()
		for _, k := range rt.Fields.Keys() {
			v, _ := rt.Fields.Get(k)
			fields.Set(k, core.NewString(core.TypeNameOf(v)))
		}
		result.Set("fields", core.NewMap(fields))

	case core.IsRecordShape(tv):
		result.Set("kind", core.NewAtom("record"))
		m, _ := core.AsMap(tv)
		fields := core.NewOrderedMap()
		for _, k := range m.Keys() {
			v, _ := m.Get(k)
			fields.Set(k, core.NewString(core.TypeNameOf(v)))
		}
		result.Set("fields", core.NewMap(fields))

	case core.IsClassType(tv):
		result.Set("kind", core.NewAtom("object"))
		oi, _ := core.AsClassType(tv)
		if oi.Parent != nil {
			result.Set("parent", core.NewString(oi.Parent.Name))
		}
		af := oi.AllFields()
		fields := core.NewOrderedMap()
		for _, k := range af.Keys() {
			v, _ := af.Get(k)
			fields.Set(k, core.NewString(core.TypeNameOf(v)))
		}
		result.Set("fields", core.NewMap(fields))

	case core.IsTableType(tv):
		result.Set("kind", core.NewAtom("table"))
		tt, _ := core.AsTableType(tv)
		fields := core.NewOrderedMap()
		for _, k := range tt.Record.Fields.Keys() {
			v, _ := tt.Record.Fields.Get(k)
			fields.Set(k, core.NewString(core.TypeNameOf(v)))
		}
		result.Set("fields", core.NewMap(fields))

	case core.IsDisjunct(tv):
		result.Set("kind", core.NewAtom("disjunct"))
		di, _ := core.AsDisjunct(tv)
		alts := make([]core.Value, len(di.Alternatives))
		for i, alt := range di.Alternatives {
			alts[i] = core.NewString(core.TypePathOf(alt))
		}
		result.Set("alternatives", core.NewList(alts))

	case core.IsTypedList(tv):
		result.Set("kind", core.NewAtom("typed_list"))
		ci, _ := core.AsChildType(tv)
		result.Set("child", core.NewString(core.TypePathOf(ci.Child)))

	case core.IsTypedMap(tv):
		result.Set("kind", core.NewAtom("typed_map"))
		ci, _ := core.AsChildType(tv)
		result.Set("child", core.NewString(core.TypePathOf(ci.Child)))

	case core.ValueType(tv).Equal(core.TFnUndef):
		result.Set("kind", core.NewAtom("function_shape"))
		uInfo, _ := tv.Data.(core.FnUndefInfo)
		sigs := make([]core.Value, 0, len(uInfo.Sigs))
		for _, spec := range uInfo.Sigs {
			sig := core.NewOrderedMap()
			params := make([]core.Value, len(spec.Params))
			for i, p := range spec.Params {
				params[i] = core.NewString(p.Type.Leaf())
			}
			sig.Set("params", core.NewList(params))
			rets := make([]core.Value, len(spec.Returns))
			for i, ret := range spec.Returns {
				rets[i] = core.NewString(ret.Leaf())
			}
			sig.Set("returns", core.NewList(rets))
			sigs = append(sigs, core.NewMap(sig))
		}
		result.Set("signatures", core.NewList(sigs))

	case tv.IsDepScalar():
		result.Set("kind", core.NewAtom("dependent_scalar"))
		info, _ := tv.AsDepScalar()
		result.Set("leaf", core.NewString(tv.Parent.Name()))
		if info.Lo != nil {
			lo := core.NewOrderedMap()
			lo.Set("kind", core.NewString(core.BoundToKind(info.Lo, true).String()))
			lo.Set("value", info.Lo.Value)
			result.Set("lo", core.NewMap(lo))
		}
		if info.Hi != nil {
			hi := core.NewOrderedMap()
			hi.Set("kind", core.NewString(core.BoundToKind(info.Hi, false).String()))
			hi.Set("value", info.Hi.Value)
			result.Set("hi", core.NewMap(hi))
		}

	default:
		result.Set("kind", core.NewAtom("literal"))
	}

	return core.NewValueRaw(core.TInspect, core.MapPayload{M: result})
}

// registerEngSpecFnSig installs `fnsig` as a spec-runner fixture so
// the eng/spec/types.tsv rows around FnSig type-shape matching can
// run against the kernel alone. The production fnsig registration
// lives in lang/go/engine/native_definition.go.
func registerEngSpecFnSig(r *core.Registry) {
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "fnsig",

		Signatures: []core.Signature{{
			Args:       []*core.Type{core.TList},
			NoEvalArgs: map[int]bool{0: true},
			Impl: core.Go(func(args []core.Value, _ map[string]core.Value, _ []core.Value, reg *core.Registry) ([]core.Value, error) {
				if !core.IsConcrete(args[0]) { //covergate:allow raw NoEval List slot: the matcher only binds a parsed list literal here, which is always concrete
					return nil, &core.BoruError{
						Code:   "fnsig_invalid_spec",
						Detail: "fnsig: argument must be a concrete list",
					}
				}
				lst, _ := core.AsList(args[0])
				spec := lst.Slice()
				if len(spec) == 0 || len(spec)%2 != 0 {
					return nil, &core.BoruError{
						Code:   "fnsig_invalid_spec",
						Detail: "fnsig: list length must be a non-zero multiple of 2 (input output pairs); use `fn` for the with-body form",
					}
				}
				info, err := core.ParseFnUndefSpec(reg, spec)
				if err != nil {
					return nil, err
				}
				return []core.Value{core.NewFnUndef(info)}, nil
			}),
			Returns: []*core.Type{core.TFnUndef}, BarrierPos: -1,
		}},
	})
}

// registerEngSpecBoolean installs not/and/or as spec-runner fixtures
// using core.CoerceBoolean. The production words with the same names
// live in lang/go/engine/native_boolean.go.
func registerEngSpecBoolean(r *core.Registry) {
	notH := func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
		return []core.Value{core.NewBoolean(!core.CoerceBoolean(args[0]))}, nil
	}
	andH := func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
		if !core.CoerceBoolean(args[1]) {
			return []core.Value{args[1]}, nil
		}
		return []core.Value{args[0]}, nil
	}
	orH := func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
		if core.CoerceBoolean(args[1]) {
			return []core.Value{args[1]}, nil
		}
		return []core.Value{args[0]}, nil
	}
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "not",

		Signatures: []core.Signature{
			{Args: []*core.Type{core.TBoolean}, Impl: core.Go(notH), Returns: []*core.Type{core.TBoolean}, BarrierPos: -1},
			{Args: []*core.Type{core.TAny}, Impl: core.Go(notH), Returns: []*core.Type{core.TBoolean}, BarrierPos: -1},
		},
	})
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "and",

		Signatures: []core.Signature{
			{Args: []*core.Type{core.TBoolean, core.TBoolean}, Impl: core.Go(andH), Returns: []*core.Type{core.TBoolean}, BarrierPos: -1},
			{Args: []*core.Type{core.TAny, core.TAny}, Impl: core.Go(andH), Returns: []*core.Type{core.TAny}, BarrierPos: -1},
		},
	})
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "or",

		Signatures: []core.Signature{
			{Args: []*core.Type{core.TBoolean, core.TBoolean}, BarrierPos: 1, Impl: core.Go(orH), Returns: []*core.Type{core.TBoolean}},
			{Args: []*core.Type{core.TAny, core.TAny}, BarrierPos: 1, Impl: core.Go(orH), Returns: []*core.Type{core.TAny}},
		},
	})
}

// registerEngSpecDo installs the `do` word as a spec-runner fixture.
// The production registration lives in
// lang/go/engine/native_control.go; engspec ships a minimal version
// that runs a list body or evaluates embedded lists in a map literal
// against a sub-engine — enough surface for the eng/spec/do.tsv rows
// to exercise the kernel's sub-engine semantics.
func registerEngSpecDo(r *core.Registry) {
	listH := func(args []core.Value, _ map[string]core.Value, _ []core.Value, r *core.Registry) ([]core.Value, error) {
		if !core.IsConcrete(args[0]) { //covergate:allow raw NoEval List slot: the matcher only binds a parsed list literal here, which is always concrete
			return nil, &core.BoruError{
				Code:   "type_error",
				Detail: "do: argument must be a concrete list, got type literal",
			}
		}
		lst, _ := core.AsList(args[0])
		sub := core.New(r)
		input := append([]core.Value{}, lst.Slice()...)
		result, err := sub.Run(input)
		if err != nil {
			return []core.Value{core.NewError(err)}, nil
		}
		return result, nil
	}
	mapH := func(args []core.Value, _ map[string]core.Value, _ []core.Value, r *core.Registry) ([]core.Value, error) {
		result, err := doEvalMapValue(r, args[0])
		if err != nil { //covergate:allow dispatch auto-evaluates plain-map args (execMatch's autoEvalMap) and propagates their errors before the handler runs, so the walk here re-visits already-evaluated data; the arms are unit-tested on doEvalMapValue directly
			return nil, err
		}
		return []core.Value{result}, nil
	}
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "do",

		Signatures: []core.Signature{
			{Args: []*core.Type{core.TList}, NoEvalArgs: map[int]bool{0: true}, Impl: core.Go(listH), Returns: []*core.Type{core.TAny}, BarrierPos: -1},
			{Args: []*core.Type{core.TMap}, Impl: core.Go(mapH), Returns: []*core.Type{core.TAny}, BarrierPos: -1},
		},
	})
}

// doEvalMapValue recursively evaluates list values within a map.
// Records, options, table types, typed lists / maps are left
// untouched — only plain concrete lists and maps are walked.
func doEvalMapValue(r *core.Registry, v core.Value) (core.Value, error) {
	if v.Parent.Equal(core.TList) && v.Data != nil && !core.IsTypedList(v) && !core.IsTableType(v) {
		lst, _ := core.AsList(v)
		sub := core.New(r)
		input := make([]core.Value, lst.Len())
		copy(input, lst.Slice())
		results, err := sub.Run(input)
		if err != nil {
			return core.Value{}, err
		}
		if len(results) == 1 {
			return results[0], nil
		}
		return core.NewList(results), nil
	}
	if v.Parent.Equal(core.TMap) && v.Data != nil && !core.IsTypedMap(v) && !core.IsRecordType(v) && !core.IsOptionsType(v) {
		// AsMap succeeds for every payload the predicate above admits
		// (MapPayload / weak-flex; typed maps are excluded) — same
		// unguarded shape as the production DoEvalMapValue in basic.
		m, _ := core.AsMap(v)
		out := core.NewOrderedMap()
		for _, key := range m.Keys() {
			val, _ := m.Get(key)
			evaluated, err := doEvalMapValue(r, val)
			if err != nil {
				return core.Value{}, err
			}
			out.Set(key, evaluated)
		}
		return core.NewMap(out), nil
	}
	return v, nil
}

// registerEngSpecTypeOps installs tor/tand as spec-runner fixtures
// using the eng-exported algorithm handlers. Production registrations
// live in lang/go/engine/native_type.go.
func registerEngSpecTypeOps(r *core.Registry) {
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "tor",

		Signatures: []core.Signature{{
			Args:       []*core.Type{core.TAny, core.TAny},
			BarrierPos: 1,
			Impl:       core.Go(core.TorHandler),
			ReturnsFn:  core.TorReturnsFn,
		}},
	})
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "tand",

		Signatures: []core.Signature{{
			Args:       []*core.Type{core.TAny, core.TAny},
			BarrierPos: 1,
			Impl:       core.Go(core.TandHandler),
			Returns:    []*core.Type{core.TAny},
		}},
	})
}

// TestSpec runs boru/eng/spec/*.tsv against the engine kernel — a fresh
// core.Registry populated by registerSpecWords, which installs only
// the spec-runner fixtures eng/spec needs (q-suffixed probes plus
// minimal in-test copies of the dispatching words). The shared TSV
// scaffolding (file walk, row parsing, ERROR handling, value
// rendering) lives in test/go/specfix.

// RegisterControlWords installs the fixture `if` / `for` control
// words (control.go). Registered separately from RegisterSpecWords:
// the shared eng/spec corpus has no control rows yet (the TS fixture
// twin does not exist), so only the eng-local battery lanes opt in.
func RegisterControlWords(r *core.Registry) {
	registerEngSpecControl(r)
	registerEngSpecCallable(r)
	registerEngSpecBaking(r)
	registerEngSpecHigherOrder(r)
}
