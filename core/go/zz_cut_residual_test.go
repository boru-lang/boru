package core

// Residual pins and helpers assembled at the core cut (triage output).

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"testing"
)

var (
	_ = errors.New
	_ = fmt.Sprint
	_ = math.Pi
	_ = sort.Ints
	_ = strings.TrimSpace
	_ sync.Mutex
	_ = testing.Short
)

// w8reg builds a fresh registry with the root context initialised — the
// standard fixture for kernel unit tests that drive registry-aware paths.
func w8reg(t *testing.T) *Registry {
	t.Helper()
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	r.InitRootContext()
	return r
}

// covRegistry builds a fresh registry with covWords plus any extra setup.
func covRegistry(t *testing.T, extra func(*Registry)) *Registry {
	t.Helper()
	r, err := NewRegistry()
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

// renderAll flattens results for comparison.
func renderAll(vs []Value) string {
	parts := make([]string, len(vs))
	for i, v := range vs {
		parts[i] = v.String()
	}
	return strings.Join(parts, " | ")
}

type w4CountRecorder struct {
	lits, calls int
}

func (c *w4CountRecorder) OnPushLit(Value)         { c.lits++ }
func (c *w4CountRecorder) OnCall(string, int, int) { c.calls++ }

// --- small pure helpers ---------------------------------------------------------------------

func TestSigOrderArgsShapes(t *testing.T) {
	a, b, c := NewInteger(1), NewInteger(2), NewInteger(3)
	// All-forward (nStack 0): unchanged.
	out := SigOrderArgs([]Value{a, b, c}, 0)
	if renderAll(out) != "1 | 2 | 3" {
		t.Errorf("all-forward = %s", renderAll(out))
	}
	// Two stack args below one forward: forward first, stack reversed.
	out = SigOrderArgs([]Value{a, b, c}, 2)
	if renderAll(out) != "3 | 2 | 1" {
		t.Errorf("mixed = %s", renderAll(out))
	}
	// Out-of-range nStack clamps to all-stack.
	out = SigOrderArgs([]Value{a, b}, 5)
	if renderAll(out) != "2 | 1" {
		t.Errorf("clamped = %s", renderAll(out))
	}
}

// runWith creates a fresh registry, applies the supplied setup fn (which
// typically registers a few test words), then parses the input slice and
// runs it. Tests in this file exercise the engine via the public native
// registration API only — no boru parser, no built-in word library.
func runWith(t *testing.T, setup func(*Registry), input []Value) []Value {
	t.Helper()
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	if setup != nil {
		setup(r)
	}
	if err := r.Err(); err != nil {
		t.Fatalf("registration: %v", err)
	}
	r.InitRootContext()
	out, err := NewTop(r).Run(input)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	return out
}

// registerAdd is a tiny native word that adds two integers.
// Used as the canonical "engine works at all" probe.
func registerAdd(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name: "add",

		Signatures: []Signature{{
			Args: []*Type{TInteger, TInteger},
			Impl: Go(func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				a, _ := AsInteger(args[0])
				b, _ := AsInteger(args[1])
				return []Value{NewInteger(a + b)}, nil
			}),
			Returns: []*Type{TInteger}, BarrierPos: -1,
		}},
	})
}

func seam7Reg(t *testing.T) *Registry {
	t.Helper()
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	r.InitRootContext()
	return r
}

// covWords registers a small vocabulary rich enough to exercise forward
// and stack collection, polymorphic dispatch, string/list/map operands,
// and a deliberately failing word for the runtime-error path.
func covWords(r *Registry) {
	intBin := func(name string, f func(a, b int64) int64) {
		r.RegisterNativeFunc(NativeFunc{
			Name: name,
			Signatures: []Signature{{
				Args: []*Type{TInteger, TInteger},
				Impl: Go(func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
					a, _ := AsInteger(args[0])
					b, _ := AsInteger(args[1])
					return []Value{NewInteger(f(a, b))}, nil
				}),
				Returns: []*Type{TInteger}, BarrierPos: -1,
			}},
		})
	}
	intBin("cadd", func(a, b int64) int64 { return a + b })
	intBin("cmul", func(a, b int64) int64 { return a * b })

	r.RegisterNativeFunc(NativeFunc{
		Name: "cneg",
		Signatures: []Signature{{
			Args: []*Type{TInteger},
			Impl: Go(func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				n, _ := AsInteger(args[0])
				return []Value{NewInteger(-n)}, nil
			}),
			Returns: []*Type{TInteger}, BarrierPos: 0,
		}},
	})

	r.RegisterNativeFunc(NativeFunc{
		Name: "ccat",
		Signatures: []Signature{{
			Args: []*Type{TString, TString},
			Impl: Go(func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				a, _ := AsString(args[0])
				b, _ := AsString(args[1])
				return []Value{NewString(a + b)}, nil
			}),
			Returns: []*Type{TString}, BarrierPos: -1,
		}},
	})

	// Polymorphic: Integer doubles, String repeats.
	r.RegisterNativeFunc(NativeFunc{
		Name: "cdub",
		Signatures: []Signature{
			{
				Args: []*Type{TInteger},
				Impl: Go(func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
					n, _ := AsInteger(args[0])
					return []Value{NewInteger(2 * n)}, nil
				}),
				Returns: []*Type{TInteger}, BarrierPos: -1,
			},
			{
				Args: []*Type{TString},
				Impl: Go(func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
					s, _ := AsString(args[0])
					return []Value{NewString(s + s)}, nil
				}),
				Returns: []*Type{TString}, BarrierPos: -1,
			},
		},
	})

	r.RegisterNativeFunc(NativeFunc{
		Name: "clen",
		Signatures: []Signature{{
			Args: []*Type{TList},
			Impl: Go(func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				elems, err := AsList(args[0])
				if err != nil {
					return nil, err
				}
				return []Value{NewInteger(int64(elems.Len()))}, nil
			}),
			Returns: []*Type{TInteger}, BarrierPos: -1,
		}},
	})

	// Frame-tail plumbing words. The engine's fn-frame tail references
	// `__pa` (pop the per-call Args/FnBaselines stacks) and force-forward
	// `undef <param>` pairs; both words live in the language layer, so an
	// eng-only registry supplies minimal equivalents (mirroring
	// lang/go/native/native_definition.go).
	r.RegisterNativeFunc(NativeFunc{
		Name: "__pa",
		Signatures: []Signature{{
			Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
				if err := PopFrameArgs(r); err != nil {
					return nil, err
				}
				return nil, nil
			}),
			Returns: []*Type{}, BarrierPos: -1,
		}},
	})
	undefImpl := Go(func(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
		name := ""
		if IsWord(args[0]) {
			w, _ := AsWord(args[0])
			name = w.Name
		} else if IsAtom(args[0]) {
			name, _ = AsAtom(args[0])
		} else {
			name, _ = AsString(args[0])
		}
		r.Check.Recorder().DeclineCarriedUndef(name)
		UninstallDef(r, name)
		return nil, nil
	}, RunInCheck())
	r.RegisterNativeFunc(NativeFunc{
		Name: "undef",
		Signatures: []Signature{
			{
				Args:       []*Type{TAtom},
				QuoteArgs:  map[int]bool{0: true},
				Impl:       undefImpl,
				Returns:    []*Type{},
				BarrierPos: -1,
			},
		},
	})

	// User fns installed by the same path `def name fn […]` takes.
	InstallFnDef(r, "ctwice", FnDefInfo{
		Signatures: []Signature{{
			Params:     []FnParam{{Name: "n", Type: TInteger}},
			Returns:    []*Type{TInteger},
			Impl:       Boru([]Value{NewOpenParen(), NewWord("cadd"), NewWord("n"), NewWord("n"), NewCloseParen()}),
			BarrierPos: BarrierAllForward,
		}},
	})
	InstallFnDef(r, "cquad", FnDefInfo{
		Signatures: []Signature{{
			Params:     []FnParam{{Name: "n", Type: TInteger}},
			Returns:    []*Type{TInteger},
			Impl:       Boru([]Value{NewOpenParen(), NewWord("ctwice"), NewOpenParen(), NewWord("ctwice"), NewWord("n"), NewCloseParen(), NewCloseParen()}),
			BarrierPos: BarrierAllForward,
		}},
	})
}
