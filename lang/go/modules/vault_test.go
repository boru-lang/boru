package modules

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/boru-lang/boru/lang/go/native"
	"github.com/boru-lang/boru/lang/go/policy"
	parser "github.com/boru-lang/boru/parser/go"
)

// End-to-end coverage for the boru:vault bridge words (vault.go) over a
// scripted fake backend — no vault anywhere, per
// design/VAULT-TUI-PORT.0.md §6.2. Positive paths are paired with the
// negative contracts (usage, no backend, backend failure, policy
// denial) per lang/go/CLAUDE.md.

type fakeVaultCall struct {
	op     string
	params map[string]any
}

// fakeVault is a scripted VaultSpec backend: canned per-op results and
// errors, recording every call.
type fakeVault struct {
	mu      sync.Mutex
	calls   []fakeVaultCall
	results map[string]any
	errs    map[string]error
}

func (f *fakeVault) spec() VaultSpec {
	return VaultSpec{Name: "fake", Do: func(op string, params map[string]any) (any, error) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.calls = append(f.calls, fakeVaultCall{op: op, params: params})
		if err := f.errs[op]; err != nil {
			return nil, err
		}
		return f.results[op], nil
	}}
}

func (f *fakeVault) lastCall(t *testing.T) fakeVaultCall {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.calls) == 0 {
		t.Fatal("no backend calls recorded")
	}
	return f.calls[len(f.calls)-1]
}

// runVaultSteps runs steps on a fully-wired registry with fv registered
// as the vault backend (fv == nil leaves NO backend registered).
func runVaultSteps(t *testing.T, fv *fakeVault, steps []string) ([]native.Value, error) {
	t.Helper()
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	reg.ParseFunc = parser.Parse
	InstallResolver(reg)
	if fv != nil {
		if rErr := RegisterHostVault(reg, fv.spec()); rErr != nil {
			t.Fatal(rErr)
		}
	}
	engine := native.NewTop(reg)
	var result []native.Value
	for _, step := range steps {
		vals, pErr := parser.Parse(step)
		if pErr != nil {
			return nil, pErr
		}
		result, err = engine.Run(vals)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

// vaultMinimalArgs renders a minimal valid argument text for an op, from
// its own param specs.
func vaultMinimalArgs(op vaultOp) string {
	parts := make([]string, 0, len(op.params))
	for _, p := range op.params {
		switch p.typ {
		case native.TString:
			parts = append(parts, `"x"`)
		case native.TInteger:
			parts = append(parts, `3`)
		case native.TList:
			parts = append(parts, `["p"]`)
		default:
			kv := make([]string, 0, len(p.req))
			for _, k := range p.req {
				kv = append(kv, k+`: "v"`)
			}
			parts = append(parts, "{"+strings.Join(kv, " ")+"}")
		}
	}
	return strings.Join(parts, " ")
}

// vaultFakeResult returns a result matching an op's declared return
// shape.
func vaultFakeResult(op vaultOp) any {
	switch op.ret {
	case native.TMap:
		return map[string]any{"ok": true}
	case native.TBoolean:
		return true
	case native.TString:
		return "s"
	case native.TList:
		return []any{"a"}
	case native.TInteger:
		return int64(2)
	default: // TAny or no result
		return nil
	}
}

// Every word: with no backend registered the (validated) call raises the
// first-class no_backend error — proving usage validation precedes the
// backend lookup for every row of the op table.
func TestVaultNoBackendEveryWord(t *testing.T) {
	for _, op := range vaultOps {
		expr := "Vault." + op.name
		if a := vaultMinimalArgs(op); a != "" {
			expr += " " + a
		}
		_, err := runVaultSteps(t, nil, []string{`import "boru:vault"`, expr})
		if err == nil || !strings.Contains(err.Error(), op.name+": no vault backend registered") {
			t.Errorf("%s: expected no_backend, got %v", op.name, err)
		}
	}
}

// Every word: with a scripted backend the call dispatches, reaches the
// backend under its clean op name, and converts the declared result
// shape without error.
func TestVaultEveryWordThroughFake(t *testing.T) {
	for _, op := range vaultOps {
		fv := &fakeVault{results: map[string]any{op.name: vaultFakeResult(op)}}
		expr := "Vault." + op.name
		if a := vaultMinimalArgs(op); a != "" {
			expr += " " + a
		}
		out, err := runVaultSteps(t, fv, []string{`import "boru:vault"`, expr})
		if err != nil {
			t.Errorf("%s: %v", op.name, err)
			continue
		}
		if got := fv.lastCall(t).op; got != op.name {
			t.Errorf("%s: backend saw op %q", op.name, got)
		}
		if op.ret == nil && len(out) != 0 {
			t.Errorf("%s: no-result op left %v on the stack", op.name, out)
		}
		if op.ret != nil && len(out) != 1 {
			t.Errorf("%s: expected one result, got %v", op.name, out)
		}
	}
}

// Result conversion: nested maps come back with sorted keys, lists,
// integers, floats and booleans all project; a missing secret is None.
func TestVaultResultConversion(t *testing.T) {
	fv := &fakeVault{results: map[string]any{
		"status": map[string]any{
			"ok": true, "aliases": 3, "backend": "file", "cost": 1.5,
			"slots": []any{"admin", "ci"},
		},
		"secret": nil,
	}}
	out, err := runVaultSteps(t, fv, []string{
		`import "boru:vault"`,
		`def s (Vault.status)`,
		`join "|" [(convert String s.ok) (convert String s.aliases) s.backend
		           (convert String s.cost) (join "," s.slots)
		           (convert String ((Vault.secret "gone") eq None))]`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	got := tuiLastString(t, out)
	if got != "true|3|file|1.5|admin,ci|true" {
		t.Fatalf("converted fields = %q", got)
	}
}

// Mutations: map-shaped args flatten into the backend params by key
// (spec/opts wrapper names never reach the backend), non-required keys
// pass through with their own types, and positional args land under
// their param names.
func TestVaultParamsFlatten(t *testing.T) {
	fv := &fakeVault{}
	_, err := runVaultSteps(t, fv, []string{
		`import "boru:vault"`,
		`Vault.add {alias: "a" value: "k" provider: "openai" expiry: ""}`,
	})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	call := fv.lastCall(t)
	if call.op != "add" || call.params["alias"] != "a" || call.params["value"] != "k" ||
		call.params["provider"] != "openai" || call.params["expiry"] != "" {
		t.Fatalf("add params = %+v", call.params)
	}
	if _, ok := call.params["spec"]; ok {
		t.Fatalf("wrapper param name leaked: %+v", call.params)
	}

	_, err = runVaultSteps(t, fv, []string{
		`import "boru:vault"`,
		`Vault.rotate {alias: "a" value: "n" revoke-caps: true}`,
	})
	if err != nil {
		t.Fatalf("rotate: %v", err)
	}
	if got := fv.lastCall(t).params["revoke-caps"]; got != true {
		t.Fatalf("revoke-caps = %v", got)
	}

	_, err = runVaultSteps(t, fv, []string{
		`import "boru:vault"`,
		`Vault.set-expiry "a" "2026-01-01"`,
		`Vault.scan ["p1" "p2"]`,
		`Vault.restore 5`,
	})
	if err != nil {
		t.Fatalf("positional: %v", err)
	}
	fv.mu.Lock()
	n := len(fv.calls)
	se, sc, re := fv.calls[n-3], fv.calls[n-2], fv.calls[n-1]
	fv.mu.Unlock()
	if se.params["alias"] != "a" || se.params["when"] != "2026-01-01" {
		t.Fatalf("set-expiry params = %+v", se.params)
	}
	paths, _ := sc.params["paths"].([]any)
	if len(paths) != 2 || paths[0] != "p1" || paths[1] != "p2" {
		t.Fatalf("scan params = %+v", sc.params)
	}
	if re.params["generation"] != int64(5) {
		t.Fatalf("restore params = %+v", re.params)
	}
}

// The infix form dispatches through the wrapper (the BarrierPos -1
// contract): sig[0] binds from the forward side, sig[1] from the stack.
func TestVaultWrapperInfixForm(t *testing.T) {
	fv := &fakeVault{}
	_, err := runVaultSteps(t, fv, []string{
		`import "boru:vault"`,
		`"w" Vault.set-expiry "a"`,
	})
	if err != nil {
		t.Fatalf("infix form: %v", err)
	}
	call := fv.lastCall(t)
	if call.params["alias"] != "a" || call.params["when"] != "w" {
		t.Fatalf("infix form params = %+v", call.params)
	}
}

// Usage negatives, all with NO backend registered — the contract is
// checked before the lookup. Expressible rows also live in
// lang/spec/module-vault.tsv.
func TestVaultUsageNegatives(t *testing.T) {
	cases := []struct{ expr, want string }{
		{`Vault.add {}`, "add: alias: required"},
		{`Vault.add {alias: "a"}`, "add: value: required"},
		{`Vault.add {alias: "a" value: ""}`, "add: value: must be a non-empty String"},
		{`Vault.add {alias: "a" value: 5}`, "add: value: must be a non-empty String"},
		{`Vault.rename {from: "a"}`, "rename: to: required"},
		{`Vault.scan [5]`, "scan: paths: elements must be Strings"},
	}
	for _, c := range cases {
		_, err := runVaultSteps(t, nil, []string{`import "boru:vault"`, c.expr})
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: expected %q, got %v", c.expr, c.want, err)
		}
	}
}

// Type-literal and wrong-typed arguments are refused without panicking
// (the TestTypeLiteralNoPanic discipline), driven straight at the
// generic handler.
func TestVaultCollectParamsNegativesNoPanic(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	find := func(name string) vaultOp {
		for _, op := range vaultOps {
			if op.name == name {
				return op
			}
		}
		t.Fatalf("no op %q", name)
		return vaultOp{}
	}
	cases := []struct {
		op   vaultOp
		arg  native.Value
		want string
	}{
		{find("secret"), native.NewTypeLiteral(native.TString), "alias: must be a String"},
		{find("restore"), native.NewTypeLiteral(native.TInteger), "generation: must be an Integer"},
		{find("scan"), native.NewTypeLiteral(native.TList), "paths: must be a List"},
		{find("add"), native.NewTypeLiteral(native.TMap), "spec: must be a Map"},
	}
	for _, c := range cases {
		func() {
			defer func() {
				if p := recover(); p != nil {
					t.Errorf("%s: panicked: %v", c.op.name, p)
				}
			}()
			h := makeVaultHandler(c.op)
			_, hErr := h([]native.Value{c.arg}, nil, nil, reg)
			if hErr == nil || !strings.Contains(hErr.Error(), c.want) {
				t.Errorf("%s: expected %q, got %v", c.op.name, c.want, hErr)
			}
		}()
	}
}

// A backend failure maps to the `vault` code carrying only the error's
// first line.
func TestVaultBackendErrorMapped(t *testing.T) {
	fv := &fakeVault{errs: map[string]error{"reveal": errors.New("boom\nsecond line")}}
	_, err := runVaultSteps(t, fv, []string{`import "boru:vault"`, `Vault.reveal "a"`})
	if err == nil || !strings.Contains(err.Error(), "reveal: boom") {
		t.Fatalf("expected mapped backend error, got %v", err)
	}
	if strings.Contains(err.Error(), "second") {
		t.Fatalf("multi-line error leaked: %v", err)
	}
}

// firstLine: single-line strings pass through untouched.
func TestVaultFirstLineSingle(t *testing.T) {
	if got := firstLine("plain"); got != "plain" {
		t.Fatalf("firstLine = %q", got)
	}
}

// Policy arms: a nil registry and a policy-free registry pass; sandbox
// (vault.install=false) yields capability_not_installed; trusted
// consults Check and proceeds (to no_backend here).
func TestVaultPolicyArms(t *testing.T) {
	if err := checkVaultPolicy(nil, "status", "read"); err != nil {
		t.Fatalf("nil registry: %v", err)
	}
	statusOp := vaultOps[0]
	h := makeVaultHandler(statusOp)

	pol, err := policy.Load("sandbox")
	if err != nil {
		t.Fatal(err)
	}
	reg, err := native.DefaultRegistryWithPolicy(pol)
	if err != nil {
		t.Fatal(err)
	}
	_, dErr := h(nil, nil, nil, reg)
	// Coded by the gate (native/policy_error.go) so `dot code` can read it.
	var dAe *native.BoruError
	if !errors.As(dErr, &dAe) || dAe.Code != "capability_not_installed" {
		t.Fatalf("sandbox status = %v", dErr)
	}

	trusted, err := policy.Load("trusted")
	if err != nil {
		t.Fatal(err)
	}
	treg, err := native.DefaultRegistryWithPolicy(trusted)
	if err != nil {
		t.Fatal(err)
	}
	_, tErr := h(nil, nil, nil, treg)
	if tErr == nil || !strings.Contains(tErr.Error(), "no vault backend registered") {
		t.Fatalf("trusted status = %v", tErr)
	}
}

// The vault scope's global-cap bindings.
func TestVaultPolicyGlobals(t *testing.T) {
	cases := []struct {
		op   string
		want string
	}{
		{"read", "disk.read"}, {"reveal", "disk.read"},
		{"mutate", "disk.write"}, {"admin", "disk.write"},
		{"clipboard", "process"},
	}
	for _, c := range cases {
		got := policy.GlobalsFor("vault", c.op)
		if len(got) != 1 || got[0] != c.want {
			t.Errorf("GlobalsFor(vault, %s) = %v", c.op, got)
		}
	}
	if got := policy.GlobalsFor("vault", "unknown"); got != nil {
		t.Errorf("GlobalsFor(vault, unknown) = %v", got)
	}
}

// Registration contracts: name and Do are required, a second backend is
// refused, and an unregistered registry resolves to nil.
func TestVaultRegisterContracts(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if got := hostVaultSpec(reg); got != nil {
		t.Fatalf("virgin registry spec = %v", got)
	}
	if rErr := RegisterHostVault(reg, VaultSpec{}); rErr == nil ||
		!strings.Contains(rErr.Error(), "name must not be empty") {
		t.Fatalf("empty name = %v", rErr)
	}
	if rErr := RegisterHostVault(reg, VaultSpec{Name: "x"}); rErr == nil ||
		!strings.Contains(rErr.Error(), "do must not be nil") {
		t.Fatalf("nil do = %v", rErr)
	}
	do := func(string, map[string]any) (any, error) { return nil, nil }
	if rErr := RegisterHostVault(reg, VaultSpec{Name: "one", Do: do}); rErr != nil {
		t.Fatalf("first registration: %v", rErr)
	}
	if rErr := RegisterHostVault(reg, VaultSpec{Name: "two", Do: do}); rErr == nil ||
		!strings.Contains(rErr.Error(), `backend "one" already registered`) {
		t.Fatalf("double registration = %v", rErr)
	}
	if got := hostVaultSpec(reg); got == nil || got.Name != "one" {
		t.Fatalf("resolved spec = %v", got)
	}
}

// vaultAnyToValue: every shape projects, map keys sort, and an
// unrecognised shape degrades to its string form.
func TestVaultAnyToValueShapes(t *testing.T) {
	if v := vaultAnyToValue(nil); native.IsConcrete(&v) || !v.Is(native.TNone) {
		t.Fatalf("nil = %v", v)
	}
	if v := vaultAnyToValue(7); mustInt(t, v) != 7 {
		t.Fatalf("int = %v", v)
	}
	if v := vaultAnyToValue(int64(8)); mustInt(t, v) != 8 {
		t.Fatalf("int64 = %v", v)
	}
	f, err := addrOf(vaultAnyToValue(1.25)).AsConcreteFloat()
	if err != nil || f != 1.25 {
		t.Fatalf("float = %v %v", f, err)
	}
	b, err := addrOf(vaultAnyToValue(true)).AsConcreteBoolean()
	if err != nil || !b {
		t.Fatalf("bool = %v %v", b, err)
	}
	if v := vaultAnyToValue(struct{ X int }{1}); !strings.Contains(mustStr(t, v), "1") {
		t.Fatalf("default = %v", v)
	}
	m := vaultAnyToValue(map[string]any{"b": 1, "a": 2})
	om, _ := native.AsMap(m)
	if om == nil || strings.Join(om.Keys(), ",") != "a,b" {
		t.Fatalf("map keys = %v", m)
	}
	lst := vaultAnyToValue([]any{"x", 1})
	rl, aErr := native.RequireConcreteList(lst, "test")
	if aErr != nil || rl.Len() != 2 {
		t.Fatalf("list = %v %v", lst, aErr)
	}
}

func mustInt(t *testing.T, v native.Value) int64 {
	t.Helper()
	n, err := v.AsConcreteInteger()
	if err != nil {
		t.Fatalf("not an Integer: %v", v)
	}
	return n
}

func mustStr(t *testing.T, v native.Value) string {
	t.Helper()
	s, err := v.AsConcreteString()
	if err != nil {
		t.Fatalf("not a String: %v", v)
	}
	return s
}

// The static-analysis hook of the no-result words: never any returns.
func TestVaultNoReturnsContract(t *testing.T) {
	if got := vaultNoReturns(nil, nil); got != nil {
		t.Fatalf("vaultNoReturns = %v", got)
	}
}

// The registry-construction error arm of BuildVaultModule, via the
// shared newDefaultRegistry seam.
func TestVaultBuildInitError(t *testing.T) {
	parent, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	orig := newDefaultRegistry
	newDefaultRegistry = func(...func(*native.Registry)) (*native.Registry, error) {
		return nil, fmt.Errorf("vault-boom-init")
	}
	t.Cleanup(func() { newDefaultRegistry = orig })
	if _, bErr := BuildVaultModule(parent); bErr == nil ||
		!strings.Contains(bErr.Error(), "vault-boom-init") {
		t.Fatalf("build init error = %v", bErr)
	}
}

// Every export has a doc line and every doc line has an export — the
// describe surface cannot drift from the op table.
func TestVaultDocsComplete(t *testing.T) {
	parent, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	desc, err := BuildVaultModule(parent)
	if err != nil {
		t.Fatal(err)
	}
	exports := desc.Exports["Vault"]
	docs := moduleDocs["boru:vault"]
	for _, k := range exports.Keys() {
		if docs[k] == "" {
			t.Errorf("export %q has no doc line", k)
		}
	}
	for k := range docs {
		if _, ok := exports.Get(k); !ok {
			t.Errorf("doc %q has no export", k)
		}
	}
	// Every registered native becomes an export — the op table plus the
	// words that carry their own handler instead of Do-dispatching
	// (`identity`, vault_identity.go).
	if len(exports.Keys()) != len(vaultNatives()) {
		t.Errorf("exports = %d, natives = %d", len(exports.Keys()), len(vaultNatives()))
	}
	if len(vaultNatives()) <= len(vaultOps) {
		t.Error("the non-Do-dispatch words are missing from vaultNatives")
	}
}
