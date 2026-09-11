package core

import "testing"

// TestUncalledDispatchDefiniteArms pins the screen the forty-ninth
// increment's trap rides on, arm by arm: it admits a plain concrete const
// and declines every operand whose runtime value the check pass does not
// hold exactly. Each decline is what keeps the baked error honest — the trap
// serialises the error built HERE, so anything that could resolve
// differently at run time must keep the whole-program refusal.
func TestUncalledDispatchDefiniteArms(t *testing.T) {
	word := NewWord("zs")
	dyn := NewDynamicCarrier(TString)
	carrier := NewCarrier(TString)
	undef := NewString("x")
	undef.Undefined = true

	for _, tc := range []struct {
		name string
		in   []Value
		want bool
	}{
		{"no candidates at all", nil, true},
		{"concrete consts", []Value{NewString("x"), NewInteger(3)}, true},
		{"a carrier", []Value{NewString("x"), carrier}, false},
		{"a dynamic", []Value{dyn}, false},
		{"an undefined placeholder", []Value{undef}, false},
		{"a raw word token", []Value{word}, false},
		{"an open paren", []Value{NewOpenParen()}, false},
		{"an unexpanded paren expr", []Value{NewParenExpr([]Value{NewInteger(1)})}, false},
		{"a reach path", []Value{NewReachFromKeys(NewWord("zs"), []Value{NewString("n")})}, false},
		{"a template string", []Value{NewInterpString([]InterpPart{{Lit: "x"}})}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := uncalledDispatchDefinite(tc.in); got != tc.want {
				t.Fatalf("uncalledDispatchDefinite = %v, want %v", got, tc.want)
			}
		})
	}
}
