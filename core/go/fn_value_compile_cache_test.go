package core

import "testing"

// compiledFnDefFor computes a fn value's dispatch form once per identity
// and registry; every copy of the value shares the token, so a method read
// off an instance or a callback applied per element compiles once. These
// pins cover the cache's arms: the hit, the misses (another registry, a
// rebuilt signature list, another name), and a payload with no identity.

func cacheFnValue(t *testing.T, name string, captured ...CapturedBinding) FnDefInfo {
	t.Helper()
	v := NewFunction(FnDefInfo{
		Name:      name,
		Anonymous: name == "",
		Captured:  captured,
		Signatures: []Signature{{
			Params:     []FnParam{{Name: "x", Type: TInteger}},
			Returns:    []*Type{TInteger},
			BarrierPos: BarrierAllForward,
			Impl:       Boru([]Value{NewWord("x")}),
		}},
	})
	fd, ok := v.Data.(FnDefInfo)
	if !ok || fd.ident == nil {
		t.Fatal("NewFunction must mint an identity token")
	}
	return fd
}

func TestCompiledFnDefForCachesPerIdentityAndRegistry(t *testing.T) {
	r := newTestRegistry(t)
	fd := cacheFnValue(t, "", CapturedBinding{Name: "k", Value: NewInteger(1)})

	first := compiledFnDefFor(r, fd)
	if first == nil || len(first.Signatures) != 1 || first.Signatures[0].DispatchHandler() == nil {
		t.Fatalf("compiled form = %+v, want one sig with the body-runner attached", first)
	}
	// A COPY of the value (the same token, the same lists) hits.
	copyOf := fd
	if again := compiledFnDefFor(r, copyOf); again != first {
		t.Error("a second dispatch of the same value must reuse the compiled form")
	}
	// Another registry compiles its own form (the body-runner closes over
	// the registry) and takes the cache over.
	r2 := newTestRegistry(t)
	other := compiledFnDefFor(r2, fd)
	if other == first {
		t.Error("a different registry must not share the compiled form")
	}
	if compiledFnDefFor(r2, fd) != other {
		t.Error("the other registry's form must be cached in turn")
	}
	if compiledFnDefFor(r, fd) == other {
		t.Error("the first registry must recompile after the cache moved registries")
	}
	// A REBUILT signature list under the same token (a looked-up name's
	// aggregate regenerates its slice) recompiles.
	rebuilt := fd
	rebuilt.Signatures = append([]Signature(nil), fd.Signatures...)
	cur := compiledFnDefFor(r, fd)
	if compiledFnDefFor(r, rebuilt) == cur {
		t.Error("a rebuilt signature list must recompile")
	}
	// The same token under another NAME compiles under that name: the
	// frame's diagnostics carry it.
	renamed := fd
	renamed.Name = "alias"
	cur = compiledFnDefFor(r, fd)
	if named := compiledFnDefFor(r, renamed); named == cur || named.Name != "alias" {
		t.Errorf("a renamed copy must compile under its own name, got %q (same form: %v)", named.Name, named == cur)
	}
}

func TestCompiledFnDefForWithoutIdentityCompilesEveryTime(t *testing.T) {
	r := newTestRegistry(t)
	// A synthesized payload (no NewFunction, no token, no captures).
	fd := FnDefInfo{
		Anonymous: true,
		Signatures: []Signature{{
			Params:     []FnParam{{Name: "x", Type: TInteger}},
			Returns:    []*Type{TInteger},
			BarrierPos: BarrierAllForward,
			Impl:       Boru([]Value{NewWord("x")}),
		}},
	}
	a := compiledFnDefFor(r, fd)
	b := compiledFnDefFor(r, fd)
	if a == nil || b == nil || a == b {
		t.Error("a payload with no identity has nowhere to cache and compiles each time")
	}
}
