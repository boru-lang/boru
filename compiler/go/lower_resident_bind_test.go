package compiler

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// lowerResidentBind's value-operand arms, driven directly (the corpus
// exercises the promoted and literal arms; these pin the SIM-TOP arm and
// the compile failure): a computed source sitting on the sim top is PEEKED when it
// is live (its downstream readers still consume it) and POPPED when it is
// dead (kept only for this bind — the install consumes it, the
// interpreter's own def semantics); a computed source neither on the sim
// top nor promoted declines the whole program rather than installing a
// wrong value.
func TestLowerResidentBindSimTopArms(t *testing.T) {
	pos := core.SrcPos{Row: 1, Col: 4}
	newLW := func(vm []vmSlot, dead map[int]bool) (*lowerer, *CompiledFn) {
		cf := &CompiledFn{}
		return &lowerer{es: NewEmitState(), p: &Program{}, code: &cf.Code, debug: &cf.Debug,
			sigIdx: map[*core.Signature]int{}, variadic: map[int]bool{}, promoted: map[int]int{},
			dead: dead, vm: vm}, cf
	}

	// Live sim-top source: peeked, the sim slot stays.
	lw, cf := newLW([]vmSlot{{seq: 7, idx: 0}}, map[int]bool{})
	if reason := lw.lowerResidentBind(&emitDynBind{name: "x", srcSeq: 7, residentTwin: 0, pos: pos}); reason != "" {
		t.Fatalf("live sim-top bind declined: %s", reason)
	}
	if len(lw.p.ResidentBinds) != 1 || lw.p.ResidentBinds[0].Pop || len(lw.vm) != 1 || len(cf.Code) != 1 || cf.Code[0].Op != OpBindResident {
		t.Fatalf("live source must be peeked in place: binds=%+v vm=%v code=%v", lw.p.ResidentBinds, lw.vm, cf.Code)
	}

	// Dead sim-top source: popped, the sim slot goes with it.
	lw, _ = newLW([]vmSlot{{seq: 7, idx: 0}}, map[int]bool{7: true})
	if reason := lw.lowerResidentBind(&emitDynBind{name: "x", srcSeq: 7, residentTwin: 0, pos: pos}); reason != "" {
		t.Fatalf("dead sim-top bind declined: %s", reason)
	}
	if len(lw.p.ResidentBinds) != 1 || !lw.p.ResidentBinds[0].Pop || len(lw.vm) != 0 {
		t.Fatalf("dead source must be consumed: binds=%+v vm=%v", lw.p.ResidentBinds, lw.vm)
	}

	// Neither on the sim top nor promoted: declines.
	lw, _ = newLW(nil, map[int]bool{})
	if reason := lw.lowerResidentBind(&emitDynBind{name: "x", srcSeq: 9, residentTwin: 0, pos: pos}); !strings.Contains(reason, "unpromoted computed value") {
		t.Fatalf("unpromoted computed source must decline, got %q", reason)
	}
}

// collectResidentBindConsumes marks the DEAD computed sources a stamped
// resident def consumes — and only those: an unstamped def site (a root
// dyn-bind) or a live source is left to its own drop discipline.
func TestCollectResidentBindConsumes(t *testing.T) {
	events := []EmitEvent{
		{kind: evDynBind, seq: 1, dyn: &emitDynBind{name: "a", residentTwin: 0, srcSeq: 5}},
		{kind: evDynBind, seq: 2, dyn: &emitDynBind{name: "b", residentTwin: -1, srcSeq: 6}},
		{kind: evDynBind, seq: 3, dyn: &emitDynBind{name: "c", residentTwin: 1, srcSeq: 8}},
	}
	out := collectResidentBindConsumes(events, map[int]bool{5: true, 6: true})
	if len(out) != 1 || !out[5] {
		t.Fatalf("consumes = %v, want exactly the stamped dead source 5", out)
	}
}
