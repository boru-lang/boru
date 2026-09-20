package native

import (
	"errors"
	"strings"
	"testing"

	"github.com/boru-lang/boru/lang/go/policy"
)

func TestFetchPolicyAllowsWithFull(t *testing.T) {
	r, err := DefaultRegistryWithPolicy(loadPolicy(t, "full"))
	if err != nil {
		t.Fatal(err)
	}
	if err := checkFetchPolicy(r, "https://example.com/foo"); err != nil {
		t.Errorf("full should permit fetch: %v", err)
	}
}

func TestFetchPolicyDeniedBySandbox(t *testing.T) {
	// sandbox sets network: install:false, plus global.network deny.
	r, err := DefaultRegistryWithPolicy(loadPolicy(t, "sandbox"))
	if err != nil {
		t.Fatal(err)
	}
	err = checkFetchPolicy(r, "https://example.com/foo")
	if err == nil {
		t.Fatal("expected sandbox to deny fetch")
	}
	// The gate CODES the compile failure (policy_error.go): it returns a BoruError
	// carrying the code, not the raw *policy.Denied, so a program can reach it
	// through `do [...] error [dot code]`. Asserting the code is asserting what
	// a user can actually observe; the *Denied is an internal shape.
	var ae *BoruError
	if !errors.As(err, &ae) {
		t.Fatalf("expected a coded BoruError, got %T (%v)", err, err)
	}
	// sandbox uninstalls network → capability_not_installed code.
	if ae.Code != "capability_not_installed" {
		t.Errorf("Code = %q, want capability_not_installed", ae.Code)
	}
}

func TestFetchPolicyDeniedByGlobalCap(t *testing.T) {
	// Custom policy: network is installed, but global.network is denied.
	pol, err := policy.LoadInline(`{
		name: "no-net-global"
		scopes: {
			global: {
				words: { default: "allow", rules: [{ deny: ["network"] }] }
			}
			network: { words: { default: "allow" } }
		}
	}`)
	if err != nil {
		t.Fatal(err)
	}
	r, err := DefaultRegistryWithPolicy(pol)
	if err != nil {
		t.Fatal(err)
	}
	err = checkFetchPolicy(r, "https://example.com/foo")
	if err == nil {
		t.Fatal("expected global cap to deny fetch")
	}
	if !strings.Contains(err.Error(), "network") {
		t.Errorf("expected 'network' in error, got: %v", err)
	}
}

func TestFetchPolicyHostAllowlist(t *testing.T) {
	pol, err := policy.LoadInline(`{
		name: "api-only"
		scopes: {
			network: {
				words: {
					default: "deny"
					rules: [
						{ allow: ["connect"], where: { host: ["api.example.com"] } }
					]
				}
			}
		}
	}`)
	if err != nil {
		t.Fatal(err)
	}
	r, err := DefaultRegistryWithPolicy(pol)
	if err != nil {
		t.Fatal(err)
	}
	if err := checkFetchPolicy(r, "https://api.example.com/x"); err != nil {
		t.Errorf("api.example.com should be allowed: %v", err)
	}
	if err := checkFetchPolicy(r, "https://evil.example.com/x"); err == nil {
		t.Error("evil.example.com should be denied")
	}
}

func TestFetchPolicyNoPolicyAllowsAll(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	if err := checkFetchPolicy(r, "https://anywhere.example/x"); err != nil {
		t.Errorf("no policy should allow: %v", err)
	}
}

func TestHostPortFromURL(t *testing.T) {
	tests := []struct {
		url  string
		host string
		port int
	}{
		{"https://example.com/path", "example.com", 443},
		{"http://example.com/path", "example.com", 80},
		{"https://example.com:8443/path", "example.com", 8443},
		{"http://localhost:3000", "localhost", 3000},
		{"ws://example.com/", "example.com", 80},
		{"wss://example.com/", "example.com", 443},
	}
	for _, tt := range tests {
		gotHost, gotPort := hostPortFromURL(tt.url)
		if gotHost != tt.host || gotPort != tt.port {
			t.Errorf("hostPortFromURL(%q) = (%q, %d), want (%q, %d)",
				tt.url, gotHost, gotPort, tt.host, tt.port)
		}
	}
}
