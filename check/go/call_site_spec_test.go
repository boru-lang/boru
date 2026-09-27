package check

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// specFn is a one-signature constant fn with an identity.
func specFn(name string) core.Value {
	return core.NewFunction(core.FnDefInfo{Name: name, Signatures: []core.Signature{{}}})
}

// A param/arg pair specialises only as a NAMED Function param handed a
// constant fn that has an identity and nothing beyond its signatures.
func TestSpecialisableFnArg(t *testing.T) {
	r, _ := core.NewRegistry()
	other, _ := core.NewRegistry()
	gParam := core.FnParam{Name: "g", Type: core.TFunction}
	if _, ok := specialisableFnArg(r, gParam, specFn("inc")); !ok {
		t.Fatal("a constant fn to a named Function param specialises")
	}
	pattern := core.NewInteger(1)
	withFn := func(edit func(*core.FnDefInfo)) core.Value {
		fd := core.FnDefInfo{Name: "inc", Signatures: []core.Signature{{}}}
		edit(&fd)
		return core.NewFunction(fd)
	}
	carrier := specFn("inc")
	carrier.Carrier = true
	for name, c := range map[string]struct {
		p core.FnParam
		a core.Value
	}{
		"unnamed param":    {core.FnParam{Type: core.TFunction}, specFn("inc")},
		"Any param":        {core.FnParam{Name: "g", Type: core.TAny}, specFn("inc")},
		"untyped param":    {core.FnParam{Name: "g"}, specFn("inc")},
		"patterned param":  {core.FnParam{Name: "g", Type: core.TFunction, Pattern: &pattern}, specFn("inc")},
		"carrier arg":      {gParam, carrier},
		"data arg":         {gParam, core.NewInteger(1)},
		"no identity":      {gParam, core.Value{Parent: core.TFunction, Data: core.FnDefInfo{Name: "x"}}},
		"capturing fn":     {gParam, withFn(func(fd *core.FnDefInfo) { fd.Captured = []core.CapturedBinding{{Name: "c"}} })},
		"generic fn":       {gParam, withFn(func(fd *core.FnDefInfo) { fd.Gen = &core.GenSpecInfo{} })},
		"macro fn":         {gParam, withFn(func(fd *core.FnDefInfo) { fd.Macro = true })},
		"modifier wrapper": {gParam, withFn(func(fd *core.FnDefInfo) { fd.Wrap = core.WrapReverse })},
		"foreign home":     {gParam, withFn(func(fd *core.FnDefInfo) { fd.Registry = other })},
	} {
		if _, ok := specialisableFnArg(r, c.p, c.a); ok {
			t.Errorf("%s: must not specialise", name)
		}
	}
}

// specialisedArgs keeps each qualifying constant fn (under a fresh value ID),
// generalises the rest as the generic unit does, and names them in the key
// suffix; with none qualifying there is nothing to specialise.
func TestSpecialisedArgs(t *testing.T) {
	params := []core.FnParam{{Name: "g", Type: core.TFunction}, {Name: "n", Type: core.TInteger}}
	fn := specFn("inc")
	gen := []core.Value{core.NewCarrier(core.TFunction), core.NewCarrier(core.TInteger)}
	r, _ := core.NewRegistry()
	args, ps, fns, suffix := specialisedArgs(r, params, []core.Value{fn, core.NewInteger(3)}, gen)
	if len(args) != 2 || !core.ExactEqual(args[0], fn) || args[0].ID == fn.ID || args[1].ID != gen[1].ID {
		t.Errorf("want the fn kept under a fresh ID and the rest generalised, got %v", args)
	}
	if len(ps) != 1 || ps[0] != 0 || len(fns) != 1 || suffix == "" {
		t.Errorf("want one guard on param 0 and a suffix, got %v %v %q", ps, fns, suffix)
	}
	if args, _, _, _ := specialisedArgs(r, params, []core.Value{core.NewInteger(1), core.NewInteger(3)}, gen); args != nil {
		t.Errorf("no constant fn arg: nothing to specialise, got %v", args)
	}
}

// The residual check: nothing declared always meets; a count miss or a
// strict value outside its declared type misses; a gradual value is a bound,
// never a miss.
func TestSpecResidualMeetsReturns(t *testing.T) {
	ints := []*core.Type{core.TInteger}
	dyn := core.NewDynamicCarrier(core.TString)
	for name, c := range map[string]struct {
		residual []core.Value
		returns  []*core.Type
		want     bool
	}{
		"nothing declared": {[]core.Value{core.NewInteger(1), core.NewInteger(2)}, nil, true},
		"count miss":       {[]core.Value{core.NewInteger(1), core.NewInteger(2)}, ints, false},
		"strict miss":      {[]core.Value{core.NewCarrier(core.TString)}, ints, false},
		"gradual":          {[]core.Value{dyn}, ints, true},
		"conforming":       {[]core.Value{core.NewCarrier(core.TInteger)}, ints, true},
	} {
		if got := specResidualMeetsReturns(c.residual, c.returns); got != c.want {
			t.Errorf("%s: got %v, want %v", name, got, c.want)
		}
	}
}

// specialiseCallSite's early refusals leave the call site to the generic
// unit; a qualifying call compiles its unit (here a stub) and counts it,
// once per (site, fn) however often the site recurs, up to the quota.
func TestSpecialiseCallSiteAdmission(t *testing.T) {
	r, _ := core.NewRegistry()
	es := r.Check.Recorder()
	params := []core.FnParam{{Name: "g", Type: core.TFunction}}
	gen := []core.Value{core.NewCarrier(core.TFunction)}
	compiled := 0
	stub := func([]core.Value, string) int { compiled++; return 7 }
	fd := core.FnDefInfo{Name: "h", Signatures: []core.Signature{{}}}
	body := []core.Value{core.NewWord("g")}
	call := func(fd core.FnDefInfo, args []core.Value) int {
		return specialiseCallSite(r, es, stub, fd, false, "h", body, params, args, gen)
	}
	inc := specFn("inc")
	r.Check.SpecOff = true
	if call(fd, []core.Value{inc}) != -1 {
		t.Error("SpecOff: no specialisation")
	}
	r.Check.SpecOff = false
	multi := fd
	multi.Signatures = []core.Signature{{}, {}}
	generic := fd
	generic.Gen = &core.GenSpecInfo{}
	for name, f := range map[string]core.FnDefInfo{"multi-signature": multi, "generic": generic} {
		if call(f, []core.Value{inc}) != -1 {
			t.Errorf("%s callee: no specialisation", name)
		}
	}
	if specialiseCallSite(r, es, stub, fd, true, "h", body, params, []core.Value{inc}, gen) != -1 {
		t.Error("a foreign-home callee: no specialisation")
	}
	if call(fd, []core.Value{core.NewInteger(1)}) != -1 || compiled != 0 {
		t.Error("no constant fn arg: no specialisation, nothing compiled")
	}
	if call(fd, []core.Value{inc}) != 7 || !r.Check.SpecTried {
		t.Error("a qualifying call records its specialised unit")
	}
	if call(fd, []core.Value{inc}) != 7 {
		t.Error("the same (site, fn) again records its unit, uncounted")
	}
	for i := 0; i < FnSpecQuota; i++ {
		call(fd, []core.Value{specFn("f")})
	}
	if call(fd, []core.Value{specFn("over")}) != -1 {
		t.Errorf("past FnSpecQuota distinct fns the site stops specialising")
	}
}
