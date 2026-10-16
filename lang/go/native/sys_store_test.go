package native

import (
	"testing"

	"github.com/boru-lang/boru/lang/go/capabilities"
)

// --- __sys structure: all containers are Stores ---

func TestSysStoreStructure(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	store := r.Contexts.Top()
	if store == nil {
		t.Fatal("no context store")
	}

	// __sys exists and is a Store
	sysVal, ok := store.Get("__sys")
	if !ok {
		t.Fatal("__sys not found in root context")
	}
	sysStore, ok := sysVal.Data().(*StoreInstanceInfo)
	if !ok {
		t.Fatalf("__sys is %T, want *StoreInstanceInfo", sysVal.Data())
	}

	// __sys.fs exists and is a Store
	fsVal, ok := sysStore.Get("fs")
	if !ok {
		t.Fatal("__sys.fs not found")
	}
	fsStore, ok := fsVal.Data().(*StoreInstanceInfo)
	if !ok {
		t.Fatalf("__sys.fs is %T, want *StoreInstanceInfo", fsVal.Data())
	}

	// __sys.fs.mem = false
	memVal, ok := fsStore.Get("mem")
	if !ok {
		t.Fatal("__sys.fs.mem not found")
	}
	_as0, _ := AsBoolean(memVal)
	if _as0 != false {
		_as1, _ := AsBoolean(memVal)
		t.Errorf("__sys.fs.mem = %v, want false", _as1)
	}

	// __sys.fs.impl = None
	implVal, ok := fsStore.Get("impl")
	if !ok {
		t.Fatal("__sys.fs.impl not found")
	}
	if !IsNoneShape(implVal) {
		t.Errorf("__sys.fs.impl type = %s, want None", implVal.String())
	}

	// __sys.__val exists and is a Store
	valVal, ok := sysStore.Get("__val")
	if !ok {
		t.Fatal("__sys.__val not found")
	}
	if _, ok := valVal.Data().(*StoreInstanceInfo); !ok {
		t.Fatalf("__sys.__val is %T, want *StoreInstanceInfo", valVal.Data())
	}
}

// --- fs.mem flag switches between OS and in-memory file ops ---

func TestEffectiveFileOpsDefaultIsOS(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	ops := EffectiveFileOps(r)
	if _, ok := ops.(*capabilities.OSFileOps); !ok {
		t.Fatalf("default EffectiveFileOps is %T, want *OSFileOps", ops)
	}
}

func TestEffectiveFileOpsMemTrue(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)

	// Set __sys.fs.mem = true via boru
	e := New(r)
	_, err := e.Run([]Value{
		// Get the fs Store from __sys
		NewWord("context"), NewWord("dot"), NewWord("__sys"),
		NewWord("dot"), NewWord("fs"),
		// Set mem = true on it
		NewWord("set"), NewWord("mem"), NewBoolean(true),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ops := EffectiveFileOps(r)
	if _, ok := ops.(*capabilities.MemFileOps); !ok {
		t.Fatalf("EffectiveFileOps with mem=true is %T, want *MemFileOps", ops)
	}
}

func TestMemFileOpsReadWrite(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)

	// Enable in-memory fs
	e := New(r)
	_, err := e.Run([]Value{
		NewWord("context"), NewWord("dot"), NewWord("__sys"),
		NewWord("dot"), NewWord("fs"),
		NewWord("set"), NewWord("mem"), NewBoolean(true),
	})
	if err != nil {
		t.Fatalf("error enabling mem: %v", err)
	}

	// Write a file via the in-memory ops
	memOps := EffectiveFileOps(r)
	mem, ok := memOps.(*capabilities.MemFileOps)
	if !ok {
		t.Fatal("expected MemFileOps")
	}
	mem.Files["test.txt"] = []byte("hello")

	// Read it back through engine file ops
	data, err := EffectiveFileOps(r).ReadFile("test.txt")
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("got %q, want %q", string(data), "hello")
	}
}

func TestMemFileOpsPersistsAcrossRuns(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)

	// Enable mem fs
	e := New(r)
	_, err := e.Run([]Value{
		NewWord("context"), NewWord("dot"), NewWord("__sys"),
		NewWord("dot"), NewWord("fs"),
		NewWord("set"), NewWord("mem"), NewBoolean(true),
	})
	if err != nil {
		t.Fatal(err)
	}

	// EffectiveFileOps should return the same MemFileOps instance
	ops1 := EffectiveFileOps(r)
	ops2 := EffectiveFileOps(r)
	if ops1 != ops2 {
		t.Fatal("MemFileOps instance should be reused across calls")
	}
}

// --- __val Store ---

func TestSysValStoreSetGet(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	e := New(r)

	// Set a value in __sys.__val
	_, err := e.Run([]Value{
		NewWord("context"), NewWord("dot"), NewWord("__sys"),
		NewWord("dot"), NewWord("__val"),
		NewWord("set"), NewWord("x"), NewInteger(42),
	})
	if err != nil {
		t.Fatalf("error setting __val.x: %v", err)
	}

	// Read it back
	result, err := e.Run([]Value{
		NewWord("context"), NewWord("dot"), NewWord("__sys"),
		NewWord("dot"), NewWord("__val"),
		NewWord("dot"), NewWord("x"),
	})
	if err != nil {
		t.Fatalf("error getting __val.x: %v", err)
	}
	_as2, _ := AsInteger(result[0])
	if len(result) != 1 || _as2 != 42 {
		t.Fatalf("got %v, want [42]", result)
	}
}
