package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// TestPlanLiteralBuild: a fresh literal's construction plan builds its own
// spine — the outer list, a nested literal, a map — and shares every other
// part: a scalar, an embedded enclosing binding's container (the keep set),
// a nested fn value. The literal itself as a fn value is a new function per
// evaluation; a mutable container baked in that is not a plain list or map
// is an independent copy.
func TestPlanLiteralBuild(t *testing.T) {
	member := core.NewList([]core.Value{core.NewInteger(9)})
	core.SetID(&member, "bind-c")
	inner := core.NewList([]core.Value{member, core.NewInteger(1)})
	om := core.NewOrderedMap()
	om.Set("m", member)
	om.Implicit = true
	mapLit := core.NewMap(om)
	fnv := core.NewFunction(core.FnDefInfo{Anonymous: true})
	flex := core.NewFlexList([]core.Value{core.NewInteger(2)})
	outer := core.NewList([]core.Value{inner, member, mapLit, fnv, flex})

	b := planLiteralBuild(&outer, map[string]bool{"bind-c": true})
	if b.Kind != BuildList || len(b.Parts) != 5 {
		t.Fatalf("outer: kind %d with %d parts, want a list of 5", b.Kind, len(b.Parts))
	}
	want := []LiteralBuildKind{BuildList, BuildShared, BuildMap, BuildShared, BuildCopy}
	for i, k := range want {
		if b.Parts[i].Kind != k {
			t.Errorf("part %d: kind %d, want %d", i, b.Parts[i].Kind, k)
		}
	}
	if in := b.Parts[0]; in.Parts[0].Kind != BuildShared || in.Parts[1].Kind != BuildShared {
		t.Errorf("the nested literal must share its kept member and its scalar: %+v", in.Parts)
	}
	if m := b.Parts[2]; len(m.Keys) != 1 || m.Keys[0] != "m" || !m.Implicit || m.Parts[0].Kind != BuildShared {
		t.Errorf("the map plan must carry its key, its marks and its kept member: %+v", m)
	}
	// Without the keep set the member is the literal's own construction.
	if plain := planLiteralBuild(&outer, nil); plain.Parts[1].Kind != BuildList {
		t.Errorf("an unkept member list is constructed: kind %d", plain.Parts[1].Kind)
	}
	if fb := planLiteralBuild(&fnv, nil); fb.Kind != BuildFn {
		t.Errorf("a fn literal is a new function per evaluation: kind %d", fb.Kind)
	}
	empty := core.NewMap(nil)
	if eb := planLiteralBuild(&empty, nil); eb.Kind != BuildMap || eb.Keys != nil {
		t.Errorf("a map with no entries plans no keys: %+v", eb)
	}
}

// TestPlanConstBuilds: only a const a fresh push reads gets a build — an
// OpPushConstFresh in any unit, an OpPushConstFreshLocal seat — and a
// program without one carries none.
func TestPlanConstBuilds(t *testing.T) {
	lit := core.NewList([]core.Value{core.NewInteger(1)})
	p := &Program{Consts: []core.Value{core.NewInteger(7), lit, lit, lit}}
	p.Code = []Instr{{Op: OpPushConst, Arg: 0}}
	planConstBuilds(p)
	if p.ConstBuilds != nil {
		t.Fatalf("no fresh push, no builds: %v", p.ConstBuilds)
	}
	p.Fns = []CompiledFn{{Code: []Instr{{Op: OpPushConstFresh, Arg: 1}, {Op: OpPushConst, Arg: 2}}}}
	p.ConstLocals = []ConstLocalRef{{ConstIdx: 3, Slot: 0}, {ConstIdx: 9, Slot: 1}}
	planConstBuilds(p)
	if len(p.ConstBuilds) != 4 || p.ConstBuilds[0] != nil || p.ConstBuilds[2] != nil {
		t.Fatalf("builds for the fresh pushes only: %v", p.ConstBuilds)
	}
	if p.ConstBuilds[1] == nil || p.ConstBuilds[3] == nil || p.ConstBuilds[1].Kind != BuildList {
		t.Fatalf("the fresh push and the seat each get their build: %v", p.ConstBuilds)
	}
}
