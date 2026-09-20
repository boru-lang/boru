package native

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/boru-lang/boru/lang/go/capabilities"
	"github.com/boru-lang/boru/lang/go/policy"
)

// mintClientCert issues a self-signed client certificate usable both as
// the credential and as the CA the server trusts. Deterministic inputs
// aside from the key, and entirely local — no network, no fixtures.
func mintClientCert(t *testing.T) (certPEM, keyPEM []byte, pool *x509.CertPool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "boru-test-client"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
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
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	pool = x509.NewCertPool()
	pool.AddCert(leaf)
	return certPEM, keyPEM, pool
}

// mtlsServer starts an HTTPS server that REQUIRES and verifies a client
// certificate, and returns it plus its own CA in PEM form.
func mtlsServer(t *testing.T, clientCAs *x509.CertPool) (*httptest.Server, string) {
	t.Helper()
	ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if len(req.TLS.PeerCertificates) == 0 {
			w.WriteHeader(401)
			return
		}
		w.WriteHeader(200)
		_, _ = w.Write([]byte(req.TLS.PeerCertificates[0].Subject.CommonName))
	}))
	ts.TLS = &tls.Config{
		ClientCAs:  clientCAs,
		ClientAuth: tls.RequireAndVerifyClientCert,
		MinVersion: tls.VersionTLS12,
	}
	ts.StartTLS()
	t.Cleanup(ts.Close)
	caPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: ts.Certificate().Raw,
	})
	return ts, string(caPEM)
}

// The full mutual-TLS round trip: a server that requires a client
// certificate accepts a request carrying a registered identity, and the
// same request without one fails. The negative half is what proves the
// success came from the certificate rather than from a lax server.
func TestFetchMutualTLS(t *testing.T) {
	certPEM, keyPEM, clientCAs := mintClientCert(t)
	ts, serverCA := mtlsServer(t, clientCAs)

	id, err := capabilities.StaticIdentity(certPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	RegisterClientIdentity(r, "acme", id)

	// With the identity: the handshake completes and the server sees
	// our certificate's subject.
	out, fErr := tlsFetch(t, r, ts.URL, tlsOpt(
		"ca", NewString(serverCA),
		"identity", NewAtom("acme"),
	))
	if fErr != nil {
		t.Fatalf("mTLS fetch: %v", fErr)
	}
	m, _ := AsMap(out[0])
	if m == nil {
		t.Fatal("expected a response map")
	}
	status, _ := m.Get("status")
	if n, _ := AsInteger(status); n != 200 {
		t.Fatalf("status = %v, want 200", status)
	}
	body, _ := m.Get("body")
	if s, _ := AsString(body); s != "boru-test-client" {
		t.Errorf("server saw subject %q, want boru-test-client", s)
	}

	// Without the identity: the server declines the handshake.
	if _, noID := tlsFetch(t, r, ts.URL, tlsOpt("ca", NewString(serverCA))); noID == nil {
		t.Error("a server requiring a client cert must reject a request without one")
	}
}

// Negative: naming an unregistered identity fails loudly rather than
// silently presenting nothing — a silent miss would surface later as an
// opaque server-side rejection.
func TestFetchUnknownIdentity(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	_, fErr := tlsFetch(t, r, "https://stub.invalid/x", tlsOpt("identity", NewAtom("nope")))
	if fErr == nil || !strings.Contains(fErr.Error(), `no client identity named "nope"`) {
		t.Fatalf("got %v, want an unknown-identity error", fErr)
	}
}

// identity: accepts an Atom (idiomatic) or a String (computed), and
// rejects anything else.
func TestParseTLSIdentityArg(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range []Value{NewAtom("acme"), NewString("acme")} {
		p, pErr := ParseTLSOpts(r, NewMap(tlsOpt("identity", v)), "fetch")
		if pErr != nil || p.Identity != "acme" {
			t.Errorf("identity %v → %q, %v", v, p.Identity, pErr)
		}
	}
	for _, bad := range []Value{NewInteger(1), NewString("")} {
		if _, pErr := ParseTLSOpts(r, NewMap(tlsOpt("identity", bad)), "fetch"); pErr == nil {
			t.Errorf("identity %v must be rejected", bad)
		}
	}
}

// Registration plumbing: lazy map creation, nil guards, and the
// policy-uninstall branch — a credential must not stay reachable in a
// profile that disables networking.
func TestClientIdentityRegistration(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if HostClientIdents(r) != nil {
		t.Error("no identities should be installed initially")
	}
	id := capabilities.IdentityFunc(func(capabilities.CertRequest) (*tls.Certificate, error) {
		return nil, nil
	})
	RegisterClientIdentity(r, "a", id)
	RegisterClientIdentity(r, "b", id)
	if got := len(HostClientIdents(r)); got != 2 {
		t.Errorf("registered %d identities, want 2", got)
	}
	// Nil guards.
	RegisterClientIdentity(nil, "x", id)
	RegisterClientIdentity(r, "", id)
	RegisterClientIdentity(r, "x", nil)
	if got := len(HostClientIdents(r)); got != 2 {
		t.Errorf("nil guards changed the map to %d entries", got)
	}
	SetHostClientIdents(nil, nil) // must not panic

	// A policy that uninstalls network removes the credential slot.
	pol, err := policy.LoadInline(`{
		name: "no-net"
		scopes: { network: { install: false } }
	}`)
	if err != nil {
		t.Fatal(err)
	}
	rNoNet, err := DefaultRegistryWithPolicy(pol)
	if err != nil {
		t.Fatal(err)
	}
	SetHostClientIdents(rNoNet, map[string]capabilities.ClientIdentity{"a": id})
	if HostClientIdents(rNoNet) != nil {
		t.Error("network.install=false must leave no identity reachable")
	}
}

// An identity that declines returns Go's empty certificate rather than
// an error, and one that fails propagates the failure.
func TestIdentityDeclineAndError(t *testing.T) {
	certPEM, keyPEM, clientCAs := mintClientCert(t)
	ts, serverCA := mtlsServer(t, clientCAs)
	_ = certPEM
	_ = keyPEM

	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	RegisterClientIdentity(r, "declines",
		capabilities.IdentityFunc(func(capabilities.CertRequest) (*tls.Certificate, error) {
			return nil, nil
		}))
	// Declining leaves the server with no certificate to verify, so the
	// handshake fails — but from the SERVER's compile failure, not a nil deref.
	if _, fErr := tlsFetch(t, r, ts.URL, tlsOpt(
		"ca", NewString(serverCA), "identity", NewAtom("declines"),
	)); fErr == nil {
		t.Error("declining to present a cert must still fail against a requiring server")
	}
}

// Equal profiles reuse a transport within a registry, and — the point of
// caching per registry — the same identity NAME in two registries does
// not share one.
func TestResolveTransportCachePerRegistry(t *testing.T) {
	certPEM, keyPEM, _ := mintClientCert(t)
	id, err := capabilities.StaticIdentity(certPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	r1, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r2, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	RegisterClientIdentity(r1, "acme", id)
	RegisterClientIdentity(r2, "acme", id)

	p := capabilities.TLSProfile{Identity: "acme"}
	a, err := ResolveTransport(r1, p, "fetch")
	if err != nil {
		t.Fatal(err)
	}
	again, err := ResolveTransport(r1, p, "fetch")
	if err != nil {
		t.Fatal(err)
	}
	if a != again {
		t.Error("the same profile in the same registry must reuse its transport")
	}
	other, err := ResolveTransport(r2, p, "fetch")
	if err != nil {
		t.Fatal(err)
	}
	if other == a {
		t.Error("two registries must not share a transport for an identically-named identity")
	}

	// The zero profile stays on the shared default.
	zero, err := ResolveTransport(r1, capabilities.TLSProfile{}, "fetch")
	if err != nil {
		t.Fatal(err)
	}
	if zero != http.DefaultTransport {
		t.Error("the zero profile must stay on http.DefaultTransport")
	}
}

// client-cert is gated exactly like tls-insecure: a declared network
// scope that does not list it denies, one that does allows, and the
// identity name reaches the policy args so a profile can allow-list
// per host.
func TestClientCertPolicyGate(t *testing.T) {
	denyPol, err := policy.LoadInline(`{
		name: "net-no-client-cert"
		scopes: {
			network: { words: { default: "deny", rules: [{ allow: ["connect"] }] } }
		}
	}`)
	if err != nil {
		t.Fatal(err)
	}
	rDeny, err := DefaultRegistryWithPolicy(denyPol)
	if err != nil {
		t.Fatal(err)
	}
	p := capabilities.TLSProfile{Identity: "acme"}
	pErr := CheckTLSPolicy(rDeny, p, "example.com", 443)
	if pErr == nil {
		t.Fatal("expected client-cert to be denied")
	}
	d, ok := pErr.(*policy.Denied)
	if !ok {
		t.Fatalf("expected *policy.Denied, got %T (%v)", pErr, pErr)
	}
	if d.Op != "client-cert" {
		t.Errorf("Denied.Op = %q, want \"client-cert\"", d.Op)
	}
	if d.Args["identity"] != "acme" {
		t.Errorf("identity missing from policy args: %v", d.Args)
	}

	allowPol, err := policy.LoadInline(`{
		name: "net-allow-client-cert"
		scopes: {
			network: { words: { default: "deny", rules: [{ allow: ["connect", "client-cert"] }] } }
		}
	}`)
	if err != nil {
		t.Fatal(err)
	}
	rAllow, err := DefaultRegistryWithPolicy(allowPol)
	if err != nil {
		t.Fatal(err)
	}
	if err := CheckTLSPolicy(rAllow, p, "example.com", 443); err != nil {
		t.Errorf("a policy allowing client-cert must permit it: %v", err)
	}
}

// StaticIdentity and FileIdentity validate the pair at REGISTRATION,
// so a mismatched or malformed cert/key fails here rather than at the
// first handshake — where it would surface as an opaque TLS error.
func TestIdentityConstructors(t *testing.T) {
	certPEM, keyPEM, _ := mintClientCert(t)

	if _, err := capabilities.StaticIdentity(certPEM, keyPEM); err != nil {
		t.Fatalf("StaticIdentity: %v", err)
	}
	if _, err := capabilities.StaticIdentity([]byte("not a pem"), keyPEM); err == nil {
		t.Error("StaticIdentity must reject a malformed certificate")
	}

	dir := t.TempDir()
	certPath := filepath.Join(dir, "client.pem")
	keyPath := filepath.Join(dir, "client.key")
	if err := os.WriteFile(certPath, certPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	id, err := capabilities.FileIdentity(certPath, keyPath)
	if err != nil {
		t.Fatalf("FileIdentity: %v", err)
	}
	cert, err := id.Certificate(capabilities.CertRequest{Host: "example.com"})
	if err != nil || cert == nil {
		t.Fatalf("FileIdentity certificate: %v %v", cert, err)
	}
	if _, err := capabilities.FileIdentity(filepath.Join(dir, "absent.pem"), keyPath); err == nil {
		t.Error("FileIdentity must reject a missing file")
	}
}

// An identity that fails aborts the handshake with its own error
// rather than silently proceeding without a certificate.
func TestIdentityErrorAbortsHandshake(t *testing.T) {
	_, _, clientCAs := mintClientCert(t)
	ts, serverCA := mtlsServer(t, clientCAs)

	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	RegisterClientIdentity(r, "broken",
		capabilities.IdentityFunc(func(capabilities.CertRequest) (*tls.Certificate, error) {
			return nil, errors.New("keystore unavailable")
		}))
	_, fErr := tlsFetch(t, r, ts.URL, tlsOpt(
		"ca", NewString(serverCA), "identity", NewAtom("broken"),
	))
	if fErr == nil || !strings.Contains(fErr.Error(), "keystore unavailable") {
		t.Fatalf("got %v, want the identity's own failure", fErr)
	}
}

// A malformed tls: map fails inside doFetch, before any transport is
// resolved or any packet sent.
func TestFetchRejectsBadTLSMap(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	before := r.Effects.Count()
	_, fErr := tlsFetch(t, r, "https://stub.invalid/x", tlsOpt("verifiy", NewBoolean(false)))
	if fErr == nil || !strings.Contains(fErr.Error(), `unknown option "verifiy"`) {
		t.Fatalf("got %v, want an unknown-option error", fErr)
	}
	if after := r.Effects.Count(); after != before {
		t.Errorf("effects %d → %d: a rejected option map sends nothing", before, after)
	}
}
