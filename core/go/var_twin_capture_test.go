package core

import "testing"

// twinCaptureEmit is a recorder that keeps every bind twin the ledger hands
// it (RecordBindTwin), active so the capture funnel runs.
type twinCaptureEmit struct {
	EmitRecorder
	twins   []BindTransition
	entries []DefEntry
}

func (c *twinCaptureEmit) Active() bool { return true }
func (c *twinCaptureEmit) RecordBindTwin(tr BindTransition, entry DefEntry) {
	c.twins = append(c.twins, tr)
	c.entries = append(c.entries, entry)
}

// A `var` DECLARATION's bind twin captures a VAR entry: InstallVar marks
// the entry before the ledger note captures it (installOpts.varMark), so
// the compiled lane's replay installs the cell a unit's assignment replaces
// in place. Measured before the mark moved: `var x 0 [1 2] each [var x 5]
// x add 1` answered 1 for the interpreter's 6 once the body's block retired
// the second cell the assignment had declared beside a plain-def replay. A
// def's twin stays a plain entry (the negative half), and the typed form
// carries its type.
func TestInstallVarTwinCapturesTheVarMark(t *testing.T) {
	r := newTestRegistry(t)
	r.Check.Mode = true
	cap := &twinCaptureEmit{EmitRecorder: TheInactiveEmit}
	r.Check.Emit = cap

	InstallVar(r, "v", NewInteger(1), nil)
	InstallVar(r, "w", NewInteger(2), TInteger)
	InstallDef(r, "d", NewInteger(3))
	if len(cap.entries) != 3 {
		t.Fatalf("captured %d twins, want 3 (two var declarations, one def)", len(cap.entries))
	}
	if !cap.entries[0].Var || cap.entries[0].VarType != nil {
		t.Errorf("untyped var declaration captured %+v, want Var with no type", cap.entries[0])
	}
	if !cap.entries[1].Var || cap.entries[1].VarType != TInteger {
		t.Errorf("typed var declaration captured %+v, want Var of Integer", cap.entries[1])
	}
	if cap.entries[2].Var {
		t.Errorf("a def's twin captured a var mark: %+v", cap.entries[2])
	}
	// The table's own entries carry the same marks — the mark moved
	// earlier, it did not move away.
	if e, ok := IsVarBinding(r, "v"); !ok || e.VarType != nil {
		t.Errorf("v is not a var binding after InstallVar: %+v / %v", e, ok)
	}
	if e, ok := IsVarBinding(r, "w"); !ok || e.VarType != TInteger {
		t.Errorf("w is not a typed var binding after InstallVar: %+v / %v", e, ok)
	}
	if _, ok := IsVarBinding(r, "d"); ok {
		t.Error("d is a var binding after InstallDef")
	}
}

// The replay half: a var declaration's twin pushes a VAR cell, which a
// resident assignment then replaces in place — one level, the interpreter's
// — where a plain push had the assignment declare a second cell (the next
// request's `undef x` exposed the extra level, branch_carried_split_test.go).
func TestApplyBindTwinReplaysVarMark(t *testing.T) {
	r := newTestRegistry(t)
	if err := ApplyBindTwin(r, BindTransition{Kind: BindDef, Name: "x"}, DefEntry{Body: NewInteger(0), Var: true, VarType: TInteger}); err != nil {
		t.Fatal(err)
	}
	e, ok := IsVarBinding(r, "x")
	if !ok || e.VarType != TInteger {
		t.Fatalf("the var twin's replay is not a typed var binding: %+v / %v", e, ok)
	}
	ApplyResidentAssign(r, "x", NewInteger(9))
	if d := r.Defs.Depth("x"); d != 1 {
		t.Errorf("x is %d deep after the assignment, want 1 (replaced in place)", d)
	}
	if v, _ := r.Defs.Top("x"); v.String() != "9" {
		t.Errorf("x reads %v after the assignment, want 9", v)
	}
	// The negative half: a plain def's replay is assigned over by a fresh
	// declaration beside it — two levels, as the interpreter's `var x 9`
	// over a def x declares.
	if err := ApplyBindTwin(r, BindTransition{Kind: BindDef, Name: "y"}, DefEntry{Body: NewInteger(0)}); err != nil {
		t.Fatal(err)
	}
	ApplyResidentAssign(r, "y", NewInteger(9))
	if d := r.Defs.Depth("y"); d != 2 {
		t.Errorf("y is %d deep after assigning over a def's replay, want 2 (a fresh declaration)", d)
	}
}
