package check

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// TestShortWindowNeverReachesReturnsFn pins NUR332's root fix. The failed-
// dispatch recovery can assume an overload the site cannot fill; a window
// shorter than the signature must never reach its ReturnsFn (which reads its
// operands positionally — `if`'s read args[0] of nothing and panicked). The
// result is the declared Returns, else one dynamic Any, with no
// missing_returns report; a full window still runs the ReturnsFn.
func TestShortWindowNeverReachesReturnsFn(t *testing.T) {
	r := newTestRegistry(t)
	called := 0
	rf := func(args []core.Value, _ *core.Registry) []core.Value {
		called++
		return []core.Value{args[0], args[1]}
	}
	two := []*core.Type{core.TAny, core.TAny}
	pos := core.SrcPos{Row: 1, Col: 3}

	// No declared Returns: one dynamic Any.
	sig := &core.Signature{Args: two, Impl: covCDImpl(), ReturnsFn: rf}
	out := declaredReturnCarriers(r, "w", sig, []core.Value{core.NewInteger(1)}, pos)
	if called != 0 {
		t.Fatalf("a short window reached the ReturnsFn (%d calls)", called)
	}
	if len(out) != 1 || !out[0].Dynamic || !out[0].Parent.Equal(core.TAny) {
		t.Errorf("short window, no Returns = %#v, want one dynamic Any", out)
	}
	if len(r.Check.Diagnostics) != 0 {
		t.Errorf("a short window adds no diagnostic, got %v", covCDDiagCodes(r))
	}

	// Declared Returns: one carrier per type, a declared Any riding dynamic.
	decl := &core.Signature{Args: two, Impl: covCDImpl(), ReturnsFn: rf, Returns: []*core.Type{core.TAny, core.TInteger}}
	out = declaredReturnCarriers(r, "w", decl, nil, pos)
	if called != 0 || len(out) != 2 || !out[0].Dynamic || out[1].Dynamic || !out[1].Parent.Equal(core.TInteger) {
		t.Errorf("short window, declared [Any Integer] = %#v (calls %d)", out, called)
	}

	// An empty Returns declares "returns nothing".
	none := &core.Signature{Args: two, Impl: covCDImpl(), ReturnsFn: rf, Returns: []*core.Type{}}
	if out = declaredReturnCarriers(r, "w", none, nil, pos); called != 0 || len(out) != 0 {
		t.Errorf("short window, empty Returns = %#v (calls %d), want nothing", out, called)
	}

	// The full window runs the ReturnsFn.
	out = declaredReturnCarriers(r, "w", sig, []core.Value{core.NewInteger(1), core.NewInteger(2)}, pos)
	if called != 1 || len(out) != 2 {
		t.Errorf("full window = %#v (calls %d), want the ReturnsFn's two results", out, called)
	}
}
