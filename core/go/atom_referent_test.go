package core

import "testing"

// TestStampAtomReferentsLeavesProgramArrays — the run-start pass gives a
// loaded program's atoms their referents in the tape's own cells, and a
// nested atom's in a new list built for this run: the program's arrays,
// which every run of it shares, are never written, so each run's atoms name
// the binding of that run. (The pass stamped the shared nested array in
// place, and the first run's referent stuck for every later run.)
func TestStampAtomReferentsLeavesProgramArrays(t *testing.T) {
	r := newTestRegistry(t)
	inner := []Value{NewAtom("x"), NewInteger(3)}
	plain := []Value{NewInteger(4)}
	prog := []Value{NewAtom("x"), NewList(inner), NewList(plain)}
	for _, want := range []string{"1", "2"} {
		r.Defs.Push("x", NewInteger(int64(want[0]-'0')))
		out, err := NewTop(r).Run(prog)
		if err != nil {
			t.Fatalf("run %s: %v", want, err)
		}
		if len(out) != 3 {
			t.Fatalf("run %s: out = %v", want, out)
		}
		nested := out[1].Data.(ListPayload).Elems
		for i, a := range []Value{out[0], nested[0]} {
			if ref, ok := AtomReferent(a); !ok || ref.String() != want {
				t.Errorf("run %s: atom %d referent = %v (%v), want %s", want, i, ref, ok, want)
			}
		}
		// A list none of whose atoms took a referent keeps its own array.
		if got := out[2].Data.(ListPayload).Elems; &got[0] != &plain[0] {
			t.Errorf("run %s: an unstamped list was rebuilt", want)
		}
	}
	if _, has := AtomReferent(prog[0]); has {
		t.Error("the caller's program cell took a referent")
	}
	if _, has := AtomReferent(inner[0]); has {
		t.Error("the program's nested array was written")
	}
}

// TestStampAtomReferentsSkipsResolvedPrefix — the cells before StartAt are
// the run's resolved data (a frame's unnamed arguments): an atom among them
// takes its referent in its cell like any, but a list among them is a value
// the caller handed in, and is never rebuilt to stamp the atoms inside it.
func TestStampAtomReferentsSkipsResolvedPrefix(t *testing.T) {
	r := newTestRegistry(t)
	r.Defs.Push("x", NewInteger(1))
	arg := []Value{NewAtom("x")}
	e := NewTop(r)
	e.StartAt = 2
	out, err := e.Run([]Value{NewList(arg), NewAtom("x")})
	if err != nil {
		t.Fatal(err)
	}
	if got := out[0].Data.(ListPayload).Elems; &got[0] != &arg[0] {
		t.Error("a list in the resolved prefix was rebuilt")
	}
	if _, has := AtomReferent(out[1]); !has {
		t.Error("an atom in the resolved prefix took no referent")
	}
}
