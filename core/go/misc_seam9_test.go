package core

import "testing"

func TestW9CheckStateGuards(t *testing.T) {
	// Begin on a nil check returns a no-op closure that is safe to call.
	done := (*CheckState)(nil).Begin()
	done()

	c := &CheckState{}
	// recordCallEdge with an empty endpoint is a no-op.
	c.RecordCallEdge("", "callee")

	// RecordFnBinder with an empty top-of-stack fn name returns early.
	c.Mode = true
	c.FnNameStack = []string{""}
	c.RecordFnBinder("x")
	if c.FnBinders != nil {
		t.Error("an empty enclosing fn must not record a binder")
	}
}

func TestW9CollectBodyLocalDefs(t *testing.T) {
	locals := map[string]bool{}
	quoted := NewInteger(1)
	quoted.Quoted = true
	// Body: a quoted token (skipped) and a nested fn value (skipped).
	CollectBodyLocalDefs(nil, []Value{quoted, NewFunction(FnDefInfo{})}, locals)
	if len(locals) != 0 {
		t.Errorf("quoted / nested-fn tokens should contribute no locals, got %v", locals)
	}
}

func TestW9BoundNodeClassType(t *testing.T) {
	ct := w9ClassType("T_bn2")
	if got := boundNode(ct); got == nil {
		t.Error("a class type bound should surface its minted node")
	}
}

func TestW9CanonFnDefMultiReturn(t *testing.T) {
	fd := FnDefInfo{
		Name: "w9cf",
		Signatures: []Signature{{
			Params:  []FnParam{{Name: "a", Type: TInteger}},
			Returns: []*Type{TInteger, TString},
			Impl:    Boru([]Value{NewWord("a")}),
		}},
	}
	if got := canonFnDef(fd); got == "" {
		t.Error("canonFnDef should render a non-empty canonical string")
	}
}
