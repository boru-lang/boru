package lang

import (
	"fmt"
	"strings"
	"testing"
)

// deferred_rematch_test.go is the whole-program half of the forty-sixth
// increment: an unmatched dispatch whose window holds a DEFERRED EXPRESSION
// — a reach path, an unexpanded paren expr, a template string.
//
// TryRecordUnmatchedDispatchTrap declined such a window outright, and the
// reason it gave was sound but aimed at the wrong instrument: a deferred
// token EXPANDS at dispatch time, so the static trap it would bake could
// describe a value the runtime never sees. The runtime REMATCH reads no
// static tag at all — it re-runs the match over the values the interpreter's
// own dispatch examines — so it defers where the expansion matches and
// raises the byte-identical error where it does not.

// drRun runs a source on both lanes and reports whether the compiled lane
// ran natively and whether the two agree byte for byte (values AND error
// text, which for these rows is the whole point).
func drRun(t *testing.T, src string) (ran bool, agree bool, cerr error) {
	t.Helper()
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	gotC, ran, cerr := a.RunCompiled(src)
	b, err := New()
	if err != nil {
		t.Fatal(err)
	}
	gotI, ierr := b.RunInterp(src)
	agree = fmt.Sprint(gotC) == fmt.Sprint(gotI) && fmt.Sprint(cerr) == fmt.Sprint(ierr)
	return ran, agree, cerr
}

// TestDeferredWindowRematchRaisesByteIdentically — the raising direction.
// These rows used to refuse the WHOLE program; they now compile and raise
// the interpreter's own signature_error, caret and candidate notes included.
func TestDeferredWindowRematchRaisesByteIdentically(t *testing.T) {
	for _, src := range []string{
		// The two ledger rows: a forward lens at stack-only `apply`.
		`def p {name:'ada'}  p apply $.name`,
		`[10 20 30] apply $.1`,
		// A prefix before the failing dispatch still runs — the raise is at
		// the same point on both lanes, not at the top of the program.
		`9 [10 20 30] apply $.1`,
	} {
		t.Run(src, func(t *testing.T) {
			ran, agree, cerr := drRun(t, src)
			if !ran {
				t.Fatalf("the deferred window must take the rematch, not the whole-program fallback: %v", cerr)
			}
			if cerr == nil {
				t.Fatal("the dispatch must still raise")
			}
			if !strings.Contains(cerr.Error(), "no signature matches") {
				t.Errorf("error drifted: %v", cerr)
			}
			if !agree {
				t.Error("the compiled raise must be byte-identical to the interpreter's")
			}
		})
	}
}

// TestDeferredWindowRematchDefersWhenItMatches — the other direction, and
// the one the old decline was protecting. A reach over a MUTATED flex cell
// resolves at run time to something the static match never saw; the rematch
// re-runs over the expanded value and hands the dispatch through.
func TestDeferredWindowRematchDefersWhenItMatches(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`def m {a:{b:1}} def f (flex m) set b/q 9 f.a drop f.a.b`, "[9]"},
		{`def m {a:{b:1}} def f (flex m) set b/q 9 f.a drop m.a.b`, "[1]"},
		// The lens spellings that always matched are untouched.
		{`def p {name:'ada'}  p $.name apply`, "[ada]"},
		{`[10 20 30] $.1 apply`, "[20]"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			a, err := New()
			if err != nil {
				t.Fatal(err)
			}
			out, _, cerr := a.RunCompiled(tc.src)
			if cerr != nil {
				t.Fatalf("RunCompiled: %v", cerr)
			}
			if got := fmt.Sprintf("%v", out); got != tc.want {
				t.Errorf("compiled %s, want %s", got, tc.want)
			}
			b, err := New()
			if err != nil {
				t.Fatal(err)
			}
			outi, ierr := b.RunInterp(tc.src)
			if ierr != nil {
				t.Fatalf("RunInterp: %v", ierr)
			}
			if got := fmt.Sprintf("%v", outi); got != tc.want {
				t.Errorf("interpreter %s, want %s — the oracle moved", got, tc.want)
			}
		})
	}
}

// TestReorderHintWindowKeepsItsRefusal is the line the rematch does not
// cross, and it is structural rather than incidental. When the operands are
// in the wrong ORDER, sigError attaches a reorder hint ("did you swap the
// arguments?") derived from TAPE STATE — which the runtime rebuild has no
// access to, so it could not reproduce the error byte for byte. The rematch
// declines on exactly that, and the interpreter answers.
func TestReorderHintWindowKeepsItsRefusal(t *testing.T) {
	const src = `$.1 [10 20 30] apply`
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	prog, reason, _, cerr := a.CompileCheck(src)
	if cerr != nil {
		t.Fatalf("check: %v", cerr)
	}
	if prog != nil {
		t.Fatal("a window carrying a reorder hint must keep the refusal — the hint reads tape state")
	}
	if !strings.Contains(reason, "unmatched dispatch recovered at apply") {
		t.Errorf("refusal reason drifted: %q", reason)
	}
	b, err := New()
	if err != nil {
		t.Fatal(err)
	}
	_, ierr := b.RunInterp(src)
	if ierr == nil || !strings.Contains(ierr.Error(), "did you swap the arguments?") {
		t.Errorf("the interpreter must still raise with the reorder hint: %v", ierr)
	}
}
