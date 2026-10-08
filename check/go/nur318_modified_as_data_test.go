package check

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// TestModifiedAsData pins NUR318's marker test: a dispatch modifier right
// after the carrier, or after the close of the group it stands alone in,
// says data; anything else, or a close with no lone group before it, does
// not.
func TestModifiedAsData(t *testing.T) {
	carrier := core.NewDynamicCarrier(core.TAny)
	dm := core.NewDispatchMod(core.DispatchModInfo{Val: true})
	op, cp, five := core.NewOpenParen(), core.NewCloseParen(), core.NewInteger(5)
	for _, c := range []struct {
		why  string
		tape []core.Value
		at   int
		want bool
	}{
		{"a marker after the carrier", []core.Value{carrier, dm}, 0, true},
		{"a marker after its lone group", []core.Value{op, carrier, cp, dm}, 1, true},
		{"a value after the carrier", []core.Value{carrier, five}, 0, false},
		{"the tape's end", []core.Value{carrier}, 0, false},
		{"a close with no group before it", []core.Value{five, carrier, cp, dm}, 1, false},
		{"a lone group with no marker", []core.Value{op, carrier, cp}, 1, false},
	} {
		e, _, fin := mayBeFnEngine(t, c.tape, nil)
		if got := modifiedAsData(e, c.at); got != c.want {
			t.Errorf("%s: modifiedAsData = %v, want %v", c.why, got, c.want)
		}
		fin()
	}
}
