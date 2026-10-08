package eng

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// TestSplitReportTape pins the report tape a split no-match renders over
// (PolySplit.Words, NUR356): each written operand read from a word is put
// back as that word; an index past the tape, or none, leaves it as it was.
func TestSplitReportTape(t *testing.T) {
	win := core.NewTape([]core.Value{core.NewWord("each"), core.NewInteger(3), core.NewInteger(5)}, core.StackHeadroom)
	if splitReportTape(win, 0, nil) != win {
		t.Error("no words: the plan's own tape")
	}
	got := splitReportTape(win, 0, map[int]core.Value{1: core.NewWord("l"), 7: core.NewWord("far")})
	if got == win || got.Len() != 3 || !core.IsWord(got.At(2)) || core.IsWord(win.At(2)) {
		t.Errorf("the word at written index 1 stands at tape 2: %v", got)
	}
}
