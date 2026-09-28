package check

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// frame_fn_bind_test.go pins NUR089's close at the analysis seam: a fn
// VALUE bound to an analysed body's param or capture is installed the way
// the run's frame installs it (core.InstallFrameBinding), so the body's
// CALL of the name matches. An inline lambda's authored signature is not
// dispatch-ready on its own; the raw push the pass used left `g 5` matching
// nothing.

// ffbLambda is `n:Integer => [n]` as afn builds it: the authored signature,
// never normalized.
func ffbLambda(r *core.Registry) core.Value {
	return core.NewFunction(core.FnDefInfo{Anonymous: true, Registry: r, Signatures: []core.FnSig{{
		Params:     []core.FnParam{{Name: "n", Type: core.TInteger}},
		Returns:    []*core.Type{core.TAny},
		Impl:       core.Boru([]core.Value{core.NewWord("n")}),
		BarrierPos: -1,
	}}})
}

func ffbNoSignature(r *core.Registry) bool {
	for _, d := range r.Check.Diagnostics {
		if d.Code == "no_signature" {
			return true
		}
	}
	return false
}

func TestRunFnBodyOnceCallsFnValueParam(t *testing.T) {
	r := zzmsReg(t)
	done := r.Check.Begin()
	defer done()
	body := []core.Value{core.NewWord("g"), core.NewInteger(5)}
	out := RunFnBodyOnce(r, "h", []string{"g"}, body, []core.Value{ffbLambda(r)}, nil, true)
	if ffbNoSignature(r) {
		t.Fatalf("the lambda bound to a param must be callable, got %v", r.Check.Diagnostics)
	}
	if len(out) != 1 || out[0].Parent == nil || !core.SigTypeMatches(out[0], core.TInteger) {
		t.Errorf("the call leaves the lambda's result, got %v", out)
	}
	if r.Defs.Has("g") {
		t.Error("the frame binding comes off with the body")
	}
}

func TestRunFnBodyOnceCallsFnValueCapture(t *testing.T) {
	r := zzmsReg(t)
	done := r.Check.Begin()
	defer done()
	body := []core.Value{core.NewWord("g"), core.NewInteger(5)}
	captures := []core.CapturedBinding{{Name: "g", Value: ffbLambda(r)}}
	out := RunFnBodyOnce(r, "h", nil, body, nil, captures, true)
	if ffbNoSignature(r) {
		t.Fatalf("the lambda bound to a capture must be callable, got %v", r.Check.Diagnostics)
	}
	if len(out) != 1 {
		t.Errorf("the call leaves the lambda's one result, got %v", out)
	}
}
