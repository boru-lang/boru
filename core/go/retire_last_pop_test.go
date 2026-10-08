package core

import "testing"

// TestMintedNodeRetiredByTheLastPop pins NUR135: a minted type node pushed
// under one name more than once — the shape a bind twin's replay of one
// captured entry produces — stays in the ID index until the LAST live
// binding holding it is popped. Before this the first pop retired it out
// from under the survivors ("unresolvable type operand").
func TestMintedNodeRetiredByTheLastPop(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	node := r.Types.MintType("Big", TInteger)
	body := NewTypeLiteral(node)
	r.Defs.PushType("Big", node, body)
	r.Defs.PushType("Big", node, body)
	if !UninstallType(r, "Big") {
		t.Fatal("the first pop must find a binding")
	}
	if got := r.Types.LookupByID(node.ID); got != node {
		t.Fatalf("the first pop retired a node a live binding still holds: LookupByID = %v", got)
	}
	if !r.Defs.HoldsType(node) {
		t.Fatal("the surviving level must still hold the node")
	}
	PopLiveBinding(r, "Big")
	if got := r.Types.LookupByID(node.ID); got != nil {
		t.Fatalf("the last pop must retire the node: LookupByID = %v", got)
	}
	if r.Defs.HoldsType(node) || r.Defs.HoldsType(nil) {
		t.Fatal("nothing holds the node now, and nil is held by nothing")
	}
}
