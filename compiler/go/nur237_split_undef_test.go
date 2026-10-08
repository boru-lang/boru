package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// TestSplitUndefExposesLive pins the carried-undef exemption for a root
// split-bound name a branch join seated: only when no loop carries the name
// and the binding under the join has no home in the slot.
func TestSplitUndefExposesLive(t *testing.T) {
	setup := func(below core.Value) *EmitState {
		reg, err := core.NewRegistry()
		if err != nil {
			t.Fatal(err)
		}
		reg.Defs.Push("x", below)
		reg.Defs.Push("x", core.Value{ID: "joined"})
		es := NewEmitState()
		es.reg = reg
		es.loopSplitBinds = map[string]bool{"x": true}
		es.units[0].nameSlots = map[string]int{"x": 2}
		es.units[0].localByID = map[string]int{"joined": 2}
		return es
	}
	if !setup(core.Value{ID: "split"}).splitUndefExposesLive("x") {
		t.Error("the split's own binding under the join is read live")
	}
	for why, es := range map[string]*EmitState{
		"a binding under the join the slot holds": func() *EmitState {
			es := setup(core.Value{ID: "earlier"})
			es.units[0].localByID["earlier"] = 2
			return es
		}(),
		"a loop carries the name": func() *EmitState {
			es := setup(core.Value{ID: "split"})
			es.loopCarried = []*loopCarriedScope{{slots: map[string]int{"x": 3}}}
			return es
		}(),
		"no join seated the name": func() *EmitState {
			es := setup(core.Value{ID: "split"})
			es.units[0].nameSlots = nil
			return es
		}(),
		"a name that is not split-bound": func() *EmitState {
			es := setup(core.Value{ID: "split"})
			es.loopSplitBinds = nil
			return es
		}(),
	} {
		if es.splitUndefExposesLive("x") {
			t.Errorf("%s: the undef keeps its decline", why)
		}
	}
	// One level: nothing lies under the join.
	es := setup(core.Value{ID: "split"})
	es.reg.Defs.Pop("x")
	if es.splitUndefExposesLive("x") {
		t.Error("an undef of the name's only level exposes nothing")
	}
}

// TestSettleDemotedStrips pins the strip settling after demotion: a strip
// over a demoted region is one value, a strip over a region still standing
// keeps its marks, and a strip over a settled strip settles too.
func TestSettleDemotedStrips(t *testing.T) {
	es := NewEmitState()
	region := eventFlags{variadicResult: true, variadicRegion: true, regionMayBeFn: true}
	strip := func(src int) eventFlags {
		f := region
		f.stripFrom, f.stripSrc = true, src
		return f
	}
	es.eventInfo = map[int]eventFlags{
		1: {variadicResult: true, dynBodyOne: true},
		2: strip(1),
		3: strip(2),
		4: region,
		5: strip(4),
	}
	es.settleDemotedStrips()
	for _, seq := range []int{2, 3} {
		if f := es.eventInfo[seq]; f.variadicRegion || f.regionMayBeFn {
			t.Errorf("strip %d over a demoted run must settle: %+v", seq, f)
		}
	}
	if f := es.eventInfo[5]; !f.variadicRegion || !f.regionMayBeFn {
		t.Errorf("a strip over a standing region stays one: %+v", f)
	}
}
