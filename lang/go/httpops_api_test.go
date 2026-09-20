package lang_test

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/boru-lang/boru/lang/go"
)

// stubRoundTripper serves a canned response and counts calls, so a test
// can prove fetch went through the installed transport and never
// touched the network.
type stubRoundTripper struct {
	hits int
	url  string
}

func (s *stubRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	s.hits++
	s.url = req.URL.String()
	return &http.Response{
		StatusCode: 204,
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader("")),
		Request:    req,
	}, nil
}

type stubHTTPOps struct{ rt http.RoundTripper }

func (s stubHTTPOps) Transport(_ lang.TLSProfile, _ lang.ClientIdentity) (http.RoundTripper, error) {
	return s.rt, nil
}

// SetHTTPOps redirects fetch through a host-supplied transport. The URL
// is unroutable, so a passing test also proves no real request was made.
func TestSetHTTPOps(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatal(err)
	}
	rt := &stubRoundTripper{}
	a.SetHTTPOps(stubHTTPOps{rt: rt})

	if _, err = a.RunInterp(`import "boru:net"`); err != nil {
		t.Fatal(err)
	}
	res, err := a.RunInterp(
		`(convert Map (Net.fetch "https://stub.invalid/thing")).status`)
	if err != nil {
		t.Fatal(err)
	}
	if rt.hits != 1 {
		t.Fatalf("installed transport called %d times, want 1", rt.hits)
	}
	if rt.url != "https://stub.invalid/thing" {
		t.Errorf("transport saw %q", rt.url)
	}
	if len(res) != 1 || fmt.Sprint(res[0]) != "204" {
		t.Errorf("status = %v, want [204]", res)
	}
}

// RegisterClientIdentity is the host's entry point for mutual TLS. The
// guest names the identity; it can never read or construct one, so the
// assertion here is that naming a REGISTERED one resolves while naming
// an unregistered one fails.
func TestRegisterClientIdentity(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatal(err)
	}
	a.RegisterClientIdentity("acme", lang.ClientIdentity(
		identityStub(func(lang.CertRequest) (*tls.Certificate, error) { return nil, nil })))

	if _, err = a.RunInterp(`import "boru:net"`); err != nil {
		t.Fatal(err)
	}
	// The host is unroutable, so this fails at the dial — the assertion
	// is that it gets that far, i.e. the NAME resolved. (That the
	// identity is then actually consulted during a handshake is proven
	// by native's TestFetchMutualTLS against a real mTLS server.)
	_, known := a.RunInterp(`Net.fetch {url: "https://stub.invalid/x"  tls: {identity: acme/q}}`)
	if known != nil && strings.Contains(known.Error(), "no client identity") {
		t.Errorf("a registered identity must resolve, got %v", known)
	}

	// An unregistered name is declined before any dial.
	_, unknown := a.RunInterp(`Net.fetch {url: "https://stub.invalid/x"  tls: {identity: nope/q}}`)
	if unknown == nil || !strings.Contains(unknown.Error(), `no client identity named "nope"`) {
		t.Errorf("got %v, want an unknown-identity error", unknown)
	}
}

type identityStub func(lang.CertRequest) (*tls.Certificate, error)

func (f identityStub) Certificate(req lang.CertRequest) (*tls.Certificate, error) { return f(req) }
