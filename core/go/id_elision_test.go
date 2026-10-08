package core

// Tests for the mode-gated Value-ID elision (design/
// INTERPRETER-PYTHON-PARITY.10.md Phase B / F2): runtime concrete mints
// carry no ID, any live check/compile pass (or an explicit mint scope)
// re-arms minting process-wide, and the emit layer treats an empty ID as
// "no identity" — skip / decline / rescue, never a map key.

import "testing"

func TestIDElisionMatrix(t *testing.T) {
	// Runtime concrete mint: elided.
	if CheckPassActive() {
		t.Fatal("no pass should be live at test entry")
	}
	if v := NewInteger(7); v.ID != "" {
		t.Errorf("runtime concrete mint carries ID %q, want elided", v.ID)
	}
	// Bare lattice values always mint: type literal and carrier.
	if v := NewTypeLiteral(TInteger); v.ID == "" {
		t.Error("type literal must always carry an ID")
	}
	if v := NewCarrier(TList); v.ID == "" {
		t.Error("carrier must always carry an ID (RegisterLocal keys on it)")
	}

	// Inside a pass: minted.
	c := &CheckState{}
	done := c.Begin()
	if !CheckPassActive() {
		t.Error("CheckPassActive must report a live pass")
	}
	if v := NewInteger(7); v.ID == "" {
		t.Error("mint inside a pass must carry an ID")
	}
	// Nested pass: still minted after the INNER done.
	c2 := &CheckState{}
	done2 := c2.Begin()
	done2()
	if v := NewInteger(7); v.ID == "" {
		t.Error("outer pass still live — mint must carry an ID")
	}
	// done called TWICE must not drive the counter negative: after the
	// single outer done the counter is zero…
	done2()
	done()
	if v := NewInteger(7); v.ID != "" {
		t.Errorf("all passes ended — mint carries ID %q, want elided", v.ID)
	}
	// …and a later legitimate pass still mints (the double-dec guard).
	c3 := &CheckState{}
	done3 := c3.Begin()
	if v := NewInteger(7); v.ID == "" {
		t.Error("later pass after a double-done must still mint IDs")
	}
	done3()

	// Explicit mint scope (the parser's arm).
	end := BeginIDMintScope()
	if v := NewString("x"); v.ID == "" {
		t.Error("mint scope must arm ID minting")
	}
	end()
	end() // second call is a no-op
	if v := NewString("x"); v.ID != "" {
		t.Errorf("after scope end mint carries ID %q, want elided", v.ID)
	}
	// nil CheckState: Begin is a no-op closure, counter untouched.
	var nilC *CheckState
	nilDone := nilC.Begin()
	nilDone()
}

func TestSameFnConstruction(t *testing.T) {
	sigs := []Signature{{Params: []FnParam{{Name: "n", Type: TInteger}}}}
	orig := NewFunction(FnDefInfo{Signatures: sigs})
	cpy := orig // by-value copy shares the Signatures backing array
	if !sameFnConstruction(orig, cpy) {
		t.Error("copies of one construction must match")
	}
	other := NewFunction(FnDefInfo{Signatures: []Signature{{Params: []FnParam{{Name: "n", Type: TInteger}}}}})
	if sameFnConstruction(orig, other) {
		t.Error("distinct constructions must not match")
	}
	if sameFnConstruction(orig, NewInteger(1)) {
		t.Error("non-fn operand must not match")
	}
	// ID fallback arm: same ID, rebuilt (non-shared) sig backing.
	end := BeginIDMintScope()
	x := NewFunction(FnDefInfo{Signatures: []Signature{{}}})
	end()
	y := x
	y.Data = FnDefInfo{Signatures: append([]Signature(nil), x.Data.(FnDefInfo).Signatures...)}
	if !sameFnConstruction(x, y) {
		t.Error("same-ID rebuilt copy must match via the ID fallback")
	}
}
