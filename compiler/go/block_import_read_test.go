package compiler

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// TestBlockImportRead pins the lowerer's decline for a run-time read of a
// namespace an `import` bound inside a block (CheckState.BlockImportNames):
// the noted name declines with its reason, every other name — and a lowerer
// with no emit state, registry or check state behind it — lowers as before.
func TestBlockImportRead(t *testing.T) {
	bare := &lowerer{p: &Program{}}
	if r := bare.blockImportRead(0); r != "" {
		t.Errorf("no emit state: %q", r)
	}
	es := NewEmitState()
	es.consts = []core.Value{core.NewString("M"), core.NewString("N"), core.NewInteger(7)}
	lw := &lowerer{p: &Program{}, es: es}
	if r := lw.blockImportRead(0); r != "" {
		t.Errorf("no registry: %q", r)
	}
	reg, err := core.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	es.reg = reg
	if r := lw.blockImportRead(0); r != "" {
		t.Errorf("nothing noted: %q", r)
	}
	reg.Check.BlockImportNames = map[string]bool{"M": true}
	if r := lw.blockImportRead(0); !strings.Contains(r, "block-local import `M` read at run time") {
		t.Errorf("the noted name declines: %q", r)
	}
	if r := lw.blockImportRead(1); r != "" {
		t.Errorf("another name: %q", r)
	}
	if r := lw.blockImportRead(2); r != "" {
		t.Errorf("a const that is no name: %q", r)
	}
	// noteBlockImportRead keeps the first decline for the event loop.
	lw.noteBlockImportRead(1)
	if lw.blockImportReason != "" {
		t.Errorf("another name notes nothing: %q", lw.blockImportReason)
	}
	lw.noteBlockImportRead(0)
	lw.noteBlockImportRead(1)
	if !strings.Contains(lw.blockImportReason, "`M`") {
		t.Errorf("the first decline stands: %q", lw.blockImportReason)
	}
	check := reg.Check
	reg.Check = nil
	if r := lw.blockImportRead(0); r != "" {
		t.Errorf("no check state: %q", r)
	}
	reg.Check = check
}
