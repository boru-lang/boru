package core

import "testing"

// A compiled closure is a pointer payload allocated with its datum: the
// value reads back the payload it was built from, under Function or the
// parent it was given, and is never type content.
func TestNewClosureValueCoAllocates(t *testing.T) {
	cl := ClosurePayload{Unit: 3, Render: "fn (Integer)"}
	v := NewClosureValue(cl)
	got, ok := v.Data().(*ClosurePayload)
	if !ok || got.Unit != 3 || got.Render != "fn (Integer)" || v.Parent() != TFunction {
		t.Fatalf("NewClosureValue: %#v under %v", v.Data(), v.Parent())
	}
	if got.IsTypeContent(v.Parent()) {
		t.Fatal("a closure is never type content")
	}
	under := NewClosureValueUnder(TAny, cl)
	if under.Parent() != TAny {
		t.Fatalf("NewClosureValueUnder: parent %v, want Any", under.Parent())
	}
	// Building from one payload twice gives each value its own: neither
	// sees the other's payload.
	if p, _ := under.Data().(*ClosurePayload); p == got {
		t.Fatal("two closure values share one payload")
	}
}

// ValueOf is a handle on the datum it is handed; the zero Value reads as
// the zero datum through every accessor.
func TestValueOfAndTheZeroDatum(t *testing.T) {
	d := &Datum{Parent: TInteger, Data: IntPayload{N: 7}, Origin: OriginUserDef}
	v := ValueOf(d)
	if v.d != d || v.Origin() != OriginUserDef {
		t.Fatal("ValueOf must hand back a handle on d")
	}
	var zero Value
	if zero.Data() != nil || zero.Parent() != nil || zero.IsInternal() || zero.Carrier() || zero.Origin() != 0 {
		t.Fatal("the zero Value reads as the zero datum")
	}
	if zero.In() != 0 || zero.Out() != 0 || zero.Depth() != 0 || zero.Rank() != 0 || zero.Name() != "" {
		t.Fatal("the zero Value has no labels, depth or type metadata")
	}
}

// A body write through one handle reaches that handle alone: the datum
// the others share is not written.
func TestBodyWriteIsTheHoldersAlone(t *testing.T) {
	a := NewValueRaw(TInteger, IntPayload{N: 1})
	b := a
	b.SetOrigin(OriginUserDef)
	if a.Origin() == OriginUserDef || b.Origin() != OriginUserDef || a.d == b.d {
		t.Fatal("SetOrigin must give its holder a datum of its own")
	}
	if b.Data() != a.Data() {
		t.Fatal("the new datum keeps the old one's payload")
	}
}
