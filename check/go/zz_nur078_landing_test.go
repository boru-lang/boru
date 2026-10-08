package check

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// TestLandingNextForWordReadsTheSpelling pins NUR078's landing rule: what
// follows a landed value is classified the way the re-step's forward phase
// reads it. A `/v` word denotes its binding's VALUE — the collected
// reference, whatever the binding holds — while the same name bare is a
// function word the phase stops at; a word bound to a plain value is
// collected either way.
func TestLandingNextForWordReadsTheSpelling(t *testing.T) {
	r := newTestRegistry(t)
	e := core.NewTop(r)
	r.Defs.Push("zf", core.NewFunction(core.FnDefInfo{Name: "zf"}))
	r.Defs.Push("zk", core.NewInteger(2))
	if got := landingNextForWord(e, core.NewWordRef("zf")); got != core.LandingNextValue {
		t.Errorf("zf/v after a landed value: %v, want the collected value", got)
	}
	if got := landingNextForWord(e, core.NewWord("zf")); got != core.LandingNextWord {
		t.Errorf("a bare fn name after a landed value: %v, want a function word", got)
	}
	if got := landingNextForWord(e, core.NewWord("zk")); got != core.LandingNextValue {
		t.Errorf("a value-bound word after a landed value: %v, want the collected value", got)
	}
}
