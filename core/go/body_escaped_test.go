package core

import "testing"

// TestBodyEscaped: the predicate reads the registry's flow flag — clear, or
// a break/continue an iterating native must end its iteration on.
func TestBodyEscaped(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if BodyEscaped(r) {
		t.Fatal("a clear flag is no escape")
	}
	r.FlowCtrl = FlowBreak
	if !BodyEscaped(r) {
		t.Fatal("a pending break is an escape")
	}
	r.FlowCtrl = FlowContinue
	if !BodyEscaped(r) {
		t.Fatal("a pending continue is an escape")
	}
	r.FlowCtrl = FlowNone
}
