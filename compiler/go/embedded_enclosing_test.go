package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// TestEmbeddedEnclosingIDs: the keep set a fn-unit body literal owes its
// fresh push — every enclosing binding's container it embeds, at any depth
// of its own spine, and nothing inside a kept member (its contents are the
// binding's, not the literal's). Scalars are never walked, an empty map's
// nil table is not, and a literal embedding no binding owes none.
func TestEmbeddedEnclosingIDs(t *testing.T) {
	bindC := core.NewList([]core.Value{core.NewInteger(9)})
	bindC.ID = "bind-c"
	insideC := core.NewList([]core.Value{core.NewInteger(1)})
	insideC.ID = "inside-c"
	bindD := core.NewList([]core.Value{insideC})
	bindD.ID = "bind-d"
	enclosing := map[string]bool{"bind-c": true, "bind-d": true, "inside-c": true}

	om := core.NewOrderedMap()
	om.Set("d", bindD)
	om.Set("n", core.NewInteger(2))
	mapLit := core.NewMap(om)
	mapLit.ID = "lit-map"
	nested := core.NewList([]core.Value{core.NewString("s"), bindC})
	nested.ID = "lit-nested"
	outer := core.NewList([]core.Value{nested, mapLit, core.NewInteger(3)})
	outer.ID = "lit-outer"

	keep := embeddedEnclosingIDs(outer, enclosing)
	if len(keep) != 2 || !keep["bind-c"] || !keep["bind-d"] {
		t.Errorf("keep = %v, want bind-c and bind-d only (inside-c is the binding's own content)", keep)
	}
	if keep := embeddedEnclosingIDs(core.NewList([]core.Value{core.NewInteger(1), nested}), map[string]bool{}); keep != nil {
		t.Errorf("a literal embedding no binding owes no keep: %v", keep)
	}
	if keep := embeddedEnclosingIDs(core.NewMap(nil), enclosing); keep != nil {
		t.Errorf("an empty map literal owes no keep: %v", keep)
	}
	emptyOM := core.NewOrderedMap()
	emptyOM.Set("c", bindC)
	if keep := embeddedEnclosingIDs(core.NewMap(emptyOM), enclosing); len(keep) != 1 || !keep["bind-c"] {
		t.Errorf("a map literal's member: keep = %v", keep)
	}
}
