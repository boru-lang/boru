package native

import (
	"path/filepath"
	"testing"

	"github.com/boru-lang/boru/lang/go/capabilities"
)

func setupMemFSForIO(t *testing.T, r *Registry) *capabilities.MemFileOps {
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

// --- write with Path ---

func TestWriteWithPath(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	mem := setupMemFSForIO(t, r)

	path := NewPathon([]string{"data", "test.txt"}, false)
	result := runBoru(t, r, []Value{
		NewWord("write"), path, NewString("hello world"),
	})
	if len(result) != 1 || !IsPathon(result[0]) {
		t.Fatalf("expected Path result, got %v", result)
	}
	_as0, _ := AsPathon(result[0])
	if _as0.String() != "data/test.txt" {
		_as1, _ := AsPathon(result[0])
		t.Errorf("got %q, want %q", _as1.String(), "data/test.txt")
	}
	// Verify content in mem FS
	resolved, _ := mem.ResolvePath("data/test.txt")
	if string(mem.Files[resolved]) != "hello world" {
		t.Errorf("file content = %q, want %q", string(mem.Files[resolved]), "hello world")
	}
}

func TestWriteWithAbsPath(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	mem := setupMemFSForIO(t, r)

	path := NewPathon([]string{"tmp", "out.txt"}, true)
	result := runBoru(t, r, []Value{
		NewWord("write"), path, NewString("abs content"),
	})
	if len(result) != 1 || !IsPathon(result[0]) {
		t.Fatalf("expected Path result, got %v", result)
	}
	if string(mem.Files[filepath.FromSlash("/tmp/out.txt")]) != "abs content" {
		t.Errorf("file content = %q, want %q", string(mem.Files[filepath.FromSlash("/tmp/out.txt")]), "abs content")
	}
}

// --- read with Path ---

func TestReadWithPath(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	mem := setupMemFSForIO(t, r)
	mem.Files["greeting.txt"] = []byte("hello")

	path := NewPathon([]string{"greeting.txt"}, false)
	result := runBoru(t, r, []Value{
		NewWord("read"), path,
	})
	_as2, _ := AsString(result[0])
	if len(result) != 1 || _as2 != "hello" {
		t.Fatalf("got %v, want 'hello'", result)
	}
}

func TestReadWithAbsPath(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	mem := setupMemFSForIO(t, r)
	mem.Files[filepath.FromSlash("/etc/config")] = []byte("key=val")

	path := NewPathon([]string{"etc", "config"}, true)
	result := runBoru(t, r, []Value{
		NewWord("read"), path,
	})
	_as3, _ := AsString(result[0])
	if len(result) != 1 || _as3 != "key=val" {
		t.Fatalf("got %v, want 'key=val'", result)
	}
}

// --- write then read roundtrip with Path ---

func TestWriteReadRoundtripPath(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	setupMemFSForIO(t, r)

	path := NewPathon([]string{"roundtrip.txt"}, false)
	runBoru(t, r, []Value{
		NewWord("write"), path, NewString("round and round"),
	})
	result := runBoru(t, r, []Value{
		NewWord("read"), path,
	})
	_as4, _ := AsString(result[0])
	if len(result) != 1 || _as4 != "round and round" {
		t.Fatalf("got %v, want 'round and round'", result)
	}
}

// --- write with Path and options ---

func TestWriteWithPathAndOptions(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	mem := setupMemFSForIO(t, r)

	path := NewPathon([]string{"log.txt"}, false)
	opts := NewOrderedMap()
	opts.Set("mode", NewString("write"))
	result := runBoru(t, r, []Value{
		NewWord("write"), path, NewString("line1"), NewMap(opts),
	})
	if len(result) != 1 || !IsPathon(result[0]) {
		t.Fatalf("expected Path, got %v", result)
	}
	resolved, _ := mem.ResolvePath("log.txt")
	if string(mem.Files[resolved]) != "line1" {
		t.Errorf("content = %q, want %q", string(mem.Files[resolved]), "line1")
	}
}

// --- read with Path and options ---

func TestReadWithPathAndOptions(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	mem := setupMemFSForIO(t, r)
	mem.Files["data.txt"] = []byte("content here")

	path := NewPathon([]string{"data.txt"}, false)
	opts := NewOrderedMap()
	opts.Set("fmt", NewString("text"))
	result := runBoru(t, r, []Value{
		NewWord("read"), path, NewMap(opts),
	})
	_as5, _ := AsString(result[0])
	if len(result) != 1 || _as5 != "content here" {
		t.Fatalf("got %v, want 'content here'", result)
	}
}

// --- String paths are rejected: file I/O is Pathon-only ---

func TestWriteStringPathRejected(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	setupMemFSForIO(t, r)

	// A String target no longer matches any write signature — the caller
	// must supply a Pathon (make Pathon "old.txt").
	if err := runBoruError(t, r, []Value{
		NewWord("write"), NewString("old.txt"), NewString("old style"),
	}); err == nil {
		t.Error("expected a String write target to be rejected (Pathon-only)")
	}
}

func TestReadStringPathRejected(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	mem := setupMemFSForIO(t, r)
	mem.Files["compat.txt"] = []byte("compat")

	// A String target no longer matches any read signature.
	if err := runBoruError(t, r, []Value{
		NewWord("read"), NewString("compat.txt"),
	}); err == nil {
		t.Error("expected a String read target to be rejected (Pathon-only)")
	}
}
