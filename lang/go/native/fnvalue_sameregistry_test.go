package native

import "testing"

// Pins the execFnDefSig same-registry flip (design/SUB-ENGINE-MAIN-TAPE-
// REVIEW.0.md R2, the TCO-STAGED Stage-5 residual): a Function VALUE whose
// FnDefInfo carries a captured Registry equal to the executing engine's
// registry dispatches through the main-tape SPLICE branch, not a CallBoru
// sub-engine.
//
// Reaching that branch requires VALUE dispatch — an anonymous FnDef
// (empty Name, so the registered-handler lookup cannot claim it) stamped
// with the executing registry, exactly the shape resolveModuleExport
// produces for an anonymous fn in an export map (`{mk: (fn […])}`)
// applied back inside its own module. Named bindings never reach it: they
// dispatch through the InstallFnDef body-runner handler, which already
// splices. An instrumented sweep confirmed the branch activations:
// same-registry fires for the anonymous-export shape and nowhere else in
// the suite.
//
// The flip is observable-invariant — every probe (result, def cleanup,
// raise, capture, args, break/continue) behaves identically pre/post —
// so these tests pin the INVARIANTS the splice branch must keep, plus the
// cross-registry negative that must stay on CallBoru. The boru-source twin
// rows live in lang/spec/module-fnvalue-boundary.tsv.

// sameRegistryAnonFnValue defines `name` via boru source, captures its
// FnDefInfo, strips the name (anonymous value dispatch) and stamps
// Registry = r — the module-export shape — then removes the def binding.
func sameRegistryAnonFnValue(t *testing.T, r *Registry, name string, fnBody Value) Value {
	t.Helper()
	_ = runBoru(t, r, []Value{NewWord("def"), NewWord(name), NewWord("fn"), fnBody, NewEnd()})
	fnVal, ok := r.Defs.Top(name)
	if !ok {
		t.Fatalf("%s not defined", name)
	}
	fd, ok := fnVal.Data.(FnDefInfo)
	if !ok {
		t.Fatalf("%s is not a FnDefInfo (got %T)", name, fnVal.Data)
	}
	fd.Name = ""
	fd.Anonymous = true
	fd.Registry = r
	_ = runBoru(t, r, []Value{NewWord("undef"), NewWord(name), NewEnd()})
	return NewFunction(fd)
}

func typedFnBody(param, typ string, retType string, body ...Value) Value {
	p := NewOrderedMap()
	p.Set(param, NewWord(typ))
	return NewList([]Value{
		NewList([]Value{NewImplicitMap(p)}),
		NewList([]Value{NewWord(retType)}),
		NewList(body),
	})
}

func TestFnValueSameRegistrySplices(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	dbl := sameRegistryAnonFnValue(t, r, "dblsr", typedFnBody("x", "Integer", "Integer",
		NewWord("x"), NewWord("mul"), NewInteger(2)))

	res := runBoru(t, r, []Value{dbl, NewInteger(21)})
	if len(res) != 1 {
		t.Fatalf("residual = %v, want one value", res)
	}
	if n, _ := AsInteger(res[0]); n != 42 {
		t.Fatalf("same-registry fn value application = %d, want 42", n)
	}
	// The frame's __pa tail must have popped the per-call args list.
	if _, ok, _ := r.Args.Top(); ok {
		t.Error("args stack not popped after same-registry value dispatch")
	}
}

func TestFnValueSameRegistryDefCleanup(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	leaky := sameRegistryAnonFnValue(t, r, "leakysr", typedFnBody("x", "Integer", "Integer",
		NewWord("def"), NewWord("tmpsr"), NewInteger(7),
		NewWord("x"), NewWord("add"), NewWord("tmpsr")))

	res := runBoru(t, r, []Value{leaky, NewInteger(1)})
	if n, _ := AsInteger(res[len(res)-1]); n != 8 {
		t.Fatalf("body result = %d, want 8", n)
	}
	// Negative: the body-local def must be torn down by the frame tail,
	// exactly as the CallBoru path tore it down.
	if r.Defs.Has("tmpsr") {
		t.Error("body-local def leaked through same-registry value dispatch")
	}
}

func TestFnValueSameRegistryBreakInLoopBody(t *testing.T) {
	// A raw fn VALUE in a for-loop body whose body breaks: the loop on
	// the same tape terminates (acc stops at 1). This held on BOTH sides
	// of the flip (the CallBoru sub-engine shared the registry's FlowCtrl,
	// and the loop resolver ran in the enclosing engine); it is the
	// drain/flow invariant the TCO-STAGED residual asked to be pinned
	// before routing this class onto the splice branch.
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	brk := sameRegistryAnonFnValue(t, r, "brksr", typedFnBody("x", "Integer", "Any",
		NewWord("break")))

	// The accumulator is a var cell: the loop body's `def` would be the
	// iteration's own since phase 2 (design/IMMUTABLE-DEF.1.md §2.1).
	res := runBoru(t, r, []Value{
		NewWord("var"), NewWord("accsr"), NewInteger(0), NewEnd(),
		NewWord("for"), NewInteger(5), NewList([]Value{
			NewWord("var"), NewWord("accsr"), NewOpenParen(), NewWord("accsr"), NewWord("add"), NewInteger(1), NewCloseParen(),
			brk, NewInteger(1),
		}), NewEnd(),
		NewWord("accsr"), NewEnd(),
	})
	if len(res) == 0 {
		t.Fatal("no residual")
	}
	if n, _ := AsInteger(res[len(res)-1]); n != 1 {
		t.Fatalf("break through same-registry value dispatch: acc = %d, want 1 (loop broken on first iteration)", n)
	}
}

func TestFnValueCrossRegistryStillCallBoru(t *testing.T) {
	// Negative guard for the flip's condition: a fn value captured over a
	// DIFFERENT registry must keep the CallBoru path — its body names
	// resolve in the captured registry, not the executing one.
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	other, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// A module-private helper exists only in `other`.
	_ = runBoru(t, other, []Value{NewWord("def"), NewWord("privsr"), NewInteger(30), NewEnd()})
	helper := sameRegistryAnonFnValue(t, other, "helpersr", typedFnBody("x", "Integer", "Integer",
		NewWord("x"), NewWord("add"), NewWord("privsr")))

	res := runBoru(t, r, []Value{helper, NewInteger(12)})
	if n, _ := AsInteger(res[len(res)-1]); n != 42 {
		t.Fatalf("cross-registry fn value = %d, want 42 (private name must resolve in captured registry)", n)
	}
	// And the private name stays invisible in the executing registry.
	if r.Defs.Has("privsr") {
		t.Error("captured registry's private def leaked into the executing registry")
	}
}

// Cross-registry fn-value dispatch shapes: stack form with a structural
// cell interleaved among the operands, a 0-arg named value, and forward
// form. (These route through the compiled-value dispatch path, not
// execFnDefSig's CallBoru splice arms — those arms are allowlisted as
// defensive residue; see test/go/covergate/allowlist.tsv.) The pins are
// behavioural: each shape must produce the callee's result with the
// callee's names resolved in the CAPTURED registry.
func TestFnValueCrossRegistrySpliceArms(t *testing.T) {
	mk := func(t *testing.T) (*Registry, *Registry) {
		t.Helper()
		r, err := DefaultRegistry()
		if err != nil {
			t.Fatal(err)
		}
		other, err := DefaultRegistry()
		if err != nil {
			t.Fatal(err)
		}
		return r, other
	}

	t.Run("stack indices with interleaved mark", func(t *testing.T) {
		r, other := mk(t)
		helper := sameRegistryAnonFnValue(t, other, "xr1", typedFnBody("x", "Integer", "Integer",
			NewWord("x"), NewWord("mul"), NewInteger(2)))
		// [12, mark, helper]: arg collection skips the mark; the splice's
		// copy-down walk must carry it over the consumed positions.
		res := runBoru(t, r, []Value{NewInteger(12), NewMark("xrm1"), helper, NewEnd()})
		if n, _ := AsInteger(res[len(res)-1]); n != 24 {
			t.Fatalf("stack-form cross-registry dispatch = %d, want 24", n)
		}
	})

	t.Run("zero-arg value", func(t *testing.T) {
		r, other := mk(t)
		zero := sameRegistryAnonFnValue(t, other, "xr0", NewList([]Value{
			NewList(nil), NewList([]Value{NewWord("Integer")}), NewList([]Value{NewInteger(30)}),
		}))
		fd := zero.Data.(FnDefInfo)
		fd.Name = "xr0named" // 0-arg dispatch requires a NAMED value (anonymous stays data)
		fd.Anonymous = false
		res := runBoru(t, r, []Value{NewFunction(fd), NewEnd()})
		if len(res) == 0 {
			t.Fatal("no residual from 0-arg cross-registry dispatch")
		}
		if n, _ := AsInteger(res[len(res)-1]); n != 30 {
			t.Fatalf("0-arg cross-registry dispatch = %d, want 30", n)
		}
	})

	t.Run("forward form without stack indices", func(t *testing.T) {
		r, other := mk(t)
		helper := sameRegistryAnonFnValue(t, other, "xrf", typedFnBody("x", "Integer", "Integer",
			NewWord("x"), NewWord("add"), NewInteger(1)))
		res := runBoru(t, r, []Value{helper, NewInteger(41), NewEnd()})
		if n, _ := AsInteger(res[len(res)-1]); n != 42 {
			t.Fatalf("forward-form cross-registry dispatch = %d, want 42", n)
		}
	})
}
