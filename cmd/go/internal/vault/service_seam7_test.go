package vault

// service_seam7_test.go — ProxyService lifecycle arms not reached by the
// existing tests: the homeDir() resolution error, the ctx.Done() graceful
// shutdown, and Serve returning a non-ErrServerClosed error.

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/boru-lang/boru/cmd/go/internal/service"
)

func s7waitRunning(t *testing.T, ps *ProxyService) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for ps.Status() != service.StateRunning {
		if time.Now().After(deadline) {
			t.Fatal("service did not reach running")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestS7_NewProxyServiceHomeDirError(t *testing.T) {
	// No BORU_HOME and no HOME → homeDir() fails → NewProxyService wraps it.
	t.Setenv(EnvHome, "")
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "") // os.UserHomeDir reads USERPROFILE on Windows
	if _, err := NewProxyService("", "", ""); err == nil ||
		!strings.Contains(err.Error(), "vault-proxy") {
		t.Fatalf("NewProxyService home error = %v, want vault-proxy wrap", err)
	}
}

func TestS7_ProxyServiceContextCancelShutdown(t *testing.T) {
	home := testHome(t)
	mustInit(t)
	ps, err := NewProxyService("127.0.0.1:0", home, "test-pass")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- ps.Start(ctx) }()
	s7waitRunning(t, ps)
	cancel() // drives the <-ctx.Done() graceful-shutdown arm
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Start after ctx cancel = %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not return after ctx cancel")
	}
	if ps.Status() != service.StateStopped {
		t.Errorf("state = %v, want stopped", ps.Status())
	}
}

func TestS7_ProxyServiceServeError(t *testing.T) {
	home := testHome(t)
	mustInit(t)
	ps, err := NewProxyService("127.0.0.1:0", home, "test-pass")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- ps.Start(context.Background()) }()
	s7waitRunning(t, ps)
	// Close the listener out from under Serve so it returns a non-
	// ErrServerClosed error, exercising Start's error return path.
	ps.mu.Lock()
	ln := ps.ln
	ps.mu.Unlock()
	if err := ln.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err == nil {
			t.Error("Start returned nil after the listener was closed")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not return after listener close")
	}
	if ps.Status() != service.StateStopped {
		t.Errorf("state = %v, want stopped", ps.Status())
	}
}
