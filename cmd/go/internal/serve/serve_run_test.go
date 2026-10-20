package serve

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/boru-lang/boru/cmd/go/internal/service"
)

// runServe invokes the subcommand the way main does.
func runServe(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := New().Run(args, bytes.NewReader(nil), &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func TestNameAndSynopsis(t *testing.T) {
	c := New()
	if c.Name() != "serve" {
		t.Errorf("Name = %q, want serve", c.Name())
	}
	if c.Synopsis() == "" {
		t.Error("Synopsis should not be empty")
	}
}

func TestRunHelp(t *testing.T) {
	for _, arg := range []string{"-h", "--help"} {
		code, out, _ := runServe(t, arg)
		if code != 0 {
			t.Errorf("%s: exit = %d, want 0", arg, code)
		}
		if !strings.Contains(out, "Usage: boru serve") {
			t.Errorf("%s: stdout should show usage; got %q", arg, out)
		}
		// Usage lists the known services.
		for _, svc := range factoryOrder {
			if !strings.Contains(out, svc) {
				t.Errorf("%s: usage missing service %q", arg, svc)
			}
		}
	}
}

func TestRunNoSegments(t *testing.T) {
	code, _, errOut := runServe(t)
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if !strings.Contains(errOut, "at least one service segment") {
		t.Errorf("stderr = %q", errOut)
	}
}

func TestRunConfigFlagRequiresPath(t *testing.T) {
	for _, args := range [][]string{{"-c"}, {"--config"}} {
		code, _, errOut := runServe(t, args...)
		if code != 1 {
			t.Errorf("%v: exit = %d, want 1", args, code)
		}
		if !strings.Contains(errOut, "requires a path") {
			t.Errorf("%v: stderr = %q", args, errOut)
		}
	}
}

func TestRunConfigMissingFile(t *testing.T) {
	code, _, errOut := runServe(t, "-c", filepath.Join(t.TempDir(), "nope.jsonic"))
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if !strings.Contains(errOut, "config") {
		t.Errorf("stderr = %q", errOut)
	}
}

func TestRunConfigEqualsFormExclusiveWithSegments(t *testing.T) {
	path := filepath.Join(t.TempDir(), "svc.jsonic")
	if err := os.WriteFile(path, []byte(`[{ name: api }]`), 0o600); err != nil {
		t.Fatal(err)
	}
	code, _, errOut := runServe(t, "--config="+path, "api")
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if !strings.Contains(errOut, "exclusive") {
		t.Errorf("stderr = %q", errOut)
	}
}

func TestRunUnknownService(t *testing.T) {
	code, _, errOut := runServe(t, "frobnicate")
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if !strings.Contains(errOut, `unknown service "frobnicate"`) {
		t.Errorf("stderr = %q", errOut)
	}
}

func TestRunStdioConflict(t *testing.T) {
	// repl and lsp-without--p both want stdio.
	code, _, errOut := runServe(t, "repl", "+", "lsp")
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if !strings.Contains(errOut, "stdio conflict") {
		t.Errorf("stderr = %q", errOut)
	}
}

func TestRunDuplicateNames(t *testing.T) {
	code, _, errOut := runServe(t, "api", "--bind", "127.0.0.1:0", "+", "api", "--bind", "127.0.0.1:0")
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if !strings.Contains(errOut, "listed twice") {
		t.Errorf("stderr = %q", errOut)
	}
}

func TestRunFactoryError(t *testing.T) {
	// registry without -r fails inside the factory.
	code, _, errOut := runServe(t, "registry")
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if !strings.Contains(errOut, "registry:") {
		t.Errorf("stderr = %q", errOut)
	}
}

// syncBuffer is a mutex-guarded bytes.Buffer safe for cross-goroutine
// write/inspect.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// TestRunSignalShutdown drives the full command: start one service,
// wait for the startup line (printed after the signal handler is
// installed), send SIGINT, and expect a clean exit 0.
func TestRunSignalShutdown(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("SIGINT delivery is not supported on windows")
	}
	t.Setenv("TMPDIR", t.TempDir()) // keep the api discovery file out of the shared tmp
	t.Setenv("TMP", t.TempDir())    // os.TempDir reads TMP, then TEMP, on Windows
	t.Setenv("TEMP", t.TempDir())

	var stdout, stderr syncBuffer
	done := make(chan int, 1)
	go func() {
		done <- New().Run([]string{"api", "--bind", "127.0.0.1:0"}, bytes.NewReader(nil), &stdout, &stderr)
	}()

	deadline := time.Now().Add(10 * time.Second)
	for !strings.Contains(stdout.String(), "boru serve: running") {
		if time.Now().After(deadline) {
			t.Fatalf("startup line never appeared; stderr=%s", stderr.String())
		}
		time.Sleep(10 * time.Millisecond)
	}

	self, _ := os.FindProcess(os.Getpid()) // not syscall.Kill: the file must build on Windows
	if err := self.Signal(os.Interrupt); err != nil {
		t.Fatal(err)
	}
	select {
	case code := <-done:
		if code != 0 {
			t.Errorf("exit = %d, want 0; stderr=%s", code, stderr.String())
		}
	case <-time.After(10 * time.Second):
		t.Fatal("serve did not exit after SIGINT")
	}
	if !strings.Contains(stdout.String(), "[api]") {
		t.Errorf("startup line should list the api service: %q", stdout.String())
	}
}

// --- factories ---

func TestFactoriesBuild(t *testing.T) {
	cases := []struct {
		segment []string
		name    string
	}{
		{[]string{"repl"}, "repl"},
		{[]string{"repl", "-r", "./mods"}, "repl"},
		{[]string{"lsp", "-p", "9999"}, "lsp"},
		{[]string{"lsp"}, "lsp"},
		{[]string{"api", "--bind", "127.0.0.1:0", "--token", "t"}, "api"},
		{[]string{"exec", "-p", "8123"}, "exec"},
		{[]string{"exec", "--perms", "sandbox"}, "exec"},
		{[]string{"tui", "--api", "http://x"}, "tui"},
	}
	for _, c := range cases {
		svcs, err := buildServices([][]string{c.segment}, bytes.NewReader(nil), io.Discard, io.Discard)
		if err != nil {
			t.Errorf("%v: %v", c.segment, err)
			continue
		}
		if len(svcs) != 1 || svcs[0].Name() != c.name {
			t.Errorf("%v: built %v, want one %q service", c.segment, svcs, c.name)
		}
	}
}

func TestFactoriesRejectBadFlags(t *testing.T) {
	for _, seg := range [][]string{
		{"repl", "--nope"},
		{"registry", "--nope"},
		{"lsp", "--nope"},
		{"api", "--nope"},
		{"exec", "--nope"},
		{"tui", "--nope"},
		{"vault-proxy", "--nope"},
	} {
		if _, err := buildServices([][]string{seg}, bytes.NewReader(nil), io.Discard, io.Discard); err == nil {
			t.Errorf("%v: expected a flag error", seg)
		}
	}
}

func TestExecFactoryBadPolicy(t *testing.T) {
	_, err := buildServices([][]string{{"exec", "--perms", "no-such-profile"}}, bytes.NewReader(nil), io.Discard, io.Discard)
	if err == nil {
		t.Error("expected a policy resolution error")
	}
}

// --- config edge cases ---

func TestLoadConfigTopLevelNotList(t *testing.T) {
	path := filepath.Join(t.TempDir(), "svc.jsonic")
	if err := os.WriteFile(path, []byte(`{ name: api }`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := loadConfig(path)
	if err == nil || !strings.Contains(err.Error(), "top-level must be a list") {
		t.Errorf("err = %v", err)
	}
}

func TestSegmentFromSpecErrors(t *testing.T) {
	if _, err := segmentFromSpec("not-a-map"); err == nil || !strings.Contains(err.Error(), "expected a map") {
		t.Errorf("err = %v, want expected-a-map", err)
	}
	if _, err := segmentFromSpec(map[string]any{"name": "api", "flags": "oops"}); err == nil ||
		!strings.Contains(err.Error(), "must be a list") {
		t.Errorf("err = %v, want flags-must-be-a-list", err)
	}
	if _, err := segmentFromSpec(map[string]any{"name": "api", "flags": []any{1}}); err == nil ||
		!strings.Contains(err.Error(), "not a string") {
		t.Errorf("err = %v, want flag-not-a-string", err)
	}
}

func TestSegmentFromSpecNoFlags(t *testing.T) {
	seg, err := segmentFromSpec(map[string]any{"name": "api"})
	if err != nil {
		t.Fatal(err)
	}
	if len(seg) != 1 || seg[0] != "api" {
		t.Errorf("seg = %v, want [api]", seg)
	}
}

// --- supervisor edges ---

// boundFake also implements SupervisorBound to cover the late-binding
// hook in run().
type boundFake struct {
	*fakeService
	bound service.Inspector
}

func (b *boundFake) Bind(insp service.Inspector) { b.bound = insp }

func TestSupervisorBindsAndReportsErrors(t *testing.T) {
	okSvc := &boundFake{fakeService: newFakeService("ok", false, false)}
	failing := newFakeService("bad", false, false)
	failing.startErr = errors.New("kaboom")

	var stdout, stderr bytes.Buffer
	sup := newSupervisor([]service.Service{okSvc, failing}, &stdout, &stderr)

	if !sup.StartTime().IsZero() {
		t.Error("StartTime should be zero before run")
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan int, 1)
	go func() { done <- sup.run(ctx) }()

	<-okSvc.startCh
	cancel()

	select {
	case code := <-done:
		if code != 1 {
			t.Errorf("exit = %d, want 1 (a service failed)", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("supervisor did not exit")
	}

	if okSvc.bound == nil {
		t.Error("SupervisorBound service should have been bound")
	}
	if sup.StartTime().IsZero() {
		t.Error("StartTime should be set after run")
	}
	if !strings.Contains(stderr.String(), "kaboom") {
		t.Errorf("stderr should report the failing service: %q", stderr.String())
	}
}

// --- segment splitting (edge shapes) ---

func TestSplitSegmentsEdges(t *testing.T) {
	cases := []struct {
		in   []string
		want int
	}{
		{nil, 0},
		{[]string{"+"}, 0},
		{[]string{"+", "+", "+"}, 0},
		{[]string{"a"}, 1},
		{[]string{"+", "a", "+"}, 1},
		{[]string{"a", "+", "+", "b"}, 2},
	}
	for _, c := range cases {
		if got := splitSegments(c.in); len(got) != c.want {
			t.Errorf("splitSegments(%v) = %v, want %d segments", c.in, got, c.want)
		}
	}
}
