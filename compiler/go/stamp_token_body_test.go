package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// TestStampTokenBodyGuards: the run-time token-body stamp declines where the
// lazy fn-value stamp declines — stamping not armed on the registry, an
// empty body, a body that mutates the registry when run (an import, a
// capitalised def) — and a nil input type reads as Any.
func TestStampTokenBodyGuards(t *testing.T) {
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	one := []core.Value{core.NewInteger(1)}
	if ref, ok := StampTokenBody(r, one, nil, core.SrcPos{}); ok || ref != nil {
		t.Fatal("stamping not armed must decline")
	}
	if ref, ok := StampTokenBody(nil, one, nil, core.SrcPos{}); ok || ref != nil {
		t.Fatal("a nil registry must decline")
	}
	r.EnableRuntimeStamping()
	if ref, ok := StampTokenBody(r, nil, nil, core.SrcPos{}); ok || ref != nil {
		t.Fatal("an empty body must decline")
	}
	hazard := []core.Value{core.NewWord("import"), core.NewString("boru:test")}
	if ref, ok := StampTokenBody(r, hazard, []*core.Type{core.TInteger}, core.SrcPos{}); ok || ref != nil {
		t.Fatal("a body with a replay hazard must decline")
	}
	// A nil input type declares Any, and the body stamps even on this bare
	// registry (no word library): its residual is the untouched Any input
	// beneath the literal, which the unapplied-fn gate used to read as a
	// dynamic value that might auto-apply (the stamp declined here until
	// NUR202's close exempted a unit's own untouched inputs — an argument
	// enters the frame resolved and is never stepped). The typed and
	// untyped inputs over a real registry are the lang pins'
	// (runtime_token_body_test.go).
	if ref, ok := StampTokenBody(r, one, []*core.Type{nil}, core.SrcPos{}); !ok || ref == nil {
		t.Fatalf("a nil input type declares Any and the literal body stamps: ok=%v ref=%v", ok, ref)
	}
}
