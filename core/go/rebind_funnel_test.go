package core

import "testing"

// THE FUNNEL GATE.
//
// The freeze discipline cost six silent miscompiles, and every one of them was
// the same shape: a binding operation, or a binding READ, that did not reach
// the notification because the notification was attached to word handlers and
// resolution branches rather than to one place. Seating both funnels in
// core/go/rebind_notify.go fixes the instances. THIS FILE is what fixes the
// class — it drives every binding operation and every read path and fails if
// one stops going through its funnel.
//
// If you are here because this test failed after adding a binding operation:
// the operation must call noteRebind, or a compiled unit that baked the name
// will keep answering from the stale bake and nothing will decline. Add the
// call, then add the row. Do not add the row alone.

// funnelRecorder captures both halves of the discipline.
type funnelRecorder struct {
	inactiveEmit
	rebound  map[string]int
	reads    map[string]FrozenBake
	gens     map[string]int64
	defReads map[string]string
}

func (f *funnelRecorder) NotifyNameRebound(name string) {
	if f.rebound == nil {
		f.rebound = map[string]int{}
	}
	f.rebound[name]++
}

func (f *funnelRecorder) NoteFrozenRead(name string, bake FrozenBake, gen int64) {
	if f.reads == nil {
		f.reads = map[string]FrozenBake{}
		f.gens = map[string]int64{}
	}
	f.reads[name] = bake
	f.gens[name] = gen
}

// NoteDefRead is captured too: a TYPE read must register its value ID against
// the name exactly as a value read does, or the compiler's resolveOperand can
// name one kind of read and not the other.
func (f *funnelRecorder) NoteDefRead(id, name string) {
	if f.defReads == nil {
		f.defReads = map[string]string{}
	}
	f.defReads[id] = name
}

func funnelRegistry(t *testing.T) (*Registry, *funnelRecorder) {
	t.Helper()
	r := compileCheckRegistry(t)
	rec := &funnelRecorder{}
	r.Check.Emit = rec
	return r, rec
}

// Every binding OPERATION notifies. The rows are the operations, not the
// words: a word library binds by calling one of these, so covering them covers
// every binder that exists or will exist.
func TestBindingOpsNotifyRebind(t *testing.T) {
	fnBody := func(*Registry) Value {
		return NewFunction(FnDefInfo{Name: "g", Signatures: []Signature{{
			Params:     []FnParam{{Name: "n", Type: TInteger}},
			Returns:    []*Type{TInteger},
			Impl:       Boru([]Value{NewWord("n")}),
			BarrierPos: BarrierAllForward,
		}}})
	}
	for _, c := range []struct {
		what string
		op   func(*Registry)
	}{
		{"InstallDef, a value", func(r *Registry) { InstallDef(r, "k", NewInteger(5)) }},
		{"InstallDef, a fn", func(r *Registry) { InstallDef(r, "g", fnBody(r)) }},
		{"UninstallDef", func(r *Registry) {
			InstallDef(r, "k", NewInteger(5))
			UninstallDef(r, "k")
		}},
		{"InstallType", func(r *Registry) { _ = InstallType(r, "T", NewTypeLiteral(TInteger)) }},
		{"UninstallType", func(r *Registry) {
			_ = InstallType(r, "T", NewTypeLiteral(TInteger))
			UninstallType(r, "T")
		}},
		{"UninstallFnSigs", func(r *Registry) {
			InstallDef(r, "g", fnBody(r))
			UninstallFnSigs(r, "g", FnUndefInfo{})
		}},
	} {
		t.Run(c.what, func(t *testing.T) {
			r, rec := funnelRegistry(t)
			c.op(r)
			if len(rec.rebound) == 0 {
				t.Errorf("%s bound a name without notifying the rebind — a unit that "+
					"baked it will keep answering from the stale bake, and nothing will "+
					"decline. Seat the operation on noteRebind (core/go/rebind_notify.go).",
					c.what)
			}
		})
	}
}

// The one operation that must NOT notify, and the reason the funnel takes a
// `!shadow` test rather than sitting on DefTable.Push: a frame binding is a
// param or a capture, scoped to one call, and declining on it would decline
// every program whose fn shadows a module name.
func TestFrameBindingDoesNotNotifyRebind(t *testing.T) {
	r, rec := funnelRegistry(t)
	InstallFrameBinding(r, "k", NewInteger(5))
	if n := rec.rebound["k"]; n != 0 {
		t.Errorf("a frame binding notified %d time(s): a param shadowing a module name "+
			"is not a rebind of it", n)
	}
}

// Every READ path notes through the one classifier. The rows are the
// resolution paths, and the `/v` row is the one that was missing: it resolves
// through ResolveRef and reaches neither of stepWord's substitution branches,
// which is how both of its bakes escaped the latch.
func TestBindingReadsNoteThroughTheFunnel(t *testing.T) {
	for _, c := range []struct {
		what string
		bind func(*Registry)
		step func(*Engine) error
		want FrozenBake
	}{
		{
			"stepWord, a value read",
			func(r *Registry) { r.Defs.Push("k", NewInteger(5)) },
			func(e *Engine) error { return e.stepWord(e.Tape.At(0)) },
			FrozenBakeValue,
		},
		{
			"stepWord, a type read",
			func(r *Registry) { r.Defs.PushType("k", TInteger, NewTypeLiteral(TInteger)) },
			func(e *Engine) error { return e.stepWord(e.Tape.At(0)) },
			FrozenBakeType,
		},
		{
			"stepWordVal (`/v`), a value read",
			func(r *Registry) { r.Defs.Push("k", NewInteger(5)) },
			func(e *Engine) error {
				return e.stepWordVal(e.Tape.At(0), WordInfo{Name: "k", ArgCount: -1, ForceVal: true})
			},
			FrozenBakeValue,
		},
		{
			"stepWordVal (`/v`), a type read",
			func(r *Registry) { r.Defs.PushType("k", TInteger, NewTypeLiteral(TInteger)) },
			func(e *Engine) error {
				return e.stepWordVal(e.Tape.At(0), WordInfo{Name: "k", ArgCount: -1, ForceVal: true})
			},
			FrozenBakeType,
		},
	} {
		t.Run(c.what, func(t *testing.T) {
			r, rec := funnelRegistry(t)
			c.bind(r)
			e := NewTop(r)
			e.Tape = NewTape([]Value{NewWord("k")}, StackHeadroom)
			if err := c.step(e); err != nil {
				t.Fatalf("%s errored: %v", c.what, err)
			}
			got, ok := rec.reads["k"]
			if !ok {
				t.Fatalf("%s did not note the read — a resolution path that skips "+
					"noteBindingRead bakes silently. Route it through the funnel "+
					"(core/go/rebind_notify.go).", c.what)
			}
			if got != c.want {
				t.Errorf("%s classified the bake as %v, want %v — the paths must not "+
					"answer the freeze question two ways", c.what, got, c.want)
			}
			if want := r.Defs.Gen("k"); rec.gens["k"] != want {
				t.Errorf("%s must note Gen(k) at the read (%d), got %d", c.what, want, rec.gens["k"])
			}
			if c.want == FrozenBakeType {
				// The type arm names its read like the value arm does.
				named := false
				for _, n := range rec.defReads {
					if n == "k" {
						named = true
					}
				}
				if !named {
					t.Errorf("%s must register the read's value ID against the name (NoteDefRead)", c.what)
				}
			}
		})
	}
}

// The read funnel's negatives: a fn-local name is not a module binding, and a
// non-concrete value (a carrier — a module-scope `flex`, a computed def) bakes
// nothing because resolveOperand routes it to a live lookup instead.
func TestBindingReadFunnelNegatives(t *testing.T) {
	r, rec := funnelRegistry(t)
	r.Defs.Push("k", NewInteger(5))
	r.FnBaselines = append(r.FnBaselines, map[string]int{"k": 0})
	e := NewTop(r)
	e.noteBindingRead("k", NewInteger(5))
	if _, ok := rec.reads["k"]; ok {
		t.Error("a fn-local read must note nothing: the name does not resolve where the unit runs")
	}

	r2, rec2 := funnelRegistry(t)
	r2.Defs.Push("c", NewCarrier(TInteger))
	e2 := NewTop(r2)
	e2.noteBindingRead("c", NewCarrier(TInteger))
	if _, ok := rec2.reads["c"]; ok {
		t.Error("a carrier read must note nothing: it routes to a live lookup, not a bake")
	}

	var nilE *Engine
	nilE.noteBindingRead("k", NewInteger(5))
}

// The funnel's own guards. They are unreachable from a running program by
// construction — a live pass always has a registry and a CheckState — so they
// are driven here, and they exist because ADR-005 forbids the alternative of
// letting a nil deref reach a user.
func TestRebindFunnelGuards(t *testing.T) {
	noteRebind(nil, "k")
	r, rec := funnelRegistry(t)
	saved := r.Check
	r.Check = nil
	noteRebind(r, "k")
	r.Check = saved
	noteRebind(r, "")
	if len(rec.rebound) != 0 {
		t.Errorf("a nil registry, a nil CheckState and an empty name must all notify nothing; got %v", rec.rebound)
	}
}

// UninstallType's three arms beyond the happy path: no registry, no binding to
// remove, and the MINTED case, where the lattice node is retired with the
// binding. The alias case (Minted false) is the one the happy-path row above
// covers — `def T Integer` ADOPTS Integer's node, and retiring it there would
// delete a builtin's identity from the ID index.
func TestUninstallTypeArms(t *testing.T) {
	if UninstallType(nil, "T") {
		t.Error("a nil registry removes nothing")
	}
	r, _ := funnelRegistry(t)
	if UninstallType(r, "NeverBound") {
		t.Error("an unbound name removes nothing")
	}

	minted := r.Types.MintType("Mz", TInteger)
	r.Defs.PushType("Mz", minted, NewTypeLiteral(minted))
	if r.Types.LookupByID(minted.ID) == nil {
		t.Fatal("the minted node must be in the ID index before the undef")
	}
	if !UninstallType(r, "Mz") {
		t.Fatal("a bound type must be removed")
	}
	if r.Types.LookupByID(minted.ID) != nil {
		t.Error("a MINTED node must be retired with its binding")
	}
}
