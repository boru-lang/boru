package lang

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
	eng "github.com/boru-lang/boru/eng/go"
	parser "github.com/boru-lang/boru/parser/go"

	"github.com/boru-lang/boru/lang/go/capabilities"
	"github.com/boru-lang/boru/lang/go/modules"
	"github.com/boru-lang/boru/lang/go/native"
	"github.com/boru-lang/boru/lang/go/policy"

	udk "github.com/voxgig/udk/go"
)

// Policy is the public alias for the permissions policy interface.
// Callers construct one via policy.Load / LoadFile / LoadInline /
// LoadAuto / FromMap and pass it through Options.
type Policy = policy.Policy

// FileOps is the interface for file system operations used by read/write words.
type FileOps = capabilities.FileOps

// Format handles encoding and decoding file content for a specific format.
type Format = native.Format

// Type represents a boru type such as "string", "number/integer", or "any".
// Use NewType to create types from slash-separated paths.
type Type = native.Type

// Value is a typed entry on the boru stack.
type Value = native.Value

// Signature describes one way a function can be called.
// Args lists the types the word needs, ordered deepest-first (Args[0] = deepest
// on the stack, Args[last] = top of the stack for prefix matching).
//
// Handler receives the matched args, the current context map, the resolved
// stack (only for FullStack signatures), and the registry. Most handlers
// only use the first parameter and ignore the rest with _.
type Signature = native.Signature

// Well-known boru types for use in Signature definitions.
var (
	TAny            = native.TAny
	TScalar         = native.TScalar
	TString         = native.TString
	TNumber         = native.TNumber
	TInteger        = native.TInteger
	TFloat          = native.TFloat
	TBoolean        = native.TBoolean
	TNode           = native.TNode
	TAtom           = native.TAtom
	TList           = native.TList
	TMap            = native.TMap
	TTable          = native.TTable
	TRecord         = native.TRecord
	TResource       = native.TResource
	TResourceEntity = native.TResourceEntity
)

// NewType creates a *Type from a slash-separated path (e.g. "string/proper",
// "number/integer"). Use this for custom or hierarchical types.
var NewType = native.NewType

// NewString creates a string Value.
var NewString = native.NewString

// NewInteger creates a number/integer Value.
var NewInteger = native.NewInteger

// NewBoolean creates a boolean Value.
var NewBoolean = native.NewBoolean

// NewList creates a list Value from a slice of Values.
var NewList = native.NewList

// NewMap creates a map Value from an OrderedMap.
var NewMap = native.NewMap

// NewAtom creates an atom Value from a bare name.
var NewAtom = native.NewAtom

// NewTypeLiteral creates the type-literal Value for a *Type — e.g. as an
// alternative for DefineEnum, or a value standing for a type.
var NewTypeLiteral = native.NewTypeLiteral

// NewMemFileOps creates an in-memory file system for testing.
func NewMemFileOps() *capabilities.MemFileOps {
	return capabilities.NewMem()
}

// HostFileOps returns the file system currently installed on the instance —
// the real OS-backed one unless a host or policy replaced it. Callers layering
// an overlay need it as the lower layer.
func (a *Boru) HostFileOps() FileOps {
	return native.HostFileOps(a.registry)
}

// Options configures a boru instance.
type Options struct {
	// Registry is a string identifier for the registry to use.
	Registry string
	// Seed sets the random seed for ID generation.
	// If zero, the current time is used.
	Seed int64
	// Policy is the optional permissions profile applied to this
	// instance. Nil means "no permissions configured" — the engine
	// and every capability wrapper treat that as allow-everything,
	// preserving the historical default. When set, the policy is
	// installed before host capabilities so SetHostX hooks can
	// auto-wrap or skip-install per the profile.
	Policy policy.Policy
	// Tape bounds the execution tape's growth (initial size, max grows,
	// growth factor). The zero value uses the engine defaults. Set via
	// the CLI's --options flag (e.g. `--options tape:initial:65536`) or
	// directly by a host.
	Tape TapeOptions
	// ScriptArgs is the script's positional-argument vector, surfaced to
	// programs as `IO.args` (a List of Strings). The CLI passes
	// everything after the script path; nil leaves the slot uninstalled,
	// which IO.args renders as an empty list.
	ScriptArgs []string
	// Env is the environment view surfaced as IO.env. Nil installs none,
	// and IO.env then reports every name as unset — the runtime never
	// reads the real process environment unless a host hands it over.
	// The CLI installs capabilities.OSEnvOps; tests and the spec runner
	// install capabilities.MapEnvOps for determinism.
	Env capabilities.EnvOps
	// Streams is the terminal probe surfaced as IO.is-tty. Nil installs none,
	// and every stream then answers false — the runtime never asks the
	// operating system what it is attached to unless a host hands over a
	// probe. The CLI installs capabilities.OSStreamProbe; tests install
	// capabilities.FixedStreamProbe, which is the only way the "yes, a
	// terminal" arm is reachable in a suite whose streams are redirected.
	Streams capabilities.StreamProbe
	// Steps caps evaluation: the interpreter's Run loop, a single
	// paren-group evaluation, and the VM's step counter. Zero uses the
	// engine defaults (core.DefaultStepLimit / DefaultSubStepLimit). Set
	// via the CLI's --options flag (e.g. `--options steps:50000000`) when
	// a legitimately long computation trips the default ceiling, or
	// downward to bound an untrusted program.
	Steps int
}

// TapeOptions configures the execution tape's bounded growth — see
// eng/go/tape.go. InitialSize 0 derives from the program; MaxGrows 0 and
// GrowthFactor 0 use the defaults (7 and 2.7).
type TapeOptions = native.TapeConfig

// boru is an independent boru execution instance.
// Each instance has its own state (set/get storage is isolated).
// Create multiple instances with New() for independent execution contexts.
//
// boru is not safe for concurrent use. (*Boru).Run and (*Boru).Check both
// mutate the underlying Registry (source pointer, def/type stacks, check
// state) and must not be called from multiple goroutines simultaneously
// on the same instance. Use one instance per goroutine for parallel work.
type Boru struct {
	registry *native.Registry
	options  Options
	manager  *udk.UniversalManager
}

// New creates a new boru instance with built-in functions.
// An optional Options value may be provided to configure the instance.
func New(opts ...Options) (*Boru, error) {
	var o Options
	if len(opts) > 0 {
		o = opts[0]
	}

	if o.Seed != 0 {
		native.SetIDSeed(o.Seed)
	}

	reg, err := newDefaultRegistryWithPolicy(o.Policy)
	if err != nil {
		return nil, err
	}
	reg.TapeConfig = o.Tape
	reg.StepLimit = o.Steps
	if o.Env != nil {
		native.SetHostEnvOps(reg, o.Env)
	}
	if o.Streams != nil {
		native.SetHostStreamProbe(reg, o.Streams)
	}
	if o.ScriptArgs != nil {
		native.SetHostScriptArgs(reg, o.ScriptArgs)
	}
	reg.SetParseFunc(parser.Parse)
	modules.InstallResolver(reg)

	um := udk.NewUniversalManager(map[string]any{
		"registry": o.Registry,
	})

	reg.Manager = um

	// Enable dynamic help generation for functions registered after this point.
	native.EnableDynamicHelp(reg)
	reg.MarkReady()

	return &Boru{registry: reg, options: o, manager: um}, nil
}

func init() {
	// Vm.run's sub-engine runner (modules/vm.go CompiledSubRun): the module
	// package cannot import lang, so the compiled-by-default entry point is
	// injected here — the same contract as the public Run (explicit armed
	// interpreter fallback on compile_failed).
	modules.CompiledSubRun = func(reg *native.Registry, src string) ([]native.Value, error) {
		a := &Boru{registry: reg}
		vals, _, _, err := a.RunAutoValues(src)
		var refused *BoruError
		if errors.As(err, &refused) && refused.Code == "compile_failed" {
			disarm := a.ArmRuntimeStamping()
			vals, err = a.RunInterpValues(src)
			disarm()
		}
		return vals, err
	}
}

// NewFromRegistry wraps an ALREADY-WIRED registry in a *Boru instance so a
// host with a long-lived registry of its own — the REPL, a service — can use
// the compiled-by-default entry points (RunAutoValues / RunCompiledReason)
// over it. The caller owns the wiring (parse func, module resolver, Manager,
// Output, policy): nothing is installed or re-installed here, and the zero
// Options apply. Plan Phase 2 prerequisite (entry-point routing).
func NewFromRegistry(reg *native.Registry) (*Boru, error) {
	if reg == nil {
		return nil, fmt.Errorf("NewFromRegistry: nil registry")
	}
	return &Boru{registry: reg}, nil
}

// Options returns the Options the instance was created with.
func (a *Boru) Options() Options {
	return a.options
}

// NativeRegistry returns the live *native.Registry backing this instance.
// Intended for tooling that needs to introspect or serve the running
// runtime's state — notably the debug-attach server (lang/go/debugserve),
// which wraps a registry behind authenticated HTTP introspection. Most
// callers should use the higher-level Run/Check API instead; this is the
// escape hatch for host-level inspection tools.
func (a *Boru) NativeRegistry() *native.Registry {
	return a.registry
}

// Policy returns the policy installed on this instance, or nil if
// none was configured. Equivalent to a.Options().Policy but reads
// the live capability slot, so it reflects any subsequent
// SetHostPolicy calls (rare; intended for tooling).
func (a *Boru) Policy() Policy {
	return native.HostPolicy(a.registry)
}

// Check parses the source and runs it through the engine in static
// type-check mode. Literals are stripped to carrier values (type-only)
// and signature handlers are replaced by carrier return propagation
// driven by Signature.Returns. The actual runtime dispatch, matching,
// and forward-collection machinery is reused verbatim so checker and
// runtime stay in absolute parity.
//
// The returned CheckResult holds the residual carrier stack (as type
// path strings) and any diagnostics the checker collected.
// SetStrictCheck toggles STRICT check mode for subsequent Check calls:
// every committed dispatch over a dynamic operand emits a non-gating
// dynamic_dispatch info diagnostic, making the gradual frontier loud —
// the Typed-Racket-style migration surface
// (design/legacy/checker-accuracy-review.10.ignore). Persistent on the instance
// until toggled off.
func (a *Boru) SetStrictCheck(on bool) {
	if a.registry.Check.Strict != on {
		// Fn-body summaries memoised under the OTHER strictness suppress
		// re-analysis on a reused instance (a bare Begin keeps them), so a
		// body's per-dispatch dynamic_dispatch advisories would never
		// surface after the toggle — drop the memo so the next Check
		// re-analyses under the new mode.
		a.registry.Check.FnSummaries = nil
		a.registry.Check.FnInflight = nil
	}
	a.registry.Check.Strict = on
}

func (a *Boru) Check(src string) (CheckResult, error) {
	values, err := parser.Parse(src)
	if err != nil {
		return CheckResult{}, err
	}

	a.registry.Source = src
	defer a.registry.Check.Begin()()
	native.ResetModuleExportGrowth(a.registry)
	native.ResetCheckFnCarrierBinds(a.registry)

	eng := native.NewTop(a.registry)
	eng.SetSource(src)
	result, err := eng.Run(values)
	// Drop fn-body forward-reference false positives (the name is
	// defined by now), then emit unused-def warnings — both need the
	// fully-populated end-of-pass state.
	native.RunPendingFnBodyChecks(a.registry)
	a.registry.RescueForwardRefDiagnostics()
	a.registry.Check.EmitUnusedDefDiagnostics()
	if err != nil {
		return CheckResult{Diagnostics: a.registry.Check.Diagnostics}, err
	}

	stack := make([]string, len(result))
	for i, v := range result {
		if v.Dynamic {
			// Surface the gradual modality in the residual stack so a
			// dynamic carrier is distinguishable from a strict one.
			stack[i] = "dynamic(" + v.Parent.Leaf() + ")"
		} else {
			stack[i] = v.Parent.Leaf()
		}
	}

	// Diagnostics carry the source position stamped by the parser onto
	// the offending value (Row 0 means the engine could not attribute the
	// diagnostic to a source token). We do not guess a location by
	// text-searching the source — a guessed position is wrong whenever the
	// word appears more than once.
	diags := a.registry.Check.Diagnostics
	var summary CheckSummary
	for i := range diags {
		switch diags[i].Severity {
		case SeverityError:
			summary.Errors++
		case SeverityWarning:
			summary.Warnings++
		default:
			summary.Infos++
		}
	}
	return CheckResult{Stack: stack, Diagnostics: diags, Summary: summary}, nil
}

// Program is the bytecode unit the compile pass produces — re-exported
// from the engine kernel for host callers (Stage 1 of
// design/legacy/boru-bytecode-plan.0.ignore).
type Program = compiler.Program

// StampEvent is one detached-stamp attempt (re-exported for hosts and the
// CLI's -compile-report surface).
type StampEvent = core.StampEvent

// StampReport returns the detached-stamp attribution recorded on this
// instance's registry (design/legacy/RUNTIME-STAMPING.0.ignore Phase 5): one event per
// stamp ATTEMPT — runtime-constructed codec fns, service handlers, and
// module fns — with the refusal reason when the compile declined. Nil when
// runtime stamping was never armed (a plain Run / -no-compile execution).
func (a *Boru) StampReport() []core.StampEvent {
	return a.registry.StampEvents()
}

// InterpEntry / BailEvent are the observability-seam event types (eng
// interp_entry.go), re-exported for the frontier test suite.
type InterpEntry = core.InterpEntry

// BailEvent is one designed VM defer-to-interpreter (see InterpEntry).
type BailEvent = core.BailEvent

// ArmInterpEntryHook forwards to the registry's interpreter-entry
// observability seam (eng interp_entry.go — a TEST seam, not API): fn fires
// on every entry into tree-walking machinery until the returned disarm func
// runs.
func (a *Boru) ArmInterpEntryHook(fn func(InterpEntry)) func() {
	return a.registry.ArmInterpEntryHook(fn)
}

// ArmRuntimeBailHook forwards to the registry's runtime-bail observability
// seam (eng interp_entry.go — a TEST seam, not API): fn fires on every
// designed VM defer-to-interpreter until the returned disarm func runs.
func (a *Boru) ArmRuntimeBailHook(fn func(BailEvent)) func() {
	return a.registry.ArmRuntimeBailHook(fn)
}

// RegionOracleEvent is one execution of the compiled lane's COLLECT oracle
// (core.RegionOracleEvent), re-exported for the corpus lane.
type RegionOracleEvent = core.RegionOracleEvent

// ArmRegionOracleHook forwards to the registry's region-oracle observability
// seam (a TEST seam, not API): fn fires on every OpCollect a program
// compiled under compiler.RegionOracle executes, until the returned disarm
// func runs.
func (a *Boru) ArmRegionOracleHook(fn func(RegionOracleEvent)) func() {
	return a.registry.ArmRegionOracleHook(fn)
}

// ArmRuntimeStamping arms detached fn-unit stamping (compiler.StampDetachedFn) on
// this instance and returns the restoring disarm func. RunCompiled /
// RunAutoValues arm it themselves for the duration of the call; this is the
// caller-side half of the Stage-J explicit-fallback contract: a host or CLI
// surface that receives compile_failed and chooses to run RunInterp itself
// keeps the compiled mode's callback contract — runtime-constructed callbacks
// (service handlers, codec fns) still compile to VM units at their store
// sites — by arming around the fallback run. The returned func restores the
// prior state (a no-op when the registry was already armed), so nesting is
// safe. A policy-gated registry stays sound under arming: StampDetachedFn
// itself refuses when a word policy is installed, exactly like CompileCheck.
func (a *Boru) ArmRuntimeStamping() func() {
	if a.registry.RuntimeStampingEnabled() {
		return func() {}
	}
	a.registry.EnableRuntimeStamping()
	return func() { a.registry.DisableRuntimeStamping() }
}

// CompileCheck runs the source through the checker with the bytecode
// recording pass enabled (Stage 1: straight-line, monomorphic native
// calls only) and linearises the trace into a Program. When the
// source contains a construct Stage 1 cannot lower — control flow,
// user fns, polymorphic or dynamic dispatch, compile-time words —
// the Program is nil and reason names the first offender; the
// CheckResult is valid either way.
func (a *Boru) CompileCheck(src string) (*Program, string, CheckResult, error) {
	// Policy-gated registries COMPILE (the 2026-07-15 lift, user-authorized):
	// every named VM dispatch consults the SAME WordChecker the
	// interpreter's policyGateWord runs (vmContext.gateWord — CALL_NATIVE,
	// CALL_USER/TAIL_CALL_USER, both poly re-matchers, CALL_DYN_METHOD),
	// raising the identical permission error, so the 2026-07-13 bypass (a
	// "deny add" policy interpreted `1 add 2` to permission-denied but ran
	// it compiled to 3) is closed at the dispatch layer instead of by
	// refusing compilation. The check pass stays ungated, mirroring
	// policyGateWord's check-mode skip. Pinned by
	// policy_compiled_gate_test.go's compiled-vs-interpreted parity sweep.
	values, err := parser.Parse(src)
	if err != nil {
		return nil, "parse error", CheckResult{}, err
	}

	a.registry.Source = src
	// BeginCompilePass arms the shared compile-pass ritual (fresh
	// EmitState, Compiling flag, fn-memo drop) in one place.
	defer a.registry.Check.BeginCompilePass()()
	native.ResetModuleExportGrowth(a.registry)
	native.ResetCheckFnCarrierBinds(a.registry)

	engine := native.NewTop(a.registry)
	engine.SetSource(src)
	residual, runErr := engine.Run(values)
	native.RunPendingFnBodyChecks(a.registry)
	a.registry.RescueForwardRefDiagnostics()
	a.registry.Check.EmitUnusedDefDiagnostics()

	res := CheckResult{
		Diagnostics:              a.registry.Check.Diagnostics,
		FnCarrierReadSubstituted: a.registry.Check.FnCarrierReadSubstituted,
		BindLedger:               a.registry.Check.BindLedger,
	}
	if sites := a.registry.Check.Recorder().Sites(); len(sites) > 0 {
		res.SiteCounts = make(map[string]int, len(sites))
		for k, v := range sites {
			res.SiteCounts[k] = v
		}
	}
	if runErr != nil {
		return nil, "check error", res, runErr
	}
	for _, d := range res.Diagnostics {
		// Refuse only on MODEL-UNDERMINING findings (undefined_word,
		// no_signature, … — dispatch did not resolve, so the recording is
		// a guess). A RuntimeMirror error is a validation finding whose
		// model is exact — the program compiles and raises the identical
		// error at runtime (trap / VM RET / the same handler) — so it
		// must not cost compile coverage. A CAUGHT model-undermining
		// finding still undermines the model: the `do` body's contents
		// were recorded from the same guess (the runtime catches the
		// error, but the compiled region computes a different value), so
		// the caught downgrade must not slip it past this gate.
		if !d.RuntimeMirror && (d.Severity == SeverityError || d.CaughtAtRuntime) {
			return nil, "check diagnostics", res, nil
		}
	}
	// Some words are deliberately lenient in check mode but raise a
	// strict error at runtime (an orphan `gen [...]`, an `unpack` of a
	// missing key). The compiled stream IS the check pass, so it would
	// silently succeed where the interpreter errors. Such a word flags
	// the suppression; refuse to compile and let the interpreter raise
	// the real error on the fallback path.
	if a.registry.Check.SuppressedRuntimeError {
		return nil, "check-mode suppressed a runtime error (uncompilable)", res, nil
	}
	// A dispatch whose forward/stack split depended on a genuinely mixed
	// gradual carrier (e.g. `and 0 false not 0`, where `and`'s
	// Disjunct(Integer,Boolean) result makes `not` forward-collect in
	// check mode but stack-grab the concrete Boolean at runtime) cannot be
	// faithfully compiled — the static split diverges from the runtime
	// one. Refuse and fall back to the interpreter.
	if a.registry.Check.AmbiguousGradualSplit {
		return nil, "forward/stack split depends on a gradual operand (uncompilable)", res, nil
	}
	// Finalizing the recorded stream needs the CONCRETE EmitState: only it
	// holds the emitted program. compiler/go's init unconditionally installs
	// NewEmitStateHook to mint one, and lang links compiler transitively
	// through eng, so a non-EmitState recorder here means a host reassigned
	// the exported core hook. Refuse to compile rather than assert — a
	// failed assertion would panic, which ADR-005 forbids.
	es, isReal := a.registry.Check.Recorder().(*compiler.EmitState)
	if !isReal { //covergate:allow compiler's init always installs the *EmitState hook that eng links in, so only a host-swapped core.NewEmitStateHook reaches this belt (§compiler)
		return nil, "no bytecode recorder installed (uncompilable)", res, nil
	}
	prog, reason, ok := es.Finalize(residual)
	if !ok {
		return nil, reason, res, nil
	}
	return prog, "", res, nil
}

// SetFileOps replaces the file operations implementation used by read/write.
func (a *Boru) SetFileOps(ops FileOps) {
	native.SetHostFileOps(a.registry, ops)
}

// Clock is the time source used by temporal and random words —
// re-exported so hosts and tests can freeze it (capabilities.FixedClock)
// for reproducible runs.
type Clock = capabilities.Clock

// SetClock replaces the instance's clock.
func (a *Boru) SetClock(clk Clock) {
	native.SetHostClock(a.registry, clk)
}

// HTTPOps is the outbound HTTP transport capability used by boru:net's
// fetch — re-exported so hosts can pin TLS settings or stub the network
// in tests. TLSProfile is the resolved per-request TLS configuration
// handed to it.
type HTTPOps = capabilities.HTTPOps

// TLSProfile is the resolved TLS configuration for one outbound request.
type TLSProfile = capabilities.TLSProfile

// SetHTTPOps replaces the HTTP transport used by fetch. With none
// installed, fetch uses http.DefaultTransport.
func (a *Boru) SetHTTPOps(ops HTTPOps) {
	native.SetHostHTTPOps(a.registry, ops)
}

// ClientIdentity supplies a client certificate for mutual TLS.
// CertRequest is what the peer asked for during the handshake.
type ClientIdentity = capabilities.ClientIdentity

// CertRequest is the boru-facing projection of crypto/tls's
// CertificateRequestInfo, handed to a ClientIdentity per handshake.
type CertRequest = capabilities.CertRequest

// StaticIdentity builds a ClientIdentity from a PEM chain and key held
// in memory; FileIdentity reads them from host paths.
var (
	StaticIdentity = capabilities.StaticIdentity
	FileIdentity   = capabilities.FileIdentity
)

// RegisterClientIdentity makes a client certificate available to boru
// source under a name, for mutual TLS:
//
//	a.RegisterClientIdentity("acme", id)
//	// boru: Net.fetch {url: "https://…"  tls: {identity: acme/q}}
//
// The guest can SELECT an identity but can never read or construct one
// — the private key stays behind the ClientIdentity interface, which is
// why an HSM- or vault-backed credential works here unchanged. Which
// identity a program may actually present is a policy decision
// (network/client-cert), not the program's.
func (a *Boru) RegisterClientIdentity(name string, id ClientIdentity) {
	native.RegisterClientIdentity(a.registry, name, id)
}

// SetOutput replaces the writer used by print, help, and other output words.
func (a *Boru) SetOutput(w io.Writer) {
	a.registry.Output = w
}

// RegisterFormat adds or replaces a format in the format registry and maps
// any given file extensions (leading dot optional) to it. Formats are used
// by the read/write words via the {fmt:"name"} option and, for any mapped
// extensions, by `read` on a matching path.
func (a *Boru) RegisterFormat(name string, f Format, exts ...string) {
	if native.HostFormats(a.registry) == nil {
		native.SetHostFormats(a.registry, make(map[string]native.Format))
	}
	_ = native.RegisterFormat(a.registry, name, f, exts...)
}

// Register adds a named word with one or more signatures. Any sig
// whose BarrierPos is left at 0 is treated as forward-collecting:
// the engine collects arguments from tokens after the word before
// falling back to stack matching.
//
// Example — register a word "double" that doubles an integer
// (extra handler params are context, stack, and registry — use _ to ignore):
//
//	a.Register("double", lang.Signature{
//	    Args: []lang.Type{lang.TInteger},
//	    Handler: func(args []lang.Value, _ map[string]lang.Value, _ []lang.Value, _ *native.Registry) ([]lang.Value, error) {
//	        n := args[0].AsConcreteInteger()
//	        return []lang.Value{lang.NewInteger(n * 2)}, nil
//	    },
//	})
func (a *Boru) Register(name string, sigs ...Signature) {
	a.registry.Register(name, sigs...)
}

// RegisterNativeFunc installs a full NativeFunc (name + signatures) on the
// instance's registry. Convenience for callers that already hold a
// native.NativeFunc value (e.g. seeding the words that moved into loadable
// modules into a test instance without an explicit import).
func (a *Boru) RegisterNativeFunc(n native.NativeFunc) {
	a.registry.RegisterNativeFunc(n)
}

// DefineType installs a user type from a body Value by the SAME path the
// `def Name body` word uses (core.InstallType), and returns the minted
// type handle for use in Register'd signatures. This is the embedding-API
// counterpart of running `def`, but with the *Type handed back — closing
// the gap where an embedder could define a type in source yet never
// obtain its handle.
//
// `body` is an ordinary type body: a bare type literal (alias), a refine
// prefab (newtype), a disjunct (DefineEnum / NewDisjunct — union/enum), a
// negation, a record/object/schema body, etc. For a body expressed in boru
// syntax, use DefineTypeFromSource; for a membership rule expressed as a
// Go func, use DefineMemberType.
func (a *Boru) DefineType(name string, body Value) (*Type, error) {
	return a.registry.DefineType(name, body)
}

// DefineEnum installs `name` as the union/enumeration of the given
// alternatives — the embedding equivalent of `def Name (v0 tor v1 …)`.
// Alternatives may be concrete values (a closed enum) or type literals
// (a union). Returns the minted type handle.
func (a *Boru) DefineEnum(name string, alternatives ...Value) (*Type, error) {
	return a.registry.DefineEnum(name, alternatives...)
}

// DefineMemberType installs `name` as the type whose inhabitants are the
// concrete values satisfying member — a membership rule expressed as a Go
// func (the one case DefineType's body path cannot express). The name is
// bound (resolves in source and exports like a `def`-installed type) and
// the minted handle returned.
func (a *Boru) DefineMemberType(name string, parent *Type, member func(v Value) bool) (*Type, error) {
	return a.registry.DefineMemberType(name, parent, member)
}

// DefineTypeFromSource installs `name` with a body written in boru syntax
// — it runs `def Name <bodySource>` on the instance and returns the
// minted type handle. The most direct embedding form: the host writes the
// body exactly as it would in a script (e.g.
// DefineTypeFromSource("Point", "refine Record [x:Integer y:Integer]"))
// and gets the *Type back to thread into Register'd signatures.
func (a *Boru) DefineTypeFromSource(name, bodySource string) (*Type, error) {
	// `name` is composed into a `def` program, so it must be a plain type
	// identifier — never source. Reject anything else BEFORE running, so a
	// name like `X end <code>` cannot terminate the def and execute
	// trailing source against the registry. (`bodySource` is intentionally
	// source; `name` is not.) The clash / part checks still run inside
	// InstallType when the def executes.
	if !validTypeIdent(name) {
		return nil, errors.New("define type: invalid type name " + strconv.Quote(name) +
			" (must be a capitalised identifier of letters, digits, '_', '-', '/')")
	}
	if _, err := a.Run("def " + name + " " + bodySource); err != nil {
		return nil, err
	}
	if t := a.registry.LookupTypeName(name); t != nil {
		return t, nil
	}
	return nil, errors.New("define type " + name + ": installed but did not resolve")
}

// validTypeIdent reports whether name is a safe type identifier to splice
// into a `def` program: capitalised, and otherwise only letters, digits,
// and the path/word separators '_', '-', '/'. This excludes whitespace
// and every boru-significant character (quotes, parens, brackets, ';',
// 'end', …), so a name can never break out of the def-name position.
func validTypeIdent(name string) bool {
	if name == "" || name[0] < 'A' || name[0] > 'Z' {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9',
			c == '_', c == '-', c == '/':
		default:
			return false
		}
	}
	return true
}

// FnParam describes one parameter of a function signature — re-exported so
// hosts can declare a mini-language kind's stack Inputs (see MiniLangSpec).
type FnParam = native.FnParam

// Handler is the signature of a native word / mini-language handler:
// func(args, ctx, stack, registry) (results, error). args are delivered in
// signature order (args[0] = sig position 0).
type Handler = native.Handler

// MiniLangSpec describes a Go-implemented mini-language for NewMiniLangFn.
// The standard [src:String opts:Map] prefix is supplied automatically;
// declare only the extra stack Inputs, the Returns, and the Handler.
type MiniLangSpec = modules.MiniLangSpec

// NewMiniLangFn builds a mini-language Function VALUE from spec — the
// successor of the removed RegisterMiniLang: the mini kind namespace is
// fixed (built-in kinds only), so a Go-implemented mini-language is handed
// to programs as a value instead of a registered kind. Bind it with
// DefineValue (or export it from a module) and call it through the `mini`
// word's value form. The handler receives args[0]=src, args[1]=opts, and
// args[2..]=the declared Inputs.
//
// Example — an integer binary op whose operands name keys in opts:
//
//	m, _ := lang.NewMiniLangFn(lang.MiniLangSpec{
//	    Name:    "iop",
//	    Returns: []*lang.Type{lang.TInteger},
//	    Handler: iopHandler, // parses "x + y", applies to opts.x, opts.y
//	})
//	a.DefineValue("iop", m)
//	a.Run(`mini iop 'x + y' {x:10, y:2}`) // → 12 (the value form needs no import)
func NewMiniLangFn(spec MiniLangSpec) (Value, error) {
	return modules.NewMiniLangFn(spec)
}

// ParseLang is the type of a parse_<lang> parser function — the handler
// form of the standard parser signature. The framework resolves the source
// before the function runs, so it receives args[0]=source (a String) and
// args[1]=opts; it returns the parse result. Handlers passed to
// NewParseLangFn (ParseLangSpec.Handler) carry this type.
type ParseLang = modules.ParseLang

// ParseLangSpec describes a Go-implemented parser for NewParseLangFn. The
// standard [source:String opts:Map] prefix is supplied automatically and the
// source is resolved to a String before the handler runs; declare only the
// Returns and the Handler (a ParseLang).
type ParseLangSpec = modules.ParseLangSpec

// NewParseLangFn builds a ParseLang Function VALUE from spec — the
// successor of the removed RegisterParser: the parse kind namespace is
// fixed (built-in kinds only), so a Go-implemented parser is handed to
// programs as a value instead of a registered kind. Bind it with
// DefineValue (or export it from a module) and call it through the `parse`
// word's value form.
//
// Example — a calc parser that returns an AST instead of evaluating:
//
//	p, _ := lang.NewParseLangFn(lang.ParseLangSpec{
//	    Name:    "calc",
//	    Returns: []*lang.Type{lang.TMap},
//	    Handler: calcParseHandler, // 'x + y' → {op:'+', left:'x', right:'y'}
//	})
//	a.DefineValue("calc", p)
//	a.Run(`import "boru:parselang"  parse calc 'x + y'`) // → {op:'+' …}
func NewParseLangFn(spec ParseLangSpec) (Value, error) {
	return modules.NewParseLangFn(spec)
}

// NewFormatParserFn wraps a read Format's decoder as a ParseLang Function
// VALUE — the value-form successor of the removed RegisterFormatParser's
// parse side. Register the format with the read registry separately when it
// should also be reachable from `read`.
func NewFormatParserFn(name string, f native.Format) (Value, error) {
	return modules.NewFormatParserFn(name, f)
}

// EmitLangSpec describes a Go-implemented emitter for NewEmitLangFn. The
// standard [value:Any opts:Map] prefix is supplied automatically; declare
// only the Returns (nil → [String]) and the Handler.
type EmitLangSpec = modules.EmitLangSpec

// NewEmitLangFn builds an emitter Function VALUE from spec — the emit kind
// namespace is fixed (built-in kinds only), so a Go-implemented emitter is
// handed to programs as a value instead of a registered kind. Bind it with
// DefineValue (or export it from a module) and call it through the `emit`
// word's value form. The handler receives args[0]=value, args[1]=opts.
//
// Example — an uppercase debug emitter:
//
//	e, _ := lang.NewEmitLangFn(lang.EmitLangSpec{
//	    Name:    "up",
//	    Handler: upEmitHandler, // renders the value, uppercased
//	})
//	a.DefineValue("up", e)
//	a.Run(`emit up {a:1}`) // the value form needs no import
func NewEmitLangFn(spec EmitLangSpec) (Value, error) {
	return modules.NewEmitLangFn(spec)
}

// DefineValue binds name to v in the instance's definition table — the host
// twin of `def name <value>` (lowercase names; capitalised names are types —
// use DefineType). The canonical way to install a NewParseLangFn value so
// programs reach it by bare name (`parse myp '…'`).
func (a *Boru) DefineValue(name string, v Value) error {
	if name == "" {
		return fmt.Errorf("define value: name must not be empty")
	}
	if name[0] >= 'A' && name[0] <= 'Z' {
		return fmt.Errorf("define value %q: lowercase names bind values (capitalised names are types — use DefineType)", name)
	}
	// The host twin of `def` enforces def's word-name grammar too — a name
	// source boru cannot spell would install an unreachable binding.
	if err := native.ValidateWordName(name); err != nil {
		return fmt.Errorf("define value %q: %w", name, err)
	}
	native.InstallDef(a.registry, name, v)
	return nil
}

// (RegisterStackOnly was retired. To install a stack-only word, set
// `BarrierPos: 0` on each Signature and call `Register` — that's the
// canonical encoding of "this sig consumes its args from the prefix
// stack only.")

// SetSDK injects an SDK instance for the given spec name.
// Used in tests to provide a pre-configured SDK (e.g. test mode with mock data).
func (a *Boru) SetSDK(spec string, sdk any) {
	a.registry.SDKCache[spec] = sdk
}

// Run parses and executes a boru source string.
// The source may span multiple lines; newlines and tabs are treated as
// whitespace (equivalent to spaces).
//
// Returns the result stack as Go values:
//   - int64 for integers
//   - string for strings
//
// State from set/get persists across multiple Run calls on the same instance.
//
// Run executes COMPILED-BY-DEFAULT (Stage J, plan Phase 11 — landed
// 2026-07-15 once the flip attempt's surfaced divergences all closed
// natively: the fn-predicate transform family, the mini host-compile
// hook, the model-watch ledger race, and the cross-request def
// persistence OpBindGlobal fixed). A genuine whole-program refusal
// degrades gracefully: RunAutoValues returns compile_failed — a
// guarantee that no observable effect escaped — and Run performs the
// explicit interpreter fallback itself, with detached fn-unit stamping
// kept armed so stored callbacks still earn the VM path (the same
// contract the CLI surfaces implement). Callers that need the
// interpreter SPECIFICALLY — parity oracles, canonical-error rendering
// — must call RunInterp, which survives the flip as the explicitly-
// named tree-walker entry point.
func (a *Boru) Run(src string) ([]any, error) {
	out, _, _, err := a.RunCompiledReason(src)
	var refused *BoruError
	if errors.As(err, &refused) && refused.Code == "compile_failed" {
		disarm := a.ArmRuntimeStamping()
		out, err = a.RunInterp(src)
		disarm()
	}
	return out, err
}

// RunInterp parses and executes src on the TREE-WALKING INTERPRETER,
// unconditionally — never the bytecode VM. It is the differential oracle
// the compiled path is measured against (byte-identical values, errors,
// and output), and it survives Stage J's Run flip as the explicitly-named
// interpreter entry point.
func (a *Boru) RunInterp(src string) ([]any, error) {
	result, err := a.runValues(src)
	if err != nil {
		return nil, err
	}
	return convertResults(result), nil
}

// RunInterpValues is RunInterp without the host-value projection — the raw
// engine Values, for callers whose renderer needs the engine's own
// Value.String() (the REPL's per-line echo and its compile_failed
// fallback). Same contract as RunInterp: unconditionally the tree-walking
// interpreter, never the VM.
func (a *Boru) RunInterpValues(src string) ([]native.Value, error) {
	return a.runValues(src)
}

// runValues is Run without the host-value projection: the raw engine stack.
// The Value-returning entry points (RunAutoValues' fallback arms) need the
// unprojected Values so a host renderer (the REPL's v.String()) stays
// byte-identical with what the engine produced.
func (a *Boru) runValues(src string) ([]native.Value, error) {
	values, err := parser.Parse(src)
	if err != nil {
		return nil, err
	}

	a.registry.Source = src
	eng := native.NewTop(a.registry)
	eng.SetSource(src)
	return eng.Run(values)
}

// convertResults maps a residual engine stack to host-friendly Go
// values — the same projection Run has always applied.
func convertResults(result []core.Value) []any {
	out := make([]any, len(result))
	for i, v := range result {
		switch {
		case v.IsDepScalar():
			// DepScalar's Parent IS the base scalar (TInteger,
			// TString, …) post the type-collapse, so it would
			// otherwise hit the AsInteger/AsString branches below and
			// pull a zero value from the DepScalarInfo payload. Route
			// through String() to render as "(Integer gte 5)".
			out[i] = v.String()
		case native.IsBareTypeNode(v):
			// Type literal (e.g. `typeof x` result, a bare `Integer`
			// or user-minted Foo literal): render its type name
			// rather than trying to extract an Integer/String payload
			// that isn't there.
			out[i] = v.String()
		case v.Parent.ConformsTo(native.TInteger):
			n, _ := native.AsInteger(v)
			out[i] = n
		case v.Parent.ConformsTo(native.TString):
			s, _ := native.AsString(v)
			out[i] = s
		default:
			out[i] = v.String()
		}
	}
	return out
}

// RunCompiled executes src in compiled (bytecode) mode when the
// emitter can lower it, and SILENTLY falls back to the interpreter
// otherwise — the plan's opt-in contract: identical results either
// way, the flag only changes the execution engine. The second return
// reports which path ran (for tooling; never branch program logic on
// it).
//
// LIMITATION — the step budget is the one place this is NOT byte-for-byte
// transparent. The interpreter meters its DefaultStepLimit per tape token
// stepped; the VM meters the SAME cap per bytecode instruction. The compiled
// stream is leaner than the expanded token walk, so for any given program the
// VM reaches at least as far as the interpreter before the cap — the divergence
// is one-directional: a long-but-terminating computation that the interpreter
// would abort with evaluation_limit may COMPLETE under compilation; the reverse
// never happens (the VM does not spuriously raise evaluation_limit on a program
// the interpreter finishes). A genuine runaway trips evaluation_limit fast in
// both. So at the ceiling, Run and RunCompiled are observably different
// programs; everywhere below it they agree. TestStepBudgetNoSpuriousLimit pins
// the agreement on a long terminating loop; TestPropertyDifferential keeps the
// generated corpus well under the cap so the divergence never makes it flaky.
//
// Two boundaries qualify "identical results":
//
//   - Internal errors. A compiled-mode VM/lowering soundness assertion or a
//     recovered handler panic (taxonomy internal_error), and any non-Boru Go
//     error, are NOT surfaced: the run rolls back and re-executes on the
//     interpreter, so a latent compiler bug degrades to the correct result
//     rather than a raw failure (the differential gate's row-count floor still
//     catches the regression). EXCEPT when observable output already escaped:
//     rolling back cannot un-print, so a re-run would duplicate every effect
//     (the L-DUP class, design/legacy/VOXGIG-COMPILE-LEAVES.2.ignore). The effect fence
//     (eng effects.go, design/legacy/RUNTIME-INDEPENDENCE-COMPLETION-PLAN.0.ignore C1)
//     then PROPAGATES the internal_error, annotated with a run-with
//     --no-compile hint, instead of silently re-running.
//   - The step budget. The interpreter counts it per tape token stepped, the
//     VM per bytecode instruction, both capped at core.DefaultStepLimit. Only
//     iteration/recursion can approach that ceiling, and for those the compiled
//     stream is leaner than the expanded token stream — so the VM reaches at
//     least as far as the interpreter and never spuriously raises
//     evaluation_limit on a program the interpreter completes. The residual
//     deviation is benign and one-directional: a long computation the
//     interpreter reports as evaluation_limit may COMPLETE under compilation.
//     A genuine runaway trips evaluation_limit fast in both (the VM does not
//     fall back on it — that would only re-burn the same budget).
func (a *Boru) RunCompiled(src string) ([]any, bool, error) {
	out, ran, _, err := a.RunCompiledReason(src)
	return out, ran, err
}

// RunCompiledReason is RunCompiled with the whole-program compilation-refusal
// reason surfaced as a third return, for tooling (chiefly the CLI's performance
// warning) that needs to know WHY a run fell back to the interpreter. It runs
// identically to RunCompiled — same results, same ranCompiled, same error — and
// additionally reports:
//
//   - "" when the program ran on the VM (ranCompiled true), or when the fallback
//     was NOT a performance refusal: a statically-invalid program (a parse/check
//     error, or the "check diagnostics" sentinel — it would fail in both engines)
//     and a runtime soundness bailout (an internal_error re-run, a latent
//     compiler bug the differential gate catches, not a compilable-subset gap).
//   - the first offending construct otherwise: a GENUINE whole-program refusal
//     (CompileCheck returned a nil Program with no check error) whose program
//     was then silently re-run on the interpreter (design/COMPILABLE-SUBSET.md
//     §1). The answer is right and nothing in the run says the compile failed,
//     which is what makes the silence bad rather than harmless: a refusal is a
//     DEFECT owed a fix, so the CLI surfaces this reason as a warning.
func (a *Boru) RunCompiledReason(src string) ([]any, bool, string, error) {
	vals, ran, reason, err := a.RunAutoValues(src)
	if err != nil {
		return nil, ran, reason, err
	}
	return convertResults(vals), ran, reason, nil
}

// RunAutoValues is the compiled-by-default entry point returning the RAW
// engine Values — RunCompiledReason without the host-value projection
// (convertResults collapses Integer→int64/String→string, which cannot back
// a renderer that needs the engine's own Value.String() — the REPL's
// per-line echo and /stack). Identical semantics: same compiled/fallback
// arms, same fence, same reason contract (plan Phase 2's prerequisite for
// entry-point routing).
func (a *Boru) RunAutoValues(src string) ([]native.Value, bool, string, error) {
	// CompileCheck executes the program in check mode, so its
	// RunInCheckMode words (def/import/type/macro, the Test harness)
	// leave real side effects on the registry. The COMPILED path needs
	// those to persist (OpPushType resolves minted IDs; islands re-run
	// through a sub-engine over the same registry). But the interpreter
	// FALLBACK re-runs the whole source, so it must NOT see them or it
	// double-applies a re-mint / re-import / re-run Test spec. Snapshot
	// the mutable scopes before the check pass and roll them back on the
	// fallback path; keep them on the compiled path.
	snap := a.registry.SnapshotForCompile()
	// §6.5 rollback-and-replay needs no snapshot here: the compiled program
	// carries its own rollback base (Program.ReplayBase — captured by the
	// recorder at its first bind, before the check pass performs a
	// transition), and RunProgram restores it before executing, so the
	// pass's runtime-visible installs are rolled back at the one safe
	// between-phases point and re-installed by the run's placed twins.
	// C1 effect fence (eng effects.go): a silent interpreter re-run — on
	// either fallback arm below — is sound only while NO observable effect
	// has escaped, because RestoreForCompile rolls back registry scopes but
	// cannot un-print emitted output; a re-run after an effect duplicates it
	// (the L-DUP class the pure-value differential is blind to). Armed BEFORE
	// the check pass, which executes module imports: an import-time effect
	// must count against the refusal arm too.
	//
	// The ledger is PER-REQUEST: a fresh one is installed for this run and
	// the prior one restored on return. Detached work from an EARLIER request
	// (a ForkConcurrent body still printing) captured the ledger pointer live
	// at ITS fork/arm time, so its late effects land on the old ledger and
	// cannot spuriously block THIS request's fallback — while a fork this
	// request spawns (an import-time module body) copies the fresh pointer
	// and counts, exactly the ownership the fence needs. Installed before
	// arming so the writer wrappers capture the fresh ledger.
	savedEffects := a.registry.Effects
	a.registry.Effects = &core.EffectLedger{}
	defer func() { a.registry.Effects = savedEffects }()
	disarmFence := a.registry.ArmEffectFence()
	defer disarmFence()
	effectsAt := a.registry.Effects.Count()
	// Compiled execution requested: arm detached fn-unit stamping so
	// runtime-constructed callbacks (service handlers, custom codec fns)
	// compile to units at their store sites (compiler.StampDetachedFn). The flag
	// stays armed through the interpreter FALLBACK below — the top level then
	// interprets but stored callbacks still earn the VM path, which is the
	// compiled mode's contract. It is RESTORED to its prior state on return
	// (defer) so a compiled-mode request never leaks the armed flag into a
	// later plain Run (-no-compile) on a reused instance; disarming is safe
	// because a callback already stamped keeps its VM path regardless of the
	// flag (InvokeCallback gates on the stored ref). Only this call's own
	// arming is undone: a caller that armed the registry itself keeps it armed.
	wasArmed := a.registry.RuntimeStampingEnabled()
	a.registry.EnableRuntimeStamping()
	if !wasArmed {
		defer a.registry.DisableRuntimeStamping()
	}
	prog, reason, res, err := a.CompileCheck(src)
	if err != nil || prog == nil {
		// Read the Stage 1 marker BEFORE the rollback: RestoreForCompile
		// copies the whole pre-pass CheckState back (§3.2's in-place
		// restore), which would wipe the per-pass flag.
		carrierRead := a.registry.Check.FnCarrierReadSubstituted
		a.registry.RestoreForCompile(snap)
		// The check pass's in-place module-load stamps were rolled back with the
		// scopes; drop them so -compile-report shows only the fallback re-run's
		// authoritative stamps, not each rolled-back stamp twice.
		a.registry.ResetStampLog()
		// C1 fence on the REFUSAL arm: the check pass executed module imports
		// (and any other RunInCheckMode word) for real — if one of them emitted
		// an observable effect, re-running the whole source would emit it
		// twice. A statically-invalid program still surfaces its own verdict —
		// the check error, or the first ERROR-severity model-undermining
		// diagnostic behind the "check diagnostics" sentinel: that program
		// fails identically in both engines, so the diagnostic IS the truthful
		// result. A CaughtAtRuntime diagnostic is deliberately NOT surfaced
		// here even though it also raises the sentinel: it was downgraded
		// because a surrounding `do [...]` catches the failure — the
		// interpreter would CONTINUE with the handler's result — so reporting
		// it as the program's own error would be wrong; it falls through to
		// the honest blocked-fallback internal_error below, like any other
		// refusal the fence cannot resolve.
		// STAGE J (plan Phase 11, C2): a GENUINE performance refusal no
		// longer silently re-runs the whole source — it returns the refusal
		// as an error, so the caller decides (RunInterp explicitly, or the
		// CLI's visible warn-and-fall-back). BORU_COMPILE_FALLBACK=1 is the
		// one-release hatch restoring the silent re-run; the tests that pin
		// refusal+fallback-parity semantics set it explicitly. The STATIC
		// classes keep the bounded oracle re-run below regardless: a
		// program with a check error or the "check diagnostics" sentinel
		// fails (or, caught, succeeds) identically in both engines, and
		// the re-run only renders the canonical result.
		// A pass that substituted a fn-carrier read (Stage 1 —
		// FnCarrierReadSubstituted) keeps the in-library fallback on a
		// refusal: before Stage 1 every program in that class refused
		// behind the SILENT check-diagnostics sentinel (the read raised a
		// false undefined_word), and a working program must not trade its
		// quiet slow path for a loud compile_failed because the
		// diagnostic became honest. The refusal REASON is still reported
		// (the CLI's performance warning), and programs whose model now
		// succeeds compile natively instead.
		if err == nil && reason != "" && reason != "check diagnostics" &&
			!carrierRead &&
			os.Getenv("BORU_COMPILE_FALLBACK") != "1" {
			// compile_failed is a GUARANTEE to the caller: no observable
			// effect escaped, so an explicit whole-source re-run (Run's own
			// fallback, the CLI surfaces') is sound. A refusal whose CHECK
			// PASS already emitted output (an import-time module-body print)
			// must therefore return the fence's internal_error instead —
			// exactly what the in-library fallback arm below does — or the
			// caller's re-run would duplicate the effect.
			if a.registry.Effects.Count() != effectsAt {
				return nil, false, "", fenceBlockedFallback(a.registry,
					a.registry.BoruError("internal_error",
						"compiled-mode refusal after the check pass emitted observable output ("+forceCompileReason(reason)+")", ""))
			}
			return nil, false, reason, a.registry.BoruError("compile_failed",
				"bytecode compilation FAILED: "+reason+
					" — this is a compiler defect, not a policy: valid code must compile."+
					" (interpret explicitly with RunInterp, or set BORU_COMPILE_FALLBACK=1 for the one-release silent fallback)", "")
		}
		if a.registry.Effects.Count() != effectsAt {
			if err != nil {
				return nil, false, "", err
			}
			for _, d := range res.Diagnostics {
				if !d.RuntimeMirror && d.Severity == SeverityError {
					return nil, false, "", a.registry.BoruError(d.Code, d.Detail, d.Word)
				}
			}
			return nil, false, "", fenceBlockedFallback(a.registry,
				a.registry.BoruError("internal_error",
					"compiled-mode refusal after the check pass emitted observable output ("+forceCompileReason(reason)+")", ""))
		}
		// C4 attribution: the remaining re-runs are SANCTIONED interpreter
		// entries — the bounded static-error oracle, and the hatch-restored
		// refusal fallback — reporting under this named seam (plan Phase 10).
		restoreAtt := a.registry.SetInterpAttribution("fallback:refusal")
		out, rerr := a.runValues(src)
		restoreAtt()
		// Report the reason ONLY for a genuine performance refusal. A
		// statically-invalid program (err != nil, or the "check diagnostics"
		// sentinel) fails in both engines — the interpreter fallback raises the
		// real error — so it is not a fallback worth warning about.
		if err != nil || reason == "check diagnostics" {
			reason = ""
		}
		if rerr != nil {
			return nil, false, reason, rerr
		}
		return out, false, reason, nil
	}
	// RunProgram rolls the check pass's runtime-visible installs back to the
	// base the Program carries (ReplayBase) at the one safe between-phases
	// point, and lets the run's placed twins (OpBindTwin → core.ApplyBindTwin)
	// re-install each transition at its source position, with OpBindGlobal
	// pushing the runtime values. The module ledger stays pass-final —
	// imports ran once on the pass and must not run again
	// (RestoreBindingsForReplay's contract). On a VM error below, the partial
	// replay is rolled back with everything else by the runtime-bail arm's
	// RestoreForCompile.
	result, err := eng.RunProgram(prog, a.registry)
	if err != nil {
		// An INTERNAL compiled-mode error — a VM/lowering soundness assertion
		// or a recovered handler panic (both carry code internal_error), or any
		// non-Boru Go error — must never reach the caller as a raw compiler bug.
		// Roll the registry back to the pre-check state (exactly as the
		// uncompilable path does) and let the interpreter render the canonical
		// result. Genuine boru runtime errors (type_error, div-by-zero, and the
		// resource ceilings evaluation_limit / tape_exhausted) match the
		// interpreter by the differential gate and are returned as-is — the
		// resource limits in particular fail FAST in both engines by design
		// (see the step-budget note above), so re-running the interpreter would
		// only burn the same budget again. The program DID compile, so this is
		// not a compilable-subset refusal — report no reason.
		if runtimeShouldFallback(err) {
			// C1 fence on the RUNTIME-BAIL arm: the compiled run may already
			// have printed/written before bailing (the L-DUP shape: every
			// section prints, then a dynamic-scope read misses); a silent
			// whole-source re-run would double every effect. Propagate the
			// internal_error, annotated, instead.
			if a.registry.Effects.Count() != effectsAt {
				return nil, true, "", fenceBlockedFallback(a.registry, err)
			}
			a.registry.RestoreForCompile(snap)
			a.registry.ResetStampLog()
			// C4 attribution: the runtime-bail re-run is the second
			// sanctioned interpreter entry (a designed VM defer resolved by
			// re-running) — named so the census distinguishes it.
			restoreAtt := a.registry.SetInterpAttribution("fallback:runtime-bail")
			out, rerr := a.runValues(src)
			restoreAtt()
			if rerr != nil {
				return nil, false, "", rerr
			}
			return out, false, "", nil
		}
		return nil, true, "", err
	}
	return result, true, "", nil
}

// RunCompiledStrict is RunCompiled in FORCE mode: it REQUIRES the bytecode
// path. Where RunCompiled silently falls back to the interpreter for a program
// the emitter cannot lower (or a VM/lowering soundness assertion), RunCompiledStrict
// surfaces that as an error instead — the returned message carries the emitter's
// refusal reason, or the VM's internal_error. Use it to GUARANTEE a program ran
// through the compiler (verifying the compilable subset, benchmarking the VM in
// isolation, or catching a compiler regression that would otherwise hide behind
// the fallback). Genuine boru runtime errors (type_error, div-by-zero, the
// resource ceilings) are returned as-is, exactly as RunCompiled returns them.
//
// Side-effect parity matches RunCompiled: on the compiled path the check pass's
// RunInCheckMode words (def/import/type/macro) persist; on every error path the
// registry is rolled back to its pre-check state.
func (a *Boru) RunCompiledStrict(src string) ([]any, error) {
	snap := a.registry.SnapshotForCompile()
	// Same arming as RunCompiled: force mode is compiled execution, so
	// runtime-constructed callbacks stamp at their store sites too. Restored to
	// its prior state on return so the armed flag never leaks into a later plain
	// Run on a reused instance (see RunCompiled).
	wasArmed := a.registry.RuntimeStampingEnabled()
	a.registry.EnableRuntimeStamping()
	if !wasArmed {
		defer a.registry.DisableRuntimeStamping()
	}
	prog, reason, res, err := a.CompileCheck(src)
	if err != nil {
		a.registry.RestoreForCompile(snap)
		return nil, err
	}
	if prog == nil {
		a.registry.RestoreForCompile(snap)
		return nil, errors.New("force-compile: " + forceCompileReason(reason) + checkDiagnosticsDetail(reason, res))
	}
	// See RunAutoValues: RunProgram rolls back to the Program's own base at
	// the between-phases point, then the run's placed twins replay.
	result, err := eng.RunProgram(prog, a.registry)
	if err != nil {
		// Force mode does NOT fall back: surface the error (including an
		// internal_error, which under RunCompiled would silently re-run the
		// interpreter) so a compiler bug is visible rather than masked.
		return nil, err
	}
	return convertResults(result), nil
}

// forceCompileReason renders an uncompilable-program refusal reason for
// RunCompiledStrict's error, defaulting the (defensive, never observed
// through CompileCheck's current return paths) empty reason to a generic
// message rather than emitting a bare "force-compile: ".
func forceCompileReason(reason string) string {
	if reason == "" {
		return "program is not compilable"
	}
	return reason
}

// checkDiagnosticsDetail names the first blocking diagnostic behind the
// "check diagnostics" refusal sentinel. The bare sentinel names nothing —
// it was force-compile's one unexplained refusal (the completeness review's
// §3 DX gap), doubly opaque because the blocking diagnostic can be
// compile-pass-only (`boru check` prints nothing). The sentinel string
// itself is load-bearing — the fallback classifier compares it by equality
// — so the detail is appended only at this user-facing boundary, with the
// same predicate the CompileCheck gate refused on.
func checkDiagnosticsDetail(reason string, res CheckResult) string {
	if reason != "check diagnostics" {
		return ""
	}
	for _, d := range res.Diagnostics {
		if !d.RuntimeMirror && (d.Severity == SeverityError || d.CaughtAtRuntime) {
			return ": [" + d.Code + "] " + d.Detail
		}
	}
	return ""
}

// fenceBlockedFallback annotates a compiled-mode error whose silent
// interpreter re-run the effect fence blocked (eng effects.go,
// design/legacy/RUNTIME-INDEPENDENCE-COMPLETION-PLAN.0.ignore C1): observable output
// already escaped, so re-running the source would duplicate it. The original
// error survives — a BoruError gains an explanatory note; a foreign Go error
// is wrapped in an internal_error carrying its text — so the caller sees both
// what failed and what to do about it.
func fenceBlockedFallback(r *native.Registry, err error) error {
	const note = "the interpreter fallback was blocked: output was already emitted, so re-running would duplicate it; run with --no-compile and report this as a compiler bug"
	var ae *core.BoruError
	if errors.As(err, &ae) {
		// A designed defer that PREPARED for this arm (a no-match whose site
		// proved the interpreter would also fail the dispatch) carries the
		// rich user-facing raise — surface it instead of an internal error
		// telling the user to report a compiler bug (plan 3c).
		if ae.DeferAlt != nil {
			return ae.DeferAlt
		}
		cp := *ae
		cp.Notes = append(append([]string(nil), ae.Notes...), note)
		return &cp
	}
	return r.BoruErrorHint("internal_error", err.Error(), "", note)
}

// runtimeShouldFallback reports whether a compiled-mode RUN error should be
// resolved by re-running on the interpreter rather than surfaced. True for an
// internal_error (a VM/lowering soundness assertion or a recovered handler
// panic — never surface a raw compiler bug; the interpreter is the correctness
// backstop) and any non-Boru (foreign) error. False for every genuine boru
// runtime error — type_error, div-by-zero, and the resource ceilings
// (evaluation_limit / tape_exhausted) — which the differential gate proves
// match the interpreter, and which the VM deliberately surfaces fast rather
// than hanging or double-running.
func runtimeShouldFallback(err error) bool {
	// A word-policy denial from the VM dispatch gate IS the program's
	// verdict — the interpreter raises the same checker error — never a
	// bail (a re-run would evaluate the program twice and diverge behind
	// the effect fence).
	var pd core.PolicyDenied
	if errors.As(err, &pd) {
		return false
	}
	var ae *core.BoruError
	if !errors.As(err, &ae) {
		return true
	}
	return ae.Code == "internal_error"
}

// CheckResult is the outcome of a static type-check run.
//
// Stack holds the carrier values left on the stack after symbolic
// execution (one per residual result), represented as their type
// path strings. Diagnostics holds any findings the checker recorded
// (e.g. missing return-type annotations). Summary captures a count
// per severity so callers can quickly decide pass/fail without
// walking the diagnostics slice.
type CheckResult struct {
	Stack       []string          `json:"stack"`
	Diagnostics []CheckDiagnostic `json:"diagnostics"`
	Summary     CheckSummary      `json:"summary"`
	// SiteCounts tallies dispatch sites by compilation class during the
	// bytecode recording pass — "mono" (a single resolved signature,
	// compiles to CALL_NATIVE), "poly" (polymorphic, not lowered),
	// "dynamic" (a dynamic carrier / interpreter-island site), and
	// "meta" (compile-time / fn-invoking / higher-order words). Populated
	// only by CompileCheck (the recording pass); nil for a plain Check.
	// It answers "why didn't my hot loop compile to a single path?".
	SiteCounts map[string]int `json:"site_counts,omitempty"`
	// FnCarrierReadSubstituted marks a compile pass that resolved a read
	// of a name def-bound to a computed fn through the fn-carrier side
	// table (Stage 1). A REFUSAL from such a pass is the transitional
	// class that refused behind the silent check-diagnostics sentinel
	// before Stage 1 — RunCompiled keeps that silent interpreter fallback
	// for it, and the census suites classify it with the sentinel rather
	// than as a hard refusal. Populated only by CompileCheck.
	FnCarrierReadSubstituted bool `json:"fn_carrier_read_substituted,omitempty"`
	// BindLedger is the RUNTIME-VISIBLE binding transitions the check pass
	// performed, in source order — the population the bind twins
	// (design/FULL-COMPILATION.0.md §6.5) have to replay. Populated only by
	// CompileCheck, and INERT: nothing consumes it to decide anything, it
	// exists so the twin work can be sized before it is built.
	BindLedger []core.BindTransition `json:"-"`
}

// CheckSummary reports the per-severity count of diagnostics from
// a check run. Errors > 0 means the program has at least one type
// violation the runtime will trip on.
type CheckSummary struct {
	Errors   int `json:"errors"`
	Warnings int `json:"warnings"`
	Infos    int `json:"infos"`
}

// CheckSeverity classifies a diagnostic.
type CheckSeverity = native.CheckSeverity

// Re-exported severity constants.
const (
	SeverityError   = native.SeverityError
	SeverityWarning = native.SeverityWarning
	SeverityInfo    = native.SeverityInfo
)

// CheckDiagnostic is a single finding from the static checker.
type CheckDiagnostic = native.CheckDiagnostic

// BoruError is the structured diagnostic every engine failure surfaces
// as (design/DIAGNOSTICS.0.md): code, detail, primary position,
// secondary labeled spans, notes, and suggestions. Hosts reach it with
// errors.As and re-render with color via Render(RenderOpts{Color:true});
// Error() is always the plain (ANSI-free) rendering.
type BoruError = native.BoruError

// ExitCode reports the status an `IO.exit` request carries, and whether
// err is one at all. It is the whole embedding contract for exit: the
// runtime never calls os.Exit, so a host decides what a program's exit
// request means to it. A CLI driver returns the code as its own process
// status and prints nothing; a long-lived host may log it and carry on.
//
// It unwraps, so a caller that wrapped the error with %w on the way up is
// still recognised. Anything that is not an exit request reports false —
// an ordinary failure is not an exit.
var ExitCode = native.ExitCode

// RenderOpts controls diagnostic rendering (color on/off).
type RenderOpts = native.RenderOpts

// DiagSpan / DiagSuggestion are the structured payload of a BoruError
// or CheckDiagnostic: secondary labeled source locations and
// actionable fixes.
type (
	DiagSpan       = native.DiagSpan
	DiagSuggestion = native.DiagSuggestion
)

// ResolveColor decides whether to color output written to w for a
// --color mode of "always", "never", or "auto" (the default: color
// only a real terminal, honoring NO_COLOR). NO_COLOR is read through
// the registry's installed environment view when there is one, so pass
// the instance's registry where one exists and nil before that (the CLI
// styling its own output).
var ResolveColor = native.ResolveColor

// RenderCheckDiagnostic renders the rich block (source excerpt, notes,
// suggestions) that sits under a check diagnostic's stable one-liner.
var RenderCheckDiagnostic = native.RenderCheckDiagnostic
