package lang

import (
	"errors"
	"fmt"
	"io"
	"strconv"

	core "github.com/boru-lang/boru/core/go"
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
	// BaseDir anchors the program's relative imports (`import "./lib.boru"`)
	// when set. The CLI's `run` and `debug` pass the script's own directory,
	// so a file means the same wherever the process was started — the rule
	// `check` and `build` already follow (NUR083); `-e` and the REPL, which
	// have no file, leave it empty and the process cwd stands.
	BaseDir string
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
	if o.BaseDir != "" {
		reg.BaseDir = o.BaseDir
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

// NewFromRegistry wraps an ALREADY-WIRED registry in a *Boru instance so a
// host with a long-lived registry of its own — the REPL, a service — can use
// the Run / Check entry points over it. The caller owns the wiring (parse
// func, module resolver, Manager, Output, policy): nothing is installed or
// re-installed here, and the zero Options apply.
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
	a.registry.Check.EmitLateBindingHints()
	if err != nil {
		return CheckResult{Diagnostics: a.registry.Check.Diagnostics}, err
	}

	stack := make([]string, len(result))
	for i, v := range result {
		if core.IsDynamic(&v) {
			// Surface the gradual modality in the residual stack so a
			// dynamic carrier is distinguishable from a strict one.
			stack[i] = "dynamic(" + v.Parent().Leaf() + ")"
		} else {
			stack[i] = v.Parent().Leaf()
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
// Run executes src on the tree-walking interpreter — the one engine.
func (a *Boru) Run(src string) ([]any, error) {
	result, err := a.runValues(src)
	if err != nil {
		return nil, err
	}
	return convertResults(result), nil
}

// RunInterp is Run under its explicit name. It dates from the bytecode lane
// (design/legacy), when it named the interpreter as the differential oracle
// the compiled path was measured against; the lane is gone and the
// interpreter is the only engine, so the two are one call. Kept for the
// callers that spell the engine out (the gates, the REPL).
func (a *Boru) RunInterp(src string) ([]any, error) {
	return a.Run(src)
}

// RunInterpValues is Run without the host-value projection — the raw engine
// Values, for callers whose renderer needs the engine's own Value.String()
// (the REPL's per-line echo).
func (a *Boru) RunInterpValues(src string) ([]native.Value, error) {
	return a.runValues(src)
}

// runValues is Run without the host-value projection: the raw engine stack,
// so a host renderer (the REPL's v.String()) stays byte-identical with what
// the engine produced.
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
		case native.IsBareTypeNode(&v):
			// Type literal (e.g. `typeof x` result, a bare `Integer`
			// or user-minted Foo literal): render its type name
			// rather than trying to extract an Integer/String payload
			// that isn't there.
			out[i] = v.String()
		case v.Parent().ConformsTo(native.TInteger):
			n, _ := native.AsInteger(v)
			out[i] = n
		case v.Parent().ConformsTo(native.TString):
			s, _ := native.AsString(v)
			out[i] = s
		default:
			out[i] = v.String()
		}
	}
	return out
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
