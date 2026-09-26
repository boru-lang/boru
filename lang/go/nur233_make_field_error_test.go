package lang

import (
	"errors"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// TestNUR233MakeFieldRefusalIsATypeError pins NUR233: a make field the run
// refuses — a value only the run knows, loop-carried here, or a field typed
// by a refinement whose bound only the run computes (NUR231) — raised a
// plain error, which the interpreter printed bare and a compiled run booked
// as a compiler defect (internal_error, with the "please report it" note).
// The refusal is a type_error on both lanes now, the code the check pass
// already gave the same refusal over a value it knows.
func TestNUR233MakeFieldRefusalIsATypeError(t *testing.T) {
	for _, src := range []string{
		`def Big (Integer gt 100) def S class {x:Big} def n 0 for 3 [def n (n add 1)] make S {x:n}`,
		`def Big (Integer gt (size "abc")) def S class {x:Big} make S {x:2}`,
	} {
		agreeOnBothLanes(t, src, `ERROR:make: field "x": expected Big, got Integer`)
		_, _, errC, _, errI := runBothEngines(t, src)
		for lane, err := range map[string]error{"compiled": errC, "interpreter": errI} {
			var ae *core.BoruError
			if !errors.As(err, &ae) || ae.Code != "type_error" || len(ae.Notes) != 0 {
				t.Errorf("%s: %s lane raised %v, want a type_error with no defect note", src, lane, err)
			}
		}
	}
}
