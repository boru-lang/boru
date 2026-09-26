package core

import "testing"

// TestNUR076BehaveMakerIsVisibleToCheck pins the core half of NUR076's close:
// a `behave make` call the check pass reaches is noted in the pass's own
// state (CheckState.NoteBehaveMaker) — nothing is installed on the type — and
// HasMaker reads the note, so `make` skips the schema validation of a type
// whose own constructor builds it. Outside a pass the note is a no-op (the
// run installs the real capability), and a fresh pass starts with none.
func TestNUR076BehaveMakerIsVisibleToCheck(t *testing.T) {
	var none *CheckState
	none.NoteBehaveMaker(TInteger) // a nil state is inert
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	p := r.Types.MintType("Nur076P", TMap)
	lit := NewTypeLiteral(p)
	r.Check.NoteBehaveMaker(p)
	if HasMaker(r, lit) {
		t.Fatal("outside a check pass the note is a no-op")
	}
	done := r.Check.Begin()
	r.Check.NoteBehaveMaker(nil) // no type, no note
	if HasMaker(r, lit) {
		t.Fatal("no maker before the behave call is reached")
	}
	r.Check.NoteBehaveMaker(p)
	if !HasMaker(r, lit) {
		t.Fatal("a behave make the pass reached gives the type a constructor")
	}
	if _, installed := p.Behavior().(Maker); installed {
		t.Fatal("the note installs nothing on the type")
	}
	done()
	done2 := r.Check.Begin()
	defer done2()
	if HasMaker(r, lit) {
		t.Fatal("a fresh pass starts with no noted makers")
	}
}
