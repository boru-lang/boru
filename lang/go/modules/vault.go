package modules

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	core "github.com/boru-lang/boru/core/go"
	"github.com/boru-lang/boru/lang/go/native"
	"github.com/boru-lang/boru/lang/go/policy"
)

// The boru:vault module — the `Vault` namespace, the language-level bridge
// to the host's secret vault (design/VAULT-TUI-PORT.0.md §3).
//
// The module ships the WORDS — argument validation, policy gating, and
// value conversion — while the vault itself (storage, crypto, keyring,
// clipboard) stays host-side: a host injects a backend via
// RegisterHostVault, exactly as a terminal backend arrives via
// RegisterHostTui. With no backend registered every word raises
// `no_backend` — the natural state of the spec harness, wasm, and CI.
//
//	import "boru:vault"
//	Vault.status                       # {ok backend locked aliases …}
//	Vault.secrets                      # [{alias provider …} …]
//	Vault.reveal "stripe"              # the secret value (reveal-gated)
//	Vault.add {alias: "x" value: "k"}  # mutations mirror the vault CLI
//
// Error vocabulary: `vault_usage` (malformed arguments — checked BEFORE
// the backend lookup so the contract is pinnable headlessly),
// `no_backend` (no host registered), `vault` (a backend operation
// failed; the message carries the host's first line).
//
// The op strings handed to VaultSpec.Do are the user-facing word names
// ("add", "reveal", …). Inner natives carry a `vault-` prefix to stay
// clear of same-named core words in the sub-registry (the boru:log
// rename pattern); the export keys are the clean spellings.

// VaultSpec describes a host-provided vault backend. Do executes one
// vault operation: op is the word name, params the validated arguments
// (map-shaped args are flattened into params by key). The result may be
// nil (no-result ops), or any composition of string / bool / int /
// int64 / float64 / []any / map[string]any — vaultAnyToValue projects
// it onto boru values. Session state (active vault, cached passphrase)
// belongs to the host adapter, never to boru (VAULT-TUI-PORT.0.md §5.1).
type VaultSpec struct {
	// Name identifies the backend in diagnostics ("cli", "fake", …).
	Name string
	// Do executes one vault operation. Required.
	Do func(op string, params map[string]any) (any, error)
}

// capVaultHost is the registry capability slot holding the
// host-registered vault backend. Per-registry, like capTuiHost — no
// process global. One backend per registry; a second registration
// errors.
const capVaultHost = "engine.vault.host"

type vaultHostState struct {
	mu   sync.Mutex
	spec *VaultSpec
}

func vaultHostStateFor(r *native.Registry, create bool) *vaultHostState {
	if s, ok, _ := core.Cap[*vaultHostState](r, capVaultHost); ok && s != nil {
		return s
	}
	if !create {
		return nil
	}
	s := &vaultHostState{}
	_ = r.Capabilities.Set(capVaultHost, s)
	return s
}

func init() {
	// module bodies (file/inline imports) resolve the host backend too —
	// the sub-registry inherits this slot by pointer at import time
	native.ModuleInheritedCaps = append(native.ModuleInheritedCaps, capVaultHost)
}

// RegisterHostVault installs a vault backend on reg — the embedder seam
// of design/VAULT-TUI-PORT.0.md §3.1. Registration may happen before or
// after the program imports "boru:vault": the words resolve the backend
// at dispatch time, not at module build.
func RegisterHostVault(reg *native.Registry, spec VaultSpec) error {
	if spec.Name == "" {
		return fmt.Errorf("register vault backend: name must not be empty")
	}
	if spec.Do == nil {
		return fmt.Errorf("register vault backend %q: do must not be nil", spec.Name)
	}
	state := vaultHostStateFor(reg, true)
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.spec != nil {
		return fmt.Errorf("register vault backend %q: backend %q already registered", spec.Name, state.spec.Name)
	}
	state.spec = &spec
	return nil
}

func hostVaultSpec(r *native.Registry) *VaultSpec {
	state := vaultHostStateFor(r, false)
	if state == nil {
		return nil
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	return state.spec
}

// ---- policy ----

// checkVaultPolicy gates one vault op behind the `vault` policy scope
// (ops: read / reveal / mutate / admin / clipboard — see
// policy.GlobalsFor for the global caps each touches). Mirrors
// checkTuiPolicy.
func checkVaultPolicy(r *native.Registry, word, op string) error {
	if r == nil {
		return nil
	}
	pol := native.HostPolicy(r)
	if pol == nil {
		return nil
	}
	args := policy.Args{}
	if !pol.Installed("vault") {
		return native.PolicyRefusal(r, word, &policy.Denied{
			Code:    policy.CodeCapabilityNotInstalled,
			Scope:   "vault",
			Op:      op,
			Profile: pol.Name(),
			Blame:   "vault.install=false",
			Args:    args,
		})
	}
	return native.PolicyRefusal(r, word, pol.Check("vault", op, args))
}

// ---- the op table ----

// vaultParam describes one positional argument of a vault word. A
// TMap-typed param is an option bag: its keys are flattened into the
// backend params, with req naming the keys that must be present as
// non-empty Strings. A TList-typed param must hold Strings.
type vaultParam struct {
	name string
	typ  *native.Type
	req  []string
}

// vaultOp describes one vault word: its user-facing name (= the
// VaultSpec.Do op string and the export key), the policy op that gates
// it, its positional params, and its return type (nil = no result).
type vaultOp struct {
	name   string
	pol    string
	params []vaultParam
	ret    *native.Type
}

func vpStr(name string) []vaultParam {
	return []vaultParam{{name: name, typ: native.TString}}
}

func vpMap(name string, req ...string) []vaultParam {
	return []vaultParam{{name: name, typ: native.TMap, req: req}}
}

// vaultOps is the whole word surface (VAULT-TUI-PORT.0.md §3.2). Rows
// are data: adding a word is a row plus a docs_vault.go line.
var vaultOps = []vaultOp{
	// session
	{name: "status", pol: "read", ret: native.TMap},
	{name: "needs-passphrase", pol: "read", ret: native.TBoolean},
	{name: "authed", pol: "read", ret: native.TBoolean},
	{name: "authenticate", pol: "read", params: vpStr("pass"), ret: native.TBoolean},
	{name: "status-text", pol: "read", ret: native.TString},
	{name: "providers-text", pol: "read", ret: native.TString},
	{name: "config-text", pol: "read", ret: native.TString},
	// secrets
	{name: "secrets", pol: "read", ret: native.TList},
	{name: "secret", pol: "read", params: vpStr("alias"), ret: native.TAny},
	{name: "reveal", pol: "reveal", params: vpStr("alias"), ret: native.TString},
	{name: "add", pol: "mutate", params: vpMap("spec", "alias", "value")},
	{name: "rotate", pol: "mutate", params: vpMap("spec", "alias", "value")},
	{name: "remove", pol: "mutate", params: vpStr("alias")},
	{name: "rename", pol: "mutate", params: vpMap("spec", "from", "to")},
	{name: "set-expiry", pol: "mutate", params: []vaultParam{
		{name: "alias", typ: native.TString}, {name: "when", typ: native.TString}}},
	{name: "clear-expiry", pol: "mutate", params: vpStr("alias")},
	{name: "recipes", pol: "read", params: vpStr("alias"), ret: native.TList},
	// access
	{name: "capabilities", pol: "read", ret: native.TMap},
	{name: "grant", pol: "mutate", params: vpMap("spec", "alias"), ret: native.TString},
	{name: "revoke", pol: "mutate", params: vpStr("id")},
	// passwords
	{name: "passwords", pol: "read", ret: native.TList},
	{name: "password-add", pol: "mutate", params: vpMap("spec", "name", "pass")},
	{name: "password-remove", pol: "mutate", params: vpStr("name")},
	{name: "password-revoke-temp", pol: "mutate"},
	{name: "temp-password-count", pol: "read", ret: native.TInteger},
	// maintenance
	{name: "verify", pol: "read", params: vpMap("opts"), ret: native.TMap},
	{name: "scan", pol: "read", params: []vaultParam{{name: "paths", typ: native.TList}}, ret: native.TString},
	{name: "history", pol: "read", ret: native.TString},
	{name: "restore", pol: "mutate", params: []vaultParam{{name: "generation", typ: native.TInteger}}},
	{name: "audit", pol: "read", params: vpMap("filters"), ret: native.TString},
	// settings
	{name: "config-set", pol: "mutate", params: []vaultParam{
		{name: "key", typ: native.TString}, {name: "value", typ: native.TString}}},
	{name: "config-unset", pol: "mutate", params: vpStr("key")},
	{name: "lock", pol: "mutate"},
	{name: "unlock", pol: "mutate"},
	{name: "locked", pol: "read", ret: native.TBoolean},
	{name: "prefs", pol: "read", ret: native.TMap},
	{name: "set-prefs", pol: "mutate", params: vpMap("prefs")},
	// multi-vault
	{name: "vaults", pol: "read", ret: native.TList},
	{name: "switch", pol: "admin", params: vpMap("spec", "folder")},
	{name: "create", pol: "admin", params: vpMap("spec", "folder", "backend")},
	{name: "set-default", pol: "admin"},
	{name: "prune-index", pol: "admin", params: vpMap("spec", "folder")},
	// misc
	{name: "copy", pol: "clipboard", params: vpStr("text"), ret: native.TString},
}

// vaultInnerName is the sub-registry spelling of a vault word — a
// `vault-` prefix keeps `add`, `remove`, `copy`, … clear of the
// same-named core words (the boru:log rename pattern).
func vaultInnerName(op string) string { return "vault-" + op }

func vaultUsage(r *native.Registry, word, msg string) error {
	return r.BoruError("vault_usage", word+": "+msg, word)
}

// vaultCollectParams validates one op's args against its param specs
// and builds the backend params map. Map-shaped args are flattened by
// key; required keys must be present as non-empty Strings.
func vaultCollectParams(op vaultOp, args []native.Value, r *native.Registry) (map[string]any, error) {
	params := map[string]any{}
	for i, p := range op.params {
		v := args[i]
		switch p.typ {
		case native.TString:
			s, err := v.AsConcreteString()
			if err != nil {
				return nil, vaultUsage(r, op.name, p.name+": must be a String")
			}
			params[p.name] = s
		case native.TInteger:
			n, err := v.AsConcreteInteger()
			if err != nil {
				return nil, vaultUsage(r, op.name, p.name+": must be an Integer")
			}
			params[p.name] = n
		case native.TList:
			lst, err := native.RequireConcreteList(v, op.name)
			if err != nil {
				return nil, vaultUsage(r, op.name, p.name+": must be a List")
			}
			items := make([]any, 0, lst.Len())
			for j := 0; j < lst.Len(); j++ {
				s, sErr := lst.Get(j).AsConcreteString()
				if sErr != nil {
					return nil, vaultUsage(r, op.name, p.name+": elements must be Strings")
				}
				items = append(items, s)
			}
			params[p.name] = items
		default: // native.TMap — the option bag
			mp, err := native.RequireConcreteMap(v, op.name)
			if err != nil {
				return nil, vaultUsage(r, op.name, p.name+": must be a Map")
			}
			for _, k := range p.req {
				rv, ok := mp.Get(k)
				if !ok {
					return nil, vaultUsage(r, op.name, k+": required")
				}
				s, sErr := rv.AsConcreteString()
				if sErr != nil || s == "" {
					return nil, vaultUsage(r, op.name, k+": must be a non-empty String")
				}
			}
			for _, k := range mp.Keys() {
				kv, _ := mp.Get(k)
				params[k] = native.ValueToAny(kv)
			}
		}
	}
	return params, nil
}

// vaultAnyToValue projects a backend result onto a boru value. Map keys
// are emitted in sorted order so results canon deterministically; an
// unrecognised shape degrades to its string form rather than erroring.
func vaultAnyToValue(v any) native.Value {
	switch val := v.(type) {
	case nil:
		return native.NewTypeLiteral(native.TNone)
	case bool:
		return native.NewBoolean(val)
	case string:
		return native.NewString(val)
	case int:
		return native.NewInteger(int64(val))
	case int64:
		return native.NewInteger(val)
	case float64:
		return native.NewFloat(val)
	case []any:
		items := make([]native.Value, len(val))
		for i, it := range val {
			items[i] = vaultAnyToValue(it)
		}
		return native.NewList(items)
	case map[string]any:
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		om := native.NewOrderedMap()
		for _, k := range keys {
			om.Set(k, vaultAnyToValue(val[k]))
		}
		return native.NewMap(om)
	default:
		return native.NewString(fmt.Sprint(val))
	}
}

// makeVaultHandler builds the one generic handler an op's native
// delegates to: policy → usage validation → backend lookup → Do →
// result conversion. Validation precedes the backend lookup so the
// usage contract is pinnable with no host registered
// (lang/spec/module-vault.tsv).
func makeVaultHandler(op vaultOp) func([]native.Value, map[string]native.Value, []native.Value, *native.Registry) ([]native.Value, error) {
	return func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
		if err := checkVaultPolicy(r, op.name, op.pol); err != nil {
			return nil, err
		}
		params, err := vaultCollectParams(op, args, r)
		if err != nil {
			return nil, err
		}
		spec := hostVaultSpec(r)
		if spec == nil {
			return nil, r.BoruError("no_backend", op.name+": no vault backend registered", op.name)
		}
		res, doErr := spec.Do(op.name, params)
		if doErr != nil {
			return nil, r.BoruError("vault", op.name+": "+firstLine(doErr.Error()), op.name)
		}
		if op.ret == nil {
			return nil, nil
		}
		return []native.Value{vaultAnyToValue(res)}, nil
	}
}

// firstLine trims a backend error to its first line — adapter failures
// can carry multi-line CLI output, and the status line wants one.
func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func vaultNoReturns(_ []native.Value, _ *native.Registry) []native.Value { return nil }

// vaultDeclaredReturns is the check-mode residual a vault op's declared
// Returns would have produced on its own — one fresh carrier of ret, and
// a declared Any riding DYNAMIC (the declaredReturnCarriers default arm's
// rule: a strict Any carrier conforms to no typed slot and would poison
// every downstream consumer). Attaching a ReturnsFn replaces Returns
// entirely, so the mirror has to hand this back verbatim.
func vaultDeclaredReturns(ret *native.Type) native.ReturnsFunc {
	if ret == nil {
		return vaultNoReturns
	}
	return func(_ []native.Value, _ *native.Registry) []native.Value {
		c := native.NewCarrier(ret)
		if ret.Equal(native.TAny) {
			c.Dynamic = true
		}
		return []native.Value{c}
	}
}

// concreteArgsGate gates a usage mirror whose validator reads EVERY
// positional arg — a required key out of an option bag, each element of
// a paths list. All n operands must be statically KNOWN all the way down,
// and the arity must be the one the word declares (a short slice would
// index out of range inside the validator).
//
// Known, not concrete: DeepKnown also admits a bare type node, because
// `{value: None}` is as decided as `{value: 5}` and the validator declines
// both at run time. A dynamic operand still closes the gate for the
// reason DeepConcreteOptionsAt states: it was matched optimistically, so
// the runtime value may not be the shape being validated.
func concreteArgsGate(n int) func([]native.Value) bool {
	return func(args []native.Value) bool {
		if len(args) != n {
			return false
		}
		for _, a := range args {
			if a.Dynamic || !native.DeepKnown(a) {
				return false
			}
		}
		return true
	}
}

// argsMatchDeclared reports whether every operand conforms to the type
// its signature position declares.
//
// A ReturnsFn is not reached only through a plain signature match: the
// union-combo expansion (carrier.go) also invokes it per ALTERNATIVE, and
// an alternative that does not inhabit the declared slot would otherwise
// reach a validator that reports "must be a Map" for an operand the run
// never routes here at all. This is the overload rule the dynamic-operand
// decline already states, applied to the shape rather than the modality.
func argsMatchDeclared(args []native.Value, declared []*native.Type) bool {
	if len(args) != len(declared) {
		return false
	}
	for i, a := range args {
		if a.Parent == nil || !a.Parent.ConformsTo(declared[i]) {
			return false
		}
	}
	return true
}

// vaultUsageMirror is the concrete-gated mirror of one vault word's
// usage contract. It runs makeVaultHandler's own PURE PREFIX — the
// policy gate, then vaultCollectParams — in the handler's declared
// order, so the diagnostic carries the byte-identical code and detail
// the runtime raises.
//
// The policy gate is consulted but its compile failure is NOT reported: when the
// policy denies the op the runtime never reaches the usage validation,
// so claiming a usage defect there would name the wrong error. Declining
// keeps the mirror's rule exact — it fires only where the run does.
func vaultUsageMirror(op vaultOp, base native.ReturnsFunc) native.ReturnsFunc {
	declared := make([]*native.Type, len(op.params))
	for i, p := range op.params {
		declared[i] = p.typ
	}
	return native.MirrorReturns(op.name, concreteArgsGate(len(op.params)),
		func(args []native.Value, r *native.Registry) error {
			if !argsMatchDeclared(args, declared) {
				return nil
			}
			if polErr := checkVaultPolicy(r, op.name, op.pol); polErr != nil {
				return nil
			}
			_, err := vaultCollectParams(op, args, r)
			return err
		}, base)
}

// vaultNatives derives the sub-registry natives from the op table: one
// single-signature native per op, all-forward (BarrierPos -1 — the
// module-wrapper dispatch rule).
func vaultNatives() []native.NativeFunc {
	T := func(ts ...*native.Type) []*native.Type { return ts }
	out := make([]native.NativeFunc, 0, len(vaultOps))
	for _, op := range vaultOps {
		argTypes := make([]*native.Type, len(op.params))
		for i, p := range op.params {
			argTypes[i] = p.typ
		}
		sig := native.Signature{
			Args:       argTypes,
			Impl:       native.Go(makeVaultHandler(op)),
			BarrierPos: -1,
		}
		if op.ret != nil {
			sig.Returns = T(op.ret)
		} else {
			sig.Returns = T()
			sig.ReturnsFn = vaultNoReturns
		}
		// A word with no positional args has no usage contract to model —
		// vaultCollectParams over an empty param list cannot fail — so only
		// the arg-taking words carry the mirror.
		if len(op.params) > 0 {
			sig.ReturnsFn = vaultUsageMirror(op, vaultDeclaredReturns(op.ret))
		}
		out = append(out, native.NativeFunc{
			Name:       vaultInnerName(op.name),
			Signatures: []native.Signature{sig},
		})
	}
	// `identity` is not a generic Do-dispatch word: it mints an opaque
	// credential handle rather than projecting a vault result onto a
	// boru value, so it carries its own handler (vault_identity.go).
	out = append(out, native.NativeFunc{
		Name: vaultInnerName("identity"),
		Signatures: []native.Signature{{
			Args:       T(native.TString),
			Impl:       native.Go(vaultIdentityHandler),
			Returns:    T(TVaultIdentity),
			ReturnsFn:  vaultIdentityMirror(),
			BarrierPos: -1,
		}},
	})
	return out
}

// BuildVaultModule creates the "boru:vault" native module: the op-table
// words in an isolated sub-registry behind trivial-delegation wrappers,
// exported under their clean spellings.
func BuildVaultModule(parent *native.Registry) (native.ModuleDesc, error) {
	natives := vaultNatives()
	subReg, err := newModuleRegistry("boru:vault", natives)
	if err != nil {
		return native.ModuleDesc{}, err
	}
	exports := native.NewOrderedMap()
	for _, n := range natives {
		exports.Set(strings.TrimPrefix(n.Name, "vault-"), makeModuleFnDef(n, subReg))
	}
	return moduleDesc(parent, "Vault", subReg, exports), nil
}
