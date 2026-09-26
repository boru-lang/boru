package debugger

import (
	"bytes"
	"strings"
	"testing"

	"github.com/boru-lang/boru/lang/go/native"
)

// TestPostMortemFaultWithoutFiringRegistry: a fault note from an engine that
// stamped no registry (a nil `from`) keeps the session's own registry as the
// raise's scope, so PostMortem still has defs to inspect (NUR201).
func TestPostMortemFaultWithoutFiringRegistry(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	s := New(reg, Config{
		In:         strings.NewReader(""),
		Out:        &buf,
		File:       "unit.boru",
		Source:     "line one",
		PostMortem: true,
	})
	s.trace(0, nil, "fault: boom", true, false, nil)
	if s.faultNote != "fault: boom" || s.faultReg != reg {
		t.Errorf("the fault's scope falls back to the session's registry: note %q, reg %p (want %p)", s.faultNote, s.faultReg, reg)
	}
}
