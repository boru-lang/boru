//go:build unix

package vault

// These lifecycle tests deliver POSIX SIGTERM to the test process.
// Shared credential/clipboard tests remain runnable on Windows.
import (
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestRunProxyLifecycleUnderAdmin drives runProxy end to end: it starts
// under an envelope-admin passphrase (emitting the admin warning) and
// shuts down cleanly on SIGTERM.
func TestRunProxyLifecycleUnderAdmin(t *testing.T) {
	home := w4EnvelopeVault(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	var stdout, stderr w4SyncBuffer
	done := make(chan int, 1)
	go func() {
		done <- runProxy([]string{"--listen=" + addr}, home, &stdout, &stderr)
	}()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 50*time.Millisecond)
		if err == nil {
			conn.Close()
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := syscall.Kill(os.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	select {
	case code := <-done:
		if code != 0 {
			t.Errorf("runProxy exit = %d, want 0 (stderr: %q)", code, stderr.String())
		}
	case <-time.After(10 * time.Second):
		t.Fatal("runProxy did not shut down on SIGTERM")
	}
	if !strings.Contains(stderr.String(), "ADMIN password") {
		t.Errorf("missing the admin-password warning: %q", stderr.String())
	}
}

// TestRunServeLifecycleUnderAdmin drives runServe end to end: it starts
// under an envelope-admin passphrase (emitting the admin warning),
// answers real wire requests — health, a wildcard-token KV read, a LIST
// — and shuts down cleanly on SIGTERM.
func TestRunServeLifecycleUnderAdmin(t *testing.T) {
	home := w4EnvelopeVault(t)
	token := grantWire(t, "--agent=sekreto", "proj:*")

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	var stdout, stderr w4SyncBuffer
	done := make(chan int, 1)
	go func() {
		done <- runServe([]string{"--listen=" + addr}, home, &stdout, &stderr)
	}()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		conn, derr := net.DialTimeout("tcp", addr, 50*time.Millisecond)
		if derr == nil {
			conn.Close()
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	base := "http://" + addr
	resp, err := http.Get(base + "/v1/sys/health")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("health over the wire = %d", resp.StatusCode)
	}
	req, _ := http.NewRequest("GET", base+"/v1/secret/data/proj/k", nil)
	req.Header.Set(headerVaultToken, token)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), `"value":"v"`) {
		t.Errorf("wire read = %d, %q", resp.StatusCode, body)
	}
	req, _ = http.NewRequest("LIST", base+"/v1/secret/metadata/proj", nil)
	req.Header.Set(headerVaultToken, token)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), `"keys":["k"]`) {
		t.Errorf("wire list = %d, %q", resp.StatusCode, body)
	}

	if err := syscall.Kill(os.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	select {
	case code := <-done:
		if code != 0 {
			t.Errorf("runServe exit = %d, want 0 (stderr: %q)", code, stderr.String())
		}
	case <-time.After(10 * time.Second):
		t.Fatal("runServe did not shut down on SIGTERM")
	}
	if !strings.Contains(stderr.String(), "ADMIN password") {
		t.Errorf("missing the admin-password warning: %q", stderr.String())
	}
	if !strings.Contains(stdout.String(), "wire protocol listening") {
		t.Errorf("missing the listening line: %q", stdout.String())
	}
}

// TestProxyRefusesToCacheTemporaryPassword drives runProxy under a temporary
// password: it must warn and NOT cache the session (so per-request auth keeps
// re-checking expiry).
func TestProxyRefusesToCacheTemporaryPassword(t *testing.T) {
	home := tempBrokerVault(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	var stdout, stderr w4SyncBuffer
	done := make(chan int, 1)
	go func() { done <- runProxy([]string{"--listen=" + addr}, home, &stdout, &stderr) }()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if conn, e := net.DialTimeout("tcp", addr, 50*time.Millisecond); e == nil {
			conn.Close()
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := syscall.Kill(os.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("runProxy did not shut down on SIGTERM")
	}
	if !strings.Contains(stderr.String(), "TEMPORARY password") {
		t.Errorf("missing temporary-password warning: %q", stderr.String())
	}
}
