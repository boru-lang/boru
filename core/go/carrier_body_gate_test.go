package core

// Gate test for the payload-less body guard in runCarrierBodyDefsAdds
// (carrier.go:2584 — `if body.Data == nil`).
//
// Every branch-aware word (`if`, loops, quotation analysis) hands its body
// slot straight to one of the four RunCarrierBody* entries, and in check mode
// that slot is not guaranteed to be a concrete list: when the body position
// holds a type-only value the payload is absent entirely (Value.Data == nil)
// rather than present-but-wrong-shape. That is a DIFFERENT state from the
// AsList guard two lines below, which fires for a value that HAS a payload of
// the wrong kind — e.g. NewCarrier(TList), whose Data is a non-nil
// ChildTypeInfo. So the inputs below must be payload-less to enter this arm:
//
//   - NewCarrier(TInteger) — a non-container carrier; NewCarrier
//     only fills Data for the List/Map element-type carriers, so Data is nil.
//   - Value{} — the zero Value a caller propagates when an upstream
//     analysis produced nothing at all.
//
// Both must be declined with (nil, nil) BEFORE the sub-engine is built, and
// without touching the registry (no diagnostics, no def-stack churn).

import (
	"testing"
)

// TestGatecarrier2584PayloadlessBodyFailedToCompile drives the Data == nil arm through
// every RunCarrierBody* entry point that funnels into runCarrierBodyDefsAdds.
func TestGatecarrier2584PayloadlessBodyFailedToCompile(t *testing.T) {
	bodies := map[string]Value{
		"non-container carrier": NewCarrier(TInteger),
		"zero value":            {},
	}
	for name, body := range bodies {
		if body.Data != nil {
			t.Fatalf("%s: precondition failed, body must be payload-less, got %#v", name, body.Data)
		}
		t.Run(name, func(t *testing.T) {
			r := w8reg(t)
			defer r.Check.Begin()()

			// keep=false, condFrag=false
			stk, adds := RunCarrierBodyWithDefs(r, body)
			if stk != nil || adds != nil {
				t.Errorf("RunCarrierBodyWithDefs: got (%v, %v), want (nil, nil)", stk, adds)
			}
			// keep=false, condFrag=true
			stk, adds = RunCarrierCondBody(r, body)
			if stk != nil || adds != nil {
				t.Errorf("RunCarrierCondBody: got (%v, %v), want (nil, nil)", stk, adds)
			}
			// keep=true, condFrag=true
			if stk = RunCarrierCondBodyKeepDefs(r, body); stk != nil {
				t.Errorf("RunCarrierCondBodyKeepDefs: got %v, want nil", stk)
			}
			// keep=true
			if stk = RunCarrierBodyKeepDefs(r, body); stk != nil {
				t.Errorf("RunCarrierBodyKeepDefs: got %v, want nil", stk)
			}
			// the plain entry
			if stk = RunCarrierBody(r, body); stk != nil {
				t.Errorf("RunCarrierBody: got %v, want nil", stk)
			}

			// The guard returns before any analysis state is disturbed.
			if len(r.Check.Diagnostics) != 0 {
				t.Errorf("payload-less body must not diagnose, got %v", r.Check.Diagnostics)
			}
			if r.Check.NestedBodyDepth != 0 || r.Check.CondBodyDepth != 0 {
				t.Errorf("body depths must be untouched, got nested=%d cond=%d",
					r.Check.NestedBodyDepth, r.Check.CondBodyDepth)
			}
		})
	}
}

// TestGatecarrier2584ListCarrierTakesTheAsListArm pins the boundary: a List
// carrier DOES carry a payload, so it slips past the 2584 guard and is declined
// one guard later by AsList. Without this the two arms are easy to conflate.
func TestGatecarrier2584ListCarrierTakesTheAsListArm(t *testing.T) {
	body := NewCarrier(TList)
	if body.Data == nil {
		t.Fatalf("a List carrier is expected to carry a ChildTypeInfo payload")
	}
	r := w8reg(t)
	defer r.Check.Begin()()
	if stk, adds := RunCarrierBodyWithDefs(r, body); stk != nil || adds != nil {
		t.Errorf("List carrier: got (%v, %v), want (nil, nil)", stk, adds)
	}
}
