package check

// Pure test helpers copied at the carve.

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// Pure test helpers copied at the carve.

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

func ljImplSig(impl core.SigImpl, args ...*core.Type) core.Signature {
	return core.Signature{Args: args, BarrierPos: -1, Impl: impl}
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
