package eng

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

func TestIdealRegistry_RegisterGet(t *testing.T) {
	ir := core.NewIdealRegistry()
	a := &core.Ideal{Name: "A", Enabled: true}
	ir.Register(a)
	if got := ir.Get("A"); got != a {
		t.Errorf("Get(A) = %v, want the registered Ideal", got)
	}
	if got := ir.Get("missing"); got != nil {
		t.Errorf("Get(missing) = %v, want nil", got)
	}
}

func TestIdealRegistry_ForResolvesByAccepts(t *testing.T) {
	ir := core.NewIdealRegistry()
	ir.Register(&core.Ideal{
		Name: "Stringy", Enabled: true,
		Accepts: func(v core.Value) bool { return core.ParentOf(&v).ConformsTo(core.TString) },
	})
	ir.Register(&core.Ideal{
		Name: "Inty", Enabled: true,
		Accepts: func(v core.Value) bool { return core.ParentOf(&v).ConformsTo(core.TInteger) },
	})
	if id := ir.For(core.NewString("x")); id == nil || id.Name != "Stringy" {
		t.Errorf("For(string) = %v, want Stringy", id)
	}
	if id := ir.For(core.NewInteger(1)); id == nil || id.Name != "Inty" {
		t.Errorf("For(integer) = %v, want Inty", id)
	}
	if id := ir.For(core.NewBoolean(true)); id != nil {
		t.Errorf("For(boolean) = %v, want nil (no kind claims it)", id)
	}
}

func TestIdealRegistry_ForSkipsDisabled(t *testing.T) {
	ir := core.NewIdealRegistry()
	ir.Register(&core.Ideal{
		Name: "Off", Enabled: false,
		Accepts: func(core.Value) bool { return true },
	})
	if id := ir.For(core.NewString("x")); id != nil {
		t.Errorf("For with only a disabled Ideal = %v, want nil", id)
	}
}

func TestIdealRegistry_ForFirstMatchWins(t *testing.T) {
	ir := core.NewIdealRegistry()
	ir.Register(&core.Ideal{Name: "First", Enabled: true, Accepts: func(core.Value) bool { return true }})
	ir.Register(&core.Ideal{Name: "Second", Enabled: true, Accepts: func(core.Value) bool { return true }})
	if id := ir.For(core.NewString("x")); id == nil || id.Name != "First" {
		t.Errorf("For = %v, want First (registration order wins ties)", id)
	}
}

func TestIdealRegistry_ReregisterReplaces(t *testing.T) {
	ir := core.NewIdealRegistry()
	ir.Register(&core.Ideal{Name: "K", Enabled: true, Accepts: func(core.Value) bool { return false }})
	ir.Register(&core.Ideal{Name: "K", Enabled: true, Accepts: func(core.Value) bool { return true }})
	got := ir.Get("K")
	if got == nil || !got.Accepts(core.NewString("x")) {
		t.Error("re-register did not replace the descriptor")
	}
	if n := len(ir.Names()); n != 1 {
		t.Errorf("re-register changed the count to %d, want 1", n)
	}
}

func TestIdealRegistry_Names(t *testing.T) {
	ir := core.NewIdealRegistry()
	ir.Register(&core.Ideal{Name: "One", Enabled: true})
	ir.Register(&core.Ideal{Name: "Two", Enabled: true})
	names := ir.Names()
	if len(names) != 2 || names[0] != "One" || names[1] != "Two" {
		t.Errorf("Names() = %v, want [One Two] in registration order", names)
	}
}

func TestIdealRegistry_NilSafe(t *testing.T) {
	var ir *core.IdealRegistry
	ir.Register(&core.Ideal{Name: "X"})
	if ir.Get("X") != nil || ir.For(core.NewString("x")) != nil ||
		ir.Match(core.NewString("x")) != nil || ir.Names() != nil {
		t.Error("nil IdealRegistry methods must be safe no-ops")
	}
}

// Match reports the first claiming Ideal regardless of Enabled; For
// additionally requires it to be enabled. The pair lets a caller tell
// a disabled kind apart from an unknown base.
func TestIdealRegistry_Match(t *testing.T) {
	ir := core.NewIdealRegistry()
	ir.Register(&core.Ideal{
		Name: "Off", Enabled: false,
		Accepts: func(v core.Value) bool { return core.ParentOf(&v).ConformsTo(core.TString) },
	})
	ir.Register(&core.Ideal{
		Name: "On", Enabled: true,
		Accepts: func(v core.Value) bool { return core.ParentOf(&v).ConformsTo(core.TInteger) },
	})
	if id := ir.Match(core.NewString("x")); id == nil || id.Name != "Off" {
		t.Errorf("Match(string) = %v, want Off (Match reports disabled kinds)", id)
	}
	if id := ir.For(core.NewString("x")); id != nil {
		t.Errorf("For(string) = %v, want nil (For skips disabled kinds)", id)
	}
	if id := ir.Match(core.NewInteger(1)); id == nil || id.Name != "On" {
		t.Errorf("Match(integer) = %v, want On", id)
	}
	if id := ir.For(core.NewInteger(1)); id == nil || id.Name != "On" {
		t.Errorf("For(integer) = %v, want On", id)
	}
	if id := ir.Match(core.NewBoolean(true)); id != nil {
		t.Errorf("Match(boolean) = %v, want nil (no kind claims it)", id)
	}
}

func TestRegisterKernelIdeals(t *testing.T) {
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Object", "Record", "Table"} {
		id := r.Ideals.Get(name)
		if id == nil {
			t.Fatalf("kernel Ideal %q not registered by NewRegistry", name)
		}
		if id.Accepts == nil {
			t.Errorf("kernel Ideal %q has no Accepts predicate", name)
		}
		if id.Instantiate == nil {
			t.Errorf("kernel Ideal %q has no Instantiate", name)
		}
	}
}

// A refinement is available for dispatch only while its whole Refines
// chain is enabled; Match reports it regardless of the chain state.
func TestIdeal_Refines(t *testing.T) {
	ir := core.NewIdealRegistry()
	base := &core.Ideal{
		Name: "Tensor", Enabled: true,
		Accepts: func(v core.Value) bool { return core.ParentOf(&v).ConformsTo(core.TInteger) },
	}
	ref := &core.Ideal{
		Name: "Matrix", Enabled: true, Refines: base,
		Accepts: func(v core.Value) bool { return core.ParentOf(&v).ConformsTo(core.TString) },
	}
	ir.Register(base)
	ir.Register(ref)
	if id := ir.For(core.NewString("x")); id == nil || id.Name != "Matrix" {
		t.Fatalf("For(string) = %v, want Matrix", id)
	}
	// Disabling the base kind makes the refinement unavailable too.
	base.Enabled = false
	if id := ir.For(core.NewString("x")); id != nil {
		t.Errorf("For with base disabled = %v, want nil (refinement follows its base)", id)
	}
	if id := ir.Match(core.NewString("x")); id == nil || id.Name != "Matrix" {
		t.Errorf("Match with base disabled = %v, want Matrix (Match ignores the chain)", id)
	}
	// Re-enabling the base restores the refinement.
	base.Enabled = true
	if id := ir.For(core.NewString("x")); id == nil || id.Name != "Matrix" {
		t.Errorf("For after re-enabling base = %v, want Matrix", id)
	}
	// Disabling the refinement itself also removes it.
	ref.Enabled = false
	if id := ir.For(core.NewString("x")); id != nil {
		t.Errorf("For with refinement disabled = %v, want nil", id)
	}
}

type fakeHostType struct{ core.HostTypeBody }

// A host module's constructed type — an ExtensionPayload whose Body
// embeds HostTypeBody — is recognised by the kernel's type machinery
// without the kernel inspecting the opaque payload.
func TestIsHostTypeBody(t *testing.T) {
	typeVal := core.NewExtension(core.TAny, fakeHostType{})
	if !core.IsHostTypeBody(typeVal) {
		t.Error("IsHostTypeBody(marked extension) = false, want true")
	}
	if !core.IsTypeBody(&typeVal) {
		t.Error("IsTypeBody(host type body) = false, want true")
	}
	// An ExtensionPayload whose Body does not embed HostTypeBody is an
	// instance, not a type.
	instVal := core.NewExtension(core.TAny, struct{ n int }{1})
	if core.IsHostTypeBody(instVal) {
		t.Error("IsHostTypeBody(plain extension) = true, want false")
	}
	if core.IsTypeBody(&instVal) {
		t.Error("IsTypeBody(plain extension instance) = true, want false")
	}
	if core.IsHostTypeBody(core.NewInteger(1)) {
		t.Error("IsHostTypeBody(integer) = true, want false")
	}
}
