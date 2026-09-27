package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// islandBind is a top-level def of name whose name token stands at column col.
func islandBind(name string, col int) EmitEvent {
	return EmitEvent{kind: evDynBind, dyn: &emitDynBind{name: name, srcSeq: -1, pos: gpos(col)}}
}

// TestDropIslandMadeDefs pins NUR282's def-group seat (planDeopts): a def
// every island runs again needs no registry-visible bind, so its name
// leaves the names the islands bind. `def ok (do b) ok` is one statement;
// its count island resumes at token 0 and makes `ok` itself.
func TestDropIslandMadeDefs(t *testing.T) {
	body := []core.Value{
		gtok(core.NewWord("def"), 1), gtok(core.NewWord("ok"), 5),
		gtok(core.NewParenExpr([]core.Value{gtok(core.NewWord("do"), 9), gtok(core.NewWord("b"), 12)}), 8),
		gtok(core.NewWord("ok"), 15),
	}
	unit := func(deopts []deoptPoint, events ...EmitEvent) *fnUnitRec {
		return &fnUnitRec{body: body, locals: []string{"b"}, nParams: 1, deopts: deopts, frag: &EmitFragment{events: events}}
	}
	es := NewEmitState()
	names := map[string]bool{"ok": true, "b": true}
	es.dropIslandMadeDefs(unit([]deoptPoint{{token: 0}}, islandBind("ok", 5)), names)
	if names["ok"] || !names["b"] {
		t.Errorf("the island makes ok itself, and the param stays bound: %v", names)
	}
	// A later island resuming after the def reads it from the compiled
	// frame; so does any island over a name a param holds, a closure
	// child seeded, or a def inside an arm.
	for _, c := range []struct {
		why string
		rec *fnUnitRec
	}{
		{"an island after the def", unit([]deoptPoint{{token: 0}, {token: 3}}, islandBind("ok", 5))},
		{"a param's rebind", unit([]deoptPoint{{token: 0}}, islandBind("b", 5))},
		{"a def inside an arm", unit([]deoptPoint{{token: 0}}, EmitEvent{kind: evBranch, br: &emitBranch{then: &EmitFragment{events: []EmitEvent{islandBind("ok", 16)}}}}, islandBind("ok", 5))},
	} {
		names := map[string]bool{"ok": true, "b": true}
		es.dropIslandMadeDefs(c.rec, names)
		if !names["ok"] || !names["b"] {
			t.Errorf("%s: both names stay: %v", c.why, names)
		}
	}
	seeded := unit([]deoptPoint{{token: 0}}, islandBind("ok", 5))
	seeded.deoptNames = map[string]bool{"ok": true}
	names = map[string]bool{"ok": true}
	if es.dropIslandMadeDefs(seeded, names); !names["ok"] {
		t.Error("a name a closure child seeded stays: its island runs inside the frame")
	}
}
