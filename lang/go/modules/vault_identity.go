package modules

import (
	"crypto/tls"
	"fmt"

	core "github.com/boru-lang/boru/core/go"
	"github.com/boru-lang/boru/lang/go/capabilities"
	"github.com/boru-lang/boru/lang/go/native"
)

// TVaultIdentity is the opaque handle `Vault.identity` returns: a
// reference to a vault-held TLS client credential. FixedID 5012, next
// free in the 5000 band after Terminal 5011; pinned in
// fixedid_stability_test.go.
//
// It is deliberately opaque and deliberately NOT a String. `Vault.reveal`
// exists and returns a String, which is right for an API token a program
// must paste into a header — but wrong for a private key, because the
// moment a key is a guest value it can be printed, logged, stored, or
// sent somewhere. This handle can be passed to `tls: {identity: …}` and
// nothing else; its Format redacts.
var TVaultIdentity = registerVaultType("Ideal/VaultIdentity", 5012, vaultIdentityBehavior{})

func registerVaultType(path string, id int, b core.TypeBehavior) *core.Type {
	t, err := core.Builtin.RegisterType(path, id, "boru:vault", b)
	if err != nil {
		native.RecordTypeInitError(fmt.Errorf("vault: register %s: %w", path, err))
	}
	return t
}

// vaultIdentityRef is the handle's payload: the vault alias, and the
// registry-scoped name the credential was registered under.
type vaultIdentityRef struct {
	alias string
	name  string
}

type vaultIdentityBehavior struct{}

func (vaultIdentityBehavior) Match(v native.Value, t *native.Type) bool {
	return native.DefaultBehavior.Match(v, t)
}

func (vaultIdentityBehavior) Equal(a, b native.Value) bool {
	ra, oka := asVaultIdentity(a)
	rb, okb := asVaultIdentity(b)
	if !oka || !okb {
		return native.DefaultBehavior.Equal(a, b)
	}
	return ra.name == rb.name
}

// Format redacts. A credential handle that printed its alias into a log
// would leak which credential a program uses even though it never leaks
// the key; the alias is recoverable from the vault by anyone entitled to
// it, and from here by nobody.
func (vaultIdentityBehavior) Format(native.Value) string { return "<vault identity>" }

func asVaultIdentity(v native.Value) (*vaultIdentityRef, bool) {
	ep, ok := v.Data.(core.ExtensionPayload)
	if !ok {
		return nil, false
	}
	ref, ok := ep.Body.(*vaultIdentityRef)
	return ref, ok
}

// VaultIdentityName returns the registered credential name behind a
// handle, for ParseTLSOpts. Reports false for any other value.
func VaultIdentityName(v native.Value) (string, bool) {
	ref, ok := asVaultIdentity(v)
	if !ok {
		return "", false
	}
	return ref.name, true
}

// validateVaultIdentityAlias is vaultIdentityHandler's PURE PREFIX: the
// alias must be a non-empty concrete String. Split out so the check-mode
// mirror below runs the handler's own source lines and the two cannot
// drift apart.
func validateVaultIdentityAlias(args []native.Value, r *native.Registry) error {
	alias, err := args[0].AsConcreteString()
	if err != nil || alias == "" {
		return r.BoruError("vault_error", "identity: alias must be a non-empty String", "identity")
	}
	return nil
}

// vaultIdentityMirror reports that compile failure statically. Nothing before it
// in the handler can fail and nothing in it touches the vault — minting a
// handle is lazy (the doc comment above) — so a concrete empty alias is a
// guaranteed run-time error. The residual stays the declared handle type.
func vaultIdentityMirror() native.ReturnsFunc {
	return native.MirrorReturns("identity", concreteArgsGate(1),
		validateVaultIdentityAlias,
		func(_ []native.Value, _ *native.Registry) []native.Value {
			return []native.Value{native.NewCarrier(TVaultIdentity)}
		})
}

// vaultIdentityHandler implements `Vault.identity <alias>`.
//
// The credential is resolved LAZILY: the ClientIdentity registered here
// reveals the vault secret when a handshake asks for a certificate, not
// now. That means a rotated secret is picked up without re-running the
// program, and a program that never dials never reveals anything.
func vaultIdentityHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	if err := validateVaultIdentityAlias(args, r); err != nil {
		return nil, err
	}
	alias, _ := args[0].AsConcreteString()
	name := "vault:" + alias
	native.RegisterClientIdentity(r, name, capabilities.IdentityFunc(
		func(capabilities.CertRequest) (*tls.Certificate, error) {
			return vaultCertificate(r, alias)
		}))
	return []native.Value{core.NewExtension(TVaultIdentity, &vaultIdentityRef{alias: alias, name: name})}, nil
}

// vaultCertificate reveals the alias through the host vault backend and
// parses it as a PEM bundle. The reveal happens in HOST Go under the
// vault's own `reveal` gate — the bytes never become a boru value, which
// is the whole point of the handle.
func vaultCertificate(r *native.Registry, alias string) (*tls.Certificate, error) {
	spec := hostVaultSpec(r)
	if spec == nil {
		return nil, fmt.Errorf("vault identity %q: no vault backend is registered", alias)
	}
	out, err := spec.Do("reveal", map[string]any{"alias": alias})
	if err != nil {
		return nil, fmt.Errorf("vault identity %q: %w", alias, err)
	}
	pemText, ok := out.(string)
	if !ok || pemText == "" {
		return nil, fmt.Errorf("vault identity %q: secret is not PEM text", alias)
	}
	// One bundle holding both the chain and the key is the usual shape
	// for a vault-held credential; each parser skips the blocks it does
	// not recognise, so the same bytes serve as both arguments.
	cert, kErr := tls.X509KeyPair([]byte(pemText), []byte(pemText))
	if kErr != nil {
		return nil, fmt.Errorf("vault identity %q: %w", alias, kErr)
	}
	return &cert, nil
}

func init() {
	// Teach ParseTLSOpts this handle shape, so `tls: {identity: h}`
	// accepts it alongside an Atom or String name.
	native.TLSIdentityHandles = append(native.TLSIdentityHandles, VaultIdentityName)
}
