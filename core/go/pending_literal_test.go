package core

import (
	"reflect"
	"testing"
)

// pending_literal_test.go pins NUR356's classification of the pending
// literals a spliced `if` arm leaves: which evaluate to what was written,
// which only read bindings (and which names), and which call.

// pendingLit is a parser-shaped (Eval) list literal of elems.
func pendingLit(elems ...Value) Value {
	return NewEvalList(elems)
}

// pendingMapLit is a parser-shaped (Eval) map literal of one key.
func pendingMapLit(key string, v Value) Value {
	m := NewOrderedMap()
	m.Set(key, v)
	out := NewMap(m)
	out.Eval = true
	return out
}

func TestPendingResidueMerge(t *testing.T) {
	a := PendingResidue{Left: true, Reads: []string{"x", "z"}}
	b := PendingResidue{Left: true, Calls: true, Reads: []string{"y", "z"}}
	if got := (PendingResidue{}).Merge(a); !reflect.DeepEqual(got, a) {
		t.Errorf("none with a: %+v", got)
	}
	if got := a.Merge(PendingResidue{}); !reflect.DeepEqual(got, a) {
		t.Errorf("a with none: %+v", got)
	}
	got := a.Merge(b)
	if !got.Left || !got.Calls || !reflect.DeepEqual(got.Reads, []string{"x", "y", "z"}) {
		t.Errorf("a with b: %+v", got)
	}
	if !got.ReadsName("y") || got.ReadsName("w") || got.ReadsName("zz") {
		t.Error("ReadsName")
	}
}

func TestPendingLiteralSteps(t *testing.T) {
	quoted := pendingLit(NewWord("x"))
	quoted.Quoted = true
	ck := pendingMapLit("k", NewInteger(1))
	ckm, _ := AsMutableMap(ck)
	ckm.Meta = map[string]any{"ck": map[string]bool{"k": true}}
	for _, tc := range []struct {
		name string
		v    Value
		want bool
	}{
		{"a quoted list", quoted, false},
		{"a word", NewWord("x"), false},
		{"scalars", pendingLit(NewInteger(1), NewString("s")), false},
		{"a nested literal of scalars", pendingLit(pendingLit(NewInteger(1))), false},
		{"a word read", pendingLit(NewWord("x")), true},
		{"a nested word read", pendingLit(NewInteger(1), pendingLit(NewWord("x"))), true},
		{"a paren", pendingLit(NewParenExpr([]Value{NewInteger(1)})), true},
		{"a map of scalars", pendingMapLit("a", NewInteger(1)), false},
		{"a map value read", pendingMapLit("a", NewWord("x")), true},
		{"a computed key", ck, true},
	} {
		if got := PendingLiteralSteps(tc.v); got != tc.want {
			t.Errorf("%s: %v", tc.name, got)
		}
	}
}

// TestPendingResidueOf: a word is a READ of a binding holding data — a
// concrete value, a typed carrier, a gradual value (guarded where it is a
// fn), a `/v` spelling — and a CALL otherwise: a fn, a value that may be
// one, a builtin word, a member path, an unbound name.
func TestPendingResidueOf(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.Register("bw", Signature{Args: []*Type{TAny}})
	r.Defs.Push("n", NewInteger(1))
	r.Defs.Push("l", NewList([]Value{NewInteger(1)}))
	r.Defs.Push("c", NewCarrier(TInteger))
	r.Defs.Push("g", NewDynamicCarrier(TAny))
	r.Defs.Push("a", NewCarrier(TAny))
	r.Defs.Push("f", NewCarrier(TFunction))
	r.Defs.Push("ty", NewTypeLiteral(TInteger))
	fv := NewValueRaw(TWord, WordInfo{Name: "f", ArgCount: -1, ForceVal: true})
	residue := func(v Value) PendingResidue {
		return pendingResidueOf(r, NewTape([]Value{v}, 0))
	}
	for _, tc := range []struct {
		name  string
		v     Value
		calls bool
		reads []string
	}{
		{"data reads", pendingLit(NewWord("n"), NewWord("l"), NewWord("c"), NewWord("g")), false, []string{"c", "g", "l", "n"}},
		{"a /v spelling", pendingLit(fv), false, []string{"f"}},
		{"a map value read", pendingMapLit("k", NewWord("n")), false, []string{"n"}},
		{"a fn", pendingLit(NewWord("f")), true, nil},
		{"an Any carrier", pendingLit(NewWord("a")), true, nil},
		{"a type", pendingLit(NewWord("ty")), true, nil},
		{"a builtin", pendingLit(NewWord("bw")), true, nil},
		{"a member path", pendingLit(NewWord("n.a")), true, nil},
		{"an unbound name", pendingLit(NewWord("nope")), true, nil},
		{"a paren", pendingLit(NewParenExpr([]Value{NewWord("n")})), true, nil},
	} {
		got := residue(tc.v)
		if !got.Left || got.Calls != tc.calls || !reflect.DeepEqual(got.Reads, tc.reads) {
			t.Errorf("%s: %+v", tc.name, got)
		}
	}
	if got := residue(pendingLit(NewInteger(1))); got.Left {
		t.Errorf("a literal of scalars leaves no residue: %+v", got)
	}
	if got := residue(NewInteger(1)); got.Left {
		t.Errorf("a scalar is no literal: %+v", got)
	}
	if readsData(nil, "n") {
		t.Error("no registry reads nothing")
	}
}

// TestRunCarrierArmBody: a spliced arm's run reports the residue its sweep
// evaluated; a kept (do) run and a plain body report none.
func TestRunCarrierArmBody(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.Defs.Push("n", NewInteger(1))
	body := NewList([]Value{pendingLit(NewWord("n"))})
	stk, _, residue := RunCarrierArmBody(r, body)
	if len(stk) != 1 || !residue.Left || residue.Calls || !residue.ReadsName("n") {
		t.Errorf("the arm's residue: %v %+v", stk, residue)
	}
	if _, _, residue := RunCarrierArmBody(r, NewList([]Value{NewInteger(1)})); residue.Left {
		t.Errorf("a scalar arm: %+v", residue)
	}
	if _, _, residue := runCarrierBody(r, body, true, false); residue.Left {
		t.Errorf("a kept run reports none: %+v", residue)
	}
	if _, _, residue := RunCarrierArmBody(r, Value{}); residue.Left {
		t.Error("no body")
	}
}
