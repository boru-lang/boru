package eng

import (
	"testing"

	check "github.com/boru-lang/boru/check/go"
	core "github.com/boru-lang/boru/core/go"
)

func TestValueHasSentinelCoverage(t *testing.T) {
	brk := core.NewWord("break")
	quotedBrk := brk
	core.SetQuoted(&quotedBrk, true)
	cases := []struct {
		name string
		v    core.Value
		want bool
	}{
		{"bare break word", brk, true},
		{"continue word", core.NewWord("continue"), true},
		{"return word", core.NewWord("return"), true},
		{"non-sentinel word", core.NewWord("print"), false},
		{"quoted break still detected", quotedBrk, true},
		{"list with break", core.NewList([]core.Value{core.NewInteger(1), brk, core.NewInteger(2)}), true},
		{"list without break", core.NewList([]core.Value{core.NewInteger(1), core.NewWord("print")}), false},
		{"list with quoted break", core.NewList([]core.Value{quotedBrk}), true},
		{"paren-expr with break", core.NewParenExpr([]core.Value{brk}), true},
		{"paren-expr without break", core.NewParenExpr([]core.Value{core.NewWord("print")}), false},
		{"integer (not a body word)", core.NewInteger(7), false},
		{"fn value skipped (own scope)", core.NewFunction(core.FnDefInfo{Name: "f", Signatures: []core.Signature{{Impl: core.Boru([]core.Value{brk})}}}), false},
		{"list containing a fn value", core.NewList([]core.Value{core.NewFunction(core.FnDefInfo{Signatures: []core.Signature{{Impl: core.Boru([]core.Value{brk})}}})}), false},
	}
	for _, c := range cases {
		if got := check.BodyHasSentinel(c.v); got != c.want {
			t.Errorf("%s: bodyHasSentinel = %v, want %v", c.name, got, c.want)
		}
	}
}
