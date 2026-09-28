package core

import "testing"

// fn_value_stack_match_test.go pins FnValueStackMatches — the legacy
// pure-stack path's selection (ExecFnDefSigStackMatch), exported for the VM's
// no-match park — in both directions: what it dispatches, and what it must
// refuse.

func stackMatchFn(sigs ...Signature) FnDefInfo {
	return FnDefInfo{Anonymous: true, Signatures: sigs}
}

func stackMatchSigOf(params ...FnParam) Signature {
	return Signature{Params: params, BarrierPos: BarrierAllForward, Impl: Boru(nil)}
}

func TestFnValueStackMatches(t *testing.T) {
	five, s := NewInteger(5), NewString("s")
	named := stackMatchFn(stackMatchSigOf(FnParam{Name: "a", Type: TInteger}, FnParam{Name: "b", Type: TString}))
	unnamed := stackMatchFn(stackMatchSigOf(FnParam{Type: TInteger}, FnParam{Type: TString}))
	om := NewOrderedMap()
	om.Set("k", NewInteger(1))
	mapPat := NewMap(om)
	other := NewOrderedMap()
	other.Set("z", NewInteger(1))
	onePat := NewInteger(1)
	for _, c := range []struct {
		label    string
		fn       FnDefInfo
		resolved []Value
		want     bool
	}{
		{"a 0-param signature always dispatches", stackMatchFn(stackMatchSigOf()), nil, true},
		{"a named signature reads the top first", named, []Value{s, five}, true},
		{"a named signature refuses the bottom-up order", named, []Value{five, s}, false},
		{"an unnamed signature reads its window bottom-up", unnamed, []Value{five, s}, true},
		{"an unnamed signature refuses the top-down order", unnamed, []Value{s, five}, false},
		{"too few stack values", named, []Value{five}, false},
		{"a map pattern the value opens onto", stackMatchFn(stackMatchSigOf(FnParam{Name: "m", Type: TMap, Pattern: &mapPat})), []Value{NewMap(om)}, true},
		{"a map pattern the value misses", stackMatchFn(stackMatchSigOf(FnParam{Name: "m", Type: TMap, Pattern: &mapPat})), []Value{NewMap(other)}, false},
		{"a scalar pattern the value unifies with", stackMatchFn(stackMatchSigOf(FnParam{Name: "n", Type: TInteger, Pattern: &onePat})), []Value{NewInteger(1)}, true},
		{"a scalar pattern the value misses", stackMatchFn(stackMatchSigOf(FnParam{Name: "n", Type: TInteger, Pattern: &onePat})), []Value{five}, false},
	} {
		if got := FnValueStackMatches(c.fn, c.resolved); got != c.want {
			t.Errorf("%s: FnValueStackMatches = %v, want %v", c.label, got, c.want)
		}
	}
}

// TestUncalledRaisePos: the value's own position, else the first candidate
// that has one.
func TestUncalledRaisePos(t *testing.T) {
	at := SrcPos{Row: 2, Col: 3}
	if got := UncalledRaisePos(at, nil); got != at {
		t.Errorf("the value's own position wins: %+v", got)
	}
	cand := WithPosAt(NewInteger(1), SrcPos{Row: 4, Col: 5})
	if got := UncalledRaisePos(SrcPos{}, []Value{NewInteger(0), cand}); got.Row != 4 || got.Col != 5 {
		t.Errorf("a position-less value takes the first positioned candidate: %+v", got)
	}
}
