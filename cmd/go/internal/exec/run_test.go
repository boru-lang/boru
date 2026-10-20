// Tests for the `boru exec` subcommand plumbing (exec.go run/Run):
// flag validation, policy resolution failures, listen failures, and
// the SIGINT-driven graceful shutdown of a live server.

package exec

import (
	"bytes"
	"fmt"
	"net"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// lockedBuffer guards a bytes.Buffer that the run goroutine writes
// while the test may still be polling.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestCmdMetadata(t *testing.T) {
	c := New()
	if c.Name() != "exec" {
		t.Errorf("Name() = %q, want exec", c.Name())
	}
	if !strings.Contains(c.Synopsis(), "HTTP") {
		t.Errorf("Synopsis() = %q, want to mention HTTP", c.Synopsis())
	}
}

func TestRunRejectsBadFlag(t *testing.T) {
	var out, stderr bytes.Buffer
	code := New().Run([]string{"-definitely-not-a-flag"}, nil, &out, &stderr)
	if code != 1 {
		t.Errorf("Run with bad flag = %d, want 1", code)
	}
}

func TestRunRejectsConflictingPolicyFlags(t *testing.T) {
	var out, stderr bytes.Buffer
	code := run([]string{"-perms", "sandbox", "-perms-file", "x.jsonic"}, &out, &stderr)
	if code != 1 {
		t.Errorf("run = %d, want 1 for mutually exclusive policy flags", code)
	}
	if !strings.Contains(stderr.String(), "mutually exclusive") {
		t.Errorf("stderr = %q, want the mutual-exclusion error", stderr.String())
	}
}

func TestRunReportsListenFailure(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	var out, stderr bytes.Buffer
	code := run([]string{"-bind", ln.Addr().String(), "-perms", "sandbox"}, &out, &stderr)
	if code != 1 {
		t.Errorf("run = %d, want 1 when the bind address is occupied", code)
	}
	if !strings.Contains(out.String(), "serving on") {
		t.Errorf("stdout = %q, want the serving banner", out.String())
	}
	if !strings.Contains(out.String(), "policy: sandbox") {
		t.Errorf("stdout = %q, want the policy banner", out.String())
	}
	if !strings.Contains(stderr.String(), "listen") {
		t.Errorf("stderr = %q, want the listen error", stderr.String())
	}
}

func TestRunPortFlagOverridesBind(t *testing.T) {
	// Occupy an all-interfaces port, then ask run for the same port
	// via -p: the override must be honoured (and then fail to bind,
	// which is how we observe it without a live server).
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port

	var out, stderr bytes.Buffer
	code := run([]string{"-p", strconv.Itoa(port)}, &out, &stderr)
	if code != 1 {
		t.Errorf("run = %d, want 1", code)
	}
	wantAddr := fmt.Sprintf(":%d", port)
	if !strings.Contains(out.String(), "serving on "+wantAddr) {
		t.Errorf("stdout = %q, want serving banner for %q (-p must override -bind)", out.String(), wantAddr)
	}
}

func TestRunServesUntilInterrupt(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("SIGINT delivery is not supported on windows")
	}

	// Reserve a free loopback port, release it, and hand it to run.
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := probe.Addr().String()
	probe.Close()

	out := &lockedBuffer{}
	stderr := &lockedBuffer{}
	codeCh := make(chan int, 1)
	go func() { codeCh <- run([]string{"-bind", addr}, out, stderr) }()

	// Wait for the server to answer before signalling: run is then
	// guaranteed to be blocked in Start with the SIGINT handler
	// installed.
	healthz := "http://" + addr + "/healthz"
	deadline := time.Now().Add(5 * time.Second)
	up := false
	for time.Now().Before(deadline) {
		resp, err := http.Get(healthz)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				up = true
				break
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !up {
		t.Fatalf("exec server never came up on %s (stderr: %s)", addr, stderr.String())
	}

	self, _ := os.FindProcess(os.Getpid()) // not syscall.Kill: the file must build on Windows
	if err := self.Signal(os.Interrupt); err != nil {
		t.Fatalf("kill: %s", err)
	}

	select {
	case code := <-codeCh:
		if code != 0 {
			t.Errorf("run = %d, want 0 after graceful SIGINT shutdown (stderr: %s)", code, stderr.String())
		}
	case <-time.After(10 * time.Second):
		t.Fatal("run did not return after SIGINT")
	}
	if !strings.Contains(out.String(), "serving on "+addr) {
		t.Errorf("stdout = %q, want the serving banner for %s", out.String(), addr)
	}
}
