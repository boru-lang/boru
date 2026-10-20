// Wave-4 coverage for the model subcommand: the --watch arm (started,
// then interrupted via a self-delivered SIGINT), the watch-with-error
// early return, and the outputPath extension handling.
package model

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	model "github.com/voxgig/model/go"
)

func TestW4WatchStartsAndStopsOnSIGINT(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("SIGINT delivery is not supported on windows")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "m.jsonic")
	if err := os.WriteFile(src, []byte("a: 1"), 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	done := make(chan int, 1)
	go func() {
		done <- New().Run([]string{"--watch", src}, nil, &stdout, &stderr)
	}()

	// Wait until the watcher announces itself, then interrupt.
	deadline := time.Now().Add(10 * time.Second)
	for !strings.Contains(stderr.String(), "watching for changes") {
		if time.Now().After(deadline) {
			t.Fatalf("watch banner never appeared; stderr=%q stdout=%q", stderr.String(), stdout.String())
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
			t.Errorf("watch run = %d, want 0; stderr: %s", code, stderr.String())
		}
	case <-time.After(10 * time.Second):
		t.Fatal("watch did not exit after SIGINT")
	}
	if !strings.Contains(stdout.String(), "wrote ") {
		t.Errorf("stdout = %q, want the initial build report", stdout.String())
	}
}

func TestW4WatchFailsFast(t *testing.T) {
	// A missing model file makes the initial watch build fail, so Run
	// returns non-zero without ever blocking on the signal.
	var stdout, stderr bytes.Buffer
	code := New().Run([]string{"--watch", filepath.Join(t.TempDir(), "zz-missing.jsonic")}, nil, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("watch on missing file = %d, want 1; stderr: %s", code, stderr.String())
	}
}

func TestW4OutputPathNoExtension(t *testing.T) {
	got := outputPath(model.ModelSpec{}, filepath.Join("some", "dir", "plain"))
	want := filepath.Join("some", "dir", "plain.json")
	if got != want {
		t.Errorf("outputPath(no ext) = %q, want %q", got, want)
	}
}

func TestW4OutputPathExplicitBase(t *testing.T) {
	got := outputPath(model.ModelSpec{Base: "outbase"}, filepath.Join("x", "m.jsonic"))
	want := filepath.Join("outbase", "m.json")
	if got != want {
		t.Errorf("outputPath(base) = %q, want %q", got, want)
	}
}
