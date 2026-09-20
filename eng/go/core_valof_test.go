package eng_test

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
	parser "github.com/boru-lang/boru/parser/go"
)

// freshRegistry builds an eng-only Registry plus a small set of probe
// natives. The /v suffix is a parser+kernel feature, so these tests
// stay in eng and only need the kernel surface — no `ref` word, no
// `apply` word, neither of which lives here. They test stepWord's
// ForceVal branch, core.ResolveRef directly, and the dispatch of
// unquoted Function values via execFnDefLiteral.
func freshRegistry(t *testing.T) *core.Registry {
	t.Helper()
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "add",

		Signatures: []core.Signature{{
			Args: []*core.Type{core.TInteger, core.TInteger},
			Impl: core.Go(func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
				a, _ := core.AsInteger(args[1])
				b, _ := core.AsInteger(args[0])
				return []core.Value{core.NewInteger(a + b)}, nil
			}),
			Returns: []*core.Type{core.TInteger}, BarrierPos: -1,
		}},
	})
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "mul",

		Signatures: []core.Signature{{
			Args: []*core.Type{core.TInteger, core.TInteger},
			Impl: core.Go(func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
				a, _ := core.AsInteger(args[1])
				b, _ := core.AsInteger(args[0])
				return []core.Value{core.NewInteger(a * b)}, nil
			}),
			Returns: []*core.Type{core.TInteger}, BarrierPos: -1,
		}},
	})
	r.Defs.Push("answer", core.NewInteger(42))
	if err := r.Err(); err != nil {
		t.Fatalf("registration: %v", err)
	}
	r.InitRootContext()
	return r
}

func runSrc(t *testing.T, r *core.Registry, src string) []core.Value {
	t.Helper()
	prog, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("Parse(%q): %v", src, err)
	}
	out, err := core.NewTop(r).Run(prog)
	if err != nil {
		t.Fatalf("Run(%q): %v", src, err)
	}
	return out
}

func runSrcErr(t *testing.T, r *core.Registry, src string) error {
	t.Helper()
	prog, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("Parse(%q): %v", src, err)
	}
	_, err = core.NewTop(r).Run(prog)
	return err
}

// --- the asymmetry the /v suffix exists to address --------------------

// TestBareWordInvokesFnBinding pins existing behavior: a bare word
// for an fn binding fires dispatch.
func TestBareWordInvokesFnBinding(t *testing.T) {
	r := freshRegistry(t)
	out := runSrc(t, r, "2 add 3")
	if len(out) != 1 {
		t.Fatalf("got %d values, want 1: %v", len(out), out)
	}
	got, _ := core.AsInteger(out[0])
	if got != 5 {
		t.Errorf("bare `add` did not invoke: got %d, want 5", got)
	}
}

// TestRefSuffixReturnsFunctionValue: /v resolves to an UNQUOTED
// Function value carrying the FnDefInfo. Unquoted is the new
// default — call site, not data.
func TestRefSuffixReturnsFunctionValue(t *testing.T) {
	r := freshRegistry(t)
	// `add/v` standalone: no following args, no preceding stack args,
	// so the unquoted Function value's sig doesn't match anything —
	// it sits as data at the end of Run.
	out := runSrc(t, r, "add/v")
	if len(out) != 1 {
		t.Fatalf("got %d values, want 1", len(out))
	}
	v := out[0]
	if !v.Parent.Equal(core.TFunction) {
		t.Errorf("top.Parent=%s, want Function", v.Parent.String())
	}
	if v.Quoted {
		t.Errorf("function value is Quoted — /v should produce unquoted in the new dispatch model")
	}
	fnDef, ok := v.Data.(core.FnDefInfo)
	if !ok {
		t.Fatalf("payload=%T, want FnDefInfo", v.Data)
	}
	if fnDef.Name != "add" {
		t.Errorf("fnDef.Name=%q, want %q", fnDef.Name, "add")
	}
}

// TestRefSuffixHoldsForwardArgsUndispatched: `/v` is a pure reference —
// it advances the pointer, leaving the resolved Function on the stack as
// data, and does NOT dispatch. So tokens that follow are not consumed as
// args: `add/v 2 3` yields [Function, 2, 3]. (To call, use the bare word
// `add 2 3`, or `apply` on the ref.)
func TestRefSuffixHoldsForwardArgsUndispatched(t *testing.T) {
	r := freshRegistry(t)
	out := runSrc(t, r, "add/v 2 3")
	if len(out) != 3 {
		t.Fatalf("got %d values, want 3 [Function 2 3]: %v", len(out), out)
	}
	if !out[0].Parent.Equal(core.TFunction) {
		t.Errorf("out[0].Parent=%s, want Function (held, not dispatched)", out[0].Parent.String())
	}
	if a, _ := core.AsInteger(out[1]); a != 2 {
		t.Errorf("out[1]=%v, want 2 (arg not consumed)", out[1])
	}
	if b, _ := core.AsInteger(out[2]); b != 3 {
		t.Errorf("out[2]=%v, want 3 (arg not consumed)", out[2])
	}
	// The call path still works through the bare word.
	out2 := runSrc(t, freshRegistry(t), "add 2 3")
	if got, _ := core.AsInteger(out2[0]); got != 5 {
		t.Errorf("add 2 3 = %d, want 5", got)
	}
}

// TestRefSuffixHoldsStackArgsUndispatched: stack-side — args already on
// the stack are likewise not consumed; `/v` just pushes the Function and
// advances. `2 3 add/v` yields [2, 3, Function].
func TestRefSuffixHoldsStackArgsUndispatched(t *testing.T) {
	r := freshRegistry(t)
	out := runSrc(t, r, "2 3 add/v")
	if len(out) != 3 {
		t.Fatalf("got %d values, want 3 [2 3 Function]: %v", len(out), out)
	}
	if a, _ := core.AsInteger(out[0]); a != 2 {
		t.Errorf("out[0]=%v, want 2", out[0])
	}
	if b, _ := core.AsInteger(out[1]); b != 3 {
		t.Errorf("out[1]=%v, want 3", out[1])
	}
	if !out[2].Parent.Equal(core.TFunction) {
		t.Errorf("out[2].Parent=%s, want Function (held, not dispatched)", out[2].Parent.String())
	}
}

// TestValSuffixOnSimpleValueBindingIsTheValue: /v is TOTAL over binding
// kinds. A simple-value binding (`answer` = 42) has no call to suppress,
// so `answer/v` is the identity and passes 42 straight through. The
// negative twin is TestRefSuffixUndefinedNameErrors below: an UNBOUND
// name still declines, because there is no value to take.
func TestValSuffixOnSimpleValueBindingIsTheValue(t *testing.T) {
	r := freshRegistry(t)
	out := runSrc(t, r, "answer/v")
	if len(out) != 1 {
		t.Fatalf("answer/v: got %v, want exactly one value", out)
	}
	got, err := core.AsInteger(out[0])
	if err != nil || got != 42 {
		t.Errorf("answer/v = %v (err %v), want 42", out[0], err)
	}
}

func TestRefSuffixUndefinedNameErrors(t *testing.T) {
	r := freshRegistry(t)
	err := runSrcErr(t, r, "nope/v")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "undefined") && !strings.Contains(err.Error(), "not bound") {
		t.Errorf("error=%q, want undefined/not-bound", err.Error())
	}
}

// --- the stable-map-lookup demonstration ------------------------------

// TestRefStableInMap proves the captured Function value retains its
// FnDef payload through map storage. The captured value is now
// UNQUOTED (live call-site shape); the test asserts identity, not
// Quoted state.
func TestRefStableInMap(t *testing.T) {
	r := freshRegistry(t)

	ops := core.NewOrderedMap()
	for _, name := range []string{"add", "mul"} {
		v, ok := resolveViaSlashR(t, r, name)
		if !ok {
			t.Fatalf("resolveViaSlashR(%q): not bound", name)
		}
		ops.Set(name, v)
	}

	for _, name := range []string{"add", "mul"} {
		v, ok := ops.Get(name)
		if !ok {
			t.Fatalf("ops[%q] missing", name)
		}
		if !v.Parent.Equal(core.TFunction) {
			t.Errorf("ops[%q].Parent=%s, want Function", name, v.Parent.String())
		}
		if v.Quoted {
			t.Errorf("ops[%q] is Quoted — captured Function should be unquoted (live call-site)", name)
		}
		fnDef, ok := v.Data.(core.FnDefInfo)
		if !ok {
			t.Fatalf("ops[%q] payload=%T, want FnDefInfo", name, v.Data)
		}
		if fnDef.Name != name {
			t.Errorf("ops[%q] fnDef.Name=%q", name, fnDef.Name)
		}
	}

	// Stability under shadowing: push a non-fn binding on top of `add`
	// (without popping the underlying FnDef). The map entry still
	// holds the original Function value — the map stores referents,
	// not names that get re-resolved.
	r.Defs.Push("add", core.NewString("shadowed"))
	v, _ := ops.Get("add")
	if !v.Parent.Equal(core.TFunction) {
		t.Fatalf("after shadow, ops[add].Parent=%s, want Function still", v.Parent.String())
	}
	fnDef, _ := v.Data.(core.FnDefInfo)
	if fnDef.Name != "add" {
		t.Errorf("after shadow, captured fnDef.Name=%q, want %q", fnDef.Name, "add")
	}
	if len(fnDef.Signatures) == 0 {
		t.Errorf("captured FnDef has no Signatures — the captured handle wouldn't dispatch")
	}

	// Hard-stability check: even after popping the underlying binding
	// entirely (so `add/v` would now fail), the previously captured
	// value still carries the full FnDef payload.
	if !r.Defs.Pop("add") {
		t.Fatal("Defs.Pop(shadow) returned false")
	}
	if !r.Defs.Pop("add") {
		t.Fatal("Defs.Pop(original) returned false")
	}
	if _, ok := r.Defs.Top("add"); ok {
		t.Fatal("expected add binding to be gone after double-pop")
	}
	stillThere, _ := ops.Get("add")
	stillFn, _ := stillThere.Data.(core.FnDefInfo)
	if stillFn.Name != "add" || len(stillFn.Signatures) == 0 {
		t.Errorf("post-undef captured fn lost shape: name=%q sigs=%d", stillFn.Name, len(stillFn.Signatures))
	}
}

// resolveViaSlashR runs `<name>/v` through the engine and returns the
// resulting value. The /v expression sits at end-of-program; with no
// following args its sig doesn't match anything and it falls through
// as data — that's how we get the captured value out.
func resolveViaSlashR(t *testing.T, r *core.Registry, name string) (core.Value, bool) {
	t.Helper()
	out := runSrc(t, r, name+"/v")
	if len(out) != 1 {
		return core.Value{}, false
	}
	return out[0], true
}

// TestResolveRefDirect exercises the exported helper independently of
// the parser. The returned Function is unquoted — same contract as
// the /v suffix path.
func TestResolveRefDirect(t *testing.T) {
	r := freshRegistry(t)
	v, ok := core.ResolveRef(r, "mul")
	if !ok {
		t.Fatal("ResolveRef(mul): not bound")
	}
	if !v.Parent.Equal(core.TFunction) {
		t.Errorf("Parent=%s, want Function", v.Parent.String())
	}
	if v.Quoted {
		t.Error("returned function is Quoted — should be unquoted")
	}
	fnDef, _ := v.Data.(core.FnDefInfo)
	if fnDef.Name != "mul" {
		t.Errorf("fnDef.Name=%q, want %q", fnDef.Name, "mul")
	}

	if _, ok := core.ResolveRef(r, "nope"); ok {
		t.Error("ResolveRef(nope): expected not-bound, got ok")
	}
}
