package core

import "testing"

// TestSingleAggregateReusedAcrossRebindings: a fn bound again under the same
// name with the same signatures — what a Function parameter's frame install
// does on every call — looks up as the aggregate the first binding built,
// plans and all; anything the aggregate is built from compiles afresh.
func TestSingleAggregateReusedAcrossRebindings(t *testing.T) {
	r := w8reg(t)
	v := NewFunction(FnDefInfo{
		Signatures: []Signature{{Params: []FnParam{{Name: "n", Type: TInteger}}, Impl: Boru([]Value{NewWord("n")})}},
	})
	bind := func(name string) *FnDefInfo {
		InstallFrameBinding(r, name, v)
		return r.Lookup(name)
	}
	first := bind("sap")
	r.Defs.Pop("sap")
	again := bind("sap")
	if again != first {
		t.Fatal("re-binding the same fn under the same name rebuilt its aggregate")
	}
	if again.plans == nil || len(again.Signatures) != 2 {
		t.Fatalf("the remembered aggregate lost its plan store or its fallback: %+v", again)
	}
	r.Defs.Pop("sap")
	// Another name, another registry, an analysis pass: built afresh.
	if other := bind("saq"); other == first {
		t.Error("a binding under another name answered with the first name's aggregate")
	}
	r.Defs.Pop("saq")
	r2 := w8reg(t)
	InstallFrameBinding(r2, "sap", v)
	if r2.Lookup("sap") == first {
		t.Error("another registry answered with the first registry's aggregate")
	}
	fd, _ := v.Data().(*FnDefInfo)
	cases := []struct {
		name string
		edit func(*FnDefInfo)
	}{
		{"signatures", func(f *FnDefInfo) { f.Signatures = append([]Signature(nil), f.Signatures...) }},
		{"captures", func(f *FnDefInfo) { f.Captured = []CapturedBinding{{Name: "c", Value: NewInteger(1)}} }},
		{"home", func(f *FnDefInfo) { f.Registry = r2 }},
		{"anonymity", func(f *FnDefInfo) { f.Anonymous = !f.Anonymous }},
		{"macro", func(f *FnDefInfo) { f.Macro = true }},
		{"mini kind", func(f *FnDefInfo) { f.MiniKind = "re" }},
		{"extension", func(f *FnDefInfo) { f.Extends = "base" }},
	}
	r.rememberSingleAggregate("sam", fd, first)
	for _, c := range cases {
		edited := *fd
		c.edit(&edited)
		if r.singleAggregateFor("sam", &edited) != nil {
			t.Errorf("a binding with another %s answered from the memo", c.name)
		}
	}
	if r.singleAggregateFor("sam", fd) != first {
		t.Error("the remembered binding missed its own memo")
	}
	r.Check.Mode = true
	if r.singleAggregateFor("sam", fd) != nil {
		t.Error("an analysis pass answered from the memo")
	}
	r.rememberSingleAggregate("sam", fd, nil)
	r.Check.Mode = false
	if r.singleAggregateFor("sam", fd) != first {
		t.Error("an analysis pass's aggregate replaced the run-time memo")
	}
	// A fn with a home (a module's export) is never kept: its aggregate's
	// Registry is read through the home predicates, not compared.
	homed := *fd
	homed.Registry = r2
	r.rememberSingleAggregate("sah", &homed, first)
	if r.singleAggregateFor("sah", fd) != nil {
		t.Error("a homed fn's aggregate was remembered")
	}
	bare := FnDefInfo{Signatures: fd.Signatures}
	r.rememberSingleAggregate("sam", &bare, first)
	if r.singleAggregateFor("sam", &bare) != nil {
		t.Error("a payload with no identity answered from a memo")
	}
}
