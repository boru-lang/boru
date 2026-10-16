package eng

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// These tests exercise the engine's deeper machinery (type system,
// def stack, signature matching, multi-arg dispatch, value identity)
// using only the public NativeFunc registration API. No parser, no
// built-in word library.

// --- *Type system primitives ---------------------------------------------

func TestTypePathBuiltins(t *testing.T) {
	// Round-trip a few well-known type names through the canonical
	// table and confirm they reach the correct *Type values.
	table := core.TypeNameTable()
	cases := []struct {
		name string
		want *core.Type
	}{
		{"Integer", core.TInteger},
		{"String", core.TString},
		{"Boolean", core.TBoolean},
		{"List", core.TList},
		{"Map", core.TMap},
		{"Any", core.TAny},
	}
	for _, c := range cases {
		got, ok := table[c.name]
		if !ok {
			t.Errorf("%q missing from TypeNameTable", c.name)
			continue
		}
		if !got.Equal(c.want) {
			t.Errorf("%q: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestTypeMatchHierarchy(t *testing.T) {
	// Integer is a subtype of Number is a subtype of Scalar.
	if !core.TInteger.ConformsTo(core.TNumber) {
		t.Error("Integer should match Number")
	}
	if !core.TInteger.ConformsTo(core.TScalar) {
		t.Error("Integer should match Scalar")
	}
	if !core.TInteger.ConformsTo(core.TAny) {
		t.Error("Integer should match Any")
	}
	if core.TNumber.ConformsTo(core.TInteger) {
		t.Error("Number should NOT match Integer (only the reverse)")
	}
}

func TestCommonAncestorType(t *testing.T) {
	// Integer + Float → Number; String + Integer → Scalar; List + Integer → Any.
	if got := core.CommonAncestorType(core.TInteger, core.TFloat); !got.Equal(core.TNumber) {
		t.Errorf("Integer+Float: got %v, want Number", got)
	}
	if got := core.CommonAncestorType(core.TString, core.TInteger); !got.Equal(core.TScalar) {
		t.Errorf("String+Integer: got %v, want Scalar", got)
	}
	if got := core.CommonAncestorType(core.TList, core.TInteger); !got.Equal(core.TAny) {
		t.Errorf("List+Integer: got %v, want Any", got)
	}
}

// --- Value constructors and identity ------------------------------------

func TestValueConstructors(t *testing.T) {
	cases := []struct {
		name  string
		value core.Value
		want  *core.Type
	}{
		{"integer", core.NewInteger(42), core.TInteger},
		{"decimal", core.NewFloat(3.14), core.TFloat},
		{"string", core.NewString("hi"), core.TString},
		{"boolean", core.NewBoolean(true), core.TBoolean},
		{"atom", core.NewAtom("x"), core.TAtom},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if !core.ParentOf(&c.value).ConformsTo(c.want) {
				t.Errorf("Parent = %v does not match expected %v", core.ParentOf(&c.value), c.want)
			}
			if !core.IsConcrete(&c.value) {
				t.Error("Data should not be nil for a concrete value")
			}
		})
	}
}

func TestTypeLiteralVsConcrete(t *testing.T) {
	lit := core.NewTypeLiteral(core.TString)
	concrete := core.NewString("hello")
	if !core.IsTypeLiteral(&lit) {
		t.Error("type literal should be IsTypeLiteral")
	}
	if core.IsConcrete(&lit) {
		t.Error("type literal should NOT be IsConcrete")
	}
	if core.IsTypeLiteral(&concrete) {
		t.Error("concrete string should NOT be IsTypeLiteral")
	}
	if !core.IsConcrete(&concrete) {
		t.Error("concrete string should be IsConcrete")
	}
}

// --- Registry def-stack helpers ------------------------------------------

func TestDefStackPushPopShadow(t *testing.T) {
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if r.Defs.Has("x") {
		t.Error("fresh registry should not have x")
	}

	r.Defs.Push("x", core.NewInteger(1))
	if !r.Defs.Has("x") {
		t.Error("after push, x should exist")
	}
	if d := r.Defs.Depth("x"); d != 1 {
		t.Errorf("depth = %d, want 1", d)
	}
	v, _ := r.Defs.Top("x")
	got, _ := core.AsInteger(v)
	if got != 1 {
		t.Errorf("top = %d, want 1", got)
	}

	// Shadow with a second push.
	r.Defs.Push("x", core.NewInteger(2))
	v, _ = r.Defs.Top("x")
	got, _ = core.AsInteger(v)
	if got != 2 {
		t.Errorf("after second push, top = %d, want 2", got)
	}

	// Pop reveals the original.
	r.Defs.Pop("x")
	v, _ = r.Defs.Top("x")
	got, _ = core.AsInteger(v)
	if got != 1 {
		t.Errorf("after pop, top = %d, want 1", got)
	}

	// Final pop empties the stack.
	r.Defs.Pop("x")
	if r.Defs.Has("x") {
		t.Error("after final pop, x should be gone")
	}
}

func TestSnapshotRestoreDefDepths(t *testing.T) {
	// SnapshotDefDepths captures a per-name depth map; RestoreToDefDepths
	// truncates each name back to its captured depth. This is the
	// mechanism fn-body sandboxing uses to drop temporary bindings.
	r, _ := core.NewRegistry()
	r.Defs.Push("a", core.NewInteger(1))
	snap := r.Defs.Snapshot()

	r.Defs.Push("a", core.NewInteger(2))
	r.Defs.Push("b", core.NewInteger(99))
	if d := r.Defs.Depth("a"); d != 2 {
		t.Errorf("depth after pushes: a=%d, want 2", d)
	}

	r.Defs.Restore(snap)
	if d := r.Defs.Depth("a"); d != 1 {
		t.Errorf("depth after restore: a=%d, want 1", d)
	}
	if r.Defs.Has("b") {
		t.Error("b should have been truncated to zero by restore")
	}
}

// --- Native dispatch: multi-arg, multi-overload --------------------------

func TestMultipleSignaturesDispatch(t *testing.T) {
	// Register a "describe" word with two overloads: one for Integer,
	// one for String. The engine must pick the right one based on arg
	// types.
	r, _ := core.NewRegistry()
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "describe",

		Signatures: []core.Signature{
			{
				Args: []*core.Type{core.TInteger},
				Impl: core.Go(func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
					n, _ := core.AsInteger(args[0])
					if n == 0 {
						return []core.Value{core.NewString("zero-int")}, nil
					}
					return []core.Value{core.NewString("nonzero-int")}, nil
				}),
				Returns: []*core.Type{core.TString}, BarrierPos: -1,
			},
			{
				Args: []*core.Type{core.TString},
				Impl: core.Go(func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
					s, _ := core.AsString(args[0])
					return []core.Value{core.NewString("string:" + s)}, nil
				}),
				Returns: []*core.Type{core.TString}, BarrierPos: -1,
			},
		},
	})
	r.InitRootContext()

	cases := []struct {
		input []core.Value
		want  string
	}{
		{[]core.Value{core.NewWord("describe"), core.NewInteger(5)}, "nonzero-int"},
		{[]core.Value{core.NewWord("describe"), core.NewInteger(0)}, "zero-int"},
		{[]core.Value{core.NewWord("describe"), core.NewString("hi")}, "string:hi"},
	}
	for _, c := range cases {
		out, err := core.NewTop(r).Run(c.input)
		if err != nil {
			t.Errorf("%v: error %v", c.input, err)
			continue
		}
		got, _ := core.AsString(out[0])
		if got != c.want {
			t.Errorf("%v: got %q, want %q", c.input, got, c.want)
		}
	}
}

func TestSignatureDispatchFavoursSpecificity(t *testing.T) {
	// Generic (Any) and specific (Integer) overloads of the same word.
	// A concrete integer arg must dispatch to the specific overload.
	hits := map[string]int{}
	r, _ := core.NewRegistry()
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "tag",

		Signatures: []core.Signature{
			{
				Args: []*core.Type{core.TAny},
				Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
					hits["any"]++
					return []core.Value{core.NewString("any")}, nil
				}),
				Returns: []*core.Type{core.TString}, BarrierPos: -1,
			},
			{
				Args: []*core.Type{core.TInteger},
				Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
					hits["int"]++
					return []core.Value{core.NewString("int")}, nil
				}),
				Returns: []*core.Type{core.TString}, BarrierPos: -1,
			},
		},
	})
	r.InitRootContext()

	if _, err := core.NewTop(r).Run([]core.Value{core.NewWord("tag"), core.NewInteger(7)}); err != nil {
		t.Fatal(err)
	}
	if hits["int"] != 1 || hits["any"] != 0 {
		t.Errorf("specificity broken: hits=%v", hits)
	}

	if _, err := core.NewTop(r).Run([]core.Value{core.NewWord("tag"), core.NewString("foo")}); err != nil {
		t.Fatal(err)
	}
	if hits["any"] != 1 {
		t.Errorf("string should hit any-overload: hits=%v", hits)
	}
}

// --- Output capture ------------------------------------------------------

func TestOutputCapture(t *testing.T) {
	// Register an "emit" word that writes to r.Output. Confirm we can
	// redirect it to a builder and read back the data — this verifies
	// the engine threads r.Output through to handlers.
	var buf strings.Builder
	r, _ := core.NewRegistry()
	r.Output = &buf
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "emit",

		Signatures: []core.Signature{{
			Args: []*core.Type{core.TString},
			Impl: core.Go(func(args []core.Value, _ map[string]core.Value, _ []core.Value, reg *core.Registry) ([]core.Value, error) {
				s, _ := core.AsString(args[0])
				reg.Output.Write([]byte(s))
				return nil, nil
			}),
			Returns: []*core.Type{}, BarrierPos: -1,
		}},
	})
	r.InitRootContext()

	if _, err := core.NewTop(r).Run([]core.Value{core.NewWord("emit"), core.NewString("hello world")}); err != nil {
		t.Fatal(err)
	}
	if buf.String() != "hello world" {
		t.Errorf("captured output = %q, want %q", buf.String(), "hello world")
	}
}

// --- Error reporting ----------------------------------------------------

func TestBoruErrorPropagation(t *testing.T) {
	// A handler that explicitly returns a BoruError must surface that
	// error from Run with the same code.
	r, _ := core.NewRegistry()
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "bork",

		Signatures: []core.Signature{{
			Args: []*core.Type{core.TInteger},
			Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, reg *core.Registry) ([]core.Value, error) {
				return nil, reg.BoruError("test_failure", "always fails", "bork")
			}), BarrierPos: -1,
		}},
	})
	r.InitRootContext()

	_, err := core.NewTop(r).Run([]core.Value{core.NewWord("bork"), core.NewInteger(0)})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "test_failure") {
		t.Errorf("error should carry the code, got %v", err)
	}
}

// --- Value helpers ------------------------------------------------------

func TestRequireConcreteList(t *testing.T) {
	concrete := core.NewList([]core.Value{core.NewInteger(1), core.NewInteger(2)})
	rl, err := core.RequireConcreteList(concrete, "test")
	if err != nil {
		t.Fatalf("concrete list rejected: %v", err)
	}
	if rl.Len() != 2 {
		t.Errorf("len = %d, want 2", rl.Len())
	}

	// A bare TList literal must be rejected.
	lit := core.NewTypeLiteral(core.TList)
	if _, err := core.RequireConcreteList(lit, "test"); err == nil {
		t.Error("expected error for type literal, got nil")
	}
}

func TestNewReadList(t *testing.T) {
	// External constructor: this is the only way to build a ReadList
	// from outside eng.
	src := []core.Value{core.NewInteger(1), core.NewInteger(2), core.NewInteger(3)}
	rl := core.NewReadList(src)
	if rl.Len() != 3 {
		t.Fatalf("len = %d, want 3", rl.Len())
	}
	got, _ := core.AsInteger(rl.Get(1))
	if got != 2 {
		t.Errorf("Get(1) = %d, want 2", got)
	}
}

// --- DefaultFormats moved to the host package; verify formats slot is empty ---

func TestRegistryFormatsStartEmpty(t *testing.T) {
	// core.NewRegistry deliberately exposes no host concerns —
	// no formats, no file ops, no SQL store, only a generic
	// capability slot. The host package wires every external
	// service in via Registry.SetCapability before running user
	// code. Pinned here so future drift surfaces in CI.
	r, _ := core.NewRegistry()
	names, err := r.Capabilities.Names()
	if err != nil {
		t.Fatalf("Names: %v", err)
	}
	if len(names) != 0 {
		t.Errorf("expected zero capabilities on a fresh registry, got %v", names)
	}
}
