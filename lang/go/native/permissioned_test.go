package native

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/boru-lang/boru/lang/go/capabilities"
	"github.com/boru-lang/boru/lang/go/policy"
)

func loadPolicy(t *testing.T, name string) policy.Policy {
	t.Helper()
	p, err := policy.Load(name)
	if err != nil {
		t.Fatalf("policy.Load(%q): %s", name, err)
	}
	return p
}

func TestSetHostFileOpsWrapsWhenPolicySet(t *testing.T) {
	r, err := DefaultRegistryWithPolicy(loadPolicy(t, "trusted"))
	if err != nil {
		t.Fatal(err)
	}
	ops := HostFileOps(r)
	if ops == nil {
		t.Fatal("expected HostFileOps to be installed")
	}
	// Wrapped ops should still delegate transparently for trusted.
	// Touch the underlying type to confirm it's the wrapper.
	if _, ok := ops.(*permissionedFileOps); !ok {
		t.Errorf("HostFileOps = %T, want *permissionedFileOps", ops)
	}
}

func TestSetHostFileOpsDoesNotWrapWithNoPolicy(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	ops := HostFileOps(r)
	if ops == nil {
		t.Fatal("expected HostFileOps to be installed")
	}
	if _, ok := ops.(*permissionedFileOps); ok {
		t.Error("no policy → should not be wrapped")
	}
}

func TestPermissionedFileOpsDeniesDiskWrite(t *testing.T) {
	r, err := DefaultRegistryWithPolicy(loadPolicy(t, "sandbox"))
	if err != nil {
		t.Fatal(err)
	}
	ops := HostFileOps(r)
	if ops == nil {
		t.Fatal("sandbox should keep FileOps installed")
	}
	err = ops.WriteFile("/tmp/foo", []byte("hi"), 0644)
	var d *policy.Denied
	if !errors.As(err, &d) {
		t.Fatalf("expected *policy.Denied, got %T (%v)", err, err)
	}
	if !strings.Contains(d.Blame, "disk.write") {
		t.Errorf("Blame = %q, expected to mention disk.write", d.Blame)
	}
}

func TestPermissionedFileOpsDeniesReadByDefault(t *testing.T) {
	// sandbox has fileops.default=deny, so reads also fail (even
	// though global.disk.read is allowed).
	r, err := DefaultRegistryWithPolicy(loadPolicy(t, "sandbox"))
	if err != nil {
		t.Fatal(err)
	}
	ops := HostFileOps(r)
	_, err = ops.ReadFile("/tmp/foo")
	var d *policy.Denied
	if !errors.As(err, &d) {
		t.Fatalf("expected *policy.Denied, got %T (%v)", err, err)
	}
	if !strings.Contains(d.Blame, "fileops.words") {
		t.Errorf("Blame = %q, expected to mention fileops.words", d.Blame)
	}
}

func TestComputeUninstallsCapabilities(t *testing.T) {
	r, err := DefaultRegistryWithPolicy(loadPolicy(t, "compute"))
	if err != nil {
		t.Fatal(err)
	}
	// Uninstalled fileops returns the notInstalled stub (not nil)
	// so consumer sites that don't nil-check don't crash. Calling
	// the stub's methods produces a capability_not_installed error.
	ops := HostFileOps(r)
	if _, ok := ops.(notInstalledFileOps); !ok {
		t.Errorf("compute should leave HostFileOps as notInstalledFileOps; got %T", ops)
	}
	if err := ops.WriteFile("/tmp/x", []byte("hi"), 0644); err == nil {
		t.Error("stub WriteFile should return capability_not_installed")
	}
	if HostSQLite(r) != nil {
		t.Error("compute should uninstall sqlite")
	}
	if HostFormats(r) == nil {
		t.Error("compute keeps formats installed")
	}
}

func TestPermissionedFileOpsResolvePathBypasses(t *testing.T) {
	// ResolvePath is pure manipulation — no policy gate.
	r, err := DefaultRegistryWithPolicy(loadPolicy(t, "sandbox"))
	if err != nil {
		t.Fatal(err)
	}
	ops := HostFileOps(r)
	if _, err := ops.ResolvePath("/tmp/x"); err != nil {
		t.Errorf("ResolvePath should not be gated: %v", err)
	}
}

func TestPermissionedFileOpsGatesWatch(t *testing.T) {
	// The watch gate follows the read-like pattern: the sandbox policy's
	// fileops.default=deny declines the subscription outright, and the
	// denial arrives before any watcher resources are allocated.
	r, err := DefaultRegistryWithPolicy(loadPolicy(t, "sandbox"))
	if err != nil {
		t.Fatal(err)
	}
	ops := HostFileOps(r)
	_, _, err = ops.Watch("/tmp/foo", capabilities.WatchOpts{})
	var d *policy.Denied
	if !errors.As(err, &d) {
		t.Fatalf("expected *policy.Denied, got %T (%v)", err, err)
	}
	if !strings.Contains(d.Blame, "fileops.words") {
		t.Errorf("Blame = %q, expected to mention fileops.words", d.Blame)
	}
}

func TestPermissionedFileOpsAllowsWatchThrough(t *testing.T) {
	// With no policy the wrapper is absent; with an allow-all policy the
	// gate passes and the inner backend's stream comes through.
	mem := capabilities.NewMem()
	if err := mem.MkdirAll("d", 0o755); err != nil {
		t.Fatal(err)
	}
	wrapped := NewPermissionedFileOps(mem, loadPolicy(t, "trusted"))
	ch, stop, err := wrapped.Watch("d", capabilities.WatchOpts{})
	if err != nil {
		t.Fatalf("allow-all watch: %v", err)
	}
	if err := mem.WriteFile("d/x.txt", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	select {
	case ev := <-ch:
		if ev.Op != "create" {
			t.Errorf("gated stream event = %+v", ev)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no event through the permissioned wrapper")
	}
	if err := stop(); err != nil {
		t.Fatal(err)
	}
}
