package eng

import (
	"strings"
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// barredRegistry registers zzbar: a `[String | Map]` overload (barrier 1)
// and a `[String Integer]` one (barrier 2) — get's Node and Error rows in
// miniature.
func barredRegistry(t *testing.T) *core.Registry {
	t.Helper()
	r := newTestRegistry(t)
	impl := core.Go(func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
		return []core.Value{core.NewString("took")}, nil
	})
	r.Register("zzbar",
		core.Signature{Args: []*core.Type{core.TString, core.TMap}, BarrierPos: 1, Impl: impl},
		core.Signature{Args: []*core.Type{core.TString, core.TInteger}, BarrierPos: 2, Impl: impl},
	)
	if err := r.Err(); err != nil {
		t.Fatal(err)
	}
	return r
}

// intArmIndex is the index of zzbar's `[String Integer]` overload in its
// sorted table.
func intArmIndex(t *testing.T, fd *core.FnDefInfo) int {
	t.Helper()
	for i := range fd.Signatures {
		if fd.Signatures[i].BarrierPos == 2 {
			return i
		}
	}
	t.Fatal("zzbar has no barrier-2 overload")
	return -1
}

// TestBarredPolyPlansTheWindow pins NUR362's run-time half: a poly whose
// written operands reach past a candidate's barrier plans its window over
// the recorded layout instead of matching it flat. Both operands written
// with nothing beneath: a Map second is `[String | Map]`'s only if it came
// off the stack — no plan, the interpreter's signature_error (the flat
// match took it); an Integer second is the barrier-2 overload's own window,
// dispatched. A window the plan cannot walk, or a layout whose live value
// has no home, defers.
func TestBarredPolyPlansTheWindow(t *testing.T) {
	r := barredRegistry(t)
	vc := &vmContext{r: r}
	fn := r.Lookup("zzbar")
	pr := &compiler.PolyRef{Word: "zzbar", Arity: 2, NOut: 1, Split: &compiler.PolySplit{NFwd: 2}}
	if !polyBarred(pr, fn.Signatures) || polyBarred(&compiler.PolyRef{Word: "zzbar"}, fn.Signatures) {
		t.Fatal("barred with its layout's two written operands, never without a layout")
	}
	m := core.NewMap(core.NewOrderedMap())
	if _, err := vc.callPolyPlanned(r, pr, fn, []core.Value{core.NewString("k"), m}, []core.Value{m, core.NewString("k")}, nil, 0); err == nil || !strings.Contains(err.Error(), "signature_error") {
		t.Errorf("`zzbar \"k\" {}`: the interpreter's no-match, got %v", err)
	}
	got, err := vc.callPolyPlanned(r, pr, fn, []core.Value{core.NewString("k"), core.NewInteger(5)}, []core.Value{core.NewInteger(5), core.NewString("k")}, nil, 0)
	if err != nil || len(got) != 1 {
		t.Fatalf("`zzbar \"k\" 5`: the barrier-2 overload over the call's window, got %v %v", got, err)
	}
	if s, _ := core.AsString(got[0]); s != "took" {
		t.Errorf("`zzbar \"k\" 5`: the handler's result, got %v", got)
	}
	tpl := core.NewInterpString([]core.InterpPart{{Lit: "a"}, {Expr: []core.Value{core.NewInteger(1)}}})
	if _, err := vc.callPolyPlanned(r, pr, fn, []core.Value{tpl, core.NewInteger(5)}, []core.Value{core.NewInteger(5), tpl}, nil, 0); err == nil || !strings.Contains(err.Error(), "NUR362") {
		t.Errorf("an unplannable window defers, got %v", err)
	}
	live := &compiler.PolyRef{Word: "zzbar", Arity: 2, NOut: 1, Split: &compiler.PolySplit{NFwd: 2, Beneath: []core.Value{core.NewInteger(1)}, Live: []compiler.SplitLive{{At: 5}}}}
	if _, err := vc.callPolyPlanned(r, live, fn, []core.Value{core.NewString("k"), m}, []core.Value{m, core.NewString("k")}, nil, 0); err == nil || !strings.Contains(err.Error(), "NUR362") {
		t.Errorf("a live value with no home defers, got %v", err)
	}
}

// TestBarredUserPolyPlansTheWindow is the user-poly twin (UserPolyRef.Split):
// the plan's overload enters its arm's unit, no plan raises at the word's
// position, and a plan outside the recorded arms — or no plan the host can
// walk — defers.
func TestBarredUserPolyPlansTheWindow(t *testing.T) {
	r := barredRegistry(t)
	vc := &vmContext{r: r}
	fd := r.Lookup("zzbar")
	intArm := intArmIndex(t, fd)
	pr := &compiler.UserPolyRef{Word: "zzbar", Arity: 2, SigIdx: []int{1 - intArm, intArm}, Split: &compiler.PolySplit{NFwd: 2}, WordPos: core.SrcPos{Row: 1, Col: 1}}
	units := []int{7, 8}
	m := core.NewMap(core.NewOrderedMap())
	if _, _, err := vc.matchUserPolyPlanned(pr, fd, units, []core.Value{core.NewString("k"), m}, nil, nil, 0); err == nil || !strings.Contains(err.Error(), "signature_error") {
		t.Errorf("no plan: the interpreter's no-match, got %v", err)
	}
	window := []core.Value{core.NewString("k"), core.NewInteger(5)}
	if u, args, err := vc.matchUserPolyPlanned(pr, fd, units, window, nil, nil, 0); err != nil || u != 8 || len(args) != 2 {
		t.Errorf("the plan's arm: got unit %d args %v err %v", u, args, err)
	}
	pr.SigIdx = pr.SigIdx[:1]
	if _, _, err := vc.matchUserPolyPlanned(pr, fd, units[:1], window, nil, nil, 0); err == nil || !strings.Contains(err.Error(), "NUR362") {
		t.Errorf("a plan outside the recorded arms defers, got %v", err)
	}
}

// TestPlanTakesWindow pins the window test: the plan's positions must be
// the written operands after the word, then the stack operands beneath it
// top first, over an overload of the window's arity that dispatches.
func TestPlanTakesWindow(t *testing.T) {
	r := barredRegistry(t)
	fd := r.Lookup("zzbar")
	sig := &fd.Signatures[intArmIndex(t, fd)]
	// Tape [s0 word w0]: the word at 1, written w0 at 2, stack s0 at 0.
	if !planTakesWindow(sig, []int{2, 0}, 0, 1, 2) {
		t.Error("the window's own positions")
	}
	for _, tc := range []struct {
		sig       *core.Signature
		positions []int
	}{
		{nil, []int{2, 0}},
		{sig, []int{2}},
		{sig, []int{2, 1}},
		{&core.Signature{Args: []*core.Type{core.TString}}, []int{2}},
	} {
		if planTakesWindow(tc.sig, tc.positions, 0, 1, 2) {
			t.Errorf("%v at %v is not the window", tc.sig, tc.positions)
		}
	}
}
