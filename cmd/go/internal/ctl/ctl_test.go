package ctl

import (
	"bytes"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// runCtl invokes the subcommand the way main does and captures the
// exit code plus both output streams.
func runCtl(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := New().Run(args, nil, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

// fakeAPI is a minimal stand-in for the api service. It records the
// last request and serves canned JSON.
type fakeAPI struct {
	t *testing.T

	lastMethod string
	lastPath   string
	lastAuth   string
	lastBody   map[string]string

	actionStatus int    // status for POST .../actions (default 200)
	actionErr    string // JSON error body for non-200 action replies
}

func (f *fakeAPI) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/services", func(w http.ResponseWriter, r *http.Request) {
		f.record(r)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"name": "registry", "state": "running", "metadata": map[string]string{"addr": ":8080"}},
			{"name": "lsp", "state": "paused"},
		})
	})
	mux.HandleFunc("GET /v1/server", func(w http.ResponseWriter, r *http.Request) {
		f.record(r)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"pid": 4242, "version": "9.9.9", "startTime": "2026-01-01T00:00:00Z",
			"uptimeSeconds": 12.7, "serviceCount": 2,
		})
	})
	mux.HandleFunc("POST /v1/services/{name}/actions", func(w http.ResponseWriter, r *http.Request) {
		f.record(r)
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.lastBody = body
		status := f.actionStatus
		if status == 0 {
			status = http.StatusOK
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if status != http.StatusOK && f.actionErr != "" {
			_ = json.NewEncoder(w).Encode(map[string]string{"error": f.actionErr})
		}
	})
	return mux
}

func (f *fakeAPI) record(r *http.Request) {
	f.lastMethod = r.Method
	f.lastPath = r.URL.Path
	f.lastAuth = r.Header.Get("Authorization")
}

func newFakeAPI(t *testing.T) (*fakeAPI, *httptest.Server) {
	t.Helper()
	f := &fakeAPI{t: t}
	ts := httptest.NewServer(f.handler())
	t.Cleanup(ts.Close)
	return f, ts
}

func TestNameAndSynopsis(t *testing.T) {
	c := New()
	if c.Name() != "ctl" {
		t.Errorf("Name = %q, want ctl", c.Name())
	}
	if c.Synopsis() == "" {
		t.Error("Synopsis should not be empty")
	}
}

func TestRunNoArgsPrintsUsage(t *testing.T) {
	code, _, errOut := runCtl(t)
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if !strings.Contains(errOut, "Usage: boru ctl") {
		t.Errorf("stderr should show usage; got %q", errOut)
	}
}

func TestRunBadFlag(t *testing.T) {
	code, _, errOut := runCtl(t, "--nope")
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if errOut == "" {
		t.Error("stderr should describe the flag error")
	}
}

func TestRunUnknownOp(t *testing.T) {
	code, _, errOut := runCtl(t, "--api", "http://127.0.0.1:0", "frobnicate")
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if !strings.Contains(errOut, `unknown op "frobnicate"`) {
		t.Errorf("stderr = %q, want unknown-op message", errOut)
	}
}

func TestActionsRequireServiceName(t *testing.T) {
	for _, op := range []string{"pause", "resume", "stop"} {
		code, _, errOut := runCtl(t, "--api", "http://127.0.0.1:0", op)
		if code != 1 {
			t.Errorf("%s: exit = %d, want 1", op, code)
		}
		if !strings.Contains(errOut, op+" requires a service name") {
			t.Errorf("%s: stderr = %q", op, errOut)
		}
	}
}

func TestStatusListsServices(t *testing.T) {
	f, ts := newFakeAPI(t)
	code, out, errOut := runCtl(t, "--api", ts.URL, "--token", "sekret", "status")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, errOut)
	}
	for _, want := range []string{"registry", "running", "lsp", "paused", "addr=:8080"} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout missing %q:\n%s", want, out)
		}
	}
	if f.lastAuth != "Bearer sekret" {
		t.Errorf("Authorization = %q, want Bearer sekret", f.lastAuth)
	}
}

func TestStatusNoTokenSendsNoAuthHeader(t *testing.T) {
	f, ts := newFakeAPI(t)
	code, _, _ := runCtl(t, "--api", ts.URL, "status")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if f.lastAuth != "" {
		t.Errorf("Authorization = %q, want empty", f.lastAuth)
	}
}

func TestStatusServerError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer ts.Close()
	code, _, errOut := runCtl(t, "--api", ts.URL, "status")
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if !strings.Contains(errOut, "500") {
		t.Errorf("stderr = %q, want the HTTP status", errOut)
	}
}

func TestStatusMalformedJSON(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("{not json"))
	}))
	defer ts.Close()
	code, _, errOut := runCtl(t, "--api", ts.URL, "status")
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if errOut == "" {
		t.Error("stderr should report the decode error")
	}
}

func TestStatusUnreachable(t *testing.T) {
	code, _, errOut := runCtl(t, "--api", "http://127.0.0.1:1", "status")
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if !strings.Contains(errOut, "ctl:") {
		t.Errorf("stderr = %q", errOut)
	}
}

func TestInfoPrintsServerFields(t *testing.T) {
	_, ts := newFakeAPI(t)
	code, out, errOut := runCtl(t, "--api", ts.URL, "info")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, errOut)
	}
	for _, want := range []string{"pid:      4242", "version:  9.9.9", "uptime:   13s", "services: 2"} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout missing %q:\n%s", want, out)
		}
	}
}

func TestInfoServerError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusForbidden)
	}))
	defer ts.Close()
	code, _, errOut := runCtl(t, "--api", ts.URL, "info")
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if !strings.Contains(errOut, "403") {
		t.Errorf("stderr = %q", errOut)
	}
}

func TestActionOK(t *testing.T) {
	for _, op := range []string{"pause", "resume", "stop"} {
		f, ts := newFakeAPI(t)
		code, out, errOut := runCtl(t, "--api", ts.URL, "--token", "tk", op, "registry")
		if code != 0 {
			t.Fatalf("%s: exit = %d, stderr = %q", op, code, errOut)
		}
		if strings.TrimSpace(out) != "ok" {
			t.Errorf("%s: stdout = %q, want ok", op, out)
		}
		if f.lastPath != "/v1/services/registry/actions" {
			t.Errorf("%s: path = %q", op, f.lastPath)
		}
		if f.lastBody["action"] != op {
			t.Errorf("%s: body action = %q", op, f.lastBody["action"])
		}
		if f.lastAuth != "Bearer tk" {
			t.Errorf("%s: Authorization = %q", op, f.lastAuth)
		}
	}
}

func TestActionErrorWithJSONBody(t *testing.T) {
	f, ts := newFakeAPI(t)
	f.actionStatus = http.StatusConflict
	f.actionErr = "service not pausable"
	code, _, errOut := runCtl(t, "--api", ts.URL, "pause", "lsp")
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if !strings.Contains(errOut, "service not pausable") {
		t.Errorf("stderr = %q, want the server's error string", errOut)
	}
}

func TestActionErrorWithoutBodyFallsBackToStatus(t *testing.T) {
	f, ts := newFakeAPI(t)
	f.actionStatus = http.StatusInternalServerError
	code, _, errOut := runCtl(t, "--api", ts.URL, "stop", "lsp")
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if !strings.Contains(errOut, "500") {
		t.Errorf("stderr = %q, want the raw status", errOut)
	}
}

// tornListener accepts connections and immediately closes them so the
// HTTP client observes a transport-level EOF.
func tornListener(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()
	return "http://" + ln.Addr().String()
}

func TestStopTreatsTornConnectionAsSuccess(t *testing.T) {
	url := tornListener(t)
	code, out, errOut := runCtl(t, "--api", url, "stop", "api")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, errOut)
	}
	if strings.TrimSpace(out) != "ok" {
		t.Errorf("stdout = %q, want ok", out)
	}
}

func TestPauseTornConnectionIsAnError(t *testing.T) {
	url := tornListener(t)
	code, _, errOut := runCtl(t, "--api", url, "pause", "api")
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if errOut == "" {
		t.Error("stderr should carry the transport error")
	}
}

func TestIsConnectionTorn(t *testing.T) {
	cases := []struct {
		err  error
		want bool
	}{
		{nil, false},
		{errors.New("unexpected EOF"), true},
		{errors.New("read tcp: connection reset by peer"), true},
		// net/http's errServerClosedIdle — the same teardown, reported
		// differently when the transport's read loop wins the race.
		{errors.New("http: server closed idle connection"), true},
		{errors.New("dial tcp: connection declined"), false},
	}
	for _, c := range cases {
		if got := isConnectionTorn(c.err); got != c.want {
			t.Errorf("isConnectionTorn(%v) = %v, want %v", c.err, got, c.want)
		}
	}
}

// writeDiscoveryFile drops an api discovery file into a fresh TMPDIR
// so the no---api path can find it.
func writeDiscoveryFile(t *testing.T, url, token string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("TMPDIR", dir)
	body, err := json.Marshal(map[string]any{"url": url, "token": token, "pid": 1234})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "boru-api.json"), body, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestDiscoveryFileFallback(t *testing.T) {
	f, ts := newFakeAPI(t)
	writeDiscoveryFile(t, ts.URL, "disc-token")
	code, out, errOut := runCtl(t, "status")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, errOut)
	}
	if !strings.Contains(out, "registry") {
		t.Errorf("stdout = %q", out)
	}
	if f.lastAuth != "Bearer disc-token" {
		t.Errorf("Authorization = %q, want the discovery token", f.lastAuth)
	}
}

func TestDiscoveryFallbackKeepsExplicitToken(t *testing.T) {
	f, ts := newFakeAPI(t)
	writeDiscoveryFile(t, ts.URL, "disc-token")
	code, _, _ := runCtl(t, "--token", "cli-token", "status")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if f.lastAuth != "Bearer cli-token" {
		t.Errorf("Authorization = %q, want the explicit --token", f.lastAuth)
	}
}

func TestDiscoveryFileMissing(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir()) // empty: no discovery file
	code, _, errOut := runCtl(t, "status")
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if !strings.Contains(errOut, "discovery") {
		t.Errorf("stderr = %q, want a discovery-file hint", errOut)
	}
}
