package native

import (
	core "github.com/boru-lang/boru/core/go"
	"github.com/boru-lang/boru/lang/go/capabilities"
	"github.com/boru-lang/boru/lang/go/policy"
)

// Host-side capability keys. The host installs implementations under
// these names on core.Registry; word handlers retrieve them through
// the typed accessors in this file. borueng itself never sees them.
const (
	CapFileOps        = "engine.fileops"         // active capabilities.FileOps
	CapMemFileOps     = "engine.fileops.mem"     // lazily created in-memory FileOps
	CapOverlayFileOps = "engine.fileops.overlay" // lazily created mem-over-host overlay
	// CapModelledFileOps holds the overlay that MODELS filesystem effects for
	// a registry running with core.CheckState.ModelEffects — a module body
	// executed during `boru check`. Distinct from CapOverlayFileOps so a body
	// that also opts into `__sys.fs overlay` gets its own layer rather than
	// silently sharing the modelling one.
	CapModelledFileOps = "engine.fileops.modelled"
	CapFormats         = "engine.formats"        // map[string]Format read/write registry
	CapExtensions      = "engine.extensions"     // map[string]string file-ext→format-name
	CapSQLite          = "engine.sqlite"         // *SQLiteStore
	CapPolicy          = "engine.policy"         // policy.Policy enforcing permissions
	CapClock           = "engine.clock"          // capabilities.Clock (the time source)
	CapLogSinks        = "engine.logsinks"       // *LogSinkRegistry (boru:log fan-out sinks)
	CapDebugOps        = "engine.debugops"       // capabilities.DebugOps (interactive stepping)
	CapScriptArgs      = "engine.scriptargs"     // []string script positional arguments (IO.args)
	CapHTTPOps         = "engine.httpops"        // capabilities.HTTPOps (boru:net fetch transport)
	CapClientIdents    = "engine.clientidents"   // map[string]capabilities.ClientIdentity (mTLS)
	CapHTTPTransports  = "engine.httptransports" // map[TLSProfile]http.RoundTripper (per-registry cache)
	CapEnv             = "engine.env"            // capabilities.EnvOps (IO.env)
	CapStreamProbe     = "engine.streamprobe"    // capabilities.StreamProbe (IO.is-tty)
	CapStdinLines      = "engine.stdinlines"     // *stdinLines (the ONE buffered reader over r.Input)
)

// EffectiveHTTPOps returns the HTTP transport capability for the current
// invocation. A host may install one under CapHTTPOps — to pin TLS
// settings, route through its own proxy, or stub the network in tests.
// When none is installed the default is used, which serves
// http.DefaultTransport and so reproduces the behaviour of an
// *http.Client with a nil Transport. Never returns nil.
//
// Unlike FileOps this has no policy-uninstall branch: the transport is
// not itself an authority. `fetch` is gated by checkFetchPolicy before
// any transport is resolved, so removing the slot would only fall back
// to the default and could not deny anything.
func EffectiveHTTPOps(r *Registry) capabilities.HTTPOps {
	if ops, ok, _ := core.Cap[capabilities.HTTPOps](r, CapHTTPOps); ok && ops != nil {
		return ops
	}
	return capabilities.DefaultHTTPOps{}
}

// SetHostHTTPOps installs an HTTPOps capability (used by host embedders
// and by tests supplying a stub transport). A nil registry or nil ops is
// a no-op, matching SetHostDebugOps.
func SetHostHTTPOps(r *Registry, ops capabilities.HTTPOps) {
	if r == nil || ops == nil {
		return
	}
	_ = r.Capabilities.Set(CapHTTPOps, ops)
}

// EffectiveDebugOps returns the installed DebugOps capability, or (nil,
// false) when none is installed. Debug.step uses it for an interactive
// step controller; with no DebugOps it falls back to printing the trace.
func EffectiveDebugOps(r *Registry) (capabilities.DebugOps, bool) {
	if ops, ok, _ := core.Cap[capabilities.DebugOps](r, CapDebugOps); ok && ops != nil {
		return ops, true
	}
	return nil, false
}

// SetHostDebugOps installs a DebugOps capability (used by the `boru debug`
// session, the REPL/TTY host, and tests supplying a scripted controller).
func SetHostDebugOps(r *Registry, ops capabilities.DebugOps) {
	if r == nil || ops == nil {
		return
	}
	_ = r.Capabilities.Set(CapDebugOps, ops)
}

// RemoveHostDebugOps uninstalls the DebugOps capability — the uninstall
// affordance SetHostDebugOps deliberately lacks (nil there is a no-op).
// A borrowing host (the `boru debug` session) restores the registry with
// this on the way out so a reused registry doesn't keep pausing into a
// dead controller.
func RemoveHostDebugOps(r *Registry) {
	if r == nil {
		return
	}
	_, _ = r.Capabilities.Delete(CapDebugOps)
}

// EffectiveClock returns the time source for the current invocation. The
// host (or the spec runner) may install a Clock under CapClock — e.g. a
// capabilities.FixedClock for deterministic tests. When none is installed,
// a real wall clock is used. Never returns nil.
func EffectiveClock(r *Registry) capabilities.Clock {
	if clk, ok, _ := core.Cap[capabilities.Clock](r, CapClock); ok && clk != nil {
		return clk
	}
	return capabilities.WallClock{}
}

// SetHostClock installs a Clock capability (used by the spec runner and
// host embedders to freeze time).
func SetHostClock(r *Registry, clk capabilities.Clock) {
	if r == nil {
		return
	}
	_ = r.Capabilities.Set(CapClock, clk)
}

// HostLogSinks returns the LogSinkRegistry installed on r, or nil if
// none has been created yet. Most callers want LogSinkRegistryFor,
// which lazily creates and installs a default registry (console sink
// attached) on first use — mirroring how `print` needs no host wiring.
func HostLogSinks(r *Registry) *LogSinkRegistry {
	lsr, _, _ := core.Cap[*LogSinkRegistry](r, CapLogSinks)
	return lsr
}

// SetHostLogSinks installs a LogSinkRegistry as the active sink
// registry. When a policy uninstalls the "log" scope (install:false)
// the slot is left empty so emission is a silent no-op — logging must
// never crash a program. Hosts call this to pre-attach an OTel/Datadog
// sink before the boru program runs `import "boru:log"`.
func SetHostLogSinks(r *Registry, lsr *LogSinkRegistry) {
	if r == nil {
		return
	}
	if pol := HostPolicy(r); pol != nil && !pol.Installed("log") {
		_, _ = r.Capabilities.Delete(CapLogSinks)
		return
	}
	_ = r.Capabilities.Set(CapLogSinks, lsr)
}

// HostPolicy returns the policy installed on r, or nil if none. A
// nil result means "no permissions configured" — the engine and
// every capability wrapper treat that as allow-everything (the
// default opt-in posture).
func HostPolicy(r *Registry) policy.Policy {
	p, _, _ := core.Cap[policy.Policy](r, CapPolicy)
	return p
}

// SetHostPolicy installs a Policy as a capability. Must be called
// before SetHostX hooks if those hooks should auto-wrap the
// capability with the policy. RewrapCapabilities can re-apply
// wrapping after a late SetHostPolicy.
func SetHostPolicy(r *Registry, p policy.Policy) {
	if p == nil {
		return
	}
	_ = r.Capabilities.Set(CapPolicy, p)
}

// HostFileOps returns the FileOps installed on r. When a policy has
// uninstalled the fileops capability (via install:false), the slot
// is empty — in that case HostFileOps returns notInstalledFileOps,
// a stub whose ReadFile/WriteFile/MkdirAll return a
// capability_not_installed error rather than a nil deref. Word
// handlers that need filesystem access can call without a nil
// guard and surface the error to the user.
//
// Returns nil only when r itself is nil (a misconfigured registry).
func HostFileOps(r *Registry) capabilities.FileOps {
	if r == nil {
		return nil
	}
	ops, ok, _ := core.Cap[capabilities.FileOps](r, CapFileOps)
	if !ok || ops == nil {
		return notInstalledFileOps{}
	}
	return ops
}

// SetHostFileOps installs the active fileops capability and re-wires
// any registered jsonic-format multisource resolver to use it.
//
// When a policy is present on r and its fileops scope has
// install=false, the capability slot is left empty so word handlers
// that try to reach it produce capability_not_installed errors.
// When a policy is present without install=false, the FileOps is
// wrapped with permissionedFileOps so each call is gated.
func SetHostFileOps(r *Registry, ops capabilities.FileOps) {
	if pol := HostPolicy(r); pol != nil {
		if !pol.Installed("fileops") {
			// Uninstall: remove the slot so HostFileOps returns nil.
			_, _ = r.Capabilities.Delete(CapFileOps)
			return
		}
		ops = NewPermissionedFileOps(ops, pol)
	}
	_ = r.Capabilities.Set(CapFileOps, ops)
	rewireFileOpsResolver(r, ops)
}

// rewireFileOpsResolver points the jsonic multisource resolver at ops so a
// `.jsonic` include (`@"other.jsonic"`) resolves through the currently
// effective filesystem. Shared by SetHostFileOps (install) and IO.unmount
// (restore) so a restored backend never leaves the resolver on the
// unmounted filesystem.
func rewireFileOpsResolver(r *Registry, ops capabilities.FileOps) {
	if formats := HostFormats(r); formats != nil {
		if jf, ok := formats["jsonic"].(*JsonicFormat); ok {
			jf.Resolver = MakeFileOpsResolver(ops)
		}
	}
}

// HostFormats returns the format registry installed on r, or nil if
// none. The map is owned by the host and may be mutated in place to
// register or replace individual formats.
func HostFormats(r *Registry) map[string]Format {
	formats, _, _ := core.Cap[map[string]Format](r, CapFormats)
	return formats
}

// SetHostFormats installs the format registry as a single capability.
// When a policy has formats.install=false, the slot is removed so
// HostFormats returns nil and read/write handlers raise
// capability_not_installed.
func SetHostFormats(r *Registry, formats map[string]Format) {
	if pol := HostPolicy(r); pol != nil && !pol.Installed("formats") {
		_, _ = r.Capabilities.Delete(CapFormats)
		return
	}
	_ = r.Capabilities.Set(CapFormats, formats)
}

// HostExtensions returns the file-extension→format-name map installed on
// r, or nil if none. The map is owned by the host and may be mutated in
// place (e.g. by RegisterFormat) to add or override extension mappings.
// Keys are lowercase, without the leading dot.
func HostExtensions(r *Registry) map[string]string {
	exts, _, _ := core.Cap[map[string]string](r, CapExtensions)
	return exts
}

// SetHostExtensions installs the extension→format map. It shares the
// `formats` policy scope with SetHostFormats: when formats.install=false
// the slot is removed so read falls back to text for every extension.
func SetHostExtensions(r *Registry, exts map[string]string) {
	if pol := HostPolicy(r); pol != nil && !pol.Installed("formats") {
		_, _ = r.Capabilities.Delete(CapExtensions)
		return
	}
	_ = r.Capabilities.Set(CapExtensions, exts)
}

// HostScriptArgs returns the script's positional arguments installed on
// r (everything after the script path on the CLI), or nil when the host
// set none — IO.args renders nil and empty identically, as an empty
// list.
func HostScriptArgs(r *Registry) []string {
	args, _, _ := core.Cap[[]string](r, CapScriptArgs)
	return args
}

// SetHostScriptArgs installs the script's positional arguments. The CLI
// calls it with everything after the script path; embedded hosts may
// set whatever invocation vector fits. Nil clears the slot.
func SetHostScriptArgs(r *Registry, args []string) {
	if args == nil {
		_, _ = r.Capabilities.Delete(CapScriptArgs)
		return
	}
	_ = r.Capabilities.Set(CapScriptArgs, args)
}

// HostSQLite returns the SQLite store installed on r, or nil if none.
func HostSQLite(r *Registry) *SQLiteStore {
	store, _, _ := core.Cap[*SQLiteStore](r, CapSQLite)
	return store
}

// SetHostSQLite installs the SQLite store as a capability. When a
// policy has sqlite.install=false, the slot is removed so HostSQLite
// returns nil and sqlite-* word handlers raise capability_not_installed.
func SetHostSQLite(r *Registry, store *SQLiteStore) {
	if pol := HostPolicy(r); pol != nil && !pol.Installed("sqlite") {
		_, _ = r.Capabilities.Delete(CapSQLite)
		return
	}
	_ = r.Capabilities.Set(CapSQLite, store)
}

// EffectiveFileOps returns the fileops to use for the current
// invocation, switched by flags on the active context store's __sys.fs:
//
//   - {mem: true} — a fully in-memory FileOps (hermetic; nothing
//     touches the host filesystem);
//   - {overlay: true} — a UNION of a writable in-memory upper over the
//     host fileops as a read-only lower (capabilities.OverlayFileOps):
//     reads fall through to real files, every mutation lands in memory,
//     and deletes are whiteouts — the partially-in-memory configuration
//     unit tests use to exercise real fixtures without mutating them.
//     {mem: true} wins when both flags are set (it is the stricter
//     isolation).
//
// Each variant is cached as a capability on first use so state persists
// across invocations; otherwise the regular host fileops is returned.
//
// Never returns nil: if the policy has uninstalled the fileops
// capability, HostFileOps returns notInstalledFileOps so consumers
// can call methods without a nil guard and get a clean error.
// Returns nil only when r itself is nil (a misconfigured registry).
//
// This logic used to live on core.Registry; it now lives here
// because borueng has no fileops concept.
func EffectiveFileOps(r *Registry) capabilities.FileOps {
	if r == nil {
		return nil
	}
	fsStore := sysFsStore(r)
	if fsStore == nil {
		return hostOrModelled(r)
	}
	if fsFlag(fsStore, "mem") {
		if mem, _, _ := core.Cap[capabilities.FileOps](r, CapMemFileOps); mem != nil {
			return mem
		}
		mem := capabilities.NewMem()
		_ = r.Capabilities.Set(CapMemFileOps, mem)
		return mem
	}
	if fsFlag(fsStore, "overlay") {
		if ov, _, _ := core.Cap[capabilities.FileOps](r, CapOverlayFileOps); ov != nil {
			return ov
		}
		ov := capabilities.NewOverlay(capabilities.NewMem(), HostFileOps(r))
		_ = r.Capabilities.Set(CapOverlayFileOps, ov)
		return ov
	}
	return hostOrModelled(r)
}

// hostOrModelled returns the host FileOps — or, when this registry MODELS
// effects (core.CheckState.ModelEffects: a module body running under
// `boru check`), a mem-over-host overlay so the body's filesystem mutations
// are contained and discarded while its reads still resolve against the real
// filesystem.
//
// The overlay is what makes the substitution safe rather than merely
// suppressive. A write-then-read-back body still sees its own bytes, a
// remove still hides the file from the rest of the body, and a read of a
// file the body never touched falls through to the real one — so a module
// that loads today cannot start failing under check. A blanket compile failure
// cannot promise any of that, which is why "deny-all around the check-pass
// body" was rejected: denial raises, and the raise aborts the import and
// loses the exports check needs.
//
// The upper's Cwd is seeded from the host's own resolution of ".", because
// the overlay uses Upper.ResolvePath as its path authority and a fresh
// MemFileOps resolves relative paths against "" — which collapses the
// upward directory walk in resolveBareModule (filepath.Dir(".") == ".") and
// would break a bare `import "foo"` inside a modelled body.
//
// Cached per registry so writes persist across calls within one body. The
// lower is captured at first use, matching the shipped `__sys.fs overlay`
// mode; a body that re-points fs mode mid-run gets the flag branches above,
// which take precedence.
func hostOrModelled(r *Registry) capabilities.FileOps {
	host := HostFileOps(r)
	if !r.Check.ModelsEffects() {
		return host
	}
	if ov, _, _ := core.Cap[capabilities.FileOps](r, CapModelledFileOps); ov != nil {
		return ov
	}
	mem := capabilities.NewMem()
	// A resolution failure is not worth a branch: the zero Cwd is exactly the
	// state a fresh MemFileOps ships with, and a FileOps that cannot resolve
	// "." could not have supported a bare import anyway.
	cwd, _ := host.ResolvePath(".")
	mem.Cwd = cwd
	ov := capabilities.NewOverlay(mem, host)
	_ = r.Capabilities.Set(CapModelledFileOps, ov)
	return ov
}

// sysFsStore walks the active context store to __sys.fs, or nil when any
// hop is absent or not a store.
func sysFsStore(r *Registry) *StoreInstanceInfo {
	store := r.Contexts.Top()
	if store == nil {
		return nil
	}
	sysVal, ok := store.Get("__sys")
	if !ok {
		return nil
	}
	sysStore, ok := sysVal.Data.(*StoreInstanceInfo)
	if !ok {
		return nil
	}
	fsVal, ok := sysStore.Get("fs")
	if !ok {
		return nil
	}
	fsStore, ok := fsVal.Data.(*StoreInstanceInfo)
	if !ok {
		return nil
	}
	return fsStore
}

// fsFlag reads one boolean toggle off the __sys.fs store.
func fsFlag(fsStore *StoreInstanceInfo, key string) bool {
	v, ok := fsStore.Get(key)
	if !ok {
		return false
	}
	asBool, _ := AsBoolean(v)
	return v.Parent.ConformsTo(TBoolean) && asBool
}

// HostEnvOps returns the installed environment view, or nil when the host
// installed none. Nil means "no environment visible", which is the hermetic
// default: IO.env then reports every name as unset rather than reaching for
// the real process environment behind the host's back.
func HostEnvOps(r *Registry) capabilities.EnvOps {
	ops, _, _ := core.Cap[capabilities.EnvOps](r, CapEnv)
	return ops
}

// SetHostEnvOps installs the environment view, honouring the policy the same
// way SetHostFileOps does: a profile that uninstalls the `env` scope clears
// the slot outright, so the capability is absent rather than merely declining
// — and a configured profile wraps it so per-name rules apply.
func SetHostEnvOps(r *Registry, ops capabilities.EnvOps) {
	if r == nil {
		return
	}
	if ops == nil {
		_, _ = r.Capabilities.Delete(CapEnv)
		return
	}
	if pol := HostPolicy(r); pol != nil {
		if !pol.Installed("env") {
			_, _ = r.Capabilities.Delete(CapEnv)
			return
		}
		ops = permissionedEnvOps{inner: ops, policy: pol}
	}
	_ = r.Capabilities.Set(CapEnv, ops)
}

// permissionedEnvOps gates each read through the policy's `env` scope, so a
// profile can expose an allowlist (read-only.jsonic allows LANG, TZ, BORU_*)
// rather than all-or-nothing. A denied name reads as UNSET rather than
// raising: a program probing for an optional variable should take its default
// path, not crash, and the alternative leaks which names exist.
type permissionedEnvOps struct {
	inner  capabilities.EnvOps
	policy policy.Policy
}

func (p permissionedEnvOps) Get(name string) (string, bool) {
	if err := p.policy.Check("env", "read", policy.Args{"name": name}); err != nil {
		return "", false
	}
	return p.inner.Get(name)
}

func (p permissionedEnvOps) All() []string {
	var out []string
	for _, n := range p.inner.All() {
		if err := p.policy.Check("env", "read", policy.Args{"name": n}); err == nil {
			out = append(out, n)
		}
	}
	return out
}

// HostStreamProbe returns the installed terminal probe, or nil when the host
// installed none. Nil means "nothing is a terminal", which is the hermetic
// default: IO.is-tty answers false rather than the runtime asking the operating
// system a question the host did not authorise.
func HostStreamProbe(r *Registry) capabilities.StreamProbe {
	probe, _, _ := core.Cap[capabilities.StreamProbe](r, CapStreamProbe)
	return probe
}

// SetHostStreamProbe installs the terminal probe, honouring the policy the
// same way SetHostFileOps and SetHostEnvOps do: a profile that uninstalls the
// `terminal` scope clears the slot, so IO.is-tty answers false.
//
// Note what is deliberately NOT here: a per-call policy Check. Every shipped
// profile except `trusted` and `full` uninstalls the terminal scope
// (profiles/sandbox.jsonic:60, and read-only / client extend sandbox;
// compute.jsonic:42, gen.jsonic:58), and Check on an uninstalled scope
// RAISES. Gating the word per call would therefore make the RFC's own colour
// idiom — `IO.is-tty (IO.stdout)` next to `IO.env "NO_COLOR"`
// (design/CLI-PROGRAMS.0.md §5) — abort under a sandbox instead of answering.
// Clearing the slot honours the profile author's intent exactly, and gives
// the program the usable answer "not a terminal" rather than a failure it
// cannot do anything with.
func SetHostStreamProbe(r *Registry, probe capabilities.StreamProbe) {
	if r == nil {
		return
	}
	if probe == nil {
		_, _ = r.Capabilities.Delete(CapStreamProbe)
		return
	}
	if pol := HostPolicy(r); pol != nil && !pol.Installed("terminal") {
		_, _ = r.Capabilities.Delete(CapStreamProbe)
		return
	}
	_ = r.Capabilities.Set(CapStreamProbe, probe)
}
