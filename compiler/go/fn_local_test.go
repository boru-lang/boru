package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// placeFnLocalDef's arms (the seventy-second increment): nothing for a nil
// or inactive recorder, no open unit, a lambda (no declaration site), a
// capturing fn, a def the unit's frames do not hold, or a root def; the
// innermost matching def event — in the current frame or an outer one of
// the unit — is stamped as a placed install, before a name the seventieth
// increment placed counts as placed already.
func TestPlaceFnLocalDef(t *testing.T) {
	decl := core.DeclSite{Pos: core.SrcPos{Row: 1, Col: 9}, File: "g.boru"}
	fnv := core.NewFunction(core.FnDefInfo{Name: "f", Signatures: []core.Signature{{Impl: core.Boru([]core.Value{core.NewInteger(1)}), Decl: decl}}})
	lam := core.NewFunction(core.FnDefInfo{Name: "f", Anonymous: true, Signatures: []core.Signature{{Impl: core.Boru([]core.Value{core.NewInteger(1)})}}})
	cap := core.NewFunction(core.FnDefInfo{Name: "f", Captured: []core.CapturedBinding{{Name: "k", Value: core.NewInteger(1)}}, Signatures: []core.Signature{{Impl: core.Boru([]core.Value{core.NewInteger(1)}), Decl: decl}}})
	var nilES *EmitState
	if nilES.placeFnLocalDef("f", fnv) {
		t.Fatal("a nil recorder places nothing")
	}
	es := NewEmitState()
	if es.placeFnLocalDef("f", fnv) {
		t.Fatal("no open unit: nothing to place")
	}
	unit, _, ok := es.StartFnCompile("g", "g", nil, nil, nil, nil, nil, false, core.SrcPos{})
	if !ok || unit < 0 {
		t.Fatalf("StartFnCompile declined: %d %v", unit, ok)
	}
	if es.placeFnLocalDef("", fnv) || es.placeFnLocalDef("f", lam) || es.placeFnLocalDef("f", cap) || es.placeFnLocalDef("f", core.NewInteger(1)) {
		t.Fatal("no name, a lambda, a capturing fn, a data value: not placed")
	}
	if es.placeFnLocalDef("f", fnv) {
		t.Fatal("no def of f in the unit's frames: not placed")
	}
	// The unit's latest def of the name is NOT the current binding (a
	// closed body redefined it — review of #468): declined, whatever the
	// family says.
	other := core.NewFunction(core.FnDefInfo{Name: "f", Signatures: []core.Signature{{Impl: core.Boru([]core.Value{core.NewInteger(2)}), Decl: core.DeclSite{Pos: core.SrcPos{Row: 2, Col: 9}, File: "g.boru"}}}})
	es.frames[len(es.frames)-1] = append(es.frames[len(es.frames)-1], EmitEvent{kind: evDynBind, dyn: &emitDynBind{name: "f", srcSeq: -1, residentTwin: -1, val: other}})
	es.specFnNames = map[string]bool{"f": true}
	if es.placeFnLocalDef("f", fnv) {
		t.Fatal("a def event that is not the current binding's is not stamped")
	}
	if fr := es.frames[len(es.frames)-1]; fr[len(fr)-1].dyn.specFn {
		t.Fatal("the stale event stays unstamped")
	}
	es.frames[len(es.frames)-1] = es.frames[len(es.frames)-1][:len(es.frames[len(es.frames)-1])-1]
	es.specFnNames = nil
	two := core.NewFunction(core.FnDefInfo{Name: "f", Signatures: []core.Signature{{Impl: core.Boru(nil), Decl: decl}, {Impl: core.Boru(nil), Decl: decl}}})
	if sameFnDecls(fnv, two) || sameFnDecls(fnv, lam) || sameFnDecls(lam, lam) || sameFnDecls(fnv, core.NewInteger(1)) || !sameFnDecls(fnv, cap) {
		t.Fatal("sameFnDecls: the count, an empty site and a data value differ; the same sites agree")
	}
	// A ROOT def event of the name is not the unit's.
	es.frames[len(es.frames)-1] = append(es.frames[len(es.frames)-1], EmitEvent{kind: evDynBind, dyn: &emitDynBind{name: "f", root: true, srcSeq: -1, residentTwin: -1}})
	if es.placeFnLocalDef("f", fnv) {
		t.Fatal("a root def is not placed here")
	}
	// The unit's own def, in an outer frame of the unit, with a nested
	// frame open: found and stamped.
	es.frames[len(es.frames)-1] = append(es.frames[len(es.frames)-1], EmitEvent{kind: evDynBind, dyn: &emitDynBind{name: "f", srcSeq: -1, residentTwin: -1, val: fnv}})
	es.frames = append(es.frames, nil)
	if !es.placeFnLocalDef("f", fnv) {
		t.Fatal("the unit's def is placed")
	}
	es.frames = es.frames[:len(es.frames)-1]
	outer := es.frames[len(es.frames)-1]
	if d := outer[len(outer)-1].dyn; !d.specFn {
		t.Fatal("the def event is stamped as a placed install")
	}
	// A family the seventieth increment placed: placed already — and a
	// body-local def of the family's name is placed ITSELF (the frames are
	// searched first; NUR149's disjoint twin), not left to the family.
	es2 := NewEmitState()
	es2.StartFnCompile("g", "g", nil, nil, nil, nil, nil, false, core.SrcPos{})
	es2.specFnNames = map[string]bool{"f": true}
	if !es2.placeFnLocalDef("f", fnv) {
		t.Fatal("a speculative family is placed already")
	}
	es2.frames[len(es2.frames)-1] = append(es2.frames[len(es2.frames)-1], EmitEvent{kind: evDynBind, dyn: &emitDynBind{name: "f", srcSeq: -1, residentTwin: -1, val: fnv}})
	if !es2.placeFnLocalDef("f", fnv) {
		t.Fatal("the unit's own def of a family's name is placed")
	}
	if fr := es2.frames[len(es2.frames)-1]; !fr[len(fr)-1].dyn.specFn {
		t.Fatal("the unit's own def of a family's name is stamped, not left to the family")
	}
	resume := es2.Suspend()
	placed := es2.placeFnLocalDef("f", fnv)
	resume()
	if placed {
		t.Fatal("a suspended recorder places nothing")
	}
}
