package compiler

// Pure test helpers copied at the carve.

// Pure test helpers copied at the carve.

// Pure test helpers copied at the carve.

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// Pure test helpers copied at the carve.

func containsStr(s, sub string) bool { return strings.Contains(s, sub) }

// countingRecorder wraps the inactive no-op recorder and counts every probe
// the check pass makes, proving the checker runs against the EmitRecorder
// INTERFACE — no code path requires the concrete *EmitState (G9 / completion
// plan 4.5). Embedding inactiveEmit supplies the full method set; the
// overridden methods tally the calls that every check pass is guaranteed to
// make.
type countingRecorder struct {
	core.EmitRecorder // the inactive no-op backs the full method set
	activeCalls       int
	armedCalls        int
	recordCalls       int
}

// covRegistry builds a fresh registry with covWords plus any extra setup.
func covRegistry(t *testing.T, extra func(*core.Registry)) *core.Registry {
	t.Helper()
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	covWords(r)
	if extra != nil {
		extra(r)
	}
	if err := r.Err(); err != nil {
		t.Fatalf("registration: %v", err)
	}
	r.InitRootContext()
	return r
}

func ffsConst(id string, n int64) core.Value {
	v := core.NewInteger(n)
	v.ID = id
	return v
}

func fnVal(sigs ...core.Signature) core.Value {
	return core.Value{ID: core.GenerateID(core.IDPrefixForType(core.TFunction)), Parent: core.TFunction, Data: core.FnDefInfo{Signatures: sigs}}
}

// instanceVal builds a flat class instance whose fields map is `om`.
func instanceVal(om *core.OrderedMap) core.Value {
	return core.Value{ID: core.GenerateID(core.IDPrefixForType(core.TAny)), Parent: core.TAny, Data: core.ClassInstanceInfo{Fields: om}}
}

func mapOf2(key string, v core.Value) *core.OrderedMap {
	om := core.NewOrderedMap()
	om.Set(key, v)
	return om
}

func mapOfFields() *core.OrderedMap {
	om := core.NewOrderedMap()
	om.Set("f", zeroArgFn())
	return om
}

// miniShuffleReg registers one shuffle-named native with the given arg types
// and returns the registry plus a pointer to its single signature.
func miniShuffleReg(t *testing.T, name string, argTypes []*core.Type) (*core.Registry, *core.Signature) {
	t.Helper()
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	r.RegisterNativeFunc(core.NativeFunc{
		Name: name,
		Signatures: []core.Signature{{
			Args: argTypes,
			Impl: core.Go(func(a []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
				return a, nil
			}),
			BarrierPos: -1,
		}},
	})
	if err := r.Err(); err != nil {
		t.Fatalf("register %s: %v", name, err)
	}
	return r, &r.Lookup(name).Signatures[0]
}

func newTestRegistry(t *testing.T) *core.Registry {
	t.Helper()
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	r.InitRootContext()
	return r
}

func parenBody(tokens ...core.Value) []core.Value {
	out := []core.Value{core.NewOpenParen()}
	out = append(out, tokens...)
	out = append(out, core.NewCloseParen())
	return out
}

// --- anonymous fn values ----------------------------------------------------

// recorderTestRegistry builds an eng-only registry with one probe native
// (`padd`), enough for a check pass to exercise dispatch through
// recordDispatchOutcome and emit a genuine diagnostic on an unknown word.
func recorderTestRegistry(t *testing.T) *core.Registry {
	t.Helper()
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "padd",
		Signatures: []core.Signature{{
			Args: []*core.Type{core.TInteger, core.TInteger},
			Impl: core.Go(func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
				a, _ := core.AsInteger(args[0])
				b, _ := core.AsInteger(args[1])
				return []core.Value{core.NewInteger(a + b)}, nil
			}),
			Returns:    []*core.Type{core.TInteger},
			BarrierPos: -1,
		}},
	})
	return r
}

// registerIslandWord registers a word with the given compile effect and
// arg count and returns the REGISTERED sig pointer (identity matters).
func registerIslandWord(t *testing.T, r *core.Registry, name string, effect core.CompileEffect, argc int, barrier int) *core.Signature {
	t.Helper()
	args := make([]*core.Type, argc)
	for i := range args {
		args[i] = core.TAny
	}
	r.RegisterNativeFunc(core.NativeFunc{
		Name: name,
		Signatures: []core.Signature{{
			Args:          args,
			CompileEffect: effect,
			Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
				return []core.Value{core.NewInteger(0)}, nil
			}),
			Returns: []*core.Type{core.TInteger}, BarrierPos: barrier,
		}},
	})
	if err := r.Err(); err != nil {
		t.Fatalf("registration of %s: %v", name, err)
	}
	return &r.Lookup(name).Signatures[0]
}

// renderAll flattens results for comparison.
func renderAll(vs []core.Value) string {
	parts := make([]string, len(vs))
	for i, v := range vs {
		parts[i] = v.String()
	}
	return strings.Join(parts, " | ")
}

func runUnitReg(t *testing.T) *core.Registry {
	t.Helper()
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	r.InitRootContext()
	return r
}

func s7MapOf(key string, v core.Value) core.Value {
	om := core.NewOrderedMap()
	om.Set(key, v)
	return core.NewMap(om)
}

func seam7Reg(t *testing.T) *core.Registry {
	t.Helper()
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	r.InitRootContext()
	return r
}

// statefulSig builds a Signature whose handler returns f(callCount).
func statefulSig(effect core.CompileEffect, f func(n int) []core.Value) *core.Signature {
	n := 0
	return &core.Signature{
		CompileEffect: effect,
		Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
			n++
			return f(n), nil
		}),
	}
}

// zeroArgFn is a genuine 0-param fn value (two 0-arg sigs → real + phantom),
// the shape the interpreter auto-dispatches when it lands with no operands.
func zeroArgFn() core.Value { return fnVal(core.Signature{}, core.Signature{}) }

func zzDriftWord() (core.WordInfo, *core.Signature) {
	return core.WordInfo{Name: "add", ArgCount: -1},
		&core.Signature{Args: []*core.Type{core.TInteger, core.TInteger}, BarrierPos: 1}
}

func cifSplice(v core.Value) []core.Value {
	if v.Parent.Equal(core.TList) && v.Data != nil && !core.IsTypedList(v) && !core.IsTableType(v) {
		elems, _ := core.AsList(v)
		out := make([]core.Value, 0, elems.Len()+2)
		out = append(out, core.NewOpenParen())
		out = append(out, elems.Slice()...)
		out = append(out, core.NewCloseParen())
		return out
	}
	return []core.Value{v}
}

// covWords registers a small vocabulary rich enough to exercise forward
// and stack collection, polymorphic dispatch, string/list/map operands,
// and a deliberately failing word for the runtime-error path.
func covWords(r *core.Registry) {
	intBin := func(name string, f func(a, b int64) int64) {
		r.RegisterNativeFunc(core.NativeFunc{
			Name: name,
			Signatures: []core.Signature{{
				Args: []*core.Type{core.TInteger, core.TInteger},
				Impl: core.Go(func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
					a, _ := core.AsInteger(args[0])
					b, _ := core.AsInteger(args[1])
					return []core.Value{core.NewInteger(f(a, b))}, nil
				}),
				Returns: []*core.Type{core.TInteger}, BarrierPos: -1,
			}},
		})
	}
	intBin("cadd", func(a, b int64) int64 { return a + b })
	intBin("cmul", func(a, b int64) int64 { return a * b })

	r.RegisterNativeFunc(core.NativeFunc{
		Name: "cneg",
		Signatures: []core.Signature{{
			Args: []*core.Type{core.TInteger},
			Impl: core.Go(func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
				n, _ := core.AsInteger(args[0])
				return []core.Value{core.NewInteger(-n)}, nil
			}),
			Returns: []*core.Type{core.TInteger}, BarrierPos: 0,
		}},
	})

	r.RegisterNativeFunc(core.NativeFunc{
		Name: "ccat",
		Signatures: []core.Signature{{
			Args: []*core.Type{core.TString, core.TString},
			Impl: core.Go(func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
				a, _ := core.AsString(args[0])
				b, _ := core.AsString(args[1])
				return []core.Value{core.NewString(a + b)}, nil
			}),
			Returns: []*core.Type{core.TString}, BarrierPos: -1,
		}},
	})

	// Polymorphic: Integer doubles, String repeats.
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "cdub",
		Signatures: []core.Signature{
			{
				Args: []*core.Type{core.TInteger},
				Impl: core.Go(func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
					n, _ := core.AsInteger(args[0])
					return []core.Value{core.NewInteger(2 * n)}, nil
				}),
				Returns: []*core.Type{core.TInteger}, BarrierPos: -1,
			},
			{
				Args: []*core.Type{core.TString},
				Impl: core.Go(func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
					s, _ := core.AsString(args[0])
					return []core.Value{core.NewString(s + s)}, nil
				}),
				Returns: []*core.Type{core.TString}, BarrierPos: -1,
			},
		},
	})

	r.RegisterNativeFunc(core.NativeFunc{
		Name: "clen",
		Signatures: []core.Signature{{
			Args: []*core.Type{core.TList},
			Impl: core.Go(func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
				elems, err := core.AsList(args[0])
				if err != nil {
					return nil, err
				}
				return []core.Value{core.NewInteger(int64(elems.Len()))}, nil
			}),
			Returns: []*core.Type{core.TInteger}, BarrierPos: -1,
		}},
	})

	// Frame-tail plumbing words. The engine's fn-frame tail references
	// `__pa` (pop the per-call Args/FnBaselines stacks) and force-forward
	// `undef <param>` pairs; both words live in the language layer, so an
	// eng-only registry supplies minimal equivalents (mirroring
	// lang/go/native/native_definition.go).
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "__pa",
		Signatures: []core.Signature{{
			Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, r *core.Registry) ([]core.Value, error) {
				if err := core.PopFrameArgs(r); err != nil {
					return nil, err
				}
				return nil, nil
			}),
			Returns: []*core.Type{}, BarrierPos: -1,
		}},
	})
	undefImpl := core.Go(func(args []core.Value, _ map[string]core.Value, _ []core.Value, r *core.Registry) ([]core.Value, error) {
		name := ""
		if core.IsWord(args[0]) {
			w, _ := core.AsWord(args[0])
			name = w.Name
		} else if core.IsAtom(args[0]) {
			name, _ = core.AsAtom(args[0])
		} else {
			name, _ = core.AsString(args[0])
		}
		r.Check.Recorder().DeclineCarriedUndef(name)
		core.UninstallDef(r, name)
		return nil, nil
	}, core.RunInCheck())
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "undef",
		Signatures: []core.Signature{
			{
				Args:       []*core.Type{core.TAtom},
				QuoteArgs:  map[int]bool{0: true},
				Impl:       undefImpl,
				Returns:    []*core.Type{},
				BarrierPos: -1,
			},
		},
	})

	// User fns installed by the same path `def name fn […]` takes.
	core.InstallFnDef(r, "ctwice", core.FnDefInfo{
		Signatures: []core.Signature{{
			Params:     []core.FnParam{{Name: "n", Type: core.TInteger}},
			Returns:    []*core.Type{core.TInteger},
			Impl:       core.Boru([]core.Value{core.NewOpenParen(), core.NewWord("cadd"), core.NewWord("n"), core.NewWord("n"), core.NewCloseParen()}),
			BarrierPos: core.BarrierAllForward,
		}},
	})
	core.InstallFnDef(r, "cquad", core.FnDefInfo{
		Signatures: []core.Signature{{
			Params:     []core.FnParam{{Name: "n", Type: core.TInteger}},
			Returns:    []*core.Type{core.TInteger},
			Impl:       core.Boru([]core.Value{core.NewOpenParen(), core.NewWord("ctwice"), core.NewOpenParen(), core.NewWord("ctwice"), core.NewWord("n"), core.NewCloseParen(), core.NewCloseParen()}),
			BarrierPos: core.BarrierAllForward,
		}},
	})
}
