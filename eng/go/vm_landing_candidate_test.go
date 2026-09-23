package eng

import (
	"errors"
	"strings"
	"testing"

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

	_, ent, err := vc.reStepLanding(r, 1, 0, []core.Value{inc}, seam7Dbg, 0)
	var be *core.BoruError
	if ent != nil || !errors.As(err, &be) || be.Code != "uncalled_function" ||
		!strings.Contains(be.Detail, "call to 'inc' matched no signature") || !strings.Contains(be.Hint, "inc/v") {
		t.Fatalf("a named fn over a candidate and an empty frame raises the interpreter's error, got %v %v", ent, err)
	}

	got, ent, err := vc.reStepLanding(r, 0, 0, []core.Value{inc}, seam7Dbg, 0)
	if err != nil || ent != nil || len(got) != 1 {
		t.Errorf("no candidate: the value stays data, got %v %v %v", got, ent, err)
	}

	got, ent, err = vc.reStepLanding(r, 1, 0, []core.Value{core.NewInteger(7), inc}, seam7Dbg, 0)
	if err != nil || ent != nil || len(got) != 2 {
		t.Errorf("a value beneath in the frame: the residual arm's, got %v %v %v", got, ent, err)
	}

	got, ent, err = vc.reStepLanding(r, 1, 1, []core.Value{core.NewInteger(7), inc}, seam7Dbg, 0)
	if ent != nil || !errors.As(err, &be) || be.Code != "uncalled_function" {
		t.Errorf("a value BELOW the frame base is the caller's: the frame is empty and the raise stands, got %v %v %v", got, ent, err)
	}

	lam := core.NewFunction(core.FnDefInfo{Anonymous: true, Signatures: oneArg})
	got, ent, err = vc.reStepLanding(r, 1, 0, []core.Value{lam}, seam7Dbg, 0)
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
