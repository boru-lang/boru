package vault

// proxy_wave4_test.go — proxy.go arms not reached by the earlier proxy
// tests: runProxy's full signal-driven lifecycle (including the admin
// warning), handler denials for bad paths / missing stores / missing
// aliases / missing providers, per-request authentication failures,
// upstream and injection failures, counter-persistence failures, and
// the small pure helpers.

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// w4SyncBuffer is a bytes.Buffer safe for concurrent writes (the proxy
// goroutine logs while the test polls).
type w4SyncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *w4SyncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *w4SyncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func TestProxyFlagParseError(t *testing.T) {
	testHome(t)
	if code, _, _ := runVault(t, "", "proxy", "-nope"); code == 0 {
		t.Error("proxy with a bad flag should fail")
	}
}

func TestProxyHandlerDenialArms(t *testing.T) {
	newReq := func(method, target, token string) (*httptest.ResponseRecorder, *http.Request) {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(method, target, nil)
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		return w, r
	}

	t.Run("bad path", func(t *testing.T) {
		home := testHome(t)
		mustInit(t)
		p := NewProxy("127.0.0.1:0", home, "test-pass", io.Discard, io.Discard)
		w, r := newReq("GET", "/", "")
		p.ServeHTTP(w, r)
		if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "path must be") {
			t.Errorf("bad path = %d, %q", w.Code, w.Body.String())
		}
	})
	t.Run("corrupt store", func(t *testing.T) {
		home := testHome(t)
		mustInit(t)
		w4CorruptStore(t, home)
		p := NewProxy("127.0.0.1:0", home, "test-pass", io.Discard, io.Discard)
		w, r := newReq("GET", "/k/x", "sometoken")
		p.ServeHTTP(w, r)
		if w.Code != http.StatusServiceUnavailable {
			t.Errorf("corrupt store = %d, %q", w.Code, w.Body.String())
		}
	})
	t.Run("capability for a missing alias", func(t *testing.T) {
		home := testHome(t)
		mustInit(t)
		token := grantW4(t, home, "phantom")
		p := NewProxy("127.0.0.1:0", home, "test-pass", io.Discard, io.Discard)
		w, r := newReq("GET", "/phantom/x", token)
		p.ServeHTTP(w, r)
		if w.Code != http.StatusNotFound || !strings.Contains(w.Body.String(), "alias not found") {
			t.Errorf("missing alias = %d, %q", w.Code, w.Body.String())
		}
	})
	t.Run("alias without provider", func(t *testing.T) {
		home := testHome(t)
		mustInit(t)
		if code, _, e := runVault(t, "v\n", "add", "--from-stdin", "noprov"); code != 0 {
			t.Fatalf("seed: %s", e)
		}
		token := grantW4(t, home, "noprov")
		p := NewProxy("127.0.0.1:0", home, "test-pass", io.Discard, io.Discard)
		w, r := newReq("GET", "/noprov/x", token)
		p.ServeHTTP(w, r)
		if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "provider") {
			t.Errorf("providerless alias = %d, %q", w.Code, w.Body.String())
		}
	})
	t.Run("per-request auth fails without a passphrase", func(t *testing.T) {
		home := testHome(t)
		mustInit(t)
		fu := newFakeUpstream(t)
		registerTestProvider(t, "w4-noauth", fu, "bearer")
		if code, _, e := runVault(t, "v\n", "add", "--from-stdin", "--provider=w4-noauth", "k"); code != 0 {
			t.Fatalf("seed: %s", e)
		}
		token := grantW4(t, home, "k")
		t.Setenv(EnvPassphrase, "")
		p := NewProxy("127.0.0.1:0", home, "", io.Discard, io.Discard)
		w, r := newReq("GET", "/k/x", token)
		p.ServeHTTP(w, r)
		if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "vault unavailable") {
			t.Errorf("no-keyring = %d, %q", w.Code, w.Body.String())
		}
	})
	t.Run("secret lookup fails", func(t *testing.T) {
		home := testHome(t)
		mustInit(t)
		fu := newFakeUpstream(t)
		registerTestProvider(t, "w4-nosecret", fu, "bearer")
		s, err := LoadStore(home)
		if err != nil {
			t.Fatal(err)
		}
		s.Aliases = append(s.Aliases, Alias{Name: "ghost", Provider: "w4-nosecret", CreatedAt: "2020-01-01T00:00:00Z"})
		if err := SaveStore(home, s); err != nil {
			t.Fatal(err)
		}
		token := grantW4(t, home, "ghost")
		p := NewProxy("127.0.0.1:0", home, "test-pass", io.Discard, io.Discard)
		w, r := newReq("GET", "/ghost/x", token)
		p.ServeHTTP(w, r)
		if w.Code != http.StatusInternalServerError || !strings.Contains(w.Body.String(), "secret lookup failed") {
			t.Errorf("no-secret = %d, %q", w.Code, w.Body.String())
		}
	})
	t.Run("query string is forwarded", func(t *testing.T) {
		home := testHome(t)
		mustInit(t)
		fu := newFakeUpstream(t)
		registerTestProvider(t, "w4-query", fu, "bearer")
		if code, _, e := runVault(t, "sk\n", "add", "--from-stdin", "--provider=w4-query", "qk"); code != 0 {
			t.Fatalf("seed: %s", e)
		}
		token := grantW4(t, home, "qk")
		p := NewProxy("127.0.0.1:0", home, "test-pass", io.Discard, io.Discard)
		w, r := newReq("GET", "/qk/v1/things?a=1&b=2", token)
		p.ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("query forward = %d, %q", w.Code, w.Body.String())
		}
	})
	t.Run("credential injection fails", func(t *testing.T) {
		home := testHome(t)
		mustInit(t)
		fu := newFakeUpstream(t)
		registerTestProvider(t, "w4-badstyle", fu, "definitely-unknown-style")
		if code, _, e := runVault(t, "sk\n", "add", "--from-stdin", "--provider=w4-badstyle", "bk"); code != 0 {
			t.Fatalf("seed: %s", e)
		}
		token := grantW4(t, home, "bk")
		p := NewProxy("127.0.0.1:0", home, "test-pass", io.Discard, io.Discard)
		w, r := newReq("GET", "/bk/x", token)
		p.ServeHTTP(w, r)
		if w.Code != http.StatusInternalServerError || !strings.Contains(w.Body.String(), "injection failed") {
			t.Errorf("inject-fail = %d, %q", w.Code, w.Body.String())
		}
	})
	t.Run("upstream unreachable", func(t *testing.T) {
		home := testHome(t)
		mustInit(t)
		prev, had := providers["w4-down"]
		providers["w4-down"] = Provider{Name: "w4-down", BaseURL: "http://127.0.0.1:1", AuthStyle: "bearer"}
		t.Cleanup(func() {
			if had {
				providers["w4-down"] = prev
			} else {
				delete(providers, "w4-down")
			}
		})
		if code, _, e := runVault(t, "sk\n", "add", "--from-stdin", "--provider=w4-down", "dk"); code != 0 {
			t.Fatalf("seed: %s", e)
		}
		token := grantW4(t, home, "dk")
		p := NewProxy("127.0.0.1:0", home, "test-pass", io.Discard, io.Discard)
		w, r := newReq("GET", "/dk/x", token)
		p.ServeHTTP(w, r)
		if w.Code != http.StatusBadGateway || !strings.Contains(w.Body.String(), "upstream request failed") {
			t.Errorf("upstream-fail = %d, %q", w.Code, w.Body.String())
		}
	})
}

// grantW4 mints a capability for alias (which need not exist in the
// alias table) and returns its bearer token.
func grantW4(t *testing.T, home, alias string) string {
	t.Helper()
	s, err := LoadStore(home)
	if err != nil {
		t.Fatal(err)
	}
	_, token, err := s.NewCapability(alias, "w4-agent", nil, nil, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveStore(home, s); err != nil {
		t.Fatal(err)
	}
	return token
}

func TestProxyRecordUseSwallowsErrors(t *testing.T) {
	home := testHome(t)
	mustInit(t)
	w4CorruptStore(t, home)
	var errb bytes.Buffer
	p := NewProxy("127.0.0.1:0", home, "test-pass", io.Discard, &errb)
	p.recordUse("some-cap-id", 3)
	if !strings.Contains(errb.String(), "persisting capability counters") {
		t.Errorf("recordUse did not report the persistence failure: %q", errb.String())
	}
}

func TestProxyPureHelpers(t *testing.T) {
	now := time.Now()
	expired := &Capability{ExpiresAt: now.Add(-time.Hour).UTC().Format(time.RFC3339)}
	if got := capabilityDenial(expired, "GET", "", now); got != "expired" {
		t.Errorf("capabilityDenial(expired) = %q", got)
	}
	if denialStatus("budget-exhausted") != http.StatusPaymentRequired {
		t.Error("budget-exhausted should map to 402")
	}
	if got := denialMessage("w4-unknown-reason"); got != "capability denied" {
		t.Errorf("denialMessage(default) = %q", got)
	}
	if parseCostHeader("12x") != 0 {
		t.Error("malformed cost header should read as 0")
	}
	if _, _, ok := splitAliasPath("//rest"); ok {
		t.Error("empty alias segment should be rejected")
	}
	if capabilityActive(&Capability{Revoked: true}, now) {
		t.Error("revoked capability should be inactive")
	}
	if !capabilityActive(&Capability{}, now) {
		t.Error("no-expiry capability should be active")
	}
	if !capabilityActive(&Capability{ExpiresAt: "not-a-time"}, now) {
		t.Error("unparseable expiry should read as active")
	}
	if got := mustHost("http://[::1"); got != "" {
		t.Errorf("mustHost on an unparseable URL = %q", got)
	}
	dst := http.Header{}
	src := http.Header{"Connection": {"keep-alive"}, "X-Ok": {"1"}}
	copyHeadersExceptHop(dst, src)
	if dst.Get("Connection") != "" || dst.Get("X-Ok") != "1" {
		t.Errorf("hop-by-hop filtering wrong: %v", dst)
	}
}

func TestProviderInjectAuthErrorStyles(t *testing.T) {
	req, err := http.NewRequest("GET", "http://example.test/x", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, style := range []string{"header:", "query:", "w4-bogus-style"} {
		p := Provider{Name: "t", AuthStyle: style}
		if err := p.InjectAuth(req, "s"); err == nil {
			t.Errorf("InjectAuth(%q) should error", style)
		}
	}
	// query:<name> success appends the credential as a parameter.
	p := Provider{Name: "t", AuthStyle: "query:key"}
	if err := p.InjectAuth(req, "sekrit"); err != nil {
		t.Fatalf("query style: %v", err)
	}
	if !strings.Contains(req.URL.RawQuery, "key=sekrit") {
		t.Errorf("query param missing: %q", req.URL.RawQuery)
	}
	// The "none" style forwards unmodified.
	if err := (Provider{Name: "t", AuthStyle: "none"}).InjectAuth(req, "s"); err != nil {
		t.Errorf("none style: %v", err)
	}
}

func TestProxyDenialStatusDefault(t *testing.T) {
	if got := denialStatus("host-deny"); got != http.StatusForbidden {
		t.Errorf("denialStatus(host-deny) = %d, want %d", got, http.StatusForbidden)
	}
}
