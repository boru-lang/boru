package native

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/boru-lang/boru/lang/go/capabilities"
)

// stubTransport serves a canned response without touching the network,
// recording the request it saw.
type stubTransport struct {
	seen   *http.Request
	status int
	body   string
}

func (s *stubTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	s.seen = req
	return &http.Response{
		StatusCode: s.status,
		Header:     http.Header{"Content-Type": []string{"text/plain"}},
		Body:       io.NopCloser(strings.NewReader(s.body)),
		Request:    req,
	}, nil
}

// stubHTTPOps hands out a fixed transport, or fails.
type stubHTTPOps struct {
	rt  http.RoundTripper
	err error
}

func (s stubHTTPOps) Transport(_ capabilities.TLSProfile, _ capabilities.ClientIdentity) (http.RoundTripper, error) {
	return s.rt, s.err
}

// With no HTTPOps installed the default is served, and it yields
// http.DefaultTransport — the transport an *http.Client with a nil
// Transport field would itself have used, so installing nothing
// preserves the pre-seam behaviour.
func TestEffectiveHTTPOpsDefault(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	ops := EffectiveHTTPOps(r)
	if _, isDefault := ops.(capabilities.DefaultHTTPOps); !isDefault {
		t.Fatalf("expected DefaultHTTPOps with none installed, got %T", ops)
	}
	rt, tErr := ops.Transport(capabilities.TLSProfile{}, nil)
	if tErr != nil {
		t.Fatalf("default Transport: %v", tErr)
	}
	if rt != http.DefaultTransport {
		t.Errorf("expected http.DefaultTransport, got %#v", rt)
	}
}

// An installed HTTPOps displaces the default.
func TestEffectiveHTTPOpsInstalled(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	want := stubHTTPOps{rt: &stubTransport{status: 200}}
	SetHostHTTPOps(r, want)
	got := EffectiveHTTPOps(r)
	if _, isDefault := got.(capabilities.DefaultHTTPOps); isDefault {
		t.Fatal("installed HTTPOps was not returned")
	}
	if _, ok := got.(stubHTTPOps); !ok {
		t.Fatalf("expected stubHTTPOps, got %T", got)
	}
}

// Negative: a nil registry or nil ops installs nothing, and the default
// still resolves. The nil-ops case must not overwrite the slot with a
// nil that EffectiveHTTPOps would then have to defend against.
func TestSetHostHTTPOpsNilGuards(t *testing.T) {
	SetHostHTTPOps(nil, stubHTTPOps{}) // must not panic

	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	SetHostHTTPOps(r, nil)
	if _, isDefault := EffectiveHTTPOps(r).(capabilities.DefaultHTTPOps); !isDefault {
		t.Error("nil ops should leave the default in place")
	}
}

// fetch routes through the installed transport: the request never
// reaches the network, and the canned response is what comes back.
func TestFetchUsesInstalledTransport(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	rt := &stubTransport{status: 201, body: "hello from the stub"}
	SetHostHTTPOps(r, stubHTTPOps{rt: rt})

	ft := MintFetchTypes(r)
	out, fErr := ft.fetchStringHandler(
		[]Value{NewString("https://stub.invalid/thing")}, nil, nil, r)
	if fErr != nil {
		t.Fatalf("fetch: %v", fErr)
	}
	if rt.seen == nil {
		t.Fatal("installed transport was never called")
	}
	if got := rt.seen.URL.String(); got != "https://stub.invalid/thing" {
		t.Errorf("transport saw url %q", got)
	}
	m, _ := AsMap(out[0])
	if m == nil {
		t.Fatal("expected a map-backed response")
	}
	status, _ := m.Get("status")
	if n, _ := AsInteger(status); n != 201 {
		t.Errorf("status = %v, want 201", status)
	}
	body, _ := m.Get("body")
	if s, _ := AsString(body); s != "hello from the stub" {
		t.Errorf("body = %q", s)
	}
}

// Negative: an HTTPOps that declines fails the fetch with a transport
// error, and — because the transport is resolved BEFORE the C1 effect
// fence — the attempt is provably unsent and stays off the ledger.
func TestFetchTransportErrorIsUnsent(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	SetHostHTTPOps(r, stubHTTPOps{err: errors.New("no transport for you")})

	ft := MintFetchTypes(r)
	before := r.Effects.Count()
	_, fErr := ft.fetchStringHandler(
		[]Value{NewString("https://stub.invalid/thing")}, nil, nil, r)
	if fErr == nil {
		t.Fatal("expected a transport error")
	}
	// The code must be `transport` (NETWORK-CLIENTS.0.md §8.2), not a
	// bare error that would surface to guests as internal_error — the
	// point is that `do […] error [case …]` can discriminate it.
	if !strings.Contains(fErr.Error(), "transport") ||
		!strings.Contains(fErr.Error(), "no transport for you") {
		t.Errorf("error = %v, want it to name the transport failure", fErr)
	}
	var ae *BoruError
	if !errors.As(fErr, &ae) {
		t.Fatalf("error is %T, want a coded *BoruError", fErr)
	}
	if ae.Code != "transport" {
		t.Errorf("error code = %q, want \"transport\"", ae.Code)
	}
	if after := r.Effects.Count(); after != before {
		t.Errorf("effects %d → %d: a declined transport sent nothing", before, after)
	}
}

// The strict accessor's SUCCESS path, and the nil-Data guard. A type
// literal has Data == nil (lang/go/CLAUDE.md, "Type literals have nil
// Data"), so an accessor reaching one must read a miss rather than
// panic — the TestTypeLiteralNoPanic discipline applied to this family.
func TestFetchAccessorHandlers(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	ft := MintFetchTypes(r)

	om := NewOrderedMap()
	om.Set("status", NewInteger(200))
	resp := NewMap(om)
	resp.Parent = ft.Response

	// getr succeeds on a present field (the miss half is pinned by the
	// spec rows; this is the half they cannot reach through `do/error`).
	out, gErr := fetchGetrHandler([]Value{NewAtom("status"), resp}, nil, nil, r)
	if gErr != nil || len(out) != 1 {
		t.Fatalf("getr on a present field: %v %v", out, gErr)
	}
	if n, _ := AsInteger(out[0]); n != 200 {
		t.Errorf("getr = %v, want 200", out[0])
	}

	// A type literal carries no map: every accessor must degrade, not
	// panic.
	lit := NewTypeLiteral(ft.Response)
	if got, hErr := fetchGetHandler([]Value{NewAtom("status"), lit}, nil, nil, r); hErr != nil ||
		len(got) != 1 || !IsBareTypeNode(got[0]) {
		t.Errorf("dot on a type literal = %v %v, want none", got, hErr)
	}
	if _, sErr := fetchGetrHandler([]Value{NewAtom("status"), lit}, nil, nil, r); sErr == nil {
		t.Error("getr on a type literal must raise, not panic")
	}
	if got, hErr := fetchHasHandler([]Value{NewAtom("status"), lit}, nil, nil, r); hErr != nil ||
		len(got) != 1 {
		t.Fatalf("has on a type literal: %v %v", got, hErr)
	} else if b, _ := AsBoolean(got[0]); b {
		t.Error("has on a type literal must be false")
	}
}
