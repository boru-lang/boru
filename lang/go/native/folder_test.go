package native

import (
	"path/filepath"
	"testing"

	"github.com/boru-lang/boru/lang/go/capabilities"
)

// helper: enable in-memory FS and return the MemFileOps
func setupMemFS(t *testing.T, r *Registry) *capabilities.MemFileOps {
	t.Helper()
	mem := capabilities.NewMem()
	if err := r.Capabilities.Set(CapMemFileOps, capabilities.FileOps(mem)); err != nil {
		t.Fatalf("set capability: %v", err)
	}
	e := New(r)
	_, err := e.Run([]Value{
		NewWord("context"), NewWord("dot"), NewWord("__sys"),
		NewWord("dot"), NewWord("fs"),
		NewWord("set"), NewWord("mem"), NewBoolean(true),
	})
	if err != nil {
		t.Fatalf("enable mem fs: %v", err)
	}
	return mem
}

// --- folder with Path ---

func TestFolderCreatesDir(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	mem := setupMemFS(t, r)

	result := runBoru(t, r, []Value{
		NewWord("folder"), NewPathon([]string{"a", "b", "c"}, false),
	})
	if len(result) != 1 || !IsPathon(result[0]) {
		t.Fatalf("expected Path result, got %v", result)
	}
	_as0, _ := AsPathon(result[0])
	if _as0.String() != "a/b/c" {
		_as1, _ := AsPathon(result[0])
		t.Errorf("got %q, want %q", _as1.String(), "a/b/c")
	}
	// Check that the directory was created in mem FS
	resolved, _ := mem.ResolvePath("a/b/c")
	if !mem.Dirs[resolved] {
		t.Errorf("directory %q not created; dirs=%v", resolved, mem.Dirs)
	}
}

func TestFolderAbsolutePath(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	mem := setupMemFS(t, r)

	result := runBoru(t, r, []Value{
		NewWord("folder"), NewPathon([]string{"tmp", "data"}, true),
	})
	if len(result) != 1 || !IsPathon(result[0]) {
		t.Fatalf("expected Path, got %v", result)
	}
	_as2, _ := AsPathon(result[0])
	if _as2.String() != "/tmp/data" {
		_as3, _ := AsPathon(result[0])
		t.Errorf("got %q, want %q", _as3.String(), "/tmp/data")
	}
	if !mem.Dirs[filepath.FromSlash("/tmp/data")] {
		t.Errorf("absolute dir not created; dirs=%v", mem.Dirs)
	}
}

func TestFolderIdempotent(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	setupMemFS(t, r)

	path := NewPathon([]string{"x", "y"}, false)
	// Create twice — should not error
	runBoru(t, r, []Value{NewWord("folder"), path})
	result := runBoru(t, r, []Value{NewWord("folder"), path})
	if len(result) != 1 || !IsPathon(result[0]) {
		t.Fatalf("idempotent call failed: got %v", result)
	}
}

// --- folder with Options ---

func TestFolderWithParentsTrue(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	mem := setupMemFS(t, r)

	opts := NewOrderedMap()
	opts.Set("parents", NewBoolean(true))
	result := runBoru(t, r, []Value{
		NewWord("folder"), NewOptionsType(opts), NewPathon([]string{"deep", "nested", "dir"}, false),
	})
	if len(result) != 1 || !IsPathon(result[0]) {
		t.Fatalf("expected Path, got %v", result)
	}
	resolved, _ := mem.ResolvePath("deep/nested/dir")
	if !mem.Dirs[resolved] {
		t.Errorf("parents dir not created; dirs=%v", mem.Dirs)
	}
}

func TestFolderWithParentsFalse(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	setupMemFS(t, r)

	opts := NewOrderedMap()
	opts.Set("parents", NewBoolean(false))
	// Even with parents=false, MkdirAll is used (idempotent single dir)
	result := runBoru(t, r, []Value{
		NewWord("folder"), NewOptionsType(opts), NewPathon([]string{"single"}, false),
	})
	if len(result) != 1 || !IsPathon(result[0]) {
		t.Fatalf("expected Path, got %v", result)
	}
}

// --- folder with make Path ---

func TestFolderWithMakePath(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	mem := setupMemFS(t, r)

	result := runBoru(t, r, []Value{
		NewWord("folder"),
		NewOpenParen(),
		NewWord("make"), NewWord("Pathon"),
		NewList([]Value{NewString("foo"), NewString("bar")}),
		NewCloseParen(),
	})
	if len(result) != 1 || !IsPathon(result[0]) {
		t.Fatalf("expected Path, got %v", result)
	}
	resolved, _ := mem.ResolvePath("foo/bar")
	if !mem.Dirs[resolved] {
		t.Errorf("dir not created via make Path; dirs=%v", mem.Dirs)
	}
}

// --- folder creates parent dirs ---

func TestFolderCreatesParentDirs(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	mem := setupMemFS(t, r)

	runBoru(t, r, []Value{
		NewWord("folder"), NewPathon([]string{"a", "b", "c"}, false),
	})
	// Parent dirs should also be recorded
	resolvedA, _ := mem.ResolvePath("a")
	resolvedAB, _ := mem.ResolvePath("a/b")
	if !mem.Dirs[resolvedA] {
		t.Errorf("parent dir 'a' not created; dirs=%v", mem.Dirs)
	}
	if !mem.Dirs[resolvedAB] {
		t.Errorf("parent dir 'a/b' not created; dirs=%v", mem.Dirs)
	}
}
