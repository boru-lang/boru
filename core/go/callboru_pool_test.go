package core

import "testing"

// CallBoru runs its body on a POOLED sub-engine (registry.go callBoruNamed):
// the engine is parked after the call with every per-call setting cleared,
// the next call reuses it, results never alias the pooled tape, and a
// nested call acquires a distinct engine.

func callBoruPoolSig(body ...Value) *Signature {
	return &Signature{
		Params:     []FnParam{{Name: "a", Type: TAny}},
		Returns:    []*Type{TInteger},
		Impl:       Boru(body),
		BarrierPos: 0,
	}
}

func TestCallBoruParksAndReusesPooledEngine(t *testing.T) {
	r := poolTestRegistry(t)
	sig := callBoruPoolSig(NewInteger(7))
	if _, err := r.CallBoru(sig, []Value{NewInteger(1)}, nil); err != nil {
		t.Fatal(err)
	}
	if len(r.enginePool) != 1 {
		t.Fatalf("pool size after CallBoru = %d, want 1", len(r.enginePool))
	}
	parked := r.enginePool[0]
	if parked.IsTop || parked.StartAt != 0 || parked.debugLabel != "" || parked.DeferResidual {
		t.Errorf("parked engine kept per-call settings: top=%v start=%d label=%q defer=%v",
			parked.IsTop, parked.StartAt, parked.debugLabel, parked.DeferResidual)
	}
	if parked.stepLimit != StepLimitFor(r, DefaultSubStepLimit) {
		t.Errorf("parked engine kept the top-level step limit %d", parked.stepLimit)
	}
	if _, err := r.CallBoruNamed(sig, []Value{NewInteger(2)}, nil, "second"); err != nil {
		t.Fatal(err)
	}
	if len(r.enginePool) != 1 || r.enginePool[0] != parked {
		t.Error("second CallBoru did not reuse the parked engine")
	}
}

func TestCallBoruResultsDoNotAliasPooledTape(t *testing.T) {
	r := poolTestRegistry(t)
	first, err := r.CallBoru(callBoruPoolSig(NewInteger(10)), []Value{NewInteger(1)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	// The next call reloads the same tape; a result aliasing the buffer
	// would now read 77.
	if _, err := r.CallBoru(callBoruPoolSig(NewInteger(77)), []Value{NewInteger(1)}, nil); err != nil {
		t.Fatal(err)
	}
	if n, _ := AsInteger(first[0]); n != 10 {
		t.Fatalf("first result clobbered by the pooled reuse: got %d, want 10", n)
	}
}

func TestCallBoruNestedAcquiresDistinctEngines(t *testing.T) {
	r := poolTestRegistry(t)
	var outer, inner *Engine
	r.Register("callboru-nest", Signature{
		Args:    []*Type{TInteger},
		Returns: []*Type{TInteger},
		Impl: Go(func(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
			outer = reg.debugEngines[len(reg.debugEngines)-1]
			res, err := reg.CallBoru(callBoruPoolSig(NewInteger(41)), []Value{NewInteger(0)}, nil)
			if err != nil {
				return nil, err
			}
			inner = reg.enginePool[len(reg.enginePool)-1]
			n, _ := AsInteger(args[0])
			m, _ := AsInteger(res[0])
			return []Value{NewInteger(n + m)}, nil
		}),
	})
	res, err := r.CallBoru(callBoruPoolSig(NewInteger(1), NewWord("callboru-nest")), []Value{NewInteger(0)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := AsInteger(res[0]); n != 42 {
		t.Fatalf("nested CallBoru result = %d, want 42", n)
	}
	if outer == nil || inner == nil || outer == inner {
		t.Fatalf("nested CallBoru must run on a distinct engine (outer=%p inner=%p)", outer, inner)
	}
	if len(r.enginePool) != 2 {
		t.Errorf("pool size after the nested calls = %d, want 2", len(r.enginePool))
	}
}

func TestCallBoruErrorStillParksEngine(t *testing.T) {
	r := poolTestRegistry(t)
	if _, err := r.CallBoru(callBoruPoolSig(NewWord("no-such-word-xyz")), []Value{NewInteger(1)}, nil); err == nil {
		t.Fatal("an undefined word in the body must error")
	}
	if len(r.enginePool) != 1 {
		t.Fatalf("pool size after an erroring call = %d, want 1 (the engine must be released)", len(r.enginePool))
	}
	res, err := r.CallBoru(callBoruPoolSig(NewInteger(9)), []Value{NewInteger(1)}, nil)
	if err != nil {
		t.Fatalf("call after the error: %v", err)
	}
	if n, _ := AsInteger(res[0]); n != 9 {
		t.Fatalf("post-error call = %d, want 9", n)
	}
}
