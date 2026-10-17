package lang_test

import (
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
)

// TestEachBodyThatConsumesItsElementStillErrors is the negative half at the
// consumer: `each_error` must still fire when the body genuinely eats its
// element. Any fix for the divergence above has to keep this — the two cases
// are indistinguishable from the handler (`len(res) == 0`) and are told apart
// only by whether the element survived the frame, so a fix that simply
// silences the error would break this instead.
func TestEachBodyThatConsumesItsElementStillErrors(t *testing.T) {
	const src = `[1 2 3] each [ drop ]`
	for _, mode := range []string{"compiled", "interpreted"} {
		a, _ := lang.New()
		var err error
		if mode == "compiled" {
			_, err = a.Run(src)
		} else {
			_, err = a.RunInterp(src)
		}
		if err == nil {
			t.Errorf("%s: a body that consumes its element leaves each nothing to "+
				"collect and must raise", mode)
		}
	}
}
