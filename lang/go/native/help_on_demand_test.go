package native

import (
	"strings"
	"testing"

	"github.com/boru-lang/boru/lang/go/native/help"
)

// help_on_demand_test.go pins the two halves of on-demand help example
// generation: the register-time hook RECORDS, and FormatWordHelp EVALUATES,
// and nothing happens in between.
//
// The split exists because the hook fires from installFnDef on EVERY fn
// installation — the user's own `def f fn […]` included, DURING their check —
// and the old hook evaluated the synthesised example for real in the registry
// that was mid-pass. Measured, that cost three things at once: the example's
// own body analysis was memoised under a key rendering arg TYPE NAMES only, so
// the program's real call site took the memo hit and lost diagnostics; the
// isolation that fixed it exposed a latent unreachable_branch attribution bug;
// and `boru check` paid for every example on every run (kg/storage.boru is
// 0.26s on demand against 2.47s eager). Recording the name and generating when
// someone asks removes all three, because nothing synthetic runs in a check.

// hodArms builds a registry whose ParseFunc is unset, so makeDynamicEval
// declines — the arm that makes FormatWordHelp a plain render.
func hodArms() *Registry { return &Registry{} }

// TestFormatWordHelpDeclines pins the two arms that must NOT evaluate: a word
// nobody recorded (it has a build-time snapshot, or is not a word at all), and
// a registry with no parser to evaluate with. Both still RENDER — declining to
// generate is never declining to describe.
func TestFormatWordHelpDeclines(t *testing.T) {
	info := help.FuncInfo{
		Name:        "hod-unrecorded",
		ForwardArgs: true,
		Sigs: []help.SigInfo{{
			Args:    []string{"Integer", "Integer"},
			Returns: []string{"Integer"},
		}},
	}
	// Not recorded: IsHelpWord is false, so no eval is even built.
	r := hodArms()
	out := FormatWordHelp(r, info)
	if !strings.Contains(out, "hod-unrecorded") {
		t.Errorf("an unrecorded word must still render:\n%s", out)
	}
	// Recorded, but the registry has no ParseFunc — makeDynamicEval declines
	// and the render falls back to the computed placeholder.
	r.NoteHelpWord("hod-unrecorded")
	if got := FormatWordHelp(r, info); got != out {
		t.Errorf("a registry with no parser must render identically:\n%s\n---\n%s", got, out)
	}
}
