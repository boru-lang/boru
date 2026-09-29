package core

import "testing"

// deftable_changed_since_test.go pins DefTable.ChangedSince, the check pass's
// const-fold purity test (NUR330): a run that bound, unbound or rebound a
// name reads changed, a run that pushed and popped back (a fn call's frame)
// reads unchanged, and a pop followed by a push reads changed unless it
// re-bound the very same value.

func TestDefTableChangedSinceNilAndInvalid(t *testing.T) {
	var nilDT *DefTable
	if nilDT.ChangedSince(nilDT.SnapshotEntries()) {
		t.Error("a nil table has no bindings to change")
	}
	dt := NewDefTable()
	dt.Push("x", NewInteger(1))
	if dt.ChangedSince(EntriesSnapshot{}) {
		t.Error("an invalid snapshot compares nothing")
	}
}

func TestDefTableChangedSinceUnchanged(t *testing.T) {
	dt := NewDefTable()
	dt.Push("x", NewInteger(1))
	dt.PushType("T", TInteger, NewInteger(2))
	snap := dt.SnapshotEntries()
	if dt.ChangedSince(snap) {
		t.Fatal("an untouched table is unchanged")
	}
	// A frame: push and pop back — the gen moved, the entries did not.
	dt.Push("x", NewInteger(9))
	dt.Push("p", NewInteger(3))
	dt.Pop("x")
	dt.Pop("p")
	if dt.ChangedSince(snap) {
		t.Error("a balanced push/pop leaves the table unchanged")
	}
}

func TestDefTableChangedSinceChanged(t *testing.T) {
	base := func() (*DefTable, EntriesSnapshot) {
		dt := NewDefTable()
		dt.Push("x", WithPosAt(NewInteger(5), SrcPos{Row: 1, Col: 7}))
		return dt, dt.SnapshotEntries()
	}
	for _, c := range []struct {
		name string
		run  func(dt *DefTable)
	}{
		{"a new name", func(dt *DefTable) { dt.Push("j", NewInteger(1)) }},
		{"a deeper binding", func(dt *DefTable) { dt.Push("x", NewInteger(1)) }},
		{"an unbound name", func(dt *DefTable) { dt.Pop("x") }},
		{"a rebind at the same depth", func(dt *DefTable) {
			dt.Pop("x")
			dt.Push("x", WithPosAt(NewInteger(1), SrcPos{Row: 1, Col: 20}))
		}},
		{"the same text from another token", func(dt *DefTable) {
			dt.Pop("x")
			dt.Push("x", WithPosAt(NewInteger(5), SrcPos{Row: 1, Col: 20}))
		}},
		{"a type half", func(dt *DefTable) {
			e, _ := dt.TopEntry("x")
			dt.Pop("x")
			dt.PushType("x", TInteger, e.Body)
		}},
	} {
		dt, snap := base()
		c.run(dt)
		if !dt.ChangedSince(snap) {
			t.Errorf("%s: must read changed", c.name)
		}
	}
	// Re-binding the very value it held is no change.
	dt, snap := base()
	e, _ := dt.TopEntry("x")
	dt.Pop("x")
	dt.Push("x", e.Body)
	if dt.ChangedSince(snap) {
		t.Error("re-binding the same value is no change")
	}
}

// Containers compare by identity, not by content: a fresh list of the same
// elements is another binding.
func TestDefTableChangedSinceContainerIdentity(t *testing.T) {
	dt := NewDefTable()
	xs := NewList([]Value{NewInteger(1)})
	dt.Push("xs", xs)
	snap := dt.SnapshotEntries()
	dt.Pop("xs")
	dt.Push("xs", xs)
	if dt.ChangedSince(snap) {
		t.Error("the same list re-bound is no change")
	}
	dt.Pop("xs")
	dt.Push("xs", NewList([]Value{NewInteger(1)}))
	if !dt.ChangedSince(snap) {
		t.Error("a fresh list of equal content is another binding")
	}
}
