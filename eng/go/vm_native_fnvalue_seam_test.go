package eng

import (
	"strconv"
	"testing"

	"github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// goValueWith builds a self-contained Go-implemented fn value (fn-util's
// wrapper shape: anonymous, Go handlers only) with one signature per arity
// given: the one-param arm multiplies by ten, the two-param arm subtracts
// the second from the first.
func goValueWith(arities ...int) core.Value {
	var sigs []core.Signature
	for _, n := range arities {
		params := make([]core.FnParam, n)
		for i := range params {
			params[i] = core.FnParam{Type: core.TInteger}
		}
		k := n
		sig := core.Signature{Params: params, BarrierPos: n, Impl: core.Go(func(a []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
			x, _ := core.AsInteger(a[0])
			if k == 1 {
				return []core.Value{core.NewInteger(x * 10)}, nil
			}
			y, _ := core.AsInteger(a[1])
			return []core.Value{core.NewInteger(x - y)}, nil
		})}
		core.NormalizeSig(&sig)
		sigs = append(sigs, sig)
	}
	return core.NewFunction(core.FnDefInfo{Name: "w", Anonymous: true, Signatures: sigs, MaxForwardArgs: arities[len(arities)-1]})
}

// TestApplyNativeFnValueTopDown pins the token seam's native arm for a
// Go-implemented fn value: the widest fitting arity takes its inputs from
// the top of the seam's window, bound top-down (the top input → the first
// param), the inputs beneath stay as the residual, and a window nothing
// fits declines to the caller's interpreter fallback.
func TestApplyNativeFnValueTopDown(t *testing.T) {
	r := seam7Reg(t)
	vc := &vmContext{p: &compiler.Program{}, r: r, ceiling: 1 << 20, stepLimit: 1 << 20}
	ints := func(ns ...int) []core.Value {
		out := make([]core.Value, len(ns))
		for i, n := range ns {
			out[i] = core.NewInteger(int64(n))
		}
		return out
	}
	one := goValueWith(1)
	fd, _ := one.Data.(core.FnDefInfo)
	if !vmNativeApplicable(r, fd) {
		t.Fatal("a self-contained Go fn value applies natively")
	}
	out, done, err := vc.applyNativeFnValueTopDown(one, fd, ints(1, 2, 3))
	if !done || err != nil || len(out) != 3 || renderInts(out) != "[1 2 30]" {
		t.Errorf("a one-param value takes the top input and leaves the rest: %v %v %v", out, done, err)
	}
	both := goValueWith(1, 2)
	fd2, _ := both.Data.(core.FnDefInfo)
	out, done, err = vc.applyNativeFnValueTopDown(both, fd2, ints(7, 3))
	if !done || err != nil || renderInts(out) != "[-4]" {
		t.Errorf("the widest fitting arity binds top-down (the top 3 is the first param): %v %v %v", out, done, err)
	}
	out, done, err = vc.applyNativeFnValueTopDown(both, fd2, ints(5))
	if !done || err != nil || renderInts(out) != "[50]" {
		t.Errorf("with one input only the one-param arm fits: %v %v %v", out, done, err)
	}
	if _, done, _ := vc.applyNativeFnValueTopDown(one, fd, []core.Value{core.NewString("s")}); done {
		t.Error("a window nothing fits declines to the interpreter fallback")
	}
	if _, done, _ := vc.applyNativeFnValueTopDown(one, fd, nil); done {
		t.Error("an empty window fits no arm")
	}
}

func renderInts(vs []core.Value) string {
	s := "["
	for i, v := range vs {
		if i > 0 {
			s += " "
		}
		n, _ := core.AsInteger(v)
		s += strconv.Itoa(int(n))
	}
	return s + "]"
}
