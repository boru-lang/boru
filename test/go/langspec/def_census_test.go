package langspec

import (
	"fmt"
	"os"
	"sort"
	"sync"
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
)

// def_census_test.go measures the def census over the corpus
// (design/IMMUTABLE-DEF.1.md §4, §5 phase 0): the rows the scope rule will
// make redefinition errors, undefined reads, warnings or removed constructs
// of, by class (core/go/def_census.go), before any of them changes. It is the
// migration list for phases 1 and 3, and a DOWNWARD ratchet on the whole:
// a row that acquires a finding is a change writing the shapes the rule
// forbids back into the corpus.
//
// `shadow` is excluded from the ceiling: an inner scope binding a name an
// enclosing scope binds is legal under the rule and stays legal, so it is
// reported, not counted. Every other class ends at 0 — `rebind`, `overlap`
// and `extend-inner` become `redefinition`, `leak-read` becomes
// `undefined_word`, `shadow-rebind` becomes a warning the corpus is rewritten
// not to raise, and `undef` and `var-construct` are removed.
//
// BORU_LOG_DEF_CENSUS=1 names every row with a finding and its findings, in
// file-then-line order — the listing the rewrite works from.

// defCensusRowCeiling is the number of corpus rows with at least one finding
// of a class other than `shadow`, measured 2026-10-06.
const defCensusRowCeiling = 144 // 151 -> 144 on 2026-10-07: the `var [[…]]` construct is gone (design/IMMUTABLE-DEF.1.md §5 phase 1), and with it the var-construct class's seven corpus rows; the construct rows rewritten as lambdas or stack-bound-def bodies add no finding of another class.

// defCensusClasses is the report's column order: the rule's verdicts from
// error to removal, then the legal shadow.
var defCensusClasses = []string{"rebind", "overlap", "extend-inner", "leak-read", "shadow-rebind", "undef", "var-construct", "shadow"}

func TestDefCensusCorpus(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	rows, counted := 0, 0
	classRows := map[string]int{}                // class → rows with a finding of it
	fileRows := map[string]int{}                 // file → counted rows
	fileClassRows := map[string]map[string]int{} // file → class → rows
	type censusRow struct {
		file string
		line int
		text string
	}
	var listing []censusRow

	specWalk(t, func(t testing.TB, r specRow) {
		if len(r.Cells) < 2 {
			return
		}
		a, err := lang.New()
		if err != nil {
			t.Fatal(err)
		}
		a.SetClock(specClock)
		res, _ := a.Check(r.Input)
		seen := map[string]bool{}
		var text string
		for _, e := range res.DefCensus {
			seen[string(e.Class)] = true
			text += fmt.Sprintf(" %s:%s@%d:%d", e.Class, e.Name, e.Site.Row, e.Site.Col)
		}

		mu.Lock()
		defer mu.Unlock()
		rows++
		count := false
		for c := range seen {
			classRows[c]++
			if fileClassRows[r.File] == nil {
				fileClassRows[r.File] = map[string]int{}
			}
			fileClassRows[r.File][c]++
			if c != "shadow" {
				count = true
			}
		}
		if !count {
			return
		}
		counted++
		fileRows[r.File]++
		if os.Getenv("BORU_LOG_DEF_CENSUS") != "" {
			listing = append(listing, censusRow{r.File, r.Line, fmt.Sprintf("DEF CENSUS %s:%s  %s", r.Key(), text, r.Input)})
		}
	})

	sort.Slice(listing, func(i, j int) bool {
		if listing[i].file != listing[j].file {
			return listing[i].file < listing[j].file
		}
		return listing[i].line < listing[j].line
	})
	for _, l := range listing {
		t.Log(l.text)
	}
	t.Logf("def census: %d rows, %d with a finding the rule forbids (ceiling %d)", rows, counted, defCensusRowCeiling)
	for _, c := range defCensusClasses {
		t.Logf("   class %-14s %d rows", c, classRows[c])
	}
	for _, f := range sortedKeys(fileRows) {
		if fileRows[f] >= 3 {
			t.Logf("   file %-34s %d rows   %s", f, fileRows[f], classBreakdown(fileClassRows[f]))
		}
	}

	gate(t, "def census rows", counted, 0, defCensusRowCeiling, true,
		"corpus rows the scope rule (design/IMMUTABLE-DEF.1.md) will reject or change: a module-scope rebind, an overlapping or inner overload, a read of a block's def after the block, a loop-counter shadow, an undef, a var construct")
}

// classBreakdown renders one file's class → rows map as "rebind 3, undef 1",
// largest first.
func classBreakdown(m map[string]int) string {
	keys := sortedKeys(m)
	sort.SliceStable(keys, func(i, j int) bool { return m[keys[i]] > m[keys[j]] })
	out := ""
	for i, k := range keys {
		if i > 0 {
			out += ", "
		}
		out += fmt.Sprintf("%s %d", k, m[k])
	}
	return out
}
