package modules

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
	"github.com/boru-lang/boru/lang/go/native"
	"github.com/boru-lang/boru/lang/go/policy"
)

// Direct unit coverage for the boru:vault check-mode mirrors
// (vaultUsageMirror, vaultIdentityMirror) and the residual they hand
// back (vaultDeclaredReturns). module-vault.tsv §4 and §6 drive the
// FLAGGED shapes end to end; these tests own the arms no row reaches —
// the gate's three declines, the policy-denied decline, and the two
// residual flavours a corpus row cannot inspect.

func vaultMirrorRegistry(t *testing.T) *native.Registry {
	t.Helper()
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	t.Cleanup(reg.Check.Begin())
	return reg
}

// vaultOpNamed returns the op-table row for name — the mirrors are built
// per row, so a test that names the word stays honest if the row moves.
func vaultOpNamed(t *testing.T, name string) vaultOp {
	t.Helper()
	for _, op := range vaultOps {
		if op.name == name {
			return op
		}
	}
	t.Fatalf("no vault op named %q", name)
	return vaultOp{}
}

func vaultMap(kv ...string) native.Value {
	m := native.NewOrderedMap()
	for i := 0; i+1 < len(kv); i += 2 {
		m.Set(kv[i], native.NewString(kv[i+1]))
	}
	return native.NewMap(m)
}

func TestVaultUsageMirrorArms(t *testing.T) {
	add := vaultOpNamed(t, "add")
	rf := vaultUsageMirror(add, vaultDeclaredReturns(add.ret))

	// The flagged shape: a concrete option bag missing a required key.
	// The detail is vaultCollectParams' own, which is the point of
	// running the handler's prefix rather than restating the rule.
	r := vaultMirrorRegistry(t)
	rf([]native.Value{vaultMap("alias", "a")}, r)
	if len(r.Check.Diagnostics) != 1 || r.Check.Diagnostics[0].Code != "vault_usage" {
		t.Fatalf("add missing `value` must flag vault_usage, got %+v", r.Check.Diagnostics)
	}
	if want := "add: value: required"; r.Check.Diagnostics[0].Detail != want {
		t.Fatalf("detail = %q, want %q", r.Check.Diagnostics[0].Detail, want)
	}

	// A complete bag is silent — the backend lookup that follows is not
	// the mirror's business.
	r2 := vaultMirrorRegistry(t)
	rf([]native.Value{vaultMap("alias", "a", "value", "k")}, r2)
	if len(r2.Check.Diagnostics) != 0 {
		t.Fatalf("a well-formed add must stay silent, got %+v", r2.Check.Diagnostics)
	}

	// Gate decline 1 — a computed key. The runtime value decides whether
	// the bag is complete, so the mirror must not read the carrier as an
	// absent (or empty) value.
	r3 := vaultMirrorRegistry(t)
	computed := native.NewOrderedMap()
	computed.Set("alias", native.NewString("a"))
	computed.Set("value", core.NewCarrier(core.TString))
	rf([]native.Value{native.NewMap(computed)}, r3)
	if len(r3.Check.Diagnostics) != 0 {
		t.Fatalf("a computed option value must not be flagged, got %+v", r3.Check.Diagnostics)
	}

	// Gate decline 2 — a DYNAMIC operand. It matched optimistically, so
	// the run may not take the overload being modelled.
	r4 := vaultMirrorRegistry(t)
	dyn := vaultMap("alias", "a")
	dyn.Dynamic = true
	rf([]native.Value{dyn}, r4)
	if len(r4.Check.Diagnostics) != 0 {
		t.Fatalf("a dynamic operand must not be flagged, got %+v", r4.Check.Diagnostics)
	}

	// Gate decline 3 — the wrong arity. Dispatch owns that compile failure, and
	// vaultCollectParams would index out of range on a short slice.
	r5 := vaultMirrorRegistry(t)
	rf(nil, r5)
	if len(r5.Check.Diagnostics) != 0 {
		t.Fatalf("a short arg slice must be left to dispatch, got %+v", r5.Check.Diagnostics)
	}

	// Gate decline 4 — an operand that does not inhabit the declared slot.
	// It reaches here only through union-combo expansion, and the run never
	// routes it to this word, so "spec: must be a Map" would be a defect
	// reported against an overload that is not taken.
	r6 := vaultMirrorRegistry(t)
	rf([]native.Value{native.NewInteger(1)}, r6)
	if len(r6.Check.Diagnostics) != 0 {
		t.Fatalf("an off-type operand must be left to dispatch, got %+v", r6.Check.Diagnostics)
	}

	// A bare type node inside the bag is DECIDED, not unknown: `None` is
	// as statically known as a String, and the run declines it the same way.
	r7 := vaultMirrorRegistry(t)
	withNone := native.NewOrderedMap()
	withNone.Set("alias", native.NewString("a"))
	withNone.Set("value", native.NewTypeLiteral(native.TNone))
	rf([]native.Value{native.NewMap(withNone)}, r7)
	if len(r7.Check.Diagnostics) != 1 || r7.Check.Diagnostics[0].Detail != "add: value: must be a non-empty String" {
		t.Fatalf("a None option value must flag, got %+v", r7.Check.Diagnostics)
	}
}

func TestArgsMatchDeclared(t *testing.T) {
	// The conformance guard itself: arity, a conforming operand, an
	// off-type one, and the nil-Parent value a zero Value carries.
	if argsMatchDeclared(nil, []*native.Type{native.TMap}) {
		t.Fatal("a short arg slice must not match")
	}
	if !argsMatchDeclared([]native.Value{vaultMap("alias", "a")}, []*native.Type{native.TMap}) {
		t.Fatal("a Map operand must match a declared Map slot")
	}
	if argsMatchDeclared([]native.Value{native.NewInteger(1)}, []*native.Type{native.TMap}) {
		t.Fatal("an Integer operand must not match a declared Map slot")
	}
	if argsMatchDeclared([]native.Value{{}}, []*native.Type{native.TMap}) {
		t.Fatal("a Parent-less value must not match")
	}
}

func TestVaultUsageMirrorDeclinesUnderPolicy(t *testing.T) {
	// When the policy denies the op the runtime raises the compile failure and
	// never reaches the usage validation, so reporting a usage defect
	// here would name the wrong error. The mirror declines instead.
	pol, err := policy.Load("sandbox")
	if err != nil {
		t.Fatalf("load sandbox policy: %v", err)
	}
	reg, err := native.DefaultRegistryWithPolicy(pol)
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	t.Cleanup(reg.Check.Begin())

	add := vaultOpNamed(t, "add")
	rf := vaultUsageMirror(add, vaultDeclaredReturns(add.ret))
	rf([]native.Value{vaultMap("alias", "a")}, reg)
	if len(reg.Check.Diagnostics) != 0 {
		t.Fatalf("a policy-denied add must not report a usage defect, got %+v", reg.Check.Diagnostics)
	}
}

func TestVaultDeclaredReturns(t *testing.T) {
	r := vaultMirrorRegistry(t)

	// No declared return — the residual stays empty, as vaultNoReturns
	// produced before the mirror was attached.
	if out := vaultDeclaredReturns(nil)(nil, r); len(out) != 0 {
		t.Fatalf("a ret-less op residual = %v, want nothing", out)
	}

	// A concrete return is one STRICT carrier of that type.
	out := vaultDeclaredReturns(native.TString)(nil, r)
	if len(out) != 1 || out[0].Dynamic || !out[0].Parent.Equal(native.TString) {
		t.Fatalf("String residual = %v, want one strict String carrier", out)
	}

	// A declared Any rides DYNAMIC — `Vault.secret`'s return. A strict
	// Any conforms to no typed slot and would poison every consumer
	// downstream, which is why declaredReturnCarriers marks it too.
	anyOut := vaultDeclaredReturns(native.TAny)(nil, r)
	if len(anyOut) != 1 || !anyOut[0].Dynamic || !anyOut[0].Parent.Equal(native.TAny) {
		t.Fatalf("Any residual = %v, want one dynamic Any carrier", anyOut)
	}
}

func TestVaultIdentityMirrorArms(t *testing.T) {
	rf := vaultIdentityMirror()

	// The flagged shape: an empty alias names no credential.
	r := vaultMirrorRegistry(t)
	out := rf([]native.Value{native.NewString("")}, r)
	if len(out) != 1 || !out[0].Parent.Equal(TVaultIdentity) {
		t.Fatalf("identity residual = %v, want one VaultIdentity carrier", out)
	}
	if len(r.Check.Diagnostics) != 1 || r.Check.Diagnostics[0].Code != "vault_error" {
		t.Fatalf("an empty alias must flag vault_error, got %+v", r.Check.Diagnostics)
	}

	// A named alias is silent — the credential is minted lazily, so
	// there is nothing else to decide statically.
	r2 := vaultMirrorRegistry(t)
	rf([]native.Value{native.NewString("acme-mtls")}, r2)
	if len(r2.Check.Diagnostics) != 0 {
		t.Fatalf("a named alias must stay silent, got %+v", r2.Check.Diagnostics)
	}

	// A computed alias closes the gate: the runtime string decides.
	r3 := vaultMirrorRegistry(t)
	rf([]native.Value{core.NewCarrier(core.TString)}, r3)
	if len(r3.Check.Diagnostics) != 0 {
		t.Fatalf("a computed alias must not be flagged, got %+v", r3.Check.Diagnostics)
	}

	// The non-String arm of the validator: a concrete Integer reaches
	// AsConcreteString's compile failure rather than the empty-string test.
	// (Dispatch rejects it first in a program; the validator must still
	// answer for it, since the mirror runs before dispatch decides.)
	r4 := vaultMirrorRegistry(t)
	rf([]native.Value{native.NewInteger(5)}, r4)
	if len(r4.Check.Diagnostics) != 1 || r4.Check.Diagnostics[0].Code != "vault_error" {
		t.Fatalf("a non-String alias must flag vault_error, got %+v", r4.Check.Diagnostics)
	}
}
