package lang

import (
	"fmt"
	"strings"
	"testing"
)

// TestNUR253ZeroOutPhantomIsOnNoRunsStack pins NUR253: a 0-output statement
// guard — an `if` whose arms leave nothing — registers a phantom None the
// recorder elides its dispatch by, and the phantom sits on the compiling
// pass's tape though it is on no run's stack. Two readers took it for a
// value:
//
//   - the full-stack fold (FoldFullStack) counted it: `if true [def k 1] []
//     end depth` answered [1] compiled for the interpreter's [0] — silent. The
//     count and a shuffle's index skip it now, the phantom kept in place;
//   - a definite no-match's recorded diagnostic listed it: "the arguments
//     were 'x' (a ProperString) and None (a None)" where the interpreter
//     reports "the argument was 'x'". The report's stack prefix is the run's
//     now (runPrefix).
func TestNUR253ZeroOutPhantomIsOnNoRunsStack(t *testing.T) {
	const void = `if true [def k 1] [] end `
	for _, r := range []struct{ src, want string }{
		{void + `depth`, "[0]"},
		{`1 ` + void + `2 depth`, "[1 2 2]"},
		{`def m {e: true} if (m 'e' get) [def k 1] [] end depth`, "[0]"},
		{void + `5 6 1 pick`, "[5 6 5]"},
		{`5 ` + void + `6 1 pick`, "[5 6 5]"},
		{`5 ` + void + `6 7 2 pick`, "[5 6 7 5]"},
		{`5 ` + void + `6 1 roll`, "[6 5]"},
		{void + `5 6 0 roll`, "[5 6]"},
		// The recorded no-match reports the run's arguments.
		{void + `def f fn [[a:Integer] [Any] [7]] end "x" f`, "ERROR:cannot call `f`"},
		{`if false [def f fn [[a:Integer] [Any] [7]]] [def f fn [[a:Integer] [Any] [8]]] end "x" f`, "ERROR:cannot call `f`"},
	} {
		agreeOnBothLanes(t, r.src, r.want)
	}
	// The report names the one argument the run holds.
	src := void + `def f fn [[a:Integer] [Any] [7]] end "x" f`
	if _, _, errC := mustNew(t).RunCompiled(src); !strings.Contains(fmt.Sprint(errC), "the argument was 'x' (a ProperString)") {
		t.Errorf("%s: the compiled report must name the run's one argument, got %v", src, errC)
	}
	// A shuffle past the run's own entries raises on the interpreter; the
	// fold declines rather than pick the phantom.
	src = void + `5 1 pick`
	if _, errI := mustNew(t).RunInterp(src); errI == nil || !strings.Contains(errI.Error(), "out of range") {
		t.Errorf("%s: interp must raise out of range, got %v", src, errI)
	}
	gotC, compiled, errC := mustNew(t).RunCompiled(src)
	if !noteCompileDefect(t, src, gotC, errC) || compiled {
		t.Errorf("%s: the fold must not pick the phantom, got %v compiled=%v err=%v", src, gotC, compiled, errC)
	}
}
