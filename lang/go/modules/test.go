package modules

import (
	"fmt"
	"strings"
	"sync"
	"time"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"

	"github.com/boru-lang/boru/lang/go/modules/test/shrink"
	"github.com/boru-lang/boru/lang/go/native"
	"github.com/boru-lang/boru/lang/go/stackform"
)

// The pure assertion handlers are NAMED so the sig can wire the same
// function as both the runtime Impl and the check-mode DryPassReturns
// mirror (a provably-failing top-level assertion flags statically).
func assertEqualHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	// args[0] is the forward / first arg (expected), args[1] is the
	// second (actual). Print order: "expected X, got Y".
	expected, actual := args[0], args[1]
	if !native.ValuesEqual(expected, actual) {
		return nil, r.BoruError("assertion_failure",
			fmt.Sprintf("Assert.equal: expected %s, got %s",
				native.FormatForPrint(expected),
				native.FormatForPrint(actual)),
			"Assert.equal")
	}
	return nil, nil
}

func assertNotEqualHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	if native.ValuesEqual(args[0], args[1]) {
		return nil, r.BoruError("assertion_failure",
			fmt.Sprintf("Assert.not-equal: both sides equal %s",
				native.FormatForPrint(args[0])),
			"Assert.not-equal")
	}
	return nil, nil
}

func assertOkHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	if !isTruthy(args[0]) {
		return nil, r.BoruError("assertion_failure",
			fmt.Sprintf("Assert.ok: value is falsy: %s", native.FormatForPrint(args[0])),
			"Assert.ok")
	}
	return nil, nil
}

func assertMatchHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	sub, _ := args[0].AsConcreteString()
	s, _ := args[1].AsConcreteString()
	if !strings.Contains(s, sub) {
		return nil, r.BoruError("assertion_failure",
			fmt.Sprintf("Assert.match: %q does not contain %q", s, sub),
			"Assert.match")
	}
	return nil, nil
}

// testRun is the per-registry state that test/describe/it accumulate
// into. It is created lazily on first call to a test word and stored
// under capTestRun on the parent (caller's) registry — so successive
// test calls from the same Run() append to the same set, and
// `Test.results` returns everything seen so far.
type testRun struct {
	mu       sync.Mutex
	path     []string       // active describe stack
	results  []native.Value // TestResult records, accumulated in order
	failures int            // count of failed test cases
}

const capTestRun = "Test.run.active"

// testParseOnce caches the parsed boru preamble that defines the
// TestCase / TestSet / TestSpec / TestResult record types plus the
// pure-Boru spec runner. The preamble is parsed once per process and
// reused for every BuildTestModule call.
var (
	testParseOnce sync.Once
	testParsed    []native.Value
	testParseErr  error
)

// BuildTestModule creates the "boru:test" native module. The module
// is intentionally hybrid:
//
//   - The imperative API (test / describe / it / assert.*) is
//     implemented in Go because it needs to manage the active
//     testRun, catch errors, and time execution.
//   - The declarative pieces (TestCase, TestSet, TestSpec, TestResult
//     record types, plus run-spec) live in boru because they are pure
//     data construction and benefit from reading like a schema.
//
// Both are folded into the `test` and `assert` exports so callers
// get one import and two dotted namespaces.
func BuildTestModule(parent *native.Registry) (native.ModuleDesc, error) {
	if parent.ParseFunc == nil {
		return native.ModuleDesc{}, fmt.Errorf("test: parser not configured")
	}

	// The boru:test preamble parse is routed through a seam
	// (design/TEST-SEAMS.10.md) so its parse/run error arms are drivable.
	parsed, perr := w9ParsePreamble(parent)
	if perr != nil {
		return native.ModuleDesc{}, fmt.Errorf("test: parse preamble: %w", perr)
	}

	// Build the module sub-registry, register the Go-implemented test
	// natives into it, then run the boru preamble so record types and
	// spec-runner fns are defined alongside them. The preamble's
	// `export` call assembles the final export map.
	modReg, err := newDefaultRegistry()
	if err != nil {
		return native.ModuleDesc{}, fmt.Errorf("test: init: %w", err)
	}
	modReg.Output = parent.Output
	modReg.ErrOutput = parent.ErrOutput
	modReg.Input = parent.Input
	// Ledger rides with the writers so this sub-registry's effects count
	// against the parent's compiled-mode fallback fence (eng effects.go);
	// the observability hooks ride along for the same reason.
	modReg.Effects = parent.Effects
	modReg.InheritObserveHooks(parent)
	modReg.ParseFunc = parent.ParseFunc
	modReg.BaseDir = parent.BaseDir

	// Inherit the module CONFIG (InitFunc + Resolver) as a unit so the
	// test preamble can itself `import "boru:<name>"` — the same
	// field-by-field copy that dropped the Resolver in RunModuleBody was
	// latent here too. Then seed native words via InitFunc, falling back
	// to a direct Register when the host installed no InitFunc.
	modReg.Modules.InheritConfig(parent.Modules)
	if modReg.Modules.InitFunc != nil {
		modReg.Modules.InitFunc(modReg)
	} else {
		native.Register(modReg)
	}

	for _, n := range testNatives(parent) {
		modReg.RegisterNativeFunc(n)
	}
	for _, n := range coverNatives(parent) {
		modReg.RegisterNativeFunc(n)
	}

	// Run the preamble. We reuse RunModuleBody's machinery via a
	// minimal local exporter (we cannot use RunModuleBody itself —
	// it builds a fresh modReg that doesn't see our natives).
	exports := map[string]*native.OrderedMap{}
	// Drop the inherited top-level no-op `export` (registered in the
	// default registry — see §8.3 in native_misc.go) before installing
	// this module's collecting handler. RegisterNativeFunc APPENDS sigs to
	// an existing word, so without this the no-op's (Atom|String, Map) sigs
	// would shadow the collector and the preamble's exports — including the
	// whole `test` namespace — would be silently discarded.
	modReg.Defs.Delete("export")
	modReg.RegisterNativeFunc(native.NativeFunc{
		Name: "export",
		Signatures: []native.Signature{
			{
				Args: []*native.Type{native.TAtom, native.TMap},
				Impl: native.Go(func(eargs []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					name, _ := eargs[0].AsConcreteAtom()
					return resolveExport(modReg, exports, name, eargs[1])
				}),
				Returns: []*native.Type{}, BarrierPos: -1,
			},
			{
				Args: []*native.Type{native.TString, native.TMap},
				Impl: native.Go(func(eargs []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					name, _ := eargs[0].AsConcreteString()
					return resolveExport(modReg, exports, name, eargs[1])
				}),
				Returns: []*native.Type{}, BarrierPos: -1,
			},
		},
	})

	tokens := append([]native.Value(nil), parsed...)
	sub := native.New(modReg)
	// C4 attribution (plan Phase 10): the preamble run IS the module load —
	// a sanctioned interpreter entry reported under its named seam, so a
	// compiled program's `import "boru:test"` adds zero UNattributed entries.
	restoreAtt := modReg.SetInterpAttribution("module-load")
	_, runErr := sub.Run(tokens)
	restoreAtt()
	if runErr != nil {
		return native.ModuleDesc{}, fmt.Errorf("test: run preamble: %w", runErr)
	}

	return native.ModuleDesc{
		// Src lets import transport escaped type machinery — the
		// preamble-minted record/table type NODES the exports now carry
		// (TestSet, TestCase, …) are adopted into the importing
		// registry's TypeTable so the compiled OpPushType path resolves
		// them (adoptEscapedTypes).
		Src:     modReg,
		ID:      parent.Modules.NextID(),
		Exports: exports,
	}, nil
}

// resolveExport collects exports from an `export "name" {...}` call,
// resolving each value through the module registry so word references
// pick up the actual bound type / fn / Go-native FnDef. This mirrors
// what RunModuleBody does internally but is duplicated here because
// BuildTestModule manages its own modReg.
func resolveExport(modReg *native.Registry, exports map[string]*native.OrderedMap, name string, mapArg native.Value) ([]native.Value, error) {
	if !native.IsConcrete(mapArg) {
		return nil, fmt.Errorf("test/export: value must be a concrete map")
	}
	rawMap, _ := native.AsMap(mapArg)
	resolved := native.NewOrderedMap()
	for _, key := range rawMap.Keys() {
		val, _ := rawMap.Get(key)
		resolved.Set(key, resolveTestExport(modReg, val))
	}
	exports[name] = resolved
	return nil, nil
}

// resolveTestExport mirrors native.resolveModuleExport but is local
// to this package — the kernel helper is unexported.
func resolveTestExport(modReg *native.Registry, v native.Value) native.Value {
	// A function value (from `name/v`) must carry the module registry
	// so it executes in module scope when called after import.
	if fnDef, ok := v.Data.(native.FnDefInfo); ok {
		if fnDef.Registry == nil {
			fnDef.Registry = modReg
		}
		if fnDef.Registry == modReg {
			return native.NewFunction(fnDef)
		}
		return v //covergate:allow boru:test's preamble imports nothing, so no export value can carry a foreign home; the arm mirrors resolveModuleExport's re-export pass-through for the day it does (§modules)
	}
	var name string
	switch {
	case native.IsWord(v):
		w, _ := native.AsWord(v)
		name = w.Name
	case v.Parent.ConformsTo(native.TString):
		name, _ = native.AsString(v)
	case native.IsAtom(v):
		name, _ = native.AsAtom(v)
	default:
		return v
	}
	if tv, ok := modReg.TopTypeBody(name); ok {
		if fnDef, ok := tv.Data.(native.FnDefInfo); ok {
			if fnDef.Registry == nil {
				fnDef.Registry = modReg
			}
			if fnDef.Registry == modReg {
				return native.NewFunction(fnDef)
			}
		}
		return tv
	}
	if val, ok := modReg.Defs.Top(name); ok {
		if fnDef, ok := val.Data.(native.FnDefInfo); ok {
			if fnDef.Registry == nil {
				fnDef.Registry = modReg
			}
			if fnDef.Registry == modReg {
				return native.NewFunction(fnDef)
			}
		}
		return val
	}
	return v
}

// activeRun returns the testRun associated with the parent registry,
// lazily creating it on first access.
func activeRun(parent *native.Registry) *testRun {
	if run, ok, _ := core.Cap[*testRun](parent, capTestRun); ok && run != nil {
		return run
	}
	run := &testRun{}
	_ = parent.Capabilities.Set(capTestRun, run)
	return run
}

// testNatives builds the Go-implemented imperative test API. Words
// are registered into the module sub-registry; their handlers reach
// the active testRun via the captured parent registry.
func testNatives(parent *native.Registry) []native.NativeFunc {
	return []native.NativeFunc{
		// describe "name" [body] — push name onto the path, run body,
		// pop. Body errors abort the describe but leave already-
		// recorded results in place.
		{
			Name: "test-describe",
			// Test.describe "g" [body] — a SIDE-EFFECT grouping body (BodyOut 0)
			// whose nested Test.test cases each compile to their own closure. The
			// word DECLARES its closure shape here (module-side), so eng names no
			// boru:test word — review §4.5.
			Callable: &native.CallableSpec{BodyPos: 1, BodyOut: 0, Inputs: func(_ []native.Value) []native.Value {
				return []native.Value{}
			}},
			Signatures: []native.Signature{{
				Args:       []*native.Type{native.TString, native.TList},
				NoEvalArgs: map[int]bool{1: true},
				Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
					name, _ := args[0].AsConcreteString()
					run := activeRun(parent)
					// Run the grouping body with the group name pushed on the path.
					// Compiled path: the body arrived as a closure (nested Test.test
					// cases each compiled to their own closure); run it via the VM
					// seam. Interpreter path: sub-engine over the token list.
					runBody := func() error {
						if native.IsCompiledClosure(args[1]) {
							_, e := native.InvokeBody(r, args[1], nil)
							return e
						}
						body, err := native.RequireConcreteList(args[1], "Test.describe")
						if err != nil {
							return err
						}
						_, e := native.New(r).Run(body.Slice())
						return e
					}
					run.mu.Lock()
					run.path = append(run.path, name)
					run.mu.Unlock()
					runErr := runBody()
					run.mu.Lock()
					run.path = run.path[:len(run.path)-1]
					run.mu.Unlock()
					if runErr != nil {
						return nil, runErr
					}
					return nil, nil
				}),
				Returns: []*native.Type{}, BarrierPos: -1,
			}},
		},
		// test "name" [body] — run body, record a TestResult. Catches
		// assertion errors so other tests continue.
		{
			Name: "test-test",
			// Test.test "n" [body] — a SIDE-EFFECT case body (BodyOut 0) whose
			// assertions raise on failure and otherwise net nothing. Declared
			// module-side so eng holds no boru:test name — review §4.5.
			Callable: &native.CallableSpec{BodyPos: 1, BodyOut: 0, Inputs: func(_ []native.Value) []native.Value {
				return []native.Value{}
			}},
			Signatures: []native.Signature{{
				Args:       []*native.Type{native.TString, native.TList},
				NoEvalArgs: map[int]bool{1: true},
				Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
					name, _ := args[0].AsConcreteString()
					run := activeRun(parent)
					// Compiled path: the body arrived as a compiled CLOSURE (the
					// bytecode compiler lowers `Test.test "n" [body]` via the
					// closure path keyed on this word's declared Callable spec); run
					// it through the VM seam. The
					// interpreter path runs the token list in a sub-engine. Both nets
					// 0 values; an assertion failure raises, runCase records pass/fail.
					if native.IsCompiledClosure(args[1]) {
						body := args[1]
						run.runCase(r, name, func() error {
							_, err := native.InvokeBody(r, body, nil)
							return err
						})
						return nil, nil
					}
					body, err := native.RequireConcreteList(args[1], "Test.test")
					if err != nil {
						return nil, err
					}
					run.runCase(r, name, func() error {
						_, e := native.New(r).Run(body.Slice())
						return e
					})
					return nil, nil
				}),
				Returns: []*native.Type{}, BarrierPos: -1,
			}},
		},
		// Test.results — return the accumulated TestResult Table.
		{
			Name: "test-results",
			Signatures: []native.Signature{{
				Args: []*native.Type{},
				Impl: native.Go(func(_ []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					return []native.Value{activeRun(parent).asTable()}, nil
				}),
				Returns: []*native.Type{native.TList}, BarrierPos: -1,
			}},
		},
		// Test.reset — clear the active TestRun.
		{
			Name: "test-reset",
			Signatures: []native.Signature{{
				Args: []*native.Type{},
				Impl: native.Go(func(_ []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					run := activeRun(parent)
					run.mu.Lock()
					run.results = nil
					run.failures = 0
					run.path = nil
					run.mu.Unlock()
					return nil, nil
				}),
				Returns: []*native.Type{}, BarrierPos: -1,
			}},
		},
		// Test.summary — return a Record with pass/fail/total counts.
		{
			Name: "test-summary",
			Signatures: []native.Signature{{
				Args: []*native.Type{},
				Impl: native.Go(func(_ []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					return []native.Value{activeRun(parent).summary()}, nil
				}),
				Returns:   []*native.Type{native.TMap},
				ReturnsFn: testSummaryShapeReturns, BarrierPos: -1,
			}},
		},
		// Test.report — return a one-line-per-property pass/fail/skip
		// summary String (plus a tally line). §11b.5: the readable CI
		// alternative to the verbose Test.results table.
		{
			Name: "test-report",
			Signatures: []native.Signature{{
				Args: []*native.Type{},
				Impl: native.Go(func(_ []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					return []native.Value{activeRun(parent).report()}, nil
				}),
				Returns: []*native.Type{native.TString}, BarrierPos: -1,
			}},
		},
		// Test.fail-count — return the failure count as an integer.
		{
			Name: "test-fail-count",
			Signatures: []native.Signature{{
				Args: []*native.Type{},
				Impl: native.Go(func(_ []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					run := activeRun(parent)
					run.mu.Lock()
					n := run.failures
					run.mu.Unlock()
					return []native.Value{native.NewInteger(int64(n))}, nil
				}),
				Returns: []*native.Type{native.TInteger}, BarrierPos: -1,
			}},
		},

		// --- assertions ---
		// All assertion words raise a boru error with code
		// "assertion_failure" when they fail. The enclosing test
		// handler catches the error and records the case as failed.
		// The PURE comparisons (equal / not-equal / ok / match) carry a
		// DryPassReturns mirror: a top-level assertion over concrete
		// literals that provably fails is flagged at `boru check` with the
		// byte-identical failure text. assert-throws runs a BODY, so it is
		// not pure and keeps the runtime-only contract.
		{
			Name: "assert-equal",
			Signatures: []native.Signature{{
				Args: []*native.Type{native.TAny, native.TAny},
				Impl: native.Go(assertEqualHandler), ReturnsFn: native.DryPassReturns(assertEqualHandler),
				Returns: []*native.Type{}, BarrierPos: -1,
			}},
		},
		{
			Name: "assert-not-equal",
			Signatures: []native.Signature{{
				Args: []*native.Type{native.TAny, native.TAny},
				Impl: native.Go(assertNotEqualHandler), ReturnsFn: native.DryPassReturns(assertNotEqualHandler),
				Returns: []*native.Type{}, BarrierPos: -1,
			}},
		},
		{
			Name: "assert-ok",
			Signatures: []native.Signature{{
				Args: []*native.Type{native.TAny},
				Impl: native.Go(assertOkHandler), ReturnsFn: native.DryPassReturns(assertOkHandler),
				Returns: []*native.Type{}, BarrierPos: -1,
			}},
		},
		{
			Name: "assert-throws",
			// `Assert.throws [body]` runs a body for its RAISE and discards its
			// residual. The word's answer is "did it raise", and a raise is a
			// raise on either engine — the compiled body traps and returns the
			// same boru error the sub-engine run would have. So the body compiles
			// to a 0-input closure and the handler drives it through InvokeBody.
			//
			// BodyOutResidual because the residual is DISCARDED: a passing body
			// nets whatever it nets before raising, a failing one nets its full
			// value list, and neither count is a contract — so the closure must
			// compile count-agnostic.
			Callable: &native.CallableSpec{BodyPos: 0, BodyOut: native.BodyOutResidual, Inputs: func(_ []native.Value) []native.Value {
				return []native.Value{}
			}},
			Signatures: []native.Signature{{
				Args:       []*native.Type{native.TList},
				NoEvalArgs: map[int]bool{0: true},
				Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
					// Validate BEFORE running, so a non-concrete body raises the
					// same shape error on both engines.
					compiled := native.IsCompiledClosure(args[0])
					var body native.ReadList
					if !compiled {
						var err error
						if body, err = native.RequireConcreteList(args[0], "Assert.throws"); err != nil {
							return nil, err
						}
					}
					var runErr error
					if compiled {
						_, runErr = native.InvokeBody(r, args[0], nil)
					} else {
						_, runErr = native.New(r).Run(body.Slice())
					}
					if runErr == nil {
						return nil, r.BoruError("assertion_failure",
							"Assert.throws: body did not throw",
							"Assert.throws")
					}
					return nil, nil
				}),
				Returns: []*native.Type{}, BarrierPos: -1,
			}},
		},
		{
			Name: "assert-match",
			Signatures: []native.Signature{{
				Args: []*native.Type{native.TString, native.TString},
				Impl: native.Go(assertMatchHandler), ReturnsFn: native.DryPassReturns(assertMatchHandler),
				Returns: []*native.Type{}, BarrierPos: -1,
			}},
		},
		// --- spec runner (Go side) ---
		// Test.invoke subject-atom inputs-list — call the subject in
		// the parent registry by pushing inputs as tokens and
		// dispatching the word in a sub-engine. Returns the top-of-
		// stack result (or Error value). Runs against `parent` (the
		// caller's registry) — the boru spec runner lives in the test
		// module's sub-registry, but the subject under test is defined
		// in the caller's scope.
		{
			Name: "test-invoke",
			Signatures: []native.Signature{
				{
					Args: []*native.Type{native.TAtom, native.TList},
					Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
						name, _ := args[0].AsConcreteAtom()
						return invokeSubject(parent, name, args[1])
					}),
					Returns: []*native.Type{native.TAny}, BarrierPos: -1,
				},
				{
					Args: []*native.Type{native.TString, native.TList},
					Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
						name, _ := args[0].AsConcreteString()
						return invokeSubject(parent, name, args[1])
					}),
					Returns: []*native.Type{native.TAny}, BarrierPos: -1,
				},
			},
		},
		// Test.record name path ok expected actual error duration-ms
		//   — append a TestResult to the active TestRun. Used by the
		//   boru spec runner to assemble results uniformly with the
		//   imperative API.
		{
			Name: "test-record",
			Signatures: []native.Signature{{
				Args: []*native.Type{
					native.TString, native.TList, native.TBoolean,
					native.TAny, native.TAny, native.TAny, native.TInteger,
				},
				Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
					// Side-effect suppression (design/legacy/module-fn-checkstate-ownership.1.ignore
					// §5c): test-record accumulates pass/fail outcomes into the run. Once
					// a module-fn body (run-case) runs IN CHECK MODE under the parent pass
					// (§5b), this handler fires during the compile/check pass — it must NOT
					// mutate observable state, or the compiled path leaks pass/fail counts
					// (the module-test:38 {passed:1 failed:1} vs {passed:2 failed:0}
					// divergence). Skip the write when the pass suppresses side effects.
					if r != nil && r.Check.SkipsSideEffect() {
						return nil, nil
					}
					name, _ := args[0].AsConcreteString()
					pathList, _ := native.AsList(args[1])
					ok, _ := args[2].AsConcreteBoolean()
					duration, _ := args[6].AsConcreteInteger()
					run := activeRun(parent)
					run.mu.Lock()
					defer run.mu.Unlock()
					path := make([]string, 0, pathList.Len())
					for _, p := range pathList.Slice() {
						s, _ := native.AsString(p)
						path = append(path, s)
					}
					run.results = append(run.results, makeResult(name, path, ok, args[3], args[4], args[5], time.Duration(duration)*time.Millisecond))
					if !ok {
						run.failures++
					}
					return nil, nil
				}),
				Returns: []*native.Type{}, BarrierPos: -1,
			}},
		},
		// Test.prop name gen property → PropertySpec map.
		//   — constructs a PropertySpec with default runs=100, seed=1,
		//   max-shrinks=200. Implemented in Go (not as a boru fn)
		//   because gen/property are List bodies that would otherwise
		//   be auto-evaluated during fn-param binding; this native
		//   uses NoEvalArgs + explicit Quoted=true to preserve the
		//   bodies intact for the Stage-5 reducer / Stage-3 runner.
		{
			Name: "test-prop",
			Signatures: []native.Signature{{
				Args:       []*native.Type{native.TString, native.TList, native.TList},
				NoEvalArgs: map[int]bool{1: true, 2: true},
				Returns:    []*native.Type{native.TMap},
				// The shape carrier flows into run-property's
				// `p:PropertySpec` PARAM as well as field reads: a
				// record-schema carrier unifies with a record-refine
				// param's field-bag pattern by the runtime's open rule
				// (unifyRecordSchemaCarrierVsMap, NUR068), so the claim
				// narrows `p get "runs"` without killing that dispatch —
				// the constraint that used to keep this sig shapeless.
				ReturnsFn:  testPropSpecShapeReturns,
				BarrierPos: -1,
				Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					name, _ := args[0].AsConcreteString()
					gen := args[1]
					prop := args[2]
					// Mark bodies Quoted so subsequent consumers
					// (e.g. run-property's `get` retrieving them
					// from the map) don't auto-evaluate them.
					gen.Quoted = true
					gen.Eval = false
					prop.Quoted = true
					prop.Eval = false
					m := native.NewOrderedMap()
					m.Set("name", native.NewString(name))
					m.Set("gen", gen)
					m.Set("property", prop)
					m.Set("runs", native.NewInteger(100))
					m.Set("seed", native.NewInteger(1))
					m.Set("max-shrinks", native.NewInteger(200))
					return []native.Value{native.NewMap(m)}, nil
				}),
			}},
		},
		// Test.check-prop name gen property runs seed max-shrinks
		//   — property-based test driver. Runs the generator body
		//   `runs` times, each iteration with a fresh seeded rand
		//   instance bound as `r`. The property body is called with
		//   the generated value on the stack; it must return Boolean.
		//   On the first false return, records a failure with the
		//   generated input. Returns a PropertyResult Map and also
		//   appends it to the active testRun.
		//
		// The `max-shrinks` arg is reserved for the Stage-5 reducer
		// (PBT-PLAN.10.md) — Stage 3 ignores it and reports the raw
		// failing input verbatim.
		{
			Name: "test-check-prop",
			// The gen/property bodies are param-carrying stored bodies: the
			// recorder compiles each to a closure unit with the SAME params
			// runCheckProp's own CallBoru frames bind (gen: named `r`; property:
			// one unnamed Any), and runCheckProp dispatches the carriers via
			// InvokeCallback — per-iteration VM runs instead of interpreter
			// frames. A body that refuses (a frame-local ${interp} — the
			// fn-scope guard) keeps its raw list and interprets, unchanged.
			StoredBodies: []native.StoredBodySpec{
				{Pos: 1, Params: []native.FnParam{{Name: "r", Type: native.TMap}}},
				{Pos: 2, Params: []native.FnParam{{Type: native.TAny}}},
			},
			Signatures: []native.Signature{{
				Args: []*native.Type{
					native.TString,  // name
					native.TList,    // gen body (quoted)
					native.TList,    // property body (quoted)
					native.TInteger, // runs
					native.TInteger, // seed
					native.TInteger, // max-shrinks (consumed by both reducers)
				},
				NoEvalArgs: map[int]bool{1: true, 2: true},
				Returns:    []*native.Type{native.TMap},
				ReturnsFn:  testPropResultShapeReturns,
				BarrierPos: -1,
				// runCheckProp runs BOTH bodies (the generator, param `r`; the
				// property, one unnamed param) through parent.CallBoru in fresh
				// isolated frames against the module-captured parent registry — the
				// SAME Go handler in interpreter and compiled mode. So the bodies
				// never couple to the VM tape or a compiled frame local: a plain
				// CALL_NATIVE bake is sound even when the bodies arrive as DYNAMIC
				// values (the declarative `_prop_spec` surface fetches them via
				// `p get "gen"` / `p get "property"`). Without this the NoEvalArgs
				// dynamic-body refusal blocked every `_prop_spec` file.
				CompileEffect: native.CompileRunsBodyIsolated,
				Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					return runCheckProp(parent, args)
				}),
			}},
		},
		// Test.skip parks a test WITHOUT running its body — two forms:
		//   Test.skip "name" [body]                         — skip a CASE (the
		//     drop-in for `Test.test "name" [body]`; records ok+skipped).
		//   Test.skip "name" [gen] [prop] runs seed shrinks — skip a PROPERTY
		//     (the drop-in for `Test.check-prop`).
		// Either way the entry still appears in Test.report / Test.results as
		// `skip:`, contributes no failure, and costs nothing to evaluate — so a
		// case can be parked while iterating instead of commented out (§11b.4).
		{
			Name: "test-skip",
			Signatures: []native.Signature{
				{
					// Case form: name + a quoted (never-run) body.
					Args:       []*native.Type{native.TString, native.TList},
					NoEvalArgs: map[int]bool{1: true},
					Returns:    []*native.Type{}, BarrierPos: -1,
					Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
						name, _ := args[0].AsConcreteString()
						activeRun(parent).skipCase(name)
						return nil, nil
					}),
				},
				{
					// Property form: name + gen + property + runs/seed/shrinks.
					Args: []*native.Type{
						native.TString,  // name
						native.TList,    // gen body (quoted, ignored)
						native.TList,    // property body (quoted, ignored)
						native.TInteger, // runs (ignored)
						native.TInteger, // seed (ignored)
						native.TInteger, // max-shrinks (consumed by both reducers)
					},
					NoEvalArgs: map[int]bool{1: true, 2: true},
					Returns:    []*native.Type{native.TMap},
					ReturnsFn:  testPropResultShapeReturns,
					BarrierPos: -1,
					Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
						return runSkipProp(parent, args)
					}),
				},
			},
		},
	}
}

// ---- check-mode shape ReturnsFns -------------------------------------
//
// The PBT surfaces build FIXED map shapes (check-prop/skip a
// PropertyResult, Test.summary the tally); surfacing the
// shapes as record-schema carriers lets field reads (`p get "runs"`,
// `(… check-prop …) get "ok"`) narrow instead of ending dynamic(Any). All
// claims are dynamic (gradual, guard-discharged); value-dependent fields
// (failing-input / shrunk-input / error — Any by the PropertyResult decl)
// stay dynamic(Any) via the schema's own Any constraint.

// testPropSpecShapeReturns mirrors the PropertySpec record the test-prop
// handler constructs (name/gen/property with the runs/seed/max-shrinks
// defaults), field-for-field in the preamble type's order. gen/property
// are quoted CODE bodies, typed List by the schema.
func testPropSpecShapeReturns(_ []native.Value, _ *native.Registry) []native.Value {
	fields := native.NewOrderedMap()
	fields.Set("name", native.NewTypeLiteral(native.TString))
	fields.Set("gen", native.NewTypeLiteral(native.TList))
	fields.Set("property", native.NewTypeLiteral(native.TList))
	fields.Set("runs", native.NewTypeLiteral(native.TInteger))
	fields.Set("seed", native.NewTypeLiteral(native.TInteger))
	fields.Set("max-shrinks", native.NewTypeLiteral(native.TInteger))
	return []native.Value{native.NewDynamicCarrierValue(native.NewRecordType(fields))}
}

func testPropResultShapeReturns(_ []native.Value, _ *native.Registry) []native.Value {
	fields := native.NewOrderedMap()
	fields.Set("name", native.NewTypeLiteral(native.TString))
	fields.Set("ok", native.NewTypeLiteral(native.TBoolean))
	fields.Set("skipped", native.NewTypeLiteral(native.TBoolean))
	fields.Set("runs", native.NewTypeLiteral(native.TInteger))
	fields.Set("shrunk-source", native.NewTypeLiteral(native.TString))
	fields.Set("shrunk-cost", native.NewTypeLiteral(native.TInteger))
	return []native.Value{native.NewDynamicCarrierValue(native.NewRecordType(fields))}
}

func testSummaryShapeReturns(_ []native.Value, _ *native.Registry) []native.Value {
	fields := native.NewOrderedMap()
	fields.Set("total", native.NewTypeLiteral(native.TInteger))
	fields.Set("passed", native.NewTypeLiteral(native.TInteger))
	fields.Set("failed", native.NewTypeLiteral(native.TInteger))
	return []native.Value{native.NewDynamicCarrierValue(native.NewRecordType(fields))}
}

// runSkipProp records a skipped property result without running the
// generator or property bodies. The skipped entry is ok=true and carries
// a `skipped: true` marker so Test.report renders it as `skip:` and it
// never counts as a failure. See §11b.4.
func runSkipProp(parent *native.Registry, args []native.Value) ([]native.Value, error) {
	name, _ := args[0].AsConcreteString()
	result := native.NewOrderedMap()
	result.Set("name", native.NewString(name))
	result.Set("ok", native.NewBoolean(true))
	result.Set("skipped", native.NewBoolean(true))
	result.Set("runs", native.NewInteger(0))
	resultVal := native.NewMap(result)
	run := activeRun(parent)
	run.mu.Lock()
	run.results = append(run.results, resultVal)
	run.mu.Unlock()
	return []native.Value{resultVal}, nil
}

// storedBodyArg splits a check-prop body arg into its dispatch sig and raw
// tokens. A COMPILED body arrives as a stored-param-body carrier
// (Signature.StoredBodies): its single sig — the handler's own CallBoru param
// shape plus the CompiledFnRef — dispatches through InvokeCallback, so the
// unit runs nested on the VM with the interpreter as its per-invoke
// fallback. A raw list (interpreter mode, or a body the compile declined)
// yields tokens only, and the caller builds today's throwaway CallBoru sig.
func storedBodyArg(arg native.Value, what string) (*native.FnSig, []native.Value, error) {
	if fd, ok := arg.Data.(native.FnDefInfo); ok && len(fd.Signatures) == 1 {
		if impl, isBoru := fd.Signatures[0].Impl.(*core.BoruImpl); isBoru && compiler.CompiledRef(&fd.Signatures[0]) != nil {
			return &fd.Signatures[0], impl.Body, nil
		}
	}
	lst, err := native.RequireConcreteList(arg, what)
	if err != nil {
		return nil, nil, err
	}
	return nil, lst.Slice(), nil
}

// requirePropCount validates one of Test.check-prop's numeric arguments
// against its legal range, raising `range_error` — the code REFERENCE.md
// already defines as "a numeric argument outside a word's legal range" —
// when the caller is outside it.
//
// `runs` has a floor of 1. A non-positive count runs the property loop
// zero times, and the driver would then report `{ok: true, runs: 0}`,
// which Test.report prints as a PASS — a vacuous success even when the
// property body is literally `[false]`. That is a caller mistake, so it
// is an ERROR rather than a silent clamp to 1: clamping would run the
// property once and pass, hiding the wrong count instead of naming it.
//
// `max-shrinks` has a floor of 0, because 0 is the documented "do not
// shrink" setting that both reducers already honour (see
// shrinkFailingInput / shrinkFailingProgram); a NEGATIVE value is the
// same class of caller mistake, silently behaving as 0 today.
//
// `seed` is deliberately unconstrained: every integer, negative
// included, is a legal seed for the per-iteration rand instance.
func requirePropCount(parent *native.Registry, name string, got, min int64) error {
	if got < min {
		return parent.BoruError("range_error",
			fmt.Sprintf("Test.check-prop: %s must be %d or more (got %d)", name, min, got),
			"Test.check-prop")
	}
	return nil
}

// runCheckProp is the PBT inner loop. Extracted for readability —
// the check-prop native's handler delegates here.
func runCheckProp(parent *native.Registry, args []native.Value) ([]native.Value, error) {
	name, _ := args[0].AsConcreteString()
	genSigC, genBody, err := storedBodyArg(args[1], "Test.check-prop gen")
	if err != nil {
		return nil, err
	}
	propSigC, propBody, err := storedBodyArg(args[2], "Test.check-prop property")
	if err != nil {
		return nil, err
	}
	runs, _ := args[3].AsConcreteInteger()
	seed, _ := args[4].AsConcreteInteger()
	maxShrinks, _ := args[5].AsConcreteInteger()
	if err := requirePropCount(parent, "runs", runs, 1); err != nil {
		return nil, err
	}
	if err := requirePropCount(parent, "max-shrinks", maxShrinks, 0); err != nil {
		return nil, err
	}

	var (
		failed       bool
		actualRuns   int64
		failingIter  int64 = -1
		failingInput       = native.NewNone()
		failingError       = native.NewNone()
	)

	for i := int64(0); i < runs; i++ {
		actualRuns++

		// Build the per-iteration seeded rand instance. Each
		// iteration uses (seed + i) so failures replay with a
		// known-good sub-seed.
		randMap, err := w9BuildSeededRandInstance(seed + i)
		if err != nil {
			failed = true
			failingIter = i
			failingError = native.NewError(err)
			break
		}

		// Run the generator body with `r` bound to the iteration's rand
		// instance. Body must leave exactly one value on the stack — the
		// generated input. A stored-param-body carrier (compiled path)
		// dispatches through InvokeCallback — the unit runs nested on the
		// VM, and stale deps / internal errors degrade to the identical
		// CallBoru frame; a raw body builds today's throwaway sig.
		var genResults []native.Value
		if genSigC != nil {
			genResults, err = core.InvokeCallback(parent, genSigC, []native.Value{native.NewMap(randMap)}, nil)
		} else {
			genSig := native.FnSig{
				Params:     []native.FnParam{{Name: "r", Type: native.TMap}},
				Returns:    []*native.Type{native.TAny},
				Impl:       native.Boru(append([]native.Value(nil), genBody...)),
				BarrierPos: -1,
			}
			genResults, err = parent.CallBoru(&genSig, []native.Value{native.NewMap(randMap)}, nil)
		}
		if err != nil {
			failed = true
			failingIter = i
			failingError = native.NewError(err)
			break
		}
		if len(genResults) == 0 {
			failed = true
			failingIter = i
			failingError = native.NewError(parent.BoruError("check_prop_error",
				"generator produced no value", "Test.check-prop"))
			break
		}
		input := genResults[len(genResults)-1]

		// Run the property body via CallBoru with one unnamed param
		// bound to the generated input. The body sees the value on
		// the stack (so stack-form bodies like `[0 gte]` work) AND
		// can reference it via `args.0` (so map-destructuring bodies
		// work). Body must leave a Boolean; anything else is a failure.
		var propResults []native.Value
		if propSigC != nil {
			propResults, err = core.InvokeCallback(parent, propSigC, []native.Value{input}, nil)
		} else {
			propSig := native.FnSig{
				Params:     []native.FnParam{{Type: native.TAny}},
				Returns:    []*native.Type{native.TAny},
				Impl:       native.Boru(append([]native.Value(nil), propBody...)),
				BarrierPos: -1,
			}
			propResults, err = parent.CallBoru(&propSig, []native.Value{input}, nil)
		}
		if err != nil {
			failed = true
			failingIter = i
			failingInput = input
			failingError = native.NewError(err)
			break
		}
		if len(propResults) == 0 {
			failed = true
			failingIter = i
			failingInput = input
			failingError = native.NewError(parent.BoruError("check_prop_error",
				"property produced no value", "Test.check-prop"))
			break
		}
		propTop := propResults[len(propResults)-1]
		propBool, err := propTop.AsConcreteBoolean()
		if err != nil {
			failed = true
			failingIter = i
			failingInput = input
			failingError = native.NewError(parent.BoruError("check_prop_error",
				fmt.Sprintf("property returned non-Boolean (%s)", propTop.Parent.String()),
				"Test.check-prop"))
			break
		}
		if !propBool {
			failed = true
			failingIter = i
			failingInput = input
			break
		}
	}

	// Shrinking: on failure, try GEN-PROGRAM shrinking first
	// (mutates the generator's stack form, re-runs the property
	// against each candidate's produced value). If the gen-program
	// reducer can't lower cost (form unrecordable, no improving
	// candidate found), fall back to VALUE-LEVEL shrinking (mutates
	// the failing value directly). The complementary path covers
	// both cases: simple value-shape failures shrink via direct
	// value mutation; structured gen programs shrink via the
	// program-level rewrites.
	shrunkInput := failingInput
	shrunkSource := ""
	shrunkCost := int64(0)
	if failed && !native.IsNone(failingInput) && failingIter >= 0 {
		genSv, genSrc, genCost, genOK := shrinkFailingProgram(
			parent, genBody, propBody, seed+failingIter, maxShrinks)
		if genOK {
			shrunkInput = genSv
			shrunkSource = genSrc
			shrunkCost = genCost
		} else {
			shrunkInput, shrunkSource, shrunkCost = shrinkFailingInput(
				parent, failingInput, propBody, maxShrinks)
		}
	}

	// Build the PropertyResult map.
	result := native.NewOrderedMap()
	result.Set("name", native.NewString(name))
	result.Set("ok", native.NewBoolean(!failed))
	result.Set("runs", native.NewInteger(actualRuns))
	result.Set("failing-input", failingInput)
	result.Set("shrunk-input", shrunkInput)
	result.Set("shrunk-source", native.NewString(shrunkSource))
	result.Set("shrunk-cost", native.NewInteger(shrunkCost))
	result.Set("error", failingError)

	// Append to the active test run so Test.results and
	// Test.summary pick this up alongside table-driven tests.
	resultVal := native.NewMap(result)
	run := activeRun(parent)
	run.mu.Lock()
	run.results = append(run.results, resultVal)
	if failed {
		run.failures++
	}
	run.mu.Unlock()

	return []native.Value{resultVal}, nil
}

// shrinkFailingProgram runs gen-program shrinking: compile the gen
// body's StackForm under the failing iteration's seed, then let the
// shrink reducer mutate the form (drop ops, shrink literals, prune
// quote bodies). Each candidate evaluates against a fresh seeded
// rand-equipped registry; the produced value runs through the
// property body via CallBoru. Failure-preserving lower-cost
// candidates win.
//
// Returns the reduced form's eval result, the pretty-printed source,
// and the reduced cost — plus a boolean indicating whether the
// reducer actually improved on the initial form. Callers fall back
// to value-level shrinking when this returns ok=false (e.g. the gen
// body uses non-recordable patterns or no candidate beat the
// initial cost).
//
// failingSeed is the per-iteration sub-seed (typically base-seed +
// iteration-index) that produced the failing input. Every candidate
// eval rebuilds rand state from this seed so replay is deterministic.
func shrinkFailingProgram(
	parent *native.Registry,
	genBody []native.Value,
	propBody []native.Value,
	failingSeed int64,
	maxShrinks int64,
) (shrunkValue native.Value, shrunkSource string, shrunkCost int64, ok bool) {
	if maxShrinks <= 0 {
		return native.NewNone(), "", 0, false
	}

	// Compile the initial gen-body StackForm. Run the gen body in a
	// sub-engine where `r` is bound to a freshly-seeded rand instance
	// (matching what the failing iteration would have done) and a
	// stackform.Recorder is installed to capture the execution.
	randMap, err := w9BuildSeededRandInstance(failingSeed)
	if err != nil {
		return native.NewNone(), "", 0, false
	}
	parent.Defs.Push("r", native.NewMap(randMap))
	_, initialForm, err := stackform.Compile(parent, append([]native.Value(nil), genBody...))
	parent.Defs.Pop("r")
	if err != nil || initialForm == nil || initialForm.Len() == 0 {
		return native.NewNone(), "", 0, false
	}

	policy := shrink.DefaultPolicy()

	// evalFn: build a fresh seeded eval registry, run the candidate
	// form to produce a value, then run the property body against it.
	// Each candidate eval is deterministic (same seed → same rng
	// stream) so the reducer's search space is reproducible.
	evalFn := func(form *stackform.StackForm) shrink.Outcome {
		if form == nil || form.Len() == 0 {
			return shrink.Invalid
		}
		evalReg, err := w9BuildSeededRandRegistry(failingSeed)
		if err != nil {
			return shrink.Invalid
		}
		vals, err := w9StackformEval(evalReg, form)
		if err != nil || len(vals) == 0 {
			return shrink.Invalid
		}
		candidateValue := vals[len(vals)-1]
		propSig := native.FnSig{
			Params:     []native.FnParam{{Type: native.TAny}},
			Returns:    []*native.Type{native.TAny},
			Impl:       native.Boru(append([]native.Value(nil), propBody...)),
			BarrierPos: -1,
		}
		res, err := parent.CallBoru(&propSig, []native.Value{candidateValue}, nil)
		if err != nil || len(res) == 0 {
			return shrink.Invalid
		}
		b, err := res[len(res)-1].AsConcreteBoolean()
		if err != nil {
			return shrink.Invalid
		}
		if !b {
			return shrink.Fail
		}
		return shrink.Pass
	}

	profile := &shrink.Profile{
		MaxSteps: int(maxShrinks),
		Policy:   policy,
	}
	reduced := shrink.Reduce(initialForm, evalFn, profile)

	initialCost := shrink.ShrinkCost(initialForm, policy)
	reducedCost := shrink.ShrinkCost(reduced, policy)
	if reducedCost >= initialCost {
		// No improvement. Caller should fall back to value-level
		// shrinking on the original failing input.
		return native.NewNone(), "", 0, false
	}

	// Materialise the shrunk value by re-evaluating the reduced form.
	evalReg, err := w9BuildSeededRandRegistry(failingSeed)
	if err != nil {
		return native.NewNone(), "", 0, false
	}
	vals, err := w9StackformEval(evalReg, reduced)
	if err != nil || len(vals) == 0 {
		return native.NewNone(), "", 0, false
	}
	return vals[len(vals)-1], stackform.Pretty(reduced), int64(reducedCost), true
}

// shrinkFailingInput reduces a property-failing input to a minimal
// counterexample via the shrink package. Used by runCheckProp on
// failure to populate shrunk-input / shrunk-source / shrunk-cost in
// the PropertyResult.
//
// Strategy is value-level shrinking: represent the failing input as
// a stackform.PushLit (or recurse for list/map structure), let the
// shrink package's candidate generators try smaller alternatives,
// and accept each one whose result still makes the property fail.
//
// maxShrinks caps the reducer's outer loop. 0 disables shrinking
// (returns the failing input verbatim). Defaults to 200 when the
// PropertySpec uses the Test.prop constructor.
func shrinkFailingInput(
	parent *native.Registry,
	failingInput native.Value,
	propBody []native.Value,
	maxShrinks int64,
) (shrunkInput native.Value, shrunkSource string, shrunkCost int64) {
	shrunkInput = failingInput
	shrunkSource = ""
	shrunkCost = 0
	if maxShrinks <= 0 {
		return
	}

	policy := shrink.DefaultPolicy()
	initialForm := &stackform.StackForm{Ops: []stackform.Op{
		stackform.PushLit{V: failingInput},
	}}

	// evalFn extracts the candidate's top value, pushes it onto a
	// fresh sub-engine on parent, runs the property body, and maps
	// the Boolean result to Outcome. Errors / non-Boolean / missing
	// values → Invalid (reducer rejects).
	evalFn := func(form *stackform.StackForm) shrink.Outcome {
		if form == nil || len(form.Ops) == 0 {
			return shrink.Invalid
		}
		// Extract top-of-stack value from the candidate. Forms here
		// are always shape [PushLit(v)] (possibly nested via
		// shrinkLiteral on a list/map) — Flatten + Run gives the
		// reconstructed value.
		vals, err := w9StackformEval(parent, form)
		if err != nil || len(vals) == 0 {
			return shrink.Invalid
		}
		candidateValue := vals[len(vals)-1]
		// Same CallBoru plumbing as the main loop: an unnamed Any
		// param makes `args.0` available inside the property body.
		propSig := native.FnSig{
			Params:     []native.FnParam{{Type: native.TAny}},
			Returns:    []*native.Type{native.TAny},
			Impl:       native.Boru(append([]native.Value(nil), propBody...)),
			BarrierPos: -1,
		}
		res, err := parent.CallBoru(&propSig, []native.Value{candidateValue}, nil)
		if err != nil || len(res) == 0 {
			return shrink.Invalid
		}
		b, err := res[len(res)-1].AsConcreteBoolean()
		if err != nil {
			return shrink.Invalid
		}
		if !b {
			return shrink.Fail
		}
		return shrink.Pass
	}

	profile := &shrink.Profile{
		MaxSteps: int(maxShrinks),
		Policy:   policy,
	}
	reducedForm := shrink.Reduce(initialForm, evalFn, profile)

	// Extract shrunk input from the reduced form. Should be a
	// single PushLit (or a series that evaluates to one value).
	if reducedForm != nil && len(reducedForm.Ops) > 0 {
		if vals, err := w9StackformEval(parent, reducedForm); err == nil && len(vals) > 0 {
			shrunkInput = vals[len(vals)-1]
		}
	}
	shrunkSource = stackform.Pretty(reducedForm)
	shrunkCost = int64(shrink.ShrinkCost(reducedForm, policy))
	return
}

// runCase executes one test body, catching errors and recording a
// TestResult under the current describe path.
func (run *testRun) runCase(r *native.Registry, name string, runBody func() error) {
	run.mu.Lock()
	pathCopy := append([]string(nil), run.path...)
	run.mu.Unlock()

	start := time.Now()
	err := runBody()
	elapsed := time.Since(start)

	ok := err == nil
	var errVal native.Value
	if err != nil {
		errVal = native.NewError(err)
		// Surface the failure loudly and NAMED at the point it happens.
		// `Test.test` catches the error so later cases still run, which
		// otherwise leaves the only signal a count (`Test.fail-count`)
		// whose summary-line assertion names nothing about which case
		// failed (voxgig DX report B7). A `FAIL <case> — <reason>` line
		// on stderr makes example-test failures as identifiable as the
		// property drivers' `failing-input`.
		if r != nil && r.ErrOutput != nil {
			label := name
			if len(pathCopy) > 0 {
				label = strings.Join(pathCopy, " / ") + " / " + name
			}
			fmt.Fprintf(r.ErrOutput, "FAIL %s — %s\n", label, firstErrLine(err))
		}
	} else {
		errVal = native.NewNone()
	}
	result := makeResult(name, pathCopy, ok, native.NewNone(), native.NewNone(), errVal, elapsed)

	run.mu.Lock()
	run.results = append(run.results, result)
	if !ok {
		run.failures++
	}
	run.mu.Unlock()
}

// skipCase records a SKIPPED test case: like runCase but the body is never
// run, so the result is ok=true + skipped=true (Test.report renders it `skip:`,
// the tally counts it under skipped, and it never fails the run). This is the
// drop-in for `Test.test "n" [body]` → `Test.skip "n" [body]` to park a case
// while iterating, instead of commenting it out or deleting the body.
func (run *testRun) skipCase(name string) {
	run.mu.Lock()
	pathCopy := append([]string(nil), run.path...)
	run.mu.Unlock()
	pathVals := make([]native.Value, len(pathCopy))
	for i, p := range pathCopy {
		pathVals[i] = native.NewString(p)
	}
	om := native.NewOrderedMap()
	om.Set("name", native.NewString(name))
	om.Set("path", native.NewList(pathVals))
	om.Set("ok", native.NewBoolean(true))
	om.Set("skipped", native.NewBoolean(true))
	om.Set("expected", native.NewNone())
	om.Set("actual", native.NewNone())
	om.Set("error", native.NewNone())
	om.Set("duration-ms", native.NewInteger(0))
	run.mu.Lock()
	run.results = append(run.results, native.NewMap(om))
	run.mu.Unlock()
}

// firstErrLine returns the first line of an error's message. boru errors
// carry a multi-line source extract; the loud per-case FAIL line wants a
// single scannable reason.
func firstErrLine(err error) string {
	msg := err.Error()
	if i := strings.IndexByte(msg, '\n'); i >= 0 {
		return msg[:i]
	}
	return msg
}

// makeResult builds a TestResult Map value matching the schema declared
// in the boru preamble (testBoruPreamble).
func makeResult(name string, path []string, ok bool, expected, actual, errVal native.Value, elapsed time.Duration) native.Value {
	pathVals := make([]native.Value, len(path))
	for i, p := range path {
		pathVals[i] = native.NewString(p)
	}
	om := native.NewOrderedMap()
	om.Set("name", native.NewString(name))
	om.Set("path", native.NewList(pathVals))
	om.Set("ok", native.NewBoolean(ok))
	om.Set("expected", expected)
	om.Set("actual", actual)
	om.Set("error", errVal)
	om.Set("duration-ms", native.NewInteger(elapsed.Milliseconds()))
	return native.NewMap(om)
}

// asTable wraps the accumulated results as a TableData value so the
// caller can pipe them through `Report.table`.
func (run *testRun) asTable() native.Value {
	run.mu.Lock()
	defer run.mu.Unlock()
	rec := native.NewOrderedMap()
	rec.Set("name", native.NewTypeLiteral(native.TString))
	rec.Set("path", native.NewTypeLiteral(native.TList))
	rec.Set("ok", native.NewTypeLiteral(native.TBoolean))
	rec.Set("expected", native.NewTypeLiteral(native.TAny))
	rec.Set("actual", native.NewTypeLiteral(native.TAny))
	rec.Set("error", native.NewTypeLiteral(native.TAny))
	rec.Set("duration-ms", native.NewTypeLiteral(native.TInteger))
	rows := make([]native.Value, len(run.results))
	copy(rows, run.results)
	return native.NewValueRaw(native.TList, native.TableData{
		Record: native.RecordTypeInfo{Fields: rec},
		Rows:   rows,
	})
}

// report renders one line per recorded result — `pass:`, `FAIL:`, or
// `skip:` followed by the name (and the error / failing input for
// failures) — plus a trailing `N passed, M failed, K skipped` tally.
// This is the readable CI summary §11b.5 asks for: `Test.results`
// returns the full 7-column table + PropertyResult dumps, which is hard
// to scan in logs.
func (run *testRun) report() native.Value {
	run.mu.Lock()
	defer run.mu.Unlock()
	var b strings.Builder
	passed, failed, skipped := 0, 0, 0
	for _, res := range run.results {
		m, _ := native.AsMap(res)
		if m == nil {
			continue
		}
		name := resultStringField(m, "name")
		switch {
		case resultBoolField(m, "skipped"):
			skipped++
			fmt.Fprintf(&b, "  skip: %s\n", name)
		case resultBoolField(m, "ok"):
			passed++
			fmt.Fprintf(&b, "  pass: %s\n", name)
		default:
			failed++
			detail := resultStringField(m, "error")
			if detail == "" || detail == "none" {
				if fi, ok := m.Get("failing-input"); ok && !native.IsNoneShape(fi) {
					detail = "failing-input: " + fi.String()
				}
			}
			if detail != "" {
				fmt.Fprintf(&b, "  FAIL: %s — %s\n", name, detail)
			} else {
				fmt.Fprintf(&b, "  FAIL: %s\n", name)
			}
		}
	}
	fmt.Fprintf(&b, "%d passed, %d failed", passed, failed)
	if skipped > 0 {
		fmt.Fprintf(&b, ", %d skipped", skipped)
	}
	return native.NewString(b.String())
}

// resultStringField returns the named field rendered as a string, or ""
// when absent / None.
func resultStringField(m native.ReadMap, key string) string {
	v, ok := m.Get(key)
	if !ok || native.IsNoneShape(v) {
		return ""
	}
	if s, err := v.AsConcreteString(); err == nil {
		return s
	}
	return v.String()
}

// resultBoolField returns the named Boolean field, false when absent.
func resultBoolField(m native.ReadMap, key string) bool {
	v, ok := m.Get(key)
	if !ok {
		return false
	}
	b, err := v.AsConcreteBoolean()
	return err == nil && b
}

// summary builds a {total, passed, failed} Map for quick reporting.
func (run *testRun) summary() native.Value {
	run.mu.Lock()
	defer run.mu.Unlock()
	total := len(run.results)
	failed := run.failures
	om := native.NewOrderedMap()
	om.Set("total", native.NewInteger(int64(total)))
	om.Set("passed", native.NewInteger(int64(total-failed)))
	om.Set("failed", native.NewInteger(int64(failed)))
	return native.NewMap(om)
}

// invokeSubject runs a subject word against an input list in the
// parent registry. Shared by the Atom and String overloads of
// test-invoke.
func invokeSubject(parent *native.Registry, name string, inputArg native.Value) ([]native.Value, error) {
	inputs, err := native.RequireConcreteList(inputArg, "Test.invoke")
	if err != nil {
		return nil, err
	}
	tokens := append([]native.Value(nil), inputs.Slice()...)
	tokens = append(tokens, dottedWordTokens(name)...)
	sub := native.New(parent)
	stack, runErr := sub.Run(tokens)
	if runErr != nil {
		return []native.Value{native.NewError(runErr)}, nil
	}
	if len(stack) == 0 {
		return []native.Value{native.NewNone()}, nil
	}
	return []native.Value{stack[len(stack)-1]}, nil
}

// dottedWordTokens returns the token sequence the engine would
// produce for a dotted reference. A plain "foo" lexes to [Word(foo)];
// "MathUtil.sqrt" lexes to [Word(MathUtil), Word(get),
// Atom(sqrt)]. Test.invoke uses this so a spec can name its
// subject as either `sqrt/q` (when the user has imported the
// module's words flat) or `MathUtil.sqrt/q` (the more common
// form, when the user has the bare module import).
func dottedWordTokens(name string) []native.Value {
	parts := strings.Split(name, ".")
	if len(parts) == 1 {
		return []native.Value{native.NewWord(name)}
	}
	out := make([]native.Value, 0, 1+2*(len(parts)-1))
	out = append(out, native.NewWord(parts[0]))
	for _, p := range parts[1:] {
		out = append(out, native.NewWord("dot"), native.NewAtom(p))
	}
	return out
}

// isTruthy mirrors the boru convention used by `if` / `and` / `or`:
// false, none and the None type literal are falsy; everything else is
// truthy. This keeps `Assert.ok` aligned with the language's other
// boolean coercion sites without introducing a new rule.
func isTruthy(v native.Value) bool {
	if native.IsNone(v) {
		return false
	}
	if v.Parent.ConformsTo(native.TBoolean) {
		b, _ := native.AsBoolean(v)
		return b
	}
	if native.IsBareTypeNode(v) {
		return false
	}
	return true
}

// testBoruPreamble defines canonical record types and the pure-Boru
// spec runner. It runs inside the test module's sub-registry after
// the Go natives are registered, so the `export` map references both
// Go words (test-test, test-describe, ...) and boru types (TestCase,
// TestSet, TestSpec, TestResult).
//
// Naming convention: Go words use kebab prefixes (test-X) to avoid
// colliding with user-facing names; the export map renames them to
// the dotted form (Test.test, Test.describe, Assert.equal, ...).
const testBoruPreamble = `

# ============================================================
# boru:test — Record / Table types
# ============================================================

# A single test case in a declarative spec.
# - name:    label printed in reports
# - in:      list of inputs pushed in order onto the subject's stack
# - out:     expected top-of-stack result after the subject runs
def TestCase refine Record [name:String in:List out:Any]

# A set of test cases — a Table over TestCase.
def TestSet refine Table (refine Record [name:String in:List out:Any])

# A whole spec: a named group with a subject (atom or dotted-string
# name referring to a word resolvable in the def stack at run time)
# and either inline cases or sub-specs (or both).
# - subject:  Atom or String naming the word under test. Strings
#             support dotted names like "MathUtil.sqrt" so a
#             spec can target a module export without first flat-
#             importing the word.
# - cases:    inline TestSet (may be empty)
# - subs:     list of sub-specs (may be empty)
def TestSpec refine Record [name:String subject:Any cases:List subs:List]

# ============================================================
# Property-based testing (PBT) — Stage 3
# ============================================================
# A PropertySpec describes a property to be checked against
# randomly-generated inputs. The framework runs gen runs
# times, each time with a fresh seeded rand instance bound as
# r inside the gen body; the resulting value is then fed to
# property which must return a Boolean. False or an error in
# any iteration is a failure.
#
# - name:         label for the report.
# - gen:          quoted code body that produces ONE value at
#                 stack top. Receives the iteration's rand
#                 instance via the bound name r.
# - property:     quoted code body that takes the generated
#                 value on the stack and leaves a Boolean.
# - runs:         number of random iterations (default 100).
# - seed:         base PRNG seed; iteration k uses seed+k for
#                 independent replay (default 1).
# - max-shrinks:  cap on the Stage-5 reducer's search depth
#                 (default 200; ignored before Stage 5).
def PropertySpec refine Record [
  name:String
  gen:List
  property:List
  runs:Integer
  seed:Integer
  max-shrinks:Integer
]

# Per-property outcome. The shrunk-* fields are populated by the
# Stage-5 reducer; Stage 3 mirrors the raw failing input there.
def PropertyResult refine Record [
  name:String
  ok:Boolean
  runs:Integer
  failing-input:Any
  shrunk-input:Any
  shrunk-source:String
  shrunk-cost:Integer
  error:Any
]

# Per-case outcome recorded by the runner.
def TestResult refine Record [
  name:String
  path:List
  ok:Boolean
  expected:Any
  actual:Any
  error:Any
  duration-ms:Integer
]

# ============================================================
# Helpers to construct specs declaratively
# ============================================================

# The constructors below deliberately declare [Map], NOT their record
# types (NUR069): the none-vs-Any half of that record is resolved (an
# Any field admits none at every boundary now), but the CallBoru
# asymmetry stands — a module fn's return is never checked interpreted
# (trim-only) while the compiled RET enforces the declared pattern, so
# ANY record annotation here would raise compiled-only errors for a
# non-conforming return the interpreter passes through. Check-mode
# narrowing does not need the annotation: the body residual (make's
# schema carrier / check-prop's shape ReturnsFn) surfaces through the
# declared bare Map on the plain pass (BuildFnBodyReturnsFn's
# record-residual rule, NUR068).
# (named test-case internally: 'case' is a core word and module
# preambles cannot redefine builtins; the export key stays 'case'.)
def test-case fn [[out:Any in:List name:String] [Map] [
  make TestCase {name:name in:in out:out}
]]

def spec fn [[cases:List subject:Any name:String] [Map] [
  make TestSpec {name:name subject:subject cases:cases subs:[]}
]]

def spec-with-subs fn [[subs:List cases:List subject:Any name:String] [Map] [
  make TestSpec {name:name subject:subject cases:cases subs:subs}
]]

# prop is a Go native constructor — see test-prop in the natives
# table. The bodies are NoEvalArgs-protected at the native boundary
# so list literals like [0 100 r.int] survive intact.

# run-property destructures a PropertySpec map and dispatches the
# Go-side check-prop driver. Returns the PropertyResult map.
#
# The gen/property fields are stored Quoted in the map (set by
# Test.prop), so a plain map.get retrieval preserves them as data
# rather than triggering auto-eval as they cross fn boundaries.
#
# Uses FORWARD form for the test-check-prop call so each arg fills
# the corresponding sig position directly (sig[0]=String, sig[1..2]
# =List, sig[3..5]=Integer).
def run-property fn [[| p:PropertySpec] [Map] [
  test-check-prop
    (p get "name")
    (p get "gen")
    (p get "property")
    (p get "runs")
    (p get "seed")
    (p get "max-shrinks")
]]

# ============================================================
# Pure-Boru spec runner
# ============================================================
# run-spec invokes each case's subject with the case inputs, compares
# the result to the case's "out" via deep equality, and records the
# outcome through test-record (Go). Sub-specs run recursively under a
# describe scope so their results inherit the parent spec name in the
# path column.

def run-case fn [[| subject:(Atom tor String) c:TestCase] [] [
  def in quote (c get "in")
  def expected (c get "out")
  def actual (in subject test-invoke)
  def matched (expected actual deq)
  0 None actual expected matched [] (c get "name") test-record
]]

def run-cases fn [[| subject:(Atom tor String) cases:List] [] [
  for (cases size) [
    def _i i
    def c (cases _i get)
    c subject run-case
  ] end
]]

def run-spec fn [[| s:TestSpec] [] [
  [
    def subject (s get "subject")
    def cases quote (s get "cases")
    def subs quote (s get "subs")
    cases subject run-cases
    for (subs size) [
      def _i i
      def subspec (subs _i get)
      subspec run-spec
    ] end
  ] (s get "name") test-describe
]]

# ============================================================
# Exports
# ============================================================

export "Test" {
  # types
  TestCase:        TestCase
  TestSet:         TestSet
  TestSpec:        TestSpec
  TestResult:      TestResult
  PropertySpec:    PropertySpec
  PropertyResult:  PropertyResult

  # spec constructors
  case:           test-case/v
  spec:           spec/v
  spec-with-subs: spec-with-subs/v
  prop:           test-prop/v

  # imperative API (Go)
  describe:    test-describe/v
  test:        test-test/v
  it:          test-test/v
  check-prop:  test-check-prop/v
  skip:        test-skip/v

  # accumulated results
  results:    test-results/v
  summary:    test-summary/v
  report:     test-report/v
  reset:      test-reset/v
  fail-count: test-fail-count/v

  # line coverage of a module-under-test
  cover:      test-cover/v
  coverage:   test-coverage/v

  # spec runner
  run-spec:     run-spec/v
  run-property: run-property/v
  invoke:       test-invoke/v
}

export "Assert" {
  equal:      assert-equal/v
  not-equal:  assert-not-equal/v
  ok:         assert-ok/v
  throws:     assert-throws/v
  match:      assert-match/v
}

`
