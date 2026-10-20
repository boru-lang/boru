package registry

import (
	"bytes"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"
)

// TestS7n_RunSIGINTCleanShutdown drives run()'s final "return 0" arm
// (registry.go): a SIGINT during a live serve cancels the
// signal.NotifyContext-derived ctx, Server.Start shuts down
// gracefully and returns nil, and run() reports a clean exit.
func TestS7n_RunSIGINTCleanShutdown(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("SIGINT delivery is not supported on windows")
	}
	dir := t.TempDir()
	var stdout, stderr bytes.Buffer
	done := make(chan int, 1)
	go func() {
		done <- run([]string{"-r", dir, "-p", "0"}, &stdout, &stderr)
	}()

	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(stdout.String(), "boru registry serving") {
		if time.Now().After(deadline) {
			t.Fatalf("serving banner never appeared; stdout=%q stderr=%q", stdout.String(), stderr.String())
		}
		time.Sleep(5 * time.Millisecond)
	}

	self, _ := os.FindProcess(os.Getpid()) // not syscall.Kill: the file must build on Windows
	if err := self.Signal(os.Interrupt); err != nil {
		t.Fatal(err)
	}

	select {
	case code := <-done:
		if code != 0 {
			t.Errorf("run() = %d, want 0; stderr: %s", code, stderr.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("run did not exit after SIGINT")
	}
}
