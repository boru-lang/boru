package vault

// serve_test.go — the wire protocol (`boru vault serve`): runServe's
// flag/guard/lifecycle arms, every handler outcome (health, lookup-self,
// KV v2 read, list), the shared auth arms, namespace-wildcard grants,
// and the pure helpers. Handler behaviour is driven directly through
// ServeHTTP; the lifecycle test exercises the real listener end to end.

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// newServeServer builds a handler-testable server for home. The broker
// session is left nil so each request authenticates via EnvPassphrase,
// exactly like a serve started without a cacheable session.
func newServeServer(home string) *serveServer {
	return &serveServer{listen: "127.0.0.1:0", homeDir: home, stdout: io.Discard, stderr: io.Discard}
}

// serveReq drives one request through the handler, presenting token (if
// any) as the X-Vault-Token header.
func serveReq(t *testing.T, srv *serveServer, method, target, token string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	r := httptest.NewRequest(method, target, nil)
	if token != "" {
		r.Header.Set(headerVaultToken, token)
	}
	srv.ServeHTTP(w, r)
	return w
}

// wireErrors decodes a {"errors":[...]} body.
func wireErrors(t *testing.T, w *httptest.ResponseRecorder) []string {
	t.Helper()
	var body struct {
		Errors []string `json:"errors"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding error body %q: %v", w.Body.String(), err)
	}
	return body.Errors
}

// wireData decodes a successful envelope, returning request_id and data.
func wireData(t *testing.T, w *httptest.ResponseRecorder) (string, map[string]any) {
	t.Helper()
	var body struct {
		RequestID string         `json:"request_id"`
		Data      map[string]any `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding data body %q: %v", w.Body.String(), err)
	}
	return body.RequestID, body.Data
}

// grantWire mints a capability through the CLI and returns its one-time
// bearer token parsed from the grant output.
func grantWire(t *testing.T, args ...string) string {
	t.Helper()
	code, out, errOut := runVault(t, "", append([]string{"grant"}, args...)...)
	if code != 0 {
		t.Fatalf("grant %v failed: %s", args, errOut)
	}
	for _, line := range strings.Split(out, "\n") {
		if rest, ok := strings.CutPrefix(line, "token:"); ok {
			return strings.TrimSpace(rest)
		}
	}
	t.Fatalf("no token line in grant output: %q", out)
	return ""
}

func TestServeFlagAndGuardArms(t *testing.T) {
	t.Run("flag parse error", func(t *testing.T) {
		testHome(t)
		if code, _, _ := runVault(t, "", "serve", "-nope"); code == 0 {
			t.Error("serve with a bad flag should fail")
		}
	})
	t.Run("non-loopback refused", func(t *testing.T) {
		testHome(t)
		code, _, errOut := runVault(t, "", "serve", "--listen=203.0.113.7:8200")
		if code == 0 || !strings.Contains(errOut, "not a loopback address") {
			t.Errorf("non-loopback = %d, %q", code, errOut)
		}
	})
	t.Run("allow-public warns then store error", func(t *testing.T) {
		// --allow-public downgrades the refusal to a warning; with no
		// vault initialized the store guard then fails before any bind.
		testHome(t)
		code, _, errOut := runVault(t, "", "serve", "--listen=203.0.113.7:8200", "--allow-public")
		if code == 0 || !strings.Contains(errOut, "warning") || !strings.Contains(errOut, "not initialized") {
			t.Errorf("allow-public = %d, %q", code, errOut)
		}
	})
	t.Run("locked vault refused", func(t *testing.T) {
		testHome(t)
		mustInit(t)
		if code, _, e := runVault(t, "", "lock"); code != 0 {
			t.Fatalf("lock: %s", e)
		}
		code, _, errOut := runVault(t, "", "serve")
		if code == 0 || !strings.Contains(errOut, "locked") {
			t.Errorf("locked = %d, %q", code, errOut)
		}
	})
}

// serveBusyPort binds a listener so runServe's ListenAndServe fails
// immediately, and returns its address.
func serveBusyPort(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	return ln.Addr().String()
}

func TestServeListenErrorArms(t *testing.T) {
	t.Run("legacy vault, port in use", func(t *testing.T) {
		// A legacy vault caches a slotless session (no warnings), then
		// ListenAndServe fails on the occupied port.
		testHome(t)
		mustInit(t)
		addr := serveBusyPort(t)
		code, _, errOut := runVault(t, "", "serve", "--listen="+addr)
		if code == 0 || !strings.Contains(errOut, "address already in use") {
			t.Errorf("busy port = %d, %q", code, errOut)
		}
	})
	t.Run("startup authenticate fails, session stays nil", func(t *testing.T) {
		testHome(t)
		mustInit(t)
		setPass(t, "") // file backend without a passphrase cannot open a session
		addr := serveBusyPort(t)
		code, _, errOut := runVault(t, "", "serve", "--listen="+addr)
		if code == 0 || !strings.Contains(errOut, "address already in use") {
			t.Errorf("busy port without session = %d, %q", code, errOut)
		}
	})
	t.Run("temporary password is not cached", func(t *testing.T) {
		w4EnvelopeVault(t)
		mustAddPassword(t, "test-pass", "tmp-pass", "tmp", "--scope=read", "--namespaces=proj", "--ttl=1h")
		setPass(t, "tmp-pass")
		addr := serveBusyPort(t)
		code, _, errOut := runVault(t, "", "serve", "--listen="+addr)
		if code == 0 || !strings.Contains(errOut, "TEMPORARY password") {
			t.Errorf("temporary password = %d, %q", code, errOut)
		}
	})
}

func TestServeProtocolNegotiation(t *testing.T) {
	home := testHome(t)
	mustInit(t)
	srv := newServeServer(home)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/v1/sys/health", nil)
	r.Header.Set(headerProtocol, "99")
	srv.ServeHTTP(w, r)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "upgrade the server") {
		t.Errorf("newer client protocol = %d, %q", w.Code, w.Body.String())
	}

	// The current version and a malformed value both pass through.
	for _, h := range []string{"1", "not-a-number"} {
		w = httptest.NewRecorder()
		r = httptest.NewRequest("GET", "/v1/sys/health", nil)
		r.Header.Set(headerProtocol, h)
		srv.ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Errorf("header %q = %d, want 200", h, w.Code)
		}
		if got := w.Header().Get(headerProtocol); got != "1" {
			t.Errorf("advertised protocol = %q, want 1", got)
		}
	}
}

func TestServeUnsupportedPath(t *testing.T) {
	home := testHome(t)
	mustInit(t)
	w := serveReq(t, newServeServer(home), "GET", "/v1/nope", "")
	if w.Code != http.StatusNotFound {
		t.Errorf("unsupported path = %d", w.Code)
	}
	if errs := wireErrors(t, w); len(errs) != 1 || errs[0] != "unsupported path" {
		t.Errorf("errors = %v", errs)
	}
}

func TestServeHealth(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		home := testHome(t)
		mustInit(t)
		w := serveReq(t, newServeServer(home), "GET", "/v1/sys/health", "")
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"sealed":false`) {
			t.Errorf("health = %d, %q", w.Code, w.Body.String())
		}
		if ct := w.Header().Get("Content-Type"); ct != "application/json" {
			t.Errorf("content-type = %q", ct)
		}
		if cc := w.Header().Get("Cache-Control"); cc != "no-store" {
			t.Errorf("Cache-Control = %q, want no-store", cc)
		}
	})
	t.Run("bad method", func(t *testing.T) {
		home := testHome(t)
		mustInit(t)
		w := serveReq(t, newServeServer(home), "POST", "/v1/sys/health", "")
		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("health POST = %d", w.Code)
		}
	})
	t.Run("uninitialized", func(t *testing.T) {
		home := testHome(t)
		w := serveReq(t, newServeServer(home), "GET", "/v1/sys/health", "")
		if w.Code != http.StatusNotImplemented || !strings.Contains(w.Body.String(), `"initialized":false`) {
			t.Errorf("uninitialized health = %d, %q", w.Code, w.Body.String())
		}
	})
	t.Run("sealed", func(t *testing.T) {
		home := testHome(t)
		mustInit(t)
		if code, _, e := runVault(t, "", "lock"); code != 0 {
			t.Fatalf("lock: %s", e)
		}
		w := serveReq(t, newServeServer(home), "GET", "/v1/sys/health", "")
		if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), `"sealed":true`) {
			t.Errorf("sealed health = %d, %q", w.Code, w.Body.String())
		}
	})
	t.Run("corrupt store", func(t *testing.T) {
		home := testHome(t)
		mustInit(t)
		w4CorruptStore(t, home)
		w := serveReq(t, newServeServer(home), "GET", "/v1/sys/health", "")
		if w.Code != http.StatusInternalServerError {
			t.Errorf("corrupt health = %d", w.Code)
		}
	})
}

// serveFixture initializes a legacy vault with one root secret ("r") and
// one namespaced secret ("proj:k"), returning home.
func serveFixture(t *testing.T) string {
	t.Helper()
	home := testHome(t)
	mustInit(t)
	if code, _, e := runVault(t, "root-value\n", "add", "--from-stdin", "r"); code != 0 {
		t.Fatalf("seed r: %s", e)
	}
	if code, _, e := runVault(t, "proj-value\n", "add", "--from-stdin", "proj:k"); code != 0 {
		t.Fatalf("seed proj:k: %s", e)
	}
	return home
}

func TestServeAuthArms(t *testing.T) {
	t.Run("missing token", func(t *testing.T) {
		home := serveFixture(t)
		w := serveReq(t, newServeServer(home), "GET", "/v1/secret/data/r", "")
		if w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), "missing client token") {
			t.Errorf("no token = %d, %q", w.Code, w.Body.String())
		}
	})
	t.Run("corrupt store", func(t *testing.T) {
		home := serveFixture(t)
		w4CorruptStore(t, home)
		w := serveReq(t, newServeServer(home), "GET", "/v1/secret/data/r", "sometoken")
		if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "vault store unreadable") {
			t.Errorf("corrupt store = %d, %q", w.Code, w.Body.String())
		}
		if strings.Contains(w.Body.String(), home) {
			t.Errorf("store path leaked to an unauthenticated caller: %q", w.Body.String())
		}
	})
	t.Run("sealed", func(t *testing.T) {
		home := serveFixture(t)
		if code, _, e := runVault(t, "", "lock"); code != 0 {
			t.Fatalf("lock: %s", e)
		}
		w := serveReq(t, newServeServer(home), "GET", "/v1/secret/data/r", "sometoken")
		if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "sealed") {
			t.Errorf("sealed = %d, %q", w.Code, w.Body.String())
		}
	})
	t.Run("unknown token", func(t *testing.T) {
		home := serveFixture(t)
		w := serveReq(t, newServeServer(home), "GET", "/v1/secret/data/r", "not-a-real-token")
		if w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), "permission denied") {
			t.Errorf("unknown token = %d, %q", w.Code, w.Body.String())
		}
	})
	t.Run("revoked capability", func(t *testing.T) {
		home := serveFixture(t)
		token := grantWire(t, "r")
		s, err := LoadStore(home)
		if err != nil {
			t.Fatal(err)
		}
		s.Capabilities[0].Revoked = true
		if err := SaveStore(home, s); err != nil {
			t.Fatal(err)
		}
		w := serveReq(t, newServeServer(home), "GET", "/v1/secret/data/r", token)
		if w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), "revoked") {
			t.Errorf("revoked = %d, %q", w.Code, w.Body.String())
		}
	})
	t.Run("call quota exhausts", func(t *testing.T) {
		home := serveFixture(t)
		token := grantWire(t, "--max-calls=1", "r")
		srv := newServeServer(home)
		if w := serveReq(t, srv, "GET", "/v1/secret/data/r", token); w.Code != http.StatusOK {
			t.Fatalf("first read = %d, %q", w.Code, w.Body.String())
		}
		w := serveReq(t, srv, "GET", "/v1/secret/data/r", token)
		if w.Code != http.StatusTooManyRequests {
			t.Errorf("second read = %d, want 429", w.Code)
		}
	})
	t.Run("method allowlist applies", func(t *testing.T) {
		home := serveFixture(t)
		token := grantWire(t, "--methods=POST", "r")
		w := serveReq(t, newServeServer(home), "GET", "/v1/secret/data/r", token)
		if w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), "method not permitted") {
			t.Errorf("method-deny = %d, %q", w.Code, w.Body.String())
		}
	})
}

func TestServeRead(t *testing.T) {
	t.Run("bad method", func(t *testing.T) {
		home := serveFixture(t)
		w := serveReq(t, newServeServer(home), "POST", "/v1/secret/data/r", "x")
		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("POST read = %d", w.Code)
		}
	})
	t.Run("bad paths", func(t *testing.T) {
		home := serveFixture(t)
		srv := newServeServer(home)
		for _, p := range []string{
			"/v1/secret/data/",          // empty
			"/v1/secret/data/a/b/c",     // too deep
			"/v1/secret/data/bad%21seg", // charset
		} {
			w := serveReq(t, srv, "GET", p, "x")
			if w.Code != http.StatusBadRequest {
				t.Errorf("read %q = %d, want 400", p, w.Code)
			}
		}
	})
	t.Run("root secret via exact token", func(t *testing.T) {
		home := serveFixture(t)
		token := grantWire(t, "--agent=app", "r")
		w := serveReq(t, newServeServer(home), "GET", "/v1/secret/data/r", token)
		if w.Code != http.StatusOK {
			t.Fatalf("read = %d, %q", w.Code, w.Body.String())
		}
		reqID, data := wireData(t, w)
		if reqID == "" {
			t.Error("empty request_id")
		}
		inner, _ := data["data"].(map[string]any)
		if inner["value"] != "root-value" {
			t.Errorf("value = %v", inner["value"])
		}
		meta, _ := data["metadata"].(map[string]any)
		if meta["created_time"] == "" || meta["version"] != float64(1) {
			t.Errorf("metadata = %v", meta)
		}
	})
	t.Run("namespaced secret via wildcard token", func(t *testing.T) {
		home := serveFixture(t)
		token := grantWire(t, "proj:*")
		w := serveReq(t, newServeServer(home), "GET", "/v1/secret/data/proj/k", token)
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"value":"proj-value"`) {
			t.Errorf("wildcard read = %d, %q", w.Code, w.Body.String())
		}
	})
	t.Run("wildcard does not cross namespaces", func(t *testing.T) {
		home := serveFixture(t)
		token := grantWire(t, "proj:*")
		w := serveReq(t, newServeServer(home), "GET", "/v1/secret/data/r", token)
		if w.Code != http.StatusForbidden {
			t.Errorf("cross-namespace read = %d, want 403", w.Code)
		}
	})
	t.Run("token for another alias", func(t *testing.T) {
		home := serveFixture(t)
		token := grantWire(t, "r")
		w := serveReq(t, newServeServer(home), "GET", "/v1/secret/data/proj/k", token)
		if w.Code != http.StatusForbidden {
			t.Errorf("alias-mismatch = %d, want 403", w.Code)
		}
	})
	t.Run("missing secret is a bare 404", func(t *testing.T) {
		home := serveFixture(t)
		token := grantWire(t, "*")
		w := serveReq(t, newServeServer(home), "GET", "/v1/secret/data/ghost", token)
		if w.Code != http.StatusNotFound {
			t.Errorf("missing = %d, want 404", w.Code)
		}
		if errs := wireErrors(t, w); len(errs) != 0 {
			t.Errorf("404 errors should be empty, got %v", errs)
		}
	})
	t.Run("ip whitelist", func(t *testing.T) {
		// httptest requests arrive from 192.0.2.1; a foreign whitelist
		// denies, a matching one admits.
		home := serveFixture(t)
		if code, _, e := runVault(t, "", "rotate", "--from-env=SERVE_ROT", "--ip-whitelist=203.0.113.0/24", "--yes", "r"); code == 0 {
			t.Fatalf("rotate without env should fail: %s", e)
		}
		t.Setenv("SERVE_ROT", "rotated-value")
		if code, _, e := runVault(t, "", "rotate", "--from-env=SERVE_ROT", "--ip-whitelist=203.0.113.0/24", "--yes", "r"); code != 0 {
			t.Fatalf("rotate: %s", e)
		}
		token := grantWire(t, "r")
		srv := newServeServer(home)
		w := serveReq(t, srv, "GET", "/v1/secret/data/r", token)
		if w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), "ip-whitelist") {
			t.Errorf("ip-denied = %d, %q", w.Code, w.Body.String())
		}
		if code, _, e := runVault(t, "", "rotate", "--from-env=SERVE_ROT", "--ip-whitelist=192.0.2.1", "--yes", "r"); code != 0 {
			t.Fatalf("re-rotate: %s", e)
		}
		w = serveReq(t, srv, "GET", "/v1/secret/data/r", token)
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"value":"rotated-value"`) {
			t.Errorf("ip-allowed = %d, %q", w.Code, w.Body.String())
		}
		// The rotation also stamps UpdatedAt, whose value read serves as
		// created_time (the freshest write wins).
		s, err := LoadStore(home)
		if err != nil {
			t.Fatal(err)
		}
		a, _ := s.FindAlias("r")
		if a.UpdatedAt == "" || !strings.Contains(w.Body.String(), a.UpdatedAt) {
			t.Errorf("created_time should be the update time %q: %q", a.UpdatedAt, w.Body.String())
		}
	})
	t.Run("per-request session fails without passphrase", func(t *testing.T) {
		home := serveFixture(t)
		token := grantWire(t, "r")
		setPass(t, "")
		w := serveReq(t, newServeServer(home), "GET", "/v1/secret/data/r", token)
		if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "vault unavailable") {
			t.Errorf("no-keyring = %d, %q", w.Code, w.Body.String())
		}
	})
	t.Run("cached session is reused", func(t *testing.T) {
		home := serveFixture(t)
		token := grantWire(t, "r")
		s, err := LoadStore(home)
		if err != nil {
			t.Fatal(err)
		}
		sess, err := authenticate(s, home, nil, io.Discard, "")
		if err != nil {
			t.Fatal(err)
		}
		defer sess.Close()
		srv := newServeServer(home)
		srv.sess = sess
		setPass(t, "") // prove the cached session, not the env, serves the read
		w := serveReq(t, srv, "GET", "/v1/secret/data/r", token)
		if w.Code != http.StatusOK {
			t.Errorf("cached-session read = %d, %q", w.Code, w.Body.String())
		}
	})
	t.Run("scope denied outside the broker password's namespaces", func(t *testing.T) {
		w4EnvelopeVault(t) // admin + reader(proj); secret proj:k
		if code, _, e := runVault(t, "root-v\n", "add", "--from-stdin", "rootkey"); code != 0 {
			t.Fatalf("seed rootkey: %s", e)
		}
		token := grantWire(t, "*")
		setPass(t, "reader-pass") // per-request session only covers proj
		home := os.Getenv(EnvHome)
		w := serveReq(t, newServeServer(home), "GET", "/v1/secret/data/rootkey", token)
		if w.Code != http.StatusForbidden {
			t.Errorf("scope-denied = %d, %q", w.Code, w.Body.String())
		}
	})
	t.Run("secret lookup fails", func(t *testing.T) {
		home := serveFixture(t)
		s, err := LoadStore(home)
		if err != nil {
			t.Fatal(err)
		}
		s.Aliases = append(s.Aliases, Alias{Name: "ghost", CreatedAt: "2020-01-01T00:00:00Z"})
		if err := SaveStore(home, s); err != nil {
			t.Fatal(err)
		}
		token := grantWire(t, "ghost")
		w := serveReq(t, newServeServer(home), "GET", "/v1/secret/data/ghost", token)
		if w.Code != http.StatusInternalServerError || !strings.Contains(w.Body.String(), "secret lookup failed") {
			t.Errorf("no-secret = %d, %q", w.Code, w.Body.String())
		}
	})
}

func TestServeLookupSelf(t *testing.T) {
	t.Run("bad method", func(t *testing.T) {
		home := serveFixture(t)
		w := serveReq(t, newServeServer(home), "POST", "/v1/auth/token/lookup-self", "x")
		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("POST lookup = %d", w.Code)
		}
	})
	t.Run("auth failure", func(t *testing.T) {
		home := serveFixture(t)
		w := serveReq(t, newServeServer(home), "GET", "/v1/auth/token/lookup-self", "wrong-token")
		if w.Code != http.StatusForbidden {
			t.Errorf("bad-token lookup = %d, want 403", w.Code)
		}
	})
	t.Run("ok", func(t *testing.T) {
		home := serveFixture(t)
		token := grantWire(t, "--agent=sekreto", "--max-calls=5", "proj:*")
		w := serveReq(t, newServeServer(home), "GET", "/v1/auth/token/lookup-self", token)
		if w.Code != http.StatusOK {
			t.Fatalf("lookup = %d, %q", w.Code, w.Body.String())
		}
		_, data := wireData(t, w)
		if data["display_name"] != "sekreto" || data["num_uses"] != float64(5) {
			t.Errorf("lookup data = %v", data)
		}
		meta, _ := data["meta"].(map[string]any)
		if meta["alias"] != "proj:*" || meta["namespace"] != "proj" {
			t.Errorf("lookup meta = %v", meta)
		}
		if data["accessor"] == "" || strings.Contains(w.Body.String(), token) {
			t.Error("accessor missing or token echoed")
		}
	})
}

func TestServeList(t *testing.T) {
	t.Run("bad methods", func(t *testing.T) {
		home := serveFixture(t)
		srv := newServeServer(home)
		if w := serveReq(t, srv, "POST", "/v1/secret/metadata", "x"); w.Code != http.StatusMethodNotAllowed {
			t.Errorf("POST list = %d", w.Code)
		}
		if w := serveReq(t, srv, "GET", "/v1/secret/metadata", "x"); w.Code != http.StatusMethodNotAllowed {
			t.Errorf("GET without ?list=true = %d", w.Code)
		}
	})
	t.Run("bad path", func(t *testing.T) {
		home := serveFixture(t)
		w := serveReq(t, newServeServer(home), "LIST", "/v1/secret/metadata/a/b", "x")
		if w.Code != http.StatusBadRequest {
			t.Errorf("deep list path = %d", w.Code)
		}
	})
	t.Run("auth failure", func(t *testing.T) {
		home := serveFixture(t)
		w := serveReq(t, newServeServer(home), "LIST", "/v1/secret/metadata", "wrong-token")
		if w.Code != http.StatusForbidden {
			t.Errorf("bad-token list = %d, want 403", w.Code)
		}
	})
	t.Run("root level shows names and folders", func(t *testing.T) {
		home := serveFixture(t)
		rootTok := grantWire(t, "*")
		projTok := grantWire(t, "proj:k")
		srv := newServeServer(home)
		w := serveReq(t, srv, "LIST", "/v1/secret/metadata", rootTok)
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"keys":["r"]`) {
			t.Errorf("root wildcard list = %d, %q", w.Code, w.Body.String())
		}
		// An exact proj:k token sees only the proj/ folder at root level…
		w = serveReq(t, srv, "LIST", "/v1/secret/metadata", projTok)
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"keys":["proj/"]`) {
			t.Errorf("exact-token root list = %d, %q", w.Code, w.Body.String())
		}
		// …and its base name inside the namespace, also via GET ?list=true.
		w = serveReq(t, srv, "GET", "/v1/secret/metadata/proj?list=true", projTok)
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"keys":["k"]`) {
			t.Errorf("namespace list = %d, %q", w.Code, w.Body.String())
		}
	})
	t.Run("list requires a session", func(t *testing.T) {
		home := serveFixture(t)
		token := grantWire(t, "*")
		setPass(t, "")
		w := serveReq(t, newServeServer(home), "LIST", "/v1/secret/metadata", token)
		if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "vault unavailable") {
			t.Errorf("sessionless list = %d, %q", w.Code, w.Body.String())
		}
	})
	t.Run("scope bounds what a list reveals", func(t *testing.T) {
		// The reader password covers only proj; a root wildcard token must
		// not learn root names through a broker it cannot read them from.
		w4EnvelopeVault(t)
		if code, _, e := runVault(t, "root-v\n", "add", "--from-stdin", "rootkey"); code != 0 {
			t.Fatalf("seed rootkey: %s", e)
		}
		rootTok := grantWire(t, "*")
		projTok := grantWire(t, "proj:*")
		setPass(t, "reader-pass")
		home := os.Getenv(EnvHome)
		srv := newServeServer(home)
		w := serveReq(t, srv, "LIST", "/v1/secret/metadata", rootTok)
		if w.Code != http.StatusNotFound {
			t.Errorf("scope-filtered root list = %d, want 404", w.Code)
		}
		// The namespace the password does cover still lists normally.
		w = serveReq(t, srv, "LIST", "/v1/secret/metadata/proj", projTok)
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"keys":["k"]`) {
			t.Errorf("in-scope list = %d, %q", w.Code, w.Body.String())
		}
	})
	t.Run("ip whitelist bounds what a list reveals", func(t *testing.T) {
		home := serveFixture(t)
		t.Setenv("SERVE_LIST_ROT", "rotated")
		if code, _, e := runVault(t, "", "rotate", "--from-env=SERVE_LIST_ROT", "--ip-whitelist=203.0.113.0/24", "--yes", "r"); code != 0 {
			t.Fatalf("rotate: %s", e)
		}
		token := grantWire(t, "*")
		w := serveReq(t, newServeServer(home), "LIST", "/v1/secret/metadata", token)
		if w.Code != http.StatusNotFound {
			t.Errorf("ip-filtered list = %d, want 404 (alias visible from a denied IP)", w.Code)
		}
	})
	t.Run("nothing visible is a bare 404", func(t *testing.T) {
		home := serveFixture(t)
		token := grantWire(t, "proj:k")
		w := serveReq(t, newServeServer(home), "LIST", "/v1/secret/metadata/other", token)
		if w.Code != http.StatusNotFound {
			t.Errorf("empty list = %d, want 404", w.Code)
		}
		if errs := wireErrors(t, w); len(errs) != 0 {
			t.Errorf("404 errors should be empty, got %v", errs)
		}
	})
}

func TestServeSeamArms(t *testing.T) {
	t.Run("entropy failure", func(t *testing.T) {
		home := serveFixture(t)
		token := grantWire(t, "r")
		prev := s7randRead
		s7randRead = func([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
		t.Cleanup(func() { s7randRead = prev })
		w := serveReq(t, newServeServer(home), "GET", "/v1/secret/data/r", token)
		if w.Code != http.StatusInternalServerError || !strings.Contains(w.Body.String(), "internal error") {
			t.Errorf("no-entropy = %d, %q", w.Code, w.Body.String())
		}
	})
	t.Run("marshal failure", func(t *testing.T) {
		home := testHome(t)
		mustInit(t)
		prev := jsonMarshal
		jsonMarshal = func(any) ([]byte, error) { return nil, io.ErrUnexpectedEOF }
		t.Cleanup(func() { jsonMarshal = prev })
		w := serveReq(t, newServeServer(home), "GET", "/v1/sys/health", "")
		if w.Code != http.StatusInternalServerError || !strings.Contains(w.Body.String(), "encoding error") {
			t.Errorf("marshal failure = %d, %q", w.Code, w.Body.String())
		}
	})
}

func TestServeRecordUseSwallowsErrors(t *testing.T) {
	home := testHome(t)
	mustInit(t)
	w4CorruptStore(t, home)
	var errb bytes.Buffer
	srv := newServeServer(home)
	srv.stderr = &errb
	srv.recordUse("some-cap-id")
	if !strings.Contains(errb.String(), "persisting capability counters") {
		t.Errorf("recordUse did not report the persistence failure: %q", errb.String())
	}
}

func TestServeLogWithoutStdout(t *testing.T) {
	home := testHome(t)
	mustInit(t)
	srv := newServeServer(home)
	srv.stdout = nil
	w := serveReq(t, srv, "GET", "/v1/sys/health", "")
	if w.Code != http.StatusOK {
		t.Errorf("health with nil stdout = %d", w.Code)
	}
}

func TestServePureHelpers(t *testing.T) {
	t.Run("aliasFromWirePath", func(t *testing.T) {
		if a, ok := aliasFromWirePath("name"); !ok || a != "name" {
			t.Errorf("root path = %q, %t", a, ok)
		}
		if a, ok := aliasFromWirePath("ns/name"); !ok || a != "ns:name" {
			t.Errorf("ns path = %q, %t", a, ok)
		}
		for _, bad := range []string{"", "a/b/c", "a/", "/b", "sp ace", "a:b"} {
			if _, ok := aliasFromWirePath(bad); ok {
				t.Errorf("path %q should be rejected", bad)
			}
		}
	})
	t.Run("capabilityCoversAlias", func(t *testing.T) {
		if !capabilityCoversAlias(&Capability{Alias: "a"}, "a") {
			t.Error("exact match should cover")
		}
		if capabilityCoversAlias(&Capability{Alias: "a"}, "b") {
			t.Error("different alias should not cover")
		}
		if !capabilityCoversAlias(&Capability{Alias: "*"}, "a") {
			t.Error("root wildcard should cover a root alias")
		}
		if capabilityCoversAlias(&Capability{Alias: "*"}, "ns:a") {
			t.Error("root wildcard should not cover a namespaced alias")
		}
		if !capabilityCoversAlias(&Capability{Alias: "ns:*"}, "ns:a") {
			t.Error("ns wildcard should cover its namespace")
		}
		if capabilityCoversAlias(&Capability{Alias: "ns:*"}, "other:a") {
			t.Error("ns wildcard should not cover another namespace")
		}
	})
	t.Run("serveToken", func(t *testing.T) {
		r := httptest.NewRequest("GET", "/", nil)
		if serveToken(r) != "" {
			t.Error("no headers should read as no token")
		}
		r.Header.Set("Authorization", "Bearer bbb")
		if serveToken(r) != "bbb" {
			t.Error("Bearer fallback failed")
		}
		r.Header.Set(headerVaultToken, "aaa")
		if serveToken(r) != "aaa" {
			t.Error("X-Vault-Token should win")
		}
	})
	t.Run("serveListKeys dedupes folders", func(t *testing.T) {
		s := &Store{Aliases: []Alias{{Name: "ns:a"}, {Name: "ns:b"}}}
		got := serveListKeys(s, &Capability{Alias: "ns:*"}, "", nil, "")
		if len(got) != 1 || got[0] != "ns/" {
			t.Errorf("root folders = %v", got)
		}
	})
}

func TestGrantWildcards(t *testing.T) {
	t.Run("root spellings", func(t *testing.T) {
		serveFixture(t)
		for _, ref := range []string{"*", ":*"} {
			code, out, errOut := runVault(t, "", "grant", ref)
			if code != 0 {
				t.Fatalf("grant %q: %s", ref, errOut)
			}
			if !strings.Contains(out, "alias:      *") || !strings.Contains(out, "namespace wildcard") {
				t.Errorf("grant %q output: %q", ref, out)
			}
		}
	})
	t.Run("bare wildcard follows the default namespace", func(t *testing.T) {
		serveFixture(t)
		t.Setenv(EnvNamespace, "proj")
		code, out, errOut := runVault(t, "", "grant", "*")
		if code != 0 {
			t.Fatalf("grant * under a default namespace: %s", errOut)
		}
		if !strings.Contains(out, "alias:      proj:*") {
			t.Errorf("bare * should qualify into the default namespace: %q", out)
		}
		// …and ':*' still forces root despite the default.
		code, out, errOut = runVault(t, "", "grant", ":*")
		if code != 0 {
			t.Fatalf("grant :* under a default namespace: %s", errOut)
		}
		if !strings.Contains(out, "alias:      *") {
			t.Errorf(":* should force the root wildcard: %q", out)
		}
	})
	t.Run("bad default namespace fails the bare wildcard", func(t *testing.T) {
		serveFixture(t)
		t.Setenv(EnvNamespace, "!bad")
		code, _, errOut := runVault(t, "", "grant", "*")
		if code == 0 || !strings.Contains(errOut, "invalid namespace") {
			t.Errorf("grant * with a bad default namespace = %d, %q", code, errOut)
		}
	})
	t.Run("invalid wildcards", func(t *testing.T) {
		serveFixture(t)
		for _, ref := range []string{"foo*", "a:b:*", "!bad:*"} {
			code, _, errOut := runVault(t, "", "grant", ref)
			if code == 0 || !strings.Contains(errOut, "invalid wildcard") {
				t.Errorf("grant %q = %d, %q", ref, code, errOut)
			}
		}
	})
	t.Run("wildcard token is refused by the proxy", func(t *testing.T) {
		home := serveFixture(t)
		token := grantWire(t, "*")
		p := NewProxy("127.0.0.1:0", home, "test-pass", io.Discard, io.Discard)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/r/x", nil)
		r.Header.Set("Authorization", "Bearer "+token)
		p.ServeHTTP(w, r)
		if w.Code != http.StatusForbidden {
			t.Errorf("proxy with wildcard token = %d, want 403", w.Code)
		}
	})
}

// TestServeGuardsAllInterfacesBind proves --listen=:PORT (which Go binds
// on every interface) is refused without --allow-public, for serve and
// the proxy alike.
func TestServeGuardsAllInterfacesBind(t *testing.T) {
	testHome(t)
	for _, mode := range []string{"serve", "proxy"} {
		code, _, errOut := runVault(t, "", mode, "--listen=:0")
		if code == 0 || !strings.Contains(errOut, "not a loopback address") {
			t.Errorf("%s --listen=:0 = %d, %q (must refuse an all-interfaces bind)", mode, code, errOut)
		}
	}
}

// TestVerifyKeepsWildcardCapabilities: a namespace wildcard binds a
// namespace, not an alias record, so verify must not call it dangling —
// and --prune must not revoke it.
func TestVerifyKeepsWildcardCapabilities(t *testing.T) {
	home := serveFixture(t)
	token := grantWire(t, "proj:*")
	if code, out, errOut := runVault(t, "", "verify", "--prune"); code != 0 {
		t.Fatalf("verify with a wildcard grant = %d, %q %q", code, out, errOut)
	}
	// The token still works after the prune: nothing was revoked.
	w := serveReq(t, newServeServer(home), "GET", "/v1/secret/data/proj/k", token)
	if w.Code != http.StatusOK {
		t.Errorf("wildcard token after verify --prune = %d, want 200", w.Code)
	}
}

// TestRotateRevokesCoveringWildcard: rotating with --revoke-caps is the
// incident-response path, so a wildcard capability that can read the
// rotated alias must die with the exact-match ones.
func TestRotateRevokesCoveringWildcard(t *testing.T) {
	home := serveFixture(t)
	wildTok := grantWire(t, "proj:*")
	otherTok := grantWire(t, "r")
	t.Setenv("SERVE_WILD_ROT", "fresh-value")
	if code, out, errOut := runVault(t, "", "rotate", "--from-env=SERVE_WILD_ROT", "--revoke-caps", "--yes", "proj:k"); code != 0 {
		t.Fatalf("rotate: %q %q", out, errOut)
	} else if !strings.Contains(out, "revoked 1 capability") {
		t.Errorf("rotate output should count the wildcard revocation: %q", out)
	}
	srv := newServeServer(home)
	w := serveReq(t, srv, "GET", "/v1/secret/data/proj/k", wildTok)
	if w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), "revoked") {
		t.Errorf("covering wildcard after rotate --revoke-caps = %d, %q (must be revoked)", w.Code, w.Body.String())
	}
	// A capability on an uncovered alias is untouched.
	w = serveReq(t, srv, "GET", "/v1/secret/data/r", otherTok)
	if w.Code != http.StatusOK {
		t.Errorf("unrelated capability after rotate = %d, want 200", w.Code)
	}
}

// TestServeHealthIsNotAudited: liveness probes write an access line but
// no audit record, so a probing service manager cannot grow the audit
// log forever.
func TestServeHealthIsNotAudited(t *testing.T) {
	home := serveFixture(t)
	var outb bytes.Buffer
	srv := newServeServer(home)
	srv.stdout = &outb
	if w := serveReq(t, srv, "GET", "/v1/sys/health", ""); w.Code != http.StatusOK {
		t.Fatalf("health = %d", w.Code)
	}
	if !strings.Contains(outb.String(), "outcome=health") {
		t.Errorf("health should still write an access line: %q", outb.String())
	}
	audit, _ := os.ReadFile(auditPath(home))
	if strings.Contains(string(audit), "serve.request") {
		t.Errorf("health probe reached the audit log: %s", audit)
	}
	// A denied secret read IS audited — the log exists for those.
	if w := serveReq(t, srv, "GET", "/v1/secret/data/r", ""); w.Code != http.StatusForbidden {
		t.Fatalf("tokenless read should be 403")
	}
	audit, _ = os.ReadFile(auditPath(home))
	if !strings.Contains(string(audit), "serve.request") {
		t.Errorf("denied read missing from the audit log: %s", audit)
	}
}
