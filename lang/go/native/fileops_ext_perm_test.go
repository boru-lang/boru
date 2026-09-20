package native

import (
	"errors"
	"testing"
	"time"

	"github.com/boru-lang/boru/lang/go/capabilities"
	"github.com/boru-lang/boru/lang/go/policy"
)

// TestPermissionedFileOpsNewMethodsAllow exercises the delegate path of every
// new gated method under a policy that permits fileops.
func TestPermissionedFileOpsNewMethodsAllow(t *testing.T) {
	mem := capabilities.NewMem()
	if err := mem.WriteFile("f.txt", []byte("hi"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := mem.MkdirAll("d", 0755); err != nil {
		t.Fatal(err)
	}
	if err := mem.WriteFile("d/a.txt", []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	ops := NewPermissionedFileOps(mem, loadPolicy(t, "trusted"))

	if _, err := ops.Stat("f.txt", false); err != nil {
		t.Errorf("stat: %v", err)
	}
	if _, err := ops.ReadDir("d"); err != nil {
		t.Errorf("readdir: %v", err)
	}
	if err := ops.Rename("f.txt", "g.txt"); err != nil {
		t.Errorf("rename: %v", err)
	}
	if err := ops.Symlink("g.txt", "sl"); err != nil {
		t.Errorf("symlink: %v", err)
	}
	if err := ops.Link("g.txt", "hl"); err != nil {
		t.Errorf("link: %v", err)
	}
	if err := ops.Chmod("g.txt", 0600); err != nil {
		t.Errorf("chmod: %v", err)
	}
	if err := ops.Chtimes("g.txt", time.Unix(1, 0), time.Unix(1, 0)); err != nil {
		t.Errorf("chtimes: %v", err)
	}
	if err := ops.Truncate("g.txt", 1); err != nil {
		t.Errorf("truncate: %v", err)
	}
	if err := ops.Remove("d/a.txt", false); err != nil {
		t.Errorf("remove: %v", err)
	}
}

// TestPermissionedFileOpsNewMethodsDeny exercises the gate-compile failure branch of
// every new method under a policy that denies fileops.
func TestPermissionedFileOpsNewMethodsDeny(t *testing.T) {
	ops := NewPermissionedFileOps(capabilities.NewMem(), loadPolicy(t, "sandbox"))
	assertDenied := func(name string, err error) {
		t.Helper()
		var d *policy.Denied
		if !errors.As(err, &d) {
			t.Errorf("%s: expected *policy.Denied, got %T (%v)", name, err, err)
		}
	}
	_, sErr := ops.Stat("f.txt", false)
	assertDenied("stat", sErr)
	_, rdErr := ops.ReadDir("d")
	assertDenied("readdir", rdErr)
	assertDenied("remove", ops.Remove("f.txt", false))
	assertDenied("rename", ops.Rename("a", "b"))
	assertDenied("symlink", ops.Symlink("a", "b"))
	assertDenied("link", ops.Link("a", "b"))
	assertDenied("chmod", ops.Chmod("a", 0600))
	assertDenied("chtimes", ops.Chtimes("a", time.Unix(1, 0), time.Unix(1, 0)))
	assertDenied("truncate", ops.Truncate("a", 0))
}

// TestNotInstalledFileOpsNewMethods exercises every new denial stub.
func TestNotInstalledFileOpsNewMethods(t *testing.T) {
	var ops capabilities.FileOps = notInstalledFileOps{}
	if _, err := ops.Stat("x", false); err == nil {
		t.Error("stat stub should error")
	}
	if _, err := ops.ReadDir("x"); err == nil {
		t.Error("readdir stub should error")
	}
	if err := ops.Remove("x", false); err == nil {
		t.Error("remove stub should error")
	}
	if err := ops.Rename("x", "y"); err == nil {
		t.Error("rename stub should error")
	}
	if err := ops.Symlink("x", "y"); err == nil {
		t.Error("symlink stub should error")
	}
	if err := ops.Link("x", "y"); err == nil {
		t.Error("link stub should error")
	}
	if err := ops.Chmod("x", 0600); err == nil {
		t.Error("chmod stub should error")
	}
	if err := ops.Chtimes("x", time.Unix(1, 0), time.Unix(1, 0)); err == nil {
		t.Error("chtimes stub should error")
	}
	if err := ops.Truncate("x", 0); err == nil {
		t.Error("truncate stub should error")
	}
	if err := ops.Chown("x", -1, -1, true); err == nil {
		t.Error("chown stub should error")
	}
	if _, err := ops.XattrGet("x", "a"); err == nil {
		t.Error("xattr-get stub should error")
	}
	if err := ops.XattrSet("x", "a", nil); err == nil {
		t.Error("xattr-set stub should error")
	}
	if _, err := ops.XattrList("x"); err == nil {
		t.Error("xattr-list stub should error")
	}
	if err := ops.XattrRemove("x", "a"); err == nil {
		t.Error("xattr-remove stub should error")
	}
}
