package core

import "testing"

// A registry's Home is the canonical registry of the MODULE it is an instance
// of: itself, or — for a concurrent fork — the registry it was forked from.
// FnHome and FnHomeForeign compare homes, never pointers, so a fn minted in a
// module runs on that module's fork when invoked there (the fork carries the
// connection's or service's live state) and moves to its defining registry
// only for a genuinely foreign call.
func TestRegistryHomeAndForeign(t *testing.T) {
	main, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	mod, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	fork := main.ForkConcurrent()
	fork2 := fork.ForkConcurrent()

	if main.Home() != main || mod.Home() != mod {
		t.Errorf("a module's own registry is its own home")
	}
	if fork.Home() != main || fork2.Home() != main {
		t.Errorf("a fork's home is the registry it was forked from, transitively: %p / %p want %p", fork.Home(), fork2.Home(), main)
	}
	if !main.SameHome(fork) || !fork.SameHome(fork2) || main.SameHome(mod) {
		t.Errorf("SameHome: main/fork %v fork/fork2 %v main/mod %v, want true true false", main.SameHome(fork), fork.SameHome(fork2), main.SameHome(mod))
	}
	var none *Registry
	if none.Home() != nil || !none.SameHome(nil) || none.SameHome(main) {
		t.Errorf("a nil registry is its own (nil) home and matches only nil")
	}

	mainFn := &FnDefInfo{Name: "pub", Registry: main}
	modFn := &FnDefInfo{Name: "exp", Registry: mod}
	goFn := &FnDefInfo{Name: "add"}
	cases := []struct {
		name    string
		r       *Registry
		fd      *FnDefInfo
		foreign bool
		home    *Registry
	}{
		{"nil fn", main, nil, false, main},
		{"a Go-built value has no home and is never foreign", mod, goFn, false, mod},
		{"at home", main, mainFn, false, main},
		{"on a fork of home: runs on the fork", fork2, mainFn, false, fork2},
		{"a module fn applied from main: foreign, moves home", main, modFn, true, mod},
		{"a main fn applied inside a module: foreign, moves home", mod, mainFn, true, main},
		{"a main fn applied inside a fork of a module: foreign", mod.ForkConcurrent(), mainFn, true, main},
	}
	for _, c := range cases {
		if got := FnHomeForeign(c.r, c.fd); got != c.foreign {
			t.Errorf("%s: FnHomeForeign = %v, want %v", c.name, got, c.foreign)
		}
		if got, _ := FnHome(c.r, c.fd); got != c.home {
			t.Errorf("%s: FnHome = %p, want %p", c.name, got, c.home)
		}
	}
}
