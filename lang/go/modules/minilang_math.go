package modules

import (
	"errors"
	"fmt"
	"math"

	"github.com/boru-lang/boru/lang/go/native"
	tabnasexpr "github.com/tabnas/expr/go"
	jsonic "github.com/tabnas/jsonic/go"
)

// The `m` mini-language: evaluate a traditional maths formula whose variables
// are bound by the named params. `mini math 'x*y-z^2' {x:.. y:.. z:..}` parses the
// formula with the tabnas/expr Pratt-parser plugin and folds the resulting AST
// to a number. Operators: `+ - * / %`, `^` (power, right-associative, binding
// tighter than `* /`), unary `+`/`-`, and `( )`.
//
// Numeric coercion follows boru's rules (dual integer/float domain): when every
// operand is integer-domain the result stays an Integer — and division
// truncates, exactly like the `div` word (`x/y` with x=5,y=2 → 2) — while any
// Float operand promotes the whole expression to Float. A formula variable
// carries the boru type of its bound param; an integer-valued numeric literal
// (e.g. the `2` in `z^2`) is integer-domain, a fractional literal is Float.
//
// IMPORTANT: we evaluate the AST OURSELVES rather than via expr's `evaluate`
// option. tabnas/expr v0.2.0 builds a self-referential AST for some malformed
// input (e.g. a trailing operator), and its own evaluation/Simplify then
// recurse without bound into a FATAL stack overflow that recover() cannot
// catch. Our own walk is depth-bounded, so a degenerate tree is a clean
// mini_syntax_error instead of a process crash.

// maxFormulaDepth bounds the AST walk. Real formulas nest only a handful of
// levels; a depth this high is reached only by a cyclic/degenerate tree, which
// we reject well before the Go stack is at risk.
const maxFormulaDepth = 256

// mval is a number in one of two domains: integer (i holds the value) or float
// (f holds it). The domain drives the boru coercion above.
type mval struct {
	i     int64
	f     float64
	isInt bool
}

func (m mval) asFloat() float64 {
	if m.isInt {
		return float64(m.i)
	}
	return m.f
}

// miniMathHandler evaluates the `m` formula in args[0] with the variable
// bindings from the opts map in args[1].
func miniMathHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	src, err := args[0].AsConcreteString()
	if err != nil {
		return nil, r.BoruError("mini_error", "math: src: "+err.Error(), "lang_math")
	}

	// Variable bindings come from the named params (opts). Each must be numeric.
	vars := map[string]mval{}
	if native.IsConcrete(&args[1]) {
		if m, merr := native.AsMap(args[1]); merr == nil && m != nil {
			for _, k := range m.Keys() {
				v, _ := m.Get(k)
				mv, ok := boruToMval(v)
				if !ok {
					return nil, r.BoruErrorHint("mini_error",
						fmt.Sprintf("math: variable %q must be a number, got %s", k, v.String()),
						"lang_math", "bind every formula variable to an Integer or Float param")
				}
				vars[k] = mv
			}
		}
	}

	// Parse-only (with the power operator registered) — NO evaluate callback.
	ast, perr := tabnasexpr.Parse(src, map[string]any{
		"op": map[string]any{
			// Power: tighter than `* /`, right-associative (2^3^2 = 2^9 = 512).
			"power": map[string]any{
				"infix": true, "src": "^", "left": 5000000, "right": 4900000,
			},
		},
	})
	if perr != nil {
		return nil, r.BoruErrorHint("mini_syntax_error", "math: "+native.FirstCleanLine(perr.Error()),
			"lang_math", "write a maths formula like x*y-z^2 with variables bound via opts")
	}

	out, eerr := evalFormula(ast, vars, 0)
	if eerr != nil {
		if errors.Is(eerr, errMalformedFormula) {
			return nil, r.BoruErrorHint("mini_syntax_error", "math: malformed formula", "lang_math",
				"write a maths formula like x*y-z^2 with variables bound via opts")
		}
		return nil, r.BoruErrorHint("mini_eval_error", "math: "+eerr.Error(), "lang_math",
			"check the operators and that every variable is bound in opts")
	}
	if out.isInt {
		return []native.Value{native.NewInteger(out.i)}, nil
	}
	return []native.Value{native.NewFloat(out.f)}, nil
}

var errMalformedFormula = errors.New("malformed formula")

// evalFormula folds the expr AST to a single mval. A node is an operator
// application (`*jsonic.ListRef`/`jsonic.ListRef` whose first element is an
// `*expr.Op`), a numeric literal (`float64`), or a variable name (`string`).
// depth bounds the recursion against degenerate (cyclic) trees.
func evalFormula(node any, vars map[string]mval, depth int) (mval, error) {
	if depth > maxFormulaDepth {
		return mval{}, errMalformedFormula
	}
	switch n := node.(type) {
	case *jsonic.ListRef:
		return evalApply(n.Val, vars, depth)
	case jsonic.ListRef:
		return evalApply(n.Val, vars, depth)
	case float64:
		if n == math.Trunc(n) && !math.IsInf(n, 0) && !math.IsNaN(n) {
			return mval{i: int64(n), isInt: true}, nil
		}
		return mval{f: n}, nil
	case string:
		mv, ok := vars[n]
		if !ok {
			return mval{}, fmt.Errorf("unknown variable %q", n)
		}
		return mv, nil
	default:
		return mval{}, errMalformedFormula
	}
}

// evalApply applies the operator at val[0] to the operands at val[1:].
func evalApply(val []any, vars map[string]mval, depth int) (mval, error) {
	if len(val) == 0 {
		return mval{}, errMalformedFormula
	}
	op, ok := val[0].(*tabnasexpr.Op)
	if !ok {
		return mval{}, errMalformedFormula
	}
	operands := val[1:]

	switch op.Name {
	case "plain-paren", "positive-prefix":
		if len(operands) != 1 {
			return mval{}, errMalformedFormula
		}
		return evalFormula(operands[0], vars, depth+1)
	case "negative-prefix":
		if len(operands) != 1 {
			return mval{}, errMalformedFormula
		}
		a, err := evalFormula(operands[0], vars, depth+1)
		if err != nil {
			return mval{}, err
		}
		if a.isInt {
			return mval{i: -a.i, isInt: true}, nil
		}
		return mval{f: -a.f}, nil
	}

	if len(operands) != 2 {
		return mval{}, errMalformedFormula
	}
	a, err := evalFormula(operands[0], vars, depth+1)
	if err != nil {
		return mval{}, err
	}
	b, err := evalFormula(operands[1], vars, depth+1)
	if err != nil {
		return mval{}, err
	}
	bothInt := a.isInt && b.isInt

	switch op.Name {
	case "addition-infix":
		if bothInt {
			return mval{i: a.i + b.i, isInt: true}, nil
		}
		return mval{f: a.asFloat() + b.asFloat()}, nil
	case "subtraction-infix":
		if bothInt {
			return mval{i: a.i - b.i, isInt: true}, nil
		}
		return mval{f: a.asFloat() - b.asFloat()}, nil
	case "multiplication-infix":
		if bothInt {
			return mval{i: a.i * b.i, isInt: true}, nil
		}
		return mval{f: a.asFloat() * b.asFloat()}, nil
	case "division-infix":
		if bothInt {
			if b.i == 0 {
				return mval{}, errors.New("division by zero")
			}
			return mval{i: a.i / b.i, isInt: true}, nil // truncating, like `div`
		}
		if b.asFloat() == 0 {
			return mval{}, errors.New("division by zero")
		}
		return mval{f: a.asFloat() / b.asFloat()}, nil
	case "remainder-infix":
		if bothInt {
			if b.i == 0 {
				return mval{}, errors.New("division by zero")
			}
			return mval{i: a.i % b.i, isInt: true}, nil
		}
		if b.asFloat() == 0 {
			return mval{}, errors.New("division by zero")
		}
		return mval{f: math.Mod(a.asFloat(), b.asFloat())}, nil
	case "power-infix":
		if bothInt && b.i >= 0 {
			return mval{i: ipow(a.i, b.i), isInt: true}, nil
		}
		return mval{f: math.Pow(a.asFloat(), b.asFloat())}, nil
	}
	return mval{}, fmt.Errorf("unsupported operator %q", op.Name)
}

// boruToMval coerces a bound param value to an mval, preserving its boru numeric
// domain (Integer → integer-domain, Float → float-domain).
func boruToMval(v native.Value) (mval, bool) {
	if !native.IsConcrete(&v) {
		return mval{}, false
	}
	switch {
	case native.ParentOf(&v).ConformsTo(native.TInteger):
		n, err := native.AsInteger(v)
		if err != nil {
			return mval{}, false
		}
		return mval{i: n, isInt: true}, true
	case native.ParentOf(&v).ConformsTo(native.TFloat):
		f, err := native.AsFloat(v)
		if err != nil {
			return mval{}, false
		}
		return mval{f: f}, true
	}
	return mval{}, false
}

// ipow is integer exponentiation by squaring (exp >= 0).
func ipow(base, exp int64) int64 {
	result := int64(1)
	for exp > 0 {
		if exp&1 == 1 {
			result *= base
		}
		base *= base
		exp >>= 1
	}
	return result
}
