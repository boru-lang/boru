package debugcmd

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/boru-lang/boru/lang/go/debugserve"
	"github.com/boru-lang/boru/lang/go/native"
	"github.com/boru-lang/boru/parser/go"
)

// runDebug invokes the subcommand the way main does.
func runDebug(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := New().Run(args, nil, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

// startAttachServer stands up a debugserve backend over a default
// registry and returns its base URL.
func startAttachServer(t *testing.T) string {
	t.Helper()
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	reg.SetParseFunc(parser.Parse)
	ts := httptest.NewServer(debugserve.NewServer(reg, "").Handler())
	t.Cleanup(ts.Close)
	return ts.URL
}

func TestNameSynopsisNew(t *testing.T) {
	c := New()
	if c.Name() != "debug" {
		t.Errorf("Name = %q, want debug", c.Name())
	}
	if c.Synopsis() == "" {
		t.Error("Synopsis should not be empty")
	}
}

func TestRunNoArgs(t *testing.T) {
	code, _, errOut := runDebug(t)
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if !strings.Contains(errOut, "usage: boru debug") {
		t.Errorf("stderr = %q", errOut)
	}
}

func TestRunBarePositionalIsLaunch(t *testing.T) {
	// A bare positional that is not serve|attach is an interactive-debugger
	// launch (design/BORU-DEBUGGER.0.md §4); a nonexistent file fails its read.
	code, _, errOut := runDebug(t, "frobnicate")
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if !strings.Contains(errOut, "read frobnicate") {
		t.Errorf("stderr = %q, want a read error for the launch file", errOut)
	}
}

func TestDiscoveryRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TMPDIR", dir)
	t.Setenv("TMP", dir) // os.TempDir reads TMP, then TEMP, on Windows
	t.Setenv("TEMP", dir)
	dp := discoveryPath()
	if dp != filepath.Join(dir, "boru-debug.json") {
		t.Errorf("discoveryPath = %q", dp)
	}
	writeDiscovery(dp, "127.0.0.1:7777", "tok")
	url, token, err := readDiscovery(dp)
	if err != nil {
		t.Fatalf("readDiscovery: %v", err)
	}
	if url != "http://127.0.0.1:7777" || token != "tok" {
		t.Errorf("got (%q, %q)", url, token)
	}
}

func TestReadDiscoveryMissing(t *testing.T) {
	if _, _, err := readDiscovery(filepath.Join(t.TempDir(), "nope.json")); err == nil {
		t.Error("expected an error for a missing discovery file")
	}
}

func TestReadDiscoveryMalformed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "boru-debug.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := readDiscovery(path); err == nil {
		t.Error("expected an error for malformed JSON")
	}
}

func TestRunAttachExplicitURL(t *testing.T) {
	url := startAttachServer(t)
	code, out, errOut := runDebug(t, "attach", "--url", url, "words")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, errOut)
	}
	if !strings.Contains(out, "add") {
		t.Errorf("words output missing add:\n%s", out)
	}
}

func TestRunAttachViaDiscoveryFile(t *testing.T) {
	url := startAttachServer(t)
	dir := t.TempDir()
	t.Setenv("TMPDIR", dir)
	t.Setenv("TMP", dir) // os.TempDir reads TMP, then TEMP, on Windows
	t.Setenv("TEMP", dir)
	body, err := json.Marshal(map[string]any{"url": url, "token": "", "pid": os.Getpid()})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "boru-debug.json"), body, 0o600); err != nil {
		t.Fatal(err)
	}
	code, out, errOut := runDebug(t, "attach", "heap")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, errOut)
	}
	if !strings.Contains(out, "alloc=") {
		t.Errorf("heap output = %q", out)
	}
}

func TestRunAttachBadFlag(t *testing.T) {
	code, _, _ := runDebug(t, "attach", "--nope")
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
}

func TestRunAttachNoVerb(t *testing.T) {
	code, _, errOut := runDebug(t, "attach", "--url", "http://127.0.0.1:1")
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if !strings.Contains(errOut, "usage: boru debug attach") {
		t.Errorf("stderr = %q", errOut)
	}
}

func TestRunAttachNoDiscovery(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	t.Setenv("TMP", t.TempDir()) // os.TempDir reads TMP, then TEMP, on Windows
	t.Setenv("TEMP", t.TempDir())
	code, _, errOut := runDebug(t, "attach", "words")
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if !strings.Contains(errOut, "no --url and no discovery file") {
		t.Errorf("stderr = %q", errOut)
	}
}

func TestAttachVerbEvalError(t *testing.T) {
	c, _, closeFn := attachToTestServer(t)
	defer closeFn()
	var out, errOut bytes.Buffer
	code := attachVerb(c, []string{"eval", "no-such-word-xyz"}, &out, &errOut)
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if !strings.Contains(errOut.String(), "eval error:") {
		t.Errorf("stderr = %q", errOut.String())
	}
}

func TestAttachVerbEventsUsage(t *testing.T) {
	c, _, closeFn := attachToTestServer(t)
	defer closeFn()
	var out, errOut bytes.Buffer
	code := attachVerb(c, []string{"events"}, &out, &errOut)
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if !strings.Contains(errOut.String(), "usage: boru debug attach events") {
		t.Errorf("stderr = %q", errOut.String())
	}
}

func TestAttachVerbUnreachableServer(t *testing.T) {
	c := debugserve.NewClient("http://127.0.0.1:1", "")
	for _, verb := range [][]string{{"words"}, {"defs"}, {"heap"}, {"eval", "1"}, {"events", "x"}} {
		var out, errOut bytes.Buffer
		if code := attachVerb(c, verb, &out, &errOut); code != 1 {
			t.Errorf("%v: exit = %d, want 1", verb, code)
		}
		if !strings.Contains(errOut.String(), "debug attach:") {
			t.Errorf("%v: stderr = %q", verb, errOut.String())
		}
	}
}

func TestRunServeBadFlag(t *testing.T) {
	code, _, _ := runDebug(t, "serve", "--nope")
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
}

func TestRunServeMissingFile(t *testing.T) {
	code, _, errOut := runDebug(t, "serve", filepath.Join(t.TempDir(), "nope.boru"))
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if !strings.Contains(errOut, "read") {
		t.Errorf("stderr = %q", errOut)
	}
}

func TestRunServeBadProgram(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.boru")
	if err := os.WriteFile(path, []byte("no-such-word-xyz-12345"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, _, errOut := runDebug(t, "serve", path)
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if !strings.Contains(errOut, "run") {
		t.Errorf("stderr = %q", errOut)
	}
}

// TestServeShutdownOnSignal runs `debug serve` on a loopback port,
// waits for the discovery file (written after the signal handler is
// installed), then interrupts the serve loop with SIGINT, which
// signal.NotifyContext converts into a clean shutdown.
func TestServeShutdownOnSignal(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("SIGINT delivery is not supported on windows")
	}
	dir := t.TempDir()
	t.Setenv("TMPDIR", dir)
	t.Setenv("TMP", dir) // os.TempDir reads TMP, then TEMP, on Windows
	t.Setenv("TEMP", dir)

	prog := filepath.Join(dir, "p.boru")
	if err := os.WriteFile(prog, []byte("def served-marker 41"), 0o600); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	done := make(chan int, 1)
	go func() {
		done <- runServe([]string{"--bind", "127.0.0.1:0", prog}, &out, &errOut)
	}()

	dp := filepath.Join(dir, "boru-debug.json")
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(dp); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("discovery file never appeared; stderr=%s", errOut.String())
		}
		time.Sleep(10 * time.Millisecond)
	}

	data, err := os.ReadFile(dp)
	if err != nil {
		t.Fatal(err)
	}
	var d struct {
		URL string `json:"url"`
		PID int    `json:"pid"`
	}
	if err := json.Unmarshal(data, &d); err != nil {
		t.Fatalf("discovery file malformed: %v", err)
	}
	if d.PID != os.Getpid() {
		t.Errorf("discovery pid = %d, want %d", d.PID, os.Getpid())
	}

	self, _ := os.FindProcess(os.Getpid()) // not syscall.Kill: the file must build on Windows
	if err := self.Signal(os.Interrupt); err != nil {
		t.Fatal(err)
	}
	select {
	case code := <-done:
		if code != 0 {
			t.Errorf("runServe exit = %d, want 0; stderr=%s", code, errOut.String())
		}
	case <-time.After(10 * time.Second):
		t.Fatal("runServe did not exit after SIGINT")
	}
	// Discovery file is removed on the way out.
	if _, err := os.Stat(dp); !os.IsNotExist(err) {
		t.Errorf("discovery file should be removed on shutdown (stat err=%v)", err)
	}
}
