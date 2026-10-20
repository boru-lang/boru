// TestS7n_VaultProxyFactorySucceeds covers vaultProxyFactory's
// success return (factories.go). serve_wave4_test.go already covers
// the "vault not initialized" failure arm and the bad-flag arm; this
// adds the one case still missing — a properly initialized vault, so
// vault.NewProxyService succeeds and the factory reaches its final
// "return svc, nil".
//
// serve/factories.go's execFactory error arm (the "if err != nil"
// after exec.NewServer) is NOT covered here: exec.NewServer
// (cmd/go/internal/exec/service.go) unconditionally returns a nil
// error in its current implementation ("return s, nil", no other
// return path), so that branch is provably dead through every call
// site — see the final report for the block-by-block accounting.

package serve

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/boru-lang/boru/cmd/go/internal/vault"
)

func TestS7n_VaultProxyFactorySucceeds(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home) // os.UserHomeDir reads USERPROFILE on Windows
	t.Setenv(vault.EnvHome, "")
	t.Setenv(vault.EnvFolder, "")
	t.Setenv(vault.EnvSuffix, "")
	t.Setenv(vault.EnvPassphrase, "s7n-vault-pass")

	var initOut, initErr bytes.Buffer
	if code := vault.Run([]string{"init", "--backend=file"}, strings.NewReader(""), &initOut, &initErr); code != 0 {
		t.Fatalf("vault init failed: %s", initErr.String())
	}

	svc, err := vaultProxyFactory([]string{"-listen", "127.0.0.1:0", "-home", home}, nil, io.Discard, io.Discard)
	if err != nil {
		t.Fatalf("vaultProxyFactory = %v, want a service", err)
	}
	if svc == nil {
		t.Fatal("vaultProxyFactory returned a nil service with a nil error")
	}
	if svc.Name() != "vault-proxy" {
		t.Errorf("Name() = %q, want vault-proxy", svc.Name())
	}
}
