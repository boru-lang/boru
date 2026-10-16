package native

import (
	"strings"
	"testing"
)

// Codex round-8 fixes (design/TYPED-CONTAINER-TAG-RETENTION.0.md):
//
//	P1 a nested-typed flex param now recursively retags its EXISTING children in
//	   place (preserving identity), so a nested write enforces the contract at
//	   every depth — closing the round-6 limitation.
//	P2 the index-merge overloads and clone get typed check residuals.

// #1 — a write into a NESTED flex-param child enforces the inner {:T} contract,
// identically in the interpreter and compiled, with flex identity preserved.
func TestNestedFlexParamEnforced(t *testing.T) {
	for _, src := range []string{
		`def f fn [[m:{:{:Integer}}] [Any] [(m.a set y/q "bad")]] def u (flex {a:{x:1}}) (f u)`,
		`def f fn [[xs:[:[:Integer]]] [Any] [((xs get 0) set 0 "bad")]] def u (flex [[1 2]]) (f u)`,
	} {
		_, err := tcRun(t, src)
		if err == nil || !strings.Contains(err.Error(), "does not conform to element type Integer") {
			t.Errorf("[%s] nested flex-param write should enforce the inner contract, got %v", src, err)
		}
	}
	// A conforming nested write succeeds AND mutates the caller's shared flex.
	out, err := tcRun(t, `def f fn [[m:{:{:Integer}}] [Any] [(m.a set y/q 5)]] def u (flex {a:{x:1}}) (f u) u`)
	if err != nil {
		t.Fatalf("conforming nested flex write: %v", err)
	}
	if s := out[len(out)-1].String(); !strings.Contains(s, "y:5") {
		t.Errorf("flex identity lost: caller's u = %s, want the nested y:5 mutation", s)
	}
}

// #2a — the index-merge overloads carry a typed check residual, so a chained
// write after an index merge is flagged.
func TestMergeIndexResidualTyped(t *testing.T) {
	list, _ := tcValue(t, `def xs:[:Integer] [1] xs`)
	patch, _ := tcValue(t, `def p {"0":2} p`)
	// mergeListMapReturns: governor is args[0] (the list).
	lm := mergeListMapReturns([]Value{list, patch}, nil)
	if c, ok := lm[0].ElemConstraint(); !ok || !c.Equal(TInteger) {
		t.Errorf("mergeListMapReturns lost the [:Integer] tag")
	}
	// mergeMapListReturns: governor is args[1] (the list).
	ml := mergeMapListReturns([]Value{patch, list}, nil)
	if c, ok := ml[0].ElemConstraint(); !ok || !c.Equal(TInteger) {
		t.Errorf("mergeMapListReturns lost the [:Integer] tag")
	}
}

// #2b — cloneReturnsFn retains the tag across every container kind.
func TestCloneResidualTyped(t *testing.T) {
	cases := []struct {
		src  string
		want bool // want a retained [:Integer]/{:Integer} tag
	}{
		{`def xs:[:Integer] [1 2 3] xs`, true},      // plain list
		{`def m:{:Integer} {a:1} m`, true},          // plain map
		{`def u:[:Integer] (flex [1 2 3]) u`, true}, // flex list (exact-Parent branch)
		{`def xs [1 2 3] xs`, false},                // untyped → no tag
	}
	for _, tc := range cases {
		v, _ := tcValue(t, tc.src)
		res := cloneReturnsFn([]Value{v}, nil)
		_, ok := res[0].ElemConstraint()
		if ok != tc.want {
			t.Errorf("[%s] clone residual tag = %v, want %v", tc.src, ok, tc.want)
		}
	}
	// zero-arg clone stays a dynamic Any carrier.
	if z := cloneReturnsFn(nil, nil); !z[0].Parent().ConformsTo(TAny) {
		t.Errorf("zero-arg cloneReturnsFn should be dynamic(Any)")
	}
}
