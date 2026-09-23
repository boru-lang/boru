package eng

import (
	"errors"
	"strings"
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// TestReStepLandingCandidateRaises pins the landing's uncalled_function arm
// (NUR186): a NAMED fn value with no zero-argument overload, landed with a
// CANDIDATE after it (the op's argument — a word, or a fn frame's tail
// markers) and nothing beneath it in the frame, raises the interpreter's own
// `call to 'inc' matched no signature`; without the candidate it stays
// data, with a value beneath it the residual arm decides, and an anonymous
// lambda parks whatever follows.
func TestReStepLandingCandidateRaises(t *testing.T) {
	r := seam7Reg(t)
	vc := &vmContext{p: landingProg(), r: r, ceiling: 1 << 20, stepLimit: 1 << 20}
	oneArg := []core.Signature{{Params: []core.FnParam{{Type: core.TInteger}}, BarrierPos: 1}}
	inc := core.NewFunction(core.FnDefInfo{Name: "inc", Signatures: oneArg})

	_, ent, err := vc.reStepLanding(r, 1, 0, []core.Value{inc}, seam7Dbg, 0, compiler.LandingWord{})
	var be *core.BoruError
	if ent != nil || !errors.As(err, &be) || be.Code != "uncalled_function" ||
		!strings.Contains(be.Detail, "call to 'inc' matched no signature") || !strings.Contains(be.Hint, "inc/v") {
		t.Fatalf("a named fn over a candidate and an empty frame raises the interpreter's error, got %v %v", ent, err)
	}

	got, ent, err := vc.reStepLanding(r, 0, 0, []core.Value{inc}, seam7Dbg, 0, compiler.LandingWord{})
	if err != nil || ent != nil || len(got) != 1 {
		t.Errorf("no candidate: the value stays data, got %v %v %v", got, ent, err)
	}

	got, ent, err = vc.reStepLanding(r, 1, 0, []core.Value{core.NewInteger(7), inc}, seam7Dbg, 0, compiler.LandingWord{})
	if err != nil || ent != nil || len(got) != 2 {
		t.Errorf("a value beneath in the frame: the residual arm's, got %v %v %v", got, ent, err)
	}

	got, ent, err = vc.reStepLanding(r, 1, 1, []core.Value{core.NewInteger(7), inc}, seam7Dbg, 0, compiler.LandingWord{})
	if ent != nil || !errors.As(err, &be) || be.Code != "uncalled_function" {
		t.Errorf("a value BELOW the frame base is the caller's: the frame is empty and the raise stands, got %v %v %v", got, ent, err)
	}

	lam := core.NewFunction(core.FnDefInfo{Anonymous: true, Signatures: oneArg})
	got, ent, err = vc.reStepLanding(r, 1, 0, []core.Value{lam}, seam7Dbg, 0, compiler.LandingWord{})
	if err != nil || ent != nil || len(got) != 1 {
		t.Errorf("an anonymous lambda parks: got %v %v %v", got, ent, err)
	}
}

func TestUncalledFunctionErrorWithoutRegistry(t *testing.T) {
	err := uncalledFunctionError(nil, core.FnDefInfo{Name: "g"})
	var be *core.BoruError
	if !errors.As(err, &be) || be.Code != "uncalled_function" || be.FullSource != "" {
		t.Errorf("no registry: no source text, got %v", err)
	}
}

// walkReg is a registry holding the two kinds of FUNCTION WORD the landing's
// walk can meet after a value: `z`, a def-bound user fn (a zero-argument
// Integer producer), and `zz-walk-native`, a registered native.
func walkReg(t *testing.T) *core.Registry {
	t.Helper()
	r := seam7Reg(t)
	r.Register("zz-walk-native", core.Signature{BarrierPos: -1, Returns: []*core.Type{core.TInteger},
		Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
			return []core.Value{core.NewInteger(0)}, nil
		})})
	if err := r.Err(); err != nil {
		t.Fatalf("registration: %v", err)
	}
	r.Defs.Push("z", core.NewFunction(core.FnDefInfo{Name: "z", Signatures: []core.Signature{{BarrierPos: -1, Returns: []*core.Type{core.TInteger}, Impl: core.Boru([]core.Value{core.NewInteger(0)})}}}))
	return r
}

// TestReStepLandingWalk pins the landing over a FUNCTION WORD (NUR190): the
// walk runs the interpreter's own plan over the value and the word, and
// answers as the interpreter's re-step does — a named fn with no overload
// taking the word raises, an anonymous one parks INERT (the later residual
// apply leaves it), a mixed overload fires its zero-argument fallback, an
// Any-typed slot's speculative claim raises the stranded-forward error, a
// `/q` slot's capture stands aside (the open half), and a Function-typed
// slot's reference bails loudly. Without a word beside the op, with a value
// beneath, or over a fn with no signatures, the wordless landing decides.
func TestReStepLandingWalk(t *testing.T) {
	r := walkReg(t)
	vc := &vmContext{p: landingProg(), r: r, ceiling: 1 << 20, stepLimit: 1 << 20}
	z := compiler.LandingWord{Name: "z", Pos: core.SrcPos{Row: 1, Col: 9}}
	native := compiler.LandingWord{Name: "zz-walk-native"}
	oneInt := core.Signature{Params: []core.FnParam{{Type: core.TInteger}}, BarrierPos: 1, Returns: []*core.Type{core.TInteger}}
	var be *core.BoruError

	// A named fn whose only overload stops at the word: the raise.
	inc := core.NewFunction(core.FnDefInfo{Name: "inc", Signatures: []core.Signature{oneInt}})
	for _, w := range []compiler.LandingWord{z, native} {
		_, ent, err := vc.reStepLanding(r, 3, 0, []core.Value{inc}, seam7Dbg, 0, w)
		if ent != nil || !errors.As(err, &be) || be.Code != "uncalled_function" {
			t.Errorf("%s after a named one-arg fn: the interpreter raises, got %v %v", w.Name, ent, err)
		}
	}

	// An anonymous fn parks — inert, so the residual apply leaves it as data.
	lam := core.NewFunction(core.FnDefInfo{Anonymous: true, Signatures: []core.Signature{oneInt}})
	got, ent, err := vc.reStepLanding(r, 3, 0, []core.Value{lam}, seam7Dbg, 0, z)
	if err != nil || ent != nil || len(got) != 1 || !got[0].Quoted || !got[0].Parent.Equal(core.TFunction) {
		t.Errorf("an anonymous fn parks inert: got %v %v %v", got, ent, err)
	}
	after, ent, err := vc.callDynamic(r, 1, false, []core.Value{got[0], core.NewInteger(0)}, seam7Dbg, 0)
	if err != nil || ent != nil || len(after) != 2 || !after[0].Parent.Equal(core.TFunction) {
		t.Errorf("the residual apply leaves the parked value as data: %v %v %v", after, ent, err)
	}
	rot, ent, err := vc.callDynamic(r, 1, true, []core.Value{got[0], core.NewInteger(0)}, seam7Dbg, 0)
	if err != nil || ent != nil || len(rot) != 2 || !rot[1].Parent.Equal(core.TFunction) {
		t.Errorf("the trailing apply rotates the parked value back on top: %v %v %v", rot, ent, err)
	}

	// A mixed overload — a zero-argument native and a one-arg — fires the
	// zero-argument fallback where the wordless landing stood aside.
	rn, fire := landingNativeReg(t, "zz-walk-fire", func() ([]core.Value, error) { return []core.Value{core.NewInteger(42)}, nil })
	rn.Defs.Push("z", core.NewFunction(core.FnDefInfo{Name: "z", Signatures: []core.Signature{{BarrierPos: -1, Returns: []*core.Type{core.TInteger}, Impl: core.Boru([]core.Value{core.NewInteger(0)})}}}))
	fd, _ := fire.Data.(core.FnDefInfo)
	fd.Signatures = append(append([]core.Signature(nil), fd.Signatures...), oneInt)
	mixed := core.NewFunction(fd)
	vcn := &vmContext{p: landingProg(), r: rn, ceiling: 1 << 20, stepLimit: 1 << 20}
	got, ent, err = vcn.reStepLanding(rn, 3, 0, []core.Value{mixed}, seam7Dbg, 0, z)
	if err != nil || ent != nil || len(got) != 1 {
		t.Fatalf("the zero-argument fallback fires: %v %v %v", got, ent, err)
	}
	if n, _ := got[0].AsConcreteInteger(); n != 42 {
		t.Errorf("the fallback's 42, got %v", got[0])
	}
	got, ent, err = vcn.reStepLanding(rn, 1, 0, []core.Value{mixed}, seam7Dbg, 0, compiler.LandingWord{})
	if err != nil || ent != nil || len(got) != 1 || !got[0].Parent.Equal(core.TFunction) {
		t.Errorf("no word beside the op: the wordless landing stands aside for a mixed overload, got %v %v %v", got, ent, err)
	}
	// An ANONYMOUS value whose plan is the zero-argument fallback parks
	// (ADR-016), as the wordless landing parks it — the sweep's `if true
	// (mk) [2]` under `error` answered 1 for `[fn]` while this was missing.
	fd.Anonymous, fd.Name = true, ""
	anon0 := core.NewFunction(fd)
	got, ent, err = vcn.reStepLanding(rn, 3, 0, []core.Value{anon0}, seam7Dbg, 0, z)
	if err != nil || ent != nil || len(got) != 1 || !got[0].Parent.Equal(core.TFunction) || got[0].Quoted {
		t.Errorf("an anonymous 0-arg value parks under a word: got %v %v %v", got, ent, err)
	}

	// An Any-typed slot claims the word speculatively: the strict barrier's
	// stranded-forward error, pointing at the word.
	anyFn := core.NewFunction(core.FnDefInfo{Name: "a", Signatures: []core.Signature{{Params: []core.FnParam{{Type: core.TAny}}, BarrierPos: 1, Returns: []*core.Type{core.TAny}}}})
	_, ent, err = vc.reStepLanding(r, 3, 0, []core.Value{anyFn}, seam7Dbg, 0, z)
	if ent != nil || !errors.As(err, &be) || be.Code != "signature_error" || !strings.Contains(be.Detail, "`z` begins its own dispatch") || be.Row != 1 || be.Col != 9 {
		t.Errorf("a speculative claim strands at the word: got %v %v", ent, err)
	}

	// A `/q` slot captures the word: the open half stands aside, unquoted.
	quoteFn := core.NewFunction(core.FnDefInfo{Name: "q", Signatures: []core.Signature{{Params: []core.FnParam{{Type: core.TAtom, Quote: true}}, QuoteArgs: map[int]bool{0: true}, BarrierPos: 1, Returns: []*core.Type{core.TAtom}}}})
	got, ent, err = vc.reStepLanding(r, 3, 0, []core.Value{quoteFn}, seam7Dbg, 0, z)
	if err != nil || ent != nil || len(got) != 1 || got[0].Quoted {
		t.Errorf("a /q capture stands aside (NUR190's open half): got %v %v %v", got, ent, err)
	}

	// A Function-typed slot takes the word's reference: the run bails loudly.
	refFn := core.NewFunction(core.FnDefInfo{Name: "g", Signatures: []core.Signature{{Params: []core.FnParam{{Type: core.TFunction}}, BarrierPos: 1, Returns: []*core.Type{core.TInteger}}}})
	_, ent, err = vc.reStepLanding(r, 3, 0, []core.Value{refFn}, seam7Dbg, 0, z)
	wantInternal(t, err, "takes the word `z` as its argument")
	if ent != nil {
		t.Errorf("the reference claim bails, got an entry %v", ent)
	}

	// The wordless landing decides with a value beneath, and over a fn with
	// no signatures at all.
	got, ent, err = vc.reStepLanding(r, 3, 0, []core.Value{core.NewInteger(7), refFn}, seam7Dbg, 0, z)
	if err != nil || ent != nil || len(got) != 2 {
		t.Errorf("a value beneath: the residual arm's, got %v %v %v", got, ent, err)
	}
	noSigs := core.NewFunction(core.FnDefInfo{Name: "h"})
	got, ent, err = vc.reStepLanding(r, 3, 0, []core.Value{noSigs}, seam7Dbg, 0, z)
	if err != nil || ent != nil || len(got) != 1 {
		t.Errorf("no signatures: nothing to walk, got %v %v %v", got, ent, err)
	}
}

func TestLandingWordAt(t *testing.T) {
	if landingWordAt(nil, 0, 0).Name != "" {
		t.Error("no program, no word")
	}
	p := &compiler.Program{LandingWords: map[int]compiler.LandingWord{3: {Name: "z"}}, Fns: []compiler.CompiledFn{{LandingWords: map[int]compiler.LandingWord{1: {Name: "y"}}}}}
	if landingWordAt(p, -1, 3).Name != "z" || landingWordAt(p, 0, 1).Name != "y" {
		t.Error("the main code's and a unit's tables")
	}
	if landingWordAt(p, 5, 1).Name != "" {
		t.Error("a unit out of range has no table")
	}
}
