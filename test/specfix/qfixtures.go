package specfix

import (
	"fmt"
	"strconv"
	"strings"

	core "github.com/boru-lang/boru/core/go"
)

// RegisterQFixtures installs the shared `…q` spec-runner fixtures
// (addq, subq, mulq, negq, concatq, describeq, tagq, factq, codeq,
// routeq, tripq, pairq, nilq, flexq, lengthq, firstq, plus the
// boundary-coverage tri*/quad*/quint*/hex*/sept* set) on r.
//
// These are NOT production words — they are dispatch / value / type-
// lattice probes. Both engspec (kernel-only setup) and langspec
// (production-language setup) install them so the moved tsv files
// continue to exercise the same dispatch shapes regardless of which
// runner picks them up.
func RegisterQFixtures(r *core.Registry) {
	registerArith(r)
	registerStringProbe(r)
	registerDispatch(r)
	registerBarrierArity(r)
	registerListProbes(r)
}

func toFloat(v core.Value) float64 {
	if v.Parent().ConformsTo(core.TInteger) {
		n, _ := core.AsInteger(v)
		return float64(n)
	}
	f, _ := core.AsFloat(v)
	return f
}

func numericBinary(intOp func(a, b int64) int64, floatOp func(a, b float64) float64) core.Handler {
	return func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
		if args[0].Parent().ConformsTo(core.TInteger) && args[1].Parent().ConformsTo(core.TInteger) {
			a, _ := core.AsInteger(args[0])
			b, _ := core.AsInteger(args[1])
			return []core.Value{core.NewInteger(intOp(a, b))}, nil
		}
		return []core.Value{core.NewFloat(floatOp(toFloat(args[0]), toFloat(args[1])))}, nil
	}
}

func registerArith(r *core.Registry) {
	numberPair := []*core.Type{core.TNumber, core.TNumber}
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "addq",
		Signatures: []core.Signature{{
			Args:    numberPair,
			Impl:    core.Go(numericBinary(func(a, b int64) int64 { return b + a }, func(a, b float64) float64 { return b + a })),
			Returns: []*core.Type{core.TNumber}, BarrierPos: -1,
		}},
	})
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "subq",
		Signatures: []core.Signature{{
			Args:    numberPair,
			Impl:    core.Go(numericBinary(func(a, b int64) int64 { return b - a }, func(a, b float64) float64 { return b - a })),
			Returns: []*core.Type{core.TNumber}, BarrierPos: -1,
		}},
	})
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "mulq",
		Signatures: []core.Signature{{
			Args:    numberPair,
			Impl:    core.Go(numericBinary(func(a, b int64) int64 { return b * a }, func(a, b float64) float64 { return b * a })),
			Returns: []*core.Type{core.TNumber}, BarrierPos: -1,
		}},
	})
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "negq",
		Signatures: []core.Signature{{
			Args: []*core.Type{core.TNumber}, BarrierPos: 1,
			Impl: core.Go(func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
				if args[0].Parent().ConformsTo(core.TInteger) {
					n, _ := core.AsInteger(args[0])
					return []core.Value{core.NewInteger(-n)}, nil
				}
				f, _ := core.AsFloat(args[0])
				return []core.Value{core.NewFloat(-f)}, nil
			}),
			Returns: []*core.Type{core.TNumber},
		}},
	})
}

func registerStringProbe(r *core.Registry) {
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "concatq",
		Signatures: []core.Signature{{
			Args: []*core.Type{core.TString, core.TString},
			Impl: core.Go(func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
				a, _ := core.AsString(args[0])
				b, _ := core.AsString(args[1])
				return []core.Value{core.NewString(b + a)}, nil
			}),
			Returns: []*core.Type{core.TString}, BarrierPos: -1,
		}},
	})
}

func registerDispatch(r *core.Registry) {
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "describeq",
		Signatures: []core.Signature{
			{
				Args: []*core.Type{core.TInteger},
				Impl: core.Go(func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
					n, _ := core.AsInteger(args[0])
					return []core.Value{core.NewString("int:" + strconv.FormatInt(n, 10))}, nil
				}),
				Returns: []*core.Type{core.TString}, BarrierPos: -1,
			},
			{
				Args: []*core.Type{core.TString},
				Impl: core.Go(func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
					s, _ := core.AsString(args[0])
					return []core.Value{core.NewString("str:" + s)}, nil
				}),
				Returns: []*core.Type{core.TString}, BarrierPos: -1,
			},
		},
	})
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "tagq",
		Signatures: []core.Signature{
			{Args: []*core.Type{core.TAny}, Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
				return []core.Value{core.NewString("any")}, nil
			}), Returns: []*core.Type{core.TString}, BarrierPos: -1},
			{Args: []*core.Type{core.TInteger}, Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
				return []core.Value{core.NewString("specific")}, nil
			}), Returns: []*core.Type{core.TString}, BarrierPos: -1},
		},
	})
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "factq",
		Signatures: []core.Signature{
			{
				Args: []*core.Type{core.TInteger}, Patterns: map[int]core.Value{0: core.NewInteger(0)},
				Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
					return []core.Value{core.NewInteger(1)}, nil
				}),
				Returns: []*core.Type{core.TInteger}, BarrierPos: -1,
			},
			{
				Args: []*core.Type{core.TInteger},
				Impl: core.Go(func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
					n, _ := core.AsInteger(args[0])
					return []core.Value{core.NewInteger(n)}, nil
				}),
				Returns: []*core.Type{core.TInteger}, BarrierPos: -1,
			},
		},
	})
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "codeq",
		Signatures: []core.Signature{
			{
				Args: []*core.Type{core.TInteger}, Patterns: map[int]core.Value{0: core.NewInteger(99)},
				Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
					return []core.Value{core.NewString("ninety-nine")}, nil
				}),
				Returns: []*core.Type{core.TString}, BarrierPos: -1,
			},
			{
				Args: []*core.Type{core.TInteger},
				Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
					return []core.Value{core.NewString("general")}, nil
				}),
				Returns: []*core.Type{core.TString}, BarrierPos: -1,
			},
		},
	})
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "routeq",
		Signatures: []core.Signature{
			{
				Args: []*core.Type{core.TString}, Patterns: map[int]core.Value{0: core.NewString("admin")},
				Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
					return []core.Value{core.NewString("matched-admin")}, nil
				}),
				Returns: []*core.Type{core.TString}, BarrierPos: -1,
			},
			{
				Args: []*core.Type{core.TString},
				Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
					return []core.Value{core.NewString("other")}, nil
				}),
				Returns: []*core.Type{core.TString}, BarrierPos: -1,
			},
		},
	})
}

func registerBarrierArity(r *core.Registry) {
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "tripq",
		Signatures: []core.Signature{{
			Args: []*core.Type{core.TInteger, core.TInteger, core.TInteger},
			Impl: core.Go(func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
				a, _ := core.AsInteger(args[0])
				b, _ := core.AsInteger(args[1])
				c, _ := core.AsInteger(args[2])
				return []core.Value{core.NewString(fmt.Sprintf("%d,%d,%d", a, b, c))}, nil
			}),
			Returns: []*core.Type{core.TString}, BarrierPos: -1,
		}},
	})
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "pairq",
		Signatures: []core.Signature{{
			Args:       []*core.Type{core.TInteger, core.TInteger},
			BarrierPos: 1,
			Impl: core.Go(func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
				a, _ := core.AsInteger(args[0])
				b, _ := core.AsInteger(args[1])
				return []core.Value{core.NewString(fmt.Sprintf("%d:%d", a, b))}, nil
			}),
			Returns: []*core.Type{core.TString},
		}},
	})
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "nilq",
		Signatures: []core.Signature{{
			Args: []*core.Type{},
			Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
				return []core.Value{core.NewString("nil")}, nil
			}),
			Returns: []*core.Type{core.TString}, BarrierPos: 0,
		}},
	})
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "flexq",
		Signatures: []core.Signature{
			{
				Args: []*core.Type{core.TInteger},
				Impl: core.Go(func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
					a, _ := core.AsInteger(args[0])
					return []core.Value{core.NewString(fmt.Sprintf("one:%d", a))}, nil
				}),
				Returns: []*core.Type{core.TString}, BarrierPos: -1,
			},
			{
				Args: []*core.Type{core.TInteger, core.TInteger},
				Impl: core.Go(func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
					a, _ := core.AsInteger(args[0])
					b, _ := core.AsInteger(args[1])
					return []core.Value{core.NewString(fmt.Sprintf("two:%d,%d", a, b))}, nil
				}),
				Returns: []*core.Type{core.TString}, BarrierPos: -1,
			},
		},
	})

	intArgsFmt := func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
		parts := make([]string, len(args))
		for i, a := range args {
			n, _ := core.AsInteger(a)
			parts[i] = strconv.FormatInt(n, 10)
		}
		return []core.Value{core.NewString(strings.Join(parts, ","))}, nil
	}
	intArity := func(name string, n, barrier int) {
		args := make([]*core.Type, n)
		for i := range args {
			args[i] = core.TInteger
		}
		r.RegisterNativeFunc(core.NativeFunc{
			Name: name,
			Signatures: []core.Signature{{
				Args: args, BarrierPos: barrier,
				Impl:    core.Go(intArgsFmt),
				Returns: []*core.Type{core.TString},
			}},
		})
	}
	intArity("tri1q", 3, 1)
	intArity("tri2q", 3, 2)
	intArity("quad1q", 4, 1)
	intArity("quadq", 4, 2)
	intArity("quad3q", 4, 3)
	intArity("quintq", 5, 3)
	intArity("hexq", 6, 3)
	intArity("septq", 7, 4)
}

func registerListProbes(r *core.Registry) {
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "lengthq",
		Signatures: []core.Signature{{
			Args: []*core.Type{core.TList},
			Impl: core.Go(func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
				lst, _ := core.AsList(args[0])
				return []core.Value{core.NewInteger(int64(lst.Len()))}, nil
			}),
			Returns: []*core.Type{core.TInteger}, BarrierPos: -1,
		}},
	})
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "firstq",
		Signatures: []core.Signature{{
			Args: []*core.Type{core.TList},
			Impl: core.Go(func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
				lst, _ := core.AsList(args[0])
				if lst.Len() == 0 {
					return []core.Value{core.NewNone()}, nil
				}
				return []core.Value{lst.Get(0)}, nil
			}),
			Returns: []*core.Type{core.TAny}, BarrierPos: -1,
		}},
	})
}
