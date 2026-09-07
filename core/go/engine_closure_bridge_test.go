package core

import "testing"

// bridgeRuntime is the interpreter-only runtime plus the closure VALUE
// bridge (CompiledRuntime.ClosureAsFnDef, NUR124's payload axis): when
// armed it answers a compiled closure with a Go-handled, ANONYMOUS
// FnDefInfo over one Integer param that triples its argument — the shape
// the VM piece builds — and otherwise declines.
type bridgeRuntime struct {
	noCompiledRuntime
	bridge bool
	params int
	calls  int
}

func (b *bridgeRuntime) ClosureAsFnDef(_ *Registry, v Value) (Value, bool) {
	if _, ok := v.Data.(ClosurePayload); !ok || !b.bridge {
		return v, false
	}
	params := make([]FnParam, b.params)
	for i := range params {
		params[i] = FnParam{Type: TInteger}
	}
	sig := Signature{Params: params, BarrierPos: len(params), Impl: Go(func(a []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		b.calls++
		if len(a) == 0 {
			return []Value{NewInteger(42)}, nil
		}
		n, _ := AsInteger(a[0])
		return []Value{NewInteger(n * 3)}, nil
	})}
	NormalizeSig(&sig)
	return NewFunction(FnDefInfo{Signatures: []Signature{sig}, Anonymous: true}), true
}

// TestClosureValueBridge pins the interpreter's side of the bridge: a
// compiled closure stepped at the pointer dispatches through the FnDefInfo
// the runtime hands back — over the stack beneath it, keeping its Go
// handler although the fn is Anonymous (compileFnDef's body-runner is for
// boru bodies) — while a declined bridge, a quoted closure and a parked
// 0-arg anonymous one stay data.
func TestClosureValueBridge(t *testing.T) {
	closure := Value{Parent: TFunction, Data: ClosurePayload{}}
	run := func(rt *bridgeRuntime, tokens ...Value) []Value {
		t.Helper()
		prev := InstallCompiledRuntime(rt)
		defer InstallCompiledRuntime(prev)
		r := covRegistry(t, nil)
		out, err := RunResolved(r, []Value{NewInteger(5)}, tokens)
		if err != nil {
			t.Fatalf("run: %v", err)
		}
		return out
	}
	t.Run("a bridged closure dispatches over the stack", func(t *testing.T) {
		rt := &bridgeRuntime{bridge: true, params: 1}
		out := run(rt, closure)
		if n, _ := AsInteger(out[len(out)-1]); len(out) != 1 || n != 15 || rt.calls != 1 {
			t.Errorf("the closure applied to the 5 beneath it: %v (calls %d)", out, rt.calls)
		}
	})
	t.Run("a bridged closure collects forward like a fn value", func(t *testing.T) {
		rt := &bridgeRuntime{bridge: true, params: 1}
		out := run(rt, closure, NewInteger(7))
		if len(out) != 2 || rt.calls != 1 {
			t.Fatalf("collects the written 7 and leaves the 5: %v", out)
		}
		if n, _ := AsInteger(out[1]); n != 21 {
			t.Errorf("g 7 = 21: %v", out)
		}
	})
	t.Run("a declined bridge leaves the closure as data", func(t *testing.T) {
		out := run(&bridgeRuntime{}, closure)
		if len(out) != 2 || !IsCompiledClosureValue(out[1]) {
			t.Errorf("data: %v", out)
		}
	})
	t.Run("a quoted closure is data on both lanes", func(t *testing.T) {
		quoted := closure
		quoted.Quoted = true
		rt := &bridgeRuntime{bridge: true, params: 1}
		if out := run(rt, quoted); len(out) != 2 || rt.calls != 0 {
			t.Errorf("quoted: %v", out)
		}
	})
	t.Run("a 0-arg anonymous closure nothing calls parks", func(t *testing.T) {
		rt := &bridgeRuntime{bridge: true, params: 0}
		out := run(rt, closure)
		if len(out) != 2 || rt.calls != 0 || !isFnDefValue(out[1]) {
			t.Errorf("parked as the bridged value: %v (calls %d)", out, rt.calls)
		}
	})
}

// IsCompiledClosureValue reports a ClosurePayload payload (the test-side
// twin of compiler.IsCompiledClosure, which core cannot import).
func IsCompiledClosureValue(v Value) bool {
	_, ok := v.Data.(ClosurePayload)
	return ok
}
