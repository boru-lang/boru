package modules

import (
	"fmt"

	"github.com/boru-lang/boru/lang/go/native"
	"github.com/boru-lang/boru/lang/go/policy"
)

// resolveFn is the module resolver used by sub-engines. It's
// initialised in init() to break the package-level cycle between
// the modules map and BuildVMModule → Resolve.
var resolveFn func(name string, parent *native.Registry) (native.ModuleDesc, error)

func init() {
	resolveFn = Resolve
}

// BuildVMModule creates the "boru:vm" native module. The module
// exposes one canonical operation — run code in a sub-engine with
// an explicit policy — and a small surface of conveniences:
//
//	Vm.run          code             # default sandbox profile
//	Vm.run-with     code policy-map  # explicit policy as data
//	Vm.run-sandbox  code             # built-in 'sandbox' profile
//	Vm.run-compute  code             # built-in 'compute' profile
//	Vm.parse        code             # → quoted token list (no run)
//	Vm.check        code             # → { ok errors warnings diagnostics }
//
// Each handler:
//
//  1. Resolves the policy (built-in name or constructed from the
//     supplied map).
//  2. Validates attenuation: child must be a subset of the parent
//     engine's effective policy. The vm word cannot grant itself
//     anything the parent doesn't already have.
//  3. Constructs a fresh native.Registry with the resolved policy.
//  4. Parses the source via parent.ParseFunc (single parser).
//  5. Runs it through a fresh engine.
//  6. Returns the last value on the residual stack to the parent.
func BuildVMModule(parent *native.Registry) (native.ModuleDesc, error) {
	if parent.ParseFunc == nil {
		return native.ModuleDesc{}, fmt.Errorf("vm: parser not configured")
	}

	// Construct a sub-registry for the module's words. Module exports
	// are dispatched through this sub-registry; the parent's policy
	// is consulted for module.call gating elsewhere.
	subReg, err := newModuleRegistry("boru:vm", vmNatives(parent))
	if err != nil {
		return native.ModuleDesc{}, err
	}

	// vm-run-with takes code + policy map. Its wrapper uses TAny params
	// (rather than the underlying TString + TMap signature) because the
	// dotted-form dispatch via `Vm.run-with` matches against the FnDef's
	// own signature, and we want the dotted form to dispatch against
	// any-typed args (the native validates types itself). It is the one
	// stack-only wrapper (BarrierPos 0).
	runWithSig := typeSig([]*native.Type{native.TAny, native.TAny}, native.TAny)
	runWithSig.stackOnly = true

	exports := native.NewOrderedMap()
	exports.Set("run", makeTypedFnDef("vm-run", subReg, native.TAny, native.TString))
	exports.Set("run-with", makeWrapFnDef("vm-run-with", subReg, runWithSig))
	exports.Set("run-sandbox", makeTypedFnDef("vm-run-sandbox", subReg, native.TAny, native.TString))
	exports.Set("run-compute", makeTypedFnDef("vm-run-compute", subReg, native.TAny, native.TString))
	exports.Set("parse", makeTypedFnDef("vm-parse", subReg, native.TAny, native.TString))
	exports.Set("check", makeTypedFnDef("vm-check", subReg, native.TAny, native.TString))

	return moduleDesc(parent, "Vm", subReg, exports), nil
}

// vmNatives builds the NativeFunc slice for the vm module. Defined
// as a closure-returning function because the handlers need to
// reference parent for policy / parser access.
func vmNatives(parent *native.Registry) []native.NativeFunc {
	return native.SideEffecting([]native.NativeFunc{
		{
			Name: "vm-run",
			Signatures: []native.Signature{{
				Args: []*native.Type{native.TString},
				Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					code, err := args[0].AsConcreteString()
					if err != nil {
						return nil, err
					}
					pol, err := s7aPolicyLoad("sandbox")
					if err != nil {
						return nil, fmt.Errorf("running Vm.run: load sandbox: %w", err)
					}
					return runInSubEngine(parent, code, pol)
				}),
				Returns:    []*native.Type{native.TAny},
				BarrierPos: -1,
			}},
		},
		{
			Name: "vm-run-sandbox",
			Signatures: []native.Signature{{
				Args: []*native.Type{native.TString},
				Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					code, err := args[0].AsConcreteString()
					if err != nil {
						return nil, err
					}
					pol, err := s7aPolicyLoad("sandbox")
					if err != nil {
						return nil, fmt.Errorf("running Vm.run-sandbox: %w", err)
					}
					return runInSubEngine(parent, code, pol)
				}),
				Returns:    []*native.Type{native.TAny},
				BarrierPos: -1,
			}},
		},
		{
			Name: "vm-run-compute",
			Signatures: []native.Signature{{
				Args: []*native.Type{native.TString},
				Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					code, err := args[0].AsConcreteString()
					if err != nil {
						return nil, err
					}
					pol, err := s7aPolicyLoad("compute")
					if err != nil {
						return nil, fmt.Errorf("running Vm.run-compute: %w", err)
					}
					return runInSubEngine(parent, code, pol)
				}),
				Returns:    []*native.Type{native.TAny},
				BarrierPos: -1,
			}},
		},
		{
			// vm-parse — boru source → inspectable structure, a step
			// below Vm.run: parse and return the token/value sequence
			// as a QUOTED list (it never auto-evaluates), without
			// running anything. The element shapes are the engine's
			// own parse values (words, literals, structural markers)
			// and are implementation-defined for now — see
			// design/legacy/PARSING.10.ignore §3. Parse errors raise
			// [boru/parse_error] with the same message the CLI prints.
			Name: "vm-parse",
			// Pure parse of the literal (see Debug.parse): the dry-pass
			// mirror flags top-level malformed source at check time.
			Signatures: func() []native.Signature {
				h := func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
					code, err := args[0].AsConcreteString()
					if err != nil {
						return nil, err
					}
					tokens, perr := parent.ParseFunc(code)
					if perr != nil {
						return nil, r.BoruError("parse_error",
							fmt.Sprintf("Vm.parse: %v", perr), "Vm.parse")
					}
					lst := native.NewList(tokens)
					native.SetQuoted(&lst, true)
					return []native.Value{lst}, nil
				}
				return []native.Signature{{
					Args:       []*native.Type{native.TString},
					Impl:       native.Go(h),
					ReturnsFn:  native.DryPassReturns(h, native.TList),
					Returns:    []*native.Type{native.TList},
					BarrierPos: -1,
				}}
			}(),
		},
		{
			Name: "vm-run-with",
			Signatures: []native.Signature{{
				Args: []*native.Type{native.TString, native.TMap},
				Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					code, err := args[0].AsConcreteString()
					if err != nil {
						return nil, err
					}
					pol, err := policyFromMapValue(args[1])
					if err != nil {
						return nil, fmt.Errorf("running Vm.run-with: %w", err)
					}
					return runInSubEngine(parent, code, pol)
				}),
				Returns:    []*native.Type{native.TAny},
				BarrierPos: -1,
			}},
		},
		{
			// vm-check — static type-check WITHOUT running. Returns a result
			// map { ok, errors, warnings, diagnostics } describing what the
			// checker found; it never raises for ill-typed or malformed input
			// (a syntax error rides back as one error-severity diagnostic), so
			// callers get one uniform shape to inspect.
			Name: "vm-check",
			Signatures: []native.Signature{{
				Args: []*native.Type{native.TString},
				Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					code, err := args[0].AsConcreteString()
					if err != nil {
						return nil, err
					}
					res, err := checkInSubEngine(parent, code)
					if err != nil {
						return nil, err
					}
					return []native.Value{res}, nil
				}),
				Returns:    []*native.Type{native.TMap},
				ReturnsFn:  vmCheckReportReturns,
				BarrierPos: -1,
			}},
		},
	}, "vm-parse", "vm-check")
}

// vmCheckReportReturns surfaces the FIXED report shape the word always
// builds (checkResultValue)
// as record-schema carriers, so field reads (`(Vm.check src).ok`) narrow
// instead of ending dynamic(Any). All claims are dynamic (gradual);
// diagnostics stays a plain List and sites a plain Map — their interiors
// are value-dependent.
func vmCheckReportReturns(_ []native.Value, _ *native.Registry) []native.Value {
	fields := native.NewOrderedMap()
	fields.Set("ok", native.NewTypeLiteral(native.TBoolean))
	fields.Set("errors", native.NewTypeLiteral(native.TInteger))
	fields.Set("warnings", native.NewTypeLiteral(native.TInteger))
	fields.Set("diagnostics", native.NewTypeLiteral(native.TList))
	return []native.Value{native.NewDynamicCarrierValue(native.NewRecordType(fields))}
}

// runInSubEngine builds a fresh registry under pol, parses src via
// the parent's parse func, runs it, and returns the last residual
// stack value to the parent. The parent's policy is composed with
// pol so every check in the sub-engine must pass BOTH layers — the
// sub-engine cannot grant anything the parent denies, regardless
// of how pol's rules are written.
//
// This replaces the earlier rule-comparison RequireSubset check,
// which only inspected scope defaults and could be bypassed by a
// parent policy with default-allow plus a specific deny rule (see
// PR #99 review feedback). Composing structurally is sound because
// it routes every dispatch through both policies — there is no
// path for a child rule to lift a parent deny.
func runInSubEngine(parent *native.Registry, src string, pol policy.Policy) ([]native.Value, error) {
	effective := pol
	if parentPol := native.HostPolicy(parent); parentPol != nil {
		effective = policy.Compose(parentPol, pol)
	}
	subReg, err := newSubEngineRegistry(parent, effective)
	if err != nil {
		return nil, err
	}

	tokens, err := parent.ParseFunc(src)
	if err != nil {
		return nil, fmt.Errorf("vm: parse: %w", err)
	}
	subReg.Source = src
	// The composed policy is enforced per dispatch by the tree-walker's
	// policyGateWord.
	eng := native.NewTop(subReg)
	eng.SetSource(src)
	stack, err := eng.Run(tokens)
	if err != nil {
		// An `IO.exit` raised INSIDE the sub-engine must not escape as an
		// exit request: the driver at the top would honour it and terminate
		// the HOST process, so `Vm.run-sandbox` would be a sandbox escape —
		// the one thing it exists to prevent. Convert it to an ordinary
		// error at the boundary. The code survives in the message so the
		// caller can still see what the sub-program asked for; it just has
		// no authority to make it happen.
		if code, isExit := native.ExitCode(err); isExit {
			return nil, fmt.Errorf("vm: sub-program requested exit with code %d "+
				"(a sub-engine cannot exit its host)", code)
		}
		return nil, err
	}
	if len(stack) == 0 {
		return []native.Value{native.NewNone()}, nil
	}
	// Return the last value as the result on the parent stack.
	return []native.Value{stack[len(stack)-1]}, nil
}

// newSubEngineRegistry constructs a fresh sub-engine registry under pol,
// wired with the parent's parser, module resolver, and dynamic help, then
// marked ready. Shared by run / check so the two stay configured
// identically — only what they DO with the registry differs.
func newSubEngineRegistry(parent *native.Registry, pol policy.Policy) (*native.Registry, error) {
	subReg, err := newDefaultRegistryWithPolicy(pol)
	if err != nil {
		return nil, fmt.Errorf("vm: init sub-engine: %w", err)
	}
	subReg.SetParseFunc(parent.ParseFunc)
	// A coverage run armed on the parent records the sub-engine's rows too.
	subReg.InheritObserveHooks(parent)
	// Indirect through resolveFn (initialised in init()) so the
	// package-level modules map can reference BuildVMModule without
	// creating an initialisation cycle through Resolve.
	if resolveFn != nil {
		subReg.Modules.Resolver = resolveFn
	}
	native.EnableDynamicHelp(subReg)
	subReg.MarkReady()
	return subReg, nil
}

// checkInSubEngine runs src through the static type-checker in a fresh
// sub-engine WITHOUT executing it, and returns a result map
// { ok, errors, warnings, diagnostics }. Diagnostics are reported as data
// rather than raised, so an ill-typed or malformed program yields a
// well-formed result the caller can inspect.
//
// The check runs under the PARENT's effective policy (not the sandbox
// profile Vm.run defaults to): the useful question is "does this check out
// under MY capabilities", and it stays safe because the sub-engine can
// never exceed the parent's policy and check mode suppresses every
// side-effecting handler anyway (only def/import/type/macro run for real).
func checkInSubEngine(parent *native.Registry, src string) (native.Value, error) {
	subReg, err := newSubEngineRegistry(parent, native.HostPolicy(parent))
	if err != nil {
		return native.Value{}, err
	}
	tokens, perr := parent.ParseFunc(src)
	if perr != nil {
		// A syntax error is itself a check finding, not a raised error:
		// surface it as one error-severity diagnostic so callers get the
		// same shape they get for a type error.
		return checkResultValue(false, []native.CheckDiagnostic{{
			Code: "parse_error", Detail: perr.Error(), Severity: native.SeverityError,
		}}), nil
	}
	subReg.Source = src
	defer subReg.Check.Begin()()
	eng := native.NewTop(subReg)
	eng.SetSource(src)
	_, runErr := eng.Run(tokens)
	subReg.RescueForwardRefDiagnostics()
	subReg.Check.EmitUnusedDefDiagnostics()

	diags := subReg.Check.Diagnostics
	// A hard check error that left no diagnostic still reports ok:false with
	// a synthesized entry so the result is never silently empty.
	if runErr != nil && len(diags) == 0 {
		diags = []native.CheckDiagnostic{{
			Code: "check_error", Detail: runErr.Error(), Severity: native.SeverityError,
		}}
	}
	ok := runErr == nil
	for _, d := range diags {
		if d.Severity == native.SeverityError {
			ok = false
		}
	}
	return checkResultValue(ok, diags), nil
}

// checkResultValue builds the { ok, errors, warnings, diagnostics } map
// Vm.check returns. Each diagnostic is a map
// { code, detail, word, row, col, severity }.
func checkResultValue(ok bool, diags []native.CheckDiagnostic) native.Value {
	errors, warnings := 0, 0
	list := make([]native.Value, 0, len(diags))
	for _, d := range diags {
		switch d.Severity {
		case native.SeverityError:
			errors++
		case native.SeverityWarning:
			warnings++
		}
		dm := native.NewOrderedMap()
		dm.Set("code", native.NewString(d.Code))
		dm.Set("detail", native.NewString(d.Detail))
		dm.Set("word", native.NewString(d.Word))
		dm.Set("row", native.NewInteger(int64(d.Row)))
		dm.Set("col", native.NewInteger(int64(d.Col)))
		dm.Set("severity", native.NewString(severityString(d.Severity)))
		list = append(list, native.NewMap(dm))
	}
	m := native.NewOrderedMap()
	m.Set("ok", native.NewBoolean(ok))
	m.Set("errors", native.NewInteger(int64(errors)))
	m.Set("warnings", native.NewInteger(int64(warnings)))
	m.Set("diagnostics", native.NewList(list))
	return native.NewMap(m)
}

// severityString renders a CheckSeverity for the result map, mapping the
// empty (unset) severity to "info" — the same default lang.CheckSummary
// applies when tallying.
func severityString(s native.CheckSeverity) string {
	if s == "" {
		return "info"
	}
	return string(s)
}

// policyFromMapValue converts a boru Map value (as received in a
// Vm.run-with arg) into a policy.Policy.
func policyFromMapValue(v native.Value) (policy.Policy, error) {
	if !native.IsConcrete(&v) {
		return nil, fmt.Errorf("policy map cannot be a type literal or carrier")
	}
	if _, err := native.RequireConcreteMap(v, "Vm.run-with policy"); err != nil {
		return nil, err
	}
	// Convert the boru map to a generic map[string]any for the policy loader.
	// RequireConcreteMap has already established that v is a concrete map
	// backed by an OrderedMap payload, so native.ValueToAny always yields a
	// map[string]any here (see native.valueToAny's TMap arm) — the assertion
	// cannot fail, so no unreachable error arm is left to guard (ADR-005).
	asMap, _ := native.ValueToAny(v).(map[string]any)
	return policy.FromMap(asMap)
}
