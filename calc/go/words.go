package calc

import (
	"fmt"
	"io"
	"math"

	core "github.com/boru-lang/boru/core/go"
)

// RegisterWords installs the calculator's word vocabulary on r. The set is
// deliberately small and self-contained: every word is defined here, with
// no dependency on eng's optional core-word library. This proves that the
// eng kernel — Registry + dispatch + parser + signature matching — is
// usable as a host library by a module that brings its own words.
//
// out is where `print` and `show` write their output; pass os.Stdout for
// the CLI or a *bytes.Buffer in tests.
func RegisterWords(r *core.Registry, out io.Writer) {
	registerArith(r)
	registerUnary(r)
	registerConstants(r)
	registerStackOps(r)
	registerDisplay(r, out)
}

// numHandler runs op on two numeric args (Integer or Float). If either
// arg is Float the result is Float; otherwise Integer division falls
// back to Float when op signals fractional output. div is the one
// exception: it always returns Float so 1 div 2 = 0.5 rather than 0.
func numHandler(op func(a, b float64) (float64, error), preferInt bool) core.Handler {
	return func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
		a, err := core.AsNumber(args[0])
		if err != nil {
			return nil, err
		}
		b, err := core.AsNumber(args[1])
		if err != nil {
			return nil, err
		}
		res, err := op(a, b)
		if err != nil {
			return nil, err
		}
		if preferInt &&
			args[0].Parent().ConformsTo(core.TInteger) &&
			args[1].Parent().ConformsTo(core.TInteger) &&
			res == math.Trunc(res) &&
			!math.IsInf(res, 0) &&
			!math.IsNaN(res) {
			return []core.Value{core.NewInteger(int64(res))}, nil
		}
		return []core.Value{core.NewFloat(res)}, nil
	}
}

func registerArith(r *core.Registry) {
	// Args are [a b] in surface order ("a sub b" => sig=[b,a]); to match
	// the engine convention every binary handler computes args[1] op args[0]
	// so the infix form reads naturally. See lang/CLAUDE.md "Non-commutative
	// two-arg sanity check" for the reasoning.
	bin := func(name string, op func(a, b float64) (float64, error), preferInt bool) {
		h := numHandler(op, preferInt)
		r.RegisterNativeFunc(core.NativeFunc{
			Name: name,

			Signatures: []core.Signature{
				{Args: []*core.Type{core.TNumber, core.TNumber}, Impl: core.Go(h), Returns: []*core.Type{core.TNumber}, BarrierPos: -1},
			},
		})
	}
	bin("add", func(a, b float64) (float64, error) { return b + a, nil }, true)
	bin("sub", func(a, b float64) (float64, error) { return b - a, nil }, true)
	bin("mul", func(a, b float64) (float64, error) { return b * a, nil }, true)
	bin("div", func(a, b float64) (float64, error) {
		if a == 0 {
			return 0, fmt.Errorf("div: division by zero")
		}
		return b / a, nil
	}, false)
	bin("mod", func(a, b float64) (float64, error) {
		if a == 0 {
			return 0, fmt.Errorf("mod: modulo by zero")
		}
		return math.Mod(b, a), nil
	}, true)
	bin("pow", func(a, b float64) (float64, error) {
		return math.Pow(b, a), nil
	}, true)
}

func registerUnary(r *core.Registry) {
	unary := func(name string, op func(float64) (float64, error), preferInt bool) {
		h := func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
			x, err := core.AsNumber(args[0])
			if err != nil {
				return nil, err
			}
			res, err := op(x)
			if err != nil {
				return nil, err
			}
			if preferInt && args[0].Parent().ConformsTo(core.TInteger) && res == math.Trunc(res) {
				return []core.Value{core.NewInteger(int64(res))}, nil
			}
			return []core.Value{core.NewFloat(res)}, nil
		}
		r.RegisterNativeFunc(core.NativeFunc{
			Name: name,

			Signatures: []core.Signature{
				{Args: []*core.Type{core.TNumber}, Impl: core.Go(h), Returns: []*core.Type{core.TNumber}, BarrierPos: -1},
			},
		})
	}
	unary("neg", func(x float64) (float64, error) { return -x, nil }, true)
	unary("abs", func(x float64) (float64, error) { return math.Abs(x), nil }, true)
	unary("sqrt", func(x float64) (float64, error) {
		if x < 0 {
			return 0, fmt.Errorf("sqrt: negative input")
		}
		return math.Sqrt(x), nil
	}, false)
}

func registerConstants(r *core.Registry) {
	push := func(name string, v core.Value) {
		r.RegisterNativeFunc(core.NativeFunc{
			Name: name,
			Signatures: []core.Signature{{
				Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
					return []core.Value{v}, nil
				}),
				Returns: []*core.Type{core.TNumber}, BarrierPos: 0,
			}},
		})
	}
	push("pi", core.NewFloat(math.Pi))
	push("e", core.NewFloat(math.E))
}

func registerStackOps(r *core.Registry) {
	// dup / drop / swap / over operate on the full stack via FullStack
	// signatures. Calc keeps its stack vocabulary small — the production
	// language layer in lang/ has the full Forth-style set.
	full := func(name string, n int, fn func(stk []core.Value) ([]core.Value, error)) {
		r.RegisterNativeFunc(core.NativeFunc{
			Name: name,
			Signatures: []core.Signature{{
				Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, stk []core.Value, _ *core.Registry) ([]core.Value, error) {
					if len(stk) < n {
						return nil, fmt.Errorf("%s: needs %d items, stack has %d", name, n, len(stk))
					}
					return fn(stk)
				}, core.FullStack()),
				Returns: []*core.Type{}, BarrierPos: 0,
			}},
		})
	}
	full("dup", 1, func(stk []core.Value) ([]core.Value, error) {
		return append(append([]core.Value{}, stk...), stk[len(stk)-1]), nil
	})
	full("drop", 1, func(stk []core.Value) ([]core.Value, error) {
		return append([]core.Value{}, stk[:len(stk)-1]...), nil
	})
	full("swap", 2, func(stk []core.Value) ([]core.Value, error) {
		out := append([]core.Value{}, stk...)
		i := len(out) - 1
		out[i], out[i-1] = out[i-1], out[i]
		return out, nil
	})
	full("over", 2, func(stk []core.Value) ([]core.Value, error) {
		return append(append([]core.Value{}, stk...), stk[len(stk)-2]), nil
	})
	full("clear", 0, func(_ []core.Value) ([]core.Value, error) {
		return []core.Value{}, nil
	})
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "depth",
		Signatures: []core.Signature{{
			Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, stk []core.Value, _ *core.Registry) ([]core.Value, error) {
				out := append([]core.Value{}, stk...)
				return append(out, core.NewInteger(int64(len(stk)))), nil
			}, core.FullStack()),
			Returns: []*core.Type{core.TInteger}, BarrierPos: 0,
		}},
	})
}

func registerDisplay(r *core.Registry, out io.Writer) {
	if out == nil {
		out = io.Discard
	}
	// print — consume the top value and write its String() representation
	// followed by a newline. Returns nothing.
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "print",
		Signatures: []core.Signature{{
			Args: []*core.Type{core.TAny},
			Impl: core.Go(func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
				fmt.Fprintln(out, args[0].String())
				return nil, nil
			}),
			Returns: []*core.Type{}, BarrierPos: 0,
		}},
	})
	// show — write the full stack without consuming it. The output is one
	// space-separated line followed by a newline, matching the REPL's
	// end-of-line printout for non-interactive inspection.
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "show",
		Signatures: []core.Signature{{
			Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, stk []core.Value, _ *core.Registry) ([]core.Value, error) {
				parts := make([]string, len(stk))
				for i, v := range stk {
					parts[i] = v.String()
				}
				if len(parts) == 0 {
					fmt.Fprintln(out, "(empty)")
				} else {
					for i, p := range parts {
						if i > 0 {
							fmt.Fprint(out, " ")
						}
						fmt.Fprint(out, p)
					}
					fmt.Fprintln(out)
				}
				return append([]core.Value{}, stk...), nil
			}, core.FullStack()),
			Returns: []*core.Type{}, BarrierPos: 0,
		}},
	})
}
