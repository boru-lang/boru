package native

import (
	"crypto/tls"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/boru-lang/boru/lang/go/capabilities"
	"github.com/boru-lang/boru/lang/go/policy"
)

// tlsTestServer starts a local HTTPS server and returns it plus its
// self-signed certificate in PEM form, which doubles as the private CA
// a client must be told to trust. Nothing here touches a real network.
func tlsTestServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte("secure"))
	}))
	t.Cleanup(ts.Close)
	caPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: ts.Certificate().Raw,
	})
	return ts, string(caPEM)
}

func tlsFetch(t *testing.T, r *Registry, url string, tlsOpts *OrderedMap) ([]Value, error) {
	t.Helper()
	req := NewOrderedMap()
	req.Set("url", NewString(url))
	if tlsOpts != nil {
		req.Set("tls", NewMap(tlsOpts))
	}
	return MintFetchTypes(r).doFetch(req, r)
}

func tlsOpt(pairs ...any) *OrderedMap {
	m := NewOrderedMap()
	for i := 0; i+1 < len(pairs); i += 2 {
		m.Set(pairs[i].(string), pairs[i+1].(Value))
	}
	return m
}

// A private-CA endpoint is reachable when its CA is supplied, and NOT
// reachable without it. The negative half is the point: it proves the
// success is verification passing, not verification being skipped.
func TestFetchTLSPrivateCA(t *testing.T) {
	ts, caPEM := tlsTestServer(t)
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}

	if _, err := tlsFetch(t, r, ts.URL, nil); err == nil {
		t.Fatal("expected an untrusted-CA failure with no ca: supplied")
	}

	out, err := tlsFetch(t, r, ts.URL, tlsOpt("ca", NewString(caPEM)))
	if err != nil {
		t.Fatalf("fetch with ca: %v", err)
	}
	m, _ := AsMap(out[0])
	if m == nil {
		t.Fatal("expected a response map")
	}
	status, _ := m.Get("status")
	if n, _ := AsInteger(status); n != 200 {
		t.Errorf("status = %v, want 200", status)
	}
}

// Bytes is the natural shape for a PEM read through boru:io, so it must
// be accepted alongside String.
func TestFetchTLSCAAsBytes(t *testing.T) {
	ts, caPEM := tlsTestServer(t)
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tlsFetch(t, r, ts.URL, tlsOpt("ca", NewBytesValue([]byte(caPEM)))); err != nil {
		t.Fatalf("fetch with Bytes ca: %v", err)
	}
}

// verify:false reaches an untrusted endpoint — and with no policy
// installed it is allowed, matching the historical no-policy default.
func TestFetchTLSInsecure(t *testing.T) {
	ts, _ := tlsTestServer(t)
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tlsFetch(t, r, ts.URL, tlsOpt("verify", NewBoolean(false))); err != nil {
		t.Fatalf("verify:false should reach the server: %v", err)
	}
}

// Negative: a bad PEM must fail loudly. Silently producing an empty
// trust pool would reject everything with a confusing chain error.
func TestFetchTLSBadCA(t *testing.T) {
	ts, _ := tlsTestServer(t)
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	_, fErr := tlsFetch(t, r, ts.URL, tlsOpt("ca", NewString("-----BEGIN CERTIFICATE-----\nnope\n")))
	if fErr == nil || !strings.Contains(fErr.Error(), "no usable certificate") {
		t.Fatalf("expected a bad-PEM error, got %v", fErr)
	}
}

// Negative: every malformed tls: option is rejected, including an
// unknown key — a misspelled `verifiy:` must not be silently dropped.
func TestParseTLSOptsRejects(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		opts *OrderedMap
		want string
	}{
		{"unknown key", tlsOpt("verifiy", NewBoolean(false)), `unknown option "verifiy"`},
		{"verify not bool", tlsOpt("verify", NewString("no")), "verify: must be a Boolean"},
		{"ca wrong type", tlsOpt("ca", NewInteger(7)), "ca: must be Bytes or a String"},
		{"sni empty", tlsOpt("sni", NewString("")), "sni: must be a non-empty String"},
		{"min unknown", tlsOpt("min", NewString("1.1")), `min: must be "1.2" or "1.3"`},
		{"min wrong type", tlsOpt("min", NewInteger(12)), `min: must be "1.2" or "1.3"`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, pErr := ParseTLSOpts(r, NewMap(c.opts), "fetch")
			if pErr == nil || !strings.Contains(pErr.Error(), c.want) {
				t.Fatalf("got %v, want it to mention %q", pErr, c.want)
			}
		})
	}
	// A non-map tls: value is rejected too.
	if _, pErr := ParseTLSOpts(r, NewString("yes"), "fetch"); pErr == nil {
		t.Error("a non-map tls: must be rejected")
	}
}

// The accepted options map onto the profile as documented.
func TestParseTLSOptsAccepts(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	p, pErr := ParseTLSOpts(r, NewMap(tlsOpt(
		"verify", NewBoolean(false),
		"sni", NewString("inner.example"),
		"min", NewString("1.3"),
		"ca", NewString("PEMDATA"),
	)), "fetch")
	if pErr != nil {
		t.Fatal(pErr)
	}
	want := capabilities.TLSProfile{
		Insecure:   true,
		RootsPEM:   "PEMDATA",
		ServerName: "inner.example",
		MinVersion: tls.VersionTLS13,
	}
	if p != want {
		t.Errorf("profile = %+v, want %+v", p, want)
	}
	// min:"1.2" is the other accepted spelling.
	p12, _ := ParseTLSOpts(r, NewMap(tlsOpt("min", NewString("1.2"))), "fetch")
	if p12.MinVersion != tls.VersionTLS12 {
		t.Errorf("min 1.2 → %v", p12.MinVersion)
	}
	// An empty map is the zero profile — the safe default, with
	// Insecure false rather than true.
	pz, _ := ParseTLSOpts(r, NewMap(NewOrderedMap()), "fetch")
	if pz != (capabilities.TLSProfile{}) {
		t.Errorf("empty tls: → %+v, want the zero profile", pz)
	}
}

// DefaultHTTPOps is a pure BUILDER: the zero profile stays on
// http.DefaultTransport, a configured profile gets a fresh transport
// carrying it, and nothing is cached here. Caching is
// ResolveTransport's job because a profile names its identity by a
// registry-scoped name — see TestResolveTransportCachePerRegistry.
func TestDefaultHTTPOpsBuilds(t *testing.T) {
	ops := capabilities.DefaultHTTPOps{}
	zero, err := ops.Transport(capabilities.TLSProfile{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if zero != http.DefaultTransport {
		t.Error("the zero profile must stay on http.DefaultTransport")
	}

	p := capabilities.TLSProfile{ServerName: "cache.example"}
	a, err := ops.Transport(p, nil)
	if err != nil {
		t.Fatal(err)
	}
	b, err := ops.Transport(p, nil)
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Error("DefaultHTTPOps must not cache — that is ResolveTransport's job")
	}
	if a == http.DefaultTransport {
		t.Error("a configured profile must not reuse the default transport")
	}
	other, err := ops.Transport(capabilities.TLSProfile{ServerName: "other.example"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if other == a {
		t.Error("different profiles must not share a transport")
	}

	// Setting TLSClientConfig otherwise disables automatic HTTP/2.
	tr, ok := a.(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport, got %T", a)
	}
	if !tr.ForceAttemptHTTP2 {
		t.Error("ForceAttemptHTTP2 must stay on, or TLS silently downgrades to HTTP/1.1")
	}
	if tr.TLSClientConfig == nil || tr.TLSClientConfig.ServerName != "cache.example" {
		t.Error("the profile must reach the tls.Config")
	}
}

// Negative: a bad PEM fails at transport construction, before any dial.
func TestDefaultHTTPOpsBadRoots(t *testing.T) {
	_, err := capabilities.DefaultHTTPOps{}.Transport(
		capabilities.TLSProfile{RootsPEM: "not a pem"}, nil)
	if err == nil {
		t.Fatal("expected a bad-PEM error")
	}
}

// verify:false is declined when a policy declares the network scope
// without listing tls-insecure — a declared scope default-denies ops it
// does not name, so this needs no extra wiring to be a real gate. The
// matching allow case proves the gate is opt-in, not unconditional.
func TestTLSInsecurePolicyGate(t *testing.T) {
	denyPol, err := policy.LoadInline(`{
		name: "net-no-insecure-tls"
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
	insecure := capabilities.TLSProfile{Insecure: true}
	pErr := CheckTLSPolicy(rDeny, insecure, "example.com", 443)
	if pErr == nil {
		t.Fatal("expected tls-insecure to be denied")
	}
	d, ok := pErr.(*policy.Denied)
	if !ok {
		t.Fatalf("expected *policy.Denied, got %T (%v)", pErr, pErr)
	}
	if d.Op != "tls-insecure" {
		t.Errorf("Denied.Op = %q, want \"tls-insecure\"", d.Op)
	}
	// A verifying profile is never gated, even under the same policy.
	if err := CheckTLSPolicy(rDeny, capabilities.TLSProfile{}, "example.com", 443); err != nil {
		t.Errorf("a verifying profile must not be gated: %v", err)
	}

	allowPol, err := policy.LoadInline(`{
		name: "net-allow-insecure-tls"
		scopes: {
			network: { words: { default: "deny", rules: [{ allow: ["connect", "tls-insecure"] }] } }
		}
	}`)
	if err != nil {
		t.Fatal(err)
	}
	rAllow, err := DefaultRegistryWithPolicy(allowPol)
	if err != nil {
		t.Fatal(err)
	}
	if err := CheckTLSPolicy(rAllow, insecure, "example.com", 443); err != nil {
		t.Errorf("a policy that allows tls-insecure must permit it: %v", err)
	}

	// No policy at all keeps the historical allow-everything default,
	// and a nil registry is a no-op rather than a panic.
	rNone, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if err := CheckTLSPolicy(rNone, insecure, "example.com", 443); err != nil {
		t.Errorf("no policy must not gate: %v", err)
	}
	if err := CheckTLSPolicy(nil, insecure, "example.com", 443); err != nil {
		t.Errorf("nil registry must be a no-op: %v", err)
	}
}

// End-to-end: a denied verify:false never reaches the wire.
func TestFetchTLSInsecureDeniedIsUnsent(t *testing.T) {
	ts, _ := tlsTestServer(t)
	pol, err := policy.LoadInline(`{
		name: "net-no-insecure-tls"
		scopes: {
			network: { words: { default: "deny", rules: [{ allow: ["connect"] }] } }
		}
	}`)
	if err != nil {
		t.Fatal(err)
	}
	r, err := DefaultRegistryWithPolicy(pol)
	if err != nil {
		t.Fatal(err)
	}
	before := r.Effects.Count()
	_, fErr := tlsFetch(t, r, ts.URL, tlsOpt("verify", NewBoolean(false)))
	if fErr == nil {
		t.Fatal("expected the fetch to be denied")
	}
	if _, ok := fErr.(*policy.Denied); !ok {
		t.Fatalf("expected *policy.Denied, got %T (%v)", fErr, fErr)
	}
	if after := r.Effects.Count(); after != before {
		t.Errorf("effects %d → %d: a denied fetch must send nothing", before, after)
	}
}
