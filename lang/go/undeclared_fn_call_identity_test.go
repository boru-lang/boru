package lang

import (
	"fmt"
	"testing"
)

// TestUndeclaredFnRepeatedCallsParity pins NUR177 (fixed 2026-09-22): an
// UNDECLARED-return fn — a lambda-bodied `def w n:Integer => [...]`, whose
// result is its analysed body residual — called more than once in one
// program. AnalyseFnBody memoises the residual per arg shape and handed the
// SAME values to every call, and the recorder keys producers by value ID,
// so both calls seated one local and the program's residual pushed it twice:
// `(w 5) (w 0)` answered [3 3] for [8 3]. freshResidual (check_fnbody.go)
// gives each call its own result identities. Every row runs on both lanes
// and must agree.
func TestUndeclaredFnRepeatedCallsParity(t *testing.T) {
	for _, src := range []string{
		`def g x:Integer => [x add 3] end def w n:Integer => [(g n)] end (w 5) (w 0)`,
		`def w n:Integer => [if (n gt 0) [n add 3] [0]] end (w 5) (w 0)`,
		// a pass-through body: the result is the argument's value under its own identity
		`def w n:Integer => [n] end (w 5) (w 0)`,
		// three calls, two of one shape and one of another
		`def w n:Integer => [n mul 2] end (w 1) (w 2) (w 3)`,
		// two closures of one factory, each applied
		`def mk fn [[k:Integer][Function][([n:Integer] => [n add k])]] end def a (mk 1) end def b (mk 2) end (a 10) (b 10)`,
		// the same call feeding a binary word
		`def w n:Integer => [n add 1] end (w 1) add (w 2)`,
	} {
		gotI, errI := mustNew(t).RunInterp(src)
		gotC, compiled, errC := mustNew(t).RunCompiled(src)
		if noteCompileDefect(t, src, gotC, errC) {
			t.Errorf("%s: did not compile: %v", src, errC)
			continue
		}
		if !compiled {
			t.Fatalf("%s: did not run compiled (%v)", src, errC)
		}
		if codeOf(errI) != codeOf(errC) || fmt.Sprint(gotI) != fmt.Sprint(gotC) {
			t.Errorf("%s:\n  interp   %v err=[%s]\n  compiled %v err=[%s]", src, gotI, codeOf(errI), gotC, codeOf(errC))
		}
	}
}
