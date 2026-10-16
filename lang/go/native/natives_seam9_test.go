package native

import (
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/boru-lang/boru/lang/go/capabilities"
)

// w9ExtStub supplies erroring implementations of the extended FileOps surface
// (Stat/ReadDir/Remove/Rename/Symlink/Link/Chmod/Chtimes/Truncate) so the
// minimal folder-focused mocks below satisfy capabilities.FileOps. These
// methods are not exercised by the doFolder tests that use these mocks.
type w9ExtStub struct{}

func (w9ExtStub) Stat(string, bool) (capabilities.FileInfo, error) {
	return capabilities.FileInfo{}, fmt.Errorf("no")
}
func (w9ExtStub) ReadDir(string) ([]capabilities.FileInfo, error) { return nil, fmt.Errorf("no") }
func (w9ExtStub) Remove(string, bool) error                       { return fmt.Errorf("no") }
func (w9ExtStub) Rename(string, string) error                     { return fmt.Errorf("no") }
func (w9ExtStub) Symlink(string, string) error                    { return fmt.Errorf("no") }
func (w9ExtStub) Link(string, string) error                       { return fmt.Errorf("no") }
func (w9ExtStub) Chmod(string, os.FileMode) error                 { return fmt.Errorf("no") }
func (w9ExtStub) Chtimes(string, time.Time, time.Time) error      { return fmt.Errorf("no") }
func (w9ExtStub) Truncate(string, int64) error                    { return fmt.Errorf("no") }
func (w9ExtStub) Chown(string, int, int, bool) error              { return fmt.Errorf("no") }
func (w9ExtStub) XattrGet(string, string) ([]byte, error)         { return nil, fmt.Errorf("no") }
func (w9ExtStub) XattrSet(string, string, []byte) error           { return fmt.Errorf("no") }
func (w9ExtStub) XattrList(string) ([]string, error)              { return nil, fmt.Errorf("no") }
func (w9ExtStub) XattrRemove(string, string) error                { return fmt.Errorf("no") }
func (w9ExtStub) TempFile(string, string) (string, error)         { return "", fmt.Errorf("no") }
func (w9ExtStub) TempDir(string, string) (string, error)          { return "", fmt.Errorf("no") }
func (w9ExtStub) Statfs(string) (capabilities.FsInfo, error) {
	return capabilities.FsInfo{}, fmt.Errorf("no")
}
func (w9ExtStub) Watch(string, capabilities.WatchOpts) (<-chan capabilities.WatchEvent, func() error, error) {
	return nil, nil, fmt.Errorf("no")
}
func (w9ExtStub) Open(string, capabilities.OpenOpts) (capabilities.FileHandle, error) {
	return nil, fmt.Errorf("no")
}
func (w9ExtStub) Lock(string, bool, bool) (io.Closer, error) { return nil, fmt.Errorf("no") }
func (w9ExtStub) Mmap(string, int64, int, bool) (capabilities.MmapRegion, error) {
	return nil, fmt.Errorf("no")
}

// Seam-9 coverage (W9_nativeC) for natives.go: reach / folder / cancel
// handler error and edge arms, driven directly with crafted args and an
// FS-hermetic (in-memory / failing) FileOps.

// w9FailResolveOps is a FileOps whose ResolvePath fails, so the
// parents=false ResolvePath error arm in doFolder is reachable.
type w9FailResolveOps struct{ w9ExtStub }

func (w9FailResolveOps) ReadFile(string) ([]byte, error) { return nil, fmt.Errorf("no") }
func (w9FailResolveOps) WriteFile(string, []byte, os.FileMode) error {
	return fmt.Errorf("no")
}
func (w9FailResolveOps) MkdirAll(string, os.FileMode) error { return fmt.Errorf("no") }
func (w9FailResolveOps) ResolvePath(string) (string, error) {
	return "", fmt.Errorf("resolve boom")
}

// w9FailMkdirOps is a hermetic FileOps whose ResolvePath succeeds but
// MkdirAll fails — exactly notInstalled's shape, injected so the doFolder
// MkdirAll error arms are reachable without touching the real FS.
type w9FailMkdirOps struct{ w9ExtStub }

func (w9FailMkdirOps) ReadFile(string) ([]byte, error)             { return nil, fmt.Errorf("no") }
func (w9FailMkdirOps) WriteFile(string, []byte, os.FileMode) error { return fmt.Errorf("no") }
func (w9FailMkdirOps) MkdirAll(string, os.FileMode) error          { return fmt.Errorf("mkdir boom") }
func (w9FailMkdirOps) ResolvePath(p string) (string, error)        { return p, nil }

func w9SetFileOps(t *testing.T, r *Registry, ops capabilities.FileOps) {
	t.Helper()
	if err := r.Capabilities.Set(CapFileOps, ops); err != nil {
		t.Fatalf("set CapFileOps: %v", err)
	}
}

func TestW9CaptureAtomReferentAlreadyBound(t *testing.T) {
	r := seam5Reg(t)
	// An atom that already carries a referent short-circuits: a binding of
	// its name made since does not replace it.
	atom := NewAtom("x")
	five := NewInteger(5)
	SetAtomReferent(&atom, &five)
	r.Defs.Push("x", NewInteger(9))
	captureAtomReferent(&atom, r)
	if ref, has := AtomReferent(atom); !has || ref.String() != "5" {
		t.Fatalf("captureAtomReferent must preserve existing referent, got %v (%v)", ref, has)
	}
}

func TestW9ReachHandlerBadKeyList(t *testing.T) {
	r := seam5Reg(t)
	// args[1] a type literal (not a concrete list) → RequireConcreteList
	// error propagates (477.16,479.3).
	if _, err := reachHandler([]Value{NewInteger(1), NewTypeLiteral(TList)}, nil, nil, r); err == nil {
		t.Fatalf("reachHandler: expected key-list error")
	}
}

func TestW9FolderHandlersWrongPathType(t *testing.T) {
	r := seam5Reg(t)
	// folderHandler: non-Pathon path (554.24,556.3).
	if _, err := folderHandler([]Value{NewInteger(1)}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "expected Path") {
		t.Fatalf("folderHandler wrong type: %v", err)
	}
	// folderOptsHandler: non-Pathon path (538.24,540.3).
	optsMap := NewOrderedMap()
	if _, err := folderOptsHandler([]Value{NewMap(optsMap), NewInteger(1)}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "expected Path") {
		t.Fatalf("folderOptsHandler wrong type: %v", err)
	}
}

func TestW9DoFolderMkdirAllError(t *testing.T) {
	// A hermetic FileOps whose MkdirAll errors drives the parents=true
	// error arm (568.53,570.4) without touching the real FS.
	r := seam5Reg(t)
	w9SetFileOps(t, r, w9FailMkdirOps{})
	out, err := folderHandler([]Value{NewPathon([]string{"a", "b"}, false)}, nil, nil, r)
	if err != nil || len(out) != 1 || !IsError(out[0]) {
		t.Fatalf("folderHandler mkdir-fail: expected Error value, got %v / %v", out, err)
	}
}

func TestW9FolderOptsPlainMapParentsFalse(t *testing.T) {
	// A plain concrete Map {parents:false} exercises the opts-map
	// extraction (542.50,543.75 / 543.75,545.4) — existing tests pass an
	// Options value, whose AsMap returns nil. parents=false then takes the
	// else branch of doFolder; the hermetic MkdirAll errors (571 / 576).
	r := seam5Reg(t)
	w9SetFileOps(t, r, w9FailMkdirOps{})
	om := NewOrderedMap()
	om.Set("parents", NewBoolean(false))
	out, err := folderOptsHandler([]Value{NewMap(om), NewPathon([]string{"z"}, false)}, nil, nil, r)
	if err != nil || len(out) != 1 || !IsError(out[0]) {
		t.Fatalf("folderOpts parents=false no-FS: expected Error value, got %v / %v", out, err)
	}
}

func TestW9DoFolderResolvePathError(t *testing.T) {
	// parents=false with a FileOps whose ResolvePath fails (573.17,575.4).
	r := seam5Reg(t)
	if err := r.Capabilities.Set(CapFileOps, capabilities.FileOps(w9FailResolveOps{})); err != nil {
		t.Fatalf("set CapFileOps: %v", err)
	}
	om := NewOrderedMap()
	om.Set("parents", NewBoolean(false))
	out, err := folderOptsHandler([]Value{NewMap(om), NewPathon([]string{"z"}, false)}, nil, nil, r)
	if err != nil || len(out) != 1 || !IsError(out[0]) {
		t.Fatalf("folderOpts ResolvePath-fail: expected Error value, got %v / %v", out, err)
	}
}

func TestW9CancelHandlersWrongType(t *testing.T) {
	r := seam5Reg(t)
	// cancelTimeoutHandler: non-Timeout (678.9,680.3).
	if _, err := cancelTimeoutHandler([]Value{NewInteger(1)}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "not a Timeout") {
		t.Fatalf("cancelTimeoutHandler wrong type: %v", err)
	}
	// cancelIntervalofHandler: non-Interval (692.9,694.3).
	if _, err := cancelIntervalofHandler([]Value{NewInteger(1)}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "not an Interval") {
		t.Fatalf("cancelIntervalofHandler wrong type: %v", err)
	}
}
