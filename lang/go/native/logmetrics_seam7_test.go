package native

import (
	"fmt"
	"testing"
)

// Seam-7 coverage for log_metrics.go (design/TEST-SEAMS.10.md). The
// sub-registry init-error arms are driven by swapping the shared
// newSubRegistry seam.

// b3FailSubReg swaps newSubRegistry to always error, restoring on cleanup.
func b3FailSubReg(t *testing.T) {
	t.Helper()
	saved := newSubRegistry
	t.Cleanup(func() { newSubRegistry = saved })
	newSubRegistry = func(...func(*Registry)) (*Registry, error) { return nil, fmt.Errorf("boom-subreg") }
}

func TestB3AttrsOrEmptyNonMap(t *testing.T) {
	// Non-map attributes -> fresh empty map.
	got := attrsOrEmpty(NewInteger(1))
	if !got.Parent().ConformsTo(TMap) {
		t.Fatalf("attrsOrEmpty(non-map) must be a Map, got %v", got)
	}
	if m, _ := AsMap(got); m == nil || m.Len() != 0 {
		t.Fatalf("attrsOrEmpty(non-map) must be empty")
	}
	// Positive pair: a concrete map passes through.
	om := NewOrderedMap()
	om.Set("a", NewInteger(1))
	if got := attrsOrEmpty(NewMap(om)); func() int { m, _ := AsMap(got); return m.Len() }() != 1 {
		t.Fatalf("attrsOrEmpty(map) must pass through")
	}
}

func TestB3BuildInstrumentSubRegError(t *testing.T) {
	b3FailSubReg(t)
	if _, err := buildInstrumentInstance(&instrumentState{kind: "counter"}); err == nil {
		t.Fatal("buildInstrumentInstance: expected sub-registry init error")
	}
}

func TestB3InstrumentCtorSubRegError(t *testing.T) {
	r := seam5Reg(t)
	b3FailSubReg(t)
	lsr := &LogSinkRegistry{}
	n := instrumentCtor("log-counter", "counter", lsr)
	// Check-mode return shape falls back to a Map carrier on init error.
	shape := n.Signatures[0].ReturnsFn
	got := shape([]Value{NewString("x")}, r)
	if len(got) != 1 || !got[0].Parent().ConformsTo(TMap) {
		t.Fatalf("instrument shape on init error must be a Map carrier, got %v", got)
	}
	// Impl surfaces the init error as a boru error.
	g, ok := n.Signatures[0].Impl.(*GoImpl)
	if !ok {
		t.Fatal("expected GoImpl")
	}
	if _, err := g.Handler([]Value{NewString("x")}, nil, nil, r); err == nil {
		t.Fatal("instrument ctor Impl: expected init error")
	}
}
