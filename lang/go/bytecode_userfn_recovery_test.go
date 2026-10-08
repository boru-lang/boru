package lang

import (
	"fmt"
	"strings"
	"testing"
)

// TestSingleOverloadUserFnRecovery pins the single-overload user-fn recovery fix
// (engine.go checkModeAssumeSig). When a user fn is dispatched over an Any-typed
// operand (e.g. a `get` over a List/Map element), matchSignature can't statically
// commit, so the engine's recovery branch runs. tryRecordPoly can't take a user fn
// (FnFrame), so it used to MarkUncompilable ("unmatched dispatch recovered at X").
// For a SINGLE-overload fn with no fallback that is unsound only statically — at
// run time the concrete arg matches the sole sig (dispatch == interpreter) or
// misses it (the VM's CALL_USER param contract raises == interpreter no_signature).
// The fix drives its ReturnsFn to record a GUARDED CALL_USER instead of declining.
//
// This is the leaf that gated the boru:test framework (run-cases) and the trie/
// decision walkers (find-kid / mk-tnode / lex-mustache).
func TestSingleOverloadUserFnRecovery(t *testing.T) {
	// MUST compile natively (RunCompiledStrict) — the recovery now records a call.
	strict := []struct{ name, src, want string }{
		{"single-overload over Any list-get", `def total fn [[| xs:List][Integer][ (xs size) ]] def d [[1 2 3]] ((d get 0)) total`, "[3]"},
		{"single-overload first-element over Any", `def first fn [[| xs:List][Any][ (xs get 0) ]] def d [[9 8 7]] ((d get 0)) first`, "[9]"},
		{"single-overload nested get (find-kid shape)", `def fk fn [[nd:Map ch:String][Any][ (nd get "kids") get ch ]] def t [{kids:{a:7}}] fk (t get 0) "a"`, "[7]"},
	}
	for _, c := range strict {
		t.Run("strict/"+c.name, func(t *testing.T) {
			a, _ := New()
			prog, reason, _, _ := a.CompileCheck(c.src)
			if prog == nil {
				t.Fatalf("must compile, declined: %q", reason)
			}
			if strings.Contains(prog.Disassemble(), "FALLBACK") {
				t.Errorf("%s must compile native (no island)", c.name)
			}
			got, err := a.RunCompiledStrict(c.src)
			if err != nil {
				t.Fatalf("RunCompiledStrict: %v", err)
			}
			b, _ := New()
			want, _ := b.RunInterp(c.src)
			if fmt.Sprint(got) != fmt.Sprint(want) {
				t.Errorf("compiled %v != interpreter %v", got, want)
			}
			if fmt.Sprint(got) != c.want {
				t.Errorf("got %v, want %s", got, c.want)
			}
		})
	}

	// A MULTI-overload fn (or one with an Any catch-all sibling) used to DECLINE
	// here — baking ONE overload was unsound (the interpreter runtime-dispatches
	// a sibling where the baked guard would raise). OpCallUserPoly now bakes
	// EVERY same-arity arm and re-runs the interpreter's MatchSignature at
	// entry, so these shapes COMPILE STRICTLY and must match the interpreter —
	// a strictly stronger pin than the old compile failure (arm-direction / no-match /
	// drift-fallback pins live in bytecode_user_poly_test.go).
	poly := []struct{ name, src string }{
		{"multi-overload over Any (Cluster C)", `def f fn [[s:String][Integer][1] [n:Integer][Integer][2]] (f ({b:2} getr "b"))`},
		{"fn with Any catch-all overload", `def g fn [[n:Integer][String]["i"] [x:Any][String]["fb"]] (g ({b:2} getr "b"))`},
	}
	for _, c := range poly {
		t.Run("poly/"+c.name, func(t *testing.T) {
			a, _ := New()
			got, _, err := a.RunCompiled(c.src) // fallback allowed
			if err != nil {
				t.Fatalf("RunCompiled error: %v", err)
			}
			b, _ := New()
			want, werr := b.RunInterp(c.src)
			if werr != nil {
				t.Fatalf("interpreter error: %v", werr)
			}
			if fmt.Sprint(got) != fmt.Sprint(want) {
				t.Errorf("compiled %v != interpreter %v", got, want)
			}
			// And it must now compile strictly, with the SAME result — the
			// user-poly re-match owns the dispatch the old compile failure deferred.
			sgot, serr := a.RunCompiledStrict(c.src)
			if serr != nil {
				t.Errorf("%s must compile strictly under the user-poly re-match, got: %v", c.name, serr)
			} else if fmt.Sprint(sgot) != fmt.Sprint(want) {
				t.Errorf("strict-compiled %v != interpreter %v", sgot, want)
			}
		})
	}
}
