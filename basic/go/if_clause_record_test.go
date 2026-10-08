package basic

import "testing"

// if_clause_record_test.go covers the element classification of the
// clause-list `if`'s compiled lowering (ifClauseRecord, 2026-09-26): which
// conditions the compile pass may decide by the handler's own truthiness,
// and which arms it can place exactly. The lowering itself runs under the
// recording pass, which this module does not link; lang's
// if_clause_list_test.go pins it end to end.

func TestIfClauseStaticCond(t *testing.T) {
	for name, c := range map[string]Value{
		"an integer leaf":          NewInteger(0),
		"a string leaf":            NewString(""),
		"a boolean leaf":           NewBoolean(false),
		"a bare word (true/false)": NewWord("false"),
		"none":                     NewNone(),
	} {
		if !ifClauseStaticCond(c) {
			t.Errorf("%s: its truthiness is the handler's CoerceBoolean over the same token", name)
		}
	}
	if ifClauseStaticCond(NewMap(NewOrderedMap())) {
		t.Error("a map condition is not decided at compile time")
	}
}

func TestIfClauseArm(t *testing.T) {
	body := NewList([]Value{NewInteger(1)})
	if arm, ok := ifClauseArm(body); !ok || arm.ID != body.ID {
		t.Error("a code-body arm runs as it stands")
	}
	for name, v := range map[string]Value{
		"a scalar leaf": NewInteger(7),
		"a true word":   NewWord("true"),
		"a none word":   NewWord("none"),
		"none":          NewNone(),
	} {
		arm, ok := ifClauseArm(v)
		if !ok {
			t.Errorf("%s: placed exactly as the one-token body", name)
			continue
		}
		if lst, err := AsList(arm); err != nil || lst.Len() != 1 {
			t.Errorf("%s: want the one-token body [v], got %v", name, arm)
		}
	}
	for name, v := range map[string]Value{
		"a word the tape would dispatch": NewWord("x"),
		"a modified literal word":        NewValueRaw(TWord, WordInfo{Name: "true", ArgCount: -1, ForceVal: true}),
		"a map":                          NewMap(NewOrderedMap()),
	} {
		if _, ok := ifClauseArm(v); ok {
			t.Errorf("%s: not placed exactly by either spelling", name)
		}
	}
}
