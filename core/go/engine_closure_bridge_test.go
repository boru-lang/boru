package core

import (
	"errors"
	"testing"
)

// bridgeRuntime is the interpreter-only runtime plus the closure VALUE
// bridge (CompiledRuntime.ClosureAsFnDef, NUR124's payload axis): when
// armed it answers a compiled closure with a Go-handled, ANONYMOUS
// FnDefInfo over one Integer param that triples its argument — the shape
// the VM piece builds, under the closure's own identity — and otherwise
// declines.
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
	cl, _ := v.Data.(ClosurePayload)
	return NewFunctionIdentified(FnDefInfo{Signatures: []Signature{sig}, Anonymous: true}, cl.Ident), true
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
	t.Run("a bridged closure's completed collection is sealed", func(t *testing.T) {
		// The completion site arms the value seal for a compiled closure
		// as for a FnDefInfo (NUR184's two-follower row, NUR124's payload
		// axis): unsealed, the re-step re-planned forward-first over the
		// NEXT literal and walked the closure past every follower — `(2
		// (mk 1)) 10 20` islanded to `[2 10 21]` for `[2 11 20]`.
		rt := &bridgeRuntime{bridge: true, params: 1}
		out := run(rt, closure, NewInteger(7), NewInteger(8))
		if len(out) != 3 || rt.calls != 1 {
			t.Fatalf("collects the 7 only, once: %v (calls %d)", out, rt.calls)
		}
		if n, _ := AsInteger(out[1]); n != 21 {
			t.Errorf("g 7 = 21 at the closure's own position: %v", out)
		}
		if n, _ := AsInteger(out[2]); n != 8 {
			t.Errorf("the 8 stays: %v", out)
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
	t.Run("a 0-arg anonymous closure nothing calls parks as the closure itself", func(t *testing.T) {
		rt := &bridgeRuntime{bridge: true, params: 0}
		out := run(rt, closure)
		// The bridge stands in for the dispatch only: the tape keeps the
		// payload, so no handler bound to a run's context escapes into
		// the residual (Codex P2 on PR #444).
		if len(out) != 2 || rt.calls != 0 || !IsCompiledClosureValue(out[1]) || isFnDefValue(out[1]) {
			t.Errorf("parked as the closure, not the bridge: %v (calls %d)", out, rt.calls)
		}
	})
	t.Run("a no-match parks as the closure itself too", func(t *testing.T) {
		// A 2-arg closure over the one value beneath it: no signature
		// matches, the anonymous value parks — and what parks is the
		// payload, not the bridge.
		rt := &bridgeRuntime{bridge: true, params: 2}
		out := run(rt, closure)
		if len(out) != 2 || rt.calls != 0 || !IsCompiledClosureValue(out[1]) {
			t.Errorf("the closure parked over the 5, nothing applied: %v (calls %d)", out, rt.calls)
		}
	})
}

// IsCompiledClosureValue reports a ClosurePayload payload (the test-side
// twin of compiler.IsCompiledClosure, which core cannot import).
func IsCompiledClosureValue(v Value) bool {
	_, ok := v.Data.(ClosurePayload)
	return ok
}

// TestDefBoundClosureDispatchesAsWord pins NUR193's core half: a compiled
// closure a program binds by `def` is the interpreter's NAMED fn
// definition. Under a run's invoker the registry's Lookup bridges the
// def-stack payload under the binding's name (lookupUncachedBridged),
// never caching the aggregate, and the word step routes the name through
// that dispatch (dispatchesAsWord) — matching over the frame, and raising
// the interpreter's own `cannot call` over an empty one — where the
// simple-value substitution used to push the payload as data and the
// re-step parked it. Outside a run (no invoker) the payload stays data.
func TestDefBoundClosureDispatchesAsWord(t *testing.T) {
	rt := &bridgeRuntime{bridge: true, params: 1}
	prev := InstallCompiledRuntime(rt)
	defer InstallCompiledRuntime(prev)
	r := covRegistry(t, nil)
	closure := Value{Parent: TFunction, Data: ClosurePayload{}}
	r.Defs.Push("a5", closure)

	if dispatchesAsWord(closure, r) || r.Lookup("a5") != nil {
		t.Fatal("outside a run the bound closure is data: no dispatch, no lookup aggregate")
	}
	if dispatchesAsWord(closure, nil) {
		t.Error("a nil registry hosts no dispatch")
	}
	// The nil computed without an invoker must not outlive the run that
	// next sets one at the same binding generation: never cached.
	if _, cached := r.dispatchCache.get("a5", r.Defs.Gen("a5")); cached {
		t.Error("a closure-bound name's lookup is never cached")
	}

	r.Invoker = func(_ *Registry, _ Value, _ []Value) ([]Value, error) { return nil, nil }
	if !dispatchesAsWord(closure, r) {
		t.Fatal("under a run's invoker the bound closure dispatches as a word")
	}
	fn := r.Lookup("a5")
	if fn == nil || fn.Name != "a5" || fn.Anonymous || len(fn.Signatures) == 0 {
		t.Fatalf("the lookup bridges the closure under its binding's name: %+v", fn)
	}
	if _, cached := r.dispatchCache.get("a5", r.Defs.Gen("a5")); cached {
		t.Error("a bridged aggregate is never cached (it captures the run's invoker)")
	}
	rt.calls = 0
	out, err := RunResolved(r, []Value{NewInteger(5)}, []Value{NewWord("a5")})
	if n, _ := AsInteger(out[len(out)-1]); err != nil || len(out) != 1 || n != 15 || rt.calls != 1 {
		t.Errorf("the word dispatches the bridged closure over the frame's 5: %v %v (calls %d)", out, err, rt.calls)
	}
	_, err = RunResolved(r, nil, []Value{NewWord("a5")})
	var be *BoruError
	if err == nil || !errors.As(err, &be) || be.Code != "signature_error" {
		t.Errorf("over an empty frame the dispatch raises the interpreter's no-match, got %v", err)
	}
	if !FnWordBarrierOn(r, NewWord("a5")) {
		t.Error("a bridged closure's name is a fn-word collection barrier, as the interpreter's definition is")
	}
}

// TestModifierClosureShapes — the dispatch-modifier wrappers over a COMPILED
// CLOSURE (UsurpClosure, ForceStackClosure, ForceForwardClosure,
// ForceArityClosure; NUR158): each reads the closure's SHAPE through the
// compiled runtime's bridge, keeps the closure VALUE as what it wraps, and
// declines (ok=false) for anything the bridge cannot describe — a non-closure,
// a QUOTED closure, a negative arity, or no bridge at all (outside a run).
func TestModifierClosureShapes(t *testing.T) {
	r := covRegistry(t, nil)
	cl := Value{Parent: TFunction, Data: ClosurePayload{}}
	quoted := cl
	quoted.Quoted = true

	// No bridge: every wrapper declines.
	prev := InstallCompiledRuntime(noCompiledRuntime{})
	if _, ok := UsurpClosure(r, cl); ok {
		t.Error("without a bridge the closure has no shape")
	}
	InstallCompiledRuntime(prev)

	b := &bridgeRuntime{bridge: true, params: 2}
	prev = InstallCompiledRuntime(b)
	t.Cleanup(func() { InstallCompiledRuntime(prev) })

	for name, wrap := range map[string]func(Value) (Value, bool){
		"usurp":         func(v Value) (Value, bool) { return UsurpClosure(r, v) },
		"force-stack":   func(v Value) (Value, bool) { return ForceStackClosure(r, v) },
		"force-forward": func(v Value) (Value, bool) { return ForceForwardClosure(r, v) },
		"force-arity":   func(v Value) (Value, bool) { return ForceArityClosure(r, v, 1) },
	} {
		w, ok := wrap(cl)
		if !ok || !w.Parent.Equal(TFunction) {
			t.Errorf("%s over a bridged closure must wrap it: ok=%v %v", name, ok, w)
		}
		if _, ok := wrap(NewInteger(1)); ok {
			t.Errorf("%s over a non-closure must decline", name)
		}
		if _, ok := wrap(quoted); ok {
			t.Errorf("%s over a quoted closure must decline", name)
		}
	}
	if _, ok := ForceArityClosure(r, cl, -1); ok {
		t.Error("a negative arity must decline")
	}
	// A bridge that declines leaves the closure shapeless.
	b.bridge = false
	if _, ok := ForceStackClosure(r, cl); ok {
		t.Error("a declining bridge leaves no shape")
	}
}
