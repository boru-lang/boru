package modules

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
	"github.com/boru-lang/boru/lang/go/native"
)

// fnUtilHandler returns the named inner native's Go handler (the seam7
// pattern: type_seam7_test.go::s7bTypeHandler).
func fnUtilHandler(t *testing.T, name string) native.Handler {
	t.Helper()
	for _, nf := range fnUtilNatives {
		if nf.Name == name {
			gi, ok := nf.Signatures[0].Impl.(*core.GoImpl)
			if !ok {
				t.Fatalf("%s: Impl not *core.GoImpl", name)
			}
			return gi.Handler
		}
	}
	t.Fatalf("fn-util native %q not found", name)
	return nil
}

func fnUtilReg(t *testing.T) *native.Registry {
	t.Helper()
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// fnTestFn builds an n-param Any fn over a Go handler — the same wrapper
// shape the module's own words produce.
func fnTestFn(n int, h native.Handler) native.Value { return goFnValue("t", n, h) }

// fnAdd1 is a 1-param fn returning arg+1 over Integers.
func fnAdd1() native.Value {
	return fnTestFn(1, func(a []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
		i, err := a[0].AsConcreteInteger()
		if err != nil {
			return nil, err
		}
		return []native.Value{native.NewInteger(i + 1)}, nil
	})
}

// fnTwoOut is a 1-param fn returning TWO values — the invokeFnUtilOne
// pipeline refusal shape, and memoize's multi-return storage shape.
func fnTwoOut() native.Value {
	return fnTestFn(1, func(a []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
		return []native.Value{a[0], a[0]}, nil
	})
}

// fnIntOnly is a 1-param Integer-typed fn, so a String argument matches
// no signature — the invokeFnUtil sig==nil arm.
func fnIntOnly() native.Value {
	params := []core.FnParam{{Type: native.TInteger}}
	sig := core.Signature{Params: params, BarrierPos: 1, Impl: core.Go(
		func(a []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
			return []native.Value{a[0]}, nil
		})}
	core.NormalizeSig(&sig)
	return native.NewFunction(native.FnDefInfo{Name: "intonly", Anonymous: true,
		Signatures: []core.Signature{sig}, MaxForwardArgs: 1})
}

// fnZeroParam is a 0-param fn — partial's nothing-to-bind arm.
func fnZeroParam() native.Value {
	return fnTestFn(0, func(_ []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
		return []native.Value{native.NewInteger(9)}, nil
	})
}

func wantErrContaining(t *testing.T, err error, sub string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), sub) {
		t.Fatalf("want error containing %q, got %v", sub, err)
	}
}

// applyWrapper invokes a produced wrapper value's single Go-impl sig
// directly with args — the runtime dispatch path minus the engine.
func applyWrapper(t *testing.T, r *native.Registry, w native.Value, args ...native.Value) ([]native.Value, error) {
	t.Helper()
	fd, ok := w.Data.(native.FnDefInfo)
	if !ok {
		t.Fatalf("wrapper is not a Function: %v", w)
	}
	gi, ok := fd.Signatures[0].Impl.(*core.GoImpl)
	if !ok {
		t.Fatalf("wrapper Impl not *core.GoImpl")
	}
	return gi.Handler(args, nil, nil, r)
}

// TestFnUtilArgGuards drives every non-function refusal arm the typed
// dispatch cannot reach (the TFunction slots reject non-fns before the
// handler runs — these are the defensive halves).
func TestFnUtilArgGuards(t *testing.T) {
	r := fnUtilReg(t)
	five := native.NewInteger(5)
	fn := fnAdd1()
	for _, tc := range []struct {
		word string
		args []native.Value
	}{
		{"compose", []native.Value{five, fn}},
		{"compose", []native.Value{fn, five}},
		{"pipe", []native.Value{five, fn}},
		{"pipe", []native.Value{fn, five}},
		{"on", []native.Value{five, fn}},
		{"on", []native.Value{fn, five}},
		{"flip", []native.Value{five}},
		{"curry", []native.Value{five}},
		{"partial", []native.Value{five, five}},
		{"memoize", []native.Value{five}},
	} {
		_, err := fnUtilHandler(t, tc.word)(tc.args, nil, nil, r)
		wantErrContaining(t, err, "must be a function value")
	}
}

// TestFnUtilShapeGuards drives the signature-shape refusals reachable
// only with handler-level operands.
func TestFnUtilShapeGuards(t *testing.T) {
	r := fnUtilReg(t)
	// partial: a 0-param fn has nothing to bind.
	_, err := fnUtilHandler(t, "partial")([]native.Value{fnZeroParam(), native.NewInteger(1)}, nil, nil, r)
	wantErrContaining(t, err, "nothing to bind")
	// memoize over a multi-overload fn refuses (the frontier rows pin
	// curry's twin in-language).
	two := native.FnDefInfo{Name: "two", Signatures: []core.Signature{
		fnAdd1().Data.(native.FnDefInfo).Signatures[0],
		fnTwoOut().Data.(native.FnDefInfo).Signatures[0],
	}}
	_, err = fnUtilHandler(t, "memoize")([]native.Value{native.NewFunction(two)}, nil, nil, r)
	wantErrContaining(t, err, "exactly one signature")
	// partial's twin of the same guard.
	_, err = fnUtilHandler(t, "partial")([]native.Value{native.NewFunction(two), native.NewInteger(1)}, nil, nil, r)
	wantErrContaining(t, err, "exactly one signature")
}

// TestFnUtilPipelineArity drives invokeFnUtilOne's two refusal arms: an
// inner fn returning two values, and an inner error propagating.
func TestFnUtilPipelineArity(t *testing.T) {
	r := fnUtilReg(t)
	// compose whose g returns two values → pipeline arity refusal.
	out, err := fnUtilHandler(t, "compose")([]native.Value{fnAdd1(), fnTwoOut()}, nil, nil, r)
	if err != nil {
		t.Fatal(err)
	}
	_, err = applyWrapper(t, r, out[0], native.NewInteger(1))
	wantErrContaining(t, err, "needs exactly 1")
	// pipe whose f rejects its argument → the sig==nil arm propagates.
	out, err = fnUtilHandler(t, "pipe")([]native.Value{fnIntOnly(), fnAdd1()}, nil, nil, r)
	if err != nil {
		t.Fatal(err)
	}
	_, err = applyWrapper(t, r, out[0], native.NewString("x"))
	wantErrContaining(t, err, "no signature of the applied function")
	// on whose projection errors on the second operand → the uy arm.
	out, err = fnUtilHandler(t, "on")([]native.Value{fnAdd1(), fnIntOnly()}, nil, nil, r)
	if err != nil {
		t.Fatal(err)
	}
	_, err = applyWrapper(t, r, out[0], native.NewInteger(1), native.NewString("x"))
	wantErrContaining(t, err, "no signature of the applied function")
	// …and on the FIRST operand → the ux arm.
	_, err = applyWrapper(t, r, out[0], native.NewString("x"), native.NewInteger(1))
	wantErrContaining(t, err, "no signature of the applied function")
	// compose whose g errors inside its handler → the first-stage error
	// propagates through invokeFnUtilOne's err arm.
	out, err = fnUtilHandler(t, "compose")([]native.Value{fnIntOnly(), fnAdd1()}, nil, nil, r)
	if err != nil {
		t.Fatal(err)
	}
	_, err = applyWrapper(t, r, out[0], native.NewString("x"))
	wantErrContaining(t, err, "not an integer value")
}

// TestFnUtilMemoizeCache drives the cache-hit arm and the multi-return
// storage copy — the halves the single-call frontier row leaves dark.
func TestFnUtilMemoizeCache(t *testing.T) {
	r := fnUtilReg(t)
	calls := 0
	counted := fnTestFn(1, func(a []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
		calls++
		return []native.Value{a[0], native.NewInteger(int64(calls))}, nil
	})
	out, err := fnUtilHandler(t, "memoize")([]native.Value{counted}, nil, nil, r)
	if err != nil {
		t.Fatal(err)
	}
	m := out[0]
	first, err := applyWrapper(t, r, m, native.NewInteger(7))
	if err != nil {
		t.Fatal(err)
	}
	second, err := applyWrapper(t, r, m, native.NewInteger(7))
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("memoize re-invoked the fn: %d calls", calls)
	}
	if len(first) != 2 || len(second) != 2 {
		t.Fatalf("multi-return not preserved: %v / %v", first, second)
	}
	if native.Canon(first) != native.Canon(second) {
		t.Fatalf("cache hit diverged: %v vs %v", first, second)
	}
	// A canon-distinct argument re-invokes.
	if _, err := applyWrapper(t, r, m, native.NewInteger(8)); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("distinct argument did not re-invoke: %d calls", calls)
	}
	// The memoized fn's own error propagates uncached.
	bad, err := fnUtilHandler(t, "memoize")([]native.Value{fnIntOnly()}, nil, nil, r)
	if err != nil {
		t.Fatal(err)
	}
	_, err = applyWrapper(t, r, bad[0], native.NewString("x"))
	wantErrContaining(t, err, "no signature")
}

// TestFnUtilCurryChain drives curryLevel's error propagation at the
// final level (a mistyped last argument reaches the original fn's
// dispatch and fails there).
func TestFnUtilCurryChain(t *testing.T) {
	r := fnUtilReg(t)
	twoInts := func() native.Value {
		params := []core.FnParam{{Type: native.TInteger}, {Type: native.TInteger}}
		sig := core.Signature{Params: params, BarrierPos: 2, Impl: core.Go(
			func(a []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				x, err := a[0].AsConcreteInteger()
				if err != nil {
					return nil, err
				}
				y, err := a[1].AsConcreteInteger()
				if err != nil {
					return nil, err
				}
				return []native.Value{native.NewInteger(x - y)}, nil
			})}
		core.NormalizeSig(&sig)
		return native.NewFunction(native.FnDefInfo{Name: "sub2", Anonymous: true,
			Signatures: []core.Signature{sig}, MaxForwardArgs: 2})
	}()
	out, err := fnUtilHandler(t, "curry")([]native.Value{twoInts}, nil, nil, r)
	if err != nil {
		t.Fatal(err)
	}
	level1, err := applyWrapper(t, r, out[0], native.NewInteger(10))
	if err != nil {
		t.Fatal(err)
	}
	res, err := applyWrapper(t, r, level1[0], native.NewInteger(3))
	if err != nil {
		t.Fatal(err)
	}
	if i, _ := res[0].AsConcreteInteger(); i != 7 {
		t.Fatalf("curry chain computed %v, want 7", res[0])
	}
	_, err = applyWrapper(t, r, level1[0], native.NewString("x"))
	wantErrContaining(t, err, "no signature")
	// Sibling partial applications must not share a bound backing array.
	l1b, err := applyWrapper(t, r, out[0], native.NewInteger(100))
	if err != nil {
		t.Fatal(err)
	}
	resA, err := applyWrapper(t, r, level1[0], native.NewInteger(1))
	if err != nil {
		t.Fatal(err)
	}
	resB, err := applyWrapper(t, r, l1b[0], native.NewInteger(1))
	if err != nil {
		t.Fatal(err)
	}
	ia, _ := resA[0].AsConcreteInteger()
	ib, _ := resB[0].AsConcreteInteger()
	if ia != 9 || ib != 99 {
		t.Fatalf("curry siblings shared state: %d / %d, want 9 / 99", ia, ib)
	}
}

// TestFnUtilBoruBody drives invokeFnUtil's InvokeCallbackFn tail with a
// boru-bodied fn (the arm the Go-impl wrappers skip).
func TestFnUtilBoruBody(t *testing.T) {
	r := fnUtilReg(t)
	sig := core.Signature{
		Params: []core.FnParam{{Type: native.TAny}},
		Impl:   native.Boru([]native.Value{native.NewWord("add"), native.NewInteger(1)}),
	}
	core.NormalizeSig(&sig)
	boruFn := native.NewFunction(native.FnDefInfo{Name: "badd1", Anonymous: true,
		Signatures: []core.Signature{sig}, MaxForwardArgs: 1})
	out, err := fnUtilHandler(t, "compose")([]native.Value{fnAdd1(), boruFn}, nil, nil, r)
	if err != nil {
		t.Fatal(err)
	}
	got, err := applyWrapper(t, r, out[0], native.NewInteger(0))
	if err != nil {
		t.Fatal(err)
	}
	if i, _ := got[0].AsConcreteInteger(); i != 2 {
		t.Fatalf("compose over a boru-bodied fn: got %v, want 2", got[0])
	}
}

// TestFnUtilConstIdentity drives the two polymorphic words' handlers
// directly, functions included (identity must NOT invoke its argument).
func TestFnUtilConstIdentity(t *testing.T) {
	r := fnUtilReg(t)
	fn := fnAdd1()
	out, err := fnUtilHandler(t, "identity")([]native.Value{fn}, nil, nil, r)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := out[0].Data.(native.FnDefInfo); !ok {
		t.Fatalf("identity invoked its Function argument: %v", out[0])
	}
	out, err = fnUtilHandler(t, "_f_const")([]native.Value{native.NewString("k")}, nil, nil, r)
	if err != nil {
		t.Fatal(err)
	}
	got, err := applyWrapper(t, r, out[0], native.NewInteger(999))
	if err != nil {
		t.Fatal(err)
	}
	if s, _ := got[0].AsConcreteString(); s != "k" {
		t.Fatalf("const returned %v, want 'k'", got[0])
	}
}

// ---- the check-mode shape claim (the thirty-fifth increment) ----

func TestFnShapeReturnsClaims(t *testing.T) {
	r := fnUtilReg(t)
	defer r.Check.Begin()()
	outs := fnShapeReturns(fnShapeConst(2))(nil, r)
	if len(outs) != 1 || !core.IsFnTypedCarrier(outs[0]) {
		t.Fatalf("the result is one Function carrier, got %v", outs)
	}
	if n, ok := r.Check.FnShapeArity(outs[0].ID); !ok || n != 2 {
		t.Errorf("a constant arity is claimed: %d/%v", n, ok)
	}
	outs = fnShapeReturns(fnShapeFromOperand(0))(nil, r)
	if len(outs) != 1 || !core.IsFnTypedCarrier(outs[0]) {
		t.Fatalf("an unknown arity still mints the carrier, got %v", outs)
	}
	if _, ok := r.Check.FnShapeArity(outs[0].ID); ok {
		t.Error("an unknown arity makes no claim")
	}
	outs = fnShapeReturns(fnShapeConst(1))(nil, nil)
	if len(outs) != 1 || !core.IsFnTypedCarrier(outs[0]) {
		t.Errorf("with no registry the carrier alone is minted, got %v", outs)
	}
}

func TestFnShapeFromOperandArms(t *testing.T) {
	two := fnTestFn(2, func(_ []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
		return nil, nil
	})
	if s, ok := fnShapeFromOperand(-1)([]native.Value{two}); !ok || s.Arity != 1 || s.Result != nil {
		t.Errorf("partial binds one slot: %+v/%v, want arity 1", s, ok)
	}
	if s, ok := fnShapeFromOperand(0)([]native.Value{two}); !ok || s.Arity != 2 {
		t.Errorf("memoize keeps the count: %+v/%v, want arity 2", s, ok)
	}
	// curry: a chain of unary levels, one per param, the last returning the value.
	three := fnTestFn(3, func(_ []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
		return nil, nil
	})
	s, ok := fnShapeCurry()([]native.Value{three})
	if !ok || s.Arity != 1 || s.Result == nil || s.Result.Arity != 1 || s.Result.Result == nil || s.Result.Result.Arity != 1 || s.Result.Result.Result != nil {
		t.Errorf("curry over three params claims three unary levels, got %+v/%v", s, ok)
	}
	one := fnTestFn(1, func(_ []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
		return nil, nil
	})
	if _, ok := fnShapeCurry()([]native.Value{one}); ok {
		t.Error("curry over a unary fn raises — no claim")
	}
	if _, ok := fnShapeCurry()(nil); ok {
		t.Error("no operand, no curry claim")
	}
	if _, ok := fnShapeFromOperand(0)(nil); ok {
		t.Error("no operand, no claim")
	}
	if _, ok := fnShapeFromOperand(0)([]native.Value{native.NewInteger(1)}); ok {
		t.Error("a non-fn operand makes no claim (the handler raises)")
	}
	if _, ok := fnShapeFromOperand(0)([]native.Value{native.NewCarrier(native.TFunction)}); ok {
		t.Error("a computed-fn carrier operand has no signature to read")
	}
	sig := core.Signature{Params: []core.FnParam{{Type: native.TAny}}, BarrierPos: 1}
	multi := native.NewFunction(native.FnDefInfo{Signatures: []core.Signature{sig, sig}})
	if _, ok := fnShapeFromOperand(0)([]native.Value{multi}); ok {
		t.Error("an overloaded operand has no one arity")
	}
}
