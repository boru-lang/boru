package eng

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// TestRegisterNativeFuncCopiesCallable pins the §4.5 decoupling contract: a
// code-body higher-order word DECLARES its closure shape on its
// NativeFunc.Callable, and RegisterNativeFunc copies that pointer onto EVERY
// one of the word's signatures so the bytecode recorder reads it back via the
// resolved sig.Callable — eng holds no name-keyed callable table. The negative
// half proves an ordinary word's signatures carry no spec, so the recorder
// declines a fn-valued body for words that do not opt in.
func TestRegisterNativeFuncCopiesCallable(t *testing.T) {
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	spec := &core.CallableSpec{BodyPos: 1, BodyOut: 0, Inputs: func(_ []core.Value) []core.Value { return nil }}
	r.RegisterNativeFunc(core.NativeFunc{
		Name:     "probe-callable",
		Callable: spec,
		Signatures: []core.Signature{
			{Args: []*core.Type{core.TList}, BarrierPos: -1},
			{Args: []*core.Type{core.TList, core.TMap}, BarrierPos: -1},
		},
	})
	if err := r.Err(); err != nil {
		t.Fatalf("register probe-callable: %v", err)
	}

	fn := r.Lookup("probe-callable")
	if fn == nil {
		t.Fatal("probe-callable did not register")
	}
	own := 0
	for i := range fn.Signatures {
		if fn.Signatures[i].Fallback {
			continue // synthetic aggregate fallback carries no spec; skip it
		}
		own++
		got := fn.Signatures[i].Callable
		if got != spec {
			t.Errorf("sig %d: Callable = %p, want the one declared spec %p on every sig", i, got, spec)
			continue
		}
		if got.BodyPos != 1 || got.BodyOut != 0 {
			t.Errorf("sig %d: spec fields not carried: %+v", i, *got)
		}
	}
	if own != 2 {
		t.Fatalf("expected 2 own signatures to carry the spec, saw %d", own)
	}

	// NEGATIVE: an ordinary word (no Callable declared) carries no spec on any
	// signature — the recorder then declines a fn-valued body at this word.
	r.RegisterNativeFunc(core.NativeFunc{
		Name:       "probe-plain",
		Signatures: []core.Signature{{Args: []*core.Type{core.TList}, BarrierPos: -1}},
	})
	plain := r.Lookup("probe-plain")
	if plain == nil {
		t.Fatal("probe-plain did not register")
	}
	for i := range plain.Signatures {
		if !plain.Signatures[i].Fallback && plain.Signatures[i].Callable != nil {
			t.Errorf("ordinary word sig %d unexpectedly carries a Callable spec", i)
		}
	}
}
