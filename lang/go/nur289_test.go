package lang

import (
	"errors"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// TestNUR289LandingWalkPlansTheInstalledSigs pins NUR289's close. A `do`
// body's result is re-stepped where it lands, and an anonymous fn whose
// Any slot then meets a function word begins a forward collection the word's
// barrier strands: `def mk fn [[][Any][([x:Any] => [x])]] end do [mk]
// typeof` is the interpreter's strict-rule signature_error. The compiled
// landing's walk planned over the value's AUTHORED signature, whose
// `BarrierAllForward` (-1) scanned nothing forward, so it parked the fn and
// answered `[Function]`. It plans over the installed view now, the sentinel
// resolved to the arity as the interpreter's dispatch resolves it. A typed
// slot the word does not fit, a `/q` capture, a Function slot and a value
// written after the result answer as before.
func TestNUR289LandingWalkPlansTheInstalledSigs(t *testing.T) {
	const mk = `def mk fn [[][Any][([x:Any] => [x])]] end `
	for _, src := range []string{
		mk + `do [mk] typeof`,
		mk + `do [(mk)] typeof`,
		`def l [([x:Any] => [x])] end do [l.0] typeof`,
	} {
		gotC, _, errC, gotI, errI := runBothEngines(t, src)
		if codeOf(errI) != "signature_error" || codeOf(errC) != codeOf(errI) || detailOf(errC) != detailOf(errI) || len(gotC) != 0 {
			t.Errorf("%s: the strict-rule strand on both lanes; got %v %v / %v %v", src, gotC, errC, gotI, errI)
		}
	}
	// The caret: the interpreter raises where the landed fn stands on its
	// tape — a `do` lands its body's lambda at the `do` — and so does the
	// walk (LandingWord.ValPos).
	for _, src := range []string{mk + `do [mk] typeof`, mk + `do [(mk)] typeof`} {
		_, _, errC, _, errI := runBothEngines(t, src)
		var bc, bi *core.BoruError
		if !errors.As(errC, &bc) || !errors.As(errI, &bi) || bc.Row == 0 || bc.Row != bi.Row || bc.Col != bi.Col {
			t.Errorf("%s: the strand raises at the landed fn's position on both lanes; compiled %v, interpreted %v", src, errC, errI)
		}
	}
	for _, c := range []struct{ src, want string }{
		{`def mk fn [[][Any][([x:Integer] => [x])]] end do [mk] typeof`, "[Function]"},
		{`def mk fn [[][Any][([x:Atom/q] => [x])]] end do [mk] typeof`, "[typeof]"},
		{`def mk fn [[][Any][([x:Function] => [x])]] end do [mk] typeof`, "[Function]"},
		{mk + `do [mk] 5`, "[5]"},
		{mk + `do [mk]`, "[fn (Any)]"},
		{mk + `(mk) typeof`, "[Function]"},
		{mk + `mk typeof`, "[Function]"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
}
