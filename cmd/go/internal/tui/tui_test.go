package tui

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/boru-lang/boru/cmd/go/internal/service"
)

// setDiscovery points $TMPDIR at a fresh dir and optionally writes an
// api discovery file there.
func setDiscovery(t *testing.T, url, token string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("TMPDIR", dir)
	t.Setenv("TMP", dir) // os.TempDir reads TMP, then TEMP, on Windows
	t.Setenv("TEMP", dir)
	if url == "" {
		return
	}
	body, err := json.Marshal(map[string]any{"url": url, "token": token, "pid": 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "boru-api.json"), body, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestNameAndSynopsis(t *testing.T) {
	c := New()
	if c.Name() != "tui" {
		t.Errorf("Name = %q, want tui", c.Name())
	}
	if c.Synopsis() == "" {
		t.Error("Synopsis should not be empty")
	}
}

func TestParseFlags(t *testing.T) {
	apiURL, token, err := parseFlags([]string{"--api", "http://x", "--token", "tk"}, io.Discard)
	if err != nil {
		t.Fatalf("parseFlags: %v", err)
	}
	if apiURL != "http://x" || token != "tk" {
		t.Errorf("got (%q, %q)", apiURL, token)
	}

	if _, _, err := parseFlags([]string{"--nope"}, io.Discard); err == nil {
		t.Error("expected an error for an unknown flag")
	}
}

func TestRunBadFlag(t *testing.T) {
	var stderr bytes.Buffer
	if code := New().Run([]string{"--nope"}, nil, io.Discard, &stderr); code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
}

func TestRunNoDiscovery(t *testing.T) {
	setDiscovery(t, "", "") // empty TMPDIR: no discovery file
	var stderr bytes.Buffer
	code := New().Run(nil, bytes.NewReader(nil), io.Discard, &stderr)
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "tui:") {
		t.Errorf("stderr = %q", stderr.String())
	}
}

func TestReadDiscoveryWithRetry(t *testing.T) {
	setDiscovery(t, "http://api.example", "tok")
	url, token, err := readDiscoveryWithRetry(io.Discard)
	if err != nil {
		t.Fatalf("readDiscoveryWithRetry: %v", err)
	}
	if url != "http://api.example" || token != "tok" {
		t.Errorf("got (%q, %q)", url, token)
	}
}

func TestReadDiscoveryWithRetryTimesOut(t *testing.T) {
	setDiscovery(t, "", "")
	start := time.Now()
	_, _, err := readDiscoveryWithRetry(io.Discard)
	if err == nil {
		t.Fatal("expected an error when no discovery file ever appears")
	}
	if elapsed := time.Since(start); elapsed < discoveryRetryBudget {
		t.Errorf("gave up after %v, want at least the %v budget", elapsed, discoveryRetryBudget)
	}
}

func TestNewServerAccessors(t *testing.T) {
	s := NewServer("http://api", "tok", bytes.NewReader(nil), io.Discard, io.Discard)
	if s.Name() != "tui" {
		t.Errorf("Name = %q", s.Name())
	}
	if !s.UsesStdio() {
		t.Error("UsesStdio should be true")
	}
	if s.Status() != service.StateStopped {
		t.Errorf("initial Status = %v, want stopped", s.Status())
	}
	if got := s.Metadata()["api"]; got != "http://api" {
		t.Errorf("Metadata[api] = %q", got)
	}
}

func TestServerStopBeforeStartIsSafe(t *testing.T) {
	s := NewServer("http://api", "", bytes.NewReader(nil), io.Discard, io.Discard)
	if err := s.Stop(context.Background()); err != nil {
		t.Errorf("Stop before Start: %v", err)
	}
}

func TestServerStartNoDiscovery(t *testing.T) {
	setDiscovery(t, "", "")
	s := NewServer("", "", bytes.NewReader(nil), io.Discard, io.Discard)
	err := s.Start(context.Background())
	if err == nil {
		t.Fatal("Start with no api URL and no discovery file should fail")
	}
	if s.Status() != service.StateStopped {
		t.Errorf("Status after failed Start = %v, want stopped", s.Status())
	}
}

// waitFor polls until cond is true or the deadline passes.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestServerStartAndStop(t *testing.T) {
	f := &fakeAPI{}
	ts := f.start(t)

	// neverReader blocks forever so bubbletea's input loop stays open.
	in, w := io.Pipe()
	defer w.Close()

	var out bytes.Buffer
	s := NewServer(ts.URL, "", in, &out, io.Discard)

	done := make(chan error, 1)
	go func() { done <- s.Start(context.Background()) }()

	waitFor(t, "running state", func() bool { return s.Status() == service.StateRunning })

	if err := s.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Start returned %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not return after Stop")
	}
	if s.Status() != service.StateStopped {
		t.Errorf("Status after Stop = %v, want stopped", s.Status())
	}
}

func TestServerStartUsesDiscoveryFile(t *testing.T) {
	f := &fakeAPI{}
	ts := f.start(t)
	setDiscovery(t, ts.URL, "disc-tok")

	in, w := io.Pipe()
	defer w.Close()

	var out bytes.Buffer
	s := NewServer("", "", in, &out, io.Discard)

	done := make(chan error, 1)
	go func() { done <- s.Start(context.Background()) }()

	waitFor(t, "running state", func() bool { return s.Status() == service.StateRunning })

	if s.apiURL != ts.URL {
		t.Errorf("apiURL = %q, want %q from discovery", s.apiURL, ts.URL)
	}
	if s.token != "disc-tok" {
		t.Errorf("token = %q, want disc-tok from discovery", s.token)
	}

	_ = s.Stop(context.Background())
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not return after Stop")
	}
}

func TestServerStartCancelledContext(t *testing.T) {
	f := &fakeAPI{}
	ts := f.start(t)

	in, w := io.Pipe()
	defer w.Close()

	s := NewServer(ts.URL, "", in, io.Discard, io.Discard)
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() { done <- s.Start(ctx) }()

	waitFor(t, "running state", func() bool { return s.Status() == service.StateRunning })
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Start after ctx cancel returned %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not return after ctx cancel")
	}
}

func TestRunQuitsOnQ(t *testing.T) {
	f := &fakeAPI{}
	ts := f.start(t)

	var out bytes.Buffer
	code := New().Run([]string{"--api", ts.URL}, strings.NewReader("q"), &out, io.Discard)
	if code != 0 {
		t.Errorf("exit = %d, want 0", code)
	}
}
