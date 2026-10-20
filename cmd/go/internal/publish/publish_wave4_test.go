// Wave-4 coverage for the publish subcommand: flag and home-dir
// failures, token resolution (empty token, vault read failure,
// explicit --vault-alias), pack failure, request-building and
// transport errors, server rejection, and the Command wrapper
// methods.
package publish

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeUserFile stores a minimal logged-in user.jsonic under home.
func writeUserFile(t *testing.T, home, content string) {
	t.Helper()
	dir := filepath.Join(home, ".boru")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "user.jsonic"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

// writePublishableModule creates a module folder pack can zip.
func writePublishableModule(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "boru.jsonic"), []byte("name: w4pub\nmajor: 0\nminor: 0\npatch: 1\nfiles: [index.boru]\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "index.boru"), []byte("1 add 2"), 0644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestW4CommandMethods(t *testing.T) {
	c := New()
	if c.Name() != "publish" {
		t.Errorf("Name() = %q, want publish", c.Name())
	}
	if c.Synopsis() == "" {
		t.Error("Synopsis() is empty")
	}
}

func TestW4PublishBadFlag(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := New().Run([]string{"-zz-bad"}, strings.NewReader(""), &out, &errOut); code != 1 {
		t.Errorf("Run(bad flag) = %d, want 1", code)
	}
}

func TestW4PublishHomeDirError(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "") // os.UserHomeDir reads USERPROFILE on Windows
	var out, errOut bytes.Buffer
	if code := Run(nil, strings.NewReader(""), &out, &errOut); code != 1 {
		t.Fatalf("Run = %d, want 1", code)
	}
}

func TestW4PublishEmptyToken(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home) // os.UserHomeDir reads USERPROFILE on Windows
	writeUserFile(t, home, "username: alice\nemail: a@b.c\ntoken: \nregistry: http://localhost:9\n")

	var out, errOut bytes.Buffer
	code := Run(nil, strings.NewReader(""), &out, &errOut)
	if code != 1 {
		t.Fatalf("Run = %d, want 1", code)
	}
	if !strings.Contains(errOut.String(), "not logged in") {
		t.Errorf("stderr = %q, want not-logged-in error", errOut.String())
	}
}

func TestW4PublishVaultReadError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home) // os.UserHomeDir reads USERPROFILE on Windows
	t.Setenv("BORU_HOME", "")
	t.Setenv("BORU_VAULT_FOLDER", "")
	t.Setenv("BORU_VAULT_SUFFIX", "")
	t.Setenv("BORU_VAULT_PASSPHRASE", "vpw")
	writeUserFile(t, home, "username: alice\nemail: a@b.c\ntoken: \nregistry: http://localhost:9\ntoken_vault: zz-missing-alias\n")

	var out, errOut bytes.Buffer
	code := Run(nil, strings.NewReader(""), &out, &errOut)
	if code != 1 {
		t.Fatalf("Run = %d, want 1; stderr: %s", code, errOut.String())
	}
	if !strings.Contains(errOut.String(), "reading registry token from vault") {
		t.Errorf("stderr = %q, want vault-read error", errOut.String())
	}
}

func TestW4PublishVaultAliasFlagOverrides(t *testing.T) {
	// --vault-alias forces the vault path even when user.jsonic has a
	// plain token; a missing vault makes it fail.
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home) // os.UserHomeDir reads USERPROFILE on Windows
	t.Setenv("BORU_HOME", "")
	t.Setenv("BORU_VAULT_FOLDER", "")
	t.Setenv("BORU_VAULT_SUFFIX", "")
	t.Setenv("BORU_VAULT_PASSPHRASE", "vpw")
	writeUserFile(t, home, "username: alice\nemail: a@b.c\ntoken: plaintext\nregistry: http://localhost:9\n")

	var out, errOut bytes.Buffer
	code := Run([]string{"--vault-alias", "zz-flag-alias"}, strings.NewReader(""), &out, &errOut)
	if code != 1 {
		t.Fatalf("Run = %d, want 1; stderr: %s", code, errOut.String())
	}
	if !strings.Contains(errOut.String(), "zz-flag-alias") {
		t.Errorf("stderr = %q, want the flag alias in the error", errOut.String())
	}
}

func TestW4PublishVaultFlagDefaultAlias(t *testing.T) {
	// --vault with no stored alias falls back to the default alias.
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home) // os.UserHomeDir reads USERPROFILE on Windows
	t.Setenv("BORU_HOME", "")
	t.Setenv("BORU_VAULT_FOLDER", "")
	t.Setenv("BORU_VAULT_SUFFIX", "")
	t.Setenv("BORU_VAULT_PASSPHRASE", "vpw")
	writeUserFile(t, home, "username: alice\nemail: a@b.c\ntoken: plaintext\nregistry: http://localhost:9\n")

	var out, errOut bytes.Buffer
	code := Run([]string{"--vault"}, strings.NewReader(""), &out, &errOut)
	if code != 1 {
		t.Fatalf("Run = %d, want 1; stderr: %s", code, errOut.String())
	}
	if !strings.Contains(errOut.String(), "boru-registry-token") {
		t.Errorf("stderr = %q, want the default alias in the error", errOut.String())
	}
}

func TestW4PublishPackFails(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home) // os.UserHomeDir reads USERPROFILE on Windows
	writeUserFile(t, home, "username: alice\nemail: a@b.c\ntoken: tok\nregistry: http://localhost:9\n")

	empty := t.TempDir() // no boru.jsonic → pack fails
	var out, errOut bytes.Buffer
	code := Run([]string{empty}, strings.NewReader(""), &out, &errOut)
	if code != 1 {
		t.Fatalf("Run = %d, want 1", code)
	}
	if !strings.Contains(errOut.String(), "pack failed") {
		t.Errorf("stderr = %q, want pack-failed error", errOut.String())
	}
}

func TestW4PublishBadRegistryURL(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home) // os.UserHomeDir reads USERPROFILE on Windows
	writeUserFile(t, home, "username: alice\nemail: a@b.c\ntoken: tok\nregistry: \n")

	dir := writePublishableModule(t)
	var out, errOut bytes.Buffer
	// A control character makes http.NewRequest reject the URL.
	code := Run([]string{"-r", "http://bad url with spaces\x7f", dir}, strings.NewReader(""), &out, &errOut)
	if code != 1 {
		t.Fatalf("Run = %d, want 1; stderr: %s", code, errOut.String())
	}
}

func TestW4PublishConnectionError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	// No registry in user.jsonic and no -r flag → default localhost:8080
	t.Setenv("USERPROFILE", home) // os.UserHomeDir reads USERPROFILE on Windows
	// fallback; then connection to a dead port fails.
	writeUserFile(t, home, "username: alice\nemail: a@b.c\ntoken: tok\nregistry: \n")

	dir := writePublishableModule(t)
	var out, errOut bytes.Buffer
	code := Run([]string{"-r", "http://127.0.0.1:1", dir}, strings.NewReader(""), &out, &errOut)
	if code != 1 {
		t.Fatalf("Run = %d, want 1; stderr: %s", code, errOut.String())
	}
}

func TestW4PublishDefaultRegistryFallback(t *testing.T) {
	// Neither -r nor a stored registry: the hardcoded default is used
	// (and fails to connect, which is fine — the arm is the point).
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home) // os.UserHomeDir reads USERPROFILE on Windows
	writeUserFile(t, home, "username: alice\nemail: a@b.c\ntoken: tok\n")

	dir := writePublishableModule(t)
	var out, errOut bytes.Buffer
	code := Run([]string{dir}, strings.NewReader(""), &out, &errOut)
	if code != 1 {
		t.Fatalf("Run = %d, want 1 (default registry unreachable)", code)
	}
}

func TestW4PublishServerRejects(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad module", http.StatusBadRequest)
	}))
	t.Cleanup(srv.Close)

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home) // os.UserHomeDir reads USERPROFILE on Windows
	writeUserFile(t, home, "username: alice\nemail: a@b.c\ntoken: tok\nregistry: "+srv.URL+"\n")

	dir := writePublishableModule(t)
	var out, errOut bytes.Buffer
	code := Run([]string{dir}, strings.NewReader(""), &out, &errOut)
	if code != 1 {
		t.Fatalf("Run = %d, want 1; stderr: %s", code, errOut.String())
	}
	if !strings.Contains(errOut.String(), "bad module") {
		t.Errorf("stderr = %q, want server rejection", errOut.String())
	}
}

func TestW4PublishSuccessViaStoredRegistry(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"module":"w4pub","version":"0.0.1"}`))
	}))
	t.Cleanup(srv.Close)

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home) // os.UserHomeDir reads USERPROFILE on Windows
	writeUserFile(t, home, "username: alice\nemail: a@b.c\ntoken: tok\nregistry: "+srv.URL+"\n")

	dir := writePublishableModule(t)
	var out, errOut bytes.Buffer
	code := New().Run([]string{dir}, strings.NewReader(""), &out, &errOut)
	if code != 0 {
		t.Fatalf("Run = %d, want 0; stderr: %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "published w4pub@0.0.1") {
		t.Errorf("stdout = %q", out.String())
	}
	if gotAuth != "Bearer tok" {
		t.Errorf("Authorization = %q, want Bearer tok", gotAuth)
	}
}
