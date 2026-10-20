// Wave-4 coverage for the login subcommand: prompt failures,
// required-field validation, transport and server-rejection arms, the
// invalid-JSON response, home-dir resolution failure, credential-save
// failure, and the Command wrapper methods.
package login

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// okLoginServer answers /api/login with a valid token response.
func okLoginServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"username":"alice","email":"a@b.c","token":"tok123"}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestW4CommandMethods(t *testing.T) {
	c := New()
	if c.Name() != "login" {
		t.Errorf("Name() = %q, want login", c.Name())
	}
	if c.Synopsis() == "" {
		t.Error("Synopsis() is empty")
	}
}

func TestW4LoginBadFlag(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := New().Run([]string{"-zz-bad"}, strings.NewReader(""), &out, &errOut); code != 1 {
		t.Errorf("Run(bad flag) = %d, want 1", code)
	}
}

func TestW4LoginUsernameEOF(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := Run(nil, strings.NewReader(""), &out, &errOut); code != 1 {
		t.Fatalf("Run = %d, want 1", code)
	}
	if !strings.Contains(errOut.String(), "no input") {
		t.Errorf("stderr = %q, want no-input error", errOut.String())
	}
}

func TestW4LoginPasswordEOF(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := Run(nil, strings.NewReader("alice\n"), &out, &errOut); code != 1 {
		t.Fatalf("Run = %d, want 1", code)
	}
}

func TestW4LoginEmptyFields(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := Run(nil, strings.NewReader("\n\n"), &out, &errOut); code != 1 {
		t.Fatalf("Run = %d, want 1", code)
	}
	if !strings.Contains(errOut.String(), "required") {
		t.Errorf("stderr = %q, want required error", errOut.String())
	}
}

func TestW4LoginConnectionError(t *testing.T) {
	var out, errOut bytes.Buffer
	code := Run([]string{"-r", "http://127.0.0.1:1"}, strings.NewReader("alice\npw\n"), &out, &errOut)
	if code != 1 {
		t.Fatalf("Run = %d, want 1", code)
	}
}

func TestW4LoginServerRejects(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
	}))
	t.Cleanup(srv.Close)

	var out, errOut bytes.Buffer
	code := Run([]string{"-r", srv.URL}, strings.NewReader("alice\npw\n"), &out, &errOut)
	if code != 1 {
		t.Fatalf("Run = %d, want 1", code)
	}
	if !strings.Contains(errOut.String(), "invalid credentials") {
		t.Errorf("stderr = %q, want rejection text", errOut.String())
	}
}

func TestW4LoginInvalidJSONResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("{not json"))
	}))
	t.Cleanup(srv.Close)

	var out, errOut bytes.Buffer
	code := Run([]string{"-r", srv.URL}, strings.NewReader("alice\npw\n"), &out, &errOut)
	if code != 1 {
		t.Fatalf("Run = %d, want 1", code)
	}
	if !strings.Contains(errOut.String(), "invalid response") {
		t.Errorf("stderr = %q, want invalid-response error", errOut.String())
	}
}

func TestW4LoginHomeDirError(t *testing.T) {
	srv := okLoginServer(t)
	t.Setenv("HOME", "")        // os.UserHomeDir fails with an empty $HOME
	t.Setenv("USERPROFILE", "") // os.UserHomeDir reads USERPROFILE on Windows

	var out, errOut bytes.Buffer
	code := Run([]string{"-r", srv.URL}, strings.NewReader("alice\npw\n"), &out, &errOut)
	if code != 1 {
		t.Fatalf("Run = %d, want 1; stderr: %s", code, errOut.String())
	}
	if !strings.Contains(errOut.String(), "error:") {
		t.Errorf("stderr = %q, want home-dir error", errOut.String())
	}
}

func TestW4LoginSaveError(t *testing.T) {
	srv := okLoginServer(t)
	home := t.TempDir()
	// $HOME/.boru exists as a FILE, so SaveClientUser fails.
	if err := os.WriteFile(filepath.Join(home, ".boru"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home) // os.UserHomeDir reads USERPROFILE on Windows

	var out, errOut bytes.Buffer
	code := Run([]string{"-r", srv.URL}, strings.NewReader("alice\npw\n"), &out, &errOut)
	if code != 1 {
		t.Fatalf("Run = %d, want 1; stderr: %s", code, errOut.String())
	}
	if !strings.Contains(errOut.String(), "saving credentials") {
		t.Errorf("stderr = %q, want saving-credentials error", errOut.String())
	}
}

func TestW4LoginSuccessPlaintext(t *testing.T) {
	srv := okLoginServer(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home) // os.UserHomeDir reads USERPROFILE on Windows

	var out, errOut bytes.Buffer
	code := New().Run([]string{"-r", srv.URL}, strings.NewReader("alice\npw\n"), &out, &errOut)
	if code != 0 {
		t.Fatalf("Run = %d, want 0; stderr: %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "logged in as alice") {
		t.Errorf("stdout = %q, want logged-in message", out.String())
	}
	data, err := os.ReadFile(filepath.Join(home, ".boru", "user.jsonic"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "token: tok123") {
		t.Errorf("user.jsonic missing token: %q", string(data))
	}
}

func TestW4LoginVaultWriteError(t *testing.T) {
	srv := okLoginServer(t)
	home := t.TempDir()
	// Block the vault folder: $HOME/.boru exists as a file, so the vault
	// write (and everything after) fails.
	if err := os.WriteFile(filepath.Join(home, ".boru"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home) // os.UserHomeDir reads USERPROFILE on Windows
	t.Setenv("BORU_HOME", "")
	t.Setenv("BORU_VAULT_FOLDER", "")
	t.Setenv("BORU_VAULT_SUFFIX", "")
	t.Setenv("BORU_VAULT_PASSPHRASE", "vpw")

	var out, errOut bytes.Buffer
	code := Run([]string{"-r", srv.URL, "--vault"}, strings.NewReader("alice\npw\n"), &out, &errOut)
	if code != 1 {
		t.Fatalf("Run = %d, want 1; stderr: %s", code, errOut.String())
	}
	if !strings.Contains(errOut.String(), "storing token in vault") {
		t.Errorf("stderr = %q, want vault-store error", errOut.String())
	}
}
