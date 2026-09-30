package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// TestBodyEscapes pins the loop-region gate of the S5 first-value def bind
// (Codex on #526): a loop body whose iteration a break or continue may cut
// short has a runtime-sized region, so its static count must not size the
// splice. Every event that may raise the signal counts, at any depth.
func TestBodyEscapes(t *testing.T) {
	frag := func(evs ...EmitEvent) *EmitFragment { return &EmitFragment{events: evs} }
	plain := EmitEvent{kind: evCall, call: emitCall{word: "add", sig: &core.Signature{}}}
	quiet := &fnUnitRec{finished: true, frag: frag(plain)}
	breaks := &fnUnitRec{finished: true, frag: frag(EmitEvent{kind: evBreak})}
	es := NewEmitState()
	es.fnRecs = []*fnUnitRec{quiet, breaks, {}}
	call := func(unit int) EmitEvent { return EmitEvent{kind: evCallUser, uc: emitUserCall{unit: unit}} }
	for why, tc := range map[string]struct {
		body *EmitFragment
		want bool
	}{
		"no body":                 {nil, false},
		"a quiet body":            {frag(plain, call(0), call(0)), false},
		"a break":                 {frag(EmitEvent{kind: evBreak}), true},
		"a continue in an arm":    {frag(EmitEvent{kind: evBranch, br: &emitBranch{then: frag(EmitEvent{kind: evContinue})}}), true},
		"an island":               {frag(EmitEvent{kind: evFallback}), true},
		"a poly native":           {frag(EmitEvent{kind: evCall, call: emitCall{poly: true}}), true},
		"a native running a body": {frag(EmitEvent{kind: evCall, call: emitCall{sig: &core.Signature{Callable: &core.CallableSpec{}}}}), true},
		"a unit that breaks":      {frag(call(1)), true},
		"a unit still open":       {frag(call(2)), true},
		"a unit the table lacks":  {frag(call(9)), true},
		"a unit-less call":        {frag(call(-1)), true},
		"a poly user call":        {frag(EmitEvent{kind: evCallUser, uc: emitUserCall{unit: 0, poly: &emitUserPolySpec{}}}), true},
	} {
		if got := es.bodyEscapes(tc.body); got != tc.want {
			t.Errorf("%s: bodyEscapes = %v, want %v", why, got, tc.want)
		}
	}
}
