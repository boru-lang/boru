package core

import "testing"

// TestUnwrapModifierChain pins the walk a tape-less consumer uses to perform a
// modifier wrapper's re-dispatch itself instead of stepping the tokens its
// handler returns.
//
// The permutation is the whole point, and it is not obvious from the
// handlers: applying the argument-order rule to each window collapses both to
// a mapping that does NOT depend on the barrier — `usurp` reverses the arg
// vector, the rebarrier family leaves it alone. Everything here is a
// consequence of that.
func TestUnwrapModifierChain(t *testing.T) {
	// Built here rather than looked up: core carries no word library, so a
	// bare registry has no `sub` to wrap. Two params is the smallest shape
	// where a reversal is observable at all.
	sig := Signature{
		Params:     []FnParam{{Name: "a", Type: TInteger}, {Name: "b", Type: TInteger}},
		BarrierPos: 1,
		Impl: Go(func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			return args, nil
		}),
	}
	NormalizeSig(&sig)
	fn := NewFunction(FnDefInfo{Name: "sub", Signatures: []Signature{sig}, MaxForwardArgs: 1})

	t.Run("a plain fn is not a wrapper", func(t *testing.T) {
		got, reverse, ok := UnwrapModifierChain(fn)
		if ok {
			t.Error("a plain fn value must not report as wrapped")
		}
		if reverse {
			t.Error("a plain fn value permutes nothing")
		}
		if _, isFn := got.Data().(*FnDefInfo); !isFn {
			t.Error("the value must come back unchanged")
		}
	})

	t.Run("a non-fn value is not a wrapper", func(t *testing.T) {
		if _, _, ok := UnwrapModifierChain(NewInteger(5)); ok {
			t.Error("an integer is not a wrapper")
		}
	})

	t.Run("usurp reverses", func(t *testing.T) {
		u, ok := UsurpFunction(fn)
		if !ok {
			t.Fatal("UsurpFunction declined a fn value")
		}
		got, reverse, wrapped := UnwrapModifierChain(u)
		if !wrapped || !reverse {
			t.Fatalf("usurp must unwrap and reverse, got wrapped=%v reverse=%v", wrapped, reverse)
		}
		if fd, isFn := got.Data().(*FnDefInfo); !isFn || fd.Name != "sub" {
			t.Errorf("must unwrap to sub, got %v", got)
		}
	})

	t.Run("the rebarrier family does not", func(t *testing.T) {
		for _, c := range []struct {
			name string
			make func(Value) (Value, bool)
		}{
			{"forward-args", ForceForwardFunction},
			{"stack-args", ForceStackFunction},
			{"force-arity", func(v Value) (Value, bool) { return ForceArityFunction(v, 2) }},
		} {
			w, ok := c.make(fn)
			if !ok {
				t.Fatalf("%s declined a fn value", c.name)
			}
			got, reverse, wrapped := UnwrapModifierChain(w)
			if !wrapped {
				t.Errorf("%s must unwrap", c.name)
			}
			if reverse {
				t.Errorf("%s re-bases the barrier only — it must not reverse", c.name)
			}
			if fd, isFn := got.Data().(*FnDefInfo); !isFn || fd.Name != "sub" {
				t.Errorf("%s must unwrap to sub, got %v", c.name, got)
			}
		}
	})

	// THE CASE ArgsReversed CANNOT ANSWER, and the reason this walks a chain
	// rather than reading a flag. UsurpFunction SETS the mark true and the
	// others propagate it, so a doubled usurp still reports reversed — safe
	// where the mark only declines a fast path, wrong where a consumer acts
	// on it. Parity is a property of the walk alone.
	t.Run("two usurps cancel, where the mark does not", func(t *testing.T) {
		u1, _ := UsurpFunction(fn)
		u2, ok := UsurpFunction(u1)
		if !ok {
			t.Fatal("usurp of a usurp declined")
		}
		if fd, _ := FnDefOf(u2.Data()); !fd.ArgsReversed {
			t.Fatal("precondition: the mark is expected to still read reversed")
		}
		got, reverse, wrapped := UnwrapModifierChain(u2)
		if !wrapped {
			t.Fatal("must unwrap")
		}
		if reverse {
			t.Error("two reversals compose to the identity — the walk must say so " +
				"even though ArgsReversed still reads true")
		}
		if fd, isFn := got.Data().(*FnDefInfo); !isFn || fd.Name != "sub" {
			t.Errorf("must unwrap all the way to sub, got %v", got)
		}
	})

	// Composition across kinds: only the usurps count.
	t.Run("a rebarrier over a usurp still reverses", func(t *testing.T) {
		u, _ := UsurpFunction(fn)
		w, ok := ForceArityFunction(u, 2)
		if !ok {
			t.Fatal("force-arity declined")
		}
		_, reverse, wrapped := UnwrapModifierChain(w)
		if !wrapped || !reverse {
			t.Errorf("force-arity over usurp must reverse, got wrapped=%v reverse=%v", wrapped, reverse)
		}
	})

	// The depth bound is unreachable by construction — each wrapper captures
	// an ALREADY-BUILT value, so a chain is a finite tree path and cannot
	// cycle. It is here so a malformed value costs a declined fast path
	// rather than a runaway walk inside the VM's dispatch loop.
	t.Run("a cyclic chain declines rather than spinning", func(t *testing.T) {
		var self Value
		self = NewFunction(FnDefInfo{Name: "loop", Wrap: WrapRebarrier, Wraps: &self})
		got, reverse, ok := UnwrapModifierChain(self)
		if ok || reverse {
			t.Error("a chain past the bound must decline, not report a result")
		}
		if fd, isFn := got.Data().(*FnDefInfo); !isFn || fd.Name != "loop" {
			t.Error("a declined walk returns the value it was given")
		}
	})
}
