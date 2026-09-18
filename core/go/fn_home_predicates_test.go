package core

import "testing"

// The named readings of a fn value's home and name. Each predicate is the ONE
// place its sentinel is read (the sentinel gate in test/go/sentinelgate keeps
// it so): a nil Registry is a Go-built value with no home; an empty ModuleRef
// is a registry that is not a module; Name and Anonymous read together tell a
// def-bound verbose fn from a closure literal.
func TestFnValueHomePredicates(t *testing.T) {
	main, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	mod, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	mod.ModuleRef = "boru:probe"
	fork := main.ForkConcurrent()
	modFork := mod.ForkConcurrent()

	// HasHome: nil, Go-built, homed.
	var none *FnDefInfo
	if none.HasHome() || (&FnDefInfo{Name: "add"}).HasHome() {
		t.Errorf("a nil fn and a Go-built value have no home")
	}
	if !(&FnDefInfo{Registry: main}).HasHome() {
		t.Errorf("a minted fn carries its home")
	}

	// NamedDef: the four combinations of Name and Anonymous.
	cases := []struct {
		name string
		fd   *FnDefInfo
		want bool
	}{
		{"nil", nil, false},
		{"def-bound verbose fn", &FnDefInfo{Name: "f"}, true},
		{"def-bound lambda (still a closure literal)", &FnDefInfo{Name: "f", Anonymous: true}, false},
		{"nameless verbose fn (a factory's inner fn)", &FnDefInfo{}, false},
		{"bare lambda", &FnDefInfo{Anonymous: true}, false},
	}
	for _, c := range cases {
		if got := c.fd.NamedDef(); got != c.want {
			t.Errorf("NamedDef %s = %v, want %v", c.name, got, c.want)
		}
	}

	// FnHomeLookup: no home, no name, an unbound name, a bound name.
	InstallDef(main, "bound", NewFunction(FnDefInfo{Signatures: []Signature{{
		Params: []FnParam{{Name: "n", Type: TInteger}},
		Impl:   Boru([]Value{NewWord("n")}),
	}}}))
	if FnHomeLookup(nil) != nil || FnHomeLookup(&FnDefInfo{Name: "bound"}) != nil {
		t.Errorf("a nil fn and a value with no home resolve to nothing")
	}
	if FnHomeLookup(&FnDefInfo{Registry: main}) != nil {
		t.Errorf("a homed value with no name resolves to nothing")
	}
	if FnHomeLookup(&FnDefInfo{Name: "unbound", Registry: main}) != nil {
		t.Errorf("a name the home does not bind resolves to nothing")
	}
	if got := FnHomeLookup(&FnDefInfo{Name: "bound", Registry: main}); got == nil || got.Name != "bound" {
		t.Errorf("a bound name resolves in its home: %+v", got)
	}

	// IsModule: nil, main, a fork of main, a module, a fork of the module.
	var noReg *Registry
	if noReg.IsModule() || main.IsModule() || fork.IsModule() {
		t.Errorf("nil, the main program and its fork are not modules")
	}
	if !mod.IsModule() || !modFork.IsModule() {
		t.Errorf("a module and a fork of it (the id copies) are instances of the module")
	}

	// FrameOn: identity, never module — a fork of the frame's registry is the
	// wrong place to pop the frame's stacks.
	dc := DefCleanupInfo{Registry: main}
	if !dc.FrameOn(main) || dc.FrameOn(fork) || dc.FrameOn(mod) || (DefCleanupInfo{}).FrameOn(main) {
		t.Errorf("FrameOn compares by identity: main %v fork %v mod %v unset %v", dc.FrameOn(main), dc.FrameOn(fork), dc.FrameOn(mod), DefCleanupInfo{}.FrameOn(main))
	}
}

// HomeExportedFn's three hands: a Go-built export is adopted into the module
// and minted afresh, the module's own fn is minted afresh (a fresh value, no
// source position of its own), a foreign-homed re-export passes through
// untouched — and a non-fn value is reported as such, unchanged.
func TestHomeExportedFn(t *testing.T) {
	mod, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	other, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	sig := []Signature{{Params: []FnParam{{Name: "n", Type: TInteger}}, Impl: Boru([]Value{NewWord("n")})}}

	if v, isFn := HomeExportedFn(NewInteger(7), mod); isFn || !v.Parent.ConformsTo(TInteger) {
		t.Errorf("a non-fn export is not a fn and is returned as-is: %v %v", isFn, v)
	}

	// The `name/v` token an export is read through carries a source position
	// inside the module body; a fresh mint drops it (a region claim keyed by
	// it would otherwise land inside the module).
	at := SrcPos{Row: 3, Col: 5}
	read := func(fd FnDefInfo) Value {
		v := NewFunction(fd)
		v.SetPos(at)
		return v
	}
	fresh := func(v Value) bool { return v.Pos() == SrcPos{} }

	v, isFn := HomeExportedFn(read(FnDefInfo{Name: "native", Signatures: sig}), mod)
	fd, _ := v.Data.(FnDefInfo)
	if !isFn || fd.Registry != mod || !fresh(v) {
		t.Errorf("a Go-built export is adopted into the module and minted afresh: isFn %v home %p (want %p) pos %v", isFn, fd.Registry, mod, v.Pos())
	}

	v, isFn = HomeExportedFn(read(FnDefInfo{Name: "own", Registry: mod, Signatures: sig}), mod)
	fd, _ = v.Data.(FnDefInfo)
	if !isFn || fd.Registry != mod || !fresh(v) {
		t.Errorf("the module's own fn is minted afresh at home: isFn %v home %p pos %v", isFn, fd.Registry, v.Pos())
	}
	if v, _ := HomeExportedFn(read(FnDefInfo{Name: "own", Registry: mod.ForkConcurrent(), Signatures: sig}), mod); !fresh(v) {
		t.Errorf("a fn minted on a fork of the module is the module's own (compared by module, not pointer): pos %v", v.Pos())
	}

	v, isFn = HomeExportedFn(read(FnDefInfo{Name: "theirs", Registry: other, Signatures: sig}), mod)
	fd, _ = v.Data.(FnDefInfo)
	if !isFn || fd.Registry != other || fresh(v) {
		t.Errorf("a re-exported foreign fn passes through untouched: isFn %v home %p (want %p) pos %v", isFn, fd.Registry, other, v.Pos())
	}
}
