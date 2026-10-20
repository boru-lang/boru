package login

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/boru-lang/boru/cmd/go/internal/auth"
	"github.com/boru-lang/boru/cmd/go/internal/registry"
	"github.com/boru-lang/boru/cmd/go/internal/vault"
)

func TestRunLoginCLI(t *testing.T) {
	regDir := t.TempDir()
	srv := httptest.NewServer(registry.Handler(regDir))
	defer srv.Close()

	regBody := `{"email":"cli@example.com","username":"cliuser","password":"clipass"}`
	resp, _ := http.Post(srv.URL+"/api/register", "application/json", strings.NewReader(regBody))
	resp.Body.Close()

	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir) // os.UserHomeDir reads USERPROFILE on Windows

	stdin := strings.NewReader("cliuser\nclipass\n")
	var stdout, stderr bytes.Buffer

	code := Run([]string{"-r", srv.URL}, stdin, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run failed: %s", stderr.String())
	}
	if !strings.Contains(stdout.String(), "logged in as cliuser") {
		t.Errorf("unexpected output: %q", stdout.String())
	}

	cu, err := auth.LoadClientUser(homeDir)
	if err != nil {
		t.Fatal(err)
	}
	if cu.Username != "cliuser" {
		t.Errorf("username = %q, want cliuser", cu.Username)
	}
	if cu.Token == "" {
		t.Error("token is empty")
	}
}

// TestRunLoginVault: `boru login --vault` stores the token in the vault
// and leaves user.jsonic with a reference (token_vault), not a plaintext
// token.
func TestRunLoginVault(t *testing.T) {
	regDir := t.TempDir()
	srv := httptest.NewServer(registry.Handler(regDir))
	defer srv.Close()

	rresp, _ := http.Post(srv.URL+"/api/register", "application/json",
		strings.NewReader(`{"email":"v@example.com","username":"vuser","password":"vpass"}`))
	rresp.Body.Close()

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home) // os.UserHomeDir reads USERPROFILE on Windows
	t.Setenv("BORU_HOME", "")
	t.Setenv("BORU_VAULT_FOLDER", "")
	t.Setenv("BORU_VAULT_SUFFIX", "")
	t.Setenv("BORU_VAULT_PASSPHRASE", "vpw")

	if code := vault.Run([]string{"init", "--backend=file"}, strings.NewReader(""), io.Discard, io.Discard); code != 0 {
		t.Fatal("vault init failed")
	}

	stdin := strings.NewReader("vuser\nvpass\n")
	var stdout, stderr bytes.Buffer
	code := Run([]string{"-r", srv.URL, "--vault"}, stdin, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("login --vault failed: %s", stderr.String())
	}
	if !strings.Contains(stdout.String(), "stored in vault") {
		t.Errorf("unexpected output: %q", stdout.String())
	}

	cu, err := auth.LoadClientUser(home)
	if err != nil {
		t.Fatal(err)
	}
	if cu.Token != "" {
		t.Errorf("token should NOT be stored in plaintext, got %q", cu.Token)
	}
	if cu.TokenVault != auth.DefaultRegistryTokenAlias {
		t.Errorf("token_vault = %q, want %q", cu.TokenVault, auth.DefaultRegistryTokenAlias)
	}
	// The token is retrievable from the vault.
	tok, err := vault.ReadSecret(home, auth.DefaultRegistryTokenAlias, strings.NewReader(""), io.Discard)
	if err != nil || tok == "" {
		t.Errorf("token not in vault: %q / %v", tok, err)
	}
}
