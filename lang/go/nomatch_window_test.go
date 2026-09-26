package lang

import (
	"strings"
	"testing"
)

// diagNotes renders a boru error's headline and notes — the lines the two
// lanes must describe alike — leaving out the position, source and caret
// lines, which legitimately differ by a column (NUR118) and, for NUR171's
// row, are still absent on the compiled lane.
func diagNotes(err error) string {
	if err == nil {
		return ""
	}
	var keep []string
	for i, line := range strings.Split(err.Error(), "\n") {
		if i == 0 || strings.HasPrefix(line, "  = note:") {
			keep = append(keep, line)
		}
	}
	return strings.Join(keep, "\n")
}

// TestNoMatchWindowIsTheAttemptedOne pins NUR172's close. A failed dispatch's
// report describes the window the dispatch ATTEMPTED on both lanes: `5
// $.name apply` — a lens's `dot` over the 5 with `name` written after it —
// used to report "the argument was 5 … takes 2 arguments, but 1 was
// supplied" interpreted (the window the collection managed to FILL) where
// the compiled poly window reported the two values the source wrote and the
// type failure on the second. The notes agree now (core.attemptedWindow); a
// pure stack no-match (`'x' add`) reports as it always did.
func TestNoMatchWindowIsTheAttemptedOne(t *testing.T) {
	for _, src := range []string{
		"5 $.name apply",
		"5 dot name",
		"'x' add",
	} {
		_, _, errC, _, errI := runBothEngines(t, src)
		if errC == nil || errI == nil {
			t.Fatalf("%q: both lanes must raise, got compiled=%v interp=%v", src, errC, errI)
		}
		if diagNotes(errC) != diagNotes(errI) {
			t.Errorf("%q: the lanes describe different windows\n compiled:\n%s\n interp:\n%s", src, diagNotes(errC), diagNotes(errI))
		}
	}
	_, _, _, _, errI := runBothEngines(t, "5 $.name apply")
	if want := "the arguments were name (an Atom) and 5 (an Integer)"; !strings.Contains(errI.Error(), want) {
		t.Errorf("interpreter must report the attempted window, got:\n%s", errI)
	}
}

// TestEmitDynamicLeadIsNotRoutedAsData pins NUR170's close: `emit m.up
// {a:1}` — a container-read fn value at the emit lead — used to be routed as
// DATA under analysis (`emitlang-auto` over the fn and the map transposed, a
// no-match at run time where the interpreter runs the emitter). The branch
// first closed it as a sound decline; main's #510 went further, and the
// compile pass records the module's runtime fn-dispatch over a gradual lead
// (TestMacroGradualLeadDispatchesAtRunTime), so the program COMPILES and
// answers as the interpreter. A bound data lead and a kind lead keep the
// verdict they had.
func TestEmitDynamicLeadIsNotRoutedAsData(t *testing.T) {
	requireEngineParity(t, `import "boru:emitlang" end def m {up: (fn [[value:Any opts:Map] [String] ['UP']])} end emit m.up {a:1}`, true)
	for _, src := range []string{
		`import "boru:emitlang" end def d {a:1} emit d`,
		`import "boru:emitlang" end emit json {a:1}`,
	} {
		requireSameVerdict(t, src)
	}
}
