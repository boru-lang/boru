package modules

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"strings"
	"testing"
	"time"

	core "github.com/boru-lang/boru/core/go"
	"github.com/boru-lang/boru/lang/go/capabilities"
	"github.com/boru-lang/boru/lang/go/native"
)

// mintTLSClientCert issues a self-signed client certificate that doubles
// as the CA the server trusts, so a mutual handshake needs no fixtures
// and no network.
func mintTLSClientCert(t *testing.T) (certPEM, keyPEM []byte, pool *x509.CertPool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(3),
		Subject:      pkix.Name{CommonName: "boru-vault-client"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		// SANs a server might authorize on, so `Net.peer-cert` has both
		// name lists to report rather than two empty ones.
		DNSNames:              []string{"client.internal"},
		EmailAddresses:        []string{"ops@example.invalid"},
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	pool = x509.NewCertPool()
	pool.AddCert(leaf)
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}),
		pool
}

// vaultIdentReg installs a fake vault backend that serves one alias.
// reveals counts how many times the secret was actually read, which is
// what proves the credential resolves lazily.
func vaultIdentReg(t *testing.T, alias, secret string, reveals *int) *native.Registry {
	t.Helper()
	r := nscReg(t)
	if err := RegisterHostVault(r, VaultSpec{
		Name: "fake",
		Do: func(op string, params map[string]any) (any, error) {
			if op != "reveal" {
				return nil, nil
			}
			if params["alias"] != alias {
				return nil, errFakeVaultMiss
			}
			*reveals++
			return secret, nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	return r
}

var errFakeVaultMiss = &fakeVaultErr{}

type fakeVaultErr struct{}

func (*fakeVaultErr) Error() string { return "no such secret" }

// Re-registering an already-installed type path records an init error
// and returns nil rather than panicking (ADR-005).
func TestVaultIdentityRegisterTypeDuplicate(t *testing.T) {
	saved := native.SwapTypeInitErrs(nil)
	t.Cleanup(func() { native.SwapTypeInitErrs(saved) })

	if tt := registerVaultType("Ideal/VaultIdentity", 5012, vaultIdentityBehavior{}); tt != nil {
		t.Fatalf("duplicate registration must return nil, got %v", tt)
	}
	err := native.TypeInitError()
	if err == nil || !strings.Contains(err.Error(), "vault: register Ideal/VaultIdentity") {
		t.Fatalf("expected recorded vault init error, got %v", err)
	}
}

// A vault-held credential authenticates a real mutual-TLS handshake,
// and the private key never becomes a boru value at any point.
func TestVaultIdentityMutualTLS(t *testing.T) {
	certPEM, keyPEM, clientCAs := mintTLSClientCert(t)
	addr, serverCA := mtlsEchoServer(t, clientCAs)

	reveals := 0
	r := vaultIdentReg(t, "acme-mtls", string(certPEM)+string(keyPEM), &reveals)

	out, err := vaultIdentityHandler(
		[]native.Value{native.NewString("acme-mtls")}, nil, nil, r)
	if err != nil {
		t.Fatalf("Vault.identity: %v", err)
	}
	handle := out[0]

	// LAZY: naming the credential must not read the secret. A program
	// that never dials never reveals anything.
	if reveals != 0 {
		t.Errorf("secret revealed %d times before any handshake", reveals)
	}

	sock, cErr := connectRawHandler([]native.Value{
		nscMap("tcp", native.NewString(addr), "tls", nscMap(
			"ca", native.NewString(serverCA),
			"identity", handle,
		)),
	}, nil, nil, r)
	if cErr != nil {
		t.Fatalf("mTLS dial with a vault identity: %v", cErr)
	}
	if reveals != 1 {
		t.Errorf("secret revealed %d times, want exactly 1 (at the handshake)", reveals)
	}
	_, _ = closeHandler([]native.Value{sock[0]}, nil, nil, r)
}

// The handle is opaque: it redacts when formatted, and carries no path
// back to the secret. This is the whole reason it is not a String —
// Vault.reveal returns one, and a private key must not be one.
func TestVaultIdentityIsOpaque(t *testing.T) {
	reveals := 0
	r := vaultIdentReg(t, "acme", "irrelevant", &reveals)
	out, err := vaultIdentityHandler(
		[]native.Value{native.NewString("acme")}, nil, nil, r)
	if err != nil {
		t.Fatal(err)
	}
	handle := out[0]

	if got := handle.String(); got != "<vault identity>" {
		t.Errorf("Format = %q, want a redacted form", got)
	}
	if strings.Contains(handle.String(), "acme") {
		t.Error("the handle must not print its alias")
	}
	// It is not a String and not a Map: no accessor reaches inside it.
	if _, ok := native.AsMap(handle); ok == nil {
		t.Error("a credential handle must not be map-backed")
	}
	if reveals != 0 {
		t.Error("formatting a handle must not reveal the secret")
	}

	// It matches its own nominal type and nothing else — `tls:` passes
	// the handle as Any, so this is the only place the type is asserted.
	b := TVaultIdentity.Behavior()
	if !b.Match(handle, TVaultIdentity) {
		t.Error("a handle must match VaultIdentity")
	}
	if b.Match(native.NewInteger(1), TVaultIdentity) {
		t.Error("an Integer must not match VaultIdentity")
	}
	if b.Match(handle, TSocket) {
		t.Error("a handle must not match another module's handle type")
	}
}

// Negative: a bad alias, a missing backend, and a secret that is not a
// usable PEM pair each fail at the handshake with a clear error rather
// than a nil deref or a silent no-certificate.
func TestVaultIdentityFailures(t *testing.T) {
	reveals := 0

	// Alias must be a non-empty String.
	r := vaultIdentReg(t, "acme", "x", &reveals)
	if _, err := vaultIdentityHandler(
		[]native.Value{native.NewString("")}, nil, nil, r); err == nil {
		t.Error("an empty alias must be rejected")
	}

	// Secret that is not a PEM keypair.
	_, _, clientCAs := mintTLSClientCert(t)
	addr, serverCA := mtlsEchoServer(t, clientCAs)
	rBad := vaultIdentReg(t, "junk", "not a pem", &reveals)
	h, err := vaultIdentityHandler([]native.Value{native.NewString("junk")}, nil, nil, rBad)
	if err != nil {
		t.Fatal(err)
	}
	_, dErr := connectRawHandler([]native.Value{
		nscMap("tcp", native.NewString(addr), "tls", nscMap(
			"ca", native.NewString(serverCA), "identity", h[0])),
	}, nil, nil, rBad)
	if dErr == nil {
		t.Error("a non-PEM secret must fail the handshake")
	}

	// No vault backend registered at all.
	rNone := nscReg(t)
	hNone, err := vaultIdentityHandler([]native.Value{native.NewString("x")}, nil, nil, rNone)
	if err != nil {
		t.Fatal(err)
	}
	if _, nErr := connectRawHandler([]native.Value{
		nscMap("tcp", native.NewString(addr), "tls", nscMap(
			"ca", native.NewString(serverCA), "identity", hNone[0])),
	}, nil, nil, rNone); nErr == nil {
		t.Error("no vault backend must fail, not silently present nothing")
	}
}

// VaultIdentityName is the hook ParseTLSOpts uses; it must recognise a
// handle and decline everything else.
func TestVaultIdentityNameHook(t *testing.T) {
	reveals := 0
	r := vaultIdentReg(t, "acme", "x", &reveals)
	out, err := vaultIdentityHandler([]native.Value{native.NewString("acme")}, nil, nil, r)
	if err != nil {
		t.Fatal(err)
	}
	name, ok := VaultIdentityName(out[0])
	if !ok || name != "vault:acme" {
		t.Errorf("VaultIdentityName = %q %v, want \"vault:acme\" true", name, ok)
	}
	for _, other := range []native.Value{
		native.NewString("acme"), native.NewInteger(1), core.NewExtension(TSocket, nil),
	} {
		if _, ok := VaultIdentityName(other); ok {
			t.Errorf("VaultIdentityName accepted %v", other)
		}
	}
	// Equal compares by registered name; a handle differs from a
	// non-handle without panicking.
	same, _ := vaultIdentityHandler([]native.Value{native.NewString("acme")}, nil, nil, r)
	if !TVaultIdentity.Behavior().Equal(out[0], same[0]) {
		t.Error("two handles for the same alias must compare equal")
	}
	if TVaultIdentity.Behavior().Equal(out[0], native.NewString("acme")) {
		t.Error("a handle must not equal a String")
	}
	if got := TVaultIdentity.Behavior().Format(native.NewString("x")); got != "<vault identity>" {
		t.Errorf("Format of a non-handle = %q", got)
	}
}

// The identity registered by the handle is a real ClientIdentity that
// the shared resolver finds under its namespaced name.
func TestVaultIdentityRegistersUnderName(t *testing.T) {
	reveals := 0
	r := vaultIdentReg(t, "acme", "x", &reveals)
	if _, err := vaultIdentityHandler(
		[]native.Value{native.NewString("acme")}, nil, nil, r); err != nil {
		t.Fatal(err)
	}
	ids := native.HostClientIdents(r)
	id, ok := ids["vault:acme"]
	if !ok || id == nil {
		t.Fatalf("no identity registered under vault:acme (%v)", ids)
	}
	if _, cErr := id.Certificate(capabilities.CertRequest{Host: "h"}); cErr == nil {
		t.Error("the junk secret should fail to parse as a keypair")
	}
}

// Every way the vault can decline to produce a credential surfaces as an
// error naming the alias, never as a nil certificate the handshake would
// read as "this client simply has none".
func TestVaultCertificateBackendFailures(t *testing.T) {
	reveals := 0
	r := vaultIdentReg(t, "acme", "x", &reveals)

	// The backend declines the alias.
	if _, err := vaultCertificate(r, "not-acme"); err == nil ||
		!strings.Contains(err.Error(), "not-acme") {
		t.Errorf("a rejected alias must error and name itself, got %v", err)
	}

	// The backend answers, but with something that is not secret text.
	rEmpty := nscReg(t)
	if err := RegisterHostVault(rEmpty, VaultSpec{
		Name: "empty",
		Do:   func(string, map[string]any) (any, error) { return "", nil },
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := vaultCertificate(rEmpty, "acme"); err == nil ||
		!strings.Contains(err.Error(), "not PEM text") {
		t.Errorf("an empty reveal must error, got %v", err)
	}
}
