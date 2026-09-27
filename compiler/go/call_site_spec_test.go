package compiler

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// SetUnitSpecialisation seats a specialised unit's guards and fallback on its
// record (an out-of-range unit is a no-op, as for every SetUnit* setter), and
// the disassembler renders a specialised unit's guards on its header.
func TestSetUnitSpecialisationAndNote(t *testing.T) {
	es := NewEmitState()
	es.SetUnitSpecialisation(-1, []int{0}, nil, core.Value{}) // out of range → no-op
	fn := core.NewFunction(core.FnDefInfo{Name: "inc"})
	es.fnRecs = append(es.fnRecs, &fnUnitRec{name: "h"})
	es.SetUnitSpecialisation(0, []int{0}, []core.Value{fn}, fn)
	if rec := es.fnRecs[0]; len(rec.specParams) != 1 || !core.ExactEqual(rec.specFallback, fn) {
		t.Fatalf("the specialisation was not seated: %+v", rec)
	}
	p := &Program{Fns: []CompiledFn{{Name: "h", NParams: 1, SpecGuards: []SpecGuard{{Param: 0, Fn: fn}}, Code: []Instr{{Op: OpRet}}}, {Name: "k", Code: []Instr{{Op: OpRet}}}}}
	dis := p.Disassemble()
	if !strings.Contains(dis, "fn f0 h/1 (locals=0) spec [l0=fn inc]:") {
		t.Errorf("want the guard on the header:\n%s", dis)
	}
	if strings.Contains(dis, "fn f1 k/0 (locals=0) spec") {
		t.Errorf("an ordinary unit carries no note:\n%s", dis)
	}
}
