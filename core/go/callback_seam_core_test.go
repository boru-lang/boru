package core

import "testing"

// callback_seam_core_test.go pins, in core's OWN suite, the three seams the
// fifth increment added for the VM and the language layer to use (the
// standalone cover-gate-core profiles core by its own tests alone):
// InvokeCallbackBody's RetTrim stamp, Registry.InPredicateCall, and the
// one-shot Engine.InertPrefix seat at Run.

// TestInPredicateCall pins the read-side helper over the predicate-call
// counter.
func TestInPredicateCall(t *testing.T) {
	r := covRegistry(t, nil)
	if r.InPredicateCall() {
		t.Error("no predicate body is running")
	}
	r.predicateCalls++
	if !r.InPredicateCall() {
		t.Error("inside a predicate body")
	}
	r.predicateCalls--
}

// TestRunSeatsInertPrefix pins the one-shot seat: a caller-declared
// InertPrefix wider than the run's own start offset becomes the run's
// inert prefix and is consumed (reset to 0) by the run.
func TestRunSeatsInertPrefix(t *testing.T) {
	r := covRegistry(t, nil)
	e := NewTop(r)
	e.InertPrefix = 2
	if _, err := e.Run([]Value{NewInteger(1), NewInteger(2), NewInteger(3)}); err != nil {
		t.Fatalf("run: %v", err)
	}
	if e.InertPrefix != 0 {
		t.Error("the declaration is one-shot")
	}
	if e.inertPrefix != 2 {
		t.Errorf("the wider declaration wins over the start offset: %d", e.inertPrefix)
	}
	e2 := NewTop(r)
	if _, err := e2.Run([]Value{NewInteger(1)}); err != nil {
		t.Fatalf("run: %v", err)
	}
	if e2.inertPrefix != 0 {
		t.Errorf("no declaration: the start offset alone: %d", e2.inertPrefix)
	}
}

// TestShadowInstallFiresNoRegisterHook pins the register hook's gate: a
// SHADOWING frame install (InstallFrameBinding — a per-call param, the
// VM's word-read replay binding) is not a registration, so neither the
// plain fn-def arm nor the trivial-delegation rebinding arm fires
// OnRegisterHook (dynamic help's example generator, which runs an engine);
// a `def` registration proper still does.
func TestShadowInstallFiresNoRegisterHook(t *testing.T) {
	r := covRegistry(t, nil)
	sub := covRegistry(t, nil)
	sub.RegisterNativeFunc(NativeFunc{
		Name: "shinner",
		Signatures: []Signature{{
			Args: []*Type{TInteger}, BarrierPos: BarrierAllForward,
			Impl: Go(func(a []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return a, nil
			}),
		}},
	})
	if err := sub.Err(); err != nil {
		t.Fatalf("registration: %v", err)
	}
	r.MarkReady()
	var hooked []string
	r.OnRegisterHook = func(name string) { hooked = append(hooked, name) }
	plain := NewFunction(FnDefInfo{Signatures: []Signature{{
		Params: []FnParam{{Name: "n", Type: TInteger}},
		Impl:   Boru([]Value{NewWord("n")}),
	}}})
	wrapper := NewFunction(FnDefInfo{
		Registry: sub,
		Signatures: []Signature{{
			Params: []FnParam{{Name: "", Type: TInteger}},
			Impl:   Boru([]Value{NewWord("shinner")}),
		}},
	})
	InstallFrameBinding(r, "shg", plain)
	InstallFrameBinding(r, "shw", wrapper)
	if len(hooked) != 0 {
		t.Errorf("a shadowing frame install is not a registration: hook fired for %v", hooked)
	}
	if r.Defs.Depth("shg") != 1 || r.Defs.Depth("shw") != 1 {
		t.Error("both frame bindings are installed")
	}
	InstallDef(r, "shdef", plain)
	if len(hooked) != 1 || hooked[0] != "shdef" {
		t.Errorf("a def registration fires the hook once: %v", hooked)
	}
}
