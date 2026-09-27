package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// islandBind is a top-level def of name whose name token stands at column col.
func islandBind(name string, col int) EmitEvent {
	return EmitEvent{kind: evDynBind, dyn: &emitDynBind{name: name, srcSeq: -1, pos: gpos(col)}}
}

// TestMarkIslandMadeDefs pins NUR282's def-group seat (planDeopts): a def
// every island runs again is island-made — it takes no registry-visible
// bind for the islands. `def ok (do b) ok` is one statement; its count
// island resumes at token 0 and makes `ok` itself.
func TestMarkIslandMadeDefs(t *testing.T) {
	body := []core.Value{
		gtok(core.NewWord("def"), 1), gtok(core.NewWord("ok"), 5),
		gtok(core.NewParenExpr([]core.Value{gtok(core.NewWord("do"), 9), gtok(core.NewWord("b"), 12)}), 8),
		gtok(core.NewWord("ok"), 15),
	}
	unit := func(deopts []deoptPoint, events ...EmitEvent) *fnUnitRec {
		return &fnUnitRec{body: body, locals: []string{"b"}, nParams: 1, deopts: deopts, frag: &EmitFragment{events: events}}
	}
	es := NewEmitState()
	late := islandBind("ok", 5)
	es.markIslandMadeDefs(unit([]deoptPoint{{token: 0}}, late))
	if !late.dyn.islandMade {
		t.Error("the island makes ok itself")
	}
	// A later island resuming after the def reads it from the compiled
	// frame; so does a closure child's island, and a def inside an arm.
	after := islandBind("ok", 5)
	es.markIslandMadeDefs(unit([]deoptPoint{{token: 0}, {token: 3}}, after))
	seeded := islandBind("ok", 5)
	child := unit([]deoptPoint{{token: 0}}, seeded)
	child.deoptNames = map[string]bool{"ok": true}
	es.markIslandMadeDefs(child)
	inner := islandBind("ok", 16)
	es.markIslandMadeDefs(unit([]deoptPoint{{token: 0}}, EmitEvent{kind: evBranch, br: &emitBranch{then: &EmitFragment{events: []EmitEvent{inner}}}}))
	for why, d := range map[string]EmitEvent{"an island after the def": after, "a name a closure child seeded": seeded, "a def inside an arm": inner} {
		if d.dyn.islandMade {
			t.Errorf("%s: the def binds for the islands", why)
		}
	}
	// An island-made def neither needs a re-pushable source nor binds.
	src := islandBind("ok", 5)
	src.dyn.srcSeq, src.dyn.islandMade = 3, true
	if !es.deoptDefsBindable([]EmitEvent{src}, map[string]bool{"ok": true}) {
		t.Error("an island-made def asks no source of its own")
	}
	lw := &lowerer{es: es, deoptNames: map[string]bool{"ok": true}}
	if lw.needDynInstall(src.dyn) || !lw.needDynInstall(islandBind("ok", 5).dyn) {
		t.Error("only the islands' read makes a def bind, and an island-made def has none")
	}
}
