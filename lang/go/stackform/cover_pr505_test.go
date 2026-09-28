package stackform

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// TestApplyOpSurfaces pins the Apply op (NUR077) through every helper that
// reads a form: it costs as a call, flattens (arity 0 applies the value in
// place with `apply`), renders with its arity, and compares structurally —
// only to an Apply of the same arity.
func TestApplyOpSurfaces(t *testing.T) {
	mk := func(op Op) *StackForm {
		f := &StackForm{}
		f.Append(op)
		return f
	}
	f := mk(Apply{Arity: 0})
	if got := Cost(f); got != 2 {
		t.Errorf("Cost(apply/0) = %d, want 2", got)
	}
	flat := Flatten(f)
	if w, err := core.AsWord(flat[len(flat)-1]); len(flat) != 1 || err != nil || w.Name != "apply" {
		t.Errorf("Flatten(apply/0) = %v, want [apply]", flat)
	}
	if got := Pretty(f); !strings.Contains(got, "apply/0") {
		t.Errorf("Pretty(apply/0) = %q, want it to name apply/0", got)
	}
	if !Equal(f, mk(Apply{Arity: 0})) {
		t.Error("two apply/0 forms are equal")
	}
	if Equal(f, mk(Apply{Arity: 1})) {
		t.Error("apply/0 and apply/1 differ")
	}
	if Equal(f, mk(Call{Name: "apply"})) {
		t.Error("an Apply is not a Call named apply")
	}
}
