package check

// Standalone-gate tests for check_fnbody.go's remaining arms: the baked
// call-target note's guard, the construction-time body check's
// once-per-body memo and its silence over a volunteered (anonymous) body
// that cannot run, the return-pattern refinement deferral, and the
// imprecise-carrier arm of the recovered call's param narrowing.

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// gsFrozenRec records NoteFrozenRead over the inactive recorder.
type gsFrozenRec struct {
	core.EmitRecorder
	noted []string
}

func (f *gsFrozenRec) NoteFrozenRead(name string, bake core.FrozenBake, _ int64) {
	if bake == core.FrozenBakeCall {
		f.noted = append(f.noted, name)
	}
}

// noteBakedCallTarget notes a MODULE-scope call target a compiled unit is
// about to bake, so a later rebind of the name re-records the caller. A
// fn-local binding is no module name (nothing to note), and the guard arm —
// no dispatching registry, or no name — notes nothing at all.
func TestGateStandaloneNoteBakedCallTarget(t *testing.T) {
	r := newTestRegistry(t)
	rec := &gsFrozenRec{EmitRecorder: core.TheInactiveEmit}

	noteBakedCallTarget(rec, r, "zgsmodfn")
	if len(rec.noted) != 1 || rec.noted[0] != "zgsmodfn" {
		t.Fatalf("a module-scope target must be noted once, got %v", rec.noted)
	}

	r.PushFnBaseline(r.Defs.Snapshot())
	r.Defs.Push("zgslocalfn", bsFn(core.NewInteger(1)))
	noteBakedCallTarget(rec, r, "zgslocalfn")
	r.Defs.Pop("zgslocalfn")
	r.PopFnBaseline()
	if len(rec.noted) != 1 {
		t.Errorf("a fn-local target is no module name, got %v", rec.noted)
	}

	noteBakedCallTarget(rec, nil, "zgsmodfn")
	noteBakedCallTarget(rec, r, "")
	if len(rec.noted) != 1 {
		t.Errorf("no registry or no name notes nothing, got %v", rec.noted)
	}
}

// gsRegisterNoteBoom installs `zgsnoteboomq`, a RunInCheck fixture that
// records an info diagnostic and then fails — a body analysis that reports
// something and then cannot proceed.
func gsRegisterNoteBoom(r *core.Registry) {
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "zgsnoteboomq",
		Signatures: []core.Signature{{
			Returns:    []*core.Type{},
			BarrierPos: -1,
			Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, reg *core.Registry) ([]core.Value, error) {
				reg.Check.AddDiagnostic(core.CheckDiagnostic{Code: "zgs_note", Detail: "zgs: noted", Severity: core.SeverityInfo})
				return nil, &core.BoruError{Code: "zgs_error", Detail: "zgs: always fails"}
			}, core.RunInCheck()),
		}},
	})
}

// gsCountDiag counts the pass's diagnostics with the code.
func gsCountDiag(r *core.Registry, code string) int {
	n := 0
	for _, d := range r.Check.Diagnostics {
		if d.Code == code {
			n++
		}
	}
	return n
}

// gsBodyFn is a one-signature boru fn whose body is the single token tok.
func gsBodyFn(tok core.Value) core.FnDefInfo {
	return core.FnDefInfo{Signatures: []core.Signature{{Impl: core.Boru([]core.Value{tok})}}}
}

// checkFnBodyAtConstruction analyses each body ONCE per pass: a fn is
// checked where its value is constructed and again where it is installed
// under a name, and the two differ only in the name. A body with a source
// position is memoised by it; a body with none (every synthesized body
// shares the zero position) is analysed every time.
func TestGateStandaloneConstructionCheckOncePerBody(t *testing.T) {
	r := zcaRegistry(t)
	gsRegisterNoteBoom(r)
	done := r.Check.Begin()
	defer done()

	positioned := gsBodyFn(s5beAt(core.NewWord("zgsnoteboomq"), 4, 2))
	checkFnBodyAtConstruction(r, "zgsonce", positioned)
	checkFnBodyAtConstruction(r, "zgsoncealias", positioned)
	if n := gsCountDiag(r, "zgs_note"); n != 1 {
		t.Fatalf("a positioned body is analysed once per pass, its note came out %d times", n)
	}

	unpositioned := gsBodyFn(core.NewWord("zgsnoteboomq"))
	checkFnBodyAtConstruction(r, "zgsany1", unpositioned)
	checkFnBodyAtConstruction(r, "zgsany2", unpositioned)
	if n := gsCountDiag(r, "zgs_note"); n != 3 {
		t.Errorf("a position-less body is not memoised: want 3 notes in all, got %d", n)
	}
}

// A body the checker VOLUNTEERED to look at — an anonymous fn value nobody
// named (name "") — that cannot be analysed reports nothing about the
// analysis itself: its fn_body_error is dropped, while every other finding
// of the body stands. A NAMED fn keeps the fn_body_error (defining a fn is
// asking for its body to be analysed).
func TestGateStandaloneConstructionCheckAnonymousBodyError(t *testing.T) {
	r := zcaRegistry(t)
	gsRegisterNoteBoom(r)
	done := r.Check.Begin()
	defer done()

	checkFnBodyAtConstruction(r, "", gsBodyFn(s5beAt(core.NewWord("zgsnoteboomq"), 6, 1)))
	if gsCountDiag(r, "fn_body_error") != 0 {
		t.Error("an anonymous body's analysis failure must stay silent")
	}
	if gsCountDiag(r, "zgs_note") != 1 {
		t.Error("the anonymous body's other findings must be kept")
	}

	checkFnBodyAtConstruction(r, "zgsnamed", gsBodyFn(s5beAt(core.NewWord("zgsnoteboomq"), 7, 1)))
	if gsCountDiag(r, "fn_body_error") != 1 {
		t.Error("a named fn's body analysis failure is reported")
	}
}

// refinementUndecided: a value-level refinement return pattern
// (`[(Integer gt 3)]`, or a union carrying one) over a residual the pass
// does not know is the run's to decide (NUR309). A compile-time-known
// scalar, a residual provably outside the refinement's base, a union with
// no refinement and a plain type pattern all decide statically.
func TestGateStandaloneRefinementUndecided(t *testing.T) {
	big := core.NewDepScalar(core.DepGT, core.NewInteger(3))
	abstractInt := core.NewCarrier(core.TInteger)

	if !refinementUndecided(big, abstractInt) {
		t.Error("an abstract Integer against (Integer gt 3) is the run's to decide")
	}
	if refinementUndecided(big, core.NewInteger(5)) {
		t.Error("a compile-time-known scalar decides")
	}
	if refinementUndecided(big, core.NewCarrier(core.TString)) {
		t.Error("a residual outside the refinement's base decides")
	}
	withRefinement := core.NewDisjunct([]core.Value{core.NewTypeLiteral(core.TString), big})
	if !refinementUndecided(withRefinement, abstractInt) {
		t.Error("a union carrying a refinement defers like the refinement")
	}
	plainUnion := core.NewDisjunct([]core.Value{core.NewTypeLiteral(core.TString), core.NewTypeLiteral(core.TBoolean)})
	if refinementUndecided(plainUnion, abstractInt) {
		t.Error("a union with no refinement decides")
	}
	if refinementUndecided(core.NewTypeLiteral(core.TInteger), abstractInt) {
		t.Error("a plain type pattern decides")
	}
}

// narrowToDeclaredParam's IMPRECISE arm: a recovered call's carrier whose
// static tag does not conform to a NOMINAL param (a join settled on Map
// where the run holds a List) compiles the body against the declared type,
// keeping the carrier's modality. A GENERIC fn's param (a type variable the
// generic lane binds), a CONSTRAINED param (a predicate the nominal guard
// cannot enforce), a conforming carrier and a concrete value all keep the
// argument as it is.
func TestGateStandaloneNarrowImpreciseCarrier(t *testing.T) {
	nc, ok := narrowToDeclaredParam(core.TList, core.NewCarrier(core.TMap), false)
	if !ok || !core.IsCarrier(&nc) || core.IsDynamic(&nc) || !core.ParentOf(&nc).Equal(core.TList) {
		t.Fatalf("an imprecise Map carrier at a List param = (%#v, %v), want a strict List carrier", nc, ok)
	}
	nc, ok = narrowToDeclaredParam(core.TList, core.NewDynamicCarrier(core.TMap), false)
	if !ok || !core.IsDynamic(&nc) || !core.ParentOf(&nc).Equal(core.TList) {
		t.Errorf("a gradual imprecise carrier narrows and stays gradual, got (%#v, %v)", nc, ok)
	}

	r := newTestRegistry(t)
	pred := covPredicateType(t, r, "ZgsNarrowPred")
	for name, c := range map[string]struct {
		pt      *core.Type
		a       core.Value
		generic bool
	}{
		"generic fn":         {core.TList, core.NewCarrier(core.TMap), true},
		"constrained param":  {pred, core.NewCarrier(core.TString), false},
		"conforming carrier": {core.TList, core.NewCarrier(core.TList), false},
		"concrete value":     {core.TList, core.NewInteger(1), false},
	} {
		if _, ok := narrowToDeclaredParam(c.pt, c.a, c.generic); ok {
			t.Errorf("%s: the argument must be kept as it is", name)
		}
	}
}
