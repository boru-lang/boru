package eng

import (
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// TestTokenBodyKeying: a body of words and scalars is named by its text with
// positions and its input shape; a body carrying a reference value by its ID;
// one with neither is not named. An input with no type reads as Any.
func TestTokenBodyKeying(t *testing.T) {
	add := core.NewWord("add")
	one := core.NewInteger(1)
	text := []core.Value{add, one, core.NewList([]core.Value{core.NewString("s")})}
	body := core.NewList(text)
	body.ID = ""
	k1, ok := tokenBodyKey(body, text, []core.Value{core.NewInteger(5)})
	if !ok || k1[:4] != "txt:" || k1[len(k1)-len("/1,Integer"):] != "/1,Integer" {
		t.Fatalf("a text body keys by its text and input shape: %q %v", k1, ok)
	}
	k2, _ := tokenBodyKey(body, text, []core.Value{core.NewString("x")})
	if k1 == k2 {
		t.Fatal("the input type is part of the key")
	}
	om := core.NewOrderedMap()
	om.Set("a", one)
	ref := []core.Value{core.NewMap(om), core.NewWord("size")}
	withID := core.NewList(ref)
	withID.ID = "L_body"
	if k, ok := tokenBodyKey(withID, ref, nil); !ok || k != "id:L_body/0" {
		t.Fatalf("a reference-bearing body keys by its ID: %q %v", k, ok)
	}
	withID.ID = ""
	if _, ok := tokenBodyKey(withID, ref, nil); ok {
		t.Fatal("a reference-bearing body with no ID is not named")
	}
	nested := []core.Value{core.NewList(ref)}
	if core.TokenBodyContentKeyable(nested) {
		t.Fatal("a reference inside a nested list is not identity-free")
	}
	if core.TokenBodyInputType(core.Value{}) != core.TAny {
		t.Fatal("an input with no type declares Any")
	}
}

// TestTokenBodyFlowEscape: the escape carries its opcode to the registry's
// flag — break and continue — and reads as an error while it travels.
func TestTokenBodyFlowEscape(t *testing.T) {
	if flowCtrlOf(compiler.OpFlowBreak) != core.FlowBreak || flowCtrlOf(compiler.OpFlowContinue) != core.FlowContinue {
		t.Fatal("the escape's opcode maps to the registry flag")
	}
	if (&flowEscape{op: compiler.OpFlowBreak}).Error() == "" {
		t.Fatal("the escape reads as an error while it travels")
	}
}
