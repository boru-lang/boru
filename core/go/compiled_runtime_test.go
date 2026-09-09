package core

import "testing"

// TestNoCompiledRuntimeDeclines pins the core default's arms (the
// interpreter-only build the piece cut produces): every operation
// declines, and InstallCompiledRuntime restores cleanly.
func TestNoCompiledRuntimeDeclines(t *testing.T) {
	prev := InstallCompiledRuntime(noCompiledRuntime{})
	defer InstallCompiledRuntime(prev)

	if res, err, ran := compiledRuntime.InvokeCompiled(nil, nil, nil); res != nil || err != nil || ran {
		t.Fatalf("no-op InvokeCompiled = %v %v %v", res, err, ran)
	}
	compiledRuntime.StampDetached(nil, FnDefInfo{}, SrcPos{})
	closure := Value{Parent: TFunction, Data: ClosurePayload{}}
	if fnv, ok := compiledRuntime.ClosureAsFnDef(nil, closure); ok || !fnv.Parent.Equal(TFunction) {
		t.Fatalf("no-op ClosureAsFnDef must decline with the value: %v %v", fnv, ok)
	}

	// A callback dispatch under the declined runtime takes CallBoru —
	// the standalone-core semantics.
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.InitRootContext()
	sig := &FnSig{
		Params:  []FnParam{{Name: "x", Type: TInteger}},
		Returns: []*Type{TInteger},
		Impl:    Boru([]Value{NewWord("x")}),
	}
	out, err := InvokeCallback(r, sig, []Value{NewInteger(7)}, nil)
	if err != nil || len(out) != 1 {
		t.Fatalf("InvokeCallback under declined runtime: %v %v", out, err)
	}
}
